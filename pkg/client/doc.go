// Package client is the public entry point for the FatSecret API client library.
// It provides the Client interface, NewClient constructor, Options configuration
// struct, per-request context helpers, and observability adapters.
//
// Basic usage:
//
//	auth, err := auth.NewOAuth2ClientCredentials(auth.OAuth2Config{
//	    ClientID:     os.Getenv("FATSECRET_CLIENT_ID"),
//	    ClientSecret: os.Getenv("FATSECRET_CLIENT_SECRET"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	c, err := client.NewClient(client.Options{
//	    Authenticator: auth,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer c.Close()
//
// See Options for the full set of configuration knobs including TLS mode,
// retry policy, structured logging, metrics collection, and response caching.
package client
