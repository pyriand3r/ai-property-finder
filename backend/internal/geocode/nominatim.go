// Package geocode resolves an address string to coordinates via Nominatim
// (OpenStreetMap's geocoder) — the sole MVP geocoding backend (ADR 0011).
// No API key, no app_config entry; the URL is hardcoded per that ADR.
package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	baseURL   = "https://nominatim.openstreetmap.org/search"
	userAgent = "PropertyFinderOBS/1.0 (single-user self-hosted app; https://github.com/pyriand3r/prop-finder)"
	// minInterval enforces Nominatim's 1 req/s fair-use policy. The Agent
	// loop dispatches tool calls sequentially anyway, so this is
	// defense-in-depth, not the primary throttle (ADR 0011).
	minInterval = time.Second
)

// ErrNotFound means Nominatim returned zero results for the query.
var ErrNotFound = errors.New("address_not_found")

// Result is a resolved address.
type Result struct {
	Lat         float64
	Lon         float64
	DisplayName string
}

// rawResult is Nominatim's wire shape — lat/lon come back as strings.
type rawResult struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	DisplayName string `json:"display_name"`
}

var (
	rateMu   sync.Mutex
	lastCall time.Time
)

// Geocode resolves address to coordinates, optionally biased by country
// (ISO 3166-1 alpha-2). Blocks briefly if called less than minInterval
// after the previous call. Returns ErrNotFound when Nominatim has no
// match; any other non-nil error is a transport/upstream failure.
func Geocode(ctx context.Context, address, country string) (*Result, error) {
	throttle(ctx)

	query := url.Values{
		"q":      {address},
		"format": {"json"},
		"limit":  {"1"},
	}
	if strings.TrimSpace(country) != "" {
		query.Set("countrycodes", strings.ToLower(strings.TrimSpace(country)))
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build geocode request: %w", err)
	}
	request.Header.Set("User-Agent", userAgent)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("geocode request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return nil, fmt.Errorf("geocode request failed: %s: %s", response.Status, string(body))
	}

	var results []rawResult
	if decodeError := json.NewDecoder(response.Body).Decode(&results); decodeError != nil {
		return nil, fmt.Errorf("decode geocode response: %w", decodeError)
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}

	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("parse latitude: %w", err)
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("parse longitude: %w", err)
	}

	return &Result{Lat: lat, Lon: lon, DisplayName: results[0].DisplayName}, nil
}

// throttle blocks until at least minInterval has passed since the previous
// call, or ctx is done.
func throttle(ctx context.Context) {
	rateMu.Lock()
	wait := minInterval - time.Since(lastCall)
	if wait > 0 {
		timer := time.NewTimer(wait)
		rateMu.Unlock()
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
		rateMu.Lock()
	}
	lastCall = time.Now()
	rateMu.Unlock()
}
