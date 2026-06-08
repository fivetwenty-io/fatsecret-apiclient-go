# fatsecret-apiclient-go

A production-ready Go client for the [FatSecret REST API](https://platform.fatsecret.com/api/Default.aspx?screen=rapiref2).

[![Go Reference](https://pkg.go.dev/badge/github.com/fivetwenty-io/fatsecret-apiclient-go.svg)](https://pkg.go.dev/github.com/fivetwenty-io/fatsecret-apiclient-go)
[![CI](https://github.com/fivetwenty-io/fatsecret-apiclient-go/actions/workflows/ci.yml/badge.svg)](https://github.com/fivetwenty-io/fatsecret-apiclient-go/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/fivetwenty-io/fatsecret-apiclient-go)](https://goreportcard.com/report/github.com/fivetwenty-io/fatsecret-apiclient-go)

## Features

- **Three authentication strategies** — OAuth 2.0 client-credentials for server-to-server access; OAuth 1.0a signed (two-legged) for profile management endpoints; OAuth 1.0a profile-delegation for per-user diary operations.
- **Tolerant decoding** — `pkg/types` handles every FatSecret wire quirk: numbers as quoted strings, booleans as `"0"`/`"1"`, and single-element arrays collapsed to bare objects.
- **Typed errors** — sentinel errors (`ErrUnauthorized`, `ErrRateLimited`, etc.) and structured types (`PermissionError`, `ParameterError`) for precise error handling.
- **Middleware chain** — cache, auth, retry, logging, and metrics compose in a single configurable pipeline.
- **Response caching** — pluggable `Cache` interface with a built-in bounded LRU memory store and TTL expiry.
- **Metrics collection** — pluggable `Collector` interface with a dependency-free `AtomicCollector` backed by `sync/atomic`.
- **Batch fan-out** — `pkg/batch` executes a function concurrently over a slice with bounded concurrency; results are returned in input order.
- **Generated 16-namespace surface** — all FatSecret namespaces (foods, food, exercises, profiles, recipes, and more) are generated from the API specification and regenerable with `make generate`.

## Installation

```
go get github.com/fivetwenty-io/fatsecret-apiclient-go
```

Requires Go 1.21 or later.

## Quick Start

The example below uses the `foods` namespace to search for foods. All namespaces follow the same pattern: construct a `client.Client` once, pass it to the namespace `New` constructor, then call typed methods.

```go
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
    // Build the OAuth 2.0 client-credentials authenticator.
    authenticator := auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
        ClientID:     os.Getenv("FATSECRET_CLIENT_ID"),
        ClientSecret: os.Getenv("FATSECRET_CLIENT_SECRET"),
    })

    // Construct the low-level HTTP client.
    c, err := client.NewClient(client.Options{
        Authenticator: authenticator,
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    defer c.Close()

    // Create the foods service bound to the client.
    svc := foods.New(c)

    // Build the search request.
    expr := "chicken breast"
    maxResults := types.APIInt(5)
    req := foods.SearchRequest{
        SearchExpression: &expr,
        MaxResults:       &maxResults,
    }

    result, err := svc.Search(context.Background(), req)
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }

    fmt.Printf("Total results: %d\n", int64(result.TotalResults))
    for _, f := range result.Food {
        fmt.Printf("  [%d] %s (%s)\n", int64(f.FoodID), f.FoodName, f.FoodType)
    }
}
```

A runnable version with environment-variable handling is at [examples/basic/](examples/basic/).

## Authentication

| Strategy | Use case | Namespaces |
|---|---|---|
| `auth.NewOAuth2ClientCredentials` | Server-to-server access to public food and exercise data | foods, food, exercises, recipes, recipe_types, food_brands, food_categories, food_sub_categories, feedback |
| `auth.NewOAuth1Signed` | Profile creation and auth-token retrieval (two-legged) | profile (create, get_auth) |
| `auth.NewOAuth1ProfileDelegation` | Per-user diary operations using a delegated auth token | food_entries, exercise_entries, saved_meals, weight, profile (delegated methods) |

Pass the chosen authenticator to `client.Options.Authenticator`. The auth middleware calls the authenticator transparently on every request, proactively refreshing OAuth 2.0 tokens before they expire.

## Examples

| Directory | Demonstrates |
|---|---|
| [examples/basic/](examples/basic/) | OAuth 2.0 client setup and food search |
| [examples/auth/](examples/auth/) | OAuth 1.0a signed and delegation strategies |
| [examples/caching/](examples/caching/) | In-memory LRU response cache |
| [examples/logging/](examples/logging/) | Structured logging via `pkg/zapadapter` |
| [examples/metrics/](examples/metrics/) | Request metrics with `AtomicCollector` |

## Generated API

The 16 namespace packages under `pkg/api/` are generated from the API specification in `spec/` using the `cmd/fsgen` generator. Do not edit generated files directly.

To regenerate after updating the spec or generator templates:

```
make generate
```

## License

See [LICENSE](LICENSE).
