package foods

import (
	"encoding/json"
	"testing"
)

// food_sub_categories is a nested wrapper on the wire, the same shape servings
// uses: {"food_sub_categories":{"food_sub_category":[...]}}. It was modelled as
// a bare string until v0.0.4, which made every decode of a food carrying the
// field fail outright — and because Food is decoded as part of a larger
// envelope, the failure took the whole reply with it, not just the one field.
//
// image-recognition/v2 with include_food_data=true is where this surfaced: the
// nested catalogue food object carries sub-categories, so a single branded food
// in a photo failed the entire recognition response. Search never requested
// sub-categories, which is why only the photo path broke.
//
// These cover the four shapes FatSecret uses for a nested list: full array,
// single-element collapse to a bare object, empty string, and absent.

func TestFood_SubCategoriesNestedArray(t *testing.T) {
	body := `{"food_id":"36310","food_name":"Granola","food_type":"Brand","food_url":"https://x",
		"food_sub_categories":{"food_sub_category":["Cereal","Breakfast Foods"]}}`

	var f Food
	if err := json.Unmarshal([]byte(body), &f); err != nil {
		t.Fatalf("Food unmarshal: %v", err)
	}
	got := f.FoodSubCategories.Items()
	if len(got) != 2 {
		t.Fatalf("want 2 sub-categories, got %d (%v)", len(got), got)
	}
	if got[0] != "Cereal" || got[1] != "Breakfast Foods" {
		t.Errorf("unexpected sub-categories: %v", got)
	}
}

// A single sub-category collapses to a bare string rather than a one-element
// array — FatSecret does this to every nested list.
func TestFood_SubCategoriesSingleCollapse(t *testing.T) {
	body := `{"food_id":"36310","food_name":"Granola","food_type":"Brand","food_url":"https://x",
		"food_sub_categories":{"food_sub_category":"Cereal"}}`

	var f Food
	if err := json.Unmarshal([]byte(body), &f); err != nil {
		t.Fatalf("Food unmarshal: %v", err)
	}
	got := f.FoodSubCategories.Items()
	if len(got) != 1 || got[0] != "Cereal" {
		t.Fatalf("want [Cereal], got %v", got)
	}
}

// No sub-categories: the wrapper is emitted as an empty string, or omitted
// entirely. Neither is an error, and both leave the slice empty.
func TestFood_SubCategoriesEmptyAndAbsent(t *testing.T) {
	for name, body := range map[string]string{
		"empty string": `{"food_id":"1","food_name":"Banana","food_type":"Generic","food_url":"https://x","food_sub_categories":""}`,
		"null":         `{"food_id":"1","food_name":"Banana","food_type":"Generic","food_url":"https://x","food_sub_categories":null}`,
		"absent":       `{"food_id":"1","food_name":"Banana","food_type":"Generic","food_url":"https://x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var f Food
			if err := json.Unmarshal([]byte(body), &f); err != nil {
				t.Fatalf("Food unmarshal: %v", err)
			}
			if got := f.FoodSubCategories.Items(); len(got) != 0 {
				t.Errorf("want no sub-categories, got %v", got)
			}
		})
	}
}

// The regression itself: sub-categories must not cost us the sibling fields.
// The pre-v0.0.4 failure mode was total — servings, name, and id all vanished
// with the reply, which is what turned one mistyped field into an outage.
func TestFood_SubCategoriesDoNotDisturbServings(t *testing.T) {
	body := `{"food_id":"36310","food_name":"Granola","food_type":"Brand","food_url":"https://x",
		"food_sub_categories":{"food_sub_category":["Cereal"]},
		"servings":{"serving":[{"serving_id":"1","serving_description":"1 cup","metric_serving_amount":"60.0","metric_serving_unit":"g"}]}}`

	var f Food
	if err := json.Unmarshal([]byte(body), &f); err != nil {
		t.Fatalf("Food unmarshal: %v", err)
	}
	if f.FoodName != "Granola" {
		t.Errorf("food_name = %q, want Granola", f.FoodName)
	}
	if got := f.Servings.Items(); len(got) != 1 {
		t.Fatalf("want 1 serving alongside sub-categories, got %d", len(got))
	}
	if got := f.FoodSubCategories.Items(); len(got) != 1 {
		t.Fatalf("want 1 sub-category alongside servings, got %d", len(got))
	}
}
