// Package store fingerprints and groups errors extracted from spans.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	uuidPattern   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	hashPattern   = regexp.MustCompile(`(?i)\b[0-9a-f]{16,}\b`)
	numberPattern = regexp.MustCompile(`\b\d+\b`)
	spacePattern  = regexp.MustCompile(`\s+`)
)

// ErrorGroup stores grouped exceptions by normalized fingerprint.
type ErrorGroup struct {
	Fingerprint  string   `json:"fingerprint"`
	ExcType      string   `json:"excType"`
	MessageNorm  string   `json:"messageNorm"`
	Count        int      `json:"count"`
	FirstSeen    int64    `json:"firstSeen"`
	LastSeen     int64    `json:"lastSeen"`
	Services     []string `json:"services"`
	SampleSpanID string   `json:"sampleSpanId"`
}

// FingerprintError returns a stable fingerprint for an exception type and normalized message.
func FingerprintError(excType, excMessage string) string {
	normalized := normalizeErrorMessage(excMessage)
	sum := sha256.Sum256([]byte(excType + "|" + normalized))
	return hex.EncodeToString(sum[:])
}

// UpsertErrorGroup inserts or updates an aggregated error group.
func UpsertErrorGroup(ctx context.Context, db *sql.DB, spanID, excType, excMessage, service string) error {
	fingerprint := FingerprintError(excType, excMessage)
	messageNorm := normalizeErrorMessage(excMessage)
	now := time.Now().UnixNano()

	return withWriteRetry(ctx, func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		var (
			group       ErrorGroup
			serviceJSON string
		)
		err = tx.QueryRowContext(
			ctx,
			`SELECT fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
			 FROM error_groups
			 WHERE fingerprint = ?`,
			fingerprint,
		).Scan(
			&group.Fingerprint,
			&group.ExcType,
			&group.MessageNorm,
			&group.Count,
			&group.FirstSeen,
			&group.LastSeen,
			&serviceJSON,
			&group.SampleSpanID,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if errors.Is(err, sql.ErrNoRows) {
			servicesJSON, marshalErr := json.Marshal([]string{service})
			if marshalErr != nil {
				return marshalErr
			}
			_, execErr := tx.ExecContext(
				ctx,
				`INSERT INTO error_groups (
					fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				fingerprint,
				excType,
				messageNorm,
				1,
				now,
				now,
				string(servicesJSON),
				spanID,
			)
			if execErr != nil {
				return execErr
			}
			return tx.Commit()
		}

		services, parseErr := decodeStringSlice(serviceJSON)
		if parseErr != nil {
			return parseErr
		}
		services = appendUniqueString(services, service)
		servicesJSON, marshalErr := json.Marshal(services)
		if marshalErr != nil {
			return marshalErr
		}

		_, execErr := tx.ExecContext(
			ctx,
			`UPDATE error_groups
			 SET count = ?,
			     last_seen = ?,
			     services = ?,
			     sample_span_id = CASE WHEN sample_span_id = '' THEN ? ELSE sample_span_id END
			 WHERE fingerprint = ?`,
			group.Count+1,
			now,
			string(servicesJSON),
			spanID,
			fingerprint,
		)
		if execErr != nil {
			return execErr
		}

		return tx.Commit()
	})
}

// ListErrorGroups returns grouped errors ordered by occurrence count descending.
func ListErrorGroups(ctx context.Context, db *sql.DB) ([]ErrorGroup, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
		 FROM error_groups
		 ORDER BY count DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list error groups: %w", err)
	}
	defer rows.Close()

	var groups []ErrorGroup
	for rows.Next() {
		var (
			group       ErrorGroup
			serviceJSON string
		)
		if err := rows.Scan(
			&group.Fingerprint,
			&group.ExcType,
			&group.MessageNorm,
			&group.Count,
			&group.FirstSeen,
			&group.LastSeen,
			&serviceJSON,
			&group.SampleSpanID,
		); err != nil {
			return nil, fmt.Errorf("scan error group: %w", err)
		}
		group.Services, err = decodeStringSlice(serviceJSON)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate error groups: %w", err)
	}

	return groups, nil
}

// GetErrorGroup returns a single grouped error by fingerprint.
func GetErrorGroup(ctx context.Context, db *sql.DB, fingerprint string) (*ErrorGroup, error) {
	var (
		group       ErrorGroup
		serviceJSON string
	)
	err := db.QueryRowContext(
		ctx,
		`SELECT fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
		 FROM error_groups
		 WHERE fingerprint = ?`,
		fingerprint,
	).Scan(
		&group.Fingerprint,
		&group.ExcType,
		&group.MessageNorm,
		&group.Count,
		&group.FirstSeen,
		&group.LastSeen,
		&serviceJSON,
		&group.SampleSpanID,
	)
	if err != nil {
		return nil, err
	}

	group.Services, err = decodeStringSlice(serviceJSON)
	if err != nil {
		return nil, err
	}

	return &group, nil
}

func normalizeErrorMessage(message string) string {
	normalized := uuidPattern.ReplaceAllString(message, "<uuid>")
	normalized = hashPattern.ReplaceAllString(normalized, "<hash>")
	normalized = numberPattern.ReplaceAllString(normalized, "?")
	normalized = spacePattern.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

func decodeStringSlice(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("decode string slice: %w", err)
	}
	sort.Strings(values)
	return values, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}
