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

// ---------------------------------------------------------------------------
// resolveTypeThroughTracing
// ---------------------------------------------------------------------------

func TestResolveTypeThroughTracing_NotIdentOrFuncLit(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// KindLiteral is not KindIdent or KindFuncLit, should return ""
	arg := makeArg(meta, metadata.KindLiteral, func(a *metadata.CallArgument) {
		a.SetValue("42")
	})

	node := &TrackerNode{
		CallGraphEdge: &metadata.CallGraphEdge{
			Caller: metadata.Call{Meta: meta},
			Callee: metadata.Call{Meta: meta},
		},
	}

	result := resolver.resolveTypeThroughTracing(arg, node)
	assert.Equal(t, "", result)
}

func TestResolveTypeThroughTracing_KindIdent_NoOrigin(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("unknownVar")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeThroughTracing(arg, node)
	// No origin found in metadata => returns ""
	assert.Equal(t, "", result)
}

func TestResolveTypeThroughTracing_KindFuncLit(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindFuncLit, func(a *metadata.CallArgument) {
		a.SetName("funcLitVar")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeThroughTracing(arg, node)
	assert.Equal(t, "", result)
}

func TestResolveTypeThroughTracing_WithOriginType(_ *testing.T) {
	meta, sp := newTypeResolverTestMeta()

	// Set up metadata so TraceVariableOrigin can find the origin
	meta.Packages = map[string]*metadata.Package{
		"main": {
			Files: map[string]*metadata.File{
				"test.go": {
					Variables: map[string]*metadata.Variable{
						"x": {
							Name: sp.Get("x"),
							Type: sp.Get("int"),
						},
					},
					Functions: map[string]*metadata.Function{
						"caller": {
							Name: sp.Get("caller"),
						},
					},
				},
			},
		},
	}

	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("x")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeThroughTracing(arg, node)
	// TraceVariableOrigin should find the variable and return its type
	// If it finds an origin type, it uses resolveTypeFromArgument on it
	// The exact result depends on metadata.TraceVariableOrigin behavior
	_ = result // Just verify no panic
}

// ---------------------------------------------------------------------------
// findConcreteTypeByName
// ---------------------------------------------------------------------------

func TestFindConcreteTypeByName_ExactMatch(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int", "U": "string"}
	result := resolver.findConcreteTypeByName("T", typeParams)
	assert.Equal(t, "int", result)
}

func TestFindConcreteTypeByName_NoMatchDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.findConcreteTypeByName("V", typeParams)
	assert.Equal(t, "", result)
}

func TestFindConcreteTypeByName_ExtractedName(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// "List[V]" should extract to "V"
	typeParams := map[string]string{"V": "float64"}
	result := resolver.findConcreteTypeByName("List[V]", typeParams)
	assert.Equal(t, "float64", result)
}

func TestFindConcreteTypeByName_SingleLetterParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"K": "string"}
	// K is a single letter, extractParameterName returns it as is
	result := resolver.findConcreteTypeByName("K", typeParams)
	assert.Equal(t, "string", result)
}

func TestFindConcreteTypeByName_ComplexParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"MyType": "concrete"}
	// A multi-char name that isn't a single letter => extractParameterName returns it as is
	result := resolver.findConcreteTypeByName("MyType", typeParams)
	assert.Equal(t, "concrete", result)
}

// ---------------------------------------------------------------------------
// ResolveGenericType
// ---------------------------------------------------------------------------

func TestResolveGenericType_EmptyTypeParams(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// No type params, no brackets => return as is
	result := resolver.ResolveGenericType("MyType", nil)
	assert.Equal(t, "MyType", result)
}

func TestResolveGenericType_EmptyTypeParams_WithBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Empty type params, but has brackets
	result := resolver.ResolveGenericType("MyType[T]", nil)
	// paramStr is "T" which is not empty or "[]", so should return as is
	assert.Equal(t, "MyType[T]", result)
}

func TestResolveGenericType_EmptyTypeParams_EmptyBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[]", nil)
	assert.Equal(t, "MyType", result)
}

func TestResolveGenericType_EmptyTypeParams_WhitespaceBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ResolveGenericType("MyType[  ]", nil)
	assert.Equal(t, "MyType", result)
}

func TestResolveGenericType_WithTypeParams_Single(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("List[T]", typeParams)
	assert.Equal(t, "List[int]", result)
}

func TestResolveGenericType_WithTypeParams_Multiple(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"K": "string", "V": "int"}
	result := resolver.ResolveGenericType("Map[K,V]", typeParams)
	assert.Equal(t, "Map[string,int]", result)
}

func TestResolveGenericType_WithTypeParams_NoBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("SimpleType", typeParams)
	// No brackets => return as is
	assert.Equal(t, "SimpleType", result)
}

func TestResolveGenericType_WithTypeParams_EmptyParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("List[ ]", typeParams)
	// Whitespace-only parameter => return base type
	assert.Equal(t, "List", result)
}

func TestResolveGenericType_WithTypeParams_EmptyBracketsParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("List[]", typeParams)
	assert.Equal(t, "List", result)
}

func TestResolveGenericType_NestedGenericDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("Wrapper[List[T]]", typeParams)
	assert.Equal(t, "Wrapper[List[int]]", result)
}

func TestResolveGenericType_UnknownParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	result := resolver.ResolveGenericType("Container[Unknown]", typeParams)
	// "Unknown" doesn't match any key, so kept as is
	assert.Equal(t, "Container[Unknown]", result)
}

func TestResolveGenericType_EmptyParamString(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	typeParams := map[string]string{"T": "int"}
	// Empty string between brackets
	result := resolver.ResolveGenericType("Type[]", typeParams)
	assert.Equal(t, "Type", result)
}

// ---------------------------------------------------------------------------
// resolveTypeParameter
// ---------------------------------------------------------------------------

func TestResolveTypeParameter_MatchInTypeParamMap(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("T")
	})

	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
		Caller:       metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		TypeParamMap: map[string]string{"T": "string"},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeParameter(arg, node)
	assert.Equal(t, "string", result)
}

func TestResolveTypeParameter_MatchInParamArgMap(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("param1")
	})

	mappedArg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("float64")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
		ParamArgMap: map[string]metadata.CallArgument{
			"param1": mappedArg,
		},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeParameter(arg, node)
	assert.Equal(t, "float64", result)
}

func TestResolveTypeParameter_NoMatch(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("unknown")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.resolveTypeParameter(arg, node)
	assert.Equal(t, "", result)
}

func TestResolveTypeParameter_NilEdge_WithCallArgTypeParams(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("T")
	})

	// Create a CallArgument that has TypeParamMap set so TypeParams() collects it
	callArg := metadata.NewCallArgument(meta)
	callArg.SetKind(metadata.KindIdent)
	callArg.TypeParamMap = map[string]string{"T": "int"}

	// Node with a CallGraphEdge (needed to pass nil-edge check) and a CallArgument with TypeParamMap
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge
	node.CallArgument = callArg

	result := resolver.resolveTypeParameter(arg, node)
	assert.Equal(t, "int", result)
}

// ---------------------------------------------------------------------------
// generateParameterName
// ---------------------------------------------------------------------------

func TestGenerateParameterName_First26(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	assert.Equal(t, "T", resolver.generateParameterName(0))
	assert.Equal(t, "U", resolver.generateParameterName(1))
	assert.Equal(t, "V", resolver.generateParameterName(2))
	assert.Equal(t, "W", resolver.generateParameterName(3))
}

func TestGenerateParameterName_Beyond26(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Index 26 => base=0, number=1 => "T1"
	assert.Equal(t, "T1", resolver.generateParameterName(26))
	// Index 27 => base=1, number=1 => "U1"
	assert.Equal(t, "U1", resolver.generateParameterName(27))
	// Index 52 => base=0, number=2 => "T2"
	assert.Equal(t, "T2", resolver.generateParameterName(52))
}

func TestGenerateParameterName_Boundary(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Index 25 is last single letter
	result := resolver.generateParameterName(25)
	assert.Len(t, result, 1)
}

// ---------------------------------------------------------------------------
// parseTypeParameterList
// ---------------------------------------------------------------------------

func TestParseTypeParameterList_Single(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.parseTypeParameterList("int")
	assert.Equal(t, "int", result["T"])
}

func TestParseTypeParameterList_Multiple(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.parseTypeParameterList("int,string")
	assert.Equal(t, "int", result["T"])
	assert.Equal(t, "string", result["U"])
}

func TestParseTypeParameterList_Empty(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.parseTypeParameterList("")
	assert.Empty(t, result)
}

func TestParseTypeParameterList_NestedBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.parseTypeParameterList("List[int],string")
	assert.Equal(t, "List[int]", result["T"])
	assert.Equal(t, "string", result["U"])
}

func TestParseTypeParameterList_WhitespaceParam(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.parseTypeParameterList("  int  , string  ")
	assert.Equal(t, "int", result["T"])
	assert.Equal(t, "string", result["U"])
}

// ---------------------------------------------------------------------------
// resolveCallType
// ---------------------------------------------------------------------------

func TestResolveCallType_NilFun(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindCall)
	result := resolver.resolveCallType(arg)
	assert.Equal(t, "func()", result)
}

func TestResolveCallType_FuncWithReturn(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("func(int) string")
	})
	arg := makeArg(meta, metadata.KindCall, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveCallType(arg)
	assert.Equal(t, "string", result)
}

func TestResolveCallType_FuncNoReturn(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("func()")
	})
	arg := makeArg(meta, metadata.KindCall, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveCallType(arg)
	// Parts after ")" is empty, so falls through to funcType
	assert.Equal(t, "func()", result)
}

func TestResolveCallType_NonFuncType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("MyType")
		a.SetName("factory")
	})
	arg := makeArg(meta, metadata.KindCall, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveCallType(arg)
	assert.Equal(t, "MyType", result)
}

func TestResolveCallType_FuncMultipleReturns(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	fun := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("func(x int) (string, error)")
	})
	arg := makeArg(meta, metadata.KindCall, func(a *metadata.CallArgument) {
		a.Fun = &fun
	})
	result := resolver.resolveCallType(arg)
	// splits by ")" takes parts[1] which is " (string, error" (missing trailing ")")
	assert.Equal(t, "(string, error", result)
}

// ---------------------------------------------------------------------------
// ExtractTypeParameters
// ---------------------------------------------------------------------------

func TestExtractTypeParameters_NoBracketsDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("SimpleType")
	assert.Empty(t, result)
}

func TestExtractTypeParameters_WithParams(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("Map[string,int]")
	assert.Equal(t, "string", result["T"])
	assert.Equal(t, "int", result["U"])
}

func TestExtractTypeParameters_EmptyBracketsDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("Type[]")
	assert.Empty(t, result)
}

func TestExtractTypeParameters_WhitespaceInBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("Type[  ]")
	assert.Empty(t, result)
}

func TestExtractTypeParameters_InvalidBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// [ but no ] or ] before [
	result := resolver.ExtractTypeParameters("Type[")
	assert.Empty(t, result)
}

func TestExtractTypeParameters_SingleParamDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("List[int]")
	assert.Equal(t, "int", result["T"])
}

func TestExtractTypeParameters_NestedGeneric(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.ExtractTypeParameters("Container[List[int],string]")
	assert.Equal(t, "List[int]", result["T"])
	assert.Equal(t, "string", result["U"])
}

// ---------------------------------------------------------------------------
// resolveSelectorType
// ---------------------------------------------------------------------------

func TestResolveSelectorType_NilXDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Field")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		// X is nil
	})

	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "Field", result)
}

func TestResolveSelectorType_EmptyBaseType(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Field")
	})
	x := makeArg(meta, metadata.KindIdent, func(_ *metadata.CallArgument) {
		// No name, no type => empty base
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})

	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "Field", result)
}

func TestResolveSelectorType_WithFieldLookup(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()

	// Add type with fields to metadata
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"types.go": {
					Types: map[string]*metadata.Type{
						"MyStruct": {
							Name: sp.Get("MyStruct"),
							Kind: sp.Get("struct"),
							Fields: []metadata.Field{
								{Name: sp.Get("Size"), Type: sp.Get("int")},
								{Name: sp.Get("Name"), Type: sp.Get("string")},
							},
						},
					},
				},
			},
		},
	}

	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Name")
	})
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("s")
		a.SetType("MyStruct")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})

	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "string", result)
}

func TestResolveSelectorType_WithPkgPrefix(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()

	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"types.go": {
					Types: map[string]*metadata.Type{
						"mypkg.Config": {
							Name: sp.Get("mypkg.Config"),
							Kind: sp.Get("struct"),
							Fields: []metadata.Field{
								{Name: sp.Get("Port"), Type: sp.Get("int")},
							},
						},
					},
				},
			},
		},
	}

	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Port")
	})
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("cfg")
		a.SetType("Config")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})

	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "int", result)
}

func TestResolveSelectorType_FallbackConcatenation(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	sel := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("Field")
	})
	x := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("obj")
		a.SetType("UnknownType")
	})
	arg := makeArg(meta, metadata.KindSelector, func(a *metadata.CallArgument) {
		a.Sel = &sel
		a.X = &x
	})

	result := resolver.resolveSelectorType(arg)
	assert.Equal(t, "UnknownType.Field", result)
}

// ---------------------------------------------------------------------------
// ResolveType — integration
// ---------------------------------------------------------------------------

func TestResolveType_NilContextDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("int")
	})
	result := resolver.ResolveType(arg, nil)
	assert.Equal(t, "int", result)
}

func TestResolveType_NilEdge(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetType("string")
	})
	node := &TrackerNode{} // no edge
	result := resolver.ResolveType(arg, node)
	assert.Equal(t, "string", result)
}

func TestResolveType_FallbackToArgumentResolution(t *testing.T) {
	meta, sp := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	arg := makeArg(meta, metadata.KindIdent, func(a *metadata.CallArgument) {
		a.SetName("val")
		a.SetType("MyType")
	})

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := resolver.ResolveType(arg, node)
	assert.Equal(t, "MyType", result)
}

// ---------------------------------------------------------------------------
// splitTypeParameters
// ---------------------------------------------------------------------------

func TestSplitTypeParameters_SimpleDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("int,string,bool")
	assert.Equal(t, []string{"int", "string", "bool"}, result)
}

func TestSplitTypeParameters_NestedDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("List[int],Map[string,bool]")
	assert.Equal(t, []string{"List[int]", "Map[string,bool]"}, result)
}

func TestSplitTypeParameters_SingleDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("int")
	assert.Equal(t, []string{"int"}, result)
}

func TestSplitTypeParameters_Empty(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.splitTypeParameters("")
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// extractParameterName
// ---------------------------------------------------------------------------

func TestExtractParameterName_SingleLetter(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	assert.Equal(t, "T", resolver.extractParameterName("T"))
	assert.Equal(t, "K", resolver.extractParameterName("K"))
}

func TestExtractParameterName_NestedBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.extractParameterName("List[V]")
	assert.Equal(t, "V", result)
}

func TestExtractParameterName_DoubleNested(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.extractParameterName("Outer[Inner[T]]")
	assert.Equal(t, "T", result)
}

func TestExtractParameterName_MultiWordWithSingleLetter(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.extractParameterName("T constraint")
	assert.Equal(t, "T", result)
}

func TestExtractParameterName_ComplexNonSingleLetter(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	result := resolver.extractParameterName("SomeComplexType")
	assert.Equal(t, "SomeComplexType", result)
}

// ---------------------------------------------------------------------------
// extractBaseTypeAndParams
// ---------------------------------------------------------------------------

func TestExtractBaseTypeAndParams_NoBracketsDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("SimpleType")
	assert.Equal(t, "SimpleType", base)
	assert.Equal(t, "", params)
}

func TestExtractBaseTypeAndParams_WithBracketsDeep(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("List[int]")
	assert.Equal(t, "List", base)
	assert.Equal(t, "int", params)
}

func TestExtractBaseTypeAndParams_InvalidBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	// Only [ with no ]
	base, params := resolver.extractBaseTypeAndParams("Type[")
	assert.Equal(t, "Type[", base)
	assert.Equal(t, "", params)
}

func TestExtractBaseTypeAndParams_NestedBrackets(t *testing.T) {
	meta, _ := newTypeResolverTestMeta()
	resolver := newTestResolver(meta)

	base, params := resolver.extractBaseTypeAndParams("Outer[Inner[T]]")
	assert.Equal(t, "Outer", base)
	assert.Equal(t, "Inner[T]", params)
}
