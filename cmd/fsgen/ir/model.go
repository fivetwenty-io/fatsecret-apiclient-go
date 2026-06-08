// Package ir defines the internal representation produced by parsing spec/fatsecret.yaml.
// All builder functions populate these structs; renderer functions consume them.
package ir

// Spec is the root of the parsed spec/fatsecret.yaml.
type Spec struct {
	// Version is the spec schema version string (currently "1").
	Version string
	// Types maps type name → TypeDef for all reusable types declared under types:.
	Types map[string]TypeDef
	// Methods is the ordered list of all API methods declared in the spec.
	Methods []Method
}

// TypeDef describes a reusable type declared under types: in the spec.
type TypeDef struct {
	// Name is the type name as declared in the spec (e.g. "Food", "Serving").
	Name string
	// Kind is "object" (has Fields) or "scalar" (alias for documentation only).
	Kind string
	// Fields is the ordered list of fields; empty for scalar kinds.
	Fields []Field
}

// Field is one member of an object TypeDef.
type Field struct {
	// Name is the wire JSON field name (snake_case).
	Name string
	// Type is the spec type keyword: string, APIInt, APIFloat, APIBool, APITernary,
	// APIDaysEpoch, FlexSlice[X], or a TypeRef name.
	Type string
	// Required true means the field is always present in the JSON response.
	Required    bool
	Description string
}

// Method is one API endpoint entry from the methods: list.
type Method struct {
	// Namespace is the spec namespace (e.g. "foods", "food_entries").
	Namespace string
	// Name is the bare method name within the namespace (e.g. "search", "get").
	Name string
	// Version is the canonical version this entry represents.
	Version int
	// AllVersions lists every known version (including deprecated).
	AllVersions []int
	// DeprecatedVersions lists version numbers that are deprecated.
	DeprecatedVersions []int
	// HTTPVerb is the HTTP method: GET, POST, PUT, DELETE.
	HTTPVerb string
	// RestPath is the literal API path, e.g. "/rest/foods/search/v3".
	RestPath string
	// AuthTier is "client_credentials", "oauth1_signed", or "oauth1_delegated".
	AuthTier string
	// Scope is "" (basic), "premier", "barcode", "nlp", "image-recognition", or "feedback".
	Scope string
	// Params is the ordered list of request parameters.
	Params []Param
	// ResponseRoot is the top-level JSON key whose value is the response payload.
	ResponseRoot string
	// ResponseType is a TypeRef name from the types: section.
	ResponseType string
	// Pagination true means the response includes paging metadata.
	Pagination bool
	// SmokeFixture is the minimal valid JSON for zero-required-param smoke tests.
	// Empty string means no smoke fixture (method has required params).
	SmokeFixture string
	// GoNameOverride when non-empty overrides the naming algorithm result.
	GoNameOverride string

	// GoMethodName is set by BuildNamespaces after naming algorithm + override.
	GoMethodName string
	// GoPackage is the Go package name this method belongs to (set by BuildNamespaces).
	GoPackage string
	// IsDeprecated is true when this method's Version is in DeprecatedVersions.
	IsDeprecated bool
	// HasRequestStruct is true when the method has ≥1 non-path parameter.
	HasRequestStruct bool
}

// HasRequiredParams returns true if any param is required.
func (m *Method) HasRequiredParams() bool {
	for _, p := range m.Params {
		if p.Required {
			return true
		}
	}
	return false
}

// Param describes a single request parameter.
type Param struct {
	// Name is the wire parameter name (snake_case).
	Name string
	// Type is the spec type keyword.
	Type string
	// Required true means the caller must supply this parameter.
	Required bool
	// Location is "query", "body", or "path".
	Location string
	// TierGate when non-empty indicates the premier (or other) tier needed for this param.
	TierGate string
	// Constraint is a free-text constraint hint (e.g. "max=50", "enum=generic,brand").
	Constraint string
}

// Namespace is the generator's per-package representation built from grouped methods.
type Namespace struct {
	// PackageName is the Go identifier used as the package clause (e.g. "foods").
	PackageName string
	// ImportPath is the fully qualified import path (e.g. "github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/foods").
	ImportPath string
	// Methods is the ordered list of methods belonging to this package (canonical only; deprecated excluded).
	Methods []Method
	// Types is the de-duplicated, topologically sorted list of TypeDefs used by methods in this namespace.
	Types []TypeDef
	// NeedsTypes is true when any method in this namespace references a non-primitive type.
	NeedsTypes bool
}

// SpecYAML mirrors the YAML structure for unmarshaling via gopkg.in/yaml.v3.
// Fields use yaml tags; builder converts to IR structs after unmarshal.
type SpecYAML struct {
	Version string              `yaml:"version"`
	Types   map[string]TypeYAML `yaml:"types"`
	Methods []MethodYAML        `yaml:"methods"`
}

// TypeYAML is the YAML shape for a type definition.
type TypeYAML struct {
	Kind   string               `yaml:"kind"`
	Fields map[string]FieldYAML `yaml:"fields"`
}

// FieldYAML is the YAML shape for one field within a type.
type FieldYAML struct {
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

// MethodYAML is the YAML shape for one method entry.
type MethodYAML struct {
	Namespace          string      `yaml:"namespace"`
	Name               string      `yaml:"name"`
	Version            int         `yaml:"version"`
	AllVersions        []int       `yaml:"all_versions"`
	DeprecatedVersions []int       `yaml:"deprecated_versions"`
	HTTPVerb           string      `yaml:"http_verb"`
	RestPath           string      `yaml:"rest_path"`
	AuthTier           string      `yaml:"auth_tier"`
	Scope              string      `yaml:"scope"`
	Pagination         bool        `yaml:"pagination"`
	Params             []ParamYAML `yaml:"params"`
	ResponseRoot       string      `yaml:"response_root"`
	ResponseType       string      `yaml:"response_type"`
	SmokeFixture       string      `yaml:"smoke_fixture"`
	GoNameOverride     string      `yaml:"go_name_override"`
}

// ParamYAML is the YAML shape for one parameter.
type ParamYAML struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Required   bool   `yaml:"required"`
	Location   string `yaml:"location"`
	TierGate   string `yaml:"tier_gate"`
	Constraint string `yaml:"constraint"`
}

// OverridesYAML is the YAML shape of spec/name_overrides.yaml.
type OverridesYAML struct {
	Version   string         `yaml:"version"`
	Overrides []OverrideYAML `yaml:"overrides"`
}

// OverrideYAML is one entry in the overrides list.
type OverrideYAML struct {
	Namespace string `yaml:"namespace"`
	Name      string `yaml:"name"`
	Version   int    `yaml:"version"`
	GoName    string `yaml:"go_name"`
}
