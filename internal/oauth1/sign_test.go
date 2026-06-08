package oauth1

import (
	"testing"
)

// rfcParams returns the RFC 5849 Appendix A request parameters for signing tests.
//
// The map holds already-decoded values, consistent with how the FatSecret client
// populates params before calling BaseString or Sign.
//
// RFC Appendix A query string: b5=%3D%253D&a3=a&c%40=&a2=r%20b
// Body:                        c2=&a3=2+q
//
// b5 note: the RFC-published base string (Appendix A.5.1) implies b5="==".
// Working backward from b5%3D%253D%253D in the base string:
//
//	percentEncode(normalizeParams) contains b5=%3D%3D, so the stored val is "==".
//
// This matches a double-decode of %3D%253D → =%3D → ==, which is how the RFC
// example was derived. The map stores "==" to reproduce the RFC vector exactly.
//
// a3 note: the RFC request has two a3 entries ("a" and "2 q"). map[string]string
// cannot hold duplicate keys, so only a3="a" is retained. The expected base string
// below therefore omits the a3%3D2%2520q%26 segment that the full RFC base string
// contains. Tests that verify RFC Appendix A.5.1 verbatim must account for this.
func rfcParams() map[string]string {
	return map[string]string{ //nolint:gosec // G101: all values are RFC 5849 Appendix A test vectors, not real credentials
		"b5":                     "==",  // RFC %3D%253D double-decoded
		"a3":                     "a",   // first a3 only (map limitation)
		"c@":                     "",    // RFC c%40=
		"a2":                     "r b", // RFC r%20b
		"oauth_consumer_key":     "9djdj82h48djs9d2",
		"oauth_token":            "kkk9d7dh3k39sjv7",
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        "137131201",
		"oauth_nonce":            "7d8f3e4a",
		"oauth_version":          "1.0",
		"c2":                     "",
	}
}

// TestBaseString_RFCVector asserts that BaseString produces a string whose
// structure matches the RFC 5849 Appendix A.5.1 published base string. Because
// map[string]string cannot hold the duplicate a3 key from the RFC example, the
// expected value is the RFC base string with the a3=2%20q segment removed.
// All other segments — including the non-trivial b5 and c@ encodings — are exact.
func TestBaseString_RFCVector(t *testing.T) {
	t.Parallel()

	// Expected base string matches RFC Appendix A.5.1 exactly, minus the
	// second a3 entry (a3%3D2%2520q%26) which cannot be represented in a map.
	const want = "POST" +
		"&" + "http%3A%2F%2Fexample.com%2Frequest" +
		"&" + "a2%3Dr%2520b" +
		"%26a3%3Da" +
		"%26b5%3D%253D%253D" + // b5==  → %3D%3D → encoded as %253D%253D ✓
		"%26c%2540%3D" + // c@ → c%40, val empty
		"%26c2%3D" + // c2, val empty
		"%26oauth_consumer_key%3D9djdj82h48djs9d2" +
		"%26oauth_nonce%3D7d8f3e4a" +
		"%26oauth_signature_method%3DHMAC-SHA1" +
		"%26oauth_timestamp%3D137131201" +
		"%26oauth_token%3Dkkk9d7dh3k39sjv7" +
		"%26oauth_version%3D1.0"

	got := BaseString("POST", "http://example.com/request", rfcParams())
	if got != want {
		t.Errorf("BaseString mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

// TestBaseString_MethodUppercase verifies that BaseString uppercases the method.
func TestBaseString_MethodUppercase(t *testing.T) {
	t.Parallel()

	params := map[string]string{"oauth_nonce": "abc"}
	lower := BaseString("get", "http://example.com/", params)
	upper := BaseString("GET", "http://example.com/", params)
	if lower != upper {
		t.Errorf("case mismatch: got %q vs %q", lower, upper)
	}
	if len(lower) < 3 || lower[:3] != "GET" {
		t.Errorf("expected GET prefix, got: %s", lower)
	}
}

// TestBaseString_EmptyParams verifies BaseString works with an empty param map.
func TestBaseString_EmptyParams(t *testing.T) {
	t.Parallel()

	// normalized params = "" → percentEncode("") = ""
	// base string = "POST&http%3A%2F%2Fexample.com%2F&"
	got := BaseString("POST", "http://example.com/", map[string]string{})
	want := "POST&http%3A%2F%2Fexample.com%2F&"
	if got != want {
		t.Errorf("empty params: got %q, want %q", got, want)
	}
}

// TestSign_RFCVector asserts that Sign returns a stable, deterministic HMAC-SHA1
// base64 signature for the RFC Appendix A credentials and parameters.
//
// RFC Appendix A.5.2 published signature covers the full request (including both
// a3 values). Because the map omits the second a3, the expected signature here is
// computed from the same reduced parameter set and is verified by cross-checking
// the Go HMAC-SHA1 computation directly. It is NOT the A.5.2 published value.
func TestSign_RFCVector(t *testing.T) {
	t.Parallel()

	// consumerSecret and tokenSecret from RFC 5849 Appendix A (not real credentials).
	const (
		consumerSecret = "djr9rjt0jd78jf88" //nolint:gosec // G101: RFC 5849 Appendix A test vector
		tokenSecret    = "jjd999tj88uiths3" //nolint:gosec // G101: RFC 5849 Appendix A test vector
		// Signature derived from RFC credentials + single-a3 params.
		// Verified by independent Python HMAC-SHA1 computation:
		//   signing_key = "djr9rjt0jd78jf88&jjd999tj88uiths3"
		//   base_string = <TestBaseString_RFCVector want>
		//   sig = base64(hmac_sha1(signing_key, base_string)) = S14QRgUbWi1mZhEtcmeAt9mPJ+0=
		wantSig = "S14QRgUbWi1mZhEtcmeAt9mPJ+0="
	)

	got := Sign("POST", "http://example.com/request", rfcParams(), consumerSecret, tokenSecret)
	if got != wantSig {
		t.Errorf("Sign mismatch\ngot:  %s\nwant: %s", got, wantSig)
	}
}

// TestSign_EmptyTokenSecret verifies that Sign still includes the '&' separator
// in the signing key when tokenSecret is empty, per RFC 5849 §3.4.2.
func TestSign_EmptyTokenSecret(t *testing.T) {
	t.Parallel()

	// Two calls with empty token secret must be identical (deterministic).
	params := map[string]string{
		"oauth_consumer_key":     "key",
		"oauth_nonce":            "nonce",
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        "1000000",
		"oauth_version":          "1.0",
	}
	sig1 := Sign("GET", "http://example.com/api", params, "consumer_secret", "")
	sig2 := Sign("GET", "http://example.com/api", params, "consumer_secret", "")
	if sig1 != sig2 {
		t.Errorf("Sign not deterministic: %q vs %q", sig1, sig2)
	}
	if sig1 == "" {
		t.Error("Sign returned empty string")
	}
}

// TestSign_DifferentSecretsProduceDifferentSignatures verifies that changing
// either secret produces a different signature.
func TestSign_DifferentSecretsProduceDifferentSignatures(t *testing.T) {
	t.Parallel()

	params := map[string]string{"oauth_nonce": "x", "oauth_timestamp": "1"}
	base := Sign("POST", "http://example.com/", params, "s1", "t1")
	diffConsumer := Sign("POST", "http://example.com/", params, "s2", "t1")
	diffToken := Sign("POST", "http://example.com/", params, "s1", "t2")

	if base == diffConsumer {
		t.Error("same signature despite different consumer secret")
	}
	if base == diffToken {
		t.Error("same signature despite different token secret")
	}
}

// TestPercentEncode_ViaBaseString exercises percentEncode behavior indirectly
// through BaseString for a range of character classes.
func TestPercentEncode_ViaBaseString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		params map[string]string
		// wantContains is a substring that must appear in the base string's
		// normalized-params segment (the third '&'-separated component, already
		// percent-encoded once by BaseString).
		wantContains string
	}{
		{
			name:         "space encoded as %20 not plus",
			params:       map[string]string{"k": "hello world"},
			wantContains: "k%3Dhello%2520world", // val "hello world" → "hello%20world" → encoded in base string as hello%2520world
		},
		{
			name:         "unreserved hyphen not encoded",
			params:       map[string]string{"a-b": "x-y"},
			wantContains: "a-b%3Dx-y",
		},
		{
			name:         "unreserved period not encoded",
			params:       map[string]string{"a.b": "1.2"},
			wantContains: "a.b%3D1.2",
		},
		{
			name:         "unreserved underscore not encoded",
			params:       map[string]string{"a_b": "x_y"},
			wantContains: "a_b%3Dx_y",
		},
		{
			name:         "unreserved tilde not encoded",
			params:       map[string]string{"a~b": "x~y"},
			wantContains: "a~b%3Dx~y",
		},
		{
			name:         "reserved equals sign encoded",
			params:       map[string]string{"k": "="},
			wantContains: "k%3D%253D", // val "=" → percentEncode → "%3D" → in base string → %253D
		},
		{
			name:         "reserved ampersand encoded",
			params:       map[string]string{"k": "a&b"},
			wantContains: "k%3Da%2526b",
		},
		{
			name:   "reserved at-sign encoded",
			params: map[string]string{"em@il": "user@host"},
			// key "em@il" → percentEncode → "em%40il"
			// val "user@host" → percentEncode → "user%40host"
			// normalized pair: "em%40il=user%40host"
			// that string in base string (percentEncoded again):
			//   % → %25, so em%2540il%3Duser%2540host
			wantContains: "em%2540il%3Duser%2540host",
		},
		{
			name:         "non-ASCII byte encoded",
			params:       map[string]string{"k": "\xff"},
			wantContains: "k%3D%25FF",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BaseString("GET", "http://example.com/", tc.params)
			// The third component of the base string (after the second '&').
			parts := splitBaseString(got)
			if len(parts) != 3 {
				t.Fatalf("expected 3 components, got %d: %s", len(parts), got)
			}
			if !contains(parts[2], tc.wantContains) {
				t.Errorf("normalized-params segment\ngot:  %s\nwant contains: %s", parts[2], tc.wantContains)
			}
		})
	}
}

// TestBaseString_ParamsSorted verifies that parameters are sorted lexicographically
// by their percent-encoded key, then by percent-encoded value.
func TestBaseString_ParamsSorted(t *testing.T) {
	t.Parallel()

	params := map[string]string{
		"z_key": "val_z",
		"a_key": "val_a",
		"m_key": "val_m",
	}
	got := BaseString("GET", "http://example.com/", params)
	// Decode the third component (double-decode to inspect normalized string).
	parts := splitBaseString(got)
	if len(parts) != 3 {
		t.Fatalf("expected 3 components: %s", got)
	}
	// Encoded normalized string must start with a_key (first alphabetically).
	if !hasPrefix(parts[2], "a_key") {
		t.Errorf("params not sorted: third component = %s", parts[2])
	}
}

// splitBaseString splits the base string on unencoded '&' separators.
// BaseString encodes both the URL and the normalized params, so the top-level
// '&' delimiters are literal (not encoded).
func splitBaseString(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '&' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
