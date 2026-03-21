// Package store persists lantern telemetry and derived views in SQLite.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"go.uber.org/zap"
)

const (
	dbWriteFailureCode = "LT003"
	dbLockedCode       = "LT004"
)

// Open opens the SQLite database configured for lantern.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable sqlite wal mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	return db, nil
}

func withWriteRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := fn(); err != nil {
			lastErr = err
			if !isLockedError(err) {
				zap.L().Warn("sqlite write failure", zap.String("code", dbWriteFailureCode), zap.Error(err))
				return fmt.Errorf("%s: %w", dbWriteFailureCode, err)
			}

			if attempt == 2 {
				zap.L().Warn("sqlite database remained locked after retries", zap.String("code", dbLockedCode), zap.Error(err))
				return fmt.Errorf("%s: %w", dbLockedCode, err)
			}

			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}

		return nil
	}

	zap.L().Warn("sqlite database remained locked after retries", zap.String("code", dbLockedCode), zap.Error(lastErr))
	return fmt.Errorf("%s: %w", dbLockedCode, lastErr)
}

func isLockedError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}
