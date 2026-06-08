package types

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// epoch is the reference point for APIDaysEpoch: midnight UTC on 1970-01-01.
var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// stripQuotes removes a single pair of surrounding double-quote characters from s if
// present. It does not unescape JSON escape sequences; callers rely on the raw token
// already being a plain ASCII number or keyword with no embedded escapes.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// isNull reports whether the raw JSON token represents JSON null.
func isNull(b []byte) bool {
	return len(b) == 4 && b[0] == 'n' && b[1] == 'u' && b[2] == 'l' && b[3] == 'l'
}

// APIInt is an int64 that unmarshals from a JSON string, a JSON number, an empty string,
// or JSON null. FatSecret returns all integer fields as quoted JSON strings
// (e.g. "food_id":"1641"), so this type is required for any numeric ID or count field.
//
// UnmarshalJSON accepts:
//   - null              → 0
//   - ""                → 0
//   - 123               → 123
//   - "123"             → 123
//
// MarshalJSON emits a bare JSON number (e.g. 123), not a quoted string.
type APIInt int64

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (a *APIInt) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || isNull(b) {
		*a = 0
		return nil
	}
	s := stripQuotes(string(b))
	if s == "" {
		*a = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("types: APIInt: cannot parse %q: %w", string(b), err)
	}
	*a = APIInt(n)
	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It emits a bare JSON number.
func (a APIInt) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, int64(a), 10), nil
}

// Int64 returns the underlying int64 value.
func (a APIInt) Int64() int64 { return int64(a) }

// APIFloat is a float64 that unmarshals from a JSON string, a JSON number, an empty
// string, or JSON null. FatSecret returns all decimal fields (calories, protein, fat,
// etc.) as quoted JSON strings (e.g. "calories":"177.3").
//
// UnmarshalJSON accepts:
//   - null              → 0
//   - ""                → 0
//   - 29.55             → 29.55
//   - "29.55"           → 29.55
//
// MarshalJSON emits a bare JSON number with the minimum digits needed to represent the
// value exactly (using strconv 'f' format with prec=-1).
type APIFloat float64

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (a *APIFloat) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || isNull(b) {
		*a = 0
		return nil
	}
	s := stripQuotes(string(b))
	if s == "" {
		*a = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("types: APIFloat: cannot parse %q: %w", string(b), err)
	}
	*a = APIFloat(f)
	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It emits a bare JSON number.
func (a APIFloat) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(a), 'f', -1, 64)), nil
}

// Float64 returns the underlying float64 value.
func (a APIFloat) Float64() float64 { return float64(a) }

// APIBool is a bool that unmarshals from FatSecret's non-standard boolean representations.
// FatSecret does not use JSON native true/false; it uses "0"/"1" string integers for the
// is_default field and may use other textual forms in less-common endpoints.
//
// UnmarshalJSON accepts:
//   - null, ""          → false
//   - 0, "0", false, "false", "no"  → false
//   - 1, "1", true,  "true",  "yes" → true
//
// MarshalJSON emits the quoted string "1" or "0" to match FatSecret's wire convention.
type APIBool bool

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (a *APIBool) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || isNull(b) {
		*a = false
		return nil
	}
	// Strip surrounding quotes so that both bare and string-wrapped forms unify.
	s := stripQuotes(string(b))
	switch strings.ToLower(s) {
	case "1", "true", "yes":
		*a = true
	case "0", "false", "no", "":
		*a = false
	default:
		return fmt.Errorf("types: APIBool: unexpected value %q", string(b))
	}
	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It emits "1" or "0" as quoted strings.
func (a APIBool) MarshalJSON() ([]byte, error) {
	if bool(a) {
		return []byte(`"1"`), nil
	}
	return []byte(`"0"`), nil
}

// Bool returns the underlying bool value.
func (a APIBool) Bool() bool { return bool(a) }

// APITernary is an int8 representing a three-valued allergen or preference flag.
// FatSecret uses -1 (Unknown), 0 (False/No), and 1 (True/Yes) delivered as quoted
// JSON strings (e.g. "value":"-1").
//
// Named constants TernaryUnknown, TernaryFalse, and TernaryTrue are provided for
// readable comparisons.
//
// UnmarshalJSON accepts:
//   - -1, "-1"  → TernaryUnknown
//   - 0,  "0"   → TernaryFalse
//   - 1,  "1"   → TernaryTrue
//
// Any other value, including null or empty string, returns an error because there is
// no unambiguous default for a ternary field.
//
// MarshalJSON emits the value as a quoted string to match the FatSecret wire format.
type APITernary int8

const (
	// TernaryUnknown represents an unknown allergen or preference state (-1).
	TernaryUnknown APITernary = -1

	// TernaryFalse represents a false/no allergen or preference state (0).
	TernaryFalse APITernary = 0

	// TernaryTrue represents a true/yes allergen or preference state (1).
	TernaryTrue APITernary = 1
)

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (a *APITernary) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || isNull(b) {
		return fmt.Errorf("types: APITernary: null is not a valid ternary value")
	}
	s := stripQuotes(string(b))
	if s == "" {
		return fmt.Errorf("types: APITernary: empty string is not a valid ternary value")
	}
	n, err := strconv.ParseInt(s, 10, 8)
	if err != nil {
		return fmt.Errorf("types: APITernary: cannot parse %q: %w", string(b), err)
	}
	if n != -1 && n != 0 && n != 1 {
		return fmt.Errorf("types: APITernary: value %d is out of range [-1, 0, 1]", n)
	}
	*a = APITernary(n)
	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It emits the value as a quoted string.
func (a APITernary) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.Itoa(int(a)) + `"`), nil
}

// Int8 returns the underlying int8 value (-1, 0, or 1).
func (a APITernary) Int8() int8 { return int8(a) }

// APIDaysEpoch is a time.Time decoded from a days-since-Unix-epoch integer.
// FatSecret's date_int field stores dates as the number of days elapsed since
// 1970-01-01 (not seconds, not milliseconds — whole days). The value arrives as a
// quoted JSON string (e.g. "date_int":"14289").
//
// UnmarshalJSON accepts the same forms as APIInt (null, bare number, quoted number,
// empty string) and converts the day count to a UTC midnight time.Time.
//
// MarshalJSON emits the day count as a quoted JSON string to match the FatSecret wire
// format, enabling round-trip encoding without loss.
type APIDaysEpoch time.Time

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (a *APIDaysEpoch) UnmarshalJSON(b []byte) error {
	var days APIInt
	if err := days.UnmarshalJSON(b); err != nil {
		return fmt.Errorf("types: APIDaysEpoch: %w", err)
	}
	*a = APIDaysEpoch(epoch.Add(time.Duration(days) * 24 * time.Hour))
	return nil
}

// MarshalJSON implements [encoding/json.Marshaler]. It emits the day count since
// 1970-01-01 UTC as a quoted JSON string (e.g. "14289").
func (a APIDaysEpoch) MarshalJSON() ([]byte, error) {
	t := time.Time(a)
	days := int64(t.Sub(epoch) / (24 * time.Hour))
	return []byte(`"` + strconv.FormatInt(days, 10) + `"`), nil
}

// Time returns the underlying time.Time value, always in UTC at midnight.
func (a APIDaysEpoch) Time() time.Time { return time.Time(a) }
