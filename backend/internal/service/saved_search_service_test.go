package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakePortalRunner struct {
	result  domain.PortalSearch
	err     error
	release chan struct{} // when set, Run waits for it to be closed
	calls   int
}

func (f *fakePortalRunner) Run(_ context.Context, _ uuid.UUID, _ domain.PortalSearchRequest) (*domain.PortalSearch, error) {
	f.calls++
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return nil, f.err
	}
	result := f.result
	result.TaskID = uuid.New()
	return &result, nil
}

type fakeChatImporter struct {
	started bool
	err     error
	sent    []string
}

func (f *fakeChatImporter) SendToActiveSession(_ context.Context, _ uuid.UUID, content string) (bool, error) {
	if f.err != nil || !f.started {
		return false, f.err
	}
	f.sent = append(f.sent, content)
	return true, nil
}

func succeededSearch(links ...string) domain.PortalSearch {
	return domain.PortalSearch{Status: domain.TaskStatusSucceeded, NewLinks: links, KnownLinks: []string{"https://x.test/known"}}
}

func validSavedSearchInput() domain.SavedSearchInput {
	return domain.SavedSearchInput{
		Name: "Algarve", URL: "https://x.test/search", MaxPages: 1, IntervalHours: 24,
		Enabled: true, ImportNew: true, MaxImport: 2,
	}
}

type savedSearchFixture struct {
	pool      *pgxpool.Pool
	service   *SavedSearchService
	portal    *fakePortalRunner
	importer  *fakeChatImporter
	profileID uuid.UUID
	now       time.Time
}

func newSavedSearchFixture(t *testing.T) *savedSearchFixture {
	t.Helper()
	pool := dbtest.NewPool(t)
	fixture := &savedSearchFixture{
		pool: pool, portal: &fakePortalRunner{}, importer: &fakeChatImporter{started: true},
		profileID: uuid.New(), now: time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC),
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO search_profile (id, name) VALUES ($1, 'P')`, fixture.profileID); err != nil {
		t.Fatal(err)
	}
	fixture.service = NewSavedSearchService(pool, fixture.portal, fixture.importer)
	fixture.service.now = func() time.Time { return fixture.now }
	return fixture
}

func (f *savedSearchFixture) create(t *testing.T, modify func(*domain.SavedSearchInput)) *domain.SavedSearch {
	t.Helper()
	input := validSavedSearchInput()
	if modify != nil {
		modify(&input)
	}
	search, err := f.service.Create(context.Background(), f.profileID, input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return search
}

func (f *savedSearchFixture) get(t *testing.T, id uuid.UUID) *domain.SavedSearch {
	t.Helper()
	search, err := f.service.get(context.Background(), f.profileID, id)
	if err != nil {
		t.Fatal(err)
	}
	return search
}

// TestSavedSearchService_Create_RejectsInvalidInput needs no database: the
// checks run before any query.
func TestSavedSearchService_Create_RejectsInvalidInput(t *testing.T) {
	service := NewSavedSearchService(nil, nil, nil)
	cases := map[string]func(*domain.SavedSearchInput){
		"name":           func(in *domain.SavedSearchInput) { in.Name = "  " },
		"url":            func(in *domain.SavedSearchInput) { in.URL = "ftp://x.test" },
		"max_pages":      func(in *domain.SavedSearchInput) { in.MaxPages = 6 },
		"page_param":     func(in *domain.SavedSearchInput) { in.MaxPages = 2 },
		"interval_hours": func(in *domain.SavedSearchInput) { in.IntervalHours = 0 },
		"max_import":     func(in *domain.SavedSearchInput) { in.MaxImport = 11 },
	}
	for field, modify := range cases {
		input := validSavedSearchInput()
		modify(&input)
		_, err := service.Create(context.Background(), uuid.New(), input)
		var validation *domain.ValidationError
		if !errors.As(err, &validation) || validation.Field != field {
			t.Errorf("%s: err = %v, want a ValidationError for that field", field, err)
		}
	}
}

func TestRememberLinks_KeepsTheNewestWithinTheCap(t *testing.T) {
	seen := make([]string, savedSearchMaxSeen)
	for i := range seen {
		seen[i] = fmt.Sprintf("old%d", i)
	}
	got := rememberLinks(seen, []string{"https://www.x.test/a/"})
	if len(got) != savedSearchMaxSeen || got[0] != "old1" || got[len(got)-1] != matchKey("https://x.test/a") {
		t.Errorf("rememberLinks kept %d entries, first %q, last %q", len(got), got[0], got[len(got)-1])
	}
}

// TestSavedSearchService_RunDue_ImportsInBatchesAndRemembersLinks covers a
// scheduled run end to end: only due searches run, at most MaxImport links go
// to the Agent, the rest stay for the next run, and a link handed over once
// is never handed over again.
func TestSavedSearchService_RunDue_ImportsInBatchesAndRemembersLinks(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, nil)
	fixture.portal.result = succeededSearch("https://x.test/a", "https://x.test/b", "https://x.test/c")

	if search.NextRunAt == nil || !search.NextRunAt.Equal(fixture.now) {
		t.Fatalf("next_run_at = %v, want now (first run at the next tick)", search.NextRunAt)
	}
	ran, err := fixture.service.RunDue(ctx)
	if err != nil || ran != 1 {
		t.Fatalf("RunDue = %d, %v; want 1, nil", ran, err)
	}
	after := fixture.get(t, search.ID)
	if after.LastStatus != domain.SavedSearchSucceeded || after.LastFound != 4 || after.LastNew != 3 || after.LastImported != 2 {
		t.Errorf("after first run: status %s found %d new %d imported %d, want succeeded 4 3 2",
			after.LastStatus, after.LastFound, after.LastNew, after.LastImported)
	}
	if after.NextRunAt == nil || !after.NextRunAt.Equal(fixture.now.Add(24*time.Hour)) {
		t.Errorf("next_run_at = %v, want one interval after the run", after.NextRunAt)
	}
	if len(fixture.importer.sent) != 1 || !strings.Contains(fixture.importer.sent[0], "https://x.test/b") || strings.Contains(fixture.importer.sent[0], "https://x.test/c\n") {
		t.Fatalf("message to the Agent = %q, want links a and b only", fixture.importer.sent)
	}

	if ran, _ = fixture.service.RunDue(ctx); ran != 0 {
		t.Errorf("RunDue before the next run is due ran %d searches, want 0", ran)
	}

	fixture.now = fixture.now.Add(25 * time.Hour)
	if ran, _ = fixture.service.RunDue(ctx); ran != 1 {
		t.Fatalf("RunDue a day later ran %d searches, want 1", ran)
	}
	second := fixture.importer.sent[1]
	if strings.Contains(second, "https://x.test/a") || strings.Contains(second, "https://x.test/b") || !strings.Contains(second, "https://x.test/c") {
		t.Errorf("second message = %q, want only link c", second)
	}
	if got := fixture.get(t, search.ID); got.LastNew != 1 || got.LastImported != 1 {
		t.Errorf("second run: new %d imported %d, want 1 1", got.LastNew, got.LastImported)
	}
}

// TestSavedSearchService_RunDue_BusyChatKeepsLinksForNextRun: when the Agent
// cannot take the links, they must not count as seen, or they would be lost.
func TestSavedSearchService_RunDue_BusyChatKeepsLinksForNextRun(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, nil)
	fixture.portal.result = succeededSearch("https://x.test/a")
	fixture.importer.started = false

	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fixture.get(t, search.ID); got.LastStatus != domain.SavedSearchSucceeded || got.LastImported != 0 || !strings.Contains(got.LastSummary, "übersprungen") {
		t.Errorf("busy run = status %s imported %d summary %q, want succeeded, 0, skipped", got.LastStatus, got.LastImported, got.LastSummary)
	}

	fixture.importer.started = true
	fixture.now = fixture.now.Add(25 * time.Hour)
	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fixture.importer.sent) != 1 || !strings.Contains(fixture.importer.sent[0], "https://x.test/a") {
		t.Errorf("messages = %q, want the link handed over at the second run", fixture.importer.sent)
	}
}

// TestSavedSearchService_RunDue_ReportsWithoutImport: with import off the Agent
// is not called, and the links still count as reported, so the next run
// shows only what is new since.
func TestSavedSearchService_RunDue_ReportsWithoutImport(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, func(in *domain.SavedSearchInput) { in.ImportNew = false })
	fixture.portal.result = succeededSearch("https://x.test/a")

	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(25 * time.Hour)
	fixture.portal.result = succeededSearch("https://x.test/a", "https://x.test/b")
	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fixture.get(t, search.ID); got.LastNew != 1 || got.LastImported != 0 {
		t.Errorf("second run: new %d imported %d, want 1 0", got.LastNew, got.LastImported)
	}
	if len(fixture.importer.sent) != 0 {
		t.Errorf("Agent got %d messages, want none", len(fixture.importer.sent))
	}
}

// TestSavedSearchService_RunDue_FailedSearchKeepsSeenLinks: a failed run must
// not wipe what earlier runs reported, and it stays scheduled.
func TestSavedSearchService_RunDue_FailedSearchKeepsSeenLinks(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, func(in *domain.SavedSearchInput) { in.ImportNew = false })
	fixture.portal.result = succeededSearch("https://x.test/a")
	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}

	fixture.now = fixture.now.Add(25 * time.Hour)
	fixture.portal.result = domain.PortalSearch{Status: domain.TaskStatusFailed, StopReason: "Seite 1 konnte nicht geladen werden"}
	if _, err := fixture.service.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	failed := fixture.get(t, search.ID)
	if failed.LastStatus != domain.SavedSearchFailed || failed.LastError == nil || !strings.Contains(*failed.LastError, "Seite 1") {
		t.Errorf("failed run = status %s error %v, want failed with the stop reason", failed.LastStatus, failed.LastError)
	}
	if failed.NextRunAt == nil || !failed.NextRunAt.After(fixture.now) {
		t.Errorf("next_run_at = %v after a failed run, want a later run scheduled", failed.NextRunAt)
	}
	seen, err := (&SavedSearchService{pool: fixture.pool}).seenLinks(ctx, search.ID)
	if err != nil || !slices.Contains(seen, matchKey("https://x.test/a")) {
		t.Errorf("seen links after a failed run = %v, %v; want link a kept", seen, err)
	}
}

func (s *SavedSearchService) seenLinks(ctx context.Context, id uuid.UUID) ([]string, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT seen_links FROM saved_search WHERE id = $1`, id).Scan(&raw); err != nil {
		return nil, err
	}
	var seen []string
	err := json.Unmarshal(raw, &seen)
	return seen, err
}

// TestSavedSearchService_DisabledIsNeverDue and the schedule rules of Update.
func TestSavedSearchService_Update_Schedule(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, func(in *domain.SavedSearchInput) { in.Enabled = false })
	if search.NextRunAt != nil {
		t.Fatalf("disabled search has next_run_at %v", search.NextRunAt)
	}
	fixture.portal.result = succeededSearch("https://x.test/a")
	if ran, _ := fixture.service.RunDue(ctx); ran != 0 {
		t.Errorf("RunDue ran a disabled search")
	}

	enabled := true
	updated, err := fixture.service.Update(ctx, fixture.profileID, search.ID, SavedSearchPatch{Enabled: &enabled})
	if err != nil || updated.NextRunAt == nil || !updated.NextRunAt.Equal(fixture.now) {
		t.Fatalf("enable: next_run_at %v, err %v; want now", updated.NextRunAt, err)
	}
	disabled := false
	updated, err = fixture.service.Update(ctx, fixture.profileID, search.ID, SavedSearchPatch{Enabled: &disabled})
	if err != nil || updated.NextRunAt != nil {
		t.Fatalf("disable: next_run_at %v, err %v; want nil", updated.NextRunAt, err)
	}

	badInterval := 0
	if _, err = fixture.service.Update(ctx, fixture.profileID, search.ID, SavedSearchPatch{IntervalHours: &badInterval}); err == nil {
		t.Error("Update accepted interval_hours 0")
	}
	if _, err = fixture.service.Update(ctx, uuid.New(), search.ID, SavedSearchPatch{}); !isNotFound(err) {
		t.Errorf("Update through another profile: err = %v, want not found", err)
	}
}

func isNotFound(err error) bool {
	var notFound *domain.NotFoundError
	return errors.As(err, &notFound)
}

// TestSavedSearchService_RunNow_RefusesASecondRun: the running claim keeps a
// manual run from starting beside one in progress, and a run interrupted by a
// restart is settled by Recover.
func TestSavedSearchService_RunNow_RefusesASecondRun(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	search := fixture.create(t, func(in *domain.SavedSearchInput) { in.Enabled = false })
	fixture.portal.result = succeededSearch("https://x.test/a")
	fixture.portal.release = make(chan struct{})

	started, err := fixture.service.RunNow(ctx, fixture.profileID, search.ID)
	if err != nil || started.LastStatus != domain.SavedSearchRunning {
		t.Fatalf("RunNow = %+v, %v; want a running search", started, err)
	}
	var conflict *domain.ConflictError
	if _, err = fixture.service.RunNow(ctx, fixture.profileID, search.ID); !errors.As(err, &conflict) {
		t.Errorf("second RunNow: err = %v, want a ConflictError", err)
	}
	if _, err = fixture.service.RunNow(ctx, uuid.New(), search.ID); !isNotFound(err) {
		t.Errorf("RunNow through another profile: err = %v, want not found", err)
	}

	failedCount, err := fixture.service.Recover(ctx)
	if err != nil || failedCount != 1 {
		t.Fatalf("Recover = %d, %v; want 1 interrupted run", failedCount, err)
	}
	if got := fixture.get(t, search.ID); got.LastStatus != domain.SavedSearchFailed || got.LastError == nil {
		t.Errorf("after Recover: status %s error %v, want failed with a reason", got.LastStatus, got.LastError)
	}

	close(fixture.portal.release) // lets the detached run finish before the test database is dropped
	deadline := time.Now().Add(5 * time.Second)
	for fixture.get(t, search.ID).LastStatus == domain.SavedSearchFailed && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

// TestAgentService_SendToActiveSession_DoesNothingWhenItCannot: a scheduled
// import must not start a second turn beside a running one, and a profile
// without an active session is not an error.
func TestAgentService_SendToActiveSession_DoesNothingWhenItCannot(t *testing.T) {
	fixture := newSavedSearchFixture(t)
	ctx := context.Background()
	agentService := NewAgentService(fixture.pool, nil, nil, t.TempDir(), nil)

	started, err := agentService.SendToActiveSession(ctx, fixture.profileID, "hi")
	if err != nil || started {
		t.Fatalf("no active session: started %t, err %v; want false, nil", started, err)
	}

	sessionID := uuid.New()
	if _, err = fixture.pool.Exec(ctx, `INSERT INTO chat_session (id, profile_id) VALUES ($1, $2)`, sessionID, fixture.profileID); err != nil {
		t.Fatal(err)
	}
	agentService.registerTurn(sessionID, func() {})
	started, err = agentService.SendToActiveSession(ctx, fixture.profileID, "hi")
	if err != nil || started {
		t.Fatalf("turn running: started %t, err %v; want false, nil", started, err)
	}
	var messages int
	if err = fixture.pool.QueryRow(ctx, `SELECT count(*) FROM chat_message WHERE session_id = $1`, sessionID).Scan(&messages); err != nil || messages != 0 {
		t.Errorf("messages written while a turn ran = %d, %v; want 0", messages, err)
	}
}
