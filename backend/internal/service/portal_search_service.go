package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	portalSearchMaxPages = 5
	// portalSearchMaxLinks bounds the collected links, so the Tool result
	// stays small enough for the model's context.
	portalSearchMaxLinks = 300
	// portalSearchMaxRecoveries is how often a task is resumed after a
	// restart. A search that keeps killing the process must not restart
	// forever (docs/adr/0028).
	portalSearchMaxRecoveries = 3
)

// pageFetcher is the slice of *fetch.Client a portal search needs.
type pageFetcher interface {
	FetchPage(ctx context.Context, pageURL string) (*fetch.Result, error)
}

// PortalSearchService backs the search_portal Tool (spec 01-16, docs/adr/
// 0028). A search is a `task` row of type portal.search whose payload holds
// everything needed to continue it: the request, the next page and the
// links collected so far. Unlike a chat turn (ADR 0008) it costs no LLM
// call, so after a restart Recover simply continues the page that was
// running.
type PortalSearchService struct {
	pool    *pgxpool.Pool
	fetcher pageFetcher
	// pageDelay is the pause between two pages of one search, so a portal
	// is not hit in a burst.
	pageDelay time.Duration
	// wait is how long Start blocks for the result before it returns the
	// running task instead.
	wait time.Duration
}

// NewPortalSearchService constructs the service. The pool may be shared with
// every other service; the service keeps no state of its own, so one instance
// per Agent turn (NewToolRegistry) is fine.
func NewPortalSearchService(pool *pgxpool.Pool, fetcher pageFetcher) *PortalSearchService {
	return &PortalSearchService{pool: pool, fetcher: fetcher, pageDelay: time.Second, wait: 30 * time.Second}
}

// portalSearchPayload is the JSON in task.payload.
type portalSearchPayload struct {
	URL          string   `json:"url"`
	PageParam    string   `json:"page_param,omitempty"`
	LinkContains string   `json:"link_contains,omitempty"`
	MaxPages     int      `json:"max_pages"`
	NextPage     int      `json:"next_page"`
	Recoveries   int      `json:"recoveries"`
	Links        []string `json:"links"`
	StopReason   string   `json:"stop_reason,omitempty"`
}

func newPortalSearchPayload(request domain.PortalSearchRequest) (*portalSearchPayload, error) {
	searchURL := strings.TrimSpace(request.URL)
	parsed, err := url.Parse(searchURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, &domain.ValidationError{Field: "url", Message: "url muss eine absolute http(s)-URL sein"}
	}
	maxPages := request.MaxPages
	if maxPages == 0 {
		maxPages = 1
	}
	if maxPages < 1 || maxPages > portalSearchMaxPages {
		return nil, &domain.ValidationError{Field: "max_pages", Message: fmt.Sprintf("max_pages muss zwischen 1 und %d liegen", portalSearchMaxPages)}
	}
	pageParam := strings.TrimSpace(request.PageParam)
	if maxPages > 1 && pageParam == "" {
		return nil, &domain.ValidationError{Field: "page_param", Message: "page_param (Name des Seiten-Query-Parameters, z.B. page) ist bei max_pages > 1 erforderlich"}
	}
	return &portalSearchPayload{
		URL:          searchURL,
		PageParam:    pageParam,
		LinkContains: strings.TrimSpace(request.LinkContains),
		MaxPages:     maxPages,
		NextPage:     1,
		Links:        []string{},
	}, nil
}

// pageURL is the URL of result page n: the search URL itself for page 1,
// otherwise the same URL with the page query parameter set to n.
func (p *portalSearchPayload) pageURL(n int) string {
	if n <= 1 {
		return p.URL
	}
	parsed, err := url.Parse(p.URL)
	if err != nil {
		return p.URL
	}
	query := parsed.Query()
	query.Set(p.PageParam, strconv.Itoa(n))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// addLinks collects the links of one fetched page that look like candidates:
// same host as the search URL, no fragment, not a result page itself, and
// containing LinkContains when set. It returns how many links were new.
func (p *portalSearchPayload) addLinks(links []string) int {
	searchHost := hostKey(p.URL)
	skip := make(map[string]bool, p.MaxPages)
	for page := 1; page <= p.MaxPages; page++ {
		skip[matchKey(p.pageURL(page))] = true
	}
	seen := make(map[string]bool, len(p.Links))
	for _, link := range p.Links {
		seen[link] = true
	}

	added := 0
	for _, link := range links {
		if len(p.Links) >= portalSearchMaxLinks {
			break
		}
		if hostKey(link) != searchHost {
			continue
		}
		normalized := normalizeLink(link)
		if normalized == "" || skip[matchKey(normalized)] || seen[normalized] {
			continue
		}
		if p.LinkContains != "" && !strings.Contains(normalized, p.LinkContains) {
			continue
		}
		seen[normalized] = true
		p.Links = append(p.Links, normalized)
		added++
	}
	return added
}

// hostKey is the host of rawURL without a leading "www.", or "" when rawURL
// is not an http(s) URL.
func hostKey(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
}

// normalizeLink drops the fragment and a trailing slash, so the same page
// linked two ways counts once and compares equal to a stored source_url.
func normalizeLink(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/")
}

// matchKey is the form of a URL used to decide whether two URLs name the same
// page: normalizeLink, without "www." and with the query keys sorted. It is
// only for comparing, never returned to the Agent (some portals need the www).
func matchKey(rawURL string) string {
	parsed, err := url.Parse(normalizeLink(rawURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Host = strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	parsed.RawQuery = parsed.Query().Encode()
	return parsed.String()
}

// Start creates the task, runs it in a detached goroutine and waits up to
// s.wait for it. A finished search returns its links; a slower one returns
// the running task, and the caller asks again with Status.
func (s *PortalSearchService) Start(ctx context.Context, profileID uuid.UUID, request domain.PortalSearchRequest) (*domain.PortalSearch, error) {
	payload, err := newPortalSearchPayload(request)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode portal search payload: %w", err)
	}

	now := time.Now().UTC()
	total := payload.MaxPages
	task := &domain.Task{
		ID:        uuid.New(),
		Type:      domain.TaskTypePortalSearch,
		Status:    domain.TaskStatusRunning,
		ProfileID: &profileID,
		Total:     &total,
		CreatedAt: now,
		StartedAt: &now,
		UpdatedAt: now,
	}
	if createError := repository.NewTaskRepo(s.pool).CreateWithPayload(ctx, task, encoded); createError != nil {
		return nil, createError
	}

	finished := make(chan struct{})
	// Detached on purpose (ADR 0008 shape): the search must outlive the
	// Agent turn or MCP request that started it.
	//nolint:gosec,contextcheck // G118/non-inherited context: intentional, see comment above.
	go func() {
		defer close(finished)
		s.run(context.Background(), task.ID, payload)
	}()

	select {
	case <-finished:
	case <-time.After(s.wait):
	case <-ctx.Done():
	}
	return s.Status(ctx, profileID, task.ID)
}

// Status returns the current state of a portal search of profileID. Links
// are split at read time into new and already-known ones, so a Listing saved
// since the search ran is reported as known.
func (s *PortalSearchService) Status(ctx context.Context, profileID, taskID uuid.UUID) (*domain.PortalSearch, error) {
	taskRepo := repository.NewTaskRepo(s.pool)
	task, err := taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Type != domain.TaskTypePortalSearch || task.ProfileID == nil || *task.ProfileID != profileID {
		return nil, &domain.NotFoundError{Entity: "task", ID: taskID.String()}
	}
	raw, err := taskRepo.GetPayload(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var payload portalSearchPayload
	if unmarshalError := json.Unmarshal(raw, &payload); unmarshalError != nil {
		return nil, fmt.Errorf("decode portal search payload: %w", unmarshalError)
	}

	listings, err := repository.NewListingRepo(s.pool).ListByProfile(ctx, profileID, nil)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(listings))
	for _, listing := range listings {
		if listing.SourceURL != nil {
			known[matchKey(*listing.SourceURL)] = true
		}
	}

	search := &domain.PortalSearch{
		TaskID:     task.ID,
		Status:     task.Status,
		URL:        payload.URL,
		PagesDone:  task.Done,
		MaxPages:   payload.MaxPages,
		NewLinks:   []string{},
		KnownLinks: []string{},
		StopReason: payload.StopReason,
		Error:      task.Error,
	}
	for _, link := range payload.Links {
		if known[matchKey(link)] {
			search.KnownLinks = append(search.KnownLinks, link)
		} else {
			search.NewLinks = append(search.NewLinks, link)
		}
	}
	return search, nil
}

// Recover continues every portal search that was running when the process
// last stopped (docs/adr/0028). A task is resumed at the page it was on, with
// the links it had; one that was already resumed portalSearchMaxRecoveries
// times is failed instead. It returns once the goroutines are started.
func (s *PortalSearchService) Recover(ctx context.Context) (int, error) {
	taskRepo := repository.NewTaskRepo(s.pool)
	tasks, err := taskRepo.ListRunning(ctx, domain.TaskTypePortalSearch)
	if err != nil {
		return 0, err
	}

	resumed := 0
	for _, task := range tasks {
		raw, payloadError := taskRepo.GetPayload(ctx, task.ID)
		var payload portalSearchPayload
		if payloadError == nil {
			payloadError = json.Unmarshal(raw, &payload)
		}
		if payloadError != nil {
			s.fail(ctx, task.ID, "Suche konnte nicht fortgesetzt werden: Zustand unlesbar")
			continue
		}
		if payload.Recoveries >= portalSearchMaxRecoveries {
			s.fail(ctx, task.ID, fmt.Sprintf("Suche nach %d Neustarts abgebrochen", payload.Recoveries))
			continue
		}
		payload.Recoveries++
		if saveError := s.save(ctx, task.ID, task.Done, task.FailedCount, &payload); saveError != nil {
			return resumed, saveError
		}
		go s.run(ctx, task.ID, &payload)
		resumed++
	}
	return resumed, nil
}

// run fetches the remaining pages one by one, saving payload and counters
// after every page so a restart loses at most the page in flight.
func (s *PortalSearchService) run(ctx context.Context, taskID uuid.UUID, payload *portalSearchPayload) {
	failed := 0
	for payload.NextPage <= payload.MaxPages {
		page := payload.NextPage
		if page > 1 && !sleepContext(ctx, s.pageDelay) {
			return
		}

		result, fetchError := s.fetcher.FetchPage(ctx, payload.pageURL(page))
		if ctx.Err() != nil {
			// Shutting down: leave the task running so Recover continues it.
			return
		}
		if fetchError != nil {
			failed++
			payload.StopReason = fmt.Sprintf("Seite %d konnte nicht geladen werden: %v", page, fetchError)
			break
		}
		if len(result.PDFData) > 0 {
			failed++
			payload.StopReason = fmt.Sprintf("Seite %d ist ein PDF, keine Ergebnisliste", page)
			break
		}
		if added := payload.addLinks(result.Links); added == 0 && page > 1 {
			payload.StopReason = fmt.Sprintf("Seite %d enthält keine neuen Links – Ende der Ergebnisse", page)
			break
		}
		payload.NextPage++
		if saveError := s.save(ctx, taskID, payload.NextPage-1, failed, payload); saveError != nil {
			log.Printf("portal search %s: save progress: %v", taskID, saveError)
		}
	}

	pagesDone := payload.NextPage - 1
	if len(payload.Links) == 0 && payload.StopReason == "" {
		payload.StopReason = "Keine Links gefunden (Seite per JavaScript gerendert oder blockiert?)"
	}
	if saveError := s.save(ctx, taskID, pagesDone, failed, payload); saveError != nil {
		log.Printf("portal search %s: save result: %v", taskID, saveError)
	}

	status := domain.TaskStatusSucceeded
	var errMsg *string
	if pagesDone == 0 {
		status = domain.TaskStatusFailed
		errMsg = &payload.StopReason
	}
	if completeError := repository.NewTaskRepo(s.pool).Complete(ctx, taskID, status, errMsg); completeError != nil {
		log.Printf("portal search %s: complete: %v", taskID, completeError)
	}
}

func (s *PortalSearchService) save(ctx context.Context, taskID uuid.UUID, done, failed int, payload *portalSearchPayload) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode portal search payload: %w", err)
	}
	return repository.NewTaskRepo(s.pool).SaveProgress(ctx, taskID, done, failed, encoded)
}

func (s *PortalSearchService) fail(ctx context.Context, taskID uuid.UUID, message string) {
	if err := repository.NewTaskRepo(s.pool).Complete(ctx, taskID, domain.TaskStatusFailed, &message); err != nil {
		log.Printf("portal search %s: mark failed: %v", taskID, err)
	}
}

// sleepContext waits for d and reports false when ctx ended first.
func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
