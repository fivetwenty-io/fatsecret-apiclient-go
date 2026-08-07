# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.0.3] — 2026-08-07

### Fixed

- **Native endpoints require a JSON request body** — `natural-language-processing/v1` and `image-recognition/v2` accept only `Content-Type: application/json`, unlike every other FatSecret endpoint, which takes `application/x-www-form-urlencoded`. Spec methods gain `body_encoding: json`, which makes the generator emit `ToJSONBody()` on the request type and pass the result as `client.Request.Body`. Values keep their JSON-native types, so `include_food_data` goes on the wire as the boolean `true` rather than the `"1"` the form encoder produces.
- **Native endpoints reject `format=json`** — the same two endpoints always answer JSON and fail when the parameter is present. `client.Request` gains `OmitFormatParam` to suppress the otherwise mandatory parameter, and generated code sets it for those endpoints. A request with no remaining parameters no longer carries a bare trailing `?`.

Either fault alone made the endpoints answer HTTP 200 with error code 1, "An unknown error occurred: 'please try again later'", which is indistinguishable from a transient upstream outage. Both had to be corrected before image recognition returned data.

### Added

- **`client.Request.OmitFormatParam`** — opt-out of the automatic `format=json` parameter. Additive; existing callers are unaffected.

## [0.0.2] — 2026-06-16

### Fixed

- **FatSecret error envelopes are surfaced** — the API reports failures as HTTP 200 with an `{"error":{code,message}}` body. `errors.FromResponse` now detects the envelope and dispatches to the typed error hierarchy, wired into `client.Do`, so real causes propagate instead of appearing as generic decode failures.
- **Singular-key list wrappers are flattened** — search results nest under `results.food` and servings under `servings.serving`. Searches previously returned empty and foods parsed with zero nutrients.
- **Barcode lookup rewritten as the documented two-call composite** — `food.find_id_for_barcode` resolves an ID, then `food.get.v4` fetches it; `food_id` 0 maps to `ErrNotFound`. Lookups previously failed with code 101.

### Changed

- **Response and barcode shapes moved into the spec and generator** — `nested: {outer, inner}` declares list wrappers, and methods gain `method_param` (method-style calls via `/rest/server.api`) and `composite: barcode_lookup`. The former hand-written overrides are deleted, so `make generate` no longer reintroduces the bugs and `make verify-generated` passes.

## [0.0.1] — 2026-06-08

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
