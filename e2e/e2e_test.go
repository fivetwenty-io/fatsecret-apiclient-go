// Package e2e contains an end-to-end demonstration that wires the whole client
// stack together against an httptest server: OAuth2 token acquisition, a typed
// generated API call, tolerant JSON decoding of FatSecret wire quirks, and
// typed-error pattern matching with both errors.Is and errors.As.
package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/foods"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
	fserrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
)

// newServer returns an httptest server that emulates both the FatSecret OAuth2
// token endpoint and the foods.search REST endpoint, including FatSecret's habit
// of returning HTTP 200 with an error envelope and numbers encoded as strings.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// OAuth2 token endpoint: client_credentials grant.
	mux.HandleFunc("/connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token-abc",
			"token_type":   "Bearer",
			"expires_in":   86400,
		})
	})

	// foods.search.v3: returns success, or a FatSecret error envelope (HTTP 200)
	// depending on the search expression — exercising the error-on-200 path.
	mux.HandleFunc("/rest/foods/search/v3", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("search_expression") {
		case "boom_rate":
			// Rate-limit: FatSecret error code 11, returned with HTTP 200.
			_, _ = w.Write([]byte(`{"error":{"code":"11","message":"too many requests"}}`))
		case "boom_perm":
			// Missing scope: FatSecret error code 14.
			_, _ = w.Write([]byte(`{"error":{"code":"14","message":"missing required scope: premier"}}`))
		default:
			// Success. Note: numbers are JSON strings, and "food" is a single
			// object (not an array) to exercise FlexSlice single-object decoding.
			_, _ = w.Write([]byte(`{"foods_search":{` +
				`"max_results":"1","page_number":"0","total_results":"1",` +
				`"food":{"food_id":"33691","food_name":"Banana","food_type":"Generic"}` +
				`}}`))
		}
	})

	return httptest.NewServer(mux)
}

func newClient(t *testing.T, baseURL string, hc *http.Client) client.Client {
	t.Helper()
	authn := auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
		ClientID:     "id",
		ClientSecret: "secret",
		TokenURL:     baseURL + "/connect/token",
		HTTPClient:   hc,
	})
	c, err := client.NewClient(client.Options{
		Authenticator: authn,
		BaseURL:       baseURL,
		Timeout:       5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// TestEndToEnd_TypedCallAndDecode proves a caller can construct the client,
// authenticate via OAuth2, make a typed call, and receive cleanly decoded Go
// types despite the API returning string-encoded numbers and a single-object
// (rather than array) result set.
func TestEndToEnd_TypedCallAndDecode(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL, srv.Client())
	defer func() { _ = c.Close() }()

	svc := foods.New(c)
	expr := "banana"
	result, err := svc.Search(context.Background(), foods.SearchRequest{SearchExpression: &expr})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if result.TotalResults.Int64() != 1 {
		t.Errorf("TotalResults = %d, want 1", result.TotalResults.Int64())
	}
	items := result.Food.Items()
	if len(items) != 1 {
		t.Fatalf("FlexSlice decoded %d foods, want 1 (single-object collapse)", len(items))
	}
	if items[0].FoodID.Int64() != 33691 {
		t.Errorf("FoodID = %d, want 33691 (string-number decode)", items[0].FoodID.Int64())
	}
	if items[0].FoodName != "Banana" {
		t.Errorf("FoodName = %q, want %q", items[0].FoodName, "Banana")
	}
}

// TestEndToEnd_RateLimitError proves a HTTP-200 FatSecret error envelope maps to
// a typed error matchable with both errors.Is (sentinel) and errors.As (detail).
func TestEndToEnd_RateLimitError(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL, srv.Client())
	defer func() { _ = c.Close() }()

	expr := "boom_rate"
	_, err := foods.New(c).Search(context.Background(), foods.SearchRequest{SearchExpression: &expr})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, fserrors.ErrRateLimited) {
		t.Errorf("errors.Is(err, ErrRateLimited) = false; err = %v", err)
	}
	var rle *fserrors.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("errors.As(*RateLimitError) = false; err = %v", err)
	}
}

// TestEndToEnd_PermissionError proves the scope-missing error (code 14) surfaces
// as a PermissionError carrying the extracted scope, matchable via errors.As.
func TestEndToEnd_PermissionError(t *testing.T) {
	t.Parallel()
	srv := newServer(t)
	defer srv.Close()

	c := newClient(t, srv.URL, srv.Client())
	defer func() { _ = c.Close() }()

	expr := "boom_perm"
	_, err := foods.New(c).Search(context.Background(), foods.SearchRequest{SearchExpression: &expr})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var pe *fserrors.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As(*PermissionError) = false; err = %v", err)
	}
	if pe.MissingScope == "" {
		t.Errorf("PermissionError.MissingScope is empty; want extracted scope")
	}
}
