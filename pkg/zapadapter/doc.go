// Package zapadapter provides a go.uber.org/zap adapter for the FatSecret client
// Logger interface. It is intentionally isolated from pkg/client so that the zap
// dependency is only linked when a caller explicitly imports this package.
//
// Usage:
//
//	zapL, _ := zap.NewProduction()
//	c, err := client.NewClient(client.Options{
//	    Authenticator: myAuth,
//	    Logger:        zapadapter.NewZapLogger(zapL),
//	})
package zapadapter
