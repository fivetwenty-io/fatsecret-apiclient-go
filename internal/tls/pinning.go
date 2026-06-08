package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PinCache is a persistent store for previously verified SHA-256 certificate
// fingerprints. Implementations must be safe for concurrent use.
//
// Load reports whether the given raw 32-byte fingerprint is present in the cache.
// Store persists a fingerprint; an error from Store is non-fatal — the TLS
// handshake succeeds regardless, but the fingerprint may not survive a restart.
type PinCache interface {
	// Load returns true if the raw 32-byte SHA-256 fingerprint is in the cache.
	Load(fingerprint []byte) bool

	// Store persists the raw 32-byte SHA-256 fingerprint for future lookups.
	Store(fingerprint []byte) error
}

// FilePinCache is a file-backed PinCache that serializes seen fingerprints as a
// JSON array of lowercase hex strings. Writes are atomic: data is written to a
// temporary file in the same directory as the cache file, then renamed over it,
// so a crash during a write cannot corrupt the existing cache.
type FilePinCache struct {
	mu   sync.RWMutex
	path string
	set  map[string]struct{} // normalized hex strings
}

// NewFilePinCache constructs a FilePinCache backed by the given file path.
// If the file exists, its contents are loaded immediately. If the file does not
// exist, an empty cache is returned and the file is created on the first Store.
// Returns an error only if the file exists but cannot be parsed.
func NewFilePinCache(path string) (*FilePinCache, error) {
	c := &FilePinCache{
		path: path,
		set:  make(map[string]struct{}),
	}
	data, err := os.ReadFile(path) // #nosec G304 -- pin-cache file path is caller-supplied client configuration
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("tls: pin cache: read %q: %w", path, err)
	}
	var hexList []string
	if err := json.Unmarshal(data, &hexList); err != nil {
		return nil, fmt.Errorf("tls: pin cache: parse %q: %w", path, err)
	}
	for _, h := range hexList {
		normalized := strings.ToLower(strings.ReplaceAll(h, ":", ""))
		if len(normalized) != 64 {
			return nil, fmt.Errorf("tls: pin cache: invalid fingerprint %q in %q", h, path)
		}
		c.set[normalized] = struct{}{}
	}
	return c, nil
}

// Load returns true if the raw 32-byte SHA-256 fingerprint is in the cache.
func (c *FilePinCache) Load(fingerprint []byte) bool {
	key := hex.EncodeToString(fingerprint)
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.set[key]
	return ok
}

// Store persists the raw 32-byte SHA-256 fingerprint. The cache file is updated
// atomically using a write-to-temp-then-rename pattern so concurrent readers
// always see a consistent JSON file.
func (c *FilePinCache) Store(fingerprint []byte) error {
	key := hex.EncodeToString(fingerprint)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.set[key]; exists {
		return nil // already stored, skip unnecessary write
	}
	c.set[key] = struct{}{}
	return c.flush()
}

// flush serializes the in-memory set to a temp file and renames it atomically.
// Must be called with c.mu held for writing.
func (c *FilePinCache) flush() error {
	list := make([]string, 0, len(c.set))
	for k := range c.set {
		list = append(list, k)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("tls: pin cache: marshal: %w", err)
	}

	dir := filepath.Dir(c.path)
	tmp, err := os.CreateTemp(dir, ".pincache-*.tmp")
	if err != nil {
		return fmt.Errorf("tls: pin cache: create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("tls: pin cache: write temp file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("tls: pin cache: close temp file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("tls: pin cache: rename %q -> %q: %w", tmpName, c.path, err)
	}
	return nil
}

// parseHexFingerprint parses a hex-encoded SHA-256 fingerprint string into a
// fixed-size [32]byte array. The input may contain colon separators (e.g.,
// "aa:bb:cc:..."). After stripping colons, exactly 64 hex characters (32 bytes)
// are required; any other length is rejected with a descriptive error.
func parseHexFingerprint(s string) ([32]byte, error) {
	clean := strings.ReplaceAll(s, ":", "")
	if len(clean) != 64 {
		return [32]byte{}, fmt.Errorf(
			"fingerprint must be 64 hex characters (32 bytes / SHA-256), got %d characters after stripping colons",
			len(clean),
		)
	}
	b, err := hex.DecodeString(clean)
	if err != nil {
		return [32]byte{}, fmt.Errorf("fingerprint contains invalid hex characters: %w", err)
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}

// leafSHA256 computes the SHA-256 digest of the leaf certificate's raw DER bytes.
// The leaf certificate is the first element of the peer certificate chain.
func leafSHA256(cert *x509.Certificate) [32]byte {
	return sha256.Sum256(cert.Raw)
}

// fingerprintVerifier returns a tls.VerifyConnection callback that enforces
// SHA-256 leaf-certificate pinning against the allowed set.
//
// When requireChain is true, the callback verifies that the standard Go TLS
// chain-and-hostname checks have already passed (i.e., tls.Config.InsecureSkipVerify
// is false) before checking the fingerprint. When requireChain is false,
// InsecureSkipVerify must be set on the parent tls.Config so that Go does not
// reject the connection before the callback runs; fingerprint matching is then
// the sole verification step.
//
// There is no trust-on-first-use: the very first connection must present a leaf
// certificate whose fingerprint is in the allowed set. On a match the fingerprint
// is recorded in pinCache (if non-nil) for diagnostic persistence.
func fingerprintVerifier(allowed [][32]byte, pinCache PinCache, requireChain bool) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fmt.Errorf("tls: fingerprint verification: no peer certificate presented")
		}

		// When requireChain is true (TLSModeFull), standard TLS verification has
		// already run. Confirm it succeeded by checking VerifiedChains.
		if requireChain && len(cs.VerifiedChains) == 0 {
			return fmt.Errorf("tls: full mode: standard chain verification did not produce a verified chain")
		}

		leaf := cs.PeerCertificates[0]
		got := leafSHA256(leaf)

		for _, want := range allowed {
			if got == want {
				// Fingerprint matched. Record in cache (non-fatal on error).
				if pinCache != nil {
					_ = pinCache.Store(got[:])
				}
				return nil
			}
		}

		// Build a human-readable list of allowed fingerprints for the error message.
		wantList := make([]string, len(allowed))
		for i, fp := range allowed {
			wantList[i] = hex.EncodeToString(fp[:])
		}
		return fmt.Errorf(
			"tls: certificate fingerprint mismatch: got %x, allowed: %s",
			got,
			strings.Join(wantList, ", "),
		)
	}
}
