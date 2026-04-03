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

// helper to build a minimal TypeResolverImpl with a metadata that has a string pool
func newTestResolver(meta *metadata.Metadata) *TypeResolverImpl {
	cfg := DefaultAPISpecConfig()
	schemaMapper := NewSchemaMapper(cfg)
	return NewTypeResolver(meta, cfg, schemaMapper)
}

func newTypeResolverTestMeta() (*metadata.Metadata, *metadata.StringPool) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}
	return meta, sp
}

// makeArg is a helper to create a CallArgument with the right Meta pointer and default -1 fields.
func makeArg(meta *metadata.Metadata, kind string, opts ...func(*metadata.CallArgument)) metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(kind)
	for _, opt := range opts {
		opt(a)
	}
	return *a
}

// ---------------------------------------------------------------------------
// 1. resolveTypeFromArgument — switch on arg.GetKind()
// ---------------------------------------------------------------------------

func TestResolveTypeFromArgument_KindIdent(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("myVar")
		a.SetType("int")
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "int", result)
}

func TestResolveTypeFromArgument_KindSelector(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Field")
	})
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("pkg")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "pkg.Field", result)
}

func TestResolveTypeFromArgument_KindCall_NilFun(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindCall)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "func()", result)
}

func TestResolveTypeFromArgument_KindCall_WithFun(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("MyFunc")
		a.SetType("func() string")
	})
	arg := makeArg(meta, metadata.KindCall, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveTypeFromArgument(arg)
	// resolveCallType extracts return type from func(...) ReturnType
	assert.Equal(t, "string", result)
}

func TestResolveTypeFromArgument_KindTypeConversion(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("string")
		a.SetType("string")
	})
	arg := makeArg(meta, metadata.KindTypeConversion, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "string", result)
}

func TestResolveTypeFromArgument_KindTypeConversion_NilFun(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindTypeConversion)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "", result)
}

func TestResolveTypeFromArgument_KindUnary_WithX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("val")
		a.SetType("MyStruct")
	})
	arg := makeArg(meta, metadata.KindUnary, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	// baseType is "MyStruct", not prefixed with *, so pointer is added
	assert.Equal(t, "*MyStruct", result)
}

func TestResolveTypeFromArgument_KindUnary_Dereference(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("ptr")
		a.SetType("*MyStruct")
	})
	arg := makeArg(meta, metadata.KindUnary, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "MyStruct", result)
}

func TestResolveTypeFromArgument_KindUnary_NilX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindUnary, func(a *metadata.CallArgument) {
		a.SetType("Foo")
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "*Foo", result)
}

func TestResolveTypeFromArgument_KindStar(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("v")
		a.SetType("int")
	})
	arg := makeArg(meta, metadata.KindStar, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "*int", result)
}

func TestResolveTypeFromArgument_KindCompositeLit_WithX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("MyStruct")
		a.SetType("MyStruct")
	})
	arg := makeArg(meta, metadata.KindCompositeLit, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "MyStruct", result)
}

func TestResolveTypeFromArgument_KindCompositeLit_NilX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindCompositeLit)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, metadata.KindCompositeLit, result)
}

func TestResolveTypeFromArgument_KindIndex_SliceType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("items")
		a.SetType("[]string")
	})
	arg := makeArg(meta, metadata.KindIndex, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "string", result)
}

func TestResolveTypeFromArgument_KindIndex_MapType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("m")
		a.SetType("map[string]int")
	})
	arg := makeArg(meta, metadata.KindIndex, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "int", result)
}

func TestResolveTypeFromArgument_KindIndex_NilX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIndex)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, metadata.KindIndex, result)
}

func TestResolveTypeFromArgument_KindInterfaceType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindInterfaceType)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "interface{}", result)
}

func TestResolveTypeFromArgument_KindMapType_Full(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	keyArg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("string")
		a.SetType("string")
	})
	valArg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("int")
		a.SetType("int")
	})
	arg := makeArg(meta, metadata.KindMapType, func(a *metadata.CallArgument) {
		a.X = &keyArg
		a.Fun = &valArg
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "map[string]int", result)
}

func TestResolveTypeFromArgument_KindMapType_NilParts(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindMapType)
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "map", result)
}

func TestResolveTypeFromArgument_KindLiteral(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindLiteral, func(a *metadata.CallArgument) {
		a.SetType("string")
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "string", result)
}

func TestResolveTypeFromArgument_KindRaw(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindRaw, func(a *metadata.CallArgument) {
		a.SetRaw("some.RawExpr")
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "some.RawExpr", result)
}

func TestResolveTypeFromArgument_DefaultKind(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Use a kind that doesn't match any case
	arg := makeArg(meta, "unknown_kind", func(a *metadata.CallArgument) {
		a.SetType("fallbackType")
	})
	result := resolver.resolveTypeFromArgument(arg)
	assert.Equal(t, "fallbackType", result)
}

// ---------------------------------------------------------------------------
// 2. resolveIdentType
// ---------------------------------------------------------------------------

func TestResolveIdentType_DirectType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("x")
		a.SetType("float64")
	})
	result := resolver.resolveIdentType(arg)
	assert.Equal(t, "float64", result)
}

func TestResolveIdentType_FromMetadataVariable(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	meta.Packages["mypkg"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"file.go": {
				Variables: map[string]*metadata.Variable{
					"counter": {
						Name: sp.Get("counter"),
						Type: sp.Get("int"),
					},
				},
			},
		},
	}
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("counter")
		a.SetPkg("mypkg")
		// Type is -1 by default from NewCallArgument, so metadata lookup is used
	})
	result := resolver.resolveIdentType(arg)
	assert.Equal(t, "int", result)
}

func TestResolveIdentType_FallbackToName(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("unknownVar")
		// No type, no pkg
	})
	result := resolver.resolveIdentType(arg)
	assert.Equal(t, "unknownVar", result)
}

func TestResolveIdentType_PkgNotFoundFallback(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("someVar")
		a.SetPkg("nonexistent")
	})
	result := resolver.resolveIdentType(arg)
	assert.Equal(t, "someVar", result)
}

// ---------------------------------------------------------------------------
// 3. ResolveType — dispatching with context
// ---------------------------------------------------------------------------

func TestResolveType_NilContext(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("v")
		a.SetType("bool")
	})
	result := resolver.ResolveType(arg, nil)
	assert.Equal(t, "bool", result)
}

func TestResolveType_ContextWithNilEdge(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	node := &TrackerNode{}
	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("v")
		a.SetType("string")
	})
	result := resolver.ResolveType(arg, node)
	assert.Equal(t, "string", result)
}

func TestResolveType_TypeParamMapResolution(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: sp.Get("caller"),
			Pkg:  sp.Get("main"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get("callee"),
			Pkg:  sp.Get("main"),
		},
		TypeParamMap: map[string]string{
			"T": "string",
		},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("T")
	})
	result := resolver.ResolveType(arg, node)
	assert.Equal(t, "string", result)
}

// ---------------------------------------------------------------------------
// 4. ResolveGenericType
// ---------------------------------------------------------------------------

func TestResolveGenericType_SingleParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[T]", map[string]string{"T": "string"})
	assert.Equal(t, "MyType[string]", result)
}

func TestResolveGenericType_MultipleParams(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("Map[K,V]", map[string]string{"K": "string", "V": "int"})
	assert.Equal(t, "Map[string,int]", result)
}

func TestResolveGenericType_NoTypeParams(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[T]", map[string]string{})
	// With empty typeParams and non-empty param string, returns base type
	assert.Equal(t, "MyType[T]", result)
}

func TestResolveGenericType_EmptyBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[]", map[string]string{})
	assert.Equal(t, "MyType", result)
}

func TestResolveGenericType_NoBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("SimpleType", map[string]string{"T": "string"})
	// No brackets, extractBaseTypeAndParams returns ("SimpleType", ""), so paramStr is empty
	assert.Equal(t, "SimpleType", result)
}

func TestResolveGenericType_NestedGeneric(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("Outer[Inner[T]]", map[string]string{"T": "int"})
	assert.Equal(t, "Outer[Inner[int]]", result)
}

func TestResolveGenericType_ParamNotInMap(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("Foo[X]", map[string]string{"T": "string"})
	// X is not in the map, so it stays as is
	assert.Equal(t, "Foo[X]", result)
}

// ---------------------------------------------------------------------------
// 5. findConcreteTypeByName
// ---------------------------------------------------------------------------

func TestFindConcreteTypeByName_DirectMatch(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.findConcreteTypeByName("T", map[string]string{"T": "string"})
	assert.Equal(t, "string", result)
}

func TestFindConcreteTypeByName_NoMatch(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.findConcreteTypeByName("X", map[string]string{"T": "string"})
	assert.Equal(t, "", result)
}

func TestFindConcreteTypeByName_ExtractedMatch(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// extractParameterName for a single uppercase letter returns the letter itself
	result := resolver.findConcreteTypeByName("K", map[string]string{"K": "string"})
	assert.Equal(t, "string", result)
}

// ---------------------------------------------------------------------------
// 6. generateParameterName
// ---------------------------------------------------------------------------

func TestGenerateParameterName_FirstThree(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	assert.Equal(t, "T", resolver.generateParameterName(0))
	assert.Equal(t, "U", resolver.generateParameterName(1))
	assert.Equal(t, "V", resolver.generateParameterName(2))
}

func TestGenerateParameterName_Index25(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// index 25 => 'T' + 25 = rune('T'+25)
	expected := string(rune('T' + 25))
	assert.Equal(t, expected, resolver.generateParameterName(25))
}

func TestGenerateParameterName_Index26_WithNumber(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// index 26 => base = 26 % 26 = 0 => 'T', number = 26/26 = 1 => "T1"
	assert.Equal(t, "T1", resolver.generateParameterName(26))
}

func TestGenerateParameterName_Index27_WithNumber(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// index 27 => base = 27 % 26 = 1 => 'U', number = 27/26 = 1 => "U1"
	assert.Equal(t, "U1", resolver.generateParameterName(27))
}

// ---------------------------------------------------------------------------
// 7. MapToOpenAPISchema
// ---------------------------------------------------------------------------

func TestMapToOpenAPISchema_String(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("string")
	require.NotNil(t, schema)
	assert.Equal(t, "string", schema.Type)
}

func TestMapToOpenAPISchema_Int(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("int")
	require.NotNil(t, schema)
	assert.Equal(t, "integer", schema.Type)
}

func TestMapToOpenAPISchema_Bool(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("bool")
	require.NotNil(t, schema)
	assert.Equal(t, "boolean", schema.Type)
}

func TestMapToOpenAPISchema_Float64(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("float64")
	require.NotNil(t, schema)
	assert.Equal(t, "number", schema.Type)
}

func TestMapToOpenAPISchema_TimeTime(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("time.Time")
	require.NotNil(t, schema)
	// time.Time is not special-cased, so it becomes a $ref to a schema component
	assert.Contains(t, schema.Ref, "time.Time")
}

func TestMapToOpenAPISchema_Byte(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("byte")
	require.NotNil(t, schema)
	assert.Equal(t, "integer", schema.Type)
}

func TestMapToOpenAPISchema_EmptyString(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("")
	assert.Nil(t, schema)
}

func TestMapToOpenAPISchema_InterfaceType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	schema := resolver.MapToOpenAPISchema("interface{}")
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
}

// ---------------------------------------------------------------------------
// 8. ExtractTypeParameters
// ---------------------------------------------------------------------------

func TestExtractTypeParameters_NoBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	params := resolver.ExtractTypeParameters("SimpleType")
	assert.Empty(t, params)
}

func TestExtractTypeParameters_SingleParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	params := resolver.ExtractTypeParameters("List[string]")
	require.Len(t, params, 1)
	assert.Equal(t, "string", params["T"])
}

func TestExtractTypeParameters_MultipleParams(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	params := resolver.ExtractTypeParameters("Map[string,int]")
	require.Len(t, params, 2)
	assert.Equal(t, "string", params["T"])
	assert.Equal(t, "int", params["U"])
}

func TestExtractTypeParameters_EmptyBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	params := resolver.ExtractTypeParameters("Foo[]")
	assert.Empty(t, params)
}

// ---------------------------------------------------------------------------
// 9. splitTypeParameters
// ---------------------------------------------------------------------------

func TestSplitTypeParameters_Simple(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("A,B,C")
	assert.Equal(t, []string{"A", "B", "C"}, result)
}

func TestSplitTypeParameters_Nested(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("Map[K,V],string")
	assert.Equal(t, []string{"Map[K,V]", "string"}, result)
}

func TestSplitTypeParameters_Single(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("T")
	assert.Equal(t, []string{"T"}, result)
}

// ---------------------------------------------------------------------------
// 10. extractBaseTypeAndParams
// ---------------------------------------------------------------------------

func TestExtractBaseTypeAndParams_WithBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("MyType[T,U]")
	assert.Equal(t, "MyType", base)
	assert.Equal(t, "T,U", params)
}

func TestExtractBaseTypeAndParams_NoBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("SimpleType")
	assert.Equal(t, "SimpleType", base)
	assert.Equal(t, "", params)
}

func TestExtractBaseTypeAndParams_MismatchedBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("Broken[T")
	// ']' not found or before '[', returns original
	assert.Equal(t, "Broken[T", base)
	assert.Equal(t, "", params)
}

// ---------------------------------------------------------------------------
// 11. resolveCompositeType edge cases
// ---------------------------------------------------------------------------

func TestResolveCompositeType_EmptyBaseType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// X resolves to empty string
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("")
	})
	arg := makeArg(meta, metadata.KindCompositeLit, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveCompositeType(arg)
	assert.Equal(t, metadata.KindCompositeLit, result)
}

// ---------------------------------------------------------------------------
// 12. resolveIndexType additional cases
// ---------------------------------------------------------------------------

func TestResolveIndexType_NonSliceNonMap(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("arr")
		a.SetType("SomeCustomType")
	})
	arg := makeArg(meta, metadata.KindIndex, func(a *metadata.CallArgument) {
		a.X = &x
	})
	result := resolver.resolveIndexType(arg)
	assert.Equal(t, "SomeCustomType", result)
}

// ---------------------------------------------------------------------------
// 13. resolveSelectorType edge cases
// ---------------------------------------------------------------------------

func TestResolveSelectorType_NilX(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Method")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
	})
	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "Method", result)
}

func TestResolveSelectorType_FieldLookupInMetadata(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	meta.Packages["mypkg"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"file.go": {
				Types: map[string]*metadata.Type{
					"User": {
						Name: sp.Get("User"),
						Kind: sp.Get("struct"),
						Fields: []metadata.Field{
							{
								Name: sp.Get("Email"),
								Type: sp.Get("string"),
							},
						},
					},
				},
			},
		},
	}
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Email")
	})
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("user")
		a.SetType("User")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})
	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "string", result)
}

// ---------------------------------------------------------------------------
// 14. getString helper
// ---------------------------------------------------------------------------

func TestGetString_NilMeta(t *testing.T) {
	resolver := &TypeResolverImpl{meta: nil}
	assert.Equal(t, "", resolver.getString(0))
}

func TestGetString_NilStringPool(t *testing.T) {
	meta := &metadata.Metadata{StringPool: nil}
	resolver := &TypeResolverImpl{meta: meta}
	assert.Equal(t, "", resolver.getString(0))
}

// ---------------------------------------------------------------------------
// 15. resolveTypeConversionType with raw fallback
// ---------------------------------------------------------------------------

func TestResolveTypeConversionType_RawFallback(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Fun resolves to empty string, so fallback to raw
	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		// name and type both empty => resolveIdentType returns ""
		a.SetName("")
	})
	arg := makeArg(meta, metadata.KindTypeConversion, func(a *metadata.CallArgument) {
		a.Fun = &fun
		a.SetRaw("customRaw")
	})
	result := resolver.resolveTypeConversionType(arg)
	// Fun resolves to empty string "" which is empty, so we get raw
	assert.Equal(t, "customRaw", result)
}

// ---------------------------------------------------------------------------
// 16. ResolveGenericType with empty/whitespace params
// ---------------------------------------------------------------------------

func TestResolveGenericType_WhitespaceBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[ ]", map[string]string{})
	assert.Equal(t, "MyType", result)
}

func TestResolveGenericType_WithParams_EmptyParamStr(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// No brackets at all, with type params provided
	result := resolver.ResolveGenericType("Plain", map[string]string{"T": "int"})
	assert.Equal(t, "Plain", result)
}
