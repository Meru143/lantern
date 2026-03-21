package tui

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestModelHandlesResizeAndTabSwitching(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	seedTestData(t, db)

	initial := NewModel(db, bus.NewEventBus())
	resized, _ := initial.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	current := resized.(model)
	require.Equal(t, 120, current.width)
	require.Equal(t, 40, current.height)
	require.Equal(t, 0, current.activeTab)
	require.Contains(t, current.View(), "Traces")

	updated, _ := current.Update(tea.KeyMsg{Type: tea.KeyRight})
	current = updated.(model)
	require.Equal(t, 1, current.activeTab)
	require.Contains(t, current.View(), "Logs")
}

func TestTracesTabOpensSelectedTraceDetail(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	seedTestData(t, db)

	initial := NewModel(db, bus.NewEventBus())
	resized, _ := initial.Update(tea.WindowSizeMsg{Width: 100, Height: 36})
	current := resized.(model)

	updated, _ := current.Update(tea.KeyMsg{Type: tea.KeyEnter})
	current = updated.(model)

	require.Contains(t, current.View(), "trace-1")
	require.Contains(t, current.View(), "GET /checkout")
	require.Contains(t, current.View(), "Charge card")
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := t.TempDir() + "/lantern.db"
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	require.NoError(t, store.RunMigrations(db))

	return db
}

func seedTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	baseTime := time.Now().Add(-5 * time.Minute).UnixNano()
	spans := []store.SpanRecord{
		{
			TraceID:       "trace-1",
			SpanID:        "root-1",
			ServiceName:   "frontend",
			Name:          "GET /checkout",
			StartTimeUnix: baseTime,
			EndTimeUnix:   baseTime + int64(50*time.Millisecond),
			Attributes:    `{"http.method":"GET"}`,
			Events:        `[]`,
		},
		{
			TraceID:       "trace-1",
			SpanID:        "child-1",
			ParentSpanID:  "root-1",
			ServiceName:   "payments",
			Name:          "Charge card",
			StartTimeUnix: baseTime + int64(5*time.Millisecond),
			EndTimeUnix:   baseTime + int64(45*time.Millisecond),
			StatusCode:    2,
			StatusMessage: "error",
			Attributes:    `{"peer.service":"payments"}`,
			Events:        `[]`,
		},
	}
	for _, span := range spans {
		require.NoError(t, store.InsertSpan(ctx, db, span))
	}

	require.NoError(t, store.InsertLog(ctx, db, store.LogRecord{
		TraceID:      "trace-1",
		SpanID:       "root-1",
		ServiceName:  "frontend",
		Timestamp:    baseTime + int64(10*time.Millisecond),
		SeverityNum:  9,
		SeverityText: "INFO",
		Body:         "checkout requested",
		Attributes:   `{"component":"checkout"}`,
	}))

	require.NoError(t, store.UpsertErrorGroup(ctx, db, "child-1", "ValueError", "checkout failed for order 123", "payments"))
}
