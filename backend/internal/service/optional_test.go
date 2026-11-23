package service

import (
	"encoding/json"
	"testing"
)

// TestOptional_Unmarshal_DistinguishesAbsentNullAndValue is the regression
// test for issue #9: encoding/json turned both a missing key and an explicit
// null into a nil *string, so a PATCH could never clear a field.
func TestOptional_Unmarshal_DistinguishesAbsentNullAndValue(t *testing.T) {
	var body struct {
		Absent  Optional[string]  `json:"absent"`
		Null    Optional[string]  `json:"null"`
		Text    Optional[string]  `json:"text"`
		Empty   Optional[string]  `json:"empty"`
		Number  Optional[float64] `json:"number"`
		NumNull Optional[float64] `json:"num_null"`
	}
	raw := `{"null":null,"text":"abc","empty":"","number":12.5,"num_null":null}`
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.Absent.Set || body.Absent.Value != nil {
		t.Errorf("absent: got %+v, want unset", body.Absent)
	}
	if !body.Null.Set || body.Null.Value != nil {
		t.Errorf("null: got %+v, want Set with nil Value", body.Null)
	}
	if !body.Text.Set || body.Text.Value == nil || *body.Text.Value != "abc" {
		t.Errorf("text: got %+v, want Set with \"abc\"", body.Text)
	}
	if !body.Empty.Set || body.Empty.Value == nil || *body.Empty.Value != "" {
		t.Errorf("empty: got %+v, want Set with empty string", body.Empty)
	}
	if !body.Number.Set || body.Number.Value == nil || *body.Number.Value != 12.5 {
		t.Errorf("number: got %+v, want Set with 12.5", body.Number)
	}
	if !body.NumNull.Set || body.NumNull.Value != nil {
		t.Errorf("num_null: got %+v, want Set with nil Value", body.NumNull)
	}
}

func TestOptional_Unmarshal_WrongTypeIsError(t *testing.T) {
	var body struct {
		Price Optional[float64] `json:"price"`
	}
	if err := json.Unmarshal([]byte(`{"price":"abc"}`), &body); err == nil {
		t.Error("expected error decoding a string into Optional[float64]")
	}
}
