package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewProvider_OpenAICompatibleNeedsBaseURLButNoKey(t *testing.T) {
	for _, id := range []string{"lmstudio", "opencode"} {
		withURL, err := NewProvider(Config{Provider: id, BaseURL: "http://localhost:1234/v1", Model: "m"})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if _, ok := withURL.(*openaiProvider); !ok {
			t.Errorf("%s with base url: got %T, want *openaiProvider", id, withURL)
		}
		withoutURL, _ := NewProvider(Config{Provider: id})
		if _, ok := withoutURL.(*unconfiguredProvider); !ok {
			t.Errorf("%s without base url: got %T, want unconfigured", id, withoutURL)
		}
	}
}

func TestOpenAIComplete_ReturnsAssistantText(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"[1,2]"}}]}`))
	}))
	defer server.Close()

	got, err := newOpenAIProvider(Config{BaseURL: server.URL + "/v1", Model: "m"}).complete(context.Background(), "s", "u")
	if err != nil || got != "[1,2]" {
		t.Fatalf("complete = %q, %v", got, err)
	}
	if path != "/v1/chat/completions" {
		t.Errorf("path = %s", path)
	}
}

// TestOpenAIComplete_ContextOverflowMapsToListingTooLong: LM Studio reports
// an overflowing prompt as a 400 with a "context overflow" message.
func TestOpenAIComplete_ContextOverflowMapsToListingTooLong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Trying to keep the first 5000 tokens when context overflows"}`))
	}))
	defer server.Close()

	_, err := newOpenAIProvider(Config{BaseURL: server.URL, Model: "m"}).complete(context.Background(), "s", "u")
	if !errors.Is(err, ErrListingTooLong) {
		t.Fatalf("err = %v, want ErrListingTooLong", err)
	}
}
