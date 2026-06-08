// Package main demonstrates the simplest usage of the FatSecret Go client: create
// a client with OAuth 2.0 client-credentials authentication and search for foods.
//
// Environment variables required:
//
//	FATSECRET_CLIENT_ID     - OAuth 2.0 client ID issued by FatSecret
//	FATSECRET_CLIENT_SECRET - OAuth 2.0 client secret issued by FatSecret
//
// Run:
//
//	FATSECRET_CLIENT_ID=xxx FATSECRET_CLIENT_SECRET=yyy go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/foods"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

func main() {
	// Read credentials from environment variables. Never hard-code secrets.
	clientID := os.Getenv("FATSECRET_CLIENT_ID")
	clientSecret := os.Getenv("FATSECRET_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "usage: FATSECRET_CLIENT_ID=<id> FATSECRET_CLIENT_SECRET=<secret> go run ./examples/basic")
		fmt.Fprintln(os.Stderr, "Both FATSECRET_CLIENT_ID and FATSECRET_CLIENT_SECRET must be set.")
		return
	}

	// Build the OAuth 2.0 client-credentials authenticator. The authenticator
	// fetches and transparently refreshes the access token as needed.
	authenticator := auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})

	// Construct the low-level HTTP client with the chosen authenticator. All
	// other Options fields use safe production defaults (30 s timeout, 2 retries).
	c, err := client.NewClient(client.Options{
		Authenticator: authenticator,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient: %v\n", err)
		return
	}
	defer c.Close() //nolint:errcheck

	// Create the foods service bound to the client. The service wraps the low-level
	// client and provides typed request/response structs for every foods endpoint.
	svc := foods.New(c)

	// Build the search request. SearchExpression is a pointer field; passing a
	// pointer to a local string variable keeps the field non-nil.
	expr := "chicken breast"
	maxResults := types.APIInt(5)
	req := foods.SearchRequest{
		SearchExpression: &expr,
		MaxResults:       &maxResults,
	}

	ctx := context.Background()
	result, err := svc.Search(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "foods.Search: %v\n", err)
		return
	}

	fmt.Printf("Search for %q returned %d total results (showing up to %d):\n",
		expr, int64(result.TotalResults), int64(result.MaxResults))
	for _, f := range result.Food {
		brand := ""
		if f.BrandName != nil {
			brand = " (" + *f.BrandName + ")"
		}
		fmt.Printf("  [%d] %s%s — %s\n", int64(f.FoodID), f.FoodName, brand, f.FoodType)
	}
}
