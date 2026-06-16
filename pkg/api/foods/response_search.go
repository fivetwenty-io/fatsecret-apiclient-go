package foods

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

// UnmarshalJSON decodes the FatSecret foods.search payload into FoodsSearchResult.
//
// The generated struct models `food` at the top level, but every foods.search
// endpoint (v1–v5) actually nests the matched food list one level deeper, under
// a "results" object:
//
//	"foods_search": {
//	  "max_results": "20", "total_results": "1982", "page_number": "0",
//	  "results": { "food": [ ... ] }
//	}
//
// FatSecret additionally returns "results" as an empty string (or omits it
// entirely) when total_results is 0, and collapses "food" to a single object —
// rather than a one-element array — when exactly one food matches. This
// unmarshaler flattens all of those shapes into the top-level Food slice so
// callers can range over result.Food directly; FlexSlice absorbs the
// single-object case.
//
// NOTE: spec/fatsecret.yaml models FoodsSearchResult without the "results"
// wrapper, so the generated struct (response.go) is wrong for this endpoint.
// This hand-written unmarshaler lives in a separate file so it survives
// regeneration; the spec/generator should be corrected to emit it directly.
func (r *FoodsSearchResult) UnmarshalJSON(b []byte) error {
	// alias breaks UnmarshalJSON recursion so the top-level scalar fields
	// (max_results, total_results, page_number) decode with the default
	// struct unmarshaler.
	type alias FoodsSearchResult
	aux := struct {
		*alias
		Results json.RawMessage `json:"results"`
	}{alias: (*alias)(r)}

	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	results := bytes.TrimSpace(aux.Results)
	if len(results) == 0 ||
		bytes.Equal(results, []byte("null")) ||
		bytes.Equal(results, []byte(`""`)) {
		// Zero-result responses omit the results object; leave Food nil.
		return nil
	}

	var nested struct {
		Food types.FlexSlice[Food] `json:"food,omitempty"`
	}
	if err := json.Unmarshal(results, &nested); err != nil {
		return fmt.Errorf("foods: decode search results: %w", err)
	}
	r.Food = nested.Food
	return nil
}

// UnmarshalJSON decodes a FatSecret food object.
//
// Like the search results list, the serving list is nested one level deeper than
// the generated struct models it: FatSecret returns
//
//	"servings": { "serving": [ ... ] }
//
// rather than "servings": [ ... ]. With a single serving FatSecret collapses
// "serving" to an object, and with none it returns an empty string or omits the
// field. This unmarshaler flattens all shapes into the top-level Servings slice;
// without it every Food parses with zero servings and therefore zero nutrients.
func (f *Food) UnmarshalJSON(b []byte) error {
	type alias Food
	aux := struct {
		*alias
		Servings json.RawMessage `json:"servings"`
	}{alias: (*alias)(f)}

	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	servings := bytes.TrimSpace(aux.Servings)
	if len(servings) == 0 ||
		bytes.Equal(servings, []byte("null")) ||
		bytes.Equal(servings, []byte(`""`)) {
		return nil
	}

	var nested struct {
		Serving types.FlexSlice[Serving] `json:"serving,omitempty"`
	}
	if err := json.Unmarshal(servings, &nested); err != nil {
		return fmt.Errorf("foods: decode food servings: %w", err)
	}
	f.Servings = nested.Serving
	return nil
}
