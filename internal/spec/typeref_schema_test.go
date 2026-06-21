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
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestSchemaFromTypeRef_MatchesStringPath shadow-tests the tree-based schema
// generator against the existing string-based mapGoTypeToOpenAPISchema: for the
// leaf/structural forms schemaFromTypeRef handles, the two MUST produce the
// identical schema. This proves the tree generator is a faithful mirror before
// it is wired into the live path. RefArray emits the same minItems/maxItems (or
// maxLength for byte arrays) as the string path's [N]T handling.
func TestSchemaFromTypeRef_MatchesStringPath(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := &metadata.Metadata{Packages: map[string]*metadata.Package{}}
	b := func(name string) *metadata.TypeRef { return &metadata.TypeRef{Kind: metadata.RefBasic, Name: name} }
	ptr := func(e *metadata.TypeRef) *metadata.TypeRef {
		return &metadata.TypeRef{Kind: metadata.RefPointer, Elem: e}
	}
	slice := func(e *metadata.TypeRef) *metadata.TypeRef {
		return &metadata.TypeRef{Kind: metadata.RefSlice, Elem: e}
	}

	cases := []*metadata.TypeRef{
		b("string"), b("int"), b("int8"), b("int16"), b("int32"), b("int64"),
		b("uint"), b("uint8"), b("uint16"), b("uint32"), b("uint64"), b("byte"),
		b("float32"), b("float64"), b("bool"), b("time.Time"), b("any"), b("struct{}"), b("interface{}"),
		{Kind: metadata.RefInterface},
		ptr(b("int")),
		slice(b("string")), slice(b("int")), slice(b("time.Time")),
		slice(b("byte")),     // []byte -> string/byte
		slice(ptr(b("int"))), // []*int
		{Kind: metadata.RefMap, Key: b("string"), Elem: b("int")},
		{Kind: metadata.RefArray, Len: 3, Elem: b("int")},   // [3]int -> minItems == maxItems
		{Kind: metadata.RefArray, Len: 16, Elem: b("byte")}, // [16]byte -> string/byte/maxLength
	}
	for _, ref := range cases {
		got := schemaFromTypeRef(ref)
		require.NotNil(t, got, ref.String())
		want, _ := schemaForType(map[string]*Schema{}, ref.String(), nil, meta, cfg, nil)
		assert.Equal(t, want, got, "schema mismatch for %q", ref.String())
	}

	// A slice/array of an unrepresentable element defers to the string path.
	assert.Nil(t, schemaFromTypeRef(slice(&metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"})))
	// An unrecognized basic name also defers (e.g. error has no default mapping).
	assert.Nil(t, schemaFromTypeRef(b("error")))
	// A string-keyed map with an unrepresentable value defers too.
	assert.Nil(t, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefMap, Key: b("string"), Elem: &metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"}}))
	// A fixed array of an unrepresentable element defers to the string path.
	assert.Nil(t, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefArray, Len: 3, Elem: &metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"}}))
	// Inferred-length arrays ([...]T, Len -1) carry no length constraint.
	assert.Equal(t, &Schema{Type: "array", Items: &Schema{Type: "integer"}},
		schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefArray, Len: -1, Elem: b("int")}))
	assert.Equal(t, &Schema{Type: "string", Format: "byte"},
		schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefArray, Len: -1, Elem: b("byte")}))

	// Forms the tree generator defers to the string path (named/generic/struct).
	assert.Nil(t, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "pkg", Name: "User"}))
	assert.Nil(t, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefStruct}))
	assert.Nil(t, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefMap, Key: b("int"), Elem: b("int")})) // non-string key
	assert.Nil(t, schemaFromTypeRef(nil))
}

// TestSchemaForType_EmptyStringBridge covers schemaForType's fallback for a type
// whose flattened string is empty (the getTypeName gap for multi-parameter
// generics): it bridges to the TypeRef's canonical string so the named type
// still resolves instead of producing nothing.
func TestSchemaForType_EmptyStringBridge(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := &metadata.Metadata{Packages: map[string]*metadata.Package{}}

	// With a TypeRef, the empty string is bridged to ref.String()="User" and
	// resolves to a $ref.
	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"}
	got, _ := schemaForType(map[string]*Schema{}, "", ref, meta, cfg, nil)
	require.NotNil(t, got)

	// Without a TypeRef, the empty string yields nothing (the broken status quo).
	none, _ := schemaForType(map[string]*Schema{}, "", nil, meta, cfg, nil)
	assert.Nil(t, none)

	// A struct ref resolves through the tree (schemaForNamedRef) to a $ref —
	// covering schemaForType's named-ref success branch.
	full := metaWithNamedTypes()
	got, schemas := schemaForType(map[string]*Schema{}, "", &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"}, full, cfg, nil)
	require.NotNil(t, got)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", got.Ref)
	assert.Contains(t, schemas, "main.User")
}

// TestSchemaForType_StringOnly covers the string-only caller path (nil TypeRef):
// the string is parsed and resolved on the tree, with a direct named type
// registering under the ORIGINAL key (preserving the "-->" separator).
func TestSchemaForType_StringOnly(t *testing.T) {
	cfg := &APISpecConfig{}

	// Primitive string → structural leaf, no metadata needed.
	got, _ := schemaForType(map[string]*Schema{}, "int", nil, metaWithNamedTypes(), cfg, nil)
	assert.Equal(t, &Schema{Type: "integer"}, got)

	// Direct named struct (dotted) → $ref, registered under the dotted key.
	used := map[string]*Schema{}
	got, _ = schemaForType(used, "main.User", nil, metaWithNamedTypes(), cfg, nil)
	require.NotNil(t, got)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", got.Ref)
	assert.Contains(t, used, "main.User")

	// Direct named struct via the "-->" separator → registered under the ORIGINAL
	// "-->" key (so field-format inference, which looks up that exact spelling,
	// still finds the component).
	used = map[string]*Schema{}
	got, _ = schemaForType(used, "main-->User", nil, metaWithNamedTypes(), cfg, nil)
	require.NotNil(t, got)
	assert.Contains(t, used, "main-->User")

	// Container of a named struct → array of $ref via schemaForRefTree.
	got, _ = schemaForType(map[string]*Schema{}, "[]main.User", nil, metaWithNamedTypes(), cfg, nil)
	require.NotNil(t, got)
	assert.Equal(t, "array", got.Type)

	// Config TypeMapping wins over structural resolution.
	mapped := &APISpecConfig{TypeMapping: []TypeMapping{{GoType: "int", OpenAPIType: &Schema{Type: "string", Format: "int-as-string"}}}}
	got, _ = schemaForType(map[string]*Schema{}, "int", nil, metaWithNamedTypes(), mapped, nil)
	assert.Equal(t, "int-as-string", got.Format)
}

// metaWithNamedTypes builds a metadata with a struct ("User") and an alias
// ("Status") in package "main", for the named-ref resolution tests.
func metaWithNamedTypes() *metadata.Metadata {
	sp := metadata.NewStringPool()
	return &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"main": {Files: map[string]*metadata.File{
				"types.go": {Types: map[string]*metadata.Type{
					"User": {
						Name:   sp.Get("User"),
						Kind:   sp.Get("struct"),
						Fields: []metadata.Field{{Name: sp.Get("Name"), Type: sp.Get("string")}},
					},
					"Status": {
						Name:   sp.Get("Status"),
						Kind:   sp.Get("alias"),
						Target: sp.Get("string"),
					},
				}},
			}},
		},
	}
}

// TestBodyTypeFromMetadataRef covers the body/param alignment seam: it yields the
// canonical TypeRef string only when the named leaf resolves in metadata.
func TestBodyTypeFromMetadataRef(t *testing.T) {
	meta := metaWithNamedTypes()
	user := func() *metadata.TypeRef { return &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"} }

	// Direct and container-wrapped in-metadata leaves → canonical string.
	assert.Equal(t, "main.User", bodyTypeFromMetadataRef(user(), meta, &APISpecConfig{}))
	assert.Equal(t, "[]main.User", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefSlice, Elem: user()}, meta, &APISpecConfig{}))
	assert.Equal(t, "*main.User", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefPointer, Elem: user()}, meta, &APISpecConfig{}))
	assert.Equal(t, "map[string]main.User", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefMap, Key: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, Elem: user()}, meta, &APISpecConfig{}))

	// A configured external type → the major-version-stripped canonical form
	// (matching the config Name and the string path), even behind a wrapper.
	extCfg := &APISpecConfig{ExternalTypes: []ExternalType{{Name: "github.com/gofiber/fiber.Map", OpenAPIType: &Schema{Type: "object"}}}}
	fiberMap := func() *metadata.TypeRef {
		return &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "github.com/gofiber/fiber/v2", Name: "Map"}
	}
	assert.Equal(t, "github.com/gofiber/fiber.Map", bodyTypeFromMetadataRef(fiberMap(), meta, extCfg))
	assert.Equal(t, "[]github.com/gofiber/fiber.Map", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefSlice, Elem: fiberMap()}, meta, extCfg))
	// Same external ref but no matching config → "" (string path).
	assert.Equal(t, "", bodyTypeFromMetadataRef(fiberMap(), meta, &APISpecConfig{}))

	// A generic instantiation whose base type is in metadata resolves too — the
	// args ride along in the canonical string for the string path's substitution.
	assert.Equal(t, "main.User[int]", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User", Args: []*metadata.TypeRef{{Kind: metadata.RefBasic, Name: "int"}}}, meta, &APISpecConfig{}))

	// Other unresolved and non-named leaves → "" (string path).
	assert.Equal(t, "", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "ext", Name: "Thing"}, meta, &APISpecConfig{}))
	assert.Equal(t, "", bodyTypeFromMetadataRef(&metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, meta, &APISpecConfig{}))
	assert.Equal(t, "", bodyTypeFromMetadataRef(nil, meta, &APISpecConfig{}))
}

// TestAliasInlineSchema covers the inline resolution of an alias-to-primitive
// (used for slice/map/array elements), and the cases that defer to componentizing.
func TestAliasInlineSchema(t *testing.T) {
	meta := metaWithNamedTypes() // Status = alias→string, User = struct
	sp := meta.StringPool
	// Add an alias whose underlying is NOT primitive (alias→struct).
	meta.Packages["main"].Files["types.go"].Types["Wrapper"] = &metadata.Type{
		Name: sp.Get("Wrapper"), Kind: sp.Get("alias"), Target: sp.Get("main.User"),
	}

	// Alias→primitive → inline schema of the underlying.
	assert.Equal(t, &Schema{Type: "string"},
		aliasInlineSchema(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Status"}, meta))
	// Non-alias (struct) → nil (componentizes elsewhere).
	assert.Nil(t, aliasInlineSchema(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"}, meta))
	// Alias→non-primitive → nil (componentizes).
	assert.Nil(t, aliasInlineSchema(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Wrapper"}, meta))
	// Unresolved → nil.
	assert.Nil(t, aliasInlineSchema(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Missing"}, meta))
}

// TestVariableTypeString covers the TypeRef-preferred variable type accessor and
// its fallback for an untyped (no-TypeRef) declaration.
func TestVariableTypeString(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	typed := &metadata.Variable{
		Type:    sp.Get("main.Status"),
		TypeRef: &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Status"},
	}
	assert.Equal(t, "main.Status", variableTypeString(typed, meta))

	untyped := &metadata.Variable{Type: sp.Get("string")} // no TypeRef
	assert.Equal(t, "string", variableTypeString(untyped, meta))
}

// TestSchemaForUnresolved covers schemaForType's terminal fallback: external
// config, a primitive arriving as a name, a dangling non-primitive ref, and the
// no-schema cases.
func TestSchemaForUnresolved(t *testing.T) {
	// Empty → nil.
	s, _ := schemaForUnresolved("", &APISpecConfig{})
	assert.Nil(t, s)

	// Configured external type → registers its component schema.
	extCfg := &APISpecConfig{ExternalTypes: []ExternalType{{Name: "pkg.Ext", OpenAPIType: &Schema{Type: "object"}}}}
	s, schemas := schemaForUnresolved("pkg.Ext", extCfg)
	require.NotNil(t, s)
	assert.Contains(t, schemas, "pkg.Ext")

	// A primitive name → its scalar schema.
	s, _ = schemaForUnresolved("time.Time", &APISpecConfig{})
	assert.Equal(t, &Schema{Type: "string", Format: "date-time"}, s)

	// A primitive with no scalar mapping (error) → nil.
	s, _ = schemaForUnresolved("error", &APISpecConfig{})
	assert.Nil(t, s)

	// A container string (not nameable as a component) → nil.
	s, _ = schemaForUnresolved("[]Unknown", &APISpecConfig{})
	assert.Nil(t, s)

	// A non-primitive name → a dangling $ref (raw replaced name).
	s, _ = schemaForUnresolved("github.com/x/y.Thing", &APISpecConfig{})
	require.NotNil(t, s)
	assert.Equal(t, refComponentsSchemasPrefix+"github.com_x_y.Thing", s.Ref)
}

// TestTypeByRef covers the tree-based metadata lookup that replaces
// typeByName(TypeParts(string)) on the named-type path.
func TestTypeByRef(t *testing.T) {
	meta := metaWithNamedTypes()

	// Qualified hit: ref.Pkg matches the package key.
	got := typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"}, meta)
	require.NotNil(t, got)
	assert.Equal(t, "User", getStringFromPool(meta, got.Name))

	// Name-only fallback: ref.Pkg does not match any package key, but the name
	// resolves across packages.
	got = typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "other/path", Name: "User"}, meta)
	require.NotNil(t, got)
	assert.Equal(t, "User", getStringFromPool(meta, got.Name))

	// Unqualified (no Pkg): still found via the fallback scan.
	got = typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed, Name: "Status"}, meta)
	require.NotNil(t, got)

	// Misses and invalid inputs.
	assert.Nil(t, typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed, Name: "Missing"}, meta))
	assert.Nil(t, typeByRef(&metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}, meta)) // not named
	assert.Nil(t, typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed}, meta))                 // empty name
	assert.Nil(t, typeByRef(nil, meta))
	assert.Nil(t, typeByRef(&metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"}, nil)) // nil meta
}

// TestSchemaForNamedRef covers the named-ref interception: structs resolve on the
// tree to a component $ref; aliases, config-mapped types, and unfound types defer
// to the string path; the cycle/usedTypes guards short-circuit to a $ref.
func TestSchemaForNamedRef(t *testing.T) {
	meta := metaWithNamedTypes()
	userRef := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"}

	// Struct → handled (ok), returns a $ref to the registered component.
	s, schemas, ok := schemaForNamedRef(map[string]*Schema{}, userRef, "", meta, &APISpecConfig{}, map[string]bool{})
	require.True(t, ok)
	require.NotNil(t, s)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", s.Ref)
	assert.Contains(t, schemas, "main.User")

	// Direct alias → componentizes via generateAliasSchema (the underlying primitive
	// as a named component). An alias FIELD is pre-resolved upstream so it never
	// reaches here; an alias container ELEMENT is inlined by schemaForRefTree.
	_, _, ok = schemaForNamedRef(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Status"}, "", meta, &APISpecConfig{}, map[string]bool{})
	assert.True(t, ok)

	// Not found → emits a dangling $ref (raw name) to a component that won't exist.
	s, _, ok = schemaForNamedRef(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Missing"}, "", meta, &APISpecConfig{}, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, refComponentsSchemasPrefix+"main.Missing", s.Ref)

	// Config TypeMapping match → deferred (schemaForType applies it before the walk).
	cfgMap := &APISpecConfig{TypeMapping: []TypeMapping{{GoType: "main.User", OpenAPIType: &Schema{Type: "string"}}}}
	_, _, ok = schemaForNamedRef(map[string]*Schema{}, userRef, "", meta, cfgMap, map[string]bool{})
	assert.False(t, ok)

	// Config ExternalType match → registers the component and references it.
	cfgExt := &APISpecConfig{ExternalTypes: []ExternalType{{Name: "main.User", OpenAPIType: &Schema{Type: "object"}}}}
	s, exns, ok := schemaForNamedRef(map[string]*Schema{}, userRef, "", meta, cfgExt, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", s.Ref)
	assert.Contains(t, exns, "main.User")

	// Cycle guard: the type already in-flight → short-circuit to a $ref.
	visiting := map[string]bool{"main.User" + mapGoTypeToOpenAPISchemaKey: true}
	s, _, ok = schemaForNamedRef(map[string]*Schema{}, userRef, "", meta, &APISpecConfig{}, visiting)
	require.True(t, ok)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", s.Ref)

	// usedTypes already resolved → short-circuit to a $ref.
	used := map[string]*Schema{"main.User": {Type: "object"}}
	s, _, ok = schemaForNamedRef(used, userRef, "", meta, &APISpecConfig{}, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, refComponentsSchemasPrefix+"main.User", s.Ref)
}

// TestSchemaForRefTree covers the recursive tree walker that wraps a named struct
// leaf in pointers/slices/arrays, and defers the forms it does not yet reproduce.
func TestSchemaForRefTree(t *testing.T) {
	cfg := &APISpecConfig{}
	userRef := func() *metadata.TypeRef { return &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User"} }
	userArrayItems := &Schema{Ref: refComponentsSchemasPrefix + "main.User"}

	// []User → {array, items:$ref}.
	s, _, ok := schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefSlice, Elem: userRef()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "array", Items: userArrayItems}, s)

	// [3]User → {array, items:$ref, minItems==maxItems==3}.
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefArray, Len: 3, Elem: userRef()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "array", Items: userArrayItems, MinItems: 3, MaxItems: 3}, s)

	// []*User → pointer is transparent, same as []User.
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefSlice, Elem: &metadata.TypeRef{Kind: metadata.RefPointer, Elem: userRef()}}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "array", Items: userArrayItems}, s)

	// map[string]User → {object, additionalProperties:$ref}.
	strKey := &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefMap, Key: strKey, Elem: userRef()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "object", AdditionalProperties: userArrayItems}, s)

	// A generic instantiation whose base type is in metadata resolves (the args
	// substitute via generateStructSchema) → ok=true.
	_, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "User", Args: []*metadata.TypeRef{{Kind: metadata.RefBasic, Name: "int"}}}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	assert.True(t, ok)

	// A non-string-keyed map → generic object (not expressible with typed values).
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefMap, Key: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "int"}, Elem: userRef()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "object"}, s)

	// Deferred forms: func, nil.
	// A slice/map whose element is an alias-to-primitive INLINES the underlying
	// (with any enum), rather than componentizing — matching the string path.
	statusElem := func() *metadata.TypeRef {
		return &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "main", Name: "Status"}
	}
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefSlice, Elem: statusElem()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "array", Items: &Schema{Type: "string"}}, s)
	s, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefMap, Key: strKey, Elem: statusElem()}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	require.True(t, ok)
	assert.Equal(t, &Schema{Type: "object", AdditionalProperties: &Schema{Type: "string"}}, s)
	_, _, ok = schemaForRefTree(map[string]*Schema{}, &metadata.TypeRef{Kind: metadata.RefFunc}, false, metaWithNamedTypes(), cfg, map[string]bool{})
	assert.False(t, ok) // func/chan/basic → string path
	_, _, ok = schemaForRefTree(map[string]*Schema{}, nil, false, metaWithNamedTypes(), cfg, map[string]bool{})
	assert.False(t, ok)
}

// TestSchemaForType_FixedArrayLengthReapplied covers the seam's re-application of
// a fixed array's length when its element defers to the string path (a named
// struct/generic element, e.g. [4]Point): schemaFromTypeRef returns nil, the
// string path resolves the array shape, and schemaForType stamps minItems ==
// maxItems == Len back on from the TypeRef. Slices and inferred-length arrays
// ([...]T) stay unconstrained. The "[]string" goType is a stand-in for whatever
// length-less array string the flattener produced; only the post-apply matters.
func TestSchemaForType_FixedArrayLengthReapplied(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := &metadata.Metadata{Packages: map[string]*metadata.Package{}}
	named := func() *metadata.TypeRef { return &metadata.TypeRef{Kind: metadata.RefNamed, Name: "Thing"} }

	// Fixed array, known length: re-applied.
	arr := &metadata.TypeRef{Kind: metadata.RefArray, Len: 5, Elem: named()}
	got, _ := schemaForType(map[string]*Schema{}, "", arr, meta, cfg, nil)
	require.NotNil(t, got)
	assert.Equal(t, "array", got.Type)
	assert.Equal(t, 5, got.MinItems)
	assert.Equal(t, 5, got.MaxItems)

	// Slice element that also resolves to an array via the string path: no length.
	sl := &metadata.TypeRef{Kind: metadata.RefSlice, Elem: named()}
	gotSlice, _ := schemaForType(map[string]*Schema{}, "", sl, meta, cfg, nil)
	require.NotNil(t, gotSlice)
	assert.Equal(t, 0, gotSlice.MinItems)
	assert.Equal(t, 0, gotSlice.MaxItems)

	// Inferred-length array ([...]T, Len -1): no constraint (Len < 0 guard).
	inferred := &metadata.TypeRef{Kind: metadata.RefArray, Len: -1, Elem: named()}
	gotInf, _ := schemaForType(map[string]*Schema{}, "", inferred, meta, cfg, nil)
	require.NotNil(t, gotInf)
	assert.Equal(t, 0, gotInf.MinItems)
	assert.Equal(t, 0, gotInf.MaxItems)
}
