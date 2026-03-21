// Package store inserts and queries log records from SQLite.
package store

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	squirrel "github.com/Masterminds/squirrel"
)

// LogRecord stores a single OTLP log record.
type LogRecord struct {
	ID           string `json:"id"`
	TraceID      string `json:"traceId"`
	SpanID       string `json:"spanId"`
	ServiceName  string `json:"serviceName"`
	Timestamp    int64  `json:"timestamp"`
	SeverityNum  int32  `json:"severityNum"`
	SeverityText string `json:"severityText"`
	Body         string `json:"body"`
	Attributes   string `json:"attributes"`
}

// LogFilter filters log list queries.
type LogFilter struct {
	ServiceName string
	MinSeverity int32
	TraceID     string
	Search      string
	Limit       int
	Offset      int
}

// InsertLog inserts or replaces a single log record.
func InsertLog(ctx context.Context, db *sql.DB, l LogRecord) error {
	if l.ID == "" {
		l.ID = makeLogID(l)
	}

	query, args, err := sqliteBuilder.
		Insert("log_records").
		Options("OR REPLACE").
		Columns(
			"id",
			"trace_id",
			"span_id",
			"service_name",
			"timestamp",
			"severity_num",
			"severity_text",
			"body",
			"attributes",
		).
		Values(
			l.ID,
			l.TraceID,
			l.SpanID,
			l.ServiceName,
			l.Timestamp,
			l.SeverityNum,
			l.SeverityText,
			l.Body,
			l.Attributes,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build log insert query: %w", err)
	}

	return withWriteRetry(ctx, func() error {
		_, execErr := db.ExecContext(ctx, query, args...)
		return execErr
	})
}

// ListLogs returns log records matching the provided filter.
func ListLogs(ctx context.Context, db *sql.DB, filter LogFilter) ([]LogRecord, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}

	builder := sqliteBuilder.
		Select(
			"id",
			"trace_id",
			"span_id",
			"service_name",
			"timestamp",
			"severity_num",
			"severity_text",
			"body",
			"attributes",
		).
		From("log_records").
		OrderBy("timestamp DESC").
		Limit(uint64(limit)).
		Offset(uint64(filter.Offset))

	if filter.ServiceName != "" {
		builder = builder.Where(squirrel.Eq{"service_name": filter.ServiceName})
	}
	if filter.MinSeverity > 0 {
		builder = builder.Where(squirrel.GtOrEq{"severity_num": filter.MinSeverity})
	}
	if filter.TraceID != "" {
		builder = builder.Where(squirrel.Eq{"trace_id": filter.TraceID})
	}
	if filter.Search != "" {
		builder = builder.Where(squirrel.Expr(`body LIKE '%' || ? || '%'`, filter.Search))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build log list query: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var logs []LogRecord
	for rows.Next() {
		var record LogRecord
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
			return nil, fmt.Errorf("scan log record: %w", err)
		}
		logs = append(logs, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate log records: %w", err)
	}

	return logs, nil
}

// DeleteLogsOlderThan deletes log rows in batches to avoid long write locks.
func DeleteLogsOlderThan(ctx context.Context, db *sql.DB, cutoff time.Time) error {
	for {
		var rowsAffected int64
		if err := withWriteRetry(ctx, func() error {
			result, execErr := db.ExecContext(
				ctx,
				`DELETE FROM log_records
				 WHERE id IN (
				 	SELECT id FROM log_records
				 	WHERE timestamp < ?
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

func makeLogID(record LogRecord) string {
	sum := sha1.Sum([]byte(fmt.Sprintf(
		"%s|%s|%s|%d|%d|%s|%s",
		record.TraceID,
		record.SpanID,
		record.ServiceName,
		record.Timestamp,
		record.SeverityNum,
		record.SeverityText,
		record.Body,
	)))
	return hex.EncodeToString(sum[:])
}
