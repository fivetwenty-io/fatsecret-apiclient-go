package auth

import "context"

// InvalidAuthenticator defers a configuration error to first use. All methods
// that return an error return the configured Err; IsAuthenticated always returns
// false; Logout is a no-op.
//
// Use this type when constructing a client whose authentication configuration may
// be absent or invalid at construction time. The error surfaces only when the
// first API call is made, rather than at construction.
type InvalidAuthenticator struct {
	// Err is the configuration error that will be returned by Authenticate,
	// GetHeaders, and Refresh. It must not be nil.
	Err error
}

// Authenticate returns the configured Err unconditionally.
func (a *InvalidAuthenticator) Authenticate(_ context.Context) error { return a.Err }

// IsAuthenticated always returns false because no valid credentials are held.
func (a *InvalidAuthenticator) IsAuthenticated() bool { return false }

// GetHeaders returns nil and the configured Err unconditionally.
func (a *InvalidAuthenticator) GetHeaders(_ context.Context) (map[string]string, error) {
	return nil, a.Err
}

// Refresh returns the configured Err unconditionally.
func (a *InvalidAuthenticator) Refresh(_ context.Context) error { return a.Err }

// Logout is a no-op because no credentials are held.
func (a *InvalidAuthenticator) Logout() {}
