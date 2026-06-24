// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// jsonFieldName distinguishes the three cases a name-only tag reader collapses:
// no tag, an explicit skip (`json:"-"`), and a name (possibly empty).
func TestJSONFieldName(t *testing.T) {
	cases := []struct {
		tag         string
		wantName    string
		wantSkip    bool
		wantPresent bool
	}{
		{tag: ``, wantName: "", wantSkip: false, wantPresent: false},
		{tag: `gorm:"column:x"`, wantName: "", wantSkip: false, wantPresent: false},
		{tag: `json:"id"`, wantName: "id", wantSkip: false, wantPresent: true},
		{tag: `json:"id,omitempty"`, wantName: "id", wantSkip: false, wantPresent: true},
		{tag: `json:",omitempty"`, wantName: "", wantSkip: false, wantPresent: true},
		{tag: `json:",string"`, wantName: "", wantSkip: false, wantPresent: true},
		{tag: `json:"-"`, wantName: "", wantSkip: true, wantPresent: true},
		// The Go subtlety: a trailing comma names a field literally "-" (NOT a skip).
		{tag: `json:"-,"`, wantName: "-", wantSkip: false, wantPresent: true},
		{tag: `json:"-,omitempty"`, wantName: "-", wantSkip: false, wantPresent: true},
		{tag: `json:"name" validate:"required"`, wantName: "name", wantSkip: false, wantPresent: true},
	}
	for _, c := range cases {
		name, skip, present := jsonFieldName(c.tag)
		assert.Equal(t, c.wantName, name, "name for %q", c.tag)
		assert.Equal(t, c.wantSkip, skip, "skip for %q", c.tag)
		assert.Equal(t, c.wantPresent, present, "present for %q", c.tag)
	}
}

func TestJSONHasStringOption(t *testing.T) {
	assert.True(t, jsonHasStringOption(`json:",string"`))
	assert.True(t, jsonHasStringOption(`json:"count,string"`))
	assert.True(t, jsonHasStringOption(`json:"count,omitempty,string"`))
	assert.False(t, jsonHasStringOption(`json:"count"`))
	assert.False(t, jsonHasStringOption(`json:"count,omitempty"`))
	// "string" as the NAME (not an option) does not count.
	assert.False(t, jsonHasStringOption(`json:"string"`))
	assert.False(t, jsonHasStringOption(`gorm:"x"`))
	assert.False(t, jsonHasStringOption(``))
}

func TestApplyJSONStringOption(t *testing.T) {
	// Integer with a format/min becomes a bare string, dropping numeric facets.
	s := &Schema{Type: "integer", Format: "int32", Minimum: floatPtr(0), Maximum: floatPtr(10), MultipleOf: 2}
	applyJSONStringOption(s)
	assert.Equal(t, "string", s.Type)
	assert.Empty(t, s.Format)
	assert.Nil(t, s.Minimum)
	assert.Nil(t, s.Maximum)
	assert.Zero(t, s.MultipleOf)

	// number and boolean likewise become string.
	num := &Schema{Type: "number"}
	applyJSONStringOption(num)
	assert.Equal(t, "string", num.Type)
	b := &Schema{Type: "boolean"}
	applyJSONStringOption(b)
	assert.Equal(t, "string", b.Type)

	// Already a string: no-op.
	str := &Schema{Type: "string", Format: "uuid"}
	applyJSONStringOption(str)
	assert.Equal(t, "string", str.Type)
	assert.Equal(t, "uuid", str.Format)

	// Non-scalar (object/array/$ref) is left untouched — Go ignores ,string there.
	obj := &Schema{Type: "object"}
	applyJSONStringOption(obj)
	assert.Equal(t, "object", obj.Type)
	ref := &Schema{Ref: "#/components/schemas/X"}
	applyJSONStringOption(ref)
	assert.Equal(t, "#/components/schemas/X", ref.Ref)
	assert.Empty(t, ref.Type)

	// nil is safe.
	applyJSONStringOption(nil)
}

// stringifyEnumValues converts non-string enum members to the string forms a
// `,string`-tagged field marshals (FIX 4), leaving existing strings untouched and
// preserving order.
func TestStringifyEnumValues(t *testing.T) {
	got := stringifyEnumValues([]interface{}{int64(0), int64(1), int64(2)})
	assert.Equal(t, []interface{}{"0", "1", "2"}, got, "ints become quoted-string forms")

	assert.Equal(t, []interface{}{"true", "false"}, stringifyEnumValues([]interface{}{true, false}),
		"bools become their string forms")

	// Floats use encoding/json's number form, NOT fmt's %v: 1e6 -> "1000000",
	// not "1e+06" (which no real ,string wire value would ever match).
	assert.Equal(t, []interface{}{"1000000", "0.0001"},
		stringifyEnumValues([]interface{}{float64(1000000), float64(0.0001)}),
		"floats match encoding/json, not fmt scientific notation")

	// Already strings: kept as-is; order preserved alongside a converted value.
	assert.Equal(t, []interface{}{"admin", "1"}, stringifyEnumValues([]interface{}{"admin", int64(1)}),
		"existing strings untouched, order preserved")

	// json.Marshal-unrepresentable value (Inf) falls back to fmt %v without panicking.
	assert.Equal(t, []interface{}{"+Inf"}, stringifyEnumValues([]interface{}{math.Inf(1)}),
		"a value json cannot marshal falls back to fmt %v")

	assert.Empty(t, stringifyEnumValues(nil))
}

// scalarField is a small builder for a primitive struct field with a tag.
func scalarField(sp *metadata.StringPool, name, prim, tag string) metadata.Field {
	return metadata.Field{
		Name:    sp.Get(name),
		Type:    sp.Get(prim),
		Tag:     sp.Get(tag),
		Scope:   sp.Get(metadata.ScopeExported),
		TypeRef: &metadata.TypeRef{Kind: metadata.RefBasic, Name: prim},
	}
}

// generateStructSchema must drop unexported fields and `json:"-"` fields, keep a
// `json:"-,"` field under the literal name "-", and force {type:string} for a
// `,string` scalar (FIX 1, FIX 2, FIX 4 together).
func TestGenerateStructSchema_VisibilityAndStringOption(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	unexported := scalarField(sp, "secret", "string", "")
	unexported.Scope = sp.Get(metadata.ScopeUnexported)

	typ := &metadata.Type{
		Name: sp.Get("Account"),
		Pkg:  sp.Get("myapp"),
		Kind: sp.Get("struct"),
		Fields: []metadata.Field{
			scalarField(sp, "ID", "int", `json:"id"`),
			unexported,
			scalarField(sp, "Token", "string", `json:"-"`),
			scalarField(sp, "Dash", "string", `json:"-,"`),
			scalarField(sp, "Balance", "int64", `json:",string"`),
			scalarField(sp, "Active", "bool", `json:"active,string"`),
		},
	}

	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	schema, _ := generateStructSchema(map[string]*Schema{}, "myapp-->Account", typ, meta, cfg, map[string]bool{})
	require.NotNil(t, schema)

	props := schema.Properties
	// Unexported and json:"-" are gone.
	assert.NotContains(t, props, "secret")
	assert.NotContains(t, props, "Token")
	assert.NotContains(t, props, "token")
	// json:"-," keeps the literal "-" name.
	require.Contains(t, props, "-")
	assert.Equal(t, "string", props["-"].Type)
	// ,string forces string on a number and a bool.
	require.Contains(t, props, "Balance")
	assert.Equal(t, "string", props["Balance"].Type)
	require.Contains(t, props, "active")
	assert.Equal(t, "string", props["active"].Type)
	// Plain exported field is unchanged.
	require.Contains(t, props, "id")
	assert.Equal(t, "integer", props["id"].Type)
	// Required excludes the dropped fields.
	assert.NotContains(t, schema.Required, "secret")
	assert.NotContains(t, schema.Required, "token")
}

func TestBindTypeParams(t *testing.T) {
	// No args -> nil (so SubstituteParams short-circuits).
	assert.Nil(t, bindTypeParams(nil, nil))

	intRef := &metadata.TypeRef{Kind: metadata.RefBasic, Name: "int"}
	strRef := &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"}

	// Declared names bind by name (Pair[K,V]).
	got := bindTypeParams([]*metadata.TypeRef{intRef, strRef}, []string{"K", "V"})
	assert.Same(t, intRef, got["K"])
	assert.Same(t, strRef, got["V"])

	// Fewer declared names than args -> positional fallback for the extras.
	got = bindTypeParams([]*metadata.TypeRef{intRef, strRef}, []string{"K"})
	assert.Same(t, intRef, got["K"])
	assert.Same(t, strRef, got["U"])

	// No declared names -> all positional.
	got = bindTypeParams([]*metadata.TypeRef{intRef, strRef}, nil)
	assert.Same(t, intRef, got["T"])
	assert.Same(t, strRef, got["U"])

	// More args than positional letters AND no declared names: the first 7 bind to
	// T..Z, and the 8th has no sound name — it is SKIPPED, never collided onto "T".
	many := make([]*metadata.TypeRef, 8)
	for i := range many {
		many[i] = &metadata.TypeRef{Kind: metadata.RefBasic, Name: "t" + string(rune('0'+i))}
	}
	got = bindTypeParams(many, nil)
	assert.Len(t, got, 7, "only the 7 positional letters bind; the 8th arg is skipped")
	assert.Same(t, many[0], got["T"], "first arg keeps T — not clobbered by the 8th")
	assert.Same(t, many[6], got["Z"])
}

func TestRefHasUnboundParam(t *testing.T) {
	assert.False(t, refHasUnboundParam(nil))
	assert.False(t, refHasUnboundParam(&metadata.TypeRef{Kind: metadata.RefBasic, Name: "int"}))
	assert.True(t, refHasUnboundParam(&metadata.TypeRef{Kind: metadata.RefParam, Name: "T"}))
	// nested in a map value
	assert.True(t, refHasUnboundParam(&metadata.TypeRef{
		Kind: metadata.RefMap,
		Key:  &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"},
		Elem: &metadata.TypeRef{Kind: metadata.RefParam, Name: "T"},
	}))
	// nested in an arg
	assert.True(t, refHasUnboundParam(&metadata.TypeRef{
		Kind: metadata.RefNamed, Pkg: "p", Name: "Box",
		Args: []*metadata.TypeRef{{Kind: metadata.RefParam, Name: "T"}},
	}))
	// fully bound map
	assert.False(t, refHasUnboundParam(&metadata.TypeRef{
		Kind: metadata.RefMap,
		Key:  &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"},
		Elem: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "int"},
	}))
}

// definedOtherUnderlying must substitute a generic instantiation's args into the
// captured underlying (FIX 5), and omit (return "") when a param stays unbound.
func TestDefinedOtherUnderlying_GenericSubstitution(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// type Box[T any] map[string]T, registered under package "myapp".
	boxType := &metadata.Type{
		Name:       sp.Get("Box"),
		Pkg:        sp.Get("myapp"),
		Kind:       sp.Get("other"),
		TypeParams: []string{"T"},
		UnderlyingRef: &metadata.TypeRef{
			Kind: metadata.RefMap,
			Key:  &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"},
			Elem: &metadata.TypeRef{Kind: metadata.RefParam, Name: "T"},
		},
	}
	meta.Packages["myapp"] = &metadata.Package{
		Types: map[string]*metadata.Type{},
		Files: map[string]*metadata.File{
			"f.go": {Types: map[string]*metadata.Type{"Box": boxType}},
		},
	}

	// Box[int] -> map[string]int
	boxInt := &metadata.TypeRef{
		Kind: metadata.RefNamed, Pkg: "myapp", Name: "Box",
		Args: []*metadata.TypeRef{{Kind: metadata.RefBasic, Name: "int"}},
	}
	assert.Equal(t, "map[string]int", definedOtherUnderlying(boxInt, meta))

	// *Box[string] resolves like Box[string] -> map[string]string
	ptrBoxStr := &metadata.TypeRef{Kind: metadata.RefPointer, Elem: &metadata.TypeRef{
		Kind: metadata.RefNamed, Pkg: "myapp", Name: "Box",
		Args: []*metadata.TypeRef{{Kind: metadata.RefBasic, Name: "string"}},
	}}
	assert.Equal(t, "map[string]string", definedOtherUnderlying(ptrBoxStr, meta))

	// Box without args leaves T unbound -> omit (return "").
	boxBare := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "myapp", Name: "Box"}
	assert.Equal(t, "", definedOtherUnderlying(boxBare, meta))
}

// boxOtherMeta registers `type Box[T any] map[string]T` (Kind "other") so the
// element-path generic substitution in schemaForOtherKind can be exercised.
func boxOtherMeta() (*metadata.Metadata, *metadata.Type) {
	meta := newTestMeta()
	sp := meta.StringPool
	box := &metadata.Type{
		Name:       sp.Get("Box"),
		Pkg:        sp.Get("pkg"),
		Kind:       sp.Get("other"),
		TypeParams: []string{"T"},
		UnderlyingRef: &metadata.TypeRef{
			Kind: metadata.RefMap,
			Key:  &metadata.TypeRef{Kind: metadata.RefBasic, Name: "string"},
			Elem: &metadata.TypeRef{Kind: metadata.RefParam, Name: "T"},
		},
	}
	meta.Packages["pkg"] = &metadata.Package{
		Files: map[string]*metadata.File{"f.go": {Types: map[string]*metadata.Type{"Box": box}}},
	}
	return meta, box
}

// schemaForOtherKind must substitute a generic instantiation's args (the ELEMENT
// path, e.g. []Box[int]) and omit when a param stays unbound (FIX 5).
func TestSchemaForOtherKind_GenericSubstitution(t *testing.T) {
	t.Run("Box[int] element -> map[string]int", func(t *testing.T) {
		meta, box := boxOtherMeta()
		ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "pkg", Name: "Box",
			Args: []*metadata.TypeRef{{Kind: metadata.RefBasic, Name: "int"}}}
		s, _, ok := schemaForOtherKind(map[string]*Schema{}, box, ref, "pkg.Box[int]", meta, &APISpecConfig{}, map[string]bool{})
		assert.True(t, ok)
		assert.Equal(t, &Schema{Type: "object", AdditionalProperties: &Schema{Type: "integer"}}, s)
	})

	t.Run("unbound param -> omit", func(t *testing.T) {
		meta, box := boxOtherMeta()
		// ref carries an arg, but it is itself another (unbound) type parameter, so
		// substitution leaves a RefParam and the field must be omitted (nil schema).
		ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "pkg", Name: "Box",
			Args: []*metadata.TypeRef{{Kind: metadata.RefParam, Name: "U"}}}
		s, ns, ok := schemaForOtherKind(map[string]*Schema{}, box, ref, "pkg.Box[U]", meta, &APISpecConfig{}, map[string]bool{})
		assert.True(t, ok)
		assert.Nil(t, s)
		assert.Nil(t, ns)
	})
}

// canAddRefSchemaForType must treat json.RawMessage as non-nameable (it is
// inlined as {}), so it is never componentized — parallel to func/chan (FIX 3).
func TestCanAddRefSchemaForType_RawMessage(t *testing.T) {
	assert.False(t, canAddRefSchemaForType("encoding/json.RawMessage"))
	assert.False(t, canAddRefSchemaForType("*encoding/json.RawMessage"))
	assert.False(t, canAddRefSchemaForType("[]encoding/json.RawMessage"))
	// A normal named type stays componentizable.
	assert.True(t, canAddRefSchemaForType("pkg.User"))
}

// schemaFromTypeRef maps json.RawMessage (and its pointer/slice wrappers) to the
// empty schema {}, while a plain []byte stays a base64 string (FIX 3).
func TestSchemaFromTypeRef_RawMessage(t *testing.T) {
	raw := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "encoding/json", Name: "RawMessage"}
	assert.Equal(t, &Schema{}, schemaFromTypeRef(raw, nil))
	// pointer is transparent
	assert.Equal(t, &Schema{}, schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefPointer, Elem: raw}, nil))
	// slice -> array of {}
	assert.Equal(t, &Schema{Type: "array", Items: &Schema{}},
		schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefSlice, Elem: raw}, nil))
	// plain []byte is unaffected: base64 string
	assert.Equal(t, &Schema{Type: "string", Format: "byte"},
		schemaFromTypeRef(&metadata.TypeRef{Kind: metadata.RefSlice, Elem: &metadata.TypeRef{Kind: metadata.RefBasic, Name: "byte"}}, nil))
}
