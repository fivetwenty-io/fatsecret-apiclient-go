package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// FlexSlice is a generic slice type whose JSON unmarshaler accepts three forms:
//
//  1. A JSON array  (e.g. [{...},{...}])  → decoded as []T directly.
//  2. A JSON object (e.g. {...})          → decoded as a single T, wrapped in []T of length 1.
//  3. JSON null or empty string           → decoded as nil.
//
// This is required for FatSecret v1 API endpoints, which collapse a one-element
// collection to a bare object instead of a single-element array. For example,
// foods.search returns "food":{...} when there is one result but "food":[{...},{...}]
// when there are multiple.
//
// MarshalJSON always emits a JSON array, so the single-object ambiguity does not
// propagate to callers that re-encode the value.
//
// The type parameter T may be any type that is itself JSON-decodable.
type FlexSlice[T any] []T

// UnmarshalJSON implements [encoding/json.Unmarshaler].
//
// Dispatch logic:
//   - First non-whitespace byte is '[' → decode as JSON array into []T.
//   - First non-whitespace byte is '{' or any other non-null byte → decode as single T,
//     then wrap in a one-element []T.
//   - Input is "null", empty, or the quoted empty string "" → set to nil.
func (f *FlexSlice[T]) UnmarshalJSON(b []byte) error {
	// Empty input: treat as nil, not an error.
	if len(b) == 0 {
		*f = nil
		return nil
	}

	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		*f = nil
		return nil
	}

	// JSON null or quoted empty string "" → nil slice.
	if isNull(trimmed) {
		*f = nil
		return nil
	}
	// Quoted empty string: `""` — two bytes, both double-quote.
	if len(trimmed) == 2 && trimmed[0] == '"' && trimmed[1] == '"' {
		*f = nil
		return nil
	}

	switch trimmed[0] {
	case '[':
		// Standard JSON array path.
		var items []T
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return fmt.Errorf("types: FlexSlice: array decode: %w", err)
		}
		*f = items

	default:
		// Single item (object, string, number, boolean) — wrap in a one-element slice.
		var item T
		if err := json.Unmarshal(trimmed, &item); err != nil {
			return fmt.Errorf("types: FlexSlice: single-item decode: %w", err)
		}
		*f = FlexSlice[T]{item}
	}

	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It always emits a JSON array,
// regardless of how many elements are present. A nil FlexSlice marshals to [].
func (f FlexSlice[T]) MarshalJSON() ([]byte, error) {
	if f == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(f))
}

// Items returns the underlying []T slice. It is equivalent to a plain []T conversion
// but is provided for code clarity at call sites.
func (f FlexSlice[T]) Items() []T { return []T(f) }
