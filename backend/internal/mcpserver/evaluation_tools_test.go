package mcpserver

import (
	"context"
	"encoding/json"
	"math"
	"propertyfinder/internal/db/dbtest"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/service"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// evaluationFixture is a profile with two active Criteria (weights 3 and 1),
// one proposed Criterion and one stale Listing.
type evaluationFixture struct {
	pool      *pgxpool.Pool
	session   *mcp.ClientSession
	profileID uuid.UUID
	listingID uuid.UUID
	price     domain.Criterion // active, weight 3
	location  domain.Criterion // active, weight 1
	proposed  domain.Criterion
}

func newEvaluationFixture(t *testing.T) evaluationFixture {
	t.Helper()
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profile, err := service.NewProfileService(pool).Create(ctx, "P", "")
	if err != nil {
		t.Fatal(err)
	}
	criteria := service.NewCriterionService(pool)
	price, err := criteria.Create(ctx, profile.ID, "Preis", "günstig", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	location, err := criteria.Create(ctx, profile.ID, "Lage", "ruhig", 1, true)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := criteria.Propose(ctx, profile.ID, "Garten", "groß", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	listingID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO listing (id, profile_id, title, document_md, user_confirmed_fields, needs_reevaluation)
VALUES ($1, $2, 'Haus am See', '# Dokument', ARRAY['title'], true)`, listingID, profile.ID); err != nil {
		t.Fatal(err)
	}
	return evaluationFixture{
		pool: pool, session: connectMCP(t, pool), profileID: profile.ID, listingID: listingID,
		price: *price, location: *location, proposed: *proposed,
	}
}

// call invokes an MCP tool and returns whether it was a tool error, the
// error text, and the structured result as a generic map.
func (f evaluationFixture) call(t *testing.T, name string, args map[string]any) (isError bool, text string, out map[string]any) {
	t.Helper()
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if result.IsError {
		for _, content := range result.Content {
			if textContent, ok := content.(*mcp.TextContent); ok {
				text += textContent.Text
			}
		}
		return true, text, nil
	}
	raw, _ := json.Marshal(result.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s result: %v", name, err)
	}
	return false, "", out
}

func (f evaluationFixture) storedGrades(t *testing.T) map[uuid.UUID]domain.ListingGrade {
	t.Helper()
	grades, err := service.NewEvaluationService(f.pool, nil, nil).GetGrades(context.Background(), f.listingID)
	if err != nil {
		t.Fatal(err)
	}
	byCriterion := make(map[uuid.UUID]domain.ListingGrade, len(grades))
	for _, grade := range grades {
		byCriterion[grade.CriterionID] = grade
	}
	return byCriterion
}

func grade(criterion domain.Criterion, letter, reasoning string) map[string]any {
	return map[string]any{"criterion_id": criterion.ID.String(), "grade": letter, "reasoning": reasoning}
}

// TestGetEvaluationBrief_MCP_ListsOnlyActiveCriteria: a proposed Criterion
// is ignored by evaluation (ADR 0024), so the brief must not offer it for
// grading; the rest of the brief carries the listing data and by-ID format.
func TestGetEvaluationBrief_MCP_ListsOnlyActiveCriteria(t *testing.T) {
	f := newEvaluationFixture(t)

	isError, text, brief := f.call(t, "get_evaluation_brief", map[string]any{"listing_id": f.listingID.String()})
	if isError {
		t.Fatalf("brief failed: %s", text)
	}
	criteria, _ := brief["criteria"].([]any)
	if len(criteria) != 2 {
		t.Fatalf("brief has %d criteria, want 2 (active only): %v", len(criteria), criteria)
	}
	for i, want := range []domain.Criterion{f.price, f.location} {
		got, _ := criteria[i].(map[string]any)
		if got["id"] != want.ID.String() || got["name"] != want.Name || got["weight"] != float64(want.Weight) || got["is_subjective"] != want.IsSubjective {
			t.Errorf("criteria[%d] = %v, want %s %q weight %d", i, got, want.ID, want.Name, want.Weight)
		}
	}
	if strings.Contains(brief["instruction"].(string), "SAME ORDER") || !strings.Contains(brief["instruction"].(string), "criterion_id") {
		t.Errorf("instruction is not the by-criterion_id format: %q", brief["instruction"])
	}
	listing, _ := brief["listing"].(map[string]any)
	if listing["document_md"] != "# Dokument" || !strings.Contains(listing["text"].(string), "Haus am See") {
		t.Errorf("listing = %v, want rendered text and document_md", listing)
	}
	if fields, _ := brief["user_confirmed_fields"].([]any); len(fields) != 1 || fields[0] != "title" {
		t.Errorf("user_confirmed_fields = %v, want [title]", brief["user_confirmed_fields"])
	}
	for _, key := range []string{"global_context", "profile_context", "response_format"} {
		if _, ok := brief[key]; !ok {
			t.Errorf("brief lacks %q", key)
		}
	}
}

// TestGetEvaluationBrief_MCP_NoActiveCriteriaIsAnError: same ValidationError
// as the internal EvaluateListing.
func TestGetEvaluationBrief_MCP_NoActiveCriteriaIsAnError(t *testing.T) {
	pool := dbtest.NewPool(t)
	ctx := context.Background()
	profile, err := service.NewProfileService(pool).Create(ctx, "P", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewCriterionService(pool).Propose(ctx, profile.ID, "Preis", "", 1, false); err != nil {
		t.Fatal(err)
	}
	listingID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO listing (id, profile_id, title) VALUES ($1, $2, 'Haus')`, listingID, profile.ID); err != nil {
		t.Fatal(err)
	}
	f := evaluationFixture{pool: pool, session: connectMCP(t, pool), listingID: listingID}

	isError, text, _ := f.call(t, "get_evaluation_brief", map[string]any{"listing_id": listingID.String()})
	if !isError || !strings.Contains(text, "keine aktiven Kriterien") {
		t.Errorf("isError=%v text=%q, want the no-active-criteria validation error", isError, text)
	}
}

// TestSubmitEvaluation_MCP_RejectsInvalidSubmissions: every kind of bad
// submission is a tool error naming the problem, and the stored grades stay
// exactly as they were.
func TestSubmitEvaluation_MCP_RejectsInvalidSubmissions(t *testing.T) {
	f := newEvaluationFixture(t)
	unknown := uuid.New().String()

	// An earlier valid evaluation that a rejected submission must not touch.
	if isError, text, _ := f.call(t, "submit_evaluation", map[string]any{
		"listing_id": f.listingID.String(),
		"grades":     []any{grade(f.price, "B", "ok"), grade(f.location, "D", "laut")},
	}); isError {
		t.Fatalf("baseline submit failed: %s", text)
	}
	before := f.storedGrades(t)

	cases := []struct {
		name    string
		grades  []any
		wantIn  string // must appear in the error: the offending ID
		wantMsg string
	}{
		{"missing", []any{grade(f.price, "A", "gut")}, f.location.ID.String(), "fehlende"},
		{"proposed criterion counts as unknown", []any{grade(f.price, "A", "gut"), grade(f.location, "A", "gut"), grade(f.proposed, "A", "gut")}, f.proposed.ID.String(), "unbekannte"},
		{"duplicate", []any{grade(f.price, "A", "gut"), grade(f.price, "B", "gut"), grade(f.location, "A", "gut")}, f.price.ID.String(), "doppelte"},
		{"unknown", []any{grade(f.price, "A", "gut"), grade(f.location, "A", "gut"), map[string]any{"criterion_id": unknown, "grade": "A", "reasoning": "x"}}, unknown, "unbekannte"},
		{"bad grade", []any{grade(f.price, "G", "gut"), grade(f.location, "A", "gut")}, f.price.ID.String(), "ungültige Note"},
		{"empty reasoning", []any{grade(f.price, "A", "  \t"), grade(f.location, "A", "gut")}, f.price.ID.String(), "reasoning"},
	}
	for _, c := range cases {
		isError, text, _ := f.call(t, "submit_evaluation", map[string]any{"listing_id": f.listingID.String(), "grades": c.grades})
		if !isError || !strings.Contains(text, c.wantIn) || !strings.Contains(text, c.wantMsg) {
			t.Errorf("%s: isError=%v text=%q, want an error containing %q and %q", c.name, isError, text, c.wantIn, c.wantMsg)
		}
		after := f.storedGrades(t)
		if len(after) != len(before) {
			t.Fatalf("%s: %d stored grades, want %d", c.name, len(after), len(before))
		}
		for id, want := range before {
			if got := after[id]; got.Grade != want.Grade || got.Reasoning != want.Reasoning {
				t.Errorf("%s: grade for %s changed to %+v, want %+v", c.name, id, got, want)
			}
		}
	}
}

// TestSubmitEvaluation_MCP_StoresGradesAndServerComputesScore also covers
// list_pending_evaluations: the stale listing is listed before the submit
// and gone after it.
func TestSubmitEvaluation_MCP_StoresGradesAndServerComputesScore(t *testing.T) {
	f := newEvaluationFixture(t)

	pendingIDs := func() []string {
		t.Helper()
		isError, text, out := f.call(t, "list_pending_evaluations", map[string]any{"profile_id": f.profileID.String()})
		if isError {
			t.Fatalf("list_pending_evaluations failed: %s", text)
		}
		var ids []string
		for _, entry := range out["listings"].([]any) {
			item := entry.(map[string]any)
			if item["title"] != "Haus am See" || item["status"] != "new" || item["overall_score"] != nil {
				t.Errorf("pending entry = %v, want title, status new and null overall_score", item)
			}
			ids = append(ids, item["id"].(string))
		}
		return ids
	}
	if ids := pendingIDs(); len(ids) != 1 || ids[0] != f.listingID.String() {
		t.Fatalf("pending before submit = %v, want [%s]", ids, f.listingID)
	}

	// Grades in reverse order of the criteria: order must not matter.
	isError, text, out := f.call(t, "submit_evaluation", map[string]any{
		"listing_id": f.listingID.String(),
		"grades":     []any{grade(f.location, " c ", "  mittel  "), grade(f.price, "A", "sehr günstig")},
	})
	if isError {
		t.Fatalf("submit failed: %s", text)
	}

	// Server computation: A=100 (weight 3), C=60 (weight 1) → 360/4.
	const wantScore = (100.0*3 + 60.0*1) / 4
	listing := out["listing"].(map[string]any)
	if score, _ := listing["overall_score"].(float64); math.Abs(score-wantScore) > 0.01 {
		t.Errorf("returned overall_score = %v, want %v", listing["overall_score"], wantScore)
	}
	if listing["needs_reevaluation"] != false {
		t.Errorf("returned needs_reevaluation = %v, want false", listing["needs_reevaluation"])
	}
	var storedScore float64
	var stale bool
	if err := f.pool.QueryRow(context.Background(), `SELECT overall_score, needs_reevaluation FROM listing WHERE id = $1`, f.listingID).Scan(&storedScore, &stale); err != nil {
		t.Fatal(err)
	}
	if math.Abs(storedScore-wantScore) > 0.01 || stale {
		t.Errorf("stored overall_score=%v needs_reevaluation=%v, want %v and false", storedScore, stale, wantScore)
	}

	stored := f.storedGrades(t)
	if got := stored[f.price.ID]; got.Grade != domain.GradeA || got.Reasoning != "sehr günstig" {
		t.Errorf("price grade = %+v, want A / sehr günstig", got)
	}
	if got := stored[f.location.ID]; got.Grade != domain.GradeC || got.Reasoning != "mittel" {
		t.Errorf("location grade = %+v, want C / mittel (normalized)", got)
	}
	if len(stored) != 2 {
		t.Errorf("%d stored grades, want 2", len(stored))
	}
	if returned, _ := out["grades"].([]any); len(returned) != 2 {
		t.Errorf("returned %d grades, want 2", len(returned))
	}

	if ids := pendingIDs(); len(ids) != 0 {
		t.Errorf("pending after submit = %v, want none", ids)
	}
}
