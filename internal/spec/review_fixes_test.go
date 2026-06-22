// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestSchemaFromTypeRef_QualifiedPrimitiveInContainer covers the headline B1 fix:
// a go/types-built time.Time is RefBasic{Pkg:"time",Name:"Time"}; before the fix,
// basicRefSchema("Time") was nil and a []time.Time / map[string]time.Time field
// was silently dropped.
func TestSchemaFromTypeRef_QualifiedPrimitiveInContainer(t *testing.T) {
	timeRef := &metadata.TypeRef{Kind: metadata.RefBasic, Pkg: "time", Name: "Time"}
	dateTime := &Schema{Type: "string", Format: "date-time"}

	assert.Equal(t, dateTime, schemaFromTypeRef(timeRef), "scalar qualified time.Time")

	slice := &metadata.TypeRef{Kind: metadata.RefSlice, Elem: timeRef}
	assert.Equal(t, &Schema{Type: "array", Items: dateTime}, schemaFromTypeRef(slice), "[]time.Time")

	m := &metadata.TypeRef{Kind: metadata.RefMap, Key: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Elem: timeRef}
	assert.Equal(t, &Schema{Type: "object", AdditionalProperties: dateTime}, schemaFromTypeRef(m), "map[string]time.Time")
}

// TestResolveUnderlyingType_FixedArrayAliasKeepsLength covers D1: a [N]Alias field
// must keep its length (the slice collapse dropped minItems/maxItems).
func TestResolveUnderlyingType_FixedArrayAliasKeepsLength(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"ex": {Files: map[string]*metadata.File{"e.go": {Types: map[string]*metadata.Type{
			"Celsius": {Name: sp.Get("Celsius"), Pkg: sp.Get("ex"), Kind: sp.Get("alias"), Target: sp.Get("float64")},
		}}}},
	}
	assert.Equal(t, "[3]float64", resolveUnderlyingType("[3]ex.Celsius", meta), "fixed-array length preserved")
	assert.Equal(t, "[]float64", resolveUnderlyingType("[]ex.Celsius", meta), "slice stays a slice")
	assert.Equal(t, "float64", resolveUnderlyingType("ex.Celsius", meta), "scalar alias unwrapped")
	assert.Equal(t, "*float64", resolveUnderlyingType("*ex.Celsius", meta), "pointer kept")
	// Nested wrappers must ALL survive (the re-apply-outermost-only bug dropped the inner one).
	assert.Equal(t, "*[]float64", resolveUnderlyingType("*[]ex.Celsius", meta), "pointer-to-slice keeps both layers")
	assert.Equal(t, "map[string]float64", resolveUnderlyingType("map[string]ex.Celsius", meta), "map value resolved, map kept")
	assert.Equal(t, "[2]*float64", resolveUnderlyingType("[2]*ex.Celsius", meta), "array-of-pointer keeps both")
}

// TestWrapperFieldIsGeneric_ContainerNotPlaceholder covers SW1: only interface{}/
// any (optionally behind a pointer) is a generic placeholder — []interface{} and
// map[string]any are concrete typed fields, not placeholders.
func TestWrapperFieldIsGeneric_ContainerNotPlaceholder(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	iface := &metadata.TypeRef{Kind: metadata.RefInterface}
	wrapper := &metadata.Type{Fields: []metadata.Field{
		{Name: sp.Get("Data"), TypeRef: iface},
		{Name: sp.Get("Ptr"), TypeRef: &metadata.TypeRef{Kind: metadata.RefPointer, Elem: iface}},
		{Name: sp.Get("Errors"), TypeRef: &metadata.TypeRef{Kind: metadata.RefSlice, Elem: iface}},
		{Name: sp.Get("Meta"), TypeRef: &metadata.TypeRef{Kind: metadata.RefMap, Key: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Elem: iface}},
	}}

	assert.True(t, wrapperFieldIsGeneric(meta, wrapper, "Data"), "interface{} is a placeholder")
	assert.True(t, wrapperFieldIsGeneric(meta, wrapper, "Ptr"), "*interface{} is a placeholder")
	assert.False(t, wrapperFieldIsGeneric(meta, wrapper, "Errors"), "[]interface{} is NOT a placeholder")
	assert.False(t, wrapperFieldIsGeneric(meta, wrapper, "Meta"), "map[string]any is NOT a placeholder")
}

// TestAddRefSchemaForType_StripsMajorVersion covers C1: a $ref for a versioned
// module type must strip the /vN segment so it matches the component key
// (schemaName strips it).
func TestAddRefSchemaForType_StripsMajorVersion(t *testing.T) {
	got := addRefSchemaForType("github.com/x/lib/v2.Thing")
	assert.Equal(t, "#/components/schemas/github.com_x_lib.Thing", got.Ref, "/v2 stripped to match component key")
	assert.NotContains(t, got.Ref, "v2")
	// Unversioned types are untouched.
	assert.Equal(t, "#/components/schemas/github.com_x_lib.Thing",
		addRefSchemaForType("github.com/x/lib.Thing").Ref, "unversioned unchanged")
}

// TestSchemaForNamedRef_ExternalVersionedTypeMatches covers C2: a configured
// external type (named in the stripped convention) must match a versioned ref, so
// its component is registered instead of a dangling $ref being emitted.
func TestSchemaForNamedRef_ExternalVersionedTypeMatches(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{ExternalTypes: []ExternalType{
		{Name: "github.com/x/lib.Map", OpenAPIType: &Schema{Type: "object"}},
	}}
	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/x/lib/v2", Name: "Map"}
	usedTypes := map[string]*Schema{}

	schema, schemas, ok := schemaForNamedRef(usedTypes, ref, "", meta, cfg, nil)
	assert.True(t, ok, "versioned external type must resolve to its configured component")
	assert.NotEmpty(t, schemas, "external component must be registered (not skipped → dangling)")
	if assert.NotNil(t, schema) {
		assert.NotContains(t, schema.Ref, "v2", "$ref strips the version to match the component key")
	}
}

// TestSchemaForNamedRef_NilVisitedTypesNoPanic covers the cycle-map self-guard:
// a direct caller passing nil visitedTypes must not panic on the struct path
// (which writes the cycle key).
func TestSchemaForNamedRef_NilVisitedTypesNoPanic(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{"a.go": {Types: map[string]*metadata.Type{
			"User": {
				Name: sp.Get("User"), Pkg: sp.Get("app"), Kind: sp.Get("struct"),
				Fields: []metadata.Field{{Name: sp.Get("Name"), Type: sp.Get("string")}},
			},
		}}}},
	}
	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "app", Name: "User"}
	assert.NotPanics(t, func() {
		_, _, _ = schemaForNamedRef(map[string]*Schema{}, ref, "app.User", meta, &APISpecConfig{}, nil)
	}, "nil visitedTypes must not panic on the struct path")
}

// TestLeafPkgInMetadata covers the gate helper: unqualified, analyzed-package,
// and SHORT bare-identifier qualifiers pass; only a path-like import qualifier
// absent from metadata (a genuinely external type) is rejected.
func TestLeafPkgInMetadata(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{"github.com/me/app/models": {}}
	assert.True(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: ""}, meta), "unqualified allowed")
	assert.True(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "github.com/me/app/models"}, meta), "analyzed package (full path)")
	assert.True(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "models"}, meta), "short qualifier = a package's last segment")
	assert.False(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "uuid"}, meta), "dotless EXTERNAL package (no matching last segment) rejected")
	assert.False(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "github.com/acme/extpkg"}, meta), "path-like external package rejected")
	assert.False(t, leafPkgInMetadata(nil, meta))
	assert.False(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "x"}, nil))
}

// TestSchemaForType_FixedArrayBridgeKeepsLength confirms the round-1 Copilot
// concern is closed by the D1 fix: after resolveUnderlyingType yields "[3]string"
// for a [3]alias field, schemaForType's goType!=ref.String() bridge must still
// carry minItems/maxItems (schemaFromParsedString re-parses the length-bearing
// string), not return an unconstrained slice.
func TestSchemaForType_FixedArrayBridgeKeepsLength(t *testing.T) {
	meta := &metadata.Metadata{Packages: map[string]*metadata.Package{}}
	ref := &metadata.TypeRef{Kind: metadata.RefArray, Len: 3,
		Elem: &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "ex", Name: "Status"}}
	s, _ := schemaForType(map[string]*Schema{}, "[3]string", ref, meta, &APISpecConfig{}, nil)
	if assert.NotNil(t, s) {
		assert.Equal(t, "array", s.Type)
		assert.Equal(t, 3, s.MinItems, "fixed-array length must survive the bridge")
		assert.Equal(t, 3, s.MaxItems)
	}
}

// TestStripMajorVersion locks that only a TRAILING /vN. (before the type name) is
// stripped — a /vN/ subpackage segment is left intact, so distinct API-version
// subpackages do NOT collapse onto one component name (the round-6 over-strip
// regression).
func TestStripMajorVersion(t *testing.T) {
	cases := map[string]string{
		"github.com/gofiber/fiber/v2.Map": "github.com/gofiber/fiber.Map", // versioned module type → stripped
		"github.com/x/lib/v2/sub.Type":    "github.com/x/lib/v2/sub.Type", // subpackage → NOT stripped (kept)
		"github.com/x/lib.Type":           "github.com/x/lib.Type",        // unversioned
		"User":                            "User",                         // unqualified
		"github.com/x/v1alpha1/p.T":       "github.com/x/v1alpha1/p.T",    // not a version segment
		"github.com/x/v2pkg.T":            "github.com/x/v2pkg.T",         // digits then a letter
	}
	for in, want := range cases {
		assert.Equal(t, want, stripMajorVersion(in), in)
	}

	// The point of keeping subpackage versions: two API versions stay DISTINCT.
	v1 := stripMajorVersion("myapp/api/v1/users.Request")
	v2 := stripMajorVersion("myapp/api/v2/users.Request")
	assert.NotEqual(t, v1, v2, "distinct API-version subpackages must not collapse to one component name")
}

// TestTypeByRefGated locks the central collision chokepoint every resolver now
// uses: a path-like external qualifier returns nil; an internal (in-metadata or
// short) qualifier resolves.
func TestTypeByRefGated(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"github.com/me/app/models": {Files: map[string]*metadata.File{"m.go": {Types: map[string]*metadata.Type{
			"User": {Name: sp.Get("User"), Pkg: sp.Get("github.com/me/app/models")},
		}}}},
	}
	// External path-like qualifier colliding on bare name -> nil (not borrowed).
	assert.Nil(t, typeByRefGated(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/acme/extpkg", Name: "User"}, meta))
	// Internal full-path qualifier -> resolves.
	assert.NotNil(t, typeByRefGated(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/me/app/models", Name: "User"}, meta))
	// Internal short qualifier -> resolves via the name fallback.
	assert.NotNil(t, typeByRefGated(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "models", Name: "User"}, meta))
	// DOTLESS external package colliding on the bare name -> nil (NOT borrowed):
	// "uuid" is no analyzed package's last segment, so it is treated as external —
	// the leaky "any dotless Pkg is internal" rule let this through before.
	assert.Nil(t, typeByRefGated(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "uuid", Name: "User"}, meta))
}

// TestShortPkgInternalTypeStillResolves locks the round-4 refinement: an INTERNAL
// type referenced with a SHORT qualifier (what getTypeName emits when go/types
// info is partial) must still resolve via typeByRef's name fallback, not be
// dropped as external by the collision gate.
func TestShortPkgInternalTypeStillResolves(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"github.com/me/app/models": {Files: map[string]*metadata.File{"m.go": {Types: map[string]*metadata.Type{
			"User": {
				Name: sp.Get("User"), Pkg: sp.Get("github.com/me/app/models"), Kind: sp.Get("struct"),
				Fields: []metadata.Field{{Name: sp.Get("Name"), Type: sp.Get("string")}},
			},
		}}}},
	}
	// "models" is the short spelling — NOT a meta.Packages key (those are full
	// import paths) but a bare identifier, so it must still resolve.
	shortRef := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "models", Name: "User"}
	assert.Equal(t, "models.User", bodyTypeFromMetadataRef(shortRef, meta, &APISpecConfig{}),
		"internal type with a short qualifier still resolves (not dropped as external)")
}

// TestExternalCollisionNotBorrowed covers the collision fix at both sites: an
// external type whose bare name matches an internal one must NOT resolve to the
// internal type (via typeByRef's name-only fallback).
func TestExternalCollisionNotBorrowed(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"github.com/me/app": {Files: map[string]*metadata.File{"a.go": {Types: map[string]*metadata.Type{
			"User": {
				Name: sp.Get("User"), Pkg: sp.Get("github.com/me/app"), Kind: sp.Get("struct"),
				Fields: []metadata.Field{{Name: sp.Get("Secret"), Type: sp.Get("string")}},
			},
		}}}},
	}
	ext := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/acme/extpkg", Name: "User"}
	internal := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/me/app", Name: "User"}

	// bodyTypeFromMetadataRef: external must not borrow the internal type.
	assert.Equal(t, "", bodyTypeFromMetadataRef(ext, meta, &APISpecConfig{}), "external not borrowed")
	assert.Equal(t, "github.com/me/app.User", bodyTypeFromMetadataRef(internal, meta, &APISpecConfig{}), "internal still resolves")

	// schemaForNamedRef: external yields a dangling $ref, NOT the internal struct.
	schema, schemas, ok := schemaForNamedRef(map[string]*Schema{}, ext, "github.com/acme/extpkg.User", meta, &APISpecConfig{}, nil)
	assert.True(t, ok)
	assert.NotEmpty(t, schema.Ref, "external collision → dangling $ref")
	for _, s := range schemas {
		assert.NotContains(t, s.Properties, "secret", "must not emit the internal User's fields under the external name")
	}
}

// TestVersionedExternalType_NoDanglingRef locks the fix for the cross-function
// dangling-$ref bug (full max-effort review, PR #61). A versioned external type
// configured by its stripped Name (the documented convention) matches in
// schemaForNamedRef (which strips before comparing) and gets a $ref under the
// stripped component name (schemaName also strips). generateSchemas, however,
// used to match ExternalTypes by the RAW (versioned) key, so it never emitted
// the component — leaving the $ref dangling. The match there now strips too,
// consistent with the schemaName-based emission on the same line.
func TestVersionedExternalType_NoDanglingRef(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	cfg := DefaultAPISpecConfig()
	cfg.ExternalTypes = []ExternalType{
		{Name: "github.com/gofiber/fiber.Map", OpenAPIType: &Schema{Type: "object"}},
	}

	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/gofiber/fiber/v2", Name: "Map"}
	key := "github.com/gofiber/fiber/v2.Map"

	usedTypes := map[string]*Schema{}
	schema, schemas, ok := schemaForNamedRef(usedTypes, ref, key, meta, cfg, nil)
	if !assert.True(t, ok, "versioned external type should match config by stripped Name") {
		return
	}
	wantRef := refComponentsSchemasPrefix + schemaName(key, cfg)
	assert.Equal(t, wantRef, schema.Ref, "field $ref targets the stripped component name")

	for k, v := range schemas {
		markUsedType(usedTypes, k, v)
	}
	components := Components{Schemas: map[string]*Schema{}}
	generateSchemas(usedTypes, cfg, components, meta)

	wantName := schemaName(key, cfg)
	assert.Contains(t, components.Schemas, wantName,
		"external component must be emitted under the same name the $ref targets (else dangling)")
	assert.Equal(t, "object", components.Schemas[wantName].Type, "emitted component is the configured external schema")
}

// TestOpaqueFuncChanFieldsOmitted locks the fix for the opaque-leaf dangling-$ref
// bug (full max-effort review, PR #61). A struct field whose type is an opaque
// func or chan leaf used to emit a $ref to a "func"/"chan" component that is never
// generated (invalid OpenAPI). Such fields are non-serializable and must be
// omitted entirely — not present in Properties, not listed in Required, and never
// a dangling $ref. struct{}/interface{} are NOT opaque (they map to an object
// schema) and stay present.
func TestOpaqueFuncChanFieldsOmitted(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	cfg := DefaultAPISpecConfig()

	typ := &metadata.Type{
		Name: sp.Get("Job"),
		Kind: sp.Get("struct"),
		Fields: []metadata.Field{
			{Name: sp.Get("Name"), Type: sp.Get("string"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Tag: sp.Get(`json:"name"`)},
			{Name: sp.Get("OnDone"), Type: sp.Get("func"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefFunc}, Tag: sp.Get(`json:"onDone"`)},
			{Name: sp.Get("Events"), Type: sp.Get("chan"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefChan}, Tag: sp.Get(`json:"events"`)},
		},
	}

	schema, _ := generateStructSchema(map[string]*Schema{}, "Job", typ, meta, cfg, nil)

	assert.Contains(t, schema.Properties, "name", "serializable field stays")
	assert.NotContains(t, schema.Properties, "onDone", "opaque func field omitted (not a dangling $ref)")
	assert.NotContains(t, schema.Properties, "events", "opaque chan field omitted")
	assert.NotContains(t, schema.Required, "onDone", "omitted field must not appear in required")
	assert.NotContains(t, schema.Required, "events", "omitted field must not appear in required")
	for name, prop := range schema.Properties {
		assert.NotEmptyf(t, prop, "property %q must not be a nil schema", name)
	}
}

// TestCanAddRefSchemaForType_OpaqueFuncChanSpellings locks the structural (not
// exact-string) recognition of opaque func/chan leaves (local coder-review LOW
// #1, PR #61). The struct-field path always sees the bare "func"/"chan" from
// TypeRef.String(), but the body/param path passes raw getTypeName strings that
// carry signatures/directions ("func(int) error", "chan<- int"). All must be
// rejected as non-nameable so none emits a dangling $ref; legitimately-named
// types that merely look similar must stay componentizable.
func TestCanAddRefSchemaForType_OpaqueFuncChanSpellings(t *testing.T) {
	for _, k := range []string{"func", "func(int) error", "func(a, b int) bool",
		"chan", "chan int", "chan<- int", "<-chan int",
		// Pointer-wrapped opaque leaves reach here un-stripped on the body/param
		// path; NamedLeaf must unwrap to the func/chan leaf and reject them.
		"*func", "**func", "*chan", "*[]func", "*chan<- int"} {
		assert.Falsef(t, canAddRefSchemaForType(k), "opaque %q must not be componentizable", k)
	}
	// Look-alikes that ARE real, nameable types must remain componentizable —
	// including pointers/slices of a NAMED leaf (NamedLeaf bottoms out at RefNamed).
	for _, k := range []string{"Function", "Channel", "funcResult", "chanutil.Pool",
		"models.Func", "*models.User", "Pair[func(),int]"} {
		assert.Truef(t, canAddRefSchemaForType(k), "named type %q must stay componentizable", k)
	}
}

// TestOpaqueFuncChanContainerFieldsOmitted extends the opaque-leaf fix to
// CONTAINER fields whose element is opaque ([]func, map[string]chan, *func): the
// element resolves to nil and the wrapper must not emit a dangling $ref or a
// component — the field is omitted, like the bare case (coder-review follow-up).
func TestOpaqueFuncChanContainerFieldsOmitted(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	cfg := DefaultAPISpecConfig()

	typ := &metadata.Type{
		Name: sp.Get("Reg"),
		Kind: sp.Get("struct"),
		Fields: []metadata.Field{
			{Name: sp.Get("Keep"), Type: sp.Get("string"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Tag: sp.Get(`json:"keep"`)},
			{Name: sp.Get("Hooks"), Type: sp.Get("[]func"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefSlice, Elem: &metadata.TypeRef{Kind: metadata.RefFunc}}, Tag: sp.Get(`json:"hooks"`)},
			{Name: sp.Get("Subs"), Type: sp.Get("map[string]chan"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefMap, Key: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Elem: &metadata.TypeRef{Kind: metadata.RefChan}}, Tag: sp.Get(`json:"subs"`)},
			{Name: sp.Get("Cb"), Type: sp.Get("*func"),
				TypeRef: &metadata.TypeRef{Kind: metadata.RefPointer, Elem: &metadata.TypeRef{Kind: metadata.RefFunc}}, Tag: sp.Get(`json:"cb"`)},
		},
	}

	schema, extra := generateStructSchema(map[string]*Schema{}, "Reg", typ, meta, cfg, nil)

	assert.Contains(t, schema.Properties, "keep", "serializable field stays")
	assert.Len(t, schema.Properties, 1, "only the serializable field survives")
	for _, f := range []string{"hooks", "subs", "cb"} {
		assert.NotContainsf(t, schema.Properties, f, "container-of-opaque field %q omitted", f)
		assert.NotContainsf(t, schema.Required, f, "omitted field %q not in required", f)
	}
	// No spurious component is registered for the opaque-container fields (the
	// "keep" string is primitive, so nothing should be registered at all). An empty
	// map asserts directly; the per-name guards below would be vacuous on their own.
	assert.Empty(t, extra, "no component registered for opaque-container fields")
	for name := range extra {
		assert.NotContainsf(t, name, "func", "no func component registered (got %q)", name)
		assert.NotContainsf(t, name, "chan", "no chan component registered (got %q)", name)
	}
}
