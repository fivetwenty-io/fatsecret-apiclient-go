// Package client is documented in doc.go.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	internalhttp "github.com/fivetwenty-io/fatsecret-apiclient-go/internal/http"
	internaltls "github.com/fivetwenty-io/fatsecret-apiclient-go/internal/tls"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/cache"
	fserrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

// Type aliases that re-export internal/http canonical types so consumers only
// need to import pkg/client. Type alias preserves full type identity:
// values produced by internal/http are directly assignment-compatible.

// Logger is the structured logging interface for the FatSecret transport layer.
// Implementations must be safe for concurrent use. Each method receives a
// human-readable message and a map of structured fields; callers do not
// guarantee field map immutability after the method returns.
type Logger = internalhttp.Logger

// Hook is a callback fired after every HTTP round-trip regardless of success or
// failure. Hooks must not block; dispatch long-running work to a goroutine inside
// the hook implementation.
type Hook = internalhttp.Hook

// HookEvent carries a point-in-time snapshot of a completed HTTP round-trip.
type HookEvent = internalhttp.HookEvent

// LogConfig controls which details the logging middleware includes in log output.
type LogConfig = internalhttp.LogConfig

// TLSMode controls how the transport verifies TLS connections to the FatSecret API.
// Use the TLSModeXxx constants to select a mode.
type TLSMode = internaltls.TLSMode

// TLS mode constants re-exported for consumer convenience so callers can write
// client.TLSModeFingerprint without importing internal/tls directly.
const (
	TLSModeNone        TLSMode = internaltls.TLSModeNone
	TLSModePeer        TLSMode = internaltls.TLSModePeer
	TLSModeHostname    TLSMode = internaltls.TLSModeHostname
	TLSModeFingerprint TLSMode = internaltls.TLSModeFingerprint
	TLSModeFull        TLSMode = internaltls.TLSModeFull
)

// Collector is re-exported from pkg/metrics for consumer convenience. Callers
// who interact only with pkg/client do not need to import pkg/metrics separately
// unless they want to use AtomicCollector or inspect Snapshot fields directly.
type Collector = metrics.Collector

// Cache is re-exported from pkg/cache for consumer convenience.
type Cache = cache.Cache

// Request is a transport-level request descriptor. Namespace packages build
// Request values and pass them to Client.Do; callers outside generated packages
// may also construct requests directly for custom API operations.
type Request struct {
	// Method is the HTTP method, e.g. "GET" or "POST". Required.
	Method string

	// Path is the URL path relative to Options.BaseURL, e.g. "/rest/server.api".
	// A leading slash is optional; Do normalises the combined URL.
	Path string

	// Params carries query-string parameters for GET requests and form-body
	// parameters for POST requests. The format=json parameter is added
	// automatically by Client.Do and must not be set by callers.
	Params url.Values

	// Body is the raw request body bytes. For FatSecret API calls this is
	// typically nil (params are encoded as form or query string). When non-nil,
	// Params are encoded into the query string and Body is used as-is.
	Body []byte

	// Headers is a map of request headers merged with defaults. Authorization
	// headers injected by the Authenticator take precedence over values set here.
	Headers map[string]string
}

// Response is the transport-level response returned by Client.Do.
// The Body field contains the raw JSON bytes; callers or generated namespace
// packages unmarshal Body into the appropriate typed struct.
type Response struct {
	// StatusCode is the HTTP response status code.
	StatusCode int

	// Body is the raw response body bytes, capped at internalhttp.DefaultMaxResponseBytes.
	Body []byte

	// Header contains the response HTTP headers.
	Header http.Header
}

// Client is the public interface for all FatSecret API interactions.
// Obtain a Client via NewClient. All methods honour context cancellation and
// deadlines at every network boundary.
type Client interface {
	// Do executes req and returns the raw HTTP response. The format=json parameter
	// is added automatically to every request. The response body is checked for
	// FatSecret error envelopes (HTTP 200 + error JSON) before being returned;
	// a non-nil error means the call failed at the transport or API level.
	Do(ctx context.Context, req *Request) (*Response, error)

	// Auth returns the active Authenticator for introspection or manual refresh.
	Auth() auth.Authenticator

	// Close drains idle keep-alive connections and releases internal resources.
	// Calling Close more than once is safe.
	Close() error
}

// clientImpl is the concrete Client implementation constructed by NewClient.
type clientImpl struct {
	authenticator auth.Authenticator
	baseURL       string
	transport     *internalhttp.Transport
	netTransport  *http.Transport // held for CloseIdleConnections in Close()
}

// Do implements Client.Do. It builds the full request URL, encodes Params as a
// query string (GET) or form body (POST), injects the mandatory format=json
// parameter, dispatches through the middleware chain, and returns a *Response.
func (c *clientImpl) Do(ctx context.Context, req *Request) (*Response, error) {
	if ctx == nil {
		return nil, fmt.Errorf("client: nil context is not allowed; use context.Background()")
	}
	if req == nil {
		return nil, fmt.Errorf("client: request must not be nil")
	}
	if req.Method == "" {
		return nil, fmt.Errorf("client: request Method must not be empty")
	}

	rawURL, body, contentType, err := c.buildRequestParts(req)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(req.Headers)+1)
	for k, v := range req.Headers {
		headers[k] = v
	}
	if contentType != "" {
		// Only set Content-Type when buildRequestParts determined one; do not
		// overwrite an explicit caller-supplied Content-Type header.
		if _, exists := headers["Content-Type"]; !exists {
			headers["Content-Type"] = contentType
		}
	}

	httpReq, err := internalhttp.BuildRequest(ctx, req.Method, rawURL, body, headers)
	if err != nil {
		return nil, fmt.Errorf("client: build request: %w", err)
	}

	httpResp, bodyBytes, err := c.transport.Do(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Body:       bodyBytes,
		Header:     httpResp.Header,
	}

	// FatSecret signals API-level failures with HTTP 200 + an
	// {"error":{"code","message"}} body, so the body must be inspected before
	// the response is treated as success. The Response is still returned
	// alongside the error so callers can inspect StatusCode/Body.
	if apiErr := fserrors.FromResponse(resp.StatusCode, resp.Body); apiErr != nil {
		return resp, apiErr
	}

	return resp, nil
}

// buildRequestParts constructs the full URL, encoded body, and Content-Type from
// req. The format=json parameter is injected to ensure FatSecret returns JSON
// instead of the default XML format.
func (c *clientImpl) buildRequestParts(req *Request) (rawURL string, body []byte, contentType string, err error) { //nolint:unparam // error return kept for future validation of URL/body construction
	base := strings.TrimRight(c.baseURL, "/")
	path := req.Path
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	rawURL = base + path

	// Clone params to avoid mutating the caller's map.
	params := make(url.Values, len(req.Params)+1)
	for k, vv := range req.Params {
		params[k] = append([]string(nil), vv...)
	}
	// format=json is mandatory — FatSecret defaults to XML without it.
	params.Set("format", "json")

	switch strings.ToUpper(req.Method) {
	case http.MethodGet, http.MethodHead, http.MethodDelete, http.MethodOptions:
		// Idempotent methods: all params (including format=json) go in the query string.
		rawURL = rawURL + "?" + params.Encode()

	default:
		// POST and PATCH: when Body is nil, encode Params as a form body.
		// When Body is set by the caller, Params go in the query string.
		if req.Body == nil {
			body = []byte(params.Encode())
			contentType = "application/x-www-form-urlencoded"
		} else {
			rawURL = rawURL + "?" + params.Encode()
			body = req.Body
			contentType = "application/json"
		}
	}

	return rawURL, body, contentType, nil
}

// Auth implements Client.Auth.
func (c *clientImpl) Auth() auth.Authenticator {
	return c.authenticator
}

// Close implements Client.Close. It drains idle keep-alive connections held
// by the underlying net/http transport, freeing file descriptors and goroutines.
func (c *clientImpl) Close() error {
	if c.netTransport != nil {
		c.netTransport.CloseIdleConnections()
	}
	return nil
}

// DoJSON is a package-level convenience function that calls c.Do and unmarshals
// the response body into out. It returns the Response so callers can inspect
// StatusCode and Header alongside the decoded data. out must be a non-nil pointer.
func DoJSON(ctx context.Context, c Client, req *Request, out any) (*Response, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		// resp is non-nil for API-level errors (FatSecret error envelopes), so
		// pass it through to let callers inspect StatusCode/Body; it is nil only
		// for transport-level failures.
		return resp, err
	}
	if out != nil && len(resp.Body) > 0 {
		if uerr := json.Unmarshal(resp.Body, out); uerr != nil {
			return resp, fmt.Errorf("client: unmarshal response: %w", uerr)
		}
	}
	return resp, nil
}
