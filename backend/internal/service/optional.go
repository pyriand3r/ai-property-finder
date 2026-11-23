package service

import "encoding/json"

// Optional distinguishes the three states a PATCH body can give a nullable
// field, which a bare pointer cannot (encoding/json maps both a missing key
// and an explicit null to nil):
//
//   - key absent        → Set == false            (leave the field unchanged)
//   - key is null       → Set == true, Value nil  (clear the field)
//   - key has a value   → Set == true, Value != nil
//
// Only UnmarshalJSON sets Set, so decoding a request body is the sole way to
// get the "clear" state; the zero value is always "absent".
type Optional[T any] struct {
	Set   bool
	Value *T
}

// Some returns an Optional carrying value — a convenience for tests and for
// code building a patch programmatically.
func Some[T any](value T) Optional[T] {
	return Optional[T]{Set: true, Value: &value}
}

// Clear returns an Optional that explicitly clears the field.
func Clear[T any]() Optional[T] {
	return Optional[T]{Set: true}
}

// UnmarshalJSON is invoked by encoding/json for a present key, including a
// literal null, but never for an absent key.
func (optional *Optional[T]) UnmarshalJSON(data []byte) error {
	optional.Set = true
	if string(data) == "null" {
		optional.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	optional.Value = &value
	return nil
}
