// Package errors defines the typed error hierarchy for the FatSecret API client.
//
// All errors returned by the client implement the standard error interface.
// Callers can inspect errors using errors.Is and errors.As from the standard library.
//
// # Sentinel Errors
//
// The package exposes sentinel errors for coarse-grained matching:
//
//	errors.Is(err, errors.ErrUnauthorized) // token invalid or missing
//	errors.Is(err, errors.ErrForbidden)    // insufficient scope or IP blocked
//	errors.Is(err, errors.ErrRateLimited)  // daily quota exceeded
//	errors.Is(err, errors.ErrServer)       // 5xx or FatSecret system error
//
// # Typed Errors
//
// For fine-grained inspection, use errors.As to extract detail:
//
//	var pe *errors.PermissionError
//	if errors.As(err, &pe) {
//	    log.Printf("missing scope: %s", pe.MissingScope)
//	}
//
//	var param *errors.ParameterError
//	if errors.As(err, &param) {
//	    log.Printf("bad parameter: %s (code %d)", param.Param, param.Code)
//	}
//
// # FatSecret Error Codes
//
// FatSecret returns an integer error code inside the response body even when
// the HTTP status is 200. Use DispatchByFatSecretCode to convert a raw code
// to the correct typed error. Use DispatchByStatus for non-200 HTTP responses
// that do not carry a FatSecret error body.
package errors
