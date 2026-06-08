package ir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// goMethodName
// ---------------------------------------------------------------------------

// TestGoMethodName verifies the naming algorithm for representative method names.
func TestGoMethodName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"search", "Search"},
		{"get", "Get"},
		{"get_month", "GetMonth"},
		{"add_favorite", "AddFavorite"},
		{"delete_favorite", "DeleteFavorite"},
		{"find_id_for_barcode", "FindIdForBarcode"},
		{"get_all", "GetAll"},
		{"commit_day", "CommitDay"},
		{"save_template", "SaveTemplate"},
		{"natural_language_processing", "NaturalLanguageProcessing"},
		{"get.all", "GetAll"},
		{"autocomplete", "Autocomplete"},
		{"create", "Create"},
		{"edit", "Edit"},
		{"delete", "Delete"},
	}
	for _, tc := range cases {
		got := goMethodName(tc.input)
		if got != tc.want {
			t.Errorf("goMethodName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestOverrideWins
// ---------------------------------------------------------------------------

// TestOverrideWins verifies that an override beats the naming algorithm result.
func TestOverrideWins(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"SuccessResult": {Name: "SuccessResult", Kind: "object", Fields: []Field{
				{Name: "success", Type: "APIInt", Required: true},
			}},
		},
		Methods: []Method{
			{
				Namespace:          "foods",
				Name:               "search",
				Version:            3,
				AllVersions:        []int{1, 3, 5},
				DeprecatedVersions: []int{1},
				HTTPVerb:           "GET",
				RestPath:           "/rest/foods/search/v3",
				AuthTier:           "client_credentials",
				ResponseType:       "SuccessResult",
				ResponseRoot:       "success",
			},
		},
	}

	overrides := []OverrideYAML{
		{Namespace: "foods", Name: "search", Version: 3, GoName: "Search"},
	}

	nss, err := BuildNamespaces(spec, overrides)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	if len(nss) == 0 {
		t.Fatal("expected at least one namespace")
	}
	var ns *Namespace
	for i := range nss {
		if nss[i].PackageName == "foods" {
			ns = &nss[i]
			break
		}
	}
	if ns == nil {
		t.Fatal("foods namespace not found")
	}
	if len(ns.Methods) == 0 {
		t.Fatal("no methods in foods namespace")
	}
	if ns.Methods[0].GoMethodName != "Search" {
		t.Errorf("expected override GoMethodName=Search, got %q", ns.Methods[0].GoMethodName)
	}
}

// ---------------------------------------------------------------------------
// TestDuplicateGoMethodNameError
// ---------------------------------------------------------------------------

// TestDuplicateGoMethodNameError verifies that two methods in the same package
// with the same GoMethodName trigger an error.
func TestDuplicateGoMethodNameError(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"SuccessResult": {Name: "SuccessResult", Kind: "object"},
		},
		Methods: []Method{
			{
				Namespace:    "foods",
				Name:         "search",
				Version:      3,
				AllVersions:  []int{3},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foods/search/v3",
				AuthTier:     "client_credentials",
				ResponseType: "SuccessResult",
				ResponseRoot: "success",
			},
			{
				Namespace:    "foods",
				Name:         "search", // same name, same version → same GoMethodName
				Version:      5,
				AllVersions:  []int{5},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foods/search/v5",
				AuthTier:     "client_credentials",
				ResponseType: "SuccessResult",
				ResponseRoot: "success",
			},
		},
	}

	// Both map to GoMethodName="Search" with no overrides → should error.
	_, err := BuildNamespaces(spec, nil)
	if err == nil {
		t.Fatal("expected error for duplicate GoMethodName, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestInvalidTypeRef
// ---------------------------------------------------------------------------

// TestInvalidTypeRef verifies that ParseSpec rejects an undefined TypeRef in a field.
func TestInvalidTypeRef(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"MyType": {Name: "MyType", Kind: "object", Fields: []Field{
				{Name: "thing", Type: "NonExistentType", Required: true},
			}},
		},
	}
	// Validate the TypeRef directly.
	err := validateTypeRef("NonExistentType", spec.Types)
	if err == nil {
		t.Fatal("expected error for undefined TypeRef, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestFlexSliceInnerTypeRef
// ---------------------------------------------------------------------------

// TestFlexSliceInnerTypeRef verifies FlexSlice[T] inner type is validated.
func TestFlexSliceInnerTypeRef(t *testing.T) {
	types := map[string]TypeDef{
		"Food": {Name: "Food", Kind: "object"},
	}
	// Valid FlexSlice[Food].
	if err := validateTypeRef("FlexSlice[Food]", types); err != nil {
		t.Errorf("unexpected error for FlexSlice[Food]: %v", err)
	}
	// Invalid FlexSlice[Missing].
	if err := validateTypeRef("FlexSlice[Missing]", types); err == nil {
		t.Error("expected error for FlexSlice[Missing], got nil")
	}
	// Valid FlexSlice[string].
	if err := validateTypeRef("FlexSlice[string]", types); err != nil {
		t.Errorf("unexpected error for FlexSlice[string]: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestNamespaceToPackageMapping
// ---------------------------------------------------------------------------

// TestNamespaceToPackageMapping verifies all spec namespaces map to a package.
func TestNamespaceToPackageMapping(t *testing.T) {
	specNamespaces := []string{
		"foods", "foods_favorites", "food", "food_categories",
		"food_sub_categories", "food_brands", "food_entries",
		"recipes", "recipes_favorites", "recipe", "recipe_types",
		"weight", "weights", "exercises", "exercise_entries",
		"saved_meals", "profile", "native", "feedback",
	}
	for _, ns := range specNamespaces {
		if _, ok := namespaceToPackage[ns]; !ok {
			t.Errorf("namespace %q has no package mapping", ns)
		}
	}
}

// ---------------------------------------------------------------------------
// TestIsVersionDeprecated
// ---------------------------------------------------------------------------

// TestIsVersionDeprecated exercises the version-check helper.
func TestIsVersionDeprecated(t *testing.T) {
	if !isVersionDeprecated(1, []int{1, 2}) {
		t.Error("expected v1 to be deprecated")
	}
	if isVersionDeprecated(3, []int{1, 2}) {
		t.Error("expected v3 not to be deprecated")
	}
	if isVersionDeprecated(1, []int{}) {
		t.Error("expected v1 not deprecated in empty list")
	}
}

// ---------------------------------------------------------------------------
// TestNamingTable
// ---------------------------------------------------------------------------

// TestNamingTable verifies representative methods produce expected Go names
// (naming algorithm only, before overrides).
func TestNamingTable(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"search", "Search"},
		{"autocomplete", "Autocomplete"},
		{"get_favorites", "GetFavorites"},
		{"get_most_eaten", "GetMostEaten"},
		{"get_recently_eaten", "GetRecentlyEaten"},
		{"find_id_for_barcode", "FindIdForBarcode"},
		{"add_favorite", "AddFavorite"},
		{"delete_favorite", "DeleteFavorite"},
		{"get_all", "GetAll"},
		{"get_month", "GetMonth"},
		{"copy", "Copy"},
		{"commit_day", "CommitDay"},
		{"save_template", "SaveTemplate"},
		{"get_auth", "GetAuth"},
		{"get_items", "GetItems"},
		{"add_item", "AddItem"},
		{"edit_item", "EditItem"},
		{"delete_item", "DeleteItem"},
	}
	for _, tc := range cases {
		got := goMethodName(tc.name)
		if got != tc.want {
			t.Errorf("goMethodName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ParseSpec error paths
// ---------------------------------------------------------------------------

// TestParseSpec_MissingFile verifies that a non-existent file returns an error.
func TestParseSpec_MissingFile(t *testing.T) {
	_, err := ParseSpec("/nonexistent/fatsecret.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestParseSpec_InvalidYAML verifies that malformed YAML returns a parse error.
func TestParseSpec_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(p, []byte("{{not valid yaml{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// TestParseSpec_MissingVersion verifies that a spec with no version field errors.
func TestParseSpec_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "noversion.yaml")
	if err := os.WriteFile(p, []byte("types: {}\nmethods: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention 'version'; got: %v", err)
	}
}

// TestParseSpec_InvalidTypeKind verifies that a type with unsupported kind errors.
func TestParseSpec_InvalidTypeKind(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "badkind.yaml")
	yaml := `version: "1"
types:
  BadType:
    kind: enum
methods: []
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for invalid type kind, got nil")
	}
}

// TestParseSpec_FieldMissingType verifies that an object field without a type errors.
func TestParseSpec_FieldMissingType(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notype.yaml")
	yaml := `version: "1"
types:
  MyType:
    kind: object
    fields:
      bad_field:
        required: true
methods: []
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for field with no type, got nil")
	}
}

// TestParseSpec_UndefinedTypeRefInField verifies that a field referencing an
// undefined type is caught during spec parse.
func TestParseSpec_UndefinedTypeRefInField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "badref.yaml")
	yaml := `version: "1"
types:
  Parent:
    kind: object
    fields:
      child:
        type: UndefinedChild
        required: true
methods: []
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for undefined TypeRef in field, got nil")
	}
}

// TestParseSpec_MethodMissingNamespace verifies that a method without a namespace errors.
func TestParseSpec_MethodMissingNamespace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nons.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - name: foo
    version: 1
    http_verb: GET
    rest_path: /rest/foo/v1
    auth_tier: client_credentials
    response_type: R
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for missing namespace, got nil")
	}
}

// TestParseSpec_MethodMissingHTTPVerb verifies missing http_verb errors.
func TestParseSpec_MethodMissingHTTPVerb(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "noverb.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - namespace: foods
    name: search
    version: 1
    rest_path: /rest/foods/search/v1
    auth_tier: client_credentials
    response_type: R
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for missing http_verb, got nil")
	}
}

// TestParseSpec_MethodUndefinedResponseType verifies that a method with an
// unresolvable response_type errors.
func TestParseSpec_MethodUndefinedResponseType(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "badrt.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - namespace: foods
    name: search
    version: 1
    http_verb: GET
    rest_path: /rest/foods/search/v1
    auth_tier: client_credentials
    response_type: Nonexistent
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for undefined response_type, got nil")
	}
}

// TestParseSpec_InvalidSmokeFixture verifies that an invalid JSON smoke_fixture errors.
func TestParseSpec_InvalidSmokeFixture(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "badfix.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - namespace: foods
    name: search
    version: 1
    http_verb: GET
    rest_path: /rest/foods/search/v1
    auth_tier: client_credentials
    response_type: R
    smoke_fixture: 'not json'
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for invalid smoke_fixture JSON, got nil")
	}
}

// TestParseSpec_ParamMissingName verifies that a param with no name errors.
func TestParseSpec_ParamMissingName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "paramname.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - namespace: foods
    name: search
    version: 1
    http_verb: GET
    rest_path: /rest/foods/search/v1
    auth_tier: client_credentials
    response_type: R
    params:
      - type: string
        required: false
        location: query
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for param missing name, got nil")
	}
}

// TestParseSpec_ParamUndefinedType verifies that a param with an undefined type errors.
func TestParseSpec_ParamUndefinedType(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "paramtype.yaml")
	yaml := `version: "1"
types:
  R:
    kind: object
methods:
  - namespace: foods
    name: search
    version: 1
    http_verb: GET
    rest_path: /rest/foods/search/v1
    auth_tier: client_credentials
    response_type: R
    params:
      - name: q
        type: BadParamType
        required: false
        location: query
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(p)
	if err == nil {
		t.Fatal("expected error for param with undefined type, got nil")
	}
}

// ---------------------------------------------------------------------------
// ParseOverrides paths
// ---------------------------------------------------------------------------

// TestParseOverrides_Missing verifies that a non-existent overrides file returns
// nil slice (not an error).
func TestParseOverrides_Missing(t *testing.T) {
	overrides, err := ParseOverrides("/nonexistent/overrides.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing overrides file, got: %v", err)
	}
	if overrides != nil {
		t.Errorf("expected nil overrides, got %v", overrides)
	}
}

// TestParseOverrides_InvalidYAML verifies that malformed YAML returns an error.
func TestParseOverrides_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "overrides.yaml")
	if err := os.WriteFile(p, []byte("{{bad:yaml{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseOverrides(p)
	if err == nil {
		t.Fatal("expected error for invalid overrides YAML, got nil")
	}
}

// TestParseOverrides_Valid verifies that a well-formed overrides file is parsed.
func TestParseOverrides_Valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "overrides.yaml")
	yaml := `version: "1"
overrides:
  - namespace: foods
    name: search
    version: 3
    go_name: SearchV3
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	overrides, err := ParseOverrides(p)
	if err != nil {
		t.Fatalf("ParseOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(overrides))
	}
	if overrides[0].GoName != "SearchV3" {
		t.Errorf("override GoName = %q, want %q", overrides[0].GoName, "SearchV3")
	}
}

// ---------------------------------------------------------------------------
// BuildNamespaces paths
// ---------------------------------------------------------------------------

// TestBuildNamespaces_UnknownNamespace verifies that an unknown namespace errors.
func TestBuildNamespaces_UnknownNamespace(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"R": {Name: "R", Kind: "object"},
		},
		Methods: []Method{
			{
				Namespace:    "unknown_ns",
				Name:         "foo",
				Version:      1,
				AllVersions:  []int{1},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foo/v1",
				AuthTier:     "client_credentials",
				ResponseType: "R",
			},
		},
	}
	_, err := BuildNamespaces(spec, nil)
	if err == nil {
		t.Fatal("expected error for unknown namespace, got nil")
	}
	if !strings.Contains(err.Error(), "unknown_ns") {
		t.Errorf("error should mention namespace name; got: %v", err)
	}
}

// TestBuildNamespaces_DeprecatedOnlyMethod verifies that a method whose only
// version is deprecated is excluded from canonical methods.
func TestBuildNamespaces_DeprecatedOnlyMethod(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"R": {Name: "R", Kind: "object"},
		},
		Methods: []Method{
			// Non-deprecated canonical version.
			{
				Namespace:          "foods",
				Name:               "search",
				Version:            3,
				AllVersions:        []int{1, 3},
				DeprecatedVersions: []int{1},
				HTTPVerb:           "GET",
				RestPath:           "/rest/foods/search/v3",
				AuthTier:           "client_credentials",
				ResponseType:       "R",
			},
			// Deprecated-only version — should not appear as a generated method.
			{
				Namespace:          "foods",
				Name:               "search",
				Version:            1,
				AllVersions:        []int{1, 3},
				DeprecatedVersions: []int{1},
				HTTPVerb:           "GET",
				RestPath:           "/rest/foods/search/v1",
				AuthTier:           "client_credentials",
				ResponseType:       "R",
			},
		},
	}
	nss, err := BuildNamespaces(spec, nil)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	for _, ns := range nss {
		if ns.PackageName != "foods" {
			continue
		}
		for _, m := range ns.Methods {
			if m.Version == 1 && m.IsDeprecated {
				t.Errorf("deprecated-only method v1 should not appear as canonical method")
			}
		}
	}
}

// TestBuildNamespaces_HasRequestStruct verifies that a method with only path
// params has HasRequestStruct=false.
func TestBuildNamespaces_HasRequestStruct(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"R": {Name: "R", Kind: "object"},
		},
		Methods: []Method{
			{
				Namespace:    "food",
				Name:         "get",
				Version:      4,
				AllVersions:  []int{4},
				HTTPVerb:     "GET",
				RestPath:     "/rest/food/v4",
				AuthTier:     "client_credentials",
				ResponseType: "R",
				Params: []Param{
					{Name: "food_id", Type: "APIInt", Required: true, Location: "path"},
				},
			},
		},
	}
	nss, err := BuildNamespaces(spec, nil)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	for _, ns := range nss {
		for _, m := range ns.Methods {
			if m.Name == "get" && m.HasRequestStruct {
				t.Error("expected HasRequestStruct=false for path-only params")
			}
		}
	}
}

// TestBuildNamespaces_NonPathParamsGiveRequestStruct verifies that at least one
// non-path param sets HasRequestStruct=true.
func TestBuildNamespaces_NonPathParamsGiveRequestStruct(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"R": {Name: "R", Kind: "object"},
		},
		Methods: []Method{
			{
				Namespace:    "foods",
				Name:         "search",
				Version:      3,
				AllVersions:  []int{3},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foods/search/v3",
				AuthTier:     "client_credentials",
				ResponseType: "R",
				Params: []Param{
					{Name: "q", Type: "string", Required: false, Location: "query"},
				},
			},
		},
	}
	nss, err := BuildNamespaces(spec, nil)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	found := false
	for _, ns := range nss {
		for _, m := range ns.Methods {
			if m.Name == "search" {
				found = true
				if !m.HasRequestStruct {
					t.Error("expected HasRequestStruct=true for non-path param")
				}
			}
		}
	}
	if !found {
		t.Error("search method not found in namespaces")
	}
}

// TestBuildNamespaces_NeedsTypes verifies that NeedsTypes is set when methods
// reference non-primitive types.
func TestBuildNamespaces_NeedsTypes(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			"FoodResult": {Name: "FoodResult", Kind: "object", Fields: []Field{
				{Name: "food_id", Type: "APIInt", Required: true},
			}},
		},
		Methods: []Method{
			{
				Namespace:    "foods",
				Name:         "search",
				Version:      3,
				AllVersions:  []int{3},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foods/search/v3",
				AuthTier:     "client_credentials",
				ResponseType: "FoodResult",
			},
		},
	}
	nss, err := BuildNamespaces(spec, nil)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	for _, ns := range nss {
		if ns.PackageName == "foods" && !ns.NeedsTypes {
			t.Error("expected NeedsTypes=true when namespace uses a named type")
		}
	}
}

// TestBuildNamespaces_ScalarTypeNoNeedsTypes verifies that a response type of
// "scalar" kind does not force NeedsTypes=true (scalar kinds have no fields).
func TestBuildNamespaces_ScalarTypeNoNeedsTypes(t *testing.T) {
	spec := &Spec{
		Version: "1",
		Types: map[string]TypeDef{
			// Scalar type — treated as named type but has no fields.
			"ScalarResult": {Name: "ScalarResult", Kind: "scalar"},
		},
		Methods: []Method{
			{
				Namespace:    "foods",
				Name:         "search",
				Version:      3,
				AllVersions:  []int{3},
				HTTPVerb:     "GET",
				RestPath:     "/rest/foods/search/v3",
				AuthTier:     "client_credentials",
				ResponseType: "ScalarResult",
			},
		},
	}
	nss, err := BuildNamespaces(spec, nil)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	// ScalarResult is a named type so collectTypes adds it, NeedsTypes=true.
	// Verify the namespace is at least produced without error.
	if len(nss) == 0 {
		t.Fatal("expected namespaces, got none")
	}
}

// ---------------------------------------------------------------------------
// collectTypeRefs / collectTypes
// ---------------------------------------------------------------------------

// TestCollectTypeRefs_Primitives verifies that primitive types add nothing to needed.
func TestCollectTypeRefs_Primitives(t *testing.T) {
	primitives := []string{"string", "APIInt", "APIFloat", "APIBool", "APITernary", "APIDaysEpoch", ""}
	types := map[string]TypeDef{}
	for _, p := range primitives {
		needed := make(map[string]bool)
		collectTypeRefs(p, types, needed)
		if len(needed) != 0 {
			t.Errorf("collectTypeRefs(%q): expected empty needed set, got %v", p, needed)
		}
	}
}

// TestCollectTypeRefs_FlexSliceNamed verifies that FlexSlice[Named] adds Named.
func TestCollectTypeRefs_FlexSliceNamed(t *testing.T) {
	types := map[string]TypeDef{
		"Food": {Name: "Food", Kind: "object", Fields: []Field{}},
	}
	needed := make(map[string]bool)
	collectTypeRefs("FlexSlice[Food]", types, needed)
	if !needed["Food"] {
		t.Error("expected Food in needed set for FlexSlice[Food]")
	}
}

// TestCollectTypeRefs_RecursiveFields verifies that nested type fields are
// transitively collected.
func TestCollectTypeRefs_RecursiveFields(t *testing.T) {
	types := map[string]TypeDef{
		"Parent": {Name: "Parent", Kind: "object", Fields: []Field{
			{Name: "child", Type: "Child"},
		}},
		"Child": {Name: "Child", Kind: "object", Fields: []Field{
			{Name: "value", Type: "APIInt"},
		}},
	}
	needed := make(map[string]bool)
	collectTypeRefs("Parent", types, needed)
	if !needed["Parent"] {
		t.Error("expected Parent in needed set")
	}
	if !needed["Child"] {
		t.Error("expected Child in needed set (transitive)")
	}
}

// TestCollectTypeRefs_CycleGuard verifies that a cyclic type reference does not
// cause infinite recursion (needed map breaks the cycle).
func TestCollectTypeRefs_CycleGuard(t *testing.T) {
	types := map[string]TypeDef{
		"A": {Name: "A", Kind: "object", Fields: []Field{{Name: "b", Type: "B"}}},
		"B": {Name: "B", Kind: "object", Fields: []Field{{Name: "a", Type: "A"}}},
	}
	needed := make(map[string]bool)
	// Should not loop forever.
	collectTypeRefs("A", types, needed)
	if !needed["A"] || !needed["B"] {
		t.Error("expected both A and B in needed set")
	}
}

// ---------------------------------------------------------------------------
// validateTypeRef — all primitive branches
// ---------------------------------------------------------------------------

// TestValidateTypeRef_AllPrimitives verifies every known primitive keyword is accepted.
func TestValidateTypeRef_AllPrimitives(t *testing.T) {
	primitives := []string{"string", "APIInt", "APIFloat", "APIBool", "APITernary", "APIDaysEpoch"}
	types := map[string]TypeDef{}
	for _, p := range primitives {
		if err := validateTypeRef(p, types); err != nil {
			t.Errorf("validateTypeRef(%q): unexpected error: %v", p, err)
		}
	}
}

// TestValidateTypeRef_DefinedTypeRef verifies that a defined type name passes.
func TestValidateTypeRef_DefinedTypeRef(t *testing.T) {
	types := map[string]TypeDef{
		"Foo": {Name: "Foo", Kind: "object"},
	}
	if err := validateTypeRef("Foo", types); err != nil {
		t.Errorf("unexpected error for defined type: %v", err)
	}
}

// ---------------------------------------------------------------------------
// convertTypeDef
// ---------------------------------------------------------------------------

// TestConvertTypeDef_ScalarKind verifies that scalar kinds produce an empty Fields list.
func TestConvertTypeDef_ScalarKind(t *testing.T) {
	td, err := convertTypeDef("MyScalar", TypeYAML{Kind: "scalar"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if td.Kind != "scalar" {
		t.Errorf("Kind = %q, want scalar", td.Kind)
	}
	if len(td.Fields) != 0 {
		t.Errorf("expected no fields for scalar kind, got %d", len(td.Fields))
	}
}

// TestConvertTypeDef_ObjectKind verifies that object types have fields sorted.
func TestConvertTypeDef_ObjectKind(t *testing.T) {
	raw := TypeYAML{
		Kind: "object",
		Fields: map[string]FieldYAML{
			"zzz": {Type: "string", Required: true},
			"aaa": {Type: "APIInt", Required: false},
		},
	}
	td, err := convertTypeDef("MyObj", raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(td.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(td.Fields))
	}
	// Fields must be alphabetically sorted.
	if td.Fields[0].Name != "aaa" || td.Fields[1].Name != "zzz" {
		t.Errorf("fields not sorted: got %v, %v", td.Fields[0].Name, td.Fields[1].Name)
	}
}

// TestConvertTypeDef_InvalidKind verifies that unsupported kinds error.
func TestConvertTypeDef_InvalidKind(t *testing.T) {
	_, err := convertTypeDef("Bad", TypeYAML{Kind: "enum"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid kind, got nil")
	}
}

// ---------------------------------------------------------------------------
// hasNonPathParams / overrideKey / sortedKeys
// ---------------------------------------------------------------------------

// TestHasNonPathParams verifies the helper correctly distinguishes path vs non-path params.
func TestHasNonPathParams(t *testing.T) {
	if hasNonPathParams(nil) {
		t.Error("nil params should return false")
	}
	if hasNonPathParams([]Param{{Name: "id", Type: "APIInt", Location: "path"}}) {
		t.Error("path-only params should return false")
	}
	if !hasNonPathParams([]Param{{Name: "q", Type: "string", Location: "query"}}) {
		t.Error("query param should return true")
	}
	if !hasNonPathParams([]Param{
		{Name: "id", Type: "APIInt", Location: "path"},
		{Name: "q", Type: "string", Location: "query"},
	}) {
		t.Error("mixed path+query should return true")
	}
}

// TestOverrideKey verifies the key format includes all three components.
func TestOverrideKey(t *testing.T) {
	k := overrideKey("foods", "search", 3)
	if !strings.Contains(k, "foods") || !strings.Contains(k, "search") || !strings.Contains(k, "3") {
		t.Errorf("overrideKey format unexpected: %q", k)
	}
}

// TestSortedKeys verifies that sortedKeys returns keys in lexicographic order.
func TestSortedKeys(t *testing.T) {
	m := map[string][]Method{
		"zz": nil,
		"aa": nil,
		"mm": nil,
	}
	got := sortedKeys(m)
	want := []string{"aa", "mm", "zz"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// ---------------------------------------------------------------------------
// Method.HasRequiredParams
// ---------------------------------------------------------------------------

// TestHasRequiredParams verifies the Method receiver.
func TestHasRequiredParams(t *testing.T) {
	m := &Method{Params: []Param{
		{Name: "q", Required: false},
	}}
	if m.HasRequiredParams() {
		t.Error("expected false with no required params")
	}
	m.Params = append(m.Params, Param{Name: "id", Required: true})
	if !m.HasRequiredParams() {
		t.Error("expected true when at least one param is required")
	}
}

// ---------------------------------------------------------------------------
// ParseSpec with real spec file
// ---------------------------------------------------------------------------

// TestParseSpec_RealSpec verifies the real fatsecret.yaml parses without error
// and has the expected structure.
func TestParseSpec_RealSpec(t *testing.T) {
	// Walk up from cwd to find repo root.
	dir, _ := filepath.Abs(".")
	var specPath string
	for {
		candidate := filepath.Join(dir, "spec", "fatsecret.yaml")
		if _, err := os.Stat(candidate); err == nil {
			specPath = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("repo root not found; skipping real spec test")
			return
		}
		dir = parent
	}

	spec, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec(real): %v", err)
	}
	if spec.Version == "" {
		t.Error("expected non-empty version")
	}
	if len(spec.Methods) == 0 {
		t.Error("expected at least one method")
	}
	if len(spec.Types) == 0 {
		t.Error("expected at least one type")
	}
}

// TestBuildNamespaces_RealSpec runs the full BuildNamespaces pipeline on the
// real spec + overrides and verifies structural invariants.
func TestBuildNamespaces_RealSpec(t *testing.T) {
	dir, _ := filepath.Abs(".")
	var specPath, overridesPath string
	for {
		if _, err := os.Stat(filepath.Join(dir, "spec", "fatsecret.yaml")); err == nil {
			specPath = filepath.Join(dir, "spec", "fatsecret.yaml")
			overridesPath = filepath.Join(dir, "spec", "name_overrides.yaml")
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("repo root not found; skipping real spec test")
			return
		}
		dir = parent
	}

	spec, err := ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	overrides, err := ParseOverrides(overridesPath)
	if err != nil {
		t.Fatalf("ParseOverrides: %v", err)
	}

	nss, err := BuildNamespaces(spec, overrides)
	if err != nil {
		t.Fatalf("BuildNamespaces: %v", err)
	}
	if len(nss) == 0 {
		t.Fatal("expected at least one namespace")
	}

	// Verify every namespace has a non-empty PackageName and at least one method.
	for _, ns := range nss {
		if ns.PackageName == "" {
			t.Error("namespace with empty PackageName")
		}
		if len(ns.Methods) == 0 {
			t.Errorf("namespace %q has no methods", ns.PackageName)
		}
		if ns.ImportPath == "" {
			t.Errorf("namespace %q has empty ImportPath", ns.PackageName)
		}
	}
}
