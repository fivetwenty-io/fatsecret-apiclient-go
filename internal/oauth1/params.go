// Package oauth1 provides low-level OAuth 1.0a (RFC 5849) signing primitives
// used by the FatSecret API client. Only [BaseString] and [Sign] are exported;
// all helper functions are unexported.
package oauth1

import (
	"fmt"
	"sort"
	"strings"
)

// percentEncode encodes s according to RFC 3986 §2.3 unreserved characters.
// The unreserved set is A-Za-z0-9 and the four characters - . _ ~; every other
// octet is percent-encoded as %XX with uppercase hexadecimal digits.
// This differs from url.QueryEscape, which uses '+' for space and does not
// restrict itself to the OAuth 1.0a-required unreserved character set.
func percentEncode(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 3) // worst-case: every byte encoded as 3 chars
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// isUnreserved reports whether c is in the RFC 3986 §2.3 unreserved set:
// ALPHA / DIGIT / "-" / "." / "_" / "~".
func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// normalizeParams percent-encodes each key and value in params, sorts the
// resulting pairs first by encoded key and then by encoded value (as required
// by RFC 5849 §3.4.1.3.2), and joins them as "key=value" pairs separated by
// '&'. The caller is responsible for excluding oauth_signature before calling.
func normalizeParams(params map[string]string) string {
	type kv struct{ key, val string }

	pairs := make([]kv, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, kv{
			key: percentEncode(k),
			val: percentEncode(v),
		})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].val < pairs[j].val
	})

	var b strings.Builder
	for idx, p := range pairs {
		if idx > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.key)
		b.WriteByte('=')
		b.WriteString(p.val)
	}
	return b.String()
}
