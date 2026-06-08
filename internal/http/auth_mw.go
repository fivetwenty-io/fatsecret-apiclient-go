package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
)

// AuthMiddleware returns a Middleware that injects authentication headers before
// every request. If the Authenticator implements auth.OAuth1RequestSigner, the
// more specific GetHeadersForRequest method is called with the base URL
// (scheme + host + path, no query string) and the request query parameters so
// that the OAuth 1.0a signature covers all request parameters. Otherwise
// GetHeaders is called.
//
// On a 401 response the middleware calls Refresh once and immediately re-sends
// the request with fresh headers. A second consecutive 401 is returned to the
// caller as-is; the response handler converts it to an AuthenticationError.
//
// AuthMiddleware rejects a nil context and returns an error immediately.
func AuthMiddleware(a auth.Authenticator) Middleware {
	return func(next RoundTripFunc) RoundTripFunc {
		return func(req *http.Request) (*http.Response, error) {
			ctx := req.Context()
			if ctx == nil {
				return nil, errors.New("auth: nil context is not allowed")
			}

			// Inject auth headers for the initial attempt.
			if err := applyAuthHeaders(ctx, a, req); err != nil {
				return nil, err
			}

			resp, err := next(req)
			if err != nil {
				return resp, err
			}

			// On 401, refresh credentials and retry exactly once.
			if resp.StatusCode == http.StatusUnauthorized {
				// Drain the body so the connection can be reused.
				if resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}

				if refreshErr := a.Refresh(ctx); refreshErr != nil {
					return nil, refreshErr
				}

				// Rewind request body for replay.
				if req.GetBody != nil {
					body, gbErr := req.GetBody()
					if gbErr != nil {
						return nil, gbErr
					}
					req.Body = body
				}

				// Inject fresh headers and retry.
				if err := applyAuthHeaders(ctx, a, req); err != nil {
					return nil, err
				}

				resp, err = next(req)
			}

			return resp, err
		}
	}
}

// applyAuthHeaders obtains headers from the authenticator and sets them on req.
// If a implements auth.OAuth1RequestSigner, GetHeadersForRequest is called with
// the base URL and request query parameters.
func applyAuthHeaders(ctx context.Context, a auth.Authenticator, req *http.Request) error {
	var headers map[string]string
	var err error

	if signer, ok := a.(auth.OAuth1RequestSigner); ok {
		base := baseURL(req)
		params := paramsFrom(req)
		headers, err = signer.GetHeadersForRequest(ctx, req.Method, base, params)
	} else {
		headers, err = a.GetHeaders(ctx)
	}
	if err != nil {
		return err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return nil
}

// baseURL returns the scheme + host + path of req without query string or fragment.
func baseURL(req *http.Request) string {
	u := *req.URL // shallow copy
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// paramsFrom returns the query parameters from the request URL as url.Values.
// Form body parameters are not included here; OAuth1 signing covers query params.
func paramsFrom(req *http.Request) url.Values {
	return req.URL.Query()
}
