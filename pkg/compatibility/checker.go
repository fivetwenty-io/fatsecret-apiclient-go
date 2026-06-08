// Package compatibility provides runtime lookup of FatSecret API method
// versioning metadata sourced from the generated Matrix catalog.
package compatibility

import (
	"strconv"
)

// entryKey is the composite index key for a Matrix entry.
type entryKey struct {
	namespace string
	name      string
}

// Checker provides O(1) lookup into the generated Matrix by namespace and
// method name.  Build one with NewChecker and share it across goroutines —
// it is read-only after construction.
type Checker struct {
	index map[entryKey]*Entry
}

// NewChecker constructs a Checker by indexing the generated Matrix.
// Duplicate namespace+name pairs (which the generator must not produce) are
// silently last-write-wins; the generated catalog guarantees uniqueness.
func NewChecker() *Checker {
	idx := make(map[entryKey]*Entry, len(Matrix))
	for i := range Matrix {
		e := &Matrix[i]
		idx[entryKey{e.Namespace, e.Name}] = e
	}
	return &Checker{index: idx}
}

// lookup returns the entry for the given namespace+name, or nil.
func (c *Checker) lookup(namespace, name string) *Entry {
	return c.index[entryKey{namespace, name}]
}

// Supports reports whether the given method exists in the Matrix.
// Returns false for any unknown namespace or name.
func (c *Checker) Supports(namespace, name string) bool {
	return c.lookup(namespace, name) != nil
}

// IsDeprecated reports whether the given version string is listed in the
// entry's Deprecated slice.  The version must be a decimal integer string
// matching the integer representation used in the Matrix (e.g. "1", "2").
// Returns false for unknown methods, non-integer version strings, and
// versions that are not deprecated.
func (c *Checker) IsDeprecated(namespace, name, version string) bool {
	e := c.lookup(namespace, name)
	if e == nil {
		return false
	}
	v, err := strconv.Atoi(version)
	if err != nil {
		return false
	}
	for _, d := range e.Deprecated {
		if d == v {
			return true
		}
	}
	return false
}

// LatestVersion returns the highest version number that is NOT listed in the
// entry's Deprecated slice, formatted as a decimal integer string.
// Returns ("", false) when the method is unknown or every version is
// deprecated.
func (c *Checker) LatestVersion(namespace, name string) (string, bool) {
	e := c.lookup(namespace, name)
	if e == nil {
		return "", false
	}

	depSet := make(map[int]struct{}, len(e.Deprecated))
	for _, d := range e.Deprecated {
		depSet[d] = struct{}{}
	}

	best := -1
	for _, v := range e.Versions {
		if _, deprecated := depSet[v]; deprecated {
			continue
		}
		if v > best {
			best = v
		}
	}
	if best < 0 {
		return "", false
	}
	return strconv.Itoa(best), true
}

// RequiresScope returns the OAuth2 scope string required by the method and
// true when the method exists.  An empty scope string means the method does
// not require a specific scope beyond authentication.  Returns ("", false)
// for unknown methods.
func (c *Checker) RequiresScope(namespace, name string) (string, bool) {
	e := c.lookup(namespace, name)
	if e == nil {
		return "", false
	}
	return e.Scope, true
}

// RequiresAuth returns the authentication tier string for the method and true
// when the method exists.  Possible tier values are defined by the API spec
// (e.g. "client_credentials", "oauth1_delegated", "oauth1_signed").
// Returns ("", false) for unknown methods.
func (c *Checker) RequiresAuth(namespace, name string) (string, bool) {
	e := c.lookup(namespace, name)
	if e == nil {
		return "", false
	}
	return e.AuthTier, true
}
