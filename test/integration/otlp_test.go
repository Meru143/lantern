//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Meru143/lantern/internal/api"
	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/config"
	"github.com/Meru143/lantern/internal/receiver"
	"github.com/Meru143/lantern/internal/store"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestOTLPIngestionAndTraceAPI(t *testing.T) {
	runtime := startTestRuntime(t)
	traceID := sendTrace(t, runtime.cfg.OTLPGRPCPort, "frontend", 5)

	require.Eventually(t, func() bool {
		spans, err := store.GetTrace(context.Background(), runtime.db, traceID)
		return err == nil && len(spans) == 5
	}, 5*time.Second, 100*time.Millisecond)

	require.Eventually(t, func() bool {
		response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/traces", runtime.cfg.UIPort))
		if err != nil {
			return false
		}
		defer response.Body.Close()

		var traces []store.TraceSummary
		if err := json.NewDecoder(response.Body).Decode(&traces); err != nil {
			return false
		}
		for _, trace := range traces {
			if trace.TraceID == traceID {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)
}

func TestWebSocketIntegration(t *testing.T) {
	runtime := startTestRuntime(t)

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/api/stream", runtime.cfg.UIPort)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	sendTrace(t, runtime.cfg.OTLPGRPCPort, "payments", 1)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	var event bus.Event
	require.NoError(t, conn.ReadJSON(&event))
	require.Equal(t, bus.EventTypeSpan, event.Type)
}

func TestRetentionCleanupIntegration(t *testing.T) {
	dbPath := t.TempDir() + "/cleanup.db"
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	require.NoError(t, store.RunMigrations(db))

	ctx := context.Background()
	oldTime := time.Now().Add(-25 * time.Hour).UnixNano()
	for index := 0; index < 10; index++ {
		require.NoError(t, store.InsertSpan(ctx, db, store.SpanRecord{
			TraceID:       "trace-old",
			SpanID:        fmt.Sprintf("old-%d", index),
			ServiceName:   "frontend",
			Name:          "old span",
			StartTimeUnix: oldTime,
			EndTimeUnix:   oldTime + int64(time.Millisecond),
			Attributes:    `{"http.method":"GET"}`,
			Events:        `[]`,
		}))
	}

	require.NoError(t, store.RunCleanup(ctx, db, 24*time.Hour))

	traces, err := store.ListTraces(ctx, db, store.TraceFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, traces, 0)
}

type testRuntime struct {
	db     *sql.DB
	cfg    *config.Config
	cancel context.CancelFunc
}

func startTestRuntime(t *testing.T) testRuntime {
	t.Helper()

	cfg := &config.Config{
		OTLPGRPCPort: getFreePort(t),
		OTLPHTTPPort: getFreePort(t),
		UIPort:       getFreePort(t),
		Retention:    "24h",
		DBPath:       t.TempDir() + "/lantern.db",
	}

	db, err := store.Open(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	require.NoError(t, store.RunMigrations(db))

	eventBus := bus.NewEventBus()
	consumer := receiver.NewSignalConsumer(db, eventBus)
	ctx, cancel := context.WithCancel(context.Background())

	require.NoError(t, receiver.Start(ctx, cfg, consumer))
	require.NoError(t, api.Start(ctx, db, eventBus, cfg.UIPort))
	waitForListener(t, cfg.OTLPGRPCPort)
	waitForHTTP(t, cfg.UIPort)

	t.Cleanup(func() {
		cancel()
		_ = api.Shutdown(context.Background())
		_ = receiver.Shutdown(context.Background())
	})

	return testRuntime{db: db, cfg: cfg, cancel: cancel}
}

func sendTrace(t *testing.T, port int, serviceName string, spans int) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(fmt.Sprintf("127.0.0.1:%d", port)),
		otlptracegrpc.WithInsecure(),
	)
	require.NoError(t, err)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
	)
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	tracer := provider.Tracer("integration-test")
	currentCtx, currentSpan := tracer.Start(ctx, "span-0")
	traceID := currentSpan.SpanContext().TraceID().String()
	startedSpans := []trace.Span{currentSpan}

	for index := 1; index < spans; index++ {
		currentCtx, currentSpan = tracer.Start(currentCtx, fmt.Sprintf("span-%d", index))
		startedSpans = append(startedSpans, currentSpan)
	}

	for index := len(startedSpans) - 1; index >= 0; index-- {
		startedSpans[index].End()
	}

	require.NoError(t, provider.ForceFlush(ctx))
	return traceID
}

func getFreePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func waitForListener(t *testing.T, port int) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 100*time.Millisecond)
}

func waitForHTTP(t *testing.T, port int) {
	t.Helper()

	require.Eventually(t, func() bool {
		response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond)
}
