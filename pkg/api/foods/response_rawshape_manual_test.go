package foods

import (
	"encoding/json"
	"testing"
)

// food_images and food_attributes have never been observed on the wire: no
// caller sets include_food_images / include_food_attributes, and requesting
// them on foods.search returns nothing at this account tier. Their real shape
// is unknown, and modelling an unverified shape as a bare scalar is exactly
// what broke food_sub_categories — one mistyped field fails the enclosing
// decode, and the enclosing decode is the whole reply.
//
// Until a live capture confirms a shape, both fields are held as
// json.RawMessage: any shape FatSecret sends is preserved byte-for-byte and
// can never fail the decode. These tests pin that property across every shape
// the vendor is known to use anywhere: nested wrapper object, bare string,
// array, and absent.

func TestFood_UnverifiedFieldsAcceptWrapperObject(t *testing.T) {
	// The likeliest true shape, by analogy with every other list field:
	// {"food_images":{"food_image":[{"image_url":"..."}]}}. As a bare string
	// this failed the whole Food decode; as RawMessage it must not.
	payload := `{
		"food_id": "12345",
		"food_name": "Granola",
		"food_images": {"food_image": [{"image_url": "https://example.test/a.jpg"}]},
		"food_attributes": {"allergens": {"allergen": [{"id": "1", "name": "Gluten", "value": "1"}]}}
	}`

	var f Food
	if err := json.Unmarshal([]byte(payload), &f); err != nil {
		t.Fatalf("object-shaped food_images/food_attributes failed the Food decode: %v", err)
	}
	if f.FoodName != "Granola" {
		t.Fatalf("sibling fields lost: food_name = %q", f.FoodName)
	}
	if len(f.FoodImages) == 0 {
		t.Fatal("food_images bytes not preserved")
	}
	var probe struct {
		FoodImage []struct {
			ImageURL string `json:"image_url"`
		} `json:"food_image"`
	}
	if err := json.Unmarshal(f.FoodImages, &probe); err != nil {
		t.Fatalf("preserved food_images bytes not re-decodable: %v", err)
	}
	if len(probe.FoodImage) != 1 || probe.FoodImage[0].ImageURL != "https://example.test/a.jpg" {
		t.Fatalf("food_images bytes mangled: %s", f.FoodImages)
	}
	if len(f.FoodAttributes) == 0 {
		t.Fatal("food_attributes bytes not preserved")
	}
}

func TestFood_UnverifiedFieldsAcceptStringAndArray(t *testing.T) {
	for name, payload := range map[string]string{
		"bare string":  `{"food_name": "Oats", "food_images": "https://example.test/a.jpg"}`,
		"array":        `{"food_name": "Oats", "food_images": ["https://example.test/a.jpg"]}`,
		"empty string": `{"food_name": "Oats", "food_images": ""}`,
	} {
		t.Run(name, func(t *testing.T) {
			var f Food
			if err := json.Unmarshal([]byte(payload), &f); err != nil {
				t.Fatalf("shape %s failed the Food decode: %v", name, err)
			}
			if f.FoodName != "Oats" {
				t.Fatalf("sibling fields lost: food_name = %q", f.FoodName)
			}
		})
	}
}

func TestFood_UnverifiedFieldsAbsent(t *testing.T) {
	var f Food
	if err := json.Unmarshal([]byte(`{"food_name": "Oats"}`), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if f.FoodImages != nil || f.FoodAttributes != nil {
		t.Fatalf("absent fields must stay nil, got images=%q attributes=%q", f.FoodImages, f.FoodAttributes)
	}
}
