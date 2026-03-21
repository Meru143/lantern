// Package bus provides in-process pub/sub for live lantern updates.
package bus

import "sync"

// EventType identifies the kind of event flowing through the bus.
type EventType string

const (
	EventTypeSpan  EventType = "span"
	EventTypeLog   EventType = "log"
	EventTypeError EventType = "error"
)

// Event is a single event broadcast to live subscribers.
type Event struct {
	Type    EventType
	Payload interface{}
}

// EventBus is an in-process publish/subscribe event bus.
type EventBus struct {
	mu   sync.RWMutex
	subs map[string]chan Event
}

// NewEventBus creates an empty event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[string]chan Event),
	}
}

// Subscribe registers a subscriber with the provided buffer size.
func (b *EventBus) Subscribe(id string, bufSize int) chan Event {
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.subs[id]; ok {
		close(existing)
	}
	b.subs[id] = ch

	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *EventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.subs[id]
	if !ok {
		return
	}

	delete(b.subs, id)
	close(ch)
}

// Publish broadcasts an event to all subscribers without blocking the publisher.
func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
