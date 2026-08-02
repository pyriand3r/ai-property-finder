package ai

import (
	"errors"
	"propertyfinder/internal/domain"
	"testing"

	"github.com/google/uuid"
)

func testCriteria(n int) []domain.Criterion {
	criteria := make([]domain.Criterion, n)
	for i := range criteria {
		criteria[i] = domain.Criterion{ID: uuid.New(), Name: "criterion"}
	}
	return criteria
}

func TestParseGrades_WellFormed(t *testing.T) {
	criteria := testCriteria(2)
	raw := `[{"index":1,"grade":"A","reasoning":"great fit"},{"index":2,"grade":"C","reasoning":"meh"}]`

	scores, err := parseGrades(raw, criteria)
	if err != nil {
		t.Fatalf("parseGrades failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("got %d scores, want 2", len(scores))
	}
	if scores[0].CriterionID != criteria[0].ID || scores[0].Grade != domain.GradeA {
		t.Errorf("score[0] = %+v, want CriterionID=%v Grade=A", scores[0], criteria[0].ID)
	}
	if scores[1].CriterionID != criteria[1].ID || scores[1].Grade != domain.GradeC {
		t.Errorf("score[1] = %+v, want CriterionID=%v Grade=C", scores[1], criteria[1].ID)
	}
}

func TestParseGrades_FencedAndProse(t *testing.T) {
	criteria := testCriteria(1)
	raw := "Sure, here you go:\n```json\n[{\"index\":1,\"grade\":\"b\",\"reasoning\":\"ok\"}]\n```\nLet me know if you need more."

	scores, err := parseGrades(raw, criteria)
	if err != nil {
		t.Fatalf("parseGrades failed on fenced+prose input: %v", err)
	}
	if scores[0].Grade != domain.GradeB {
		t.Errorf("grade = %q, want B (lowercase input should normalize)", scores[0].Grade)
	}
}

func TestParseGrades_WrongLength(t *testing.T) {
	criteria := testCriteria(2)
	raw := `[{"index":1,"grade":"A","reasoning":"x"}]`

	_, err := parseGrades(raw, criteria)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for wrong-length array, got %v", err)
	}
}

func TestParseGrades_InvalidGrade(t *testing.T) {
	criteria := testCriteria(1)
	raw := `[{"index":1,"grade":"Z","reasoning":"x"}]`

	_, err := parseGrades(raw, criteria)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for invalid grade letter, got %v", err)
	}
}

func TestParseGrades_MalformedJSON(t *testing.T) {
	criteria := testCriteria(1)

	_, err := parseGrades("not json at all", criteria)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for non-JSON input, got %v", err)
	}
}

func TestParseRanking_WellFormed(t *testing.T) {
	raw := `{"kurzfazits":["Beste Lage","Guenstigster Preis"],"summary":"Nummer 1 hat die beste Lage, Nummer 2 ist am guenstigsten."}`

	result, err := parseRanking(raw, 2)
	if err != nil {
		t.Fatalf("parseRanking failed: %v", err)
	}
	if len(result.Kurzfazits) != 2 {
		t.Fatalf("got %d kurzfazits, want 2", len(result.Kurzfazits))
	}
	if result.Kurzfazits[0] != "Beste Lage" || result.Kurzfazits[1] != "Guenstigster Preis" {
		t.Errorf("kurzfazits = %+v, unexpected content/order", result.Kurzfazits)
	}
	if result.Summary == "" {
		t.Error("summary is empty")
	}
}

func TestParseRanking_FencedAndProse(t *testing.T) {
	raw := "Here's the ranking:\n```json\n{\"kurzfazits\":[\"A\"],\"summary\":\"B\"}\n```\nHope that helps!"

	result, err := parseRanking(raw, 1)
	if err != nil {
		t.Fatalf("parseRanking failed on fenced+prose input: %v", err)
	}
	if result.Kurzfazits[0] != "A" || result.Summary != "B" {
		t.Errorf("result = %+v, unexpected content", result)
	}
}

func TestParseRanking_WrongLength(t *testing.T) {
	raw := `{"kurzfazits":["only one"],"summary":"x"}`

	_, err := parseRanking(raw, 2)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for wrong-length kurzfazits, got %v", err)
	}
}

func TestParseRanking_EmptySummary(t *testing.T) {
	raw := `{"kurzfazits":["A"],"summary":"   "}`

	_, err := parseRanking(raw, 1)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for empty summary, got %v", err)
	}
}

func TestParseRanking_MalformedJSON(t *testing.T) {
	_, err := parseRanking("not json at all", 1)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse for non-JSON input, got %v", err)
	}
}

func TestRenderListing_OmitsEmptyFields(t *testing.T) {
	listing := domain.Listing{Title: "Test House"}
	rendered := renderListing(listing)
	if rendered != "Title: Test House" {
		t.Errorf("renderListing with only Title set = %q, want exactly %q", rendered, "Title: Test House")
	}
}

func TestRenderListing_IncludesSetFields(t *testing.T) {
	price := 310000.0
	currency := "SEK"
	listing := domain.Listing{Title: "Kappmuren 2", Price: &price, PriceCurrency: &currency}
	rendered := renderListing(listing)
	want := "Title: Kappmuren 2\nPrice: 310000.00 SEK"
	if rendered != want {
		t.Errorf("renderListing = %q, want %q", rendered, want)
	}
}
