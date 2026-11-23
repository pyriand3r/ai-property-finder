package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/events"
	"strings"
	"testing"
	"time"
)

func TestParseLastEventID(t *testing.T) {
	tests := map[string]int64{
		"":      0,
		"0":     0,
		"7":     7,
		"12345": 12345,
		"abc":   0,
		"-1":    0,
		"1x":    0,
		"1.5":   0,
	}
	for raw, want := range tests {
		if got := parseLastEventID(raw); got != want {
			t.Errorf("parseLastEventID(%q) = %d, want %d", raw, got, want)
		}
	}
}

// readSSEEvent reads one SSE frame (up to the blank line) and returns its
// lines. Fails the test if the stream stalls.
func readSSEEvent(t *testing.T, reader *bufio.Reader) []string {
	t.Helper()
	type result struct {
		lines []string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- result{lines, err}
				return
			}
			line = strings.TrimRight(line, "\n")
			if line == "" {
				done <- result{lines, nil}
				return
			}
			lines = append(lines, line)
		}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("read SSE frame: %v (got %v)", res.err, res.lines)
		}
		return res.lines
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an SSE frame")
		return nil
	}
}

// TestEventHandler_Stream_ReplaysMissedThenStreamsLive covers the
// reconnect contract (ADR 0008 / spec 01-04): Last-Event-ID replays what
// the client missed, then live events follow, in SSE framing.
func TestEventHandler_Stream_ReplaysMissedThenStreamsLive(t *testing.T) {
	broker := events.NewBroker()
	defer broker.Stop()
	broker.Publish("chat.delta", map[string]string{"n": "1"})
	broker.Publish("chat.delta", map[string]string{"n": "2"})
	broker.Publish("chat.message", map[string]string{"n": "3"})
	server := httptest.NewServer(http.HandlerFunc((&EventHandler{broker: broker}).Stream))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Last-Event-ID", "1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	reader := bufio.NewReader(response.Body)

	first := readSSEEvent(t, reader)
	second := readSSEEvent(t, reader)
	if strings.Join(first, "|") != `id: 2|event: chat.delta|data: {"n":"2"}` {
		t.Errorf("first replayed frame = %v", first)
	}
	if strings.Join(second, "|") != `id: 3|event: chat.message|data: {"n":"3"}` {
		t.Errorf("second replayed frame = %v", second)
	}

	broker.Publish("chat.done", map[string]string{"n": "4"})
	live := readSSEEvent(t, reader)
	if strings.Join(live, "|") != `id: 4|event: chat.done|data: {"n":"4"}` {
		t.Errorf("live frame = %v", live)
	}
}

// TestEventHandler_Stream_NoReplayWithoutLastEventID: a fresh connection
// only sees events published after it subscribed.
func TestEventHandler_Stream_NoReplayWithoutLastEventID(t *testing.T) {
	broker := events.NewBroker()
	defer broker.Stop()
	broker.Publish("old", nil)
	server := httptest.NewServer(http.HandlerFunc((&EventHandler{broker: broker}).Stream))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	connected := make(chan *http.Response, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			close(connected)
			return
		}
		connected <- response
	}()
	// Headers are only sent with the first flush, so publish until the
	// client has connected; the first frame it sees must not be "old".
	var response *http.Response
	deadline := time.After(3 * time.Second)
	for response == nil {
		select {
		case resp, ok := <-connected:
			if !ok {
				t.Fatal("connect failed")
			}
			response = resp
		case <-time.After(20 * time.Millisecond):
			broker.Publish("new", nil)
		case <-deadline:
			t.Fatal("timed out connecting")
		}
	}
	defer response.Body.Close()

	frame := readSSEEvent(t, bufio.NewReader(response.Body))

	if len(frame) < 2 || frame[1] != "event: new" {
		t.Errorf("first frame = %v, want a \"new\" event and no replay of \"old\"", frame)
	}
}

// nonFlushingWriter hides http.Flusher, like chi's Timeout wrapper would.
type nonFlushingWriter struct {
	header http.Header
	status int
}

func (writer *nonFlushingWriter) Header() http.Header         { return writer.header }
func (writer *nonFlushingWriter) Write(p []byte) (int, error) { return len(p), nil }
func (writer *nonFlushingWriter) WriteHeader(status int)      { writer.status = status }

func TestEventHandler_Stream_WithoutFlusherIs500(t *testing.T) {
	broker := events.NewBroker()
	defer broker.Stop()
	writer := &nonFlushingWriter{header: http.Header{}}

	(&EventHandler{broker: broker}).Stream(writer, httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody))

	if writer.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on a writer that cannot stream", writer.status)
	}
}
