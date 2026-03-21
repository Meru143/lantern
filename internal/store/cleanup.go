// Package store runs retention cleanup tasks against SQLite.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RunCleanup deletes retained data older than the configured duration.
func RunCleanup(ctx context.Context, db *sql.DB, retention time.Duration) error {
	cutoff := time.Now().Add(-retention)

	if err := DeleteTracesOlderThan(ctx, db, cutoff); err != nil {
		return fmt.Errorf("cleanup traces: %w", err)
	}
	if err := DeleteLogsOlderThan(ctx, db, cutoff); err != nil {
		return fmt.Errorf("cleanup logs: %w", err)
	}
	if err := DeleteOldDBSpans(ctx, db, cutoff); err != nil {
		return fmt.Errorf("cleanup db query stats: %w", err)
	}

	return nil
}

// ParseRetention parses a retention string such as "24h".
func ParseRetention(retention string) (time.Duration, error) {
	return time.ParseDuration(retention)
}
