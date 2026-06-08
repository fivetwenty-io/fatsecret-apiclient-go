package errors

import (
	"errors"
	"strings"
	"testing"
)

// --- DispatchByFatSecretCode ---

func TestDispatchByFatSecretCode_ZeroReturnsNil(t *testing.T) {
	if err := DispatchByFatSecretCode(0, "ok", 200); err != nil {
		t.Fatalf("expected nil for code 0, got %v", err)
	}
}

func TestDispatchByFatSecretCode_KnownCodes(t *testing.T) { //nolint:cyclop // table-driven test necessarily mirrors the dispatch switch complexity
	cases := []struct {
		name       string
		code       int
		msg        string
		httpStatus int
		sentinel   error
		wantAuth   bool
		wantPerm   bool
		wantParam  bool
		wantRate   bool
	}{
		// code 1 — unknown error → generic APIError / ErrServer
		{name: "code1_unknown", code: 1, msg: "unknown error", httpStatus: 200, sentinel: ErrServer},

		// codes 2-9 — OAuth 1.0 → AuthenticationError / ErrUnauthorized
		{name: "code2_invalid_sig_method", code: 2, msg: "invalid signature method", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code3_invalid_consumer", code: 3, msg: "invalid consumer key", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code4_invalid_sig", code: 4, msg: "invalid signature", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code5_expired_ts", code: 5, msg: "invalid timestamp", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code6_used_nonce", code: 6, msg: "invalid nonce", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code7_expired_token", code: 7, msg: "expired token", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code8_invalid_token_alt", code: 8, msg: "invalid token", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "code9_invalid_access_token", code: 9, msg: "invalid access token", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},

		// code 10 — unknown method → generic APIError / ErrServer
		{name: "code10_unknown_method", code: 10, msg: "unknown method", httpStatus: 200, sentinel: ErrServer},

		// codes 11-12 — rate limit → RateLimitError / ErrRateLimited
		{name: "code11_app_limit", code: 11, msg: "app request limit reached", httpStatus: 200, sentinel: ErrRateLimited, wantRate: true},
		{name: "code12_user_limit", code: 12, msg: "user too many actions", httpStatus: 200, sentinel: ErrRateLimited, wantRate: true},

		// code 13 — OAuth 2.0 invalid token → AuthenticationError / ErrUnauthorized
		{name: "code13_oauth2_token", code: 13, msg: "invalid token", httpStatus: 200, sentinel: ErrUnauthorized, wantAuth: true},

		// code 14 — missing scope → PermissionError / ErrForbidden
		{name: "code14_missing_scope", code: 14, msg: "Missing scope: premier", httpStatus: 200, sentinel: ErrForbidden, wantPerm: true},

		// code 20 — system unavailable → generic APIError / ErrServer
		{name: "code20_unavailable", code: 20, msg: "system temporarily unavailable", httpStatus: 200, sentinel: ErrServer},

		// code 21 — invalid IP → PermissionError / ErrIPBlocked
		{name: "code21_invalid_ip", code: 21, msg: "invalid IP address detected", httpStatus: 200, sentinel: ErrIPBlocked, wantPerm: true},

		// code 22 — invalid request → generic APIError / ErrServer
		{name: "code22_invalid_request", code: 22, msg: "invalid request", httpStatus: 200, sentinel: ErrServer},

		// code 23 — API not found → generic APIError / ErrServer
		{name: "code23_api_not_found", code: 23, msg: "api not found", httpStatus: 200, sentinel: ErrServer},

		// code 24 — timeout → generic APIError / ErrTimeout
		{name: "code24_timeout", code: 24, msg: "timeout occurred", httpStatus: 200, sentinel: ErrTimeout},

		// codes 101-109 — parameter errors → ParameterError / ErrParameter
		{name: "code101_missing_param", code: 101, msg: "missing required parameter", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code102_invalid_int", code: 102, msg: "invalid integer value", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code103_invalid_double", code: 103, msg: "invalid double value", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code104_invalid_decimal", code: 104, msg: "invalid decimal value", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code105_invalid_long", code: 105, msg: "invalid long value", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code106_invalid_id", code: 106, msg: "invalid ID", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code107_out_of_range", code: 107, msg: "value out of range", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code108_invalid_type", code: 108, msg: "invalid type", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code109_char_limit", code: 109, msg: "character limit exceeded", httpStatus: 200, sentinel: ErrParameter, wantParam: true},

		// codes 201-211 — domain errors → ParameterError / ErrParameter
		{name: "code201_domain", code: 201, msg: "domain error 201", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code202_domain", code: 202, msg: "domain error 202", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code203_domain", code: 203, msg: "domain error 203", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code204_domain", code: 204, msg: "domain error 204", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code205_domain", code: 205, msg: "domain error 205", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code206_domain", code: 206, msg: "domain error 206", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code207_domain", code: 207, msg: "domain error 207", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code208_domain", code: 208, msg: "domain error 208", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code209_domain", code: 209, msg: "domain error 209", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code210_domain", code: 210, msg: "domain error 210", httpStatus: 200, sentinel: ErrParameter, wantParam: true},
		{name: "code211_domain", code: 211, msg: "domain error 211", httpStatus: 200, sentinel: ErrParameter, wantParam: true},

		// unknown code → generic APIError / ErrServer
		{name: "code_unknown_9999", code: 9999, msg: "something unknown", httpStatus: 200, sentinel: ErrServer},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := DispatchByFatSecretCode(tc.code, tc.msg, tc.httpStatus)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			// sentinel match via errors.Is
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(%v) = false, want true for sentinel %v", err, tc.sentinel)
			}

			// typed assertions
			if tc.wantAuth {
				var target *AuthenticationError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*AuthenticationError) = false, want true")
				}
			}
			if tc.wantPerm {
				var target *PermissionError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*PermissionError) = false, want true")
				}
			}
			if tc.wantParam {
				var target *ParameterError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*ParameterError) = false, want true")
				}
			}
			if tc.wantRate {
				var target *RateLimitError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*RateLimitError) = false, want true")
				}
			}

			// Error() starts with a lowercase letter and has no trailing period.
			// The message field is caller-supplied and may contain mixed case;
			// only the fixed prefix "fatsecret: ..." is required to be lowercase.
			s := err.Error()
			if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
				t.Errorf("Error() %q starts with uppercase", s)
			}
			if strings.HasSuffix(s, ".") {
				t.Errorf("Error() %q has trailing period", s)
			}
		})
	}
}

// TestDispatchByFatSecretCode_Code14_MissingScope verifies MissingScope extraction
// via errors.As for the three documented message patterns.
func TestDispatchByFatSecretCode_Code14_MissingScope(t *testing.T) {
	cases := []struct {
		name      string
		msg       string
		wantScope string
	}{
		{name: "colon_form", msg: "Missing scope: premier", wantScope: "premier"},
		{name: "space_form", msg: "missing scope barcode", wantScope: "barcode"},
		{name: "is_required_form", msg: "scope 'nlp' is required", wantScope: "nlp"},
		{name: "required_scope_colon", msg: "required scope: nlp", wantScope: "nlp"},
		{name: "no_scope_in_msg", msg: "access denied", wantScope: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := DispatchByFatSecretCode(14, tc.msg, 200)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			var pe *PermissionError
			if !errors.As(err, &pe) {
				t.Fatalf("errors.As(*PermissionError) = false")
			}
			if pe.MissingScope != tc.wantScope {
				t.Errorf("MissingScope = %q, want %q", pe.MissingScope, tc.wantScope)
			}
		})
	}
}

// TestDispatchByFatSecretCode_Code21_MissingScopeEmpty confirms code 21 (IP block)
// produces a PermissionError with empty MissingScope and ErrIPBlocked sentinel.
func TestDispatchByFatSecretCode_Code21_MissingScopeEmpty(t *testing.T) {
	err := DispatchByFatSecretCode(21, "invalid IP address detected", 200)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrIPBlocked) {
		t.Errorf("errors.Is(ErrIPBlocked) = false")
	}
	var pe *PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As(*PermissionError) = false")
	}
	if pe.MissingScope != "" {
		t.Errorf("MissingScope = %q, want empty for code 21", pe.MissingScope)
	}
}

// --- DispatchByStatus ---

func TestDispatchByStatus_Below400ReturnsNil(t *testing.T) {
	for _, status := range []int{200, 201, 204, 301, 302, 399} {
		if err := DispatchByStatus(status, ""); err != nil {
			t.Errorf("DispatchByStatus(%d) = %v, want nil", status, err)
		}
	}
}

func TestDispatchByStatus_KnownCodes(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		sentinel error
		wantAuth bool
		wantPerm bool
		wantRate bool
	}{
		{name: "401_unauthorized", status: 401, sentinel: ErrUnauthorized, wantAuth: true},
		{name: "403_forbidden", status: 403, sentinel: ErrForbidden, wantPerm: true},
		{name: "404_not_found", status: 404, sentinel: ErrNotFound},
		{name: "408_timeout", status: 408, sentinel: ErrTimeout},
		{name: "409_conflict", status: 409, sentinel: ErrConflict},
		{name: "429_rate_limit", status: 429, sentinel: ErrRateLimited, wantRate: true},
		{name: "500_server_error", status: 500, sentinel: ErrServer},
		{name: "502_bad_gateway", status: 502, sentinel: ErrServer},
		{name: "503_unavailable", status: 503, sentinel: ErrServer},
		// default 4xx (not in the explicit list) → ErrServer
		{name: "400_bad_request", status: 400, sentinel: ErrServer},
		{name: "405_method_not_allowed", status: 405, sentinel: ErrServer},
		{name: "422_unprocessable", status: 422, sentinel: ErrServer},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := DispatchByStatus(tc.status, "body text")
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			if !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(%v) = false, want true for sentinel %v", err, tc.sentinel)
			}

			if tc.wantAuth {
				var target *AuthenticationError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*AuthenticationError) = false, want true")
				}
			}
			if tc.wantPerm {
				var target *PermissionError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*PermissionError) = false, want true")
				}
			}
			if tc.wantRate {
				var target *RateLimitError
				if !errors.As(err, &target) {
					t.Errorf("errors.As(*RateLimitError) = false, want true")
				}
			}

			// HTTPStatus preserved in the underlying APIError
			var ae *APIError
			if errors.As(err, &ae) {
				if ae.HTTPStatus != tc.status {
					t.Errorf("APIError.HTTPStatus = %d, want %d", ae.HTTPStatus, tc.status)
				}
			}
		})
	}
}

// TestDispatchByStatus_BodyPreserved confirms the body string lands in APIError.Message.
func TestDispatchByStatus_BodyPreserved(t *testing.T) {
	const body = "gateway timeout response"
	err := DispatchByStatus(408, body)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatal("errors.As(*APIError) = false")
	}
	if ae.Message != body {
		t.Errorf("Message = %q, want %q", ae.Message, body)
	}
}

// --- Error() string contract ---

func TestAPIError_ErrorString_LowercaseNoTrailingPeriod(t *testing.T) {
	err := DispatchByFatSecretCode(101, "missing required parameter", 200)
	s := err.Error()
	// Fixed prefix "fatsecret: ..." starts lowercase.
	if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
		t.Errorf("Error() %q starts with uppercase", s)
	}
	if strings.HasSuffix(s, ".") {
		t.Errorf("Error() %q has trailing period", s)
	}
}

// TestAuthenticationError_ErrorsAs confirms errors.As succeeds on AuthenticationError
// returned from both dispatchers.
func TestAuthenticationError_ErrorsAs(t *testing.T) {
	t.Run("from_fsc_code", func(t *testing.T) {
		err := DispatchByFatSecretCode(13, "invalid token", 200)
		var ae *AuthenticationError
		if !errors.As(err, &ae) {
			t.Error("errors.As(*AuthenticationError) = false, want true")
		}
	})
	t.Run("from_status", func(t *testing.T) {
		err := DispatchByStatus(401, "unauthorized")
		var ae *AuthenticationError
		if !errors.As(err, &ae) {
			t.Error("errors.As(*AuthenticationError) = false, want true")
		}
	})
}
