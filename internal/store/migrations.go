// Package store applies the SQLite schema required by lantern.
package store

import (
	"database/sql"
	"fmt"
)

var baseMigrations = []string{
	`CREATE TABLE IF NOT EXISTS spans (
		trace_id TEXT,
		span_id TEXT PRIMARY KEY,
		parent_span_id TEXT,
		service_name TEXT,
		name TEXT,
		kind INTEGER,
		start_time INTEGER,
		end_time INTEGER,
		status_code INTEGER,
		status_message TEXT,
		attributes TEXT,
		events TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS log_records (
		id TEXT PRIMARY KEY,
		trace_id TEXT,
		span_id TEXT,
		service_name TEXT,
		timestamp INTEGER,
		severity_num INTEGER,
		severity_text TEXT,
		body TEXT,
		attributes TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS error_groups (
		fingerprint TEXT PRIMARY KEY,
		exc_type TEXT,
		message_norm TEXT,
		count INTEGER,
		first_seen INTEGER,
		last_seen INTEGER,
		services TEXT,
		sample_span_id TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_service ON spans(service_name, start_time)`,
	`CREATE INDEX IF NOT EXISTS idx_logs_trace_id ON log_records(trace_id)`,
	`CREATE INDEX IF NOT EXISTS idx_logs_service ON log_records(service_name, timestamp)`,
}

// RunMigrations creates the SQLite schema required by lantern.
func RunMigrations(db *sql.DB) error {
	for idx, statement := range baseMigrations {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("apply migration %d: %w", idx+1, err)
		}
	}
	return nil
}
