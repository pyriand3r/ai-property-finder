package service

import (
	"encoding/json"
	"testing"
)

// TestParseCriteriaProposal_DecodesHTMLEntities is the regression test for
// issue #4: the model wrote `Grundstück &amp; Lage` into a propose_criteria
// argument and the entity was stored in criterion.name as it was.
func TestParseCriteriaProposal_DecodesHTMLEntities(t *testing.T) {
	raw := json.RawMessage(`{"action":"add","name":"Grundstück &amp; Lage","description":"Nähe &lt;5 km &amp; ruhig &quot;Gasse&quot; it&#39;s","weight":3}`)

	got, err := parseCriteriaProposal(raw)
	if err != nil {
		t.Fatalf("parseCriteriaProposal: %v", err)
	}
	if want := "Grundstück & Lage"; got.Name != want {
		t.Errorf("Name = %q, want %q", got.Name, want)
	}
	if want := `Nähe <5 km & ruhig "Gasse" it's`; got.Description != want {
		t.Errorf("Description = %q, want %q", got.Description, want)
	}
	if got.Weight == nil || *got.Weight != 3 {
		t.Errorf("Weight = %v, want 3", got.Weight)
	}
}

func TestParseCriteriaProposal_LeavesPlainTextUntouched(t *testing.T) {
	raw := json.RawMessage(`{"action":"add","name":"Preis & Lage","description":"A < B"}`)

	got, err := parseCriteriaProposal(raw)
	if err != nil {
		t.Fatalf("parseCriteriaProposal: %v", err)
	}
	if got.Name != "Preis & Lage" || got.Description != "A < B" {
		t.Errorf("got %q / %q, want plain text unchanged", got.Name, got.Description)
	}
}
