# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **HTTP client** (`pkg/client`) — `Client` interface with `Do`, `Auth`, and `Close`; `NewClient` constructor with `Options` covering base URL, timeout, retry policy, TLS mode, structured logging, hooks, metrics, and caching.
- **OAuth 2.0 client-credentials authentication** (`pkg/auth`) — `OAuth2ClientCredentials` strategy with transparent token fetch and proactive refresh; safe for concurrent use.
- **OAuth 1.0a signed authentication** (`pkg/auth`) — `OAuth1Signed` two-legged strategy for profile management endpoints.
- **OAuth 1.0a profile-delegation authentication** (`pkg/auth`) — `OAuth1ProfileDelegation` per-user strategy using a consumer key/secret plus a delegated auth token and secret.
- **Tolerant scalar types** (`pkg/types`) — `APIInt`, `APIFloat`, `APIBool`, `APIDate`, and `FlexSlice` implement `encoding/json.Unmarshaler` to handle FatSecret wire quirks: numbers as quoted strings, booleans as `"0"`/`"1"`, ternary allergen values, and single-element arrays collapsed to bare objects.
- **Typed error hierarchy** (`pkg/errors`) — sentinel errors (`ErrUnauthorized`, `ErrForbidden`, `ErrRateLimited`, `ErrServer`) and structured types (`PermissionError`, `ParameterError`) with `errors.Is` / `errors.As` support; `DispatchByFatSecretCode` and `DispatchByStatus` for mapping raw error codes to typed errors.
- **Middleware chain** — composable pipeline: cache → auth → retry → logging/hooks/metrics; each layer is independently configurable via `client.Options`.
- **Response cache** (`pkg/cache`) — `Cache` interface, `NoopCache`, and `MemoryCache` (bounded LRU with TTL expiry).
- **Metrics collection** (`pkg/metrics`) — `Collector` interface, `NoopCollector`, and `AtomicCollector` backed by `sync/atomic` with a fixed-boundary duration histogram.
- **Batch fan-out** (`pkg/batch`) — generic `Run` helper for concurrent execution over a slice with bounded parallelism; results preserve input order; per-item errors do not cancel other items.
- **Zap logging adapter** (`pkg/zapadapter`) — `ZapLogger` bridges `go.uber.org/zap` to the `client.Logger` interface.
- **Compatibility matrix** (`pkg/compatibility`) — generated `Matrix` catalog of all API methods with version, deprecation, auth tier, and scope metadata; `Checker` provides O(1) indexed lookup.
- **Generated 16-namespace API surface** (`pkg/api/`) — typed `Service` interfaces and `New` constructors for all FatSecret namespaces: `exercise_entries`, `exercises`, `feedback`, `food`, `food_brands`, `food_categories`, `food_entries`, `food_sub_categories`, `foods`, `native`, `profile`, `recipe`, `recipe_types`, `recipes`, `saved_meals`, `weight`.
- **Code generator** (`cmd/fsgen`) — reads the YAML API specification in `spec/` and generates namespace packages, request/response types, and the compatibility matrix.
- **Examples** — runnable examples for OAuth 2.0 quick-start (`examples/basic/`), OAuth 1.0a strategies (`examples/auth/`), response caching (`examples/caching/`), structured logging (`examples/logging/`), and metrics (`examples/metrics/`).
