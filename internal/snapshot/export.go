// Package snapshot exports and imports portable lantern bundles.
package snapshot

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Meru143/lantern/internal/store"
	_ "modernc.org/sqlite"
)

var (
	snapshotUUIDPattern   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	snapshotHashPattern   = regexp.MustCompile(`(?i)\b[0-9a-f]{16,}\b`)
	snapshotNumberPattern = regexp.MustCompile(`\b\d+\b`)
	snapshotSpacePattern  = regexp.MustCompile(`\s+`)
)

// ExportMetadata describes the metadata written alongside a snapshot database.
type ExportMetadata struct {
	Version         string `json:"version"`
	ExportedAt      string `json:"exportedAt"`
	DurationSeconds int64  `json:"durationSeconds"`
	SpansCount      int    `json:"spansCount"`
	LogsCount       int    `json:"logsCount"`
}

// Export creates a filtered .lantern snapshot bundle for the requested duration window.
func Export(ctx context.Context, dbPath string, duration time.Duration, outPath string) error {
	if duration <= 0 {
		return fmt.Errorf("LT006: invalid snapshot duration %s", duration)
	}
	if dbPath == "" {
		return fmt.Errorf("LT006: database path is empty")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return wrapSnapshotIOError(dbPath, err)
	}

	tempDir, err := os.MkdirTemp("", "lantern-snapshot-*")
	if err != nil {
		return wrapSnapshotIOError("", err)
	}
	defer os.RemoveAll(tempDir)

	sourceCopyPath := filepath.Join(tempDir, "source-copy.db")
	filteredPath := filepath.Join(tempDir, "data.db")

	if err := vacuumInto(ctx, dbPath, sourceCopyPath); err != nil {
		return err
	}

	copyDB, err := store.Open(sourceCopyPath)
	if err != nil {
		return wrapSnapshotIOError(sourceCopyPath, err)
	}
	defer copyDB.Close()

	filteredDB, err := store.Open(filteredPath)
	if err != nil {
		return wrapSnapshotIOError(filteredPath, err)
	}
	if err := store.RunMigrations(filteredDB); err != nil {
		_ = filteredDB.Close()
		return wrapSnapshotIOError(filteredPath, err)
	}

	cutoff := time.Now().Add(-duration)
	spansCount, err := copySpans(ctx, copyDB, filteredDB, cutoff)
	if err != nil {
		_ = filteredDB.Close()
		return err
	}
	logsCount, err := copyLogs(ctx, copyDB, filteredDB, cutoff)
	if err != nil {
		_ = filteredDB.Close()
		return err
	}
	if err := rebuildErrorGroups(ctx, filteredDB); err != nil {
		_ = filteredDB.Close()
		return err
	}
	if err := rebuildQueryStats(ctx, filteredDB); err != nil {
		_ = filteredDB.Close()
		return err
	}

	if _, err := filteredDB.ExecContext(ctx, `PRAGMA wal_checkpoint(FULL);`); err != nil {
		_ = filteredDB.Close()
		return wrapSnapshotIOError(filteredPath, err)
	}
	if err := filteredDB.Close(); err != nil {
		return wrapSnapshotIOError(filteredPath, err)
	}

	meta := ExportMetadata{
		Version:         "1",
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		DurationSeconds: int64(duration.Seconds()),
		SpansCount:      spansCount,
		LogsCount:       logsCount,
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return wrapSnapshotIOError(outPath, err)
	}
	if err := writeBundle(outPath, filteredPath, meta); err != nil {
		return err
	}

	return nil
}

func vacuumInto(ctx context.Context, sourcePath, targetPath string) error {
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return wrapSnapshotIOError(sourcePath, err)
	}
	defer db.Close()

	quotedTarget := strings.ReplaceAll(targetPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+quotedTarget+"'"); err != nil {
		return wrapSnapshotIOError(targetPath, err)
	}

	return nil
}

func copySpans(ctx context.Context, source, target *sql.DB, cutoff time.Time) (int, error) {
	rows, err := source.QueryContext(
		ctx,
		`SELECT trace_id, span_id, parent_span_id, service_name, name, kind,
		        start_time, end_time, status_code, status_message, attributes, events
		 FROM spans
		 WHERE end_time >= ?
		 ORDER BY start_time ASC`,
		cutoff.UnixNano(),
	)
	if err != nil {
		return 0, wrapSnapshotIOError("spans", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var span store.SpanRecord
		if err := rows.Scan(
			&span.TraceID,
			&span.SpanID,
			&span.ParentSpanID,
			&span.ServiceName,
			&span.Name,
			&span.Kind,
			&span.StartTimeUnix,
			&span.EndTimeUnix,
			&span.StatusCode,
			&span.StatusMessage,
			&span.Attributes,
			&span.Events,
		); err != nil {
			return 0, wrapSnapshotIOError("spans", err)
		}
		if err := store.InsertSpan(ctx, target, span); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, wrapSnapshotIOError("spans", err)
	}

	return count, nil
}

func copyLogs(ctx context.Context, source, target *sql.DB, cutoff time.Time) (int, error) {
	rows, err := source.QueryContext(
		ctx,
		`SELECT id, trace_id, span_id, service_name, timestamp, severity_num, severity_text, body, attributes
		 FROM log_records
		 WHERE timestamp >= ?
		 ORDER BY timestamp ASC`,
		cutoff.UnixNano(),
	)
	if err != nil {
		return 0, wrapSnapshotIOError("log_records", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var record store.LogRecord
		if err := rows.Scan(
			&record.ID,
			&record.TraceID,
			&record.SpanID,
			&record.ServiceName,
			&record.Timestamp,
			&record.SeverityNum,
			&record.SeverityText,
			&record.Body,
			&record.Attributes,
		); err != nil {
			return 0, wrapSnapshotIOError("log_records", err)
		}
		if err := store.InsertLog(ctx, target, record); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, wrapSnapshotIOError("log_records", err)
	}

	return count, nil
}

func rebuildErrorGroups(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM error_groups`); err != nil {
		return wrapSnapshotIOError("error_groups", err)
	}

	type aggregate struct {
		excType      string
		messageNorm  string
		count        int
		firstSeen    int64
		lastSeen     int64
		services     []string
		sampleSpanID string
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT span_id, service_name, end_time, events
		 FROM spans
		 WHERE events <> ''`,
	)
	if err != nil {
		return wrapSnapshotIOError("spans", err)
	}
	defer rows.Close()

	groups := make(map[string]*aggregate)
	for rows.Next() {
		var (
			spanID    string
			service   string
			timestamp int64
			eventsRaw string
		)
		if err := rows.Scan(&spanID, &service, &timestamp, &eventsRaw); err != nil {
			return wrapSnapshotIOError("spans", err)
		}

		events, err := parseExceptionEvents(eventsRaw)
		if err != nil {
			return err
		}
		for _, event := range events {
			fingerprint := store.FingerprintError(event.excType, event.message)
			group := groups[fingerprint]
			if group == nil {
				group = &aggregate{
					excType:      event.excType,
					messageNorm:  normalizeSnapshotErrorMessage(event.message),
					firstSeen:    timestamp,
					lastSeen:     timestamp,
					sampleSpanID: spanID,
				}
				groups[fingerprint] = group
			}
			group.count++
			if timestamp < group.firstSeen {
				group.firstSeen = timestamp
			}
			if timestamp > group.lastSeen {
				group.lastSeen = timestamp
			}
			group.services = appendUnique(group.services, service)
		}
	}
	if err := rows.Err(); err != nil {
		return wrapSnapshotIOError("spans", err)
	}

	for fingerprint, group := range groups {
		servicesJSON, err := json.Marshal(group.services)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO error_groups (
				fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			fingerprint,
			group.excType,
			group.messageNorm,
			group.count,
			group.firstSeen,
			group.lastSeen,
			string(servicesJSON),
			group.sampleSpanID,
		); err != nil {
			return wrapSnapshotIOError("error_groups", err)
		}
	}

	return nil
}

func rebuildQueryStats(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM db_queries`); err != nil {
		return wrapSnapshotIOError("db_queries", err)
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT trace_id, span_id, start_time, end_time, attributes
		 FROM spans
		 WHERE attributes LIKE '%"db.system"%'`,
	)
	if err != nil {
		return wrapSnapshotIOError("spans", err)
	}
	defer rows.Close()

	for rows.Next() {
		var span store.SpanRecord
		if err := rows.Scan(&span.TraceID, &span.SpanID, &span.StartTimeUnix, &span.EndTimeUnix, &span.Attributes); err != nil {
			return wrapSnapshotIOError("spans", err)
		}
		if err := store.RecordDBSpan(ctx, db, span); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return wrapSnapshotIOError("spans", err)
	}

	return nil
}

type exceptionEvent struct {
	excType string
	message string
}

func parseExceptionEvents(raw string) ([]exceptionEvent, error) {
	if raw == "" {
		return nil, nil
	}

	var decoded []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, wrapSnapshotIOError("events", err)
	}

	result := make([]exceptionEvent, 0)
	for _, event := range decoded {
		name, _ := event["name"].(string)
		if name != "exception" {
			continue
		}
		attributes, _ := event["attributes"].(map[string]interface{})
		excType, _ := attributes["exception.type"].(string)
		message, _ := attributes["exception.message"].(string)
		if excType == "" || message == "" {
			continue
		}
		result = append(result, exceptionEvent{excType: excType, message: message})
	}

	return result, nil
}

func normalizeSnapshotErrorMessage(message string) string {
	normalized := snapshotUUIDPattern.ReplaceAllString(message, "<uuid>")
	normalized = snapshotHashPattern.ReplaceAllString(normalized, "<hash>")
	normalized = snapshotNumberPattern.ReplaceAllString(normalized, "?")
	normalized = snapshotSpacePattern.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func writeBundle(outPath, filteredDBPath string, meta ExportMetadata) error {
	file, err := os.Create(outPath)
	if err != nil {
		return wrapSnapshotIOError(outPath, err)
	}
	defer file.Close()

	archive := zip.NewWriter(file)
	defer archive.Close()

	dataEntry, err := archive.Create("data.db")
	if err != nil {
		return wrapSnapshotIOError(outPath, err)
	}
	if err := copyFileToWriter(dataEntry, filteredDBPath); err != nil {
		return err
	}

	metaEntry, err := archive.Create("meta.json")
	if err != nil {
		return wrapSnapshotIOError(outPath, err)
	}
	if err := json.NewEncoder(metaEntry).Encode(meta); err != nil {
		return wrapSnapshotIOError(outPath, err)
	}

	if err := archive.Close(); err != nil {
		return wrapSnapshotIOError(outPath, err)
	}

	return nil
}

func copyFileToWriter(destination io.Writer, sourcePath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return wrapSnapshotIOError(sourcePath, err)
	}
	defer file.Close()

	if _, err := io.Copy(destination, file); err != nil {
		return wrapSnapshotIOError(sourcePath, err)
	}

	return nil
}

func wrapSnapshotIOError(path string, err error) error {
	if path == "" {
		return fmt.Errorf("LT006: %w", err)
	}
	return fmt.Errorf("LT006: %s: %w", path, err)
}
