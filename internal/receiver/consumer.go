// Package receiver converts OTLP signals into lantern store records.
package receiver

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"github.com/Meru143/lantern/internal/bus"
	"github.com/Meru143/lantern/internal/store"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// SignalConsumer consumes OTLP traces and logs and persists them into SQLite.
type SignalConsumer struct {
	db       *sql.DB
	eventBus *bus.EventBus
}

var _ consumer.Traces = (*SignalConsumer)(nil)
var _ consumer.Logs = (*SignalConsumer)(nil)

// NewSignalConsumer constructs a consumer backed by the lantern store and event bus.
func NewSignalConsumer(db *sql.DB, eventBus *bus.EventBus) *SignalConsumer {
	return &SignalConsumer{
		db:       db,
		eventBus: eventBus,
	}
}

// Capabilities reports that the consumer does not mutate incoming pdata.
func (c *SignalConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeCapabilities preserves the TODO-specified helper shape while delegating to Capabilities.
func (c *SignalConsumer) ConsumeCapabilities() consumer.Capabilities {
	return c.Capabilities()
}

// ConsumeTraces stores all incoming spans and publishes live updates.
func (c *SignalConsumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	resourceSpans := td.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		serviceName := resourceServiceName(resourceSpan.Resource().Attributes())
		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				record := store.SpanRecord{
					TraceID:       encodeTraceID(span.TraceID()),
					SpanID:        encodeSpanID(span.SpanID()),
					ParentSpanID:  encodeSpanID(span.ParentSpanID()),
					ServiceName:   serviceName,
					Name:          span.Name(),
					Kind:          int32(span.Kind()),
					StartTimeUnix: int64(span.StartTimestamp()),
					EndTimeUnix:   int64(span.EndTimestamp()),
					StatusCode:    int32(span.Status().Code()),
					StatusMessage: span.Status().Message(),
					Attributes:    marshalRaw(span.Attributes().AsRaw()),
					Events:        marshalSpanEvents(span.Events()),
				}

				if err := store.InsertSpan(ctx, c.db, record); err != nil {
					zap.L().Warn("drop span after sqlite write failure", zap.Error(err), zap.String("trace_id", record.TraceID), zap.String("span_id", record.SpanID))
					continue
				}

				if c.eventBus != nil {
					c.eventBus.Publish(bus.Event{Type: bus.EventTypeSpan, Payload: record})
				}

				if span.Kind() == ptrace.SpanKindClient {
					if _, ok := span.Attributes().Get("db.system"); ok {
						if err := store.RecordDBSpan(ctx, c.db, record); err != nil {
							zap.L().Warn("drop db span analytics update after sqlite write failure", zap.Error(err), zap.String("span_id", record.SpanID))
						}
					}
				}

				excType, excMessage, ok := spanException(span)
				if ok {
					if err := store.UpsertErrorGroup(ctx, c.db, record.SpanID, excType, excMessage, serviceName); err != nil {
						zap.L().Warn("drop error group update after sqlite write failure", zap.Error(err), zap.String("span_id", record.SpanID))
					} else if c.eventBus != nil {
						c.eventBus.Publish(bus.Event{Type: bus.EventTypeError, Payload: map[string]string{
							"spanId":      record.SpanID,
							"exception":   excType,
							"serviceName": serviceName,
						}})
					}
				}
			}
		}
	}

	return nil
}

// ConsumeLogs stores all incoming log records and publishes live updates.
func (c *SignalConsumer) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	resourceLogs := ld.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		serviceName := resourceServiceName(resourceLog.Resource().Attributes())
		scopeLogs := resourceLog.ScopeLogs()
		for j := 0; j < scopeLogs.Len(); j++ {
			logRecords := scopeLogs.At(j).LogRecords()
			for k := 0; k < logRecords.Len(); k++ {
				logRecord := logRecords.At(k)
				record := store.LogRecord{
					TraceID:      encodeTraceID(logRecord.TraceID()),
					SpanID:       encodeSpanID(logRecord.SpanID()),
					ServiceName:  serviceName,
					Timestamp:    int64(logRecord.Timestamp()),
					SeverityNum:  int32(logRecord.SeverityNumber()),
					SeverityText: logRecord.SeverityText(),
					Body:         valueToString(logRecord.Body()),
					Attributes:   marshalRaw(logRecord.Attributes().AsRaw()),
				}

				if err := store.InsertLog(ctx, c.db, record); err != nil {
					zap.L().Warn("drop log after sqlite write failure", zap.Error(err), zap.String("trace_id", record.TraceID), zap.String("span_id", record.SpanID))
					continue
				}

				if c.eventBus != nil {
					c.eventBus.Publish(bus.Event{Type: bus.EventTypeLog, Payload: record})
				}
			}
		}
	}

	return nil
}

func resourceServiceName(attributes pcommon.Map) string {
	if value, ok := attributes.Get("service.name"); ok {
		return value.Str()
	}
	return "unknown"
}

func encodeTraceID(traceID pcommon.TraceID) string {
	return hex.EncodeToString(traceID[:])
}

func encodeSpanID(spanID pcommon.SpanID) string {
	return hex.EncodeToString(spanID[:])
}

func marshalRaw(value interface{}) string {
	if value == nil {
		return ""
	}

	payload, err := json.Marshal(value)
	if err != nil {
		zap.L().Warn("failed to marshal OTLP payload to JSON", zap.Error(err))
		return ""
	}
	return string(payload)
}

func marshalSpanEvents(events ptrace.SpanEventSlice) string {
	if events.Len() == 0 {
		return "[]"
	}

	rawEvents := make([]map[string]interface{}, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		rawEvents = append(rawEvents, map[string]interface{}{
			"name":       event.Name(),
			"timestamp":  int64(event.Timestamp()),
			"attributes": event.Attributes().AsRaw(),
		})
	}

	return marshalRaw(rawEvents)
}

func spanException(span ptrace.Span) (string, string, bool) {
	if excType, ok := span.Attributes().Get("exception.type"); ok {
		if excMessage, ok := span.Attributes().Get("exception.message"); ok {
			return excType.Str(), excMessage.Str(), true
		}
	}

	events := span.Events()
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		if event.Name() != "exception" {
			continue
		}

		excType, hasType := event.Attributes().Get("exception.type")
		excMessage, hasMessage := event.Attributes().Get("exception.message")
		if hasType && hasMessage {
			return excType.Str(), excMessage.Str(), true
		}
	}

	return "", "", false
}

func valueToString(value pcommon.Value) string {
	switch value.Type() {
	case pcommon.ValueTypeStr:
		return value.Str()
	case pcommon.ValueTypeEmpty:
		return ""
	default:
		return marshalRaw(value.AsRaw())
	}
}
