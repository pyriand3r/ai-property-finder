package agent

import (
	"fmt"
	"propertyfinder/internal/domain"
	"strings"
	"testing"
)

func TestBuildListingContextBlock_EmptyWhenNoConfirmedFields(t *testing.T) {
	listing := domain.Listing{Title: "Kappmuren 2", UserConfirmedFields: nil}

	if block := BuildListingContextBlock(listing); block != "" {
		t.Errorf("block = %q, want empty for a listing with no confirmed fields", block)
	}
}

func TestBuildListingContextBlock_RendersOnlyConfirmedFields(t *testing.T) {
	price := 310000.0
	currency := "SEK"
	address := "Kappmuren 12, 240 32 Malmö"
	description := "Should not appear — not confirmed"

	listing := domain.Listing{
		Title:               "Kappmuren 2",
		Price:               &price,
		PriceCurrency:       &currency,
		Address:             &address,
		Description:         &description,
		UserConfirmedFields: []string{"price", "address"},
	}

	block := BuildListingContextBlock(listing)

	if !strings.Contains(block, "## Aktuelles Inserat (Kappmuren 2)") {
		t.Errorf("block missing title heading: %q", block)
	}
	if !strings.Contains(block, "- price: 310000 SEK") {
		t.Errorf("block missing confirmed price line: %q", block)
	}
	if !strings.Contains(block, "- address: Kappmuren 12, 240 32 Malmö") {
		t.Errorf("block missing confirmed address line: %q", block)
	}
	if strings.Contains(block, "description") {
		t.Errorf("block should not mention description (not confirmed): %q", block)
	}
}

func TestBuildListingContextBlock_FieldOrderMatchesConfirmableListingFields(t *testing.T) {
	price := 100.0
	address := "Somewhere 1"

	// Confirmed out of order — output must still follow
	// confirmableListingFields' order (price before address), not
	// UserConfirmedFields' order.
	listing := domain.Listing{
		Title:               "X",
		Price:               &price,
		Address:             &address,
		UserConfirmedFields: []string{"address", "price"},
	}

	block := BuildListingContextBlock(listing)
	priceIdx := strings.Index(block, "- price:")
	addressIdx := strings.Index(block, "- address:")
	if priceIdx == -1 || addressIdx == -1 || priceIdx > addressIdx {
		t.Errorf("block = %q, want price line before address line regardless of confirmation order", block)
	}
}

func TestBuildSystemPrompt_OmitsEmptyListingBlock(t *testing.T) {
	blocks := BuildSystemPrompt(false, 10, "", "", "")

	for _, block := range blocks {
		if strings.Contains(block.Text, "Aktuelles Inserat") {
			t.Errorf("blocks contain a listing context section despite empty input: %+v", blocks)
		}
	}
}

func TestBuildSystemPrompt_IncludesListingBlockWhenNonEmpty(t *testing.T) {
	listingBlock := "## Aktuelles Inserat (Test)\n- price: 100 SEK"
	blocks := BuildSystemPrompt(false, 10, "", "", listingBlock)

	found := false
	for _, block := range blocks {
		if block.Text == listingBlock {
			found = true
		}
	}
	if !found {
		t.Errorf("blocks = %+v, want one block matching the listing context text exactly", blocks)
	}
}

// TestBuildSystemPrompt_BatchLimitFitsTurnBudget guards issue #20: the number
// of Listings the Agent may import from one message is stated in the prompt
// only when Tools exist, and must stay within the default tool-call budget
// (about four calls per Listing, ADR 0013) so a full batch is not cut off
// mid-way.
func TestBuildSystemPrompt_BatchLimitFitsTurnBudget(t *testing.T) {
	const toolCallsPerListing = 4
	if MaxBatchListingsPerTurn*toolCallsPerListing > DefaultMaxToolCallsPerTurn {
		t.Errorf("MaxBatchListingsPerTurn = %d needs %d tool calls, default budget is %d",
			MaxBatchListingsPerTurn, MaxBatchListingsPerTurn*toolCallsPerListing, DefaultMaxToolCallsPerTurn)
	}

	want := fmt.Sprintf("höchstens %d Inserate", MaxBatchListingsPerTurn)
	if got := BuildSystemPrompt(true, 5, "", "", "")[0].Text; !strings.Contains(got, want) {
		t.Errorf("prompt with tools lacks %q", want)
	}
	if got := BuildSystemPrompt(false, 5, "", "", "")[0].Text; strings.Contains(got, "Mehrere Links") {
		t.Errorf("prompt without tools mentions batch ingestion: %q", got)
	}
}
