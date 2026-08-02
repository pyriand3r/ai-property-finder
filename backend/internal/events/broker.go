// Package events provides the SSE plumbing shared by background task
// progress (Layer 2.3) and, later, chat token/tool-call streaming (Layer
// 6.3) — one broker, one wire format, reused rather than rebuilt per
// feature (spec 01-04, ADR 0003 SSE-over-WebSocket).
package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Event is one SSE notification fanned out to every connected client. ID is
// a per-process monotonic counter, used for Last-Event-ID replay.
type Event struct {
	ID      int64
	Type    string
	Payload any
}

// Broker fans events out to all connected clients and keeps a ring buffer
// for Last-Event-ID replay on reconnect, plus a heartbeat so idle
// connections aren't killed by intermediate proxies/timeouts. It is a
// notification channel only — the durable source of truth is the task
// table (or, from Layer 6, chat_message).
type Broker struct {
	mu         sync.RWMutex
	clients    map[chan Event]struct{}
	ringBuffer []Event
	ringSize   int
	ringIndex  int
	nextID     atomic.Int64
	heartbeat  *time.Ticker
}

// NewBroker constructs a Broker with a 200-event ring buffer and a 15s
// heartbeat ticker.
func NewBroker() *Broker {
	return &Broker{
		clients:    make(map[chan Event]struct{}),
		ringBuffer: make([]Event, 0, 200),
		ringSize:   200,
		heartbeat:  time.NewTicker(15 * time.Second),
	}
}

// Subscribe registers a new client channel receiving every published event.
// The caller drains the channel inside its SSE handler loop and calls
// Unsubscribe on disconnect.
func (broker *Broker) Subscribe() chan Event {
	ch := make(chan Event, 64)
	broker.mu.Lock()
	broker.clients[ch] = struct{}{}
	broker.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel and stops further delivery.
func (broker *Broker) Unsubscribe(ch chan Event) {
	broker.mu.Lock()
	delete(broker.clients, ch)
	broker.mu.Unlock()
	close(ch)
}

// Publish fans an event out to every connected client and appends it to the
// ring buffer for Last-Event-ID replay.
func (broker *Broker) Publish(eventType string, payload any) {
	event := Event{
		ID:      broker.nextID.Add(1),
		Type:    eventType,
		Payload: payload,
	}
	broker.mu.Lock()
	if len(broker.ringBuffer) < broker.ringSize {
		broker.ringBuffer = append(broker.ringBuffer, event)
	} else {
		broker.ringBuffer[broker.ringIndex] = event
		broker.ringIndex = (broker.ringIndex + 1) % broker.ringSize
	}
	clients := broker.collectClientsLocked()
	broker.mu.Unlock()

	for client := range clients {
		select {
		case client <- event:
		default:
			// Client is too slow; drop the event rather than block the
			// publisher. The frontend reconciles via the task table (or
			// chat_message history) on reconnect.
		}
	}
}

// Replay pushes every buffered event with id greater than lastEventID into
// ch. Returns false when lastEventID is older than the buffer's oldest
// entry, signaling the caller to emit a resync event instead.
func (broker *Broker) Replay(ch chan Event, lastEventID int64) bool {
	broker.mu.RLock()
	defer broker.mu.RUnlock()

	if len(broker.ringBuffer) == 0 {
		return true
	}
	oldest := broker.oldestIDLocked()
	if lastEventID < oldest {
		return false
	}
	for _, event := range broker.ringBuffer {
		if event.ID > lastEventID {
			select {
			case ch <- event:
			default:
			}
		}
	}
	return true
}

// Heartbeat returns the broker's 15s ticker channel used by the SSE handler
// to emit `: ping` keep-alive comments.
func (broker *Broker) Heartbeat() <-chan time.Time {
	return broker.heartbeat.C
}

// Stop halts the heartbeat ticker. Call once on shutdown.
func (broker *Broker) Stop() {
	broker.heartbeat.Stop()
}

func (broker *Broker) collectClientsLocked() map[chan Event]struct{} {
	clients := make(map[chan Event]struct{}, len(broker.clients))
	for ch := range broker.clients {
		clients[ch] = struct{}{}
	}
	return clients
}

func (broker *Broker) oldestIDLocked() int64 {
	oldest := broker.ringBuffer[0].ID
	for _, event := range broker.ringBuffer[1:] {
		if event.ID < oldest {
			oldest = event.ID
		}
	}
	return oldest
}

// FormatSSE writes one event to an http.ResponseWriter with the canonical
// SSE framing: `id: <n>\nevent: <type>\ndata: <json>\n\n`.
func FormatSSE(writer http.ResponseWriter, event Event) {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, payload)
}
