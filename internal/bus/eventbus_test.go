package bus

import (
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	t.Parallel()

	eventBus := NewEventBus()
	sub := eventBus.Subscribe("test", 1)
	expected := Event{Type: EventTypeSpan, Payload: "payload"}

	eventBus.Publish(expected)

	select {
	case got := <-sub:
		if got.Type != expected.Type {
			t.Fatalf("got type %q, want %q", got.Type, expected.Type)
		}
		if got.Payload != expected.Payload {
			t.Fatalf("got payload %v, want %v", got.Payload, expected.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()

	eventBus := NewEventBus()
	sub := eventBus.Subscribe("test", 1)
	eventBus.Unsubscribe("test")

	eventBus.Publish(Event{Type: EventTypeLog, Payload: "payload"})

	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("expected closed channel after unsubscribe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for closed subscription channel")
	}
}

func TestPublishDoesNotBlockSlowSubscribers(t *testing.T) {
	t.Parallel()

	eventBus := NewEventBus()
	_ = eventBus.Subscribe("slow", 1)

	eventBus.Publish(Event{Type: EventTypeSpan, Payload: "first"})

	done := make(chan struct{})
	go func() {
		eventBus.Publish(Event{Type: EventTypeSpan, Payload: "second"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("publish blocked on slow subscriber")
	}
}
