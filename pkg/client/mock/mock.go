// Package mock provides a test-only Client implementation that forwards
// requests to an in-process HTTP server (typically httptest.NewServer).
// It is used exclusively by generated smoke tests; do not use in production code.
package mock

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	pkgclient "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client"
)

// mockClient is a minimal Client that proxies requests to a fixed base URL
// without any authentication, retry, or caching layers.
type mockClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a pkg/client.Client that forwards every request to baseURL.
// baseURL should be the URL produced by httptest.NewServer (e.g. "http://127.0.0.1:PORT").
// The returned Client skips all authentication and middleware — suitable only for tests.
func NewClient(baseURL string) pkgclient.Client {
	return &mockClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

// Do implements client.Client.Do. It combines baseURL + req.Path + req.Params,
// then dispatches via the standard http.Client. No authentication is applied.
func (m *mockClient) Do(ctx context.Context, req *pkgclient.Request) (*pkgclient.Response, error) {
	if ctx == nil {
		return nil, fmt.Errorf("mock: nil context")
	}
	if req == nil {
		return nil, fmt.Errorf("mock: nil request")
	}

	// Build URL.
	rawURL := m.baseURL
	if req.Path != "" {
		if !strings.HasPrefix(req.Path, "/") {
			rawURL += "/"
		}
		rawURL += req.Path
	}

	// Clone params; inject format=json as the real client does, honouring the
	// same OmitFormatParam opt-out so tests exercise the URL the server sees.
	params := make(url.Values, len(req.Params)+1)
	for k, vv := range req.Params {
		params[k] = append([]string(nil), vv...)
	}
	if !req.OmitFormatParam {
		params.Set("format", "json")
	}

	withQuery := func(u string) string {
		if encoded := params.Encode(); encoded != "" {
			return u + "?" + encoded
		}
		return u
	}

	var bodyReader io.Reader
	var contentType string
	method := strings.ToUpper(req.Method)

	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete, http.MethodOptions:
		rawURL = withQuery(rawURL)
	default:
		if req.Body == nil {
			bodyReader = strings.NewReader(params.Encode())
			contentType = "application/x-www-form-urlencoded"
		} else {
			rawURL = withQuery(rawURL)
			bodyReader = strings.NewReader(string(req.Body))
			contentType = "application/json"
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("mock: build request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mock: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mock: read body: %w", err)
	}

	return &pkgclient.Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Header:     resp.Header,
	}, nil
}

// Auth implements client.Client.Auth. Returns nil; mock has no authenticator.
func (m *mockClient) Auth() auth.Authenticator { return nil }

// Close implements client.Client.Close. No-op for mock.
func (m *mockClient) Close() error { return nil }
