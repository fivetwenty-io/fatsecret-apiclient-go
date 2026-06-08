// Package tls provides TLS configuration builders for the FatSecret API client.
// It defines the TLSMode type and constructs *tls.Config values for each mode,
// ranging from fully insecure (opt-in only) to standard hostname verification
// and SHA-256 certificate fingerprint pinning.
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
)

// TLSMode controls how the transport verifies TLS connections to the FatSecret API.
type TLSMode string

const (
	// TLSModeNone disables all certificate verification. This is an explicit
	// opt-in insecure mode intended only for local development or testing
	// against self-signed certificates. Never use in production.
	TLSModeNone TLSMode = "none"

	// TLSModePeer verifies the certificate chain but does not check the server
	// hostname. Useful when connecting through a proxy that presents a valid
	// chain-trusted certificate for a different hostname.
	TLSModePeer TLSMode = "peer"

	// TLSModeHostname applies standard Go TLS verification: full certificate
	// chain validation and hostname matching. This is the default mode and
	// the recommended setting for production use.
	TLSModeHostname TLSMode = "hostname"

	// TLSModeFingerprint skips standard chain/hostname verification and instead
	// pins the connection to a specific leaf certificate identified by its
	// SHA-256 fingerprint. At least one fingerprint must be supplied. There is
	// no trust-on-first-use fallback: the very first connection must match.
	TLSModeFingerprint TLSMode = "fingerprint"

	// TLSModeFull applies standard Go TLS verification (chain + hostname) and
	// additionally checks the leaf certificate's SHA-256 fingerprint against
	// the supplied allow-list, providing defense-in-depth against a compromised
	// intermediate CA.
	TLSModeFull TLSMode = "full"
)

// BuildTLSConfig returns a *tls.Config configured for the requested mode.
//
// fingerprints is a slice of hex-encoded SHA-256 certificate fingerprints
// (64 hex characters each, colons optional). It is required and must be
// non-empty when mode is TLSModeFingerprint or TLSModeFull.
//
// pinCache is consulted on each successful fingerprint match to persist seen
// fingerprints across process restarts. A nil pinCache disables persistence.
//
// The returned *tls.Config is ready for use in an http.Transport.TLSClientConfig
// field. Callers must not modify it after handing it to the transport.
func BuildTLSConfig(mode TLSMode, fingerprints []string, pinCache PinCache) (*tls.Config, error) {
	switch mode {
	case TLSModeNone:
		cfg := &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- InsecureSkipVerify with a custom verifier (peer/fingerprint/opt-in-insecure mode) selected explicitly by the caller
		return cfg, nil

	case TLSModePeer:
		verifier, err := peerOnlyVerifier()
		if err != nil {
			return nil, fmt.Errorf("tls: peer mode: %w", err)
		}
		// InsecureSkipVerify required to suppress built-in hostname check;
		// chain verification is performed manually in VerifyPeerCertificate callback.
		// SessionTicketsDisabled prevents session resumption from bypassing the custom check.
		cfg := &tls.Config{
			InsecureSkipVerify:     true,     // #nosec G402 -- InsecureSkipVerify with a custom verifier (peer/fingerprint/opt-in-insecure mode) selected explicitly by the caller
			VerifyPeerCertificate:  verifier, // #nosec G123 -- session resumption disabled below; custom chain check cannot be bypassed
			SessionTicketsDisabled: true,
			ClientSessionCache:     nil,
		}
		return cfg, nil

	case TLSModeHostname:
		// Return the zero-value tls.Config, which applies Go's default TLS
		// behavior: full certificate chain validation and hostname matching.
		return &tls.Config{}, nil //nolint:gosec // zero-value is the secure default

	case TLSModeFingerprint:
		if len(fingerprints) == 0 {
			return nil, fmt.Errorf("tls: fingerprint mode requires at least one fingerprint in TLSCertFingerprints")
		}
		parsed, err := parseFingerprints(fingerprints)
		if err != nil {
			return nil, fmt.Errorf("tls: invalid fingerprint: %w", err)
		}
		// InsecureSkipVerify required to bypass chain/hostname checks;
		// security is enforced exclusively via SHA-256 leaf fingerprint in VerifyConnection.
		cfg := &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- InsecureSkipVerify with a custom verifier (peer/fingerprint/opt-in-insecure mode) selected explicitly by the caller
			VerifyConnection:   fingerprintVerifier(parsed, pinCache, false),
		}
		return cfg, nil

	case TLSModeFull:
		if len(fingerprints) == 0 {
			return nil, fmt.Errorf("tls: full mode requires at least one fingerprint in TLSCertFingerprints")
		}
		parsed, err := parseFingerprints(fingerprints)
		if err != nil {
			return nil, fmt.Errorf("tls: invalid fingerprint: %w", err)
		}
		// Standard Go TLS verification (chain + hostname) runs first because
		// InsecureSkipVerify is false. VerifyConnection then adds fingerprint
		// pinning on top as a second layer of defense.
		cfg := &tls.Config{
			VerifyConnection: fingerprintVerifier(parsed, pinCache, true),
		}
		return cfg, nil

	default:
		return nil, fmt.Errorf("tls: unknown mode %q", mode)
	}
}

// peerOnlyVerifier returns a VerifyPeerCertificate callback that validates the
// certificate chain presented by the server without checking the server hostname.
// The callback receives the raw DER bytes and any verified chains already
// constructed by the TLS handshake; because InsecureSkipVerify is set on the
// parent tls.Config, Go skips its own verification, so we must rebuild it here.
func peerOnlyVerifier() (func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error, error) {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("tls: peer verification: no certificates presented")
		}

		// Parse all DER-encoded certificates from the handshake.
		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for i, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				return fmt.Errorf("tls: peer verification: parse cert[%d]: %w", i, err)
			}
			certs = append(certs, cert)
		}

		// Build an intermediates pool from all non-leaf certificates.
		intermediates := x509.NewCertPool()
		for _, cert := range certs[1:] {
			intermediates.AddCert(cert)
		}

		// Verify chain only; leave DNSName empty so hostname is not checked.
		opts := x509.VerifyOptions{
			Intermediates: intermediates,
		}
		if _, err := certs[0].Verify(opts); err != nil {
			return fmt.Errorf("tls: peer chain verification failed: %w", err)
		}
		return nil
	}, nil
}

// parseFingerprints parses a slice of hex fingerprint strings into fixed-size
// [32]byte arrays. It returns an error on the first invalid entry.
func parseFingerprints(hexFPs []string) ([][32]byte, error) {
	out := make([][32]byte, 0, len(hexFPs))
	for i, fp := range hexFPs {
		parsed, err := parseHexFingerprint(fp)
		if err != nil {
			return nil, fmt.Errorf("fingerprint[%d] %q: %w", i, fp, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}
