package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/store"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRouterServesEmbeddedIndex(t *testing.T) {
	t.Parallel()

	state := newTestServerState(t)
	server := httptest.NewServer(state.routes())
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = response.Body.Close()
	})

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, string(body), "<title>lantern</title>")
}

func TestTracesHandlersReturnSummariesAndTree(t *testing.T) {
	t.Parallel()

	state := newTestServerState(t)
	server := httptest.NewServer(state.routes())
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/api/traces?service=frontend&minDuration=1ms")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = response.Body.Close()
	})

	require.Equal(t, http.StatusOK, response.StatusCode)

	var summaries []store.TraceSummary
	require.NoError(t, json.NewDecoder(response.Body).Decode(&summaries))
	require.Len(t, summaries, 1)
	require.Equal(t, "trace-1", summaries[0].TraceID)
	require.Equal(t, "GET /checkout", summaries[0].Name)
	require.ElementsMatch(t, []string{"frontend", "payments"}, summaries[0].ServiceNames)
	require.True(t, summaries[0].HasError)

	detailResponse, err := http.Get(server.URL + "/api/traces/trace-1")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = detailResponse.Body.Close()
	})

	require.Equal(t, http.StatusOK, detailResponse.StatusCode)

	var detail traceDetailResponse
	require.NoError(t, json.NewDecoder(detailResponse.Body).Decode(&detail))
	require.Equal(t, "trace-1", detail.TraceID)
	require.Len(t, detail.Flat, 3)
	require.Len(t, detail.Spans, 1)
	require.Equal(t, "root-1", detail.Spans[0].Span.SpanID)
	require.Len(t, detail.Spans[0].Children, 1)
	require.Equal(t, "child-1", detail.Spans[0].Children[0].Span.SpanID)
	require.Len(t, detail.Spans[0].Children[0].Children, 1)
	require.Equal(t, "db-1", detail.Spans[0].Children[0].Children[0].Span.SpanID)
}

func TestLogsErrorsQueriesAndServicesHandlersReturnJSON(t *testing.T) {
	t.Parallel()

	state := newTestServerState(t)
	server := httptest.NewServer(state.routes())
	t.Cleanup(server.Close)

	testCases := []struct {
		name   string
		path   string
		assert func(t *testing.T, body []byte)
	}{
		{
			name: "logs",
			path: "/api/logs?service=frontend&severity=9&search=checkout",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var records []store.LogRecord
				require.NoError(t, json.Unmarshal(body, &records))
				require.Len(t, records, 1)
				require.Equal(t, "trace-1", records[0].TraceID)
			},
		},
		{
			name: "services",
			path: "/api/services",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var response servicesResponse
				require.NoError(t, json.Unmarshal(body, &response))
				require.Contains(t, response.Services, "frontend")
				require.Contains(t, response.Services, "payments")
				require.Len(t, response.Edges, 1)
				require.Equal(t, "frontend", response.Edges[0].From)
				require.Equal(t, "payments", response.Edges[0].To)
				require.Equal(t, 1, response.Edges[0].Count)
				require.Equal(t, 100.0, response.Edges[0].ErrorRate)
			},
		},
		{
			name: "errors",
			path: "/api/errors",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var groups []store.ErrorGroup
				require.NoError(t, json.Unmarshal(body, &groups))
				require.Len(t, groups, 1)
				require.Equal(t, "ValueError", groups[0].ExcType)
			},
		},
		{
			name: "error detail",
			path: "/api/errors/" + store.FingerprintError("ValueError", "checkout failed for order 123"),
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var detail errorDetailResponse
				require.NoError(t, json.Unmarshal(body, &detail))
				require.NotNil(t, detail.Group)
				require.NotNil(t, detail.SampleSpan)
				require.Equal(t, "db-1", detail.SampleSpan.SpanID)
			},
		},
		{
			name: "queries",
			path: "/api/queries",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var stats []store.DBQueryStat
				require.NoError(t, json.Unmarshal(body, &stats))
				require.Len(t, stats, 1)
				require.Equal(t, "SELECT * FROM payments WHERE id = ?", stats[0].Normalized)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := http.Get(server.URL + tc.path)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = response.Body.Close()
			})

			require.Equal(t, http.StatusOK, response.StatusCode)

			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			tc.assert(t, body)
		})
	}
}

func TestStreamHandlerBroadcastsEvents(t *testing.T) {
	t.Parallel()

	state := newTestServerState(t)
	server := httptest.NewServer(state.routes())
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/stream"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	state.eventBus.Publish(bus.Event{
		Type:    bus.EventTypeSpan,
		Payload: map[string]any{"traceId": "trace-1"},
	})

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	var event bus.Event
	require.NoError(t, conn.ReadJSON(&event))
	require.Equal(t, bus.EventTypeSpan, event.Type)

	payload, ok := event.Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "trace-1", payload["traceId"])
}

func newTestServerState(t *testing.T) *serverState {
	t.Helper()

	ctx := context.Background()
	db := openTestDB(t)
	eventBus := bus.NewEventBus()

	baseTime := time.Now().Add(-5 * time.Minute).UnixNano()
	seedSpans := []store.SpanRecord{
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
		{
			TraceID:       "trace-1",
			SpanID:        "db-1",
			ParentSpanID:  "child-1",
			ServiceName:   "payments",
			Name:          "SELECT payments",
			StartTimeUnix: baseTime + int64(10*time.Millisecond),
			EndTimeUnix:   baseTime + int64(20*time.Millisecond),
			Kind:          3,
			StatusCode:    2,
			StatusMessage: "sql timeout",
			Attributes:    `{"db.system":"postgresql","db.statement":"SELECT * FROM payments WHERE id = 123"}`,
			Events:        `[{"name":"exception","attributes":{"exception.type":"ValueError","exception.message":"checkout failed for order 123"}}]`,
		},
	}

	for _, span := range seedSpans {
		require.NoError(t, store.InsertSpan(ctx, db, span))
	}
	require.NoError(t, store.RecordDBSpan(ctx, db, seedSpans[2]))
	require.NoError(t, store.UpsertErrorGroup(ctx, db, "db-1", "ValueError", "checkout failed for order 123", "payments"))
	require.NoError(t, store.InsertLog(ctx, db, store.LogRecord{
		TraceID:      "trace-1",
		SpanID:       "root-1",
		ServiceName:  "frontend",
		Timestamp:    baseTime + int64(15*time.Millisecond),
		SeverityNum:  9,
		SeverityText: "ERROR",
		Body:         "checkout failed for user 42",
		Attributes:   `{"component":"checkout"}`,
	}))

	return newServerState(db, eventBus)
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
