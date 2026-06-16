package food_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/food"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client/mock"
	fserrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

// barcodeServer routes the two-step barcode flow: the method-style find-id call
// returns a food_id wrapper, and the food/v4 get returns the full food.
func barcodeServer(t *testing.T, foodIDJSON, foodJSON string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/server.api"):
			if m := r.URL.Query().Get("method"); m != "food.find_id_for_barcode" {
				t.Errorf("find-id: method = %q, want food.find_id_for_barcode", m)
			}
			_, _ = w.Write([]byte(foodIDJSON))
		case strings.HasPrefix(r.URL.Path, "/rest/food/v4"):
			_, _ = w.Write([]byte(foodJSON))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestFindIDForBarcode_ResolvesAndFetchesFullFood(t *testing.T) {
	url := barcodeServer(t,
		`{"food_id":{"value":"51878"}}`,
		`{"food":{"food_id":"51878","food_name":"Diet Coke","food_type":"Brand","brand_name":"Coca-Cola","food_url":"https://x","servings":{"serving":{"serving_id":"1","serving_description":"1 can","calories":"0","protein":"0","carbohydrate":"0","fat":"0","is_default":"1"}}}}`,
	)
	svc := food.New(mock.NewClient(url))

	bc := "0049000028911"
	f, err := svc.FindIDForBarcode(context.Background(), food.FindIDForBarcodeRequest{Barcode: &bc})
	if err != nil {
		t.Fatalf("FindIDForBarcode: %v", err)
	}
	if int64(f.FoodID) != 51878 {
		t.Errorf("FoodID = %d, want 51878", int64(f.FoodID))
	}
	if f.FoodName != "Diet Coke" {
		t.Errorf("FoodName = %q, want Diet Coke", f.FoodName)
	}
	if len(f.Servings) != 1 {
		t.Fatalf("Servings len = %d, want 1 (nested serving must be flattened)", len(f.Servings))
	}
}

func TestFindIDForBarcode_UnknownBarcode_ReturnsNotFound(t *testing.T) {
	// FatSecret returns food_id 0 for a barcode absent from its catalog.
	url := barcodeServer(t, `{"food_id":{"value":"0"}}`, `{}`)
	svc := food.New(mock.NewClient(url))

	bc := "0000000000000"
	_, err := svc.FindIDForBarcode(context.Background(), food.FindIDForBarcodeRequest{Barcode: &bc})
	if !errors.Is(err, fserrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for food_id 0, got %v", err)
	}
}

// ensure types import is used even if assertions above change.
var _ = types.APIInt(0)
