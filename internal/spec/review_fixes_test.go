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

// TestLeafPkgInMetadata covers the gate helper: unqualified or analyzed-package
// refs pass; an external (unanalyzed) package does not.
func TestLeafPkgInMetadata(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{"github.com/me/app": {}}
	assert.True(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: ""}, meta), "unqualified allowed")
	assert.True(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "github.com/me/app"}, meta), "analyzed package")
	assert.False(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "github.com/acme/extpkg"}, meta), "external package")
	assert.False(t, leafPkgInMetadata(nil, meta))
	assert.False(t, leafPkgInMetadata(&metadata.TypeRef{Pkg: "x"}, nil))
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
