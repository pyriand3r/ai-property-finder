package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakePortal serves canned result pages by URL and records every fetch.
type fakePortal struct {
	mu    sync.Mutex
	pages map[string][]string
	errs  map[string]error
	calls []string
}

func (f *fakePortal) FetchPage(_ context.Context, pageURL string) (*fetch.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pageURL)
	if err := f.errs[pageURL]; err != nil {
		return nil, err
	}
	links, ok := f.pages[pageURL]
	if !ok {
		return nil, fmt.Errorf("unexpected fetch of %s", pageURL)
	}
	return &fetch.Result{Links: links}, nil
}

func (f *fakePortal) fetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func newTestPortalSearch(pool *pgxpool.Pool, portal *fakePortal) *PortalSearchService {
	service := NewPortalSearchService(pool, portal)
	service.pageDelay = 0
	service.wait = 5 * time.Second
	return service
}

func TestNewPortalSearchPayload_RejectsInvalidRequests(t *testing.T) {
	cases := map[string]domain.PortalSearchRequest{
		"url":        {URL: "ftp://x.test/search"},
		"max_pages":  {URL: "https://x.test/s", MaxPages: 6},
		"page_param": {URL: "https://x.test/s", MaxPages: 2},
	}
	for field, request := range cases {
		_, err := newPortalSearchPayload(request)
		var validation *domain.ValidationError
		if !errors.As(err, &validation) || validation.Field != field {
			t.Errorf("%s: err = %v, want a ValidationError for that field", field, err)
		}
	}
}

// TestPortalSearchPayload_AddLinks_KeepsOnlyCandidateListings: navigation
// to other hosts, the result pages themselves and fragment-only variants of a
// link must not reach the Agent, or it would try to ingest them (spec 01-16).
func TestPortalSearchPayload_AddLinks_KeepsOnlyCandidateListings(t *testing.T) {
	payload, err := newPortalSearchPayload(domain.PortalSearchRequest{
		URL: "https://www.portal.test/search?q=hus", PageParam: "page", MaxPages: 2, LinkContains: "/bostad/",
	})
	if err != nil {
		t.Fatal(err)
	}

	added := payload.addLinks([]string{
		"https://portal.test/bostad/1/",
		"https://portal.test/bostad/1#photos", // same page as the first
		"https://portal.test/bostad/2",
		"https://other.test/bostad/3", // other host
		"https://portal.test/about",   // filtered by link_contains
		"mailto:info@portal.test",
	})

	want := []string{"https://portal.test/bostad/1", "https://portal.test/bostad/2"}
	if added != len(want) || !slices.Equal(payload.Links, want) {
		t.Errorf("added %d links %v, want %v", added, payload.Links, want)
	}

	// Without link_contains the result pages themselves are still not candidates.
	unfiltered, _ := newPortalSearchPayload(domain.PortalSearchRequest{URL: "https://www.portal.test/search?q=hus", PageParam: "page", MaxPages: 2})
	unfiltered.addLinks([]string{"https://portal.test/search?q=hus", "https://portal.test/search?page=2&q=hus", "https://portal.test/bostad/9"})
	if wantUnfiltered := []string{"https://portal.test/bostad/9"}; !slices.Equal(unfiltered.Links, wantUnfiltered) {
		t.Errorf("unfiltered links = %v, want %v", unfiltered.Links, wantUnfiltered)
	}
	if got := payload.pageURL(2); got != "https://www.portal.test/search?page=2&q=hus" {
		t.Errorf("pageURL(2) = %q", got)
	}
}

func listingSourceURL(t *testing.T, pool *pgxpool.Pool, profileID uuid.UUID, sourceURL string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO listing (id, profile_id, title, source_url) VALUES ($1, $2, 'Haus', $3)`, uuid.New(), profileID, sourceURL); err != nil {
		t.Fatalf("seed listing: %v", err)
	}
}

func TestPortalSearchService_Start_CollectsPagesUntilResultsRepeat(t *testing.T) {
	pool := dbtest.NewPool(t)
	profileID := seedProfile(t, pool)
	listingSourceURL(t, pool, profileID, "https://portal.test/bostad/1")
	portal := &fakePortal{pages: map[string][]string{
		"https://portal.test/s":        {"https://portal.test/bostad/1", "https://portal.test/bostad/2"},
		"https://portal.test/s?page=2": {"https://portal.test/bostad/3"},
		"https://portal.test/s?page=3": {"https://portal.test/bostad/3"}, // last page repeats
	}}

	search, err := newTestPortalSearch(pool, portal).Start(context.Background(), profileID, domain.PortalSearchRequest{
		URL: "https://portal.test/s", PageParam: "page", MaxPages: 4,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if search.Status != domain.TaskStatusSucceeded || search.PagesDone != 2 || search.StopReason == "" {
		t.Errorf("search = %+v, want succeeded after 2 pages with a stop reason", search)
	}
	if want := []string{"https://portal.test/bostad/2", "https://portal.test/bostad/3"}; !slices.Equal(search.NewLinks, want) {
		t.Errorf("new links = %v, want %v", search.NewLinks, want)
	}
	if want := []string{"https://portal.test/bostad/1"}; !slices.Equal(search.KnownLinks, want) {
		t.Errorf("known links = %v, want %v", search.KnownLinks, want)
	}
	if len(portal.fetched()) != 3 {
		t.Errorf("fetched %v, want pages 1-3 and no page 4", portal.fetched())
	}
}

func TestPortalSearchService_Start_FirstPageFailureFailsTheTask(t *testing.T) {
	pool := dbtest.NewPool(t)
	profileID := seedProfile(t, pool)
	portal := &fakePortal{errs: map[string]error{"https://portal.test/s": errors.New("403 Forbidden")}}

	search, err := newTestPortalSearch(pool, portal).Start(context.Background(), profileID, domain.PortalSearchRequest{URL: "https://portal.test/s"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if search.Status != domain.TaskStatusFailed || search.Error == nil || search.PagesDone != 0 {
		t.Errorf("search = %+v, want a failed task with an error", search)
	}
}

func TestPortalSearchService_Status_HidesOtherProfilesTasks(t *testing.T) {
	pool := dbtest.NewPool(t)
	owner, other := seedProfile(t, pool), seedProfile(t, pool)
	portal := &fakePortal{pages: map[string][]string{"https://portal.test/s": {}}}
	service := newTestPortalSearch(pool, portal)

	search, err := service.Start(context.Background(), owner, domain.PortalSearchRequest{URL: "https://portal.test/s"})
	if err != nil {
		t.Fatal(err)
	}
	if search.StopReason == "" {
		t.Errorf("an empty result page must say why it is empty, got %+v", search)
	}

	_, err = service.Status(context.Background(), other, search.TaskID)
	if _, ok := errors.AsType[*domain.NotFoundError](err); !ok {
		t.Errorf("err = %v, want NotFoundError for a task of another profile", err)
	}
}

// seedRunningSearch inserts the state a crashed process leaves behind: a
// running task whose payload says page 1 is done.
func seedRunningSearch(t *testing.T, pool *pgxpool.Pool, profileID uuid.UUID, payload portalSearchPayload) uuid.UUID {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	total, done := payload.MaxPages, payload.NextPage-1
	task := &domain.Task{
		ID: uuid.New(), Type: domain.TaskTypePortalSearch, Status: domain.TaskStatusRunning,
		ProfileID: &profileID, Total: &total, Done: done, CreatedAt: now, StartedAt: &now, UpdatedAt: now,
	}
	if err := repository.NewTaskRepo(pool).CreateWithPayload(context.Background(), task, encoded); err != nil {
		t.Fatal(err)
	}
	return task.ID
}

func waitForSearch(t *testing.T, service *PortalSearchService, profileID, taskID uuid.UUID) *domain.PortalSearch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		search, err := service.Status(context.Background(), profileID, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if search.Status != domain.TaskStatusRunning || time.Now().After(deadline) {
			return search
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPortalSearchService_Recover_ContinuesAtTheSavedPage is the point of
// docs/adr/0028: after a restart the search resumes at the page it was on,
// keeps the links it already had and does not fetch finished pages again.
func TestPortalSearchService_Recover_ContinuesAtTheSavedPage(t *testing.T) {
	pool := dbtest.NewPool(t)
	profileID := seedProfile(t, pool)
	taskID := seedRunningSearch(t, pool, profileID, portalSearchPayload{
		URL: "https://portal.test/s", PageParam: "page", MaxPages: 3, NextPage: 2, Links: []string{"https://portal.test/bostad/1"},
	})
	portal := &fakePortal{pages: map[string][]string{
		"https://portal.test/s?page=2": {"https://portal.test/bostad/2"},
		"https://portal.test/s?page=3": {"https://portal.test/bostad/3"},
	}}
	service := newTestPortalSearch(pool, portal)

	resumed, err := service.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	search := waitForSearch(t, service, profileID, taskID)

	if resumed != 1 || search.Status != domain.TaskStatusSucceeded || search.PagesDone != 3 {
		t.Errorf("resumed %d, search = %+v, want one resumed search finished after 3 pages", resumed, search)
	}
	if len(search.NewLinks) != 3 {
		t.Errorf("new links = %v, want the saved link plus pages 2 and 3", search.NewLinks)
	}
	if got := portal.fetched(); len(got) != 2 || got[0] != "https://portal.test/s?page=2" {
		t.Errorf("fetched %v, want pages 2 and 3 only", got)
	}
}

// TestPortalSearchService_Recover_GivesUpAfterRepeatedRestarts: a search that
// keeps crashing the process must not restart on every boot.
func TestPortalSearchService_Recover_GivesUpAfterRepeatedRestarts(t *testing.T) {
	pool := dbtest.NewPool(t)
	profileID := seedProfile(t, pool)
	taskID := seedRunningSearch(t, pool, profileID, portalSearchPayload{
		URL: "https://portal.test/s", MaxPages: 1, NextPage: 1, Recoveries: portalSearchMaxRecoveries, Links: []string{},
	})
	portal := &fakePortal{}
	service := newTestPortalSearch(pool, portal)

	resumed, err := service.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	search, err := service.Status(context.Background(), profileID, taskID)
	if err != nil {
		t.Fatal(err)
	}

	if resumed != 0 || search.Status != domain.TaskStatusFailed || search.Error == nil || len(portal.fetched()) != 0 {
		t.Errorf("resumed %d, search = %+v, fetched %v, want a failed task and no fetch", resumed, search, portal.fetched())
	}
}
