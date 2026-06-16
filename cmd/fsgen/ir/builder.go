package ir

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// moduleRoot is the Go module path; used to construct ImportPaths.
const moduleRoot = "github.com/fivetwenty-io/fatsecret-apiclient-go"

// namespaceToPackage maps spec namespace names to their Go package names.
// Multiple spec namespaces can map to the same Go package (P2 §4 collapse).
var namespaceToPackage = map[string]string{
	"foods":               "foods",
	"foods_favorites":     "foods",
	"food":                "food",
	"food_categories":     "food_categories",
	"food_sub_categories": "food_sub_categories",
	"food_brands":         "food_brands",
	"food_entries":        "food_entries",
	"recipes":             "recipes",
	"recipes_favorites":   "recipes",
	"recipe":              "recipe",
	"recipe_types":        "recipe_types",
	"weight":              "weight",
	"weights":             "weight",
	"exercises":           "exercises",
	"exercise_entries":    "exercise_entries",
	"saved_meals":         "saved_meals",
	"profile":             "profile",
	"native":              "native",
	"feedback":            "feedback",
}

// deprecatedMethodKeys tracks which (namespace+name+version) combos are deprecated-only
// entries that should appear in the matrix but NOT be generated as Service methods.
// A method is deprecated-only when its Version appears in DeprecatedVersions AND
// there exists another entry with the same namespace+name but a higher non-deprecated version.

// ParseSpec reads and validates the spec YAML file, returning a populated Spec.
// Errors include file read failures, YAML parse errors, missing required fields,
// undefined TypeRefs, and invalid smoke_fixture JSON.
func ParseSpec(specPath string) (*Spec, error) { //nolint:cyclop // validates many required YAML fields in sequence; extraction would spread related validation across helpers
	data, err := os.ReadFile(specPath) // #nosec G304 -- spec path is a trusted CLI argument
	if err != nil {
		return nil, fmt.Errorf("ir: read spec %q: %w", specPath, err)
	}
	var raw SpecYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("ir: parse spec YAML: %w", err)
	}
	if raw.Version == "" {
		return nil, fmt.Errorf("ir: spec missing version field")
	}

	spec := &Spec{
		Version: raw.Version,
		Types:   make(map[string]TypeDef, len(raw.Types)),
	}

	// Convert raw types; preserve field order by sorting field names for determinism.
	for typeName, tyRaw := range raw.Types {
		td, err := convertTypeDef(typeName, tyRaw, raw.Types)
		if err != nil {
			return nil, fmt.Errorf("ir: type %q: %w", typeName, err)
		}
		spec.Types[typeName] = td
	}

	// Validate all FlexSlice[T] and object field TypeRefs are defined.
	for typeName, td := range spec.Types {
		for _, f := range td.Fields {
			if err := validateTypeRef(f.Type, spec.Types); err != nil {
				return nil, fmt.Errorf("ir: type %q field %q: %w", typeName, f.Name, err)
			}
		}
	}

	// Convert methods.
	for i, mRaw := range raw.Methods {
		m, err := convertMethod(mRaw)
		if err != nil {
			return nil, fmt.Errorf("ir: method[%d] (%s.%s v%d): %w", i, mRaw.Namespace, mRaw.Name, mRaw.Version, err)
		}
		// Validate response_type exists.
		if _, ok := spec.Types[m.ResponseType]; !ok {
			return nil, fmt.Errorf("ir: method %s.%s v%d: response_type %q not in types", m.Namespace, m.Name, m.Version, m.ResponseType)
		}
		// Validate param types.
		for _, p := range m.Params {
			if err := validateTypeRef(p.Type, spec.Types); err != nil {
				return nil, fmt.Errorf("ir: method %s.%s v%d param %q: %w", m.Namespace, m.Name, m.Version, p.Name, err)
			}
		}
		// Validate smoke_fixture is valid JSON when present.
		if m.SmokeFixture != "" {
			if !json.Valid([]byte(m.SmokeFixture)) {
				return nil, fmt.Errorf("ir: method %s.%s v%d: smoke_fixture is not valid JSON", m.Namespace, m.Name, m.Version)
			}
		}
		spec.Methods = append(spec.Methods, m)
	}

	return spec, nil
}

// ParseOverrides reads and parses the name_overrides.yaml file.
// Returns an empty slice (not an error) when the file does not exist.
func ParseOverrides(overridesPath string) ([]OverrideYAML, error) {
	data, err := os.ReadFile(overridesPath) // #nosec G304 -- spec path is a trusted CLI argument
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ir: read overrides %q: %w", overridesPath, err)
	}
	var raw OverridesYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("ir: parse overrides YAML: %w", err)
	}
	return raw.Overrides, nil
}

// BuildNamespaces groups methods by Go package, applies naming algorithm and
// overrides, resolves TypeDefs, and returns one Namespace per Go package.
// Returns an error if any GoMethodName duplicates within a package remain after
// applying overrides.
func BuildNamespaces(spec *Spec, overrides []OverrideYAML) ([]Namespace, error) { //nolint:cyclop // orchestrates namespace deduplication, override application, and type resolution; extraction would not reduce decision points
	// Build override lookup: (namespace, name, version) → go_name.
	overrideMap := make(map[string]string)
	for _, o := range overrides {
		key := overrideKey(o.Namespace, o.Name, o.Version)
		overrideMap[key] = o.GoName
	}

	// Identify deprecated-only methods: version appears in deprecated_versions AND
	// there is another entry in the same (namespace, name) group with a non-deprecated version.
	type nsNameKey struct{ ns, name string }
	// Map from (ns,name) → set of non-deprecated versions.
	nonDeprecated := make(map[nsNameKey]map[int]bool)
	for _, m := range spec.Methods {
		key := nsNameKey{m.Namespace, m.Name}
		if nonDeprecated[key] == nil {
			nonDeprecated[key] = make(map[int]bool)
		}
		isThisVersionDeprecated := false
		for _, dv := range m.DeprecatedVersions {
			if dv == m.Version {
				isThisVersionDeprecated = true
				break
			}
		}
		if !isThisVersionDeprecated {
			nonDeprecated[key][m.Version] = true
		}
	}

	// Apply naming algorithm and overrides to all methods.
	allMethods := make([]Method, len(spec.Methods))
	for i, m := range spec.Methods {
		pkg, ok := namespaceToPackage[m.Namespace]
		if !ok {
			return nil, fmt.Errorf("ir: unknown namespace %q (add to namespaceToPackage)", m.Namespace)
		}
		m.GoPackage = pkg

		// Determine if this entry's version is deprecated.
		m.IsDeprecated = isVersionDeprecated(m.Version, m.DeprecatedVersions)

		// Apply naming algorithm.
		goName := goMethodName(m.Name)

		// Apply override if present.
		key := overrideKey(m.Namespace, m.Name, m.Version)
		if ov, ok := overrideMap[key]; ok {
			goName = ov
		}
		m.GoMethodName = goName

		// HasRequestStruct: true when ≥1 non-path param exists.
		m.HasRequestStruct = hasNonPathParams(m.Params)

		allMethods[i] = m
	}

	// Group methods by Go package.
	pkgMethods := make(map[string][]Method)
	for _, m := range allMethods {
		pkgMethods[m.GoPackage] = append(pkgMethods[m.GoPackage], m)
	}

	// For each package, filter to canonical (non-deprecated-only) methods and
	// check for duplicate GoMethodNames.
	var namespaces []Namespace
	pkgNames := sortedKeys(pkgMethods)

	for _, pkg := range pkgNames {
		methods := pkgMethods[pkg]
		var canonical []Method

		for _, m := range methods {
			key := nsNameKey{m.Namespace, m.Name}
			ndMap := nonDeprecated[key]
			// Include method if:
			//   - its version is NOT deprecated, OR
			//   - there is no non-deprecated version for this (ns,name) (standalone deprecated entry)
			if len(ndMap) == 0 || ndMap[m.Version] {
				canonical = append(canonical, m)
			}
			// deprecated-only entries go to matrix only; not generated as methods
		}

		// Sort canonical methods by GoMethodName for determinism.
		sort.Slice(canonical, func(i, j int) bool {
			return canonical[i].GoMethodName < canonical[j].GoMethodName
		})

		// Check duplicate GoMethodName within package.
		seen := make(map[string]string)
		for _, m := range canonical {
			prev, dup := seen[m.GoMethodName]
			if dup {
				return nil, fmt.Errorf("ir: package %q: duplicate GoMethodName %q (from %s.%s v%d and %s)",
					pkg, m.GoMethodName, m.Namespace, m.Name, m.Version, prev)
			}
			seen[m.GoMethodName] = fmt.Sprintf("%s.%s v%d", m.Namespace, m.Name, m.Version)
		}

		// Collect types referenced by this package's methods.
		types, err := collectTypes(canonical, spec.Types)
		if err != nil {
			return nil, fmt.Errorf("ir: package %q: %w", pkg, err)
		}
		needsTypes := len(types) > 0

		ns := Namespace{
			PackageName: pkg,
			ImportPath:  moduleRoot + "/pkg/api/" + pkg,
			Methods:     canonical,
			Types:       types,
			NeedsTypes:  needsTypes,
		}
		namespaces = append(namespaces, ns)
	}

	return namespaces, nil
}

// convertTypeDef converts a raw YAML type to an IR TypeDef with sorted fields.
func convertTypeDef(name string, raw TypeYAML, _ map[string]TypeYAML) (TypeDef, error) {
	td := TypeDef{Name: name, Kind: raw.Kind}
	if raw.Kind != "object" && raw.Kind != "scalar" {
		return TypeDef{}, fmt.Errorf("kind must be 'object' or 'scalar', got %q", raw.Kind)
	}
	if raw.Kind == "object" {
		// Collect and sort field names for deterministic output.
		names := make([]string, 0, len(raw.Fields))
		for fn := range raw.Fields {
			names = append(names, fn)
		}
		sort.Strings(names)
		for _, fn := range names {
			f := raw.Fields[fn]
			if f.Type == "" {
				return TypeDef{}, fmt.Errorf("field %q missing type", fn)
			}
			field := Field{
				Name:        fn,
				Type:        f.Type,
				Required:    f.Required,
				Description: f.Description,
			}
			if f.Nested != nil {
				if f.Nested.Outer == "" || f.Nested.Inner == "" {
					return TypeDef{}, fmt.Errorf("field %q: nested requires both outer and inner", fn)
				}
				if !strings.HasPrefix(f.Type, "FlexSlice[") {
					return TypeDef{}, fmt.Errorf("field %q: nested is only valid for FlexSlice[T] fields, got %q", fn, f.Type)
				}
				field.NestedOuter = f.Nested.Outer
				field.NestedInner = f.Nested.Inner
			}
			td.Fields = append(td.Fields, field)
		}
	}
	return td, nil
}

// convertMethod converts a raw YAML method to an IR Method.
func convertMethod(raw MethodYAML) (Method, error) {
	if raw.Namespace == "" {
		return Method{}, fmt.Errorf("missing namespace")
	}
	if raw.Name == "" {
		return Method{}, fmt.Errorf("missing name")
	}
	if raw.Version == 0 {
		return Method{}, fmt.Errorf("missing version")
	}
	if raw.HTTPVerb == "" {
		return Method{}, fmt.Errorf("missing http_verb")
	}
	if raw.RestPath == "" {
		return Method{}, fmt.Errorf("missing rest_path")
	}
	if raw.AuthTier == "" {
		return Method{}, fmt.Errorf("missing auth_tier")
	}
	if raw.ResponseType == "" {
		return Method{}, fmt.Errorf("missing response_type")
	}

	params := make([]Param, len(raw.Params))
	for i, p := range raw.Params {
		if p.Name == "" {
			return Method{}, fmt.Errorf("param[%d] missing name", i)
		}
		if p.Type == "" {
			return Method{}, fmt.Errorf("param %q missing type", p.Name)
		}
		params[i] = Param(p)
	}

	if raw.Composite != "" && !knownComposites[raw.Composite] {
		return Method{}, fmt.Errorf("unknown composite %q", raw.Composite)
	}

	return Method{
		Namespace:          raw.Namespace,
		Name:               raw.Name,
		Version:            raw.Version,
		AllVersions:        raw.AllVersions,
		DeprecatedVersions: raw.DeprecatedVersions,
		HTTPVerb:           raw.HTTPVerb,
		RestPath:           raw.RestPath,
		AuthTier:           raw.AuthTier,
		Scope:              raw.Scope,
		Params:             params,
		ResponseRoot:       raw.ResponseRoot,
		ResponseType:       raw.ResponseType,
		MethodParam:        raw.MethodParam,
		Composite:          raw.Composite,
		Pagination:         raw.Pagination,
		SmokeFixture:       raw.SmokeFixture,
		GoNameOverride:     raw.GoNameOverride,
	}, nil
}

// knownComposites enumerates the multi-call flows the service template can render.
var knownComposites = map[string]bool{
	"barcode_lookup": true,
}

// validateTypeRef checks that the type keyword is a known primitive or a defined TypeRef.
// Accepts: string, APIInt, APIFloat, APIBool, APITernary, APIDaysEpoch, FlexSlice[X].
func validateTypeRef(typeName string, types map[string]TypeDef) error {
	switch typeName {
	case "string", "APIInt", "APIFloat", "APIBool", "APITernary", "APIDaysEpoch":
		return nil
	}
	if strings.HasPrefix(typeName, "FlexSlice[") && strings.HasSuffix(typeName, "]") {
		inner := typeName[len("FlexSlice[") : len(typeName)-1]
		// Inner type may be a primitive or a TypeRef.
		return validateTypeRef(inner, types)
	}
	// Must be a defined TypeRef.
	if _, ok := types[typeName]; !ok {
		return fmt.Errorf("undefined TypeRef %q", typeName)
	}
	return nil
}

// collectTypes returns the de-duplicated, sorted list of TypeDefs needed by the
// given methods. Recursively includes types referenced by fields.
func collectTypes(methods []Method, allTypes map[string]TypeDef) ([]TypeDef, error) {
	needed := make(map[string]bool)
	for _, m := range methods {
		collectTypeRefs(m.ResponseType, allTypes, needed)
		for _, p := range m.Params {
			collectTypeRefs(p.Type, allTypes, needed)
		}
	}

	var result []TypeDef
	names := make([]string, 0, len(needed))
	for n := range needed {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		td, ok := allTypes[n]
		if !ok {
			return nil, fmt.Errorf("type %q referenced but not defined", n)
		}
		result = append(result, td)
	}
	return result, nil
}

// collectTypeRefs recursively walks a type string and adds all named TypeRefs to the set.
func collectTypeRefs(typeName string, allTypes map[string]TypeDef, needed map[string]bool) {
	switch typeName {
	case "string", "APIInt", "APIFloat", "APIBool", "APITernary", "APIDaysEpoch", "":
		return
	}
	if strings.HasPrefix(typeName, "FlexSlice[") && strings.HasSuffix(typeName, "]") {
		inner := typeName[len("FlexSlice[") : len(typeName)-1]
		collectTypeRefs(inner, allTypes, needed)
		return
	}
	if _, ok := allTypes[typeName]; ok {
		if needed[typeName] {
			return // already visited
		}
		needed[typeName] = true
		// Recurse into fields.
		td := allTypes[typeName]
		for _, f := range td.Fields {
			collectTypeRefs(f.Type, allTypes, needed)
		}
	}
}

// goMethodName applies the naming algorithm to a spec method name:
// convert snake_case to PascalCase.
func goMethodName(name string) string {
	// Split on underscore and dots.
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '.'
	})
	var sb strings.Builder
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		// Uppercase first rune, preserve rest.
		runes := []rune(part)
		sb.WriteRune(unicode.ToUpper(runes[0]))
		for _, r := range runes[1:] {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// isVersionDeprecated returns true if version v appears in deprecatedVersions.
func isVersionDeprecated(v int, deprecatedVersions []int) bool {
	for _, dv := range deprecatedVersions {
		if dv == v {
			return true
		}
	}
	return false
}

// hasNonPathParams returns true if any param has location != "path".
func hasNonPathParams(params []Param) bool {
	for _, p := range params {
		if p.Location != "path" {
			return true
		}
	}
	return false
}

// overrideKey produces a unique map key for a (namespace, name, version) triple.
func overrideKey(namespace, name string, version int) string {
	return fmt.Sprintf("%s|%s|%d", namespace, name, version)
}

// sortedKeys returns the sorted keys of a map[string][]Method.
func sortedKeys(m map[string][]Method) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
