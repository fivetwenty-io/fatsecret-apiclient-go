package http

import (
	"bytes"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/cache"
)

// CacheMiddleware returns a Middleware that caches GET response bodies keyed by
// a canonical representation of the request URL and sorted query parameters. Only
// GET requests are cached; all other methods pass through without cache interaction.
// On a cache hit the rest of the chain is not called; a synthetic *http.Response
// is returned with status 200 and the cached body. On a miss the request proceeds
// normally and a successful response body is stored in the cache with the given ttl.
//
// This middleware must be positioned first in the chain (outermost) so it runs
// before authentication and retry overhead on hits.
func CacheMiddleware(c cache.Cache, ttl time.Duration) Middleware {
	return func(next RoundTripFunc) RoundTripFunc {
		return func(req *http.Request) (*http.Response, error) {
			// Only cache GET requests.
			if req.Method != http.MethodGet {
				return next(req)
			}

			key := cacheKey(req)

			// Cache hit: return synthetic response without calling downstream.
			if data, ok := c.Get(key); ok {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK (cached)",
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(data)),
					Request:    req,
				}, nil
			}

			// Cache miss: send the request.
			resp, err := next(req)
			if err != nil {
				return resp, err
			}

			// Only cache successful responses.
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return resp, nil
			}

			// Buffer the response body so we can both cache it and give it to
			// the caller.
			if resp.Body != nil {
				body, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr == nil && len(body) > 0 {
					c.Set(key, body, ttl)
				}
				// Restore body for the caller regardless of caching outcome.
				resp.Body = io.NopCloser(bytes.NewReader(body))
			}

			return resp, nil
		}
	}
}

// cacheKey builds a deterministic cache key for a GET request. The key format
// is "GET:<base-url>?<sorted-params>" where base-url has no query component and
// params are sorted alphabetically by key then value for canonicality.
func cacheKey(req *http.Request) string {
	base := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path

	params := req.URL.Query()
	if len(params) == 0 {
		return "GET:" + base
	}

	// Sort keys for deterministic ordering.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("GET:")
	sb.WriteString(base)
	sb.WriteByte('?')

	for i, k := range keys {
		vals := params[k]
		sort.Strings(vals)
		for j, v := range vals {
			if i > 0 || j > 0 {
				sb.WriteByte('&')
			}
			sb.WriteString(k)
			sb.WriteByte('=')
			sb.WriteString(v)
		}
	}

	return sb.String()
}
