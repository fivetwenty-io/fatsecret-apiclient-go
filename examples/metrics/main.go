// Package main demonstrates observability collection in the FatSecret Go client
// using the built-in lock-free AtomicCollector.
//
// When a metrics.Collector is provided in Options.Metrics, the client middleware
// records counters and a duration histogram for every HTTP round-trip:
//
//   - RequestsTotal   — total outbound request attempts (including retries)
//   - FailuresTotal   — requests that completed with a non-2xx status or transport error
//   - ActiveConnections — current in-flight request count (gauge, should be 0 after all calls)
//   - BytesSent / BytesRecv — cumulative body bytes
//   - DurationHistogram — per-bucket observation counts (bucket boundaries in ms)
//
// After making a few calls this example calls Snapshot() to print the collected
// metrics. In production you would export the snapshot to Prometheus, StatsD, or
// any other observability backend.
//
// Environment variables required:
//
//	FATSECRET_CLIENT_ID     - OAuth 2.0 client ID issued by FatSecret
//	FATSECRET_CLIENT_SECRET - OAuth 2.0 client secret issued by FatSecret
//
// Run:
//
//	FATSECRET_CLIENT_ID=xxx FATSECRET_CLIENT_SECRET=yyy go run ./examples/metrics
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/foods"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

func main() {
	clientID := os.Getenv("FATSECRET_CLIENT_ID")
	clientSecret := os.Getenv("FATSECRET_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "usage: FATSECRET_CLIENT_ID=<id> FATSECRET_CLIENT_SECRET=<secret> go run ./examples/metrics")
		fmt.Fprintln(os.Stderr, "Both FATSECRET_CLIENT_ID and FATSECRET_CLIENT_SECRET must be set.")
		return
	}

	// NewAtomicCollector returns a lock-free, dependency-free Collector backed by
	// sync/atomic counters. All methods are safe for concurrent use. The collector
	// satisfies the metrics.Collector interface accepted by client.Options.Metrics.
	col := metrics.NewAtomicCollector()

	c, err := client.NewClient(client.Options{
		Authenticator: auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}),
		Metrics: col,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient: %v\n", err)
		return
	}
	defer c.Close() //nolint:errcheck

	svc := foods.New(c)
	ctx := context.Background()

	// Make several distinct searches to accumulate metrics across multiple calls.
	queries := []string{"chicken breast", "brown rice", "avocado"}
	maxResults := types.APIInt(3)
	for _, q := range queries {
		expr := q
		result, err := svc.Search(ctx, foods.SearchRequest{
			SearchExpression: &expr,
			MaxResults:       &maxResults,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "foods.Search(%q): %v\n", q, err)
			// Continue so partial metrics are still printed.
			continue
		}
		fmt.Printf("Search %q → %d results\n", q, int64(result.TotalResults))
	}

	// Snapshot reads all counters atomically. The returned value is a plain struct
	// with no synchronisation requirements; fields are safe to read directly.
	snap := col.Snapshot()

	fmt.Println("\n--- Metrics Snapshot ---")
	fmt.Printf("RequestsTotal:     %d\n", snap.RequestsTotal)
	fmt.Printf("FailuresTotal:     %d\n", snap.FailuresTotal)
	fmt.Printf("ActiveConnections: %d\n", snap.ActiveConnections)
	fmt.Printf("BytesSent:         %d bytes\n", snap.BytesSent)
	fmt.Printf("BytesRecv:         %d bytes\n", snap.BytesRecv)

	fmt.Println("\nDuration Histogram (ms buckets):")
	for _, b := range snap.DurationHistogram {
		if b.Count == 0 {
			continue
		}
		label := fmt.Sprintf("≤%dms", b.UpperMs)
		if b.UpperMs == -1 {
			label = "+Inf"
		}
		fmt.Printf("  %-10s  %d observations\n", label, b.Count)
	}
}
