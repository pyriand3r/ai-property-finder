package events

import (
	"net/http/httptest"
	"testing"
	"time"
)

func drain(ch chan Event) []Event {
	var got []Event
	for {
		select {
		case event := <-ch:
			got = append(got, event)
		default:
			return got
		}
	}
}

func TestBroker_Publish_AssignsMonotonicIDsAndFansOutToAllClients(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	first, second := broker.Subscribe(), broker.Subscribe()

	broker.Publish("a", 1)
	broker.Publish("b", 2)

	for name, ch := range map[string]chan Event{"first": first, "second": second} {
		got := drain(ch)
		if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 || got[0].Type != "a" || got[1].Type != "b" {
			t.Errorf("%s client got %+v, want events a(1), b(2)", name, got)
		}
	}
}

func TestBroker_Unsubscribe_StopsDeliveryAndClosesChannel(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	ch := broker.Subscribe()
	other := broker.Subscribe()

	broker.Unsubscribe(ch)
	broker.Publish("a", nil)

	if _, open := <-ch; open {
		t.Error("unsubscribed channel still open / received an event")
	}
	if got := drain(other); len(got) != 1 {
		t.Errorf("remaining client got %d events, want 1", len(got))
	}
}

// TestBroker_Publish_SlowClientDoesNotBlockPublisher: the 64-slot client
// buffer fills up, further events are dropped instead of blocking Publish.
func TestBroker_Publish_SlowClientDoesNotBlockPublisher(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	ch := broker.Subscribe()

	done := make(chan struct{})
	go func() {
		for range 100 {
			broker.Publish("x", nil)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a client that never reads")
	}
	if got := len(drain(ch)); got != cap(ch) {
		t.Errorf("slow client received %d events, want %d (buffer size)", got, cap(ch))
	}
}

func TestBroker_Replay_DeliversOnlyEventsAfterLastID(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	for range 5 {
		broker.Publish("x", nil)
	}
	ch := make(chan Event, 16)

	if !broker.Replay(ch, 3) {
		t.Fatal("Replay returned false for an id inside the buffer")
	}
	got := drain(ch)
	if len(got) != 2 || got[0].ID != 4 || got[1].ID != 5 {
		t.Errorf("replayed %+v, want ids 4 and 5", got)
	}
}

func TestBroker_Replay_EmptyBufferIsOK(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	ch := make(chan Event, 1)

	if !broker.Replay(ch, 10) {
		t.Error("Replay on an empty buffer returned false, want true")
	}
	if got := drain(ch); len(got) != 0 {
		t.Errorf("replayed %+v from an empty buffer", got)
	}
}

// TestBroker_Replay_GapBeyondRingBufferReportsResync: once the ring has
// overwritten the client's last id, Replay must return false so the handler
// can tell the client to resync, and must not push a partial history.
func TestBroker_Replay_GapBeyondRingBufferReportsResync(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	for range broker.ringSize + 50 {
		broker.Publish("x", nil)
	}
	ch := make(chan Event, 300)

	if broker.Replay(ch, 1) {
		t.Error("Replay returned true for an id that fell out of the ring buffer")
	}
	if got := drain(ch); len(got) != 0 {
		t.Errorf("replayed %d events despite reporting a gap", len(got))
	}
}

func TestBroker_Publish_RingBufferKeepsNewestEvents(t *testing.T) {
	broker := NewBroker()
	defer broker.Stop()
	total := broker.ringSize + 10
	for range total {
		broker.Publish("x", nil)
	}

	if len(broker.ringBuffer) != broker.ringSize {
		t.Fatalf("ring holds %d events, want %d", len(broker.ringBuffer), broker.ringSize)
	}
	if oldest := broker.oldestIDLocked(); oldest != int64(total-broker.ringSize+1) {
		t.Errorf("oldest id = %d, want %d", oldest, total-broker.ringSize+1)
	}
}

func TestFormatSSE_WritesCanonicalFraming(t *testing.T) {
	recorder := httptest.NewRecorder()

	FormatSSE(recorder, Event{ID: 7, Type: "chat.message", Payload: map[string]string{"session_id": "s1"}})

	want := "id: 7\nevent: chat.message\ndata: {\"session_id\":\"s1\"}\n\n"
	if got := recorder.Body.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSSE_UnmarshalablePayloadWritesNothing(t *testing.T) {
	recorder := httptest.NewRecorder()

	FormatSSE(recorder, Event{ID: 1, Type: "x", Payload: make(chan int)})

	if recorder.Body.Len() != 0 {
		t.Errorf("wrote %q for an unmarshalable payload, want nothing", recorder.Body.String())
	}
}
