package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
)

// BuildRequest constructs an *http.Request from the supplied parts. body may
// be nil for bodyless methods (GET, HEAD, DELETE, etc.). headers is applied
// on top of any default headers. The returned request has GetBody and
// ContentLength set so the retry middleware can replay the body byte-for-byte.
//
// BuildRequest returns an error if ctx is nil, method or urlStr are empty,
// or if the underlying http.NewRequestWithContext call fails.
func BuildRequest(
	ctx context.Context,
	method, urlStr string,
	body []byte,
	headers map[string]string,
) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("http: nil context is not allowed")
	}
	if method == "" {
		return nil, errors.New("http: method must not be empty")
	}
	if urlStr == "" {
		return nil, errors.New("http: url must not be empty")
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return nil, err
	}

	if len(body) > 0 {
		captured := make([]byte, len(body))
		copy(captured, body)
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(captured)), nil
		}
		req.ContentLength = int64(len(captured))
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// captureBody buffers r.Body into memory so GetBody can supply a fresh reader
// on every retry attempt. It replaces r.Body with a new NopCloser over the
// buffered bytes and sets r.GetBody and r.ContentLength. A nil or http.NoBody
// body is a no-op, as is a request whose GetBody is already set (e.g. from
// BuildRequest). Returns an error if the body cannot be read.
func captureBody(r *http.Request) error {
	// If GetBody is already set, body is already captured for replay.
	if r.GetBody != nil {
		return nil
	}
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	b, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	r.ContentLength = int64(len(b))
	return nil
}
