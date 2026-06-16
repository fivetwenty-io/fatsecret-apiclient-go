package foods

import (
	"encoding/json"
	"testing"
)

// unwrap extracts the foods_search object from a full FatSecret reply envelope,
// mirroring what service.Search / service.SearchV5 do before unmarshalling.
func unwrapFoodsSearch(t *testing.T, body string) FoodsSearchResult {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("envelope unmarshal: %v", err)
	}
	raw, ok := env["foods_search"]
	if !ok {
		t.Fatalf("missing foods_search key in %s", body)
	}
	var res FoodsSearchResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("FoodsSearchResult unmarshal: %v", err)
	}
	return res
}

func TestFoodsSearchResult_NestedResultsArray(t *testing.T) {
	body := `{"foods_search":{"max_results":"2","total_results":"1982","page_number":"0","results":{"food":[
		{"food_id":"35755","food_name":"Bananas","food_type":"Generic","food_url":"https://x"},
		{"food_id":"35756","food_name":"Banana Bread","food_type":"Generic","food_url":"https://y"}
	]}}}`
	res := unwrapFoodsSearch(t, body)

	if res.TotalResults.Int64() != 1982 {
		t.Errorf("TotalResults = %d, want 1982", res.TotalResults.Int64())
	}
	if res.MaxResults.Int64() != 2 {
		t.Errorf("MaxResults = %d, want 2", res.MaxResults.Int64())
	}
	if len(res.Food) != 2 {
		t.Fatalf("Food len = %d, want 2", len(res.Food))
	}
	if res.Food[0].FoodName != "Bananas" || res.Food[1].FoodName != "Banana Bread" {
		t.Errorf("unexpected food names: %q, %q", res.Food[0].FoodName, res.Food[1].FoodName)
	}
}

func TestFoodsSearchResult_NestedResultsSingleObject(t *testing.T) {
	// FatSecret collapses results.food to a single object when one match exists.
	body := `{"foods_search":{"max_results":"50","total_results":"1","page_number":"0","results":{"food":
		{"food_id":"35755","food_name":"Bananas","food_type":"Generic","food_url":"https://x"}
	}}}`
	res := unwrapFoodsSearch(t, body)
	if len(res.Food) != 1 {
		t.Fatalf("Food len = %d, want 1 (single-object collapse)", len(res.Food))
	}
	if res.Food[0].FoodName != "Bananas" {
		t.Errorf("FoodName = %q, want Bananas", res.Food[0].FoodName)
	}
}

func TestFoodsSearchResult_ZeroResults(t *testing.T) {
	// Zero matches: FatSecret returns results as an empty string.
	for _, body := range []string{
		`{"foods_search":{"max_results":"50","total_results":"0","page_number":"0","results":""}}`,
		`{"foods_search":{"max_results":"50","total_results":"0","page_number":"0"}}`,
	} {
		res := unwrapFoodsSearch(t, body)
		if res.TotalResults.Int64() != 0 {
			t.Errorf("TotalResults = %d, want 0", res.TotalResults.Int64())
		}
		if len(res.Food) != 0 {
			t.Errorf("Food len = %d, want 0 for %s", len(res.Food), body)
		}
	}
}

func TestFoodsSearchResult_ServingsPreserved(t *testing.T) {
	body := `{"foods_search":{"max_results":"1","total_results":"1","page_number":"0","results":{"food":{"food_id":"35755","food_name":"Bananas","food_type":"Generic","food_url":"https://x","servings":{"serving":{"serving_id":"1","serving_description":"1 medium","metric_serving_amount":"118.000","metric_serving_unit":"g","calories":"105","protein":"1.29","carbohydrate":"26.95","fat":"0.39","is_default":"1"}}}}}}`
	res := unwrapFoodsSearch(t, body)
	if len(res.Food) != 1 || len(res.Food[0].Servings) != 1 {
		t.Fatalf("expected 1 food with 1 serving, got %d food", len(res.Food))
	}
	s := res.Food[0].Servings[0]
	if s.Protein.Float64() != 1.29 {
		t.Errorf("Protein = %v, want 1.29", s.Protein.Float64())
	}
	if s.MetricServingAmount == nil || s.MetricServingAmount.Float64() != 118 {
		t.Errorf("MetricServingAmount = %v, want 118", s.MetricServingAmount)
	}
}
