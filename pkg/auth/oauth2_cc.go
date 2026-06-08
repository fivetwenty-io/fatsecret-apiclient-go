package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// defaultTokenURL is the FatSecret OAuth 2.0 token endpoint.
	defaultTokenURL = "https://oauth.fatsecret.com/connect/token" // #nosec G101 -- not a credential: public OAuth 2.0 token endpoint URL

	// defaultRefreshBefore is the proactive-refresh window applied before token expiry.
	defaultRefreshBefore = 5 * time.Minute
)

// OAuth2Config holds the configuration for the OAuth 2.0 client-credentials flow.
type OAuth2Config struct {
	// ClientID is the OAuth 2.0 client identifier issued by FatSecret.
	// Required; NewOAuth2ClientCredentials returns an [InvalidAuthenticator] when empty.
	ClientID string

	// ClientSecret is the OAuth 2.0 client secret issued by FatSecret.
	// Required; NewOAuth2ClientCredentials returns an [InvalidAuthenticator] when empty.
	ClientSecret string

	// TokenURL is the OAuth 2.0 token endpoint. Defaults to
	// "https://oauth.fatsecret.com/connect/token" when empty.
	TokenURL string

	// Scopes is the list of OAuth 2.0 scopes to request. When nil or empty the
	// scope parameter is omitted from the token request, which grants the full set
	// of scopes available to the client.
	Scopes []string

	// RefreshBefore is the duration before token expiry at which a proactive refresh
	// is triggered. Defaults to 5 minutes when zero.
	RefreshBefore time.Duration

	// HTTPClient is the HTTP client used for token requests. When nil, http.DefaultClient
	// is used.
	HTTPClient *http.Client
}

// tokenResponse is the JSON body returned by the FatSecret token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// OAuth2ClientCredentials implements the OAuth 2.0 client-credentials flow for
// the FatSecret API. It proactively refreshes the access token before expiry and
// deduplicates concurrent refresh calls using a singleflight group.
//
// Create instances with [NewOAuth2ClientCredentials]; the zero value is not valid.
type OAuth2ClientCredentials struct {
	cfg          OAuth2Config
	mu           sync.RWMutex
	token        string
	issuedAt     time.Time
	expiresIn    time.Duration
	refreshGroup singleflight.Group
}

// NewOAuth2ClientCredentials constructs and validates an [OAuth2ClientCredentials].
// It returns an [InvalidAuthenticator] wrapping a descriptive error when cfg.ClientID
// or cfg.ClientSecret is empty. Defaults are applied to TokenURL and RefreshBefore
// when those fields are zero.
func NewOAuth2ClientCredentials(cfg OAuth2Config) Authenticator {
	if cfg.ClientID == "" {
		return &InvalidAuthenticator{Err: fmt.Errorf("auth: OAuth2Config.ClientID must not be empty")}
	}
	if cfg.ClientSecret == "" {
		return &InvalidAuthenticator{Err: fmt.Errorf("auth: OAuth2Config.ClientSecret must not be empty")}
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = defaultTokenURL
	}
	if cfg.RefreshBefore == 0 {
		cfg.RefreshBefore = defaultRefreshBefore
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	return &OAuth2ClientCredentials{cfg: cfg}
}

// Authenticate fetches the initial access token. It is equivalent to [Refresh]
// and is safe to call multiple times; concurrent calls collapse into one request.
func (a *OAuth2ClientCredentials) Authenticate(ctx context.Context) error {
	return a.Refresh(ctx)
}

// IsAuthenticated reports whether a valid, non-expired (outside the refresh window)
// access token is currently held.
func (a *OAuth2ClientCredentials) IsAuthenticated() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.token == "" {
		return false
	}
	return time.Since(a.issuedAt) <= a.expiresIn-a.cfg.RefreshBefore
}

// GetHeaders returns {"Authorization": "Bearer <token>"}. It triggers a proactive
// refresh when the token is absent or nearing expiry. The refresh is deduplicated:
// concurrent callers all wait for the single in-flight request.
func (a *OAuth2ClientCredentials) GetHeaders(ctx context.Context) (map[string]string, error) {
	a.mu.RLock()
	needsRefresh := a.token == "" || time.Since(a.issuedAt) > a.expiresIn-a.cfg.RefreshBefore
	tok := a.token
	a.mu.RUnlock()

	if needsRefresh {
		if err := a.Refresh(ctx); err != nil {
			return nil, err
		}
		a.mu.RLock()
		tok = a.token
		a.mu.RUnlock()
	}
	return map[string]string{"Authorization": "Bearer " + tok}, nil
}

// Refresh unconditionally fetches a new access token from the token endpoint.
// Concurrent calls collapse into a single HTTP request via singleflight; all
// waiters receive the same error result.
func (a *OAuth2ClientCredentials) Refresh(ctx context.Context) error {
	_, err, _ := a.refreshGroup.Do("token", func() (any, error) {
		return nil, a.fetchToken(ctx)
	})
	return err
}

// Logout discards the held access token. After Logout, IsAuthenticated returns
// false and GetHeaders returns an error until Authenticate or Refresh is called.
func (a *OAuth2ClientCredentials) Logout() {
	a.mu.Lock()
	a.token = ""
	a.issuedAt = time.Time{}
	a.expiresIn = 0
	a.mu.Unlock()
}

// fetchToken posts the client-credentials grant to the token endpoint, parses the
// response, and stores the token under a write lock. It is only called from within
// the singleflight group in Refresh.
func (a *OAuth2ClientCredentials) fetchToken(ctx context.Context) error {
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(a.cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(a.cfg.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("auth: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(a.cfg.ClientID, a.cfg.ClientSecret)

	issuedAt := time.Now()
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("auth: reading token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Attempt to extract an error description from the response body; fall back to raw body.
		var errBody struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if jsonErr := json.Unmarshal(body, &errBody); jsonErr == nil && errBody.Error != "" {
			desc := errBody.Error
			if errBody.ErrorDescription != "" {
				desc += ": " + errBody.ErrorDescription
			}
			return fmt.Errorf("auth: token endpoint returned %d: %s", resp.StatusCode, desc)
		}
		truncated := string(body)
		if len(truncated) > 256 {
			truncated = truncated[:256] + "…"
		}
		return fmt.Errorf("auth: token endpoint returned %d: %s", resp.StatusCode, truncated)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return fmt.Errorf("auth: parsing token response: %w", err)
	}
	if tr.AccessToken == "" {
		return fmt.Errorf("auth: token endpoint returned empty access_token")
	}
	if tr.ExpiresIn <= 0 {
		return fmt.Errorf("auth: token endpoint returned invalid expires_in %d", tr.ExpiresIn)
	}

	a.mu.Lock()
	a.token = tr.AccessToken
	a.issuedAt = issuedAt
	a.expiresIn = time.Duration(tr.ExpiresIn) * time.Second
	a.mu.Unlock()
	return nil
}
