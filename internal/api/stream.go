package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/Meru143/lantern/internal/bus"
)

func (s *serverState) streamEvents(w http.ResponseWriter, r *http.Request) {
	if s.eventBus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus unavailable")
		return
	}

	subscriberID, err := subscriberID()
	if err != nil {
		return
	}
	events := s.eventBus.Subscribe(subscriberID, 64)
	defer s.eventBus.Unsubscribe(subscriberID)

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-readDone:
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := conn.WriteJSON(bus.Event{
				Type:    event.Type,
				Payload: event.Payload,
			}); err != nil {
				return
			}
		}
	}
}

func subscriberID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
