package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	pkgerrors "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/errors"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

const (
	// DefaultMaxResponseBytes caps the response body read at 10 MiB to protect
	// against runaway responses. Configurable via TransportOptions.MaxResponseBytes.
	DefaultMaxResponseBytes int64 = 10 * 1024 * 1024
)

// Transport wraps a standard *http.Client with a fully-composed middleware chain
// and observability instrumentation. Callers obtain a Transport via NewTransport;
// the zero value is not valid.
type Transport struct {
	client           *http.Client
	chain            RoundTripFunc
	collector        metrics.Collector
	maxResponseBytes int64
}

// TransportOptions carries configuration for NewTransport.
type TransportOptions struct {
	// HTTPClient is the underlying *http.Client. When nil, http.DefaultClient is used.
	HTTPClient *http.Client

	// Chain is the fully-composed middleware chain produced by Chain(...). Required.
	Chain RoundTripFunc

	// Collector is the metrics collector. When nil, a no-op collector is used.
	Collector metrics.Collector

	// MaxResponseBytes caps the number of bytes read from each response body.
	// Zero or negative values default to DefaultMaxResponseBytes (10 MiB).
	MaxResponseBytes int64
}

// NewTransport constructs a Transport from opts. Returns an error if opts.Chain
// is nil.
func NewTransport(opts TransportOptions) (*Transport, error) {
	if opts.Chain == nil {
		return nil, errors.New("transport: Chain must not be nil")
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	col := opts.Collector
	if col == nil {
		col = metrics.NoopCollector{}
	}

	maxBytes := opts.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}

	return &Transport{
		client:           httpClient,
		chain:            opts.Chain,
		collector:        col,
		maxResponseBytes: maxBytes,
	}, nil
}

// Do executes req using the middleware chain. ctx must not be nil.
// The response body is read in full (up to MaxResponseBytes), decoded for
// FatSecret error envelopes, and dispatched to the appropriate typed error.
// Returns the *http.Response (with body replaced by a fresh reader) and the
// decoded body bytes on success. On any error the metrics failure counter is
// incremented.
func (t *Transport) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	if ctx == nil {
		return nil, nil, errors.New("client: nil context is not allowed")
	}

	// Ensure the request carries the provided context.
	req = req.WithContext(ctx)

	resp, err := t.chain(req)
	if err != nil {
		t.collector.IncFailures(req.Method, req.URL.Path, 0)
		return nil, nil, fmt.Errorf("transport: %w", err)
	}

	body, readErr := t.readBody(resp)
	if readErr != nil {
		t.collector.IncFailures(req.Method, req.URL.Path, resp.StatusCode)
		return resp, nil, fmt.Errorf("transport: read body: %w", readErr)
	}

	// Replace body so callers can still read it.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// Attempt to decode FatSecret error envelope first.
	if apiErr := decodeFatSecretError(body, resp.StatusCode); apiErr != nil {
		t.collector.IncFailures(req.Method, req.URL.Path, resp.StatusCode)
		return resp, body, fmt.Errorf("fatsecret api: %w", apiErr)
	}

	// If status >= 400 with no FatSecret envelope, dispatch by HTTP status.
	if resp.StatusCode >= 400 {
		statusErr := pkgerrors.DispatchByStatus(resp.StatusCode, string(body))
		if statusErr != nil {
			t.collector.IncFailures(req.Method, req.URL.Path, resp.StatusCode)
			return resp, body, fmt.Errorf("fatsecret: %w", statusErr)
		}
	}

	return resp, body, nil
}

// readBody reads the response body up to t.maxResponseBytes and closes it.
func (t *Transport) readBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	lr := io.LimitReader(resp.Body, t.maxResponseBytes)
	return io.ReadAll(lr)
}

// fatSecretErrorEnvelope mirrors the FatSecret error JSON structure:
// {"error":{"code":"N","message":"..."}}
type fatSecretErrorEnvelope struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeFatSecretError attempts to parse body as a FatSecret error envelope.
// Returns nil when no envelope is present or the body is not valid JSON.
// FatSecret encodes the error code as a string; this function converts it to int
// via strconv.Atoi before dispatching.
func decodeFatSecretError(body []byte, httpStatus int) error {
	if len(body) == 0 {
		return nil
	}
	// Quick check: must contain "error" key to avoid costly JSON parse on every response.
	if !bytes.Contains(body, []byte(`"error"`)) {
		return nil
	}

	var env fatSecretErrorEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Error == nil {
		return nil
	}

	code, err := strconv.Atoi(env.Error.Code)
	if err != nil || code == 0 {
		return nil
	}

	return pkgerrors.DispatchByFatSecretCode(code, env.Error.Message, httpStatus)
}
