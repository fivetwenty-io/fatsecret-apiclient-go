package types

import (
	"encoding/json"
	"testing"
	"time"
)

// ---- APIInt ----------------------------------------------------------------

func TestAPIInt_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "null", input: `null`, want: 0},
		{name: "empty_quoted", input: `""`, want: 0},
		{name: "bare_zero", input: `0`, want: 0},
		{name: "bare_positive", input: `123`, want: 123},
		{name: "bare_negative", input: `-42`, want: -42},
		{name: "quoted_positive", input: `"1641"`, want: 1641},
		{name: "quoted_negative", input: `"-7"`, want: -7},
		{name: "quoted_zero", input: `"0"`, want: 0},
		{name: "large_id", input: `"9999999999"`, want: 9999999999},
		{name: "invalid_string", input: `"abc"`, wantErr: true},
		{name: "invalid_float", input: `"1.5"`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got APIInt
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%d)", got.Int64())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Int64() != tc.want {
				t.Fatalf("got %d, want %d", got.Int64(), tc.want)
			}
		})
	}
}

func TestAPIInt_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value APIInt
		want  string
	}{
		{name: "zero", value: 0, want: `0`},
		{name: "positive", value: 123, want: `123`},
		{name: "negative", value: -7, want: `-7`},
		{name: "large", value: 9999999999, want: `9999999999`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// ---- APIFloat --------------------------------------------------------------

func TestAPIFloat_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "null", input: `null`, want: 0},
		{name: "empty_quoted", input: `""`, want: 0},
		{name: "bare_zero", input: `0`, want: 0},
		{name: "bare_float", input: `29.55`, want: 29.55},
		{name: "bare_integer", input: `177`, want: 177},
		{name: "quoted_float", input: `"177.3"`, want: 177.3},
		{name: "quoted_zero", input: `"0"`, want: 0},
		{name: "quoted_negative", input: `"-3.14"`, want: -3.14},
		{name: "quoted_integer", input: `"500"`, want: 500},
		{name: "invalid_string", input: `"abc"`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got APIFloat
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%f)", got.Float64())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Float64() != tc.want {
				t.Fatalf("got %f, want %f", got.Float64(), tc.want)
			}
		})
	}
}

func TestAPIFloat_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value APIFloat
		want  string
	}{
		{name: "zero", value: 0, want: `0`},
		{name: "float", value: 29.55, want: `29.55`},
		{name: "negative", value: -3.14, want: `-3.14`},
		// FormatFloat 'f' prec=-1 uses minimum digits; 177.3 is not exactly representable
		// so we marshal and re-parse to avoid hardcoding the exact string representation.
		{name: "large_integer", value: 500, want: `500`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// TestAPIFloat_MarshalJSON_RoundTrip verifies that marshal→unmarshal is stable
// for values that have non-trivial float representations.
func TestAPIFloat_MarshalJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	originals := []float64{177.3, 0.1, 99.999, -12.5, 0}
	for _, orig := range originals {
		orig := orig
		t.Run("", func(t *testing.T) {
			t.Parallel()
			v := APIFloat(orig)
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var got APIFloat
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if got.Float64() != orig {
				t.Fatalf("round-trip mismatch: started %f, got %f", orig, got.Float64())
			}
		})
	}
}

// ---- APIBool ---------------------------------------------------------------

func TestAPIBool_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		// null / empty → false
		{name: "null", input: `null`, want: false},
		{name: "empty_quoted", input: `""`, want: false},
		// falsy forms
		{name: "bare_0", input: `0`, want: false},
		{name: "quoted_0", input: `"0"`, want: false},
		{name: "bare_false", input: `false`, want: false},
		{name: "quoted_false", input: `"false"`, want: false},
		{name: "quoted_no", input: `"no"`, want: false},
		{name: "quoted_False_upper", input: `"False"`, want: false},
		// truthy forms
		{name: "bare_1", input: `1`, want: true},
		{name: "quoted_1", input: `"1"`, want: true},
		{name: "bare_true", input: `true`, want: true},
		{name: "quoted_true", input: `"true"`, want: true},
		{name: "quoted_yes", input: `"yes"`, want: true},
		{name: "quoted_True_upper", input: `"True"`, want: true},
		// error forms
		{name: "unknown_string", input: `"maybe"`, wantErr: true},
		{name: "bare_2", input: `2`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got APIBool
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%v)", got.Bool())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Bool() != tc.want {
				t.Fatalf("got %v, want %v", got.Bool(), tc.want)
			}
		})
	}
}

func TestAPIBool_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value APIBool
		want  string
	}{
		{name: "true_emits_quoted_1", value: true, want: `"1"`},
		{name: "false_emits_quoted_0", value: false, want: `"0"`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// ---- APITernary ------------------------------------------------------------

func TestAPITernary_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    APITernary
		wantErr bool
	}{
		// valid
		{name: "bare_neg1", input: `-1`, want: TernaryUnknown},
		{name: "quoted_neg1", input: `"-1"`, want: TernaryUnknown},
		{name: "bare_0", input: `0`, want: TernaryFalse},
		{name: "quoted_0", input: `"0"`, want: TernaryFalse},
		{name: "bare_1", input: `1`, want: TernaryTrue},
		{name: "quoted_1", input: `"1"`, want: TernaryTrue},
		// errors
		{name: "null_errors", input: `null`, wantErr: true},
		{name: "empty_quoted_errors", input: `""`, wantErr: true},
		{name: "out_of_range_2", input: `2`, wantErr: true},
		{name: "out_of_range_neg2", input: `-2`, wantErr: true},
		{name: "non_numeric", input: `"abc"`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got APITernary
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%d)", got.Int8())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAPITernary_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value APITernary
		want  string
	}{
		{name: "unknown", value: TernaryUnknown, want: `"-1"`},
		{name: "false", value: TernaryFalse, want: `"0"`},
		{name: "true", value: TernaryTrue, want: `"1"`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

// ---- APIDaysEpoch ----------------------------------------------------------

func TestAPIDaysEpoch_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	epochDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{name: "null_zero", input: `null`, want: epochDate},
		{name: "empty_quoted_zero", input: `""`, want: epochDate},
		{name: "bare_zero", input: `0`, want: epochDate},
		{name: "quoted_zero", input: `"0"`, want: epochDate},
		{name: "bare_1", input: `1`, want: epochDate.Add(24 * time.Hour)},
		{name: "quoted_14289", input: `"14289"`, want: epochDate.Add(14289 * 24 * time.Hour)},
		// 14289 days from 1970-01-01 = 2009-02-14
		{name: "known_date", input: `"14289"`, want: time.Date(2009, 2, 14, 0, 0, 0, 0, time.UTC)},
		{name: "invalid_string", input: `"abc"`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got APIDaysEpoch
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Time().Equal(tc.want) {
				t.Fatalf("got %v, want %v", got.Time(), tc.want)
			}
		})
	}
}

func TestAPIDaysEpoch_MarshalJSON(t *testing.T) {
	t.Parallel()

	epochDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value APIDaysEpoch
		want  string
	}{
		{name: "epoch", value: APIDaysEpoch(epochDate), want: `"0"`},
		{name: "one_day", value: APIDaysEpoch(epochDate.Add(24 * time.Hour)), want: `"1"`},
		{name: "14289_days", value: APIDaysEpoch(time.Date(2009, 2, 14, 0, 0, 0, 0, time.UTC)), want: `"14289"`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tc.want {
				t.Fatalf("got %s, want %s", b, tc.want)
			}
		})
	}
}

func TestAPIDaysEpoch_RoundTrip(t *testing.T) {
	t.Parallel()

	dates := []time.Time{
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2009, 2, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	for _, orig := range dates {
		orig := orig
		t.Run(orig.Format("2006-01-02"), func(t *testing.T) {
			t.Parallel()
			v := APIDaysEpoch(orig)
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var got APIDaysEpoch
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if !got.Time().Equal(orig) {
				t.Fatalf("round-trip mismatch: started %v, got %v", orig, got.Time())
			}
		})
	}
}
