package service

import (
	"context"
	"encoding/json"
	"errors"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/db/repository"
	"propertyfinder/internal/domain"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests cover the proposed/active criterion workflow (docs/adr/0024,
// issue #29) against a real Postgres (dbtest skips them without
// TEST_DATABASE_URL): the status transitions, the name uniqueness rule, and
// the guarantee that a proposed criterion never reaches evaluation or scoring.

func seedProfile(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO search_profile (id, name) VALUES ($1, $2)`, id, "P-"+id.String()); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return id
}

func seedListing(t *testing.T, pool *pgxpool.Pool, profileID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO listing (id, profile_id, title) VALUES ($1, $2, 'Haus')`, id, profileID); err != nil {
		t.Fatalf("seed listing: %v", err)
	}
	return id
}

func needsReevaluation(t *testing.T, pool *pgxpool.Pool, listingID uuid.UUID) bool {
	t.Helper()
	var flag bool
	if err := pool.QueryRow(context.Background(), `SELECT needs_reevaluation FROM listing WHERE id = $1`, listingID).Scan(&flag); err != nil {
		t.Fatal(err)
	}
	return flag
}

func TestCriterionService_Create_IsActive_Propose_IsProposed(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewCriterionService(pool)
	profileID := seedProfile(t, pool)

	active, err := service.Create(context.Background(), profileID, "Preis", "", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := service.Propose(context.Background(), profileID, "Lage", "", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != domain.CriterionStatusActive || proposed.Status != domain.CriterionStatusProposed {
		t.Errorf("statuses = %q / %q, want active / proposed", active.Status, proposed.Status)
	}
	if proposed.SortOrder <= active.SortOrder {
		t.Errorf("proposed sort_order %d must come after active %d, so it never shifts active ones", proposed.SortOrder, active.SortOrder)
	}

	all, _ := service.List(context.Background(), profileID)
	onlyActive, _ := repository.NewCriterionRepo(pool).ListActiveByProfile(context.Background(), profileID)
	if len(all) != 2 || len(onlyActive) != 1 || onlyActive[0].ID != active.ID {
		t.Errorf("List = %d criteria, ListActive = %+v, want 2 and just the active one", len(all), onlyActive)
	}
}

// TestCriterionService_ProposeAndApprove_ReevaluationFlag: a proposal must
// not touch listings (it is ignored by evaluation); approving it is what
// changes the evaluation basis, so only then are listings flagged.
func TestCriterionService_ProposeAndApprove_ReevaluationFlag(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewCriterionService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	listingID := seedListing(t, pool, profileID)

	proposed, err := service.Propose(ctx, profileID, "Lage", "", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if needsReevaluation(t, pool, listingID) {
		t.Error("Propose flagged listings for re-evaluation, want no side effect")
	}
	newName := "Lage am See"
	if _, updateErr := service.Update(ctx, proposed.ID, &newName, nil, nil, nil); updateErr != nil {
		t.Fatal(updateErr)
	}
	if needsReevaluation(t, pool, listingID) {
		t.Error("editing a proposed criterion flagged listings for re-evaluation")
	}

	approved, err := service.Approve(ctx, proposed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.CriterionStatusActive {
		t.Errorf("approved status = %q, want active", approved.Status)
	}
	if !needsReevaluation(t, pool, listingID) {
		t.Error("Approve did not flag listings for re-evaluation")
	}

	var conflict *domain.ConflictError
	if _, err := service.Approve(ctx, proposed.ID); !errors.As(err, &conflict) {
		t.Errorf("second Approve error = %v, want ConflictError", err)
	}
}

func TestCriterionService_DeleteProposed_DoesNotFlagListings(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewCriterionService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	listingID := seedListing(t, pool, profileID)

	proposed, _ := service.Propose(ctx, profileID, "Lage", "", 2, true)
	if err := service.Delete(ctx, proposed.ID); err != nil {
		t.Fatal(err)
	}
	if needsReevaluation(t, pool, listingID) {
		t.Error("deleting a proposed criterion flagged listings for re-evaluation")
	}
}

// TestCriterionService_NamesAreUnique: a proposal may carry neither the name
// of an active nor of another proposed criterion (case-insensitive); the
// plain form and renames obey the same rule (docs/adr/0024).
func TestCriterionService_NamesAreUnique(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewCriterionService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)

	if _, err := service.Create(ctx, profileID, "Preis", "", 1, false); err != nil {
		t.Fatal(err)
	}
	proposed, err := service.Propose(ctx, profileID, "Lage", "", 1, true)
	if err != nil {
		t.Fatal(err)
	}

	var conflict *domain.ConflictError
	attempts := map[string]func() error{
		"propose vs active":   func() error { _, e := service.Propose(ctx, profileID, " preis ", "", 1, false); return e },
		"propose vs proposed": func() error { _, e := service.Propose(ctx, profileID, "LAGE", "", 1, false); return e },
		"create vs proposed":  func() error { _, e := service.Create(ctx, profileID, "lage", "", 1, false); return e },
		"rename vs active": func() error {
			name := "PREIS"
			_, e := service.Update(ctx, proposed.ID, &name, nil, nil, nil)
			return e
		},
	}
	for label, attempt := range attempts {
		if err := attempt(); !errors.As(err, &conflict) {
			t.Errorf("%s: error = %v, want ConflictError", label, err)
		}
	}

	// Another profile may reuse the names, and keeping a name on update is fine.
	if _, err := service.Propose(ctx, seedProfile(t, pool), "Preis", "", 1, false); err != nil {
		t.Errorf("same name in another profile: %v", err)
	}
	same := "Lage"
	if _, err := service.Update(ctx, proposed.ID, &same, nil, nil, nil); err != nil {
		t.Errorf("update keeping its own name: %v", err)
	}
}

// TestCriterionService_UpdateProposed_RefusesActive is the MCP guarantee:
// an external client cannot change an active criterion, and nothing is
// partially applied.
func TestCriterionService_UpdateProposed_RefusesActive(t *testing.T) {
	pool := dbtest.NewPool(t)
	service := NewCriterionService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)

	active, _ := service.Create(ctx, profileID, "Preis", "alt", 3, false)
	proposed, _ := service.Propose(ctx, profileID, "Lage", "alt", 3, true)

	newDescription, newWeight := "neu", 5
	var conflict *domain.ConflictError
	if _, err := service.UpdateProposed(ctx, active.ID, nil, &newDescription, &newWeight, nil); !errors.As(err, &conflict) {
		t.Fatalf("UpdateProposed on active: error = %v, want ConflictError", err)
	}
	got, _ := repository.NewCriterionRepo(pool).GetByID(ctx, active.ID)
	if got.Description != "alt" || got.Weight != 3 {
		t.Errorf("active criterion was modified: %+v", got)
	}

	updated, err := service.UpdateProposed(ctx, proposed.ID, nil, &newDescription, &newWeight, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "neu" || updated.Weight != 5 || updated.Status != domain.CriterionStatusProposed {
		t.Errorf("updated proposal = %+v, want new text/weight and still proposed", updated)
	}
}

// capturingProvider records the criteria EvaluateListing is asked to grade.
type capturingProvider struct {
	ai.Provider
	criteria []domain.Criterion
}

func (p *capturingProvider) EvaluateListing(_ context.Context, _ domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	p.criteria = criteria
	scores := make([]domain.CriterionScore, len(criteria))
	for i, criterion := range criteria {
		scores[i] = domain.CriterionScore{Index: i, CriterionID: criterion.ID, Grade: domain.GradeA, Reasoning: "ok"}
	}
	return scores, nil
}

// TestEvaluationService_IgnoresProposedCriteria: grades, the criteria sent to
// the model and the overall score all come from active criteria only; a
// profile with only proposals cannot be evaluated at all.
func TestEvaluationService_IgnoresProposedCriteria(t *testing.T) {
	pool := dbtest.NewPool(t)
	criteria := NewCriterionService(pool)
	provider := &capturingProvider{}
	evaluation := NewEvaluationService(pool, provider, nil)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	listingID := seedListing(t, pool, profileID)

	if _, err := criteria.Propose(ctx, profileID, "Lage", "", 5, true); err != nil {
		t.Fatal(err)
	}
	var validation *domain.ValidationError
	if _, err := evaluation.EvaluateListing(ctx, listingID); !errors.As(err, &validation) {
		t.Fatalf("evaluating with only proposed criteria: error = %v, want ValidationError", err)
	}

	active, _ := criteria.Create(ctx, profileID, "Preis", "", 1, false)
	if _, err := evaluation.EvaluateListing(ctx, listingID); err != nil {
		t.Fatal(err)
	}
	if len(provider.criteria) != 1 || provider.criteria[0].ID != active.ID {
		t.Errorf("model was asked to grade %+v, want only the active criterion", provider.criteria)
	}
}

// TestRecomputeScores_IgnoresProposedCriteria: even if a grade row exists for
// a criterion, the score is weighted over active criteria only — so a
// criterion's weight cannot leak into scores before approval.
func TestRecomputeScores_IgnoresProposedCriteria(t *testing.T) {
	pool := dbtest.NewPool(t)
	criteria := NewCriterionService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	listingID := seedListing(t, pool, profileID)

	active, _ := criteria.Create(ctx, profileID, "Preis", "", 1, false)  // graded A = 100
	proposed, _ := criteria.Propose(ctx, profileID, "Lage", "", 5, true) // graded F = 0, must not count
	for criterionID, grade := range map[uuid.UUID]string{active.ID: "A", proposed.ID: "F"} {
		if _, err := pool.Exec(ctx, `INSERT INTO listing_criterion_grade (listing_id, criterion_id, grade, reasoning) VALUES ($1, $2, $3, '')`, listingID, criterionID, grade); err != nil {
			t.Fatal(err)
		}
	}

	if err := repository.NewListingRepo(pool).RecomputeScores(ctx, profileID); err != nil {
		t.Fatal(err)
	}
	var score float64
	if err := pool.QueryRow(ctx, `SELECT overall_score FROM listing WHERE id = $1`, listingID).Scan(&score); err != nil {
		t.Fatal(err)
	}
	if score != 100 {
		t.Errorf("score = %v, want 100 (the proposed criterion must be ignored)", score)
	}
}

// storeProposal persists an assistant message carrying one propose_criteria
// call, the way AgentService does after the tool ran.
func storeProposal(t *testing.T, pool *pgxpool.Pool, profileID uuid.UUID, input string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	chatRepo := repository.NewChatRepo(pool)
	sessionID := uuid.New()
	if err := chatRepo.CreateSession(ctx, sessionID, profileID); err != nil {
		t.Fatal(err)
	}
	calls, _ := json.Marshal([]agent.StoredToolCall{{ID: "toolu_1", Name: "propose_criteria", Input: json.RawMessage(input)}})
	message := &domain.ChatMessage{ID: uuid.New(), SessionID: sessionID, Role: domain.ChatRoleAssistant, Content: "", ToolCalls: calls}
	if err := chatRepo.CreateMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	return message.ID
}

// TestToolCallService_ConfirmApprovesStoredProposal: the chat flow end to
// end — the tool stores a proposed row, Confirm activates that same row (no
// second one), Reject deletes it.
func TestToolCallService_ConfirmApprovesStoredProposal(t *testing.T) {
	pool := dbtest.NewPool(t)
	criteria := NewCriterionService(pool)
	toolCalls := NewToolCallService(pool)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	input := `{"action":"add","name":"Lage","description":"ruhig","weight":2}`

	proposed, err := criteria.Propose(ctx, profileID, "Lage", "ruhig", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	messageID := storeProposal(t, pool, profileID, input)

	result, err := toolCalls.Confirm(ctx, messageID, 0)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, ok := result.(*domain.Criterion)
	if !ok || confirmed.ID != proposed.ID || confirmed.Status != domain.CriterionStatusActive {
		t.Fatalf("Confirm returned %+v, want the stored proposal now active", result)
	}
	if all, _ := criteria.List(ctx, profileID); len(all) != 1 {
		t.Errorf("profile has %d criteria after Confirm, want 1 (no duplicate)", len(all))
	}

	// Reject removes a still-proposed row, but never an active one.
	rejectedProfile := seedProfile(t, pool)
	if _, err := criteria.Propose(ctx, rejectedProfile, "Lage", "ruhig", 2, true); err != nil {
		t.Fatal(err)
	}
	if err := toolCalls.Reject(ctx, storeProposal(t, pool, rejectedProfile, input), 0); err != nil {
		t.Fatal(err)
	}
	if all, _ := criteria.List(ctx, rejectedProfile); len(all) != 0 {
		t.Errorf("proposal still exists after Reject: %+v", all)
	}
	if err := toolCalls.Reject(ctx, messageID, 0); err == nil {
		t.Error("second action on an already confirmed call must fail")
	}
}

// TestToolCallService_ConfirmWithoutStoredRowCreatesActive: proposals from the
// rejection analysis live only in chat_message.tool_calls (no row was ever
// written), so Confirm still has to create the criterion.
func TestToolCallService_ConfirmWithoutStoredRowCreatesActive(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profileID := seedProfile(t, pool)
	messageID := storeProposal(t, pool, profileID, `{"action":"add","name":"Ruhe","description":"","weight":1}`)

	result, err := NewToolCallService(pool).Confirm(ctx, messageID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if created, ok := result.(*domain.Criterion); !ok || created.Status != domain.CriterionStatusActive {
		t.Errorf("Confirm returned %+v, want a new active criterion", result)
	}
}
