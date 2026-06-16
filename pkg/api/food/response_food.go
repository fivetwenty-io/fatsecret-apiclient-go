package food

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

// UnmarshalJSON decodes a FatSecret food object.
//
// The generated struct models `servings` as a flat list, but FatSecret nests it
// under a singular "serving" key:
//
//	"servings": { "serving": [ ... ] }
//
// With a single serving FatSecret collapses "serving" to an object, and with
// none it returns an empty string or omits the field. This unmarshaler flattens
// all shapes into the top-level Servings slice; without it food.Get and the
// barcode lookup parse with zero servings and therefore zero nutrients.
//
// NOTE: spec/fatsecret.yaml models Food without the "serving" wrapper, so the
// generated struct (response.go) is wrong for this field. This hand-written
// unmarshaler lives in a separate file so it survives regeneration; the
// spec/generator should be corrected to emit it directly.
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
		return fmt.Errorf("food: decode food servings: %w", err)
	}
	f.Servings = nested.Serving
	return nil
}
