// Package main demonstrates structured request/response logging in the FatSecret
// Go client using the standard-library slog adapter.
//
// The client supports two logger adapters:
//
//  1. client.NewSlogLogger — wraps a *slog.Logger; no extra dependencies.
//  2. zapadapter.NewZapLogger — wraps a *zap.Logger from go.uber.org/zap.
//     Import "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/zapadapter"
//     and pass zapadapter.NewZapLogger(zapLogger) to Options.Logger.
//
// LogConfig controls which details are included in each log entry:
//   - Enabled:    master switch; set to true to emit logs.
//   - BodySample: include a sample of request/response bodies.
//   - MaxBytes:   cap the number of body bytes logged per entry (0 = none when
//     BodySample is true).
//
// Environment variables required:
//
//	FATSECRET_CLIENT_ID     - OAuth 2.0 client ID issued by FatSecret
//	FATSECRET_CLIENT_SECRET - OAuth 2.0 client secret issued by FatSecret
//
// Run:
//
//	FATSECRET_CLIENT_ID=xxx FATSECRET_CLIENT_SECRET=yyy go run ./examples/logging
package main

import (
	"context"
	"fmt"
	"log/slog"
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
		fmt.Fprintln(os.Stderr, "usage: FATSECRET_CLIENT_ID=<id> FATSECRET_CLIENT_SECRET=<secret> go run ./examples/logging")
		fmt.Fprintln(os.Stderr, "Both FATSECRET_CLIENT_ID and FATSECRET_CLIENT_SECRET must be set.")
		return
	}

	// Create a structured slog logger that writes JSON lines to stderr.
	// client.NewSlogLogger wraps any *slog.Logger in the Logger interface so the
	// HTTP transport can emit structured request/response log entries.
	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := client.NewSlogLogger(slog.New(jsonHandler))

	// LogConfig controls the verbosity of each log entry.
	//   Enabled:    true  — emit a log entry for every HTTP round-trip.
	//   BodySample: true  — include a truncated body sample in each entry.
	//   MaxBytes:   512   — limit body samples to 512 bytes to avoid noise.
	logCfg := client.LogConfig{
		Enabled:    true,
		BodySample: true,
		MaxBytes:   512,
	}

	c, err := client.NewClient(client.Options{
		Authenticator: auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}),
		Logger:    logger,
		LogConfig: logCfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "client.NewClient: %v\n", err)
		return
	}
	defer c.Close() //nolint:errcheck

	// Each HTTP round-trip now emits a structured JSON log line to stderr containing
	// the method, path, status code, duration, and (because BodySample is true) up
	// to 512 bytes of the response body.
	expr := "broccoli"
	maxResults := types.APIInt(3)
	result, err := foods.New(c).Search(context.Background(), foods.SearchRequest{
		SearchExpression: &expr,
		MaxResults:       &maxResults,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "foods.Search: %v\n", err)
		return
	}

	fmt.Printf("Search returned %d results for %q.\n", len(result.Food), expr)

	// --- Zap adapter (comment-only demonstration) ---
	//
	// To use go.uber.org/zap instead of slog:
	//
	//   import "go.uber.org/zap"
	//   import "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/zapadapter"
	//
	//   zapLog, _ := zap.NewProduction()
	//   defer zapLog.Sync()
	//
	//   c, _ := client.NewClient(client.Options{
	//       Authenticator: ...,
	//       Logger:        zapadapter.NewZapLogger(zapLog),
	//       LogConfig:     client.LogConfig{Enabled: true, BodySample: true, MaxBytes: 512},
	//   })
	//
	// pkg/zapadapter imports pkg/client; pkg/client does NOT import pkg/zapadapter,
	// so importing pkg/zapadapter only links the zap dependency when your binary
	// actually uses it.
}
