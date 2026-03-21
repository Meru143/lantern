// Package store analyzes database spans and exposes normalized query statistics.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	sqlStringPattern = regexp.MustCompile(`'([^']|'')*'`)
	sqlNumberPattern = regexp.MustCompile(`\b\d+(\.\d+)?\b`)
)

// DBQueryStat stores aggregated query statistics for UI and API responses.
type DBQueryStat struct {
	Normalized    string   `json:"normalized"`
	RawSample     string   `json:"rawSample"`
	Count         int      `json:"count"`
	TotalDuration int64    `json:"totalDuration"`
	MaxDuration   int64    `json:"maxDuration"`
	P50Latency    int64    `json:"p50Latency"`
	P95Latency    int64    `json:"p95Latency"`
	P99Latency    int64    `json:"p99Latency"`
	TraceIDs      []string `json:"traceIds"`
}

// NormalizeSQL strips literal values from SQL statements for grouping.
func NormalizeSQL(query string) string {
	normalized := uuidPattern.ReplaceAllString(query, "<uuid>")
	normalized = sqlStringPattern.ReplaceAllString(normalized, "'?'")
	normalized = sqlNumberPattern.ReplaceAllString(normalized, "?")
	normalized = spacePattern.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

// RecordDBSpan updates aggregated query statistics from a DB client span.
func RecordDBSpan(ctx context.Context, db *sql.DB, s SpanRecord) error {
	statement, ok := attributeStringValue(s.Attributes, "db.statement")
	if !ok || statement == "" {
		return nil
	}

	normalized := NormalizeSQL(statement)
	duration := s.EndTimeUnix - s.StartTimeUnix
	nowTraceIDs := []string{s.TraceID}

	return withWriteRetry(ctx, func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		var (
			currentCount   int
			totalDuration  int64
			maxDuration    int64
			traceIDsRaw    string
			existingSample string
		)
		scanErr := tx.QueryRowContext(
			ctx,
			`SELECT count, total_duration, max_duration, trace_ids, raw_sample
			 FROM db_queries
			 WHERE normalized = ?`,
			normalized,
		).Scan(&currentCount, &totalDuration, &maxDuration, &traceIDsRaw, &existingSample)
		if scanErr != nil && scanErr != sql.ErrNoRows {
			return scanErr
		}

		traceIDs, err := decodeStringSlice(traceIDsRaw)
		if err != nil {
			return err
		}
		for _, traceID := range nowTraceIDs {
			traceIDs = appendUniqueString(traceIDs, traceID)
		}

		traceIDsJSON, err := json.Marshal(traceIDs)
		if err != nil {
			return err
		}

		if scanErr == sql.ErrNoRows {
			_, execErr := tx.ExecContext(
				ctx,
				`INSERT INTO db_queries (normalized, raw_sample, count, total_duration, max_duration, trace_ids)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				normalized,
				statement,
				1,
				duration,
				duration,
				string(traceIDsJSON),
			)
			if execErr != nil {
				return execErr
			}
			return tx.Commit()
		}

		if duration > maxDuration {
			maxDuration = duration
		}
		if existingSample == "" {
			existingSample = statement
		}

		_, execErr := tx.ExecContext(
			ctx,
			`UPDATE db_queries
			 SET raw_sample = ?, count = ?, total_duration = ?, max_duration = ?, trace_ids = ?
			 WHERE normalized = ?`,
			existingSample,
			currentCount+1,
			totalDuration+duration,
			maxDuration,
			string(traceIDsJSON),
			normalized,
		)
		if execErr != nil {
			return execErr
		}

		return tx.Commit()
	})
}

// ListSlowQueries lists DB query statistics ordered by max duration descending.
func ListSlowQueries(ctx context.Context, db *sql.DB, limit int) ([]DBQueryStat, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT trace_id, start_time, end_time, attributes
		 FROM spans
		 WHERE attributes LIKE '%"db.system"%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("query db spans: %w", err)
	}
	defer rows.Close()

	type aggregate struct {
		sample    string
		durations []int64
		traceIDs  []string
		total     int64
		max       int64
	}

	aggregates := make(map[string]*aggregate)
	for rows.Next() {
		var (
			traceID    string
			start      int64
			end        int64
			attributes string
		)
		if err := rows.Scan(&traceID, &start, &end, &attributes); err != nil {
			return nil, fmt.Errorf("scan db span: %w", err)
		}

		statement, ok := attributeStringValue(attributes, "db.statement")
		if !ok || statement == "" {
			continue
		}

		normalized := NormalizeSQL(statement)
		duration := end - start
		stat := aggregates[normalized]
		if stat == nil {
			stat = &aggregate{sample: statement}
			aggregates[normalized] = stat
		}

		stat.total += duration
		stat.durations = append(stat.durations, duration)
		if duration > stat.max {
			stat.max = duration
		}
		stat.traceIDs = appendUniqueString(stat.traceIDs, traceID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate db spans: %w", err)
	}

	stats := make([]DBQueryStat, 0, len(aggregates))
	for normalized, aggregate := range aggregates {
		sort.Slice(aggregate.durations, func(i, j int) bool {
			return aggregate.durations[i] < aggregate.durations[j]
		})
		stats = append(stats, DBQueryStat{
			Normalized:    normalized,
			RawSample:     aggregate.sample,
			Count:         len(aggregate.durations),
			TotalDuration: aggregate.total,
			MaxDuration:   aggregate.max,
			P50Latency:    percentileDuration(aggregate.durations, 0.50),
			P95Latency:    percentileDuration(aggregate.durations, 0.95),
			P99Latency:    percentileDuration(aggregate.durations, 0.99),
			TraceIDs:      aggregate.traceIDs,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].MaxDuration > stats[j].MaxDuration
	})
	if len(stats) > limit {
		stats = stats[:limit]
	}

	return stats, nil
}

// DeleteOldDBSpans rebuilds the aggregated query table from the remaining retained spans.
func DeleteOldDBSpans(ctx context.Context, db *sql.DB, cutoff time.Time) error {
	if err := withWriteRetry(ctx, func() error {
		_, execErr := db.ExecContext(ctx, `DELETE FROM db_queries`)
		return execErr
	}); err != nil {
		return err
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT trace_id, span_id, start_time, end_time, attributes
		 FROM spans
		 WHERE end_time >= ? AND attributes LIKE '%"db.system"%'`,
		cutoff.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("query retained db spans: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var span SpanRecord
		if err := rows.Scan(&span.TraceID, &span.SpanID, &span.StartTimeUnix, &span.EndTimeUnix, &span.Attributes); err != nil {
			return fmt.Errorf("scan retained db span: %w", err)
		}
		if err := RecordDBSpan(ctx, db, span); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate retained db spans: %w", err)
	}

	return nil
}

func percentileDuration(sorted []int64, fraction float64) int64 {
	if len(sorted) == 0 {
		return 0
	}

	index := int(math.Ceil(float64(len(sorted))*fraction)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func attributeStringValue(attributes, key string) (string, bool) {
	if attributes == "" {
		return "", false
	}

	var values map[string]interface{}
	if err := json.Unmarshal([]byte(attributes), &values); err != nil {
		return "", false
	}

	raw, ok := values[key]
	if !ok {
		return "", false
	}

	value, ok := raw.(string)
	return value, ok
}
