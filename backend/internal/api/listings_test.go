package api

import (
	"encoding/json"
	"testing"
)

// TestListingPatchBody_NullClearsAndAbsentKeeps pins the wire contract for
// issue #9: through toPatch, an explicit null reaches the service as a clear
// and an omitted key as "unchanged".
func TestListingPatchBody_NullClearsAndAbsentKeeps(t *testing.T) {
	var body listingPatchBody
	raw := `{"seller_phone":null,"address":"Rua 2","price":null,"title":null}`
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	patch := body.toPatch()

	if !patch.SellerPhone.Set || patch.SellerPhone.Value != nil {
		t.Errorf("seller_phone null: got %+v, want clear", patch.SellerPhone)
	}
	if !patch.Price.Set || patch.Price.Value != nil {
		t.Errorf("price null: got %+v, want clear", patch.Price)
	}
	if !patch.Address.Set || patch.Address.Value == nil || *patch.Address.Value != "Rua 2" {
		t.Errorf("address: got %+v, want set to Rua 2", patch.Address)
	}
	if patch.SellerEmail.Set {
		t.Errorf("seller_email (absent): got %+v, want unset", patch.SellerEmail)
	}
	if patch.Title != nil {
		t.Errorf("title null: got %q, want nil (not clearable)", *patch.Title)
	}
}
