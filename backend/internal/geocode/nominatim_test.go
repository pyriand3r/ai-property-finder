package geocode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// useServer points Geocode at handler and resets the rate limiter so tests
// don't wait out minInterval between each other.
func useServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := baseURL
	baseURL = server.URL
	resetThrottle()
	t.Cleanup(func() {
		server.Close()
		baseURL = previous
		resetThrottle()
	})
}

func resetThrottle() {
	rateMu.Lock()
	lastCall = time.Time{}
	rateMu.Unlock()
}

func TestGeocode_Success_ParsesStringCoordinatesAndSendsQuery(t *testing.T) {
	var gotQuery, gotAgent string
	useServer(t, func(writer http.ResponseWriter, request *http.Request) {
		gotQuery = request.URL.RawQuery
		gotAgent = request.Header.Get("User-Agent")
		_, _ = writer.Write([]byte(`[{"lat":"38.7223","lon":"-9.1393","display_name":"Lisboa, Portugal"}]`))
	})

	result, err := Geocode(context.Background(), "Rua Augusta 1, Lisboa", " PT ")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}

	if result.Lat != 38.7223 || result.Lon != -9.1393 || result.DisplayName != "Lisboa, Portugal" {
		t.Errorf("result = %+v", result)
	}
	for _, want := range []string{"q=Rua+Augusta+1%2C+Lisboa", "format=json", "limit=1", "countrycodes=pt"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q lacks %q", gotQuery, want)
		}
	}
	if gotAgent != userAgent {
		t.Errorf("User-Agent = %q, want %q (Nominatim policy requires an identifying agent)", gotAgent, userAgent)
	}
}

func TestGeocode_BlankCountryOmitsCountryCodes(t *testing.T) {
	var gotQuery string
	useServer(t, func(writer http.ResponseWriter, request *http.Request) {
		gotQuery = request.URL.RawQuery
		_, _ = writer.Write([]byte(`[{"lat":"1","lon":"2","display_name":"x"}]`))
	})

	if _, err := Geocode(context.Background(), "x", "  "); err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if strings.Contains(gotQuery, "countrycodes") {
		t.Errorf("query %q contains countrycodes for a blank country", gotQuery)
	}
}

func TestGeocode_EmptyResultIsErrNotFound(t *testing.T) {
	useServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`[]`))
	})

	if _, err := Geocode(context.Background(), "nowhere", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGeocode_FailureModes_AreNotErrNotFound(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"non-200 status": func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "slow down", http.StatusTooManyRequests)
		},
		"invalid json": func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`not json`))
		},
		"unparseable latitude": func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`[{"lat":"abc","lon":"2","display_name":"x"}]`))
		},
		"unparseable longitude": func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`[{"lat":"1","lon":"abc","display_name":"x"}]`))
		},
	}
	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			useServer(t, handler)

			result, err := Geocode(context.Background(), "x", "")

			if err == nil || result != nil {
				t.Fatalf("got (%v, %v), want an error", result, err)
			}
			if errors.Is(err, ErrNotFound) {
				t.Errorf("upstream failure reported as ErrNotFound: %v", err)
			}
		})
	}
}

func TestGeocode_NonOKErrorIncludesStatusAndBody(t *testing.T) {
	useServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "blocked by policy", http.StatusForbidden)
	})

	_, err := Geocode(context.Background(), "x", "")

	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "blocked by policy") {
		t.Errorf("err = %v, want status and body in message", err)
	}
}

// TestGeocode_ThrottlesConsecutiveCalls pins Nominatim's 1 req/s fair-use
// policy (ADR 0011): the second call must wait out minInterval.
func TestGeocode_ThrottlesConsecutiveCalls(t *testing.T) {
	useServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`[{"lat":"1","lon":"2","display_name":"x"}]`))
	})

	if _, err := Geocode(context.Background(), "x", ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	start := time.Now()
	if _, err := Geocode(context.Background(), "x", ""); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if elapsed := time.Since(start); elapsed < minInterval-100*time.Millisecond {
		t.Errorf("second call returned after %v, want it delayed by ~%v", elapsed, minInterval)
	}
}

// TestGeocode_CanceledContextAbortsThrottleWait: a canceled caller must
// not sit out the full rate-limit wait.
func TestGeocode_CanceledContextAbortsThrottleWait(t *testing.T) {
	useServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`[{"lat":"1","lon":"2","display_name":"x"}]`))
	})
	if _, err := Geocode(context.Background(), "x", ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	_, err := Geocode(ctx, "x", "")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > minInterval/2 {
		t.Errorf("canceled call took %v, want it to abort the throttle wait early", elapsed)
	}
}
