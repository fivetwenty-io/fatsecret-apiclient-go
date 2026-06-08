// Package main demonstrates response caching in the FatSecret Go client using the
// built-in in-memory LRU cache.
//
// When a cache.Cache is provided in Options, the client middleware intercepts
// successful GET responses, stores them keyed by URL+params, and returns the
// cached bytes on subsequent identical requests without touching the network or
// re-running authentication.
//
// This example calls food.Get twice with the same food ID. The first call hits
// the network; the second call is served from cache. Because the cache operates
// inside the middleware stack, the service layer (food.New(c).Get) is unaware of
// whether the response came from the network or the cache.
//
// Environment variables required:
//
//	FATSECRET_CLIENT_ID     - OAuth 2.0 client ID issued by FatSecret
//	FATSECRET_CLIENT_SECRET - OAuth 2.0 client secret issued by FatSecret
//
// Run:
//
//	FATSECRET_CLIENT_ID=xxx FATSECRET_CLIENT_SECRET=yyy go run ./examples/caching
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/food"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/cache"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

func main() {
	clientID := os.Getenv("FATSECRET_CLIENT_ID")
	clientSecret := os.Getenv("FATSECRET_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "usage: FATSECRET_CLIENT_ID=<id> FATSECRET_CLIENT_SECRET=<secret> go run ./examples/caching")
		fmt.Fprintln(os.Stderr, "Both FATSECRET_CLIENT_ID and FATSECRET_CLIENT_SECRET must be set.")
		return
	}

	// Create a bounded LRU in-memory cache with room for 100 entries. Each entry
	// is keyed by the full request URL including query parameters, so distinct
	// food IDs are cached independently. The least-recently-used entry is evicted
	// when the cache is full.
	memCache := cache.NewMemoryCache(100)

	c, err := client.NewClient(client.Options{
		Authenticator: auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}),
		Cache:    memCache,
		CacheTTL: 5 * time.Minute, // cached responses remain valid for 5 minutes
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient: %v\n", err)
		return
	}
	defer c.Close() //nolint:errcheck

	svc := food.New(c)
	ctx := context.Background()

	// Food ID 35718 is "Chicken Breast" on FatSecret (a stable generic entry).
	foodID := types.APIInt(35718)
	req := food.GetRequest{FoodID: &foodID}

	// First call: cache miss — the client fetches from the FatSecret API.
	fmt.Println("First call (cache miss — network request)...")
	start1 := time.Now()
	f1, err := svc.Get(ctx, req)
	elapsed1 := time.Since(start1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "food.Get (first): %v\n", err)
		return
	}
	fmt.Printf("  %s  (%v)\n", f1.FoodName, elapsed1.Round(time.Millisecond))

	// Second call: cache hit — the client returns the stored bytes immediately,
	// skipping authentication, the network, and retry logic.
	fmt.Println("Second call (cache hit — served from memory)...")
	start2 := time.Now()
	f2, err := svc.Get(ctx, req)
	elapsed2 := time.Since(start2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "food.Get (second): %v\n", err)
		return
	}
	fmt.Printf("  %s  (%v)\n", f2.FoodName, elapsed2.Round(time.Millisecond))

	fmt.Printf("\nCache acceleration: first=%v  second=%v  entries_cached=%d\n",
		elapsed1.Round(time.Millisecond), elapsed2.Round(time.Millisecond), memCache.Len())
}
