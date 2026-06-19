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
		want, _ := mapGoTypeToOpenAPISchema(map[string]*Schema{}, ref.String(), meta, cfg, nil)
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
	got, _ := schemaForType(map[string]*Schema{}, "[]string", arr, meta, cfg, nil)
	require.NotNil(t, got)
	assert.Equal(t, "array", got.Type)
	assert.Equal(t, 5, got.MinItems)
	assert.Equal(t, 5, got.MaxItems)

	// Slice element that also resolves to an array via the string path: no length.
	sl := &metadata.TypeRef{Kind: metadata.RefSlice, Elem: named()}
	gotSlice, _ := schemaForType(map[string]*Schema{}, "[]string", sl, meta, cfg, nil)
	require.NotNil(t, gotSlice)
	assert.Equal(t, 0, gotSlice.MinItems)
	assert.Equal(t, 0, gotSlice.MaxItems)

	// Inferred-length array ([...]T, Len -1): no constraint (Len < 0 guard).
	inferred := &metadata.TypeRef{Kind: metadata.RefArray, Len: -1, Elem: named()}
	gotInf, _ := schemaForType(map[string]*Schema{}, "[]string", inferred, meta, cfg, nil)
	require.NotNil(t, gotInf)
	assert.Equal(t, 0, gotInf.MinItems)
	assert.Equal(t, 0, gotInf.MaxItems)
}
