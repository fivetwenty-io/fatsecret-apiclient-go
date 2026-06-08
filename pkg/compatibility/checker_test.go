package compatibility

import (
	"testing"
)

func TestChecker_Supports(t *testing.T) {
	t.Parallel()

	c := NewChecker()

	tests := []struct {
		name      string
		namespace string
		method    string
		want      bool
	}{
		{
			name:      "unknown namespace returns false",
			namespace: "nonexistent_namespace",
			method:    "get",
			want:      false,
		},
		{
			name:      "unknown method in known namespace returns false",
			namespace: "food",
			method:    "nonexistent_method",
			want:      false,
		},
		{
			name:      "both unknown returns false",
			namespace: "ghost",
			method:    "phantom",
			want:      false,
		},
		{
			name:      "known method returns true",
			namespace: "food",
			method:    "get",
			want:      true,
		},
		{
			name:      "known method with empty scope returns true",
			namespace: "food_entries",
			method:    "create",
			want:      true,
		},
		{
			name:      "exercises.get returns true",
			namespace: "exercises",
			method:    "get",
			want:      true,
		},
		{
			name:      "native.nlp returns true",
			namespace: "native",
			method:    "nlp",
			want:      true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := c.Supports(tt.namespace, tt.method)
			if got != tt.want {
				t.Errorf("Supports(%q, %q) = %v, want %v", tt.namespace, tt.method, got, tt.want)
			}
		})
	}
}

func TestChecker_IsDeprecated(t *testing.T) {
	t.Parallel()

	c := NewChecker()

	tests := []struct {
		name      string
		namespace string
		method    string
		version   string
		want      bool
	}{
		{
			name:      "unknown namespace returns false",
			namespace: "ghost",
			method:    "get",
			version:   "1",
			want:      false,
		},
		{
			name:      "exercises.get version 1 is deprecated",
			namespace: "exercises",
			method:    "get",
			version:   "1",
			want:      true,
		},
		{
			name:      "exercises.get version 2 is not deprecated",
			namespace: "exercises",
			method:    "get",
			version:   "2",
			want:      false,
		},
		{
			name:      "food.get version 2 is deprecated",
			namespace: "food",
			method:    "get",
			version:   "2",
			want:      true,
		},
		{
			name:      "food.get version 3 is deprecated",
			namespace: "food",
			method:    "get",
			version:   "3",
			want:      true,
		},
		{
			name:      "food.get version 4 is not deprecated",
			namespace: "food",
			method:    "get",
			version:   "4",
			want:      false,
		},
		{
			name:      "foods.search version 1 is deprecated",
			namespace: "foods",
			method:    "search",
			version:   "1",
			want:      true,
		},
		{
			name:      "foods.search version 5 is not deprecated",
			namespace: "foods",
			method:    "search",
			version:   "5",
			want:      false,
		},
		{
			name:      "non-integer version string returns false",
			namespace: "food",
			method:    "get",
			version:   "v2",
			want:      false,
		},
		{
			name:      "empty version string returns false",
			namespace: "food",
			method:    "get",
			version:   "",
			want:      false,
		},
		{
			name:      "food_entries.get version 1 is deprecated",
			namespace: "food_entries",
			method:    "get",
			version:   "1",
			want:      true,
		},
		{
			name:      "food_entries.get version 2 is not deprecated",
			namespace: "food_entries",
			method:    "get",
			version:   "2",
			want:      false,
		},
		{
			name:      "recipe.get version 1 is deprecated",
			namespace: "recipe",
			method:    "get",
			version:   "1",
			want:      true,
		},
		{
			name:      "recipes.search version 2 is deprecated",
			namespace: "recipes",
			method:    "search",
			version:   "2",
			want:      true,
		},
		{
			name:      "recipes.search version 3 is not deprecated",
			namespace: "recipes",
			method:    "search",
			version:   "3",
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := c.IsDeprecated(tt.namespace, tt.method, tt.version)
			if got != tt.want {
				t.Errorf("IsDeprecated(%q, %q, %q) = %v, want %v",
					tt.namespace, tt.method, tt.version, got, tt.want)
			}
		})
	}
}

func TestChecker_LatestVersion(t *testing.T) {
	t.Parallel()

	c := NewChecker()

	tests := []struct {
		name      string
		namespace string
		method    string
		wantVer   string
		wantOK    bool
	}{
		{
			name:      "unknown method returns empty and false",
			namespace: "ghost",
			method:    "get",
			wantVer:   "",
			wantOK:    false,
		},
		{
			// exercises.get: Versions [1,2], Deprecated [1] → latest non-deprecated = 2
			name:      "exercises.get latest non-deprecated is 2",
			namespace: "exercises",
			method:    "get",
			wantVer:   "2",
			wantOK:    true,
		},
		{
			// food.get: Versions [2,3,4,5], Deprecated [2,3] → latest = 5
			name:      "food.get latest non-deprecated is 5",
			namespace: "food",
			method:    "get",
			wantVer:   "5",
			wantOK:    true,
		},
		{
			// foods.search: Versions [1,3,5], Deprecated [1] → latest = 5
			name:      "foods.search latest non-deprecated is 5",
			namespace: "foods",
			method:    "search",
			wantVer:   "5",
			wantOK:    true,
		},
		{
			// recipes.search: Versions [1,2,3], Deprecated [1,2] → latest = 3
			name:      "recipes.search latest non-deprecated is 3",
			namespace: "recipes",
			method:    "search",
			wantVer:   "3",
			wantOK:    true,
		},
		{
			// food_entries.get_month: Versions [1,2], Deprecated [1] → latest = 2
			name:      "food_entries.get_month latest non-deprecated is 2",
			namespace: "food_entries",
			method:    "get_month",
			wantVer:   "2",
			wantOK:    true,
		},
		{
			// feedback.submit: Versions [1], Deprecated [] → latest = 1
			name:      "feedback.submit latest is 1",
			namespace: "feedback",
			method:    "submit",
			wantVer:   "1",
			wantOK:    true,
		},
		{
			// saved_meals.get: Versions [1,2], Deprecated [1] → latest = 2
			name:      "saved_meals.get latest non-deprecated is 2",
			namespace: "saved_meals",
			method:    "get",
			wantVer:   "2",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotVer, gotOK := c.LatestVersion(tt.namespace, tt.method)
			if gotVer != tt.wantVer || gotOK != tt.wantOK {
				t.Errorf("LatestVersion(%q, %q) = (%q, %v), want (%q, %v)",
					tt.namespace, tt.method, gotVer, gotOK, tt.wantVer, tt.wantOK)
			}
		})
	}
}

func TestChecker_RequiresScope(t *testing.T) {
	t.Parallel()

	c := NewChecker()

	tests := []struct {
		name      string
		namespace string
		method    string
		wantScope string
		wantOK    bool
	}{
		{
			name:      "unknown method returns empty and false",
			namespace: "ghost",
			method:    "get",
			wantScope: "",
			wantOK:    false,
		},
		{
			name:      "feedback.submit requires feedback scope",
			namespace: "feedback",
			method:    "submit",
			wantScope: "feedback",
			wantOK:    true,
		},
		{
			name:      "native.nlp requires nlp scope",
			namespace: "native",
			method:    "nlp",
			wantScope: "nlp",
			wantOK:    true,
		},
		{
			name:      "native.recognize requires image-recognition scope",
			namespace: "native",
			method:    "recognize",
			wantScope: "image-recognition",
			wantOK:    true,
		},
		{
			name:      "food.find_id_for_barcode requires barcode scope",
			namespace: "food",
			method:    "find_id_for_barcode",
			wantScope: "barcode",
			wantOK:    true,
		},
		{
			name:      "exercises.get requires basic scope",
			namespace: "exercises",
			method:    "get",
			wantScope: "basic",
			wantOK:    true,
		},
		{
			name:      "food_brands.get_all requires premier scope",
			namespace: "food_brands",
			method:    "get_all",
			wantScope: "premier",
			wantOK:    true,
		},
		{
			// food_entries.create has empty scope (oauth1_delegated, no scope restriction)
			name:      "food_entries.create has empty scope but entry exists",
			namespace: "food_entries",
			method:    "create",
			wantScope: "",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotScope, gotOK := c.RequiresScope(tt.namespace, tt.method)
			if gotScope != tt.wantScope || gotOK != tt.wantOK {
				t.Errorf("RequiresScope(%q, %q) = (%q, %v), want (%q, %v)",
					tt.namespace, tt.method, gotScope, gotOK, tt.wantScope, tt.wantOK)
			}
		})
	}
}

func TestChecker_RequiresAuth(t *testing.T) {
	t.Parallel()

	c := NewChecker()

	tests := []struct {
		name      string
		namespace string
		method    string
		wantAuth  string
		wantOK    bool
	}{
		{
			name:      "unknown method returns empty and false",
			namespace: "ghost",
			method:    "get",
			wantAuth:  "",
			wantOK:    false,
		},
		{
			name:      "exercises.get requires client_credentials",
			namespace: "exercises",
			method:    "get",
			wantAuth:  "client_credentials",
			wantOK:    true,
		},
		{
			name:      "food_entries.create requires oauth1_delegated",
			namespace: "food_entries",
			method:    "create",
			wantAuth:  "oauth1_delegated",
			wantOK:    true,
		},
		{
			name:      "profile.create requires oauth1_signed",
			namespace: "profile",
			method:    "create",
			wantAuth:  "oauth1_signed",
			wantOK:    true,
		},
		{
			name:      "profile.get requires oauth1_delegated",
			namespace: "profile",
			method:    "get",
			wantAuth:  "oauth1_delegated",
			wantOK:    true,
		},
		{
			name:      "profile.get_auth requires oauth1_signed",
			namespace: "profile",
			method:    "get_auth",
			wantAuth:  "oauth1_signed",
			wantOK:    true,
		},
		{
			name:      "feedback.submit requires client_credentials",
			namespace: "feedback",
			method:    "submit",
			wantAuth:  "client_credentials",
			wantOK:    true,
		},
		{
			name:      "native.nlp requires client_credentials",
			namespace: "native",
			method:    "nlp",
			wantAuth:  "client_credentials",
			wantOK:    true,
		},
		{
			name:      "foods_favorites.get_favorites requires oauth1_delegated",
			namespace: "foods_favorites",
			method:    "get_favorites",
			wantAuth:  "oauth1_delegated",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAuth, gotOK := c.RequiresAuth(tt.namespace, tt.method)
			if gotAuth != tt.wantAuth || gotOK != tt.wantOK {
				t.Errorf("RequiresAuth(%q, %q) = (%q, %v), want (%q, %v)",
					tt.namespace, tt.method, gotAuth, gotOK, tt.wantAuth, tt.wantOK)
			}
		})
	}
}

// TestNewChecker_IndexSize verifies the Checker indexes all Matrix entries
// without collisions (generator must not produce duplicate namespace+name).
func TestNewChecker_IndexSize(t *testing.T) {
	t.Parallel()

	c := NewChecker()
	if got, want := len(c.index), len(Matrix); got != want {
		t.Errorf("index size = %d, want %d (Matrix has duplicates or indexing dropped entries)", got, want)
	}
}
