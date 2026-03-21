package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInsertSpanAndGetTraceRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)

	traceID := "trace-roundtrip"
	for i := 0; i < 5; i++ {
		err := InsertSpan(ctx, db, SpanRecord{
			TraceID:       traceID,
			SpanID:        "span-" + string(rune('0'+i)),
			ServiceName:   "api",
			Name:          "operation",
			StartTimeUnix: int64(100 + i),
			EndTimeUnix:   int64(200 + i),
		})
		if err != nil {
			t.Fatalf("insert span %d: %v", i, err)
		}
	}

	got, err := GetTrace(ctx, db, traceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d spans, want 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].StartTimeUnix > got[i].StartTimeUnix {
			t.Fatalf("spans not sorted by start time: %d > %d", got[i-1].StartTimeUnix, got[i].StartTimeUnix)
		}
	}
}

func TestListTracesFilterByServiceName(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)

	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-api",
		SpanID:        "span-api",
		ServiceName:   "api",
		Name:          "api-request",
		StartTimeUnix: 100,
		EndTimeUnix:   200,
	})
	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-worker",
		SpanID:        "span-worker",
		ServiceName:   "worker",
		Name:          "job",
		StartTimeUnix: 300,
		EndTimeUnix:   600,
	})

	got, err := ListTraces(ctx, db, TraceFilter{ServiceName: "api"})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d traces, want 1", len(got))
	}
	if got[0].TraceID != "trace-api" {
		t.Fatalf("got trace %q, want trace-api", got[0].TraceID)
	}
}

func TestListTracesFilterByHasError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)

	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-ok",
		SpanID:        "span-ok",
		ServiceName:   "api",
		Name:          "ok",
		StartTimeUnix: 100,
		EndTimeUnix:   200,
		StatusCode:    0,
	})
	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-error",
		SpanID:        "span-error",
		ServiceName:   "api",
		Name:          "error",
		StartTimeUnix: 300,
		EndTimeUnix:   400,
		StatusCode:    2,
	})

	got, err := ListTraces(ctx, db, TraceFilter{HasError: true})
	if err != nil {
		t.Fatalf("list traces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d traces, want 1", len(got))
	}
	if got[0].TraceID != "trace-error" {
		t.Fatalf("got trace %q, want trace-error", got[0].TraceID)
	}
}

func TestDeleteTracesOlderThanRemovesOldRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()

	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-old",
		SpanID:        "span-old",
		ServiceName:   "api",
		Name:          "old",
		StartTimeUnix: now.Add(-2 * time.Hour).UnixNano(),
		EndTimeUnix:   now.Add(-90 * time.Minute).UnixNano(),
	})
	insertTraceSpan(t, ctx, db, SpanRecord{
		TraceID:       "trace-new",
		SpanID:        "span-new",
		ServiceName:   "api",
		Name:          "new",
		StartTimeUnix: now.Add(-30 * time.Minute).UnixNano(),
		EndTimeUnix:   now.Add(-20 * time.Minute).UnixNano(),
	})

	if err := DeleteTracesOlderThan(ctx, db, now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("delete old traces: %v", err)
	}

	oldTrace, err := GetTrace(ctx, db, "trace-old")
	if err != nil {
		t.Fatalf("get old trace: %v", err)
	}
	if len(oldTrace) != 0 {
		t.Fatalf("expected old trace to be deleted, got %d spans", len(oldTrace))
	}

	newTrace, err := GetTrace(ctx, db, "trace-new")
	if err != nil {
		t.Fatalf("get new trace: %v", err)
	}
	if len(newTrace) != 1 {
		t.Fatalf("expected new trace to remain, got %d spans", len(newTrace))
	}
}

func insertTraceSpan(t *testing.T, ctx context.Context, db *sql.DB, span SpanRecord) {
	t.Helper()
	if err := InsertSpan(ctx, db, span); err != nil {
		t.Fatalf("insert span: %v", err)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "lantern.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}
