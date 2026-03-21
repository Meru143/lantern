// Package store inserts and queries trace records from SQLite.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	squirrel "github.com/Masterminds/squirrel"
)

var sqliteBuilder = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Question)

// SpanRecord stores a single span as persisted in SQLite.
type SpanRecord struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	ServiceName   string
	Name          string
	Kind          int32
	StartTimeUnix int64
	EndTimeUnix   int64
	StatusCode    int32
	StatusMessage string
	Attributes    string
	Events        string
}

// TraceFilter filters trace list queries.
type TraceFilter struct {
	ServiceName string
	HasError    bool
	MinDuration time.Duration
	Limit       int
	Offset      int
}

// TraceSummary is the summary row shown in trace listings.
type TraceSummary struct {
	TraceID       string   `json:"traceId"`
	Name          string   `json:"name"`
	ServiceNames  []string `json:"serviceNames"`
	StartTimeUnix int64    `json:"startTimeUnix"`
	EndTimeUnix   int64    `json:"endTimeUnix"`
	DurationNanos int64    `json:"durationNanos"`
	SpanCount     int      `json:"spanCount"`
	HasError      bool     `json:"hasError"`
}

// InsertSpan inserts or replaces a single span record.
func InsertSpan(ctx context.Context, db *sql.DB, s SpanRecord) error {
	query, args, err := sqliteBuilder.
		Insert("spans").
		Options("OR REPLACE").
		Columns(
			"trace_id",
			"span_id",
			"parent_span_id",
			"service_name",
			"name",
			"kind",
			"start_time",
			"end_time",
			"status_code",
			"status_message",
			"attributes",
			"events",
		).
		Values(
			s.TraceID,
			s.SpanID,
			s.ParentSpanID,
			s.ServiceName,
			s.Name,
			s.Kind,
			s.StartTimeUnix,
			s.EndTimeUnix,
			s.StatusCode,
			s.StatusMessage,
			s.Attributes,
			s.Events,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build span insert query: %w", err)
	}

	return withWriteRetry(ctx, func() error {
		_, execErr := db.ExecContext(ctx, query, args...)
		return execErr
	})
}

// GetTrace returns all spans for a trace ordered by start time.
func GetTrace(ctx context.Context, db *sql.DB, traceID string) ([]SpanRecord, error) {
	query, args, err := sqliteBuilder.
		Select(
			"trace_id",
			"span_id",
			"parent_span_id",
			"service_name",
			"name",
			"kind",
			"start_time",
			"end_time",
			"status_code",
			"status_message",
			"attributes",
			"events",
		).
		From("spans").
		Where(squirrel.Eq{"trace_id": traceID}).
		OrderBy("start_time ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build trace query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trace %s: %w", traceID, err)
	}
	defer rows.Close()

	var spans []SpanRecord
	for rows.Next() {
		var span SpanRecord
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
			return nil, fmt.Errorf("scan trace row: %w", err)
		}
		spans = append(spans, span)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace rows: %w", err)
	}

	return spans, nil
}

// ListTraces returns trace summaries ordered by most recent start time.
func ListTraces(ctx context.Context, db *sql.DB, filter TraceFilter) ([]TraceSummary, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var (
		clauses []string
		args    []interface{}
	)
	clauses = append(clauses, "1 = 1")
	if filter.ServiceName != "" {
		clauses = append(clauses, "s.trace_id IN (SELECT trace_id FROM spans WHERE service_name = ?)")
		args = append(args, filter.ServiceName)
	}
	if filter.HasError {
		clauses = append(clauses, "s.trace_id IN (SELECT trace_id FROM spans WHERE status_code <> 0)")
	}

	query := `
SELECT
	s.trace_id,
	COALESCE(
		(SELECT name FROM spans root WHERE root.trace_id = s.trace_id ORDER BY start_time ASC LIMIT 1),
		''
	) AS trace_name,
	MIN(s.start_time) AS trace_start,
	MAX(s.end_time) AS trace_end,
	COUNT(*) AS span_count,
	COALESCE(GROUP_CONCAT(DISTINCT s.service_name), '') AS service_names,
	MAX(CASE WHEN s.status_code <> 0 THEN 1 ELSE 0 END) AS has_error
FROM spans s
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY s.trace_id`
	if filter.MinDuration > 0 {
		query += "\nHAVING (MAX(s.end_time) - MIN(s.start_time)) >= ?"
		args = append(args, filter.MinDuration.Nanoseconds())
	}
	query += "\nORDER BY trace_start DESC LIMIT ? OFFSET ?"
	args = append(args, limit, filter.Offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close()

	var traces []TraceSummary
	for rows.Next() {
		var (
			summary      TraceSummary
			serviceNames string
			hasError     int
		)
		if err := rows.Scan(
			&summary.TraceID,
			&summary.Name,
			&summary.StartTimeUnix,
			&summary.EndTimeUnix,
			&summary.SpanCount,
			&serviceNames,
			&hasError,
		); err != nil {
			return nil, fmt.Errorf("scan trace summary: %w", err)
		}
		if serviceNames != "" {
			summary.ServiceNames = strings.Split(serviceNames, ",")
		}
		summary.DurationNanos = summary.EndTimeUnix - summary.StartTimeUnix
		summary.HasError = hasError == 1
		traces = append(traces, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace summaries: %w", err)
	}

	return traces, nil
}

// DeleteTracesOlderThan deletes trace rows in batches to avoid long write locks.
func DeleteTracesOlderThan(ctx context.Context, db *sql.DB, cutoff time.Time) error {
	for {
		var rowsAffected int64
		if err := withWriteRetry(ctx, func() error {
			result, execErr := db.ExecContext(
				ctx,
				`DELETE FROM spans
				 WHERE span_id IN (
				 	SELECT span_id FROM spans
				 	WHERE end_time < ?
				 	LIMIT 500
				 )`,
				cutoff.UnixNano(),
			)
			if execErr != nil {
				return execErr
			}
			rowsAffected, execErr = result.RowsAffected()
			return execErr
		}); err != nil {
			return err
		}

		if rowsAffected == 0 {
			return nil
		}
	}
}
