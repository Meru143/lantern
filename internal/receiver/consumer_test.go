package receiver

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/store"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestConsumeTracesStoresSpanAndPublishesEvent(t *testing.T) {
	t.Parallel()

	db := openReceiverTestDB(t)
	eventBus := bus.NewEventBus()
	sub := eventBus.Subscribe("test", 1)
	consumer := NewSignalConsumer(db, eventBus)

	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "checkout")
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4}))
	span.SetSpanID(pcommon.SpanID([8]byte{5, 6, 7, 8}))
	span.SetName("GET /checkout")
	span.SetStartTimestamp(pcommon.Timestamp(100))
	span.SetEndTimestamp(pcommon.Timestamp(250))
	span.Attributes().PutStr("http.method", "GET")

	if err := consumer.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("consume traces: %v", err)
	}

	got, err := store.GetTrace(context.Background(), db, "01020304000000000000000000000000")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d spans, want 1", len(got))
	}
	if got[0].ServiceName != "checkout" {
		t.Fatalf("got service %q, want checkout", got[0].ServiceName)
	}

	select {
	case event := <-sub:
		if event.Type != bus.EventTypeSpan {
			t.Fatalf("got event type %q, want %q", event.Type, bus.EventTypeSpan)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published span event")
	}
}

func openReceiverTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "receiver.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := store.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}
