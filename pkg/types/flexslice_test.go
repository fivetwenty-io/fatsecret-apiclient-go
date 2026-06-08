package types

import (
	"encoding/json"
	"testing"
)

// item is a simple struct used as the type parameter for FlexSlice tests.
type item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ---- FlexSlice UnmarshalJSON -----------------------------------------------

func TestFlexSlice_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantLen int
		wantNil bool
		wantErr bool
		// first element check (when wantLen > 0)
		wantFirst *item
	}{
		{
			name:    "null_gives_nil",
			input:   `null`,
			wantNil: true,
		},
		{
			name:    "empty_quoted_gives_nil",
			input:   `""`,
			wantNil: true,
		},
		{
			name:      "single_object_gives_len1",
			input:     `{"id":1,"name":"apple"}`,
			wantLen:   1,
			wantFirst: &item{ID: 1, Name: "apple"},
		},
		{
			name:      "array_one_element_gives_len1",
			input:     `[{"id":2,"name":"banana"}]`,
			wantLen:   1,
			wantFirst: &item{ID: 2, Name: "banana"},
		},
		{
			name:      "array_multiple_gives_lenN",
			input:     `[{"id":1,"name":"apple"},{"id":2,"name":"banana"},{"id":3,"name":"cherry"}]`,
			wantLen:   3,
			wantFirst: &item{ID: 1, Name: "apple"},
		},
		{
			name:    "empty_array_gives_len0",
			input:   `[]`,
			wantLen: 0,
			wantNil: false,
		},
		{
			name:    "invalid_json_errors",
			input:   `{broken`,
			wantErr: true,
		},
		{
			name:    "invalid_array_element_errors",
			input:   `[{broken}]`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got FlexSlice[item]
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (len=%d)", len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil slice, got len=%d", len(got))
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Fatalf("got len=%d, want len=%d", len(got), tc.wantLen)
			}
			if tc.wantFirst != nil && tc.wantLen > 0 {
				if got[0] != *tc.wantFirst {
					t.Fatalf("got first=%+v, want first=%+v", got[0], *tc.wantFirst)
				}
			}
		})
	}
}

// ---- FlexSlice MarshalJSON -------------------------------------------------

func TestFlexSlice_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value FlexSlice[item]
		want  string
	}{
		{
			name:  "nil_emits_empty_array",
			value: nil,
			want:  `[]`,
		},
		{
			name:  "empty_slice_emits_empty_array",
			value: FlexSlice[item]{},
			want:  `[]`,
		},
		{
			name:  "single_item_emits_array",
			value: FlexSlice[item]{{ID: 1, Name: "apple"}},
			want:  `[{"id":1,"name":"apple"}]`,
		},
		{
			name: "multiple_items_emits_array",
			value: FlexSlice[item]{
				{ID: 1, Name: "apple"},
				{ID: 2, Name: "banana"},
			},
			want: `[{"id":1,"name":"apple"},{"id":2,"name":"banana"}]`,
		},
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

// ---- FlexSlice round-trip --------------------------------------------------

// TestFlexSlice_RoundTrip verifies that a value unmarshalled from a single object
// re-marshals as an array (single-object ambiguity does not propagate on re-encode).
func TestFlexSlice_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("single_object_roundtrip_as_array", func(t *testing.T) {
		t.Parallel()
		input := `{"id":42,"name":"mango"}`
		var fs FlexSlice[item]
		if err := json.Unmarshal([]byte(input), &fs); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(fs) != 1 {
			t.Fatalf("expected len=1, got len=%d", len(fs))
		}
		b, err := json.Marshal(fs)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		// Must re-marshal as JSON array, not bare object.
		if string(b) != `[{"id":42,"name":"mango"}]` {
			t.Fatalf("got %s, want array form", b)
		}
		// Re-unmarshal from array must still yield len=1.
		var fs2 FlexSlice[item]
		if err := json.Unmarshal(b, &fs2); err != nil {
			t.Fatalf("second unmarshal error: %v", err)
		}
		if len(fs2) != 1 || fs2[0] != fs[0] {
			t.Fatalf("round-trip mismatch: got %+v", fs2)
		}
	})

	t.Run("array_roundtrip_stable", func(t *testing.T) {
		t.Parallel()
		input := `[{"id":1,"name":"a"},{"id":2,"name":"b"}]`
		var fs FlexSlice[item]
		if err := json.Unmarshal([]byte(input), &fs); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		b, err := json.Marshal(fs)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var fs2 FlexSlice[item]
		if err := json.Unmarshal(b, &fs2); err != nil {
			t.Fatalf("second unmarshal error: %v", err)
		}
		if len(fs2) != len(fs) {
			t.Fatalf("length mismatch: got %d, want %d", len(fs2), len(fs))
		}
		for i := range fs {
			if fs2[i] != fs[i] {
				t.Fatalf("element %d mismatch: got %+v, want %+v", i, fs2[i], fs[i])
			}
		}
	})
}

// ---- FlexSlice Items() -----------------------------------------------------

func TestFlexSlice_Items(t *testing.T) {
	t.Parallel()

	t.Run("nil_returns_nil", func(t *testing.T) {
		t.Parallel()
		var fs FlexSlice[item]
		if fs.Items() != nil {
			t.Fatalf("expected nil, got non-nil")
		}
	})

	t.Run("non_nil_returns_underlying_slice", func(t *testing.T) {
		t.Parallel()
		fs := FlexSlice[item]{{ID: 1, Name: "x"}}
		items := fs.Items()
		if len(items) != 1 || items[0].ID != 1 {
			t.Fatalf("unexpected items: %+v", items)
		}
	})
}

// ---- FlexSlice with scalar type parameter ----------------------------------

// TestFlexSlice_ScalarTypeParam checks that FlexSlice works with non-struct types
// (e.g. string), since T is constrained to "any".
func TestFlexSlice_ScalarTypeParam(t *testing.T) {
	t.Parallel()

	t.Run("string_single_object_wraps", func(t *testing.T) {
		t.Parallel()
		// A bare JSON string is neither '[' nor '{', so falls into the default
		// single-item branch and is wrapped in a len-1 slice.
		var fs FlexSlice[string]
		if err := json.Unmarshal([]byte(`"hello"`), &fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fs) != 1 || fs[0] != "hello" {
			t.Fatalf("got %+v, want [hello]", fs)
		}
	})

	t.Run("string_array", func(t *testing.T) {
		t.Parallel()
		var fs FlexSlice[string]
		if err := json.Unmarshal([]byte(`["a","b","c"]`), &fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fs) != 3 {
			t.Fatalf("got len=%d, want 3", len(fs))
		}
		expected := []string{"a", "b", "c"}
		for i, v := range fs {
			if v != expected[i] {
				t.Fatalf("element %d: got %q, want %q", i, v, expected[i])
			}
		}
	})
}
