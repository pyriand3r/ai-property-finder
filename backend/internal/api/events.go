package api

import (
	"net/http"
	"propertyfinder/internal/events"
)

// EventHandler streams SSE events from the broker to a single connected
// client (spec 01-04 Layer 2.3 "Task-SSE-Plumbing"; reused for chat
// streaming in Layer 6.3).
type EventHandler struct {
	broker *events.Broker
}

// Stream handles GET /api/events. It subscribes to the broker, replays
// events missed during a disconnect (or emits a resync when the gap
// exceeds the ring buffer), then fans out live events with a 15s heartbeat
// keep-alive. Uses http.Flusher directly — chi's Timeout middleware must
// not wrap this route (it doesn't implement Flusher).
func (handler *EventHandler) Stream(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")

	ch := handler.broker.Subscribe()
	defer handler.broker.Unsubscribe(ch)

	lastID := parseLastEventID(request.Header.Get("Last-Event-ID"))
	if lastID > 0 {
		if !handler.broker.Replay(ch, lastID) {
			flusher.Flush()
		}
	}

	ctx := request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			events.FormatSSE(writer, event)
			flusher.Flush()
		case <-handler.broker.Heartbeat():
			_, _ = writer.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

// parseLastEventID parses the Last-Event-ID header into a monotonic event
// id; 0 means "no prior id". Malformed values are treated as 0 so a
// misbehaving client cannot crash the stream.
func parseLastEventID(raw string) int64 {
	var id int64
	for _, runeValue := range raw {
		if runeValue < '0' || runeValue > '9' {
			return 0
		}
		id = id*10 + int64(runeValue-'0')
	}
	return id
}
