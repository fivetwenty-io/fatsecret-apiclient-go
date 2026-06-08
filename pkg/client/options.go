package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	internalhttp "github.com/fivetwenty-io/fatsecret-apiclient-go/internal/http"
	internaltls "github.com/fivetwenty-io/fatsecret-apiclient-go/internal/tls"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/auth"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/cache"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/metrics"
)

const (
	defaultBaseURL    = "https://platform.fatsecret.com"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
	defaultRetryDelay = 500 * time.Millisecond
)

// Options is the single configuration struct passed to NewClient. The zero value
// is invalid because Authenticator is required; all other fields default to safe
// production values when left at zero. NewClient applies setDefaults before
// validating, so callers only need to set what differs from the documented defaults.
type Options struct {
	// Authenticator is the auth strategy used for every request. Required.
	// Use pkg/auth.NewOAuth2ClientCredentials, NewOAuth1Signed, or
	// NewOAuth1ProfileDelegation to construct one.
	Authenticator auth.Authenticator

	// BaseURL is the root URL of the FatSecret REST API.
	// Default: "https://platform.fatsecret.com".
	BaseURL string

	// Timeout is the end-to-end deadline for a single request attempt (not
	// counting retries). Default: 30s. A negative value disables the timeout.
	Timeout time.Duration

	// MaxRetries is the number of additional attempts after the initial failure.
	// Total attempts = MaxRetries + 1. Default: 2.
	MaxRetries int

	// RetryDelay is the base delay before the first retry. Subsequent delays grow
	// via exponential backoff (delay * 2^attempt) plus 10% jitter. Default: 500ms.
	RetryDelay time.Duration

	// TLSMode controls how the transport verifies TLS connections.
	// Default: TLSModeHostname (standard chain + hostname verification).
	TLSMode TLSMode

	// TLSFingerprints is a slice of hex-encoded SHA-256 certificate fingerprints.
	// Required when TLSMode is TLSModeFingerprint or TLSModeFull.
	TLSFingerprints []string

	// PinCachePath is the filesystem path for a persistent certificate pin cache.
	// An empty string uses in-memory-only pin tracking (no persistence across
	// process restarts). Only meaningful when TLSMode is TLSModeFingerprint or
	// TLSModeFull.
	PinCachePath string

	// InsecureSkipVerify disables all TLS certificate verification. Only for
	// local development against self-signed certificates. Setting this to true
	// logs a WARN when a Logger is present and overrides TLSMode to TLSModeNone.
	InsecureSkipVerify bool

	// Logger receives structured log output for request/response events.
	// Default: no logging. Provide a Logger to enable request tracing.
	Logger Logger

	// LogConfig controls which details are included in log output.
	LogConfig LogConfig

	// Hooks is a list of callbacks fired after every HTTP round-trip regardless
	// of success or failure. Hooks must not block.
	Hooks []Hook

	// Metrics is the observability collector for request counts, durations, and
	// byte totals. Default: no-op collector.
	Metrics metrics.Collector

	// Cache is the GET-response cache. When set, successful GET responses are
	// stored and replayed on cache hit, skipping authentication and retry overhead.
	// Default: no-op cache (caching disabled).
	Cache cache.Cache

	// CacheTTL is the time-to-live for cached GET responses. Zero means entries
	// never expire (cache implementation defined). Only used when Cache is set.
	CacheTTL time.Duration

	// --- Zero-knob transport tuning ---
	// All fields in this section default to zero. A zero value means "inherit the
	// Go stdlib default for this field"; the field is written to *http.Transport
	// only when non-zero. This is the zero-knob contract.

	// DialTimeout is the timeout for establishing a new TCP connection.
	// Zero: no explicit dial timeout beyond the request's context deadline.
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the maximum time allowed for a TLS handshake.
	// Zero: stdlib default (10s).
	TLSHandshakeTimeout time.Duration

	// MaxIdleConnsPerHost caps the number of idle (keep-alive) connections
	// retained per host. Zero: stdlib default (2).
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the maximum time an idle keep-alive connection stays
	// open before being closed. Zero: stdlib default (90s).
	IdleConnTimeout time.Duration

	// KeepAlive is the interval between TCP keep-alive probes sent on active
	// connections. Zero: OS-level default (typically 15s).
	KeepAlive time.Duration
}

// setDefaults fills zero-value fields in opts with documented defaults.
// Transport-tuning fields (DialTimeout, TLSHandshakeTimeout, MaxIdleConnsPerHost,
// IdleConnTimeout, KeepAlive) are intentionally left at zero to honour the
// zero-knob contract: they are applied to the transport only when non-zero.
func setDefaults(opts *Options) {
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.RetryDelay == 0 {
		opts.RetryDelay = defaultRetryDelay
	}
	if opts.TLSMode == "" && !opts.InsecureSkipVerify {
		opts.TLSMode = TLSModeHostname
	}
	if opts.Metrics == nil {
		opts.Metrics = metrics.NoopCollector{}
	}
	if opts.Cache == nil {
		opts.Cache = cache.NoopCache{}
	}
}

// validate checks opts for configuration errors that would make the client
// inoperable. Call after setDefaults so that defaulted fields are present
// during validation.
func validate(opts Options) error {
	if opts.Authenticator == nil {
		return fmt.Errorf("client: Authenticator is required")
	}

	if opts.InsecureSkipVerify {
		if opts.Logger != nil {
			opts.Logger.Warn(
				"client: InsecureSkipVerify is true — TLS certificate verification is disabled; never use in production",
				nil,
			)
		}
	}

	effectiveMode := opts.TLSMode
	if opts.InsecureSkipVerify {
		effectiveMode = TLSModeNone
	}

	switch effectiveMode {
	case TLSModeFingerprint, TLSModeFull:
		if len(opts.TLSFingerprints) == 0 {
			return fmt.Errorf(
				"client: TLSMode %q requires at least one entry in TLSFingerprints",
				effectiveMode,
			)
		}
	}

	if opts.BaseURL != "" {
		if _, err := url.ParseRequestURI(opts.BaseURL); err != nil {
			return fmt.Errorf("client: BaseURL %q is not a valid URL: %w", opts.BaseURL, err)
		}
	}

	return nil
}

// NewClient constructs a ready-to-use Client from opts. setDefaults is applied
// first so callers only need to supply fields that differ from package defaults.
//
// NewClient returns an error when:
//   - Authenticator is nil
//   - TLSMode is TLSModeFingerprint or TLSModeFull with no TLSFingerprints
//   - BaseURL is non-empty but not a parseable URL
//   - TLS configuration fails (bad fingerprint format, pin cache unreadable, etc.)
func NewClient(opts Options) (Client, error) {
	setDefaults(&opts)
	if err := validate(opts); err != nil {
		return nil, err
	}

	// Resolve the effective TLS mode after InsecureSkipVerify override.
	tlsMode := opts.TLSMode
	if opts.InsecureSkipVerify {
		tlsMode = TLSModeNone
	}

	// Build the persistent pin cache when a path is provided for fingerprint modes.
	var pinCache internaltls.PinCache
	if opts.PinCachePath != "" && (tlsMode == TLSModeFingerprint || tlsMode == TLSModeFull) {
		pc, err := internaltls.NewFilePinCache(opts.PinCachePath)
		if err != nil {
			return nil, fmt.Errorf("client: pin cache: %w", err)
		}
		pinCache = pc
	}

	tlsCfg, err := internaltls.BuildTLSConfig(tlsMode, opts.TLSFingerprints, pinCache)
	if err != nil {
		return nil, fmt.Errorf("client: tls: %w", err)
	}

	// Build the *http.Transport; only non-zero tuning fields are written.
	netTransport := buildNetTransport(opts, tlsCfg)

	httpClient := &http.Client{
		Transport: netTransport,
		Timeout:   opts.Timeout,
	}

	// Build the base RoundTripFunc that dispatches via the *http.Client.
	base := func(req *http.Request) (*http.Response, error) {
		return httpClient.Do(req) // #nosec G704 -- BaseURL is caller-supplied client configuration; targeting FatSecret is the library's purpose
	}

	// Compose the middleware chain: Cache → Auth → Retry → Logging.
	// Cache is added outermost only when a real (non-noop) cache is configured.
	var mws []internalhttp.Middleware
	if _, isNoop := opts.Cache.(cache.NoopCache); !isNoop {
		mws = append(mws, internalhttp.CacheMiddleware(opts.Cache, opts.CacheTTL))
	}
	mws = append(mws,
		internalhttp.AuthMiddleware(opts.Authenticator),
		internalhttp.RetryMiddleware(opts.MaxRetries, opts.RetryDelay),
		internalhttp.LoggingMiddleware(opts.Logger, opts.Hooks, opts.Metrics, opts.LogConfig),
	)

	chain := internalhttp.Chain(base, mws...)

	tr, err := internalhttp.NewTransport(internalhttp.TransportOptions{
		HTTPClient: httpClient,
		Chain:      chain,
		Collector:  opts.Metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("client: transport: %w", err)
	}

	return &clientImpl{
		authenticator: opts.Authenticator,
		baseURL:       opts.BaseURL,
		transport:     tr,
		netTransport:  netTransport,
	}, nil
}

// buildNetTransport constructs an *http.Transport with opts tuning applied.
// Only non-zero duration and integer fields are written; zero fields inherit
// Go stdlib defaults (zero-knob contract).
func buildNetTransport(opts Options, tlsCfg *tls.Config) *http.Transport {
	dialer := &net.Dialer{}
	if opts.DialTimeout > 0 {
		dialer.Timeout = opts.DialTimeout
	}
	if opts.KeepAlive > 0 {
		dialer.KeepAlive = opts.KeepAlive
	}

	t := &http.Transport{
		DialContext:        dialer.DialContext,
		TLSClientConfig:    tlsCfg,
		ForceAttemptHTTP2:  true,
		DisableCompression: false,
	}

	if opts.TLSHandshakeTimeout > 0 {
		t.TLSHandshakeTimeout = opts.TLSHandshakeTimeout
	}
	if opts.MaxIdleConnsPerHost > 0 {
		t.MaxIdleConnsPerHost = opts.MaxIdleConnsPerHost
	}
	if opts.IdleConnTimeout > 0 {
		t.IdleConnTimeout = opts.IdleConnTimeout
	}

	return t
}
