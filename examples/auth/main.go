// Package main demonstrates both authentication strategies supported by the
// FatSecret Go client.
//
//  1. OAuth 2.0 client-credentials (public data) — used for most read endpoints
//     such as foods.Search and food.Get. The authenticator fetches a bearer token
//     automatically and refreshes it before expiry.
//
//  2. OAuth 1.0a profile-delegation (user-specific data) — required for endpoints
//     in namespaces such as food_entries, exercise_entries, and weight. The caller
//     must first obtain a per-user auth_token and auth_secret from FatSecret's
//     profile.create endpoint, then supply those credentials here.
//
// Environment variables required:
//
//	FATSECRET_CLIENT_ID     - OAuth 2.0 client ID (also used as the OAuth 1.0a consumer key)
//	FATSECRET_CLIENT_SECRET - OAuth 2.0 client secret (also used as the OAuth 1.0a consumer secret)
//
// Optional (needed only for the OAuth 1.0a delegation block to print its output):
//
//	FATSECRET_AUTH_TOKEN  - per-user OAuth 1.0a access token
//	FATSECRET_AUTH_SECRET - per-user OAuth 1.0a access token secret
//
// Run:
//
//	FATSECRET_CLIENT_ID=xxx FATSECRET_CLIENT_SECRET=yyy go run ./examples/auth
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
	clientID := os.Getenv("FATSECRET_CLIENT_ID")
	clientSecret := os.Getenv("FATSECRET_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "usage: FATSECRET_CLIENT_ID=<id> FATSECRET_CLIENT_SECRET=<secret> go run ./examples/auth")
		fmt.Fprintln(os.Stderr, "Both FATSECRET_CLIENT_ID and FATSECRET_CLIENT_SECRET must be set.")
		return
	}

	// --- Part 1: OAuth 2.0 client-credentials (public data) ---
	//
	// NewOAuth2ClientCredentials returns an auth.Authenticator. The token is
	// fetched lazily on the first request and refreshed 5 minutes before expiry.
	oauth2Auth := auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		// Scopes: nil requests the full scope set available to the client.
		// Supply []string{"basic"} or []string{"premier"} to restrict the grant.
	})

	c, err := client.NewClient(client.Options{
		Authenticator: oauth2Auth,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient (oauth2): %v\n", err)
		return
	}
	defer c.Close() //nolint:errcheck

	// Use the OAuth 2.0 client to search for public food data.
	expr := "apple"
	maxResults := types.APIInt(3)
	result, err := foods.New(c).Search(context.Background(), foods.SearchRequest{
		SearchExpression: &expr,
		MaxResults:       &maxResults,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "foods.Search (oauth2): %v\n", err)
		return
	}
	fmt.Printf("[OAuth 2.0] foods.Search %q → %d total results\n",
		expr, int64(result.TotalResults))
	for _, f := range result.Food {
		fmt.Printf("  %d  %s\n", int64(f.FoodID), f.FoodName)
	}

	// --- Part 2: OAuth 1.0a profile-delegation (user-specific data) ---
	//
	// To access user data endpoints (food_entries, exercise_entries, weight, etc.)
	// the caller must hold a per-user access token obtained from FatSecret's
	// profile.create endpoint. Those credentials are supplied via environment
	// variables here to avoid hard-coding secrets.
	//
	// NewOAuth1ProfileDelegation returns an auth.OAuth1RequestSigner (which
	// also satisfies auth.Authenticator) so it can be passed directly to
	// client.Options.Authenticator. When any required field is empty, the
	// constructor returns an InvalidAuthenticator that surfaces a clear error
	// on the first request rather than panicking.
	authToken := os.Getenv("FATSECRET_AUTH_TOKEN")
	authSecret := os.Getenv("FATSECRET_AUTH_SECRET")

	if authToken == "" || authSecret == "" {
		fmt.Println("\n[OAuth 1.0a] FATSECRET_AUTH_TOKEN and FATSECRET_AUTH_SECRET not set.")
		fmt.Println("  Set those variables to demonstrate per-user food-entry access.")
		fmt.Println("  Example construction shown below (no network call made).")

		// Demonstrate construction without making any network call.
		_ = auth.NewOAuth1ProfileDelegation(auth.OAuth1ProfileConfig{ // #nosec G101 -- env-var names in example, not hardcoded secrets
			ConsumerKey:    clientID,
			ConsumerSecret: clientSecret,
			AuthToken:      "<user-auth-token>",
			AuthSecret:     "<user-auth-secret>",
		})
		fmt.Println("  auth.NewOAuth1ProfileDelegation constructed successfully (validation placeholder credentials).")
		return
	}

	// Build the OAuth 1.0a delegation authenticator for the authenticated user.
	oauth1Auth := auth.NewOAuth1ProfileDelegation(auth.OAuth1ProfileConfig{
		ConsumerKey:    clientID,
		ConsumerSecret: clientSecret,
		AuthToken:      authToken,
		AuthSecret:     authSecret,
	})

	userClient, err := client.NewClient(client.Options{
		Authenticator: oauth1Auth,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient (oauth1): %v\n", err)
		return
	}
	defer userClient.Close() //nolint:errcheck

	fmt.Println("\n[OAuth 1.0a] client constructed for delegated user access.")
	fmt.Println("  Use food_entries.New(userClient).Get(ctx, food_entries.GetRequest{...})")
	fmt.Println("  to retrieve the authenticated user's food diary entries.")
}
