package service

import (
	"context"
	"fmt"
	"log"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	savedSearchMaxNameLen  = 100
	savedSearchMaxInterval = 720
	savedSearchMaxImport   = 10
	// savedSearchMaxSeen bounds the remembered links of one saved search.
	// The oldest are dropped first.
	savedSearchMaxSeen = 5000
	// SavedSearchTick is how often the scheduler looks for due searches.
	SavedSearchTick = time.Minute
)

// portalRunner is the slice of *PortalSearchService a scheduled run needs.
type portalRunner interface {
	Run(ctx context.Context, profileID uuid.UUID, request domain.PortalSearchRequest) (*domain.PortalSearch, error)
}

// chatImporter hands a message to the Agent in a profile's active chat
// session. It reports false, with no error, when it cannot right now: the
// profile has no active session or the Agent is in the middle of a turn.
type chatImporter interface {
	SendToActiveSession(ctx context.Context, profileID uuid.UUID, content string) (bool, error)
}

// SavedSearchService keeps saved portal searches and runs them on a schedule
// (spec 01-17, docs/adr/0029). A run is a portal search (ADR 0028) followed,
// when the search asks for it, by one Agent message that imports the new
// links. The scheduler is a single goroutine, so runs never overlap; the
// `last_status = running` claim in the database also keeps a manual run and
// a scheduled run from overlapping.
type SavedSearchService struct {
	pool     *pgxpool.Pool
	portal   portalRunner
	importer chatImporter
	now      func() time.Time
}

// NewSavedSearchService constructs the service. portal runs the searches and
// importer hands the new links to the Agent.
func NewSavedSearchService(pool *pgxpool.Pool, portal portalRunner, importer chatImporter) *SavedSearchService {
	return &SavedSearchService{pool: pool, portal: portal, importer: importer, now: func() time.Time { return time.Now().UTC() }}
}

// SavedSearchPatch is a partial update; nil fields stay as they are.
type SavedSearchPatch struct {
	Name          *string
	URL           *string
	PageParam     *string
	MaxPages      *int
	LinkContains  *string
	IntervalHours *int
	Enabled       *bool
	ImportNew     *bool
	MaxImport     *int
}

// validateSavedSearchInput trims and checks input. The request fields follow
// the rules of the search_portal Tool (newPortalSearchPayload).
func validateSavedSearchInput(input domain.SavedSearchInput) (domain.SavedSearchInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > savedSearchMaxNameLen {
		return input, &domain.ValidationError{Field: "name", Message: fmt.Sprintf("name ist erforderlich (höchstens %d Zeichen)", savedSearchMaxNameLen)}
	}
	payload, err := newPortalSearchPayload(domain.PortalSearchRequest{
		URL: input.URL, PageParam: input.PageParam, MaxPages: input.MaxPages, LinkContains: input.LinkContains,
	})
	if err != nil {
		return input, err
	}
	input.URL, input.PageParam, input.MaxPages, input.LinkContains = payload.URL, payload.PageParam, payload.MaxPages, payload.LinkContains
	if input.IntervalHours < 1 || input.IntervalHours > savedSearchMaxInterval {
		return input, &domain.ValidationError{Field: "interval_hours", Message: fmt.Sprintf("interval_hours muss zwischen 1 und %d liegen", savedSearchMaxInterval)}
	}
	if input.MaxImport < 1 || input.MaxImport > savedSearchMaxImport {
		return input, &domain.ValidationError{Field: "max_import", Message: fmt.Sprintf("max_import muss zwischen 1 und %d liegen", savedSearchMaxImport)}
	}
	return input, nil
}

// List returns the saved searches of a profile.
func (s *SavedSearchService) List(ctx context.Context, profileID uuid.UUID) ([]domain.SavedSearch, error) {
	if _, err := repository.NewProfileRepo(s.pool).GetByID(ctx, profileID); err != nil {
		return nil, err
	}
	return repository.NewSavedSearchRepo(s.pool).List(ctx, profileID)
}

// get returns a saved search of profileID; another profile's is not found.
func (s *SavedSearchService) get(ctx context.Context, profileID, id uuid.UUID) (*domain.SavedSearch, error) {
	search, err := repository.NewSavedSearchRepo(s.pool).GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if search.ProfileID != profileID {
		return nil, &domain.NotFoundError{Entity: "saved_search", ID: id.String()}
	}
	return search, nil
}

// Create saves a search. An enabled search first runs at the next scheduler
// tick.
func (s *SavedSearchService) Create(ctx context.Context, profileID uuid.UUID, input domain.SavedSearchInput) (*domain.SavedSearch, error) {
	input, err := validateSavedSearchInput(input)
	if err != nil {
		return nil, err
	}
	if _, profileError := repository.NewProfileRepo(s.pool).GetByID(ctx, profileID); profileError != nil {
		return nil, profileError
	}

	now := s.now()
	search := &domain.SavedSearch{
		ID: uuid.New(), ProfileID: profileID, Name: input.Name, URL: input.URL, PageParam: input.PageParam,
		MaxPages: input.MaxPages, LinkContains: input.LinkContains, IntervalHours: input.IntervalHours,
		Enabled: input.Enabled, ImportNew: input.ImportNew, MaxImport: input.MaxImport,
		LastStatus: domain.SavedSearchIdle, CreatedAt: now, UpdatedAt: now,
	}
	if search.Enabled {
		search.NextRunAt = &now
	}
	repo := repository.NewSavedSearchRepo(s.pool)
	if createError := repo.Create(ctx, search); createError != nil {
		return nil, createError
	}
	return repo.GetByID(ctx, search.ID)
}

// Update applies patch. Enabling a disabled search schedules it for the next
// tick; disabling clears the schedule; changing the interval of an enabled
// search takes effect after its next run.
func (s *SavedSearchService) Update(ctx context.Context, profileID, id uuid.UUID, patch SavedSearchPatch) (*domain.SavedSearch, error) {
	search, err := s.get(ctx, profileID, id)
	if err != nil {
		return nil, err
	}
	input := domain.SavedSearchInput{
		Name: search.Name, URL: search.URL, PageParam: search.PageParam, MaxPages: search.MaxPages,
		LinkContains: search.LinkContains, IntervalHours: search.IntervalHours, Enabled: search.Enabled,
		ImportNew: search.ImportNew, MaxImport: search.MaxImport,
	}
	if patch.Name != nil {
		input.Name = *patch.Name
	}
	if patch.URL != nil {
		input.URL = *patch.URL
	}
	if patch.PageParam != nil {
		input.PageParam = *patch.PageParam
	}
	if patch.MaxPages != nil {
		input.MaxPages = *patch.MaxPages
	}
	if patch.LinkContains != nil {
		input.LinkContains = *patch.LinkContains
	}
	if patch.IntervalHours != nil {
		input.IntervalHours = *patch.IntervalHours
	}
	if patch.Enabled != nil {
		input.Enabled = *patch.Enabled
	}
	if patch.ImportNew != nil {
		input.ImportNew = *patch.ImportNew
	}
	if patch.MaxImport != nil {
		input.MaxImport = *patch.MaxImport
	}
	input, err = validateSavedSearchInput(input)
	if err != nil {
		return nil, err
	}

	now := s.now()
	wasEnabled := search.Enabled
	search.Name, search.URL, search.PageParam, search.MaxPages = input.Name, input.URL, input.PageParam, input.MaxPages
	search.LinkContains, search.IntervalHours, search.Enabled = input.LinkContains, input.IntervalHours, input.Enabled
	search.ImportNew, search.MaxImport, search.UpdatedAt = input.ImportNew, input.MaxImport, now
	switch {
	case !search.Enabled:
		search.NextRunAt = nil
	case !wasEnabled || search.NextRunAt == nil:
		search.NextRunAt = &now
	}
	repo := repository.NewSavedSearchRepo(s.pool)
	if updateError := repo.Update(ctx, search); updateError != nil {
		return nil, updateError
	}
	return repo.GetByID(ctx, id)
}

// Delete removes a saved search. The Listings it imported stay.
func (s *SavedSearchService) Delete(ctx context.Context, profileID, id uuid.UUID) error {
	if _, err := s.get(ctx, profileID, id); err != nil {
		return err
	}
	return repository.NewSavedSearchRepo(s.pool).Delete(ctx, id)
}

// RunNow starts a run of a saved search right away, disabled or not, and
// returns the search marked running. The run itself is detached from ctx
// (ADR 0008 shape): it must outlive the HTTP request. A search that is
// already running is a ConflictError.
//
//nolint:contextcheck // non-inherited context: the run must outlive the request, see comment above.
func (s *SavedSearchService) RunNow(ctx context.Context, profileID, id uuid.UUID) (*domain.SavedSearch, error) {
	search, err := s.get(ctx, profileID, id)
	if err != nil {
		return nil, err
	}
	repo := repository.NewSavedSearchRepo(s.pool)
	claimed, err := repo.Claim(ctx, id, s.now(), true)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, &domain.ConflictError{Message: "Die Suche läuft bereits"}
	}
	go s.execute(context.Background(), *search) //nolint:gosec // G118: detached on purpose, see comment above.
	return repo.GetByID(ctx, id)
}

// Recover marks the runs the last process left running as failed. They are
// not continued: their portal search task is (PortalSearchService.Recover),
// but nobody is left to hand its links to the Agent. The next scheduled run
// repeats the search. Called once at boot, after the migrations.
func (s *SavedSearchService) Recover(ctx context.Context) (int64, error) {
	return repository.NewSavedSearchRepo(s.pool).FailRunning(ctx, "Lauf durch Neustart unterbrochen")
}

// RunScheduler runs due searches every SavedSearchTick until ctx ends. It
// blocks; main starts it in a goroutine.
func (s *SavedSearchService) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(SavedSearchTick)
	defer ticker.Stop()
	for {
		if _, err := s.RunDue(ctx); err != nil && ctx.Err() == nil {
			log.Printf("saved search scheduler: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunDue runs, one after another, every saved search that is due, and
// returns how many it ran. A run in progress when ctx ends stays `running`.
func (s *SavedSearchService) RunDue(ctx context.Context) (int, error) {
	repo := repository.NewSavedSearchRepo(s.pool)
	due, err := repo.ListDue(ctx, s.now())
	if err != nil {
		return 0, err
	}
	ran := 0
	for _, search := range due {
		select {
		case <-ctx.Done():
			return ran, nil
		default:
		}
		claimed, claimError := repo.Claim(ctx, search.ID, s.now(), false)
		if claimError != nil {
			return ran, claimError
		}
		if !claimed {
			continue
		}
		s.execute(ctx, search)
		ran++
	}
	return ran, nil
}

// execute runs the portal search of a claimed saved search and writes the
// outcome. If ctx ends in the middle the search stays running: Recover
// settles it at the next boot.
func (s *SavedSearchService) execute(ctx context.Context, search domain.SavedSearch) {
	repo := repository.NewSavedSearchRepo(s.pool)
	outcome := s.run(ctx, repo, search)
	if ctx.Err() != nil {
		return
	}
	if err := repo.Finish(ctx, search.ID, outcome); err != nil {
		log.Printf("saved search %s: write outcome: %v", search.ID, err)
	}
}

func (s *SavedSearchService) run(ctx context.Context, repo *repository.SavedSearchRepo, search domain.SavedSearch) domain.SavedSearchOutcome {
	failed := func(message string) domain.SavedSearchOutcome {
		return domain.SavedSearchOutcome{Status: domain.SavedSearchFailed, Summary: "Suche fehlgeschlagen", Error: &message}
	}

	result, err := s.portal.Run(ctx, search.ProfileID, domain.PortalSearchRequest{
		URL: search.URL, PageParam: search.PageParam, MaxPages: search.MaxPages, LinkContains: search.LinkContains,
	})
	if err != nil {
		return failed(err.Error())
	}
	taskID := result.TaskID
	if result.Status != domain.TaskStatusSucceeded {
		message := result.StopReason
		if result.Error != nil {
			message = *result.Error
		}
		outcome := failed(message)
		outcome.TaskID = &taskID
		return outcome
	}

	seenBefore, err := repo.SeenLinks(ctx, search.ID)
	if err != nil {
		return failed(err.Error())
	}
	fresh := freshLinks(result.NewLinks, seenBefore)

	outcome := domain.SavedSearchOutcome{
		Status: domain.SavedSearchSucceeded, TaskID: &taskID,
		Found: len(result.NewLinks) + len(result.KnownLinks), New: len(fresh),
	}
	reported := fresh
	var note string
	if search.ImportNew && len(fresh) > 0 {
		batch := fresh[:min(len(fresh), search.MaxImport)]
		started, sendError := s.importer.SendToActiveSession(ctx, search.ProfileID, savedSearchImportMessage(search, batch, len(fresh)))
		switch {
		case sendError != nil:
			log.Printf("saved search %s: hand links to the Agent: %v", search.ID, sendError)
			note = "Import fehlgeschlagen: " + sendError.Error()
			reported = nil
		case !started:
			note = "Import übersprungen: Chat ist gerade belegt – die Links bleiben für den nächsten Lauf"
			reported = nil
		default:
			outcome.Imported = len(batch)
			reported = batch
			note = fmt.Sprintf("%d an den Agent übergeben", len(batch))
		}
	}
	outcome.SeenLinks = rememberLinks(seenBefore, reported)
	outcome.Summary = fmt.Sprintf("%d Treffer, %d neu", outcome.Found, outcome.New)
	if note != "" {
		outcome.Summary += ", " + note
	}
	if result.StopReason != "" {
		outcome.Summary += " (" + result.StopReason + ")"
	}
	return outcome
}

// freshLinks returns the links of candidates that are not in seen (a list of
// matchKeys), in order.
func freshLinks(candidates, seen []string) []string {
	seenSet := make(map[string]bool, len(seen))
	for _, key := range seen {
		seenSet[key] = true
	}
	fresh := make([]string, 0, len(candidates))
	for _, link := range candidates {
		if !seenSet[matchKey(link)] {
			fresh = append(fresh, link)
		}
	}
	return fresh
}

// rememberLinks appends the matchKeys of links to seen and keeps the newest
// savedSearchMaxSeen.
func rememberLinks(seen, links []string) []string {
	remembered := append(make([]string, 0, len(seen)+len(links)), seen...)
	for _, link := range links {
		remembered = append(remembered, matchKey(link))
	}
	if len(remembered) > savedSearchMaxSeen {
		remembered = remembered[len(remembered)-savedSearchMaxSeen:]
	}
	return remembered
}

// savedSearchImportMessage is the user message a run hands to the Agent
// (German, like the rest of the Agent's text). It says the user agreed to
// the import in advance, because the preamble otherwise tells the Agent to
// ask before loading Listings (spec 00-04).
func savedSearchImportMessage(search domain.SavedSearch, batch []string, totalNew int) string {
	var message strings.Builder
	fmt.Fprintf(&message, "[Geplante Suche] Die gespeicherte Suche „%s“ (%s) hat %d neue Links gefunden, die noch nicht im Profil sind. ", search.Name, search.URL, totalNew)
	message.WriteString("Der Nutzer hat den automatischen Import dieser Suche im Voraus freigegeben: Frage nicht nach. ")
	fmt.Fprintf(&message, "Lade diese %d Links: rufe fetch_url für alle in derselben Runde auf, speichere jedes Inserat mit save_listing und bewerte es danach mit evaluate_listing. Rufe search_portal nicht erneut auf. ", len(batch))
	if totalNew > len(batch) {
		fmt.Fprintf(&message, "Weitere neue Links (%d) folgen beim nächsten Lauf. ", totalNew-len(batch))
	}
	message.WriteString("Fasse am Ende kurz zusammen, was angelegt wurde und was nicht.\n")
	for _, link := range batch {
		message.WriteString("\n- " + link)
	}
	return message.String()
}
