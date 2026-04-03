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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// --- helpers ---

// pmcTestMeta creates a minimal metadata with a string pool.
func pmcTestMeta() *metadata.Metadata {
	return &metadata.Metadata{
		StringPool:      metadata.NewStringPool(),
		Packages:        map[string]*metadata.Package{},
		CallGraph:       []metadata.CallGraphEdge{},
		ParentFunctions: map[string][]*metadata.CallGraphEdge{},
	}
}

// pmcTestCfg returns a minimal APISpecConfig with sensible defaults.
func pmcTestCfg() *APISpecConfig {
	return &APISpecConfig{
		Defaults: Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "application/json",
			ResponseStatus:      200,
		},
	}
}

// buildCallGraphEdge creates a CallGraphEdge fully wired to the given metadata.
func buildCallGraphEdge(meta *metadata.Metadata, callerName, callerPkg, calleeName, calleePkg string, args []*metadata.CallArgument) *metadata.CallGraphEdge {
	sp := meta.StringPool
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: sp.Get(callerName),
			Pkg:  sp.Get(callerPkg),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get(calleeName),
			Pkg:  sp.Get(calleePkg),
		},
		Args: args,
	}
	edge.Caller.Edge = edge
	edge.Callee.Edge = edge
	return edge
}

// buildLiteralArg creates a KindLiteral argument.
func buildLiteralArg(meta *metadata.Metadata, value string) *metadata.CallArgument {
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue(value)
	return arg
}

// buildIdentArg creates a KindIdent argument.
func buildIdentArg(meta *metadata.Metadata, name, pkg string) *metadata.CallArgument {
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName(name)
	if pkg != "" {
		arg.SetPkg(pkg)
	}
	return arg
}

// buildTrackerNode builds a TrackerNode for testing.
func buildTrackerNode(edge *metadata.CallGraphEdge) *TrackerNode {
	return &TrackerNode{
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}
}

// newRoutePatternMatcher is a convenience to build a RoutePatternMatcherImpl wired up to metadata.
func newRoutePatternMatcher(meta *metadata.Metadata, pattern RoutePattern) *RoutePatternMatcherImpl {
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            pattern,
	}
}

// newRequestPatternMatcher builds a RequestPatternMatcherImpl wired up to metadata.
func newRequestPatternMatcher(meta *metadata.Metadata, pattern RequestBodyPattern) *RequestPatternMatcherImpl {
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            pattern,
	}
}

// ========================================================================
// 1. extractRouteDetails
// ========================================================================

func TestExtractRouteDetails_MethodFromCall(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "main", "main", "GetUsers", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodFromCall: true,
		MethodExtraction: &MethodExtractionConfig{
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
				{Patterns: []string{"post"}, Method: "POST", Priority: 10},
			},
			UsePrefix:   true,
			UseContains: true,
		},
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "GET", routeInfo.Method)
}

func TestExtractRouteDetails_MethodFromHandler(t *testing.T) {
	meta := pmcTestMeta()

	handlerArg := buildIdentArg(meta, "createUser", "myapp")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", []*metadata.CallArgument{
		buildLiteralArg(meta, "/users"),
		handlerArg,
	})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodFromHandler: true,
		HandlerFromArg:    true,
		HandlerArgIndex:   1,
		MethodExtraction: &MethodExtractionConfig{
			MethodMappings: []MethodMapping{
				{Patterns: []string{"create"}, Method: "POST", Priority: 10},
			},
			UsePrefix:   true,
			UseContains: true,
		},
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "POST", routeInfo.Method)
}

func TestExtractRouteDetails_PathFromArg(t *testing.T) {
	meta := pmcTestMeta()

	pathArg := buildLiteralArg(meta, "/users")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", []*metadata.CallArgument{pathArg})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		PathFromArg:  true,
		PathArgIndex: 0,
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "/users", routeInfo.Path)
}

func TestExtractRouteDetails_PathFromArgFallsBackToSlash(t *testing.T) {
	meta := pmcTestMeta()

	// Create an arg that resolves to empty string (no value, no name)
	emptyArg := metadata.NewCallArgument(meta)
	emptyArg.SetKind(metadata.KindLiteral)
	// Value is -1 (default) since we don't call SetValue

	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", []*metadata.CallArgument{emptyArg})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		PathFromArg:  true,
		PathArgIndex: 0,
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "/", routeInfo.Path)
}

func TestExtractRouteDetails_HandlerFromArg(t *testing.T) {
	meta := pmcTestMeta()

	handlerArg := buildIdentArg(meta, "myHandler", "myapp")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", []*metadata.CallArgument{
		buildLiteralArg(meta, "/test"),
		handlerArg,
	})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		HandlerFromArg:  true,
		HandlerArgIndex: 1,
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Contains(t, routeInfo.Handler, "myHandler")
	assert.Contains(t, routeInfo.Function, "myHandler")
	assert.Equal(t, "myapp", routeInfo.Package)
}

func TestExtractRouteDetails_MethodArgIndex(t *testing.T) {
	meta := pmcTestMeta()

	methodArg := buildLiteralArg(meta, "POST")
	pathArg := buildLiteralArg(meta, "/users")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http",
		[]*metadata.CallArgument{methodArg, pathArg})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodArgIndex: 0,
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "POST", routeInfo.Method)
}

func TestExtractRouteDetails_MethodArgIndex_InvalidMethodFallsThrough(t *testing.T) {
	meta := pmcTestMeta()

	// An arg with a value that is not a valid HTTP method, but GetArgumentInfo returns valid one
	methodArg := buildLiteralArg(meta, "not_a_method")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http",
		[]*metadata.CallArgument{methodArg})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodArgIndex: 0,
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: true,
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
			},
			UsePrefix:     true,
			UseContains:   true,
			DefaultMethod: "GET",
		},
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	// Since the value is not valid, it falls through to inferMethodFromContext
	assert.NotEmpty(t, routeInfo.Method)
}

// ========================================================================
// 2. inferMethodFromContext
// ========================================================================

func TestInferMethodFromContext_NoInferEnabled(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: nil, // no config
	})

	result := matcher.inferMethodFromContext(nil, edge)
	assert.Equal(t, "", result)
}

func TestInferMethodFromContext_InferDisabled(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: false,
		},
	})

	result := matcher.inferMethodFromContext(nil, edge)
	assert.Equal(t, "", result)
}

func TestInferMethodFromContext_ParentWithMethodsCall(t *testing.T) {
	meta := pmcTestMeta()

	// Build parent edge
	parentEdge := buildCallGraphEdge(meta, "main", "main", "Router", "mux", nil)
	parentNode := buildTrackerNode(parentEdge)

	// Build child with "Methods" call that contains "DELETE"
	methodsArg := buildLiteralArg(meta, "DELETE")
	methodsEdge := buildCallGraphEdge(meta, "main", "main", "Methods", "mux",
		[]*metadata.CallArgument{methodsArg})
	methodsChild := buildTrackerNode(methodsEdge)
	parentNode.AddChild(methodsChild)

	// Build the current node (the one we're inferring for)
	currentEdge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "mux",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/resource")})
	currentNode := buildTrackerNode(currentEdge)
	parentNode.AddChild(currentNode)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: true,
			MethodMappings:   []MethodMapping{},
		},
	})

	result := matcher.inferMethodFromContext(currentNode, currentEdge)
	assert.Equal(t, "DELETE", result)
}

func TestInferMethodFromContext_NoParent_FallsBackToFunctionName(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "getUsers", "main", "HandleFunc", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: true,
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
			},
			UsePrefix:     true,
			UseContains:   true,
			DefaultMethod: "POST",
		},
	})

	// node has no parent
	result := matcher.inferMethodFromContext(node, edge)
	assert.Equal(t, "GET", result)
}

func TestInferMethodFromContext_FallbackToGET(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "doSomething", "main", "HandleFunc", "net/http",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/test")})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: true,
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
			},
			UsePrefix:     true,
			UseContains:   false, // don't use contains
			DefaultMethod: "POST",
		},
	})

	result := matcher.inferMethodFromContext(node, edge)
	assert.Equal(t, "GET", result)
}

func TestInferMethodFromContext_HandlerArgInference(t *testing.T) {
	meta := pmcTestMeta()

	// Edge with 2 args where handler arg (index 1) has "deleteResource" name
	handlerArg := buildIdentArg(meta, "deleteResource", "main")
	edge := buildCallGraphEdge(meta, "setup", "main", "Handle", "net/http",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/res"), handlerArg})
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodExtraction: &MethodExtractionConfig{
			InferFromContext: true,
			MethodMappings: []MethodMapping{
				{Patterns: []string{"delete"}, Method: "DELETE", Priority: 10},
			},
			UsePrefix:     true,
			UseContains:   true,
			DefaultMethod: "POST",
		},
	})

	result := matcher.inferMethodFromContext(node, edge)
	assert.Equal(t, "DELETE", result)
}

// ========================================================================
// 3. MatchNode for RoutePatternMatcher
// ========================================================================

func TestRouteMatchNode_NilNode(t *testing.T) {
	meta := pmcTestMeta()
	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{CallRegex: ".*"}})
	assert.False(t, matcher.MatchNode(nil))
}

func TestRouteMatchNode_NilEdge(t *testing.T) {
	meta := pmcTestMeta()
	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{CallRegex: ".*"}})
	node := &TrackerNode{} // no edge
	assert.False(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_CallRegexMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{CallRegex: `^HandleFunc$`}})
	assert.True(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_CallRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{CallRegex: `^GET$`}})
	assert.False(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_RecvTypeRegexMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "github.com/go-chi/chi", nil)
	edge.Callee.RecvType = sp.Get("Mux")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{RecvTypeRegex: `^github\.com/go-chi/chi\.Mux$`}})
	assert.True(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_RecvTypeRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "github.com/go-chi/chi", nil)
	edge.Callee.RecvType = sp.Get("Router")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{RecvTypeRegex: `^github\.com/go-chi/chi\.Mux$`}})
	assert.False(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_RecvTypeExactMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "github.com/go-chi/chi", nil)
	edge.Callee.RecvType = sp.Get("Mux")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{RecvType: "github.com/go-chi/chi.Mux"}})
	assert.True(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_RecvTypeExactNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "github.com/go-chi/chi", nil)
	edge.Callee.RecvType = sp.Get("Router")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{RecvType: "github.com/go-chi/chi.Mux"}})
	assert.False(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_FunctionNameRegex(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "setupRoutes", "main", "HandleFunc", "net/http", nil)
	node := buildTrackerNode(edge)

	// FunctionNameRegex checks the caller (Caller.Name)
	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{FunctionNameRegex: `^setup.*$`}})
	assert.True(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_FunctionNameRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "initRoutes", "main", "HandleFunc", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{FunctionNameRegex: `^setup.*$`}})
	assert.False(t, matcher.MatchNode(node))
}

func TestRouteMatchNode_InvalidRecvTypeRegex(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "chi", nil)
	edge.Callee.RecvType = sp.Get("Mux")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{BasePattern: BasePattern{RecvTypeRegex: `[invalid`}})
	assert.False(t, matcher.MatchNode(node))
}

// ========================================================================
// 4. ExtractRequest
// ========================================================================

func TestExtractRequest_TypeFromArg_Normal(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	typeArg := buildIdentArg(meta, "UserRequest", "myapp")
	typeArg.SetType("myapp.UserRequest")
	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin",
		[]*metadata.CallArgument{typeArg})
	// Set Position so edge works well
	edge.Position = sp.Get("test.go:10")
	node := buildTrackerNode(edge)

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.BodyType)
}

func TestExtractRequest_LiteralType(t *testing.T) {
	meta := pmcTestMeta()

	litArg := buildLiteralArg(meta, "42")
	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin",
		[]*metadata.CallArgument{litArg})
	node := buildTrackerNode(edge)

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	require.NotNil(t, result)
	assert.Equal(t, "int", result.BodyType)
}

func TestExtractRequest_Deref(t *testing.T) {
	meta := pmcTestMeta()

	typeArg := buildIdentArg(meta, "req", "myapp")
	typeArg.SetType("*myapp.UserRequest")
	edge := buildCallGraphEdge(meta, "handler", "main", "Decode", "json",
		[]*metadata.CallArgument{typeArg})
	node := buildTrackerNode(edge)

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
		Deref:        true,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	require.NotNil(t, result)
	// The Deref flag strips the leading "*"
	assert.NotContains(t, result.BodyType, "*")
}

func TestExtractRequest_NoBodyType_ReturnsNil(t *testing.T) {
	meta := pmcTestMeta()

	// arg that resolves to empty type
	emptyArg := metadata.NewCallArgument(meta)
	emptyArg.SetKind(metadata.KindIdent)
	// Name and Type are -1, so GetArgumentInfo will return ""

	edge := buildCallGraphEdge(meta, "handler", "main", "Decode", "json",
		[]*metadata.CallArgument{emptyArg})
	node := buildTrackerNode(edge)

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	assert.Nil(t, result)
}

func TestExtractRequest_GenericTypeResolution(t *testing.T) {
	meta := pmcTestMeta()

	typeArg := buildIdentArg(meta, "req", "myapp")
	typeArg.IsGenericType = true
	typeArg.SetGenericTypeName("TRequest")
	// Don't set resolved type so it goes through the generic path

	edge := buildCallGraphEdge(meta, "handler", "main", "Decode", "json",
		[]*metadata.CallArgument{typeArg})
	node := buildTrackerNode(edge)
	// Set up type param map on the node's edge
	edge.TypeParamMap = map[string]string{
		"TRequest": "myapp.CreateUserRequest",
	}

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	require.NotNil(t, result)
	assert.Contains(t, result.BodyType, "CreateUserRequest")
}

func TestExtractRequest_ResolvedType(t *testing.T) {
	meta := pmcTestMeta()

	typeArg := buildIdentArg(meta, "req", "myapp")
	typeArg.SetResolvedType("myapp.ResolvedRequest")

	edge := buildCallGraphEdge(meta, "handler", "main", "Decode", "json",
		[]*metadata.CallArgument{typeArg})
	node := buildTrackerNode(edge)

	matcher := newRequestPatternMatcher(meta, RequestBodyPattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	})

	route := &RouteInfo{
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	result := matcher.ExtractRequest(node, route)
	require.NotNil(t, result)
	assert.Contains(t, result.BodyType, "ResolvedRequest")
}

// ========================================================================
// 5. ExtractRoute
// ========================================================================

func TestExtractRoute_BasicExtraction(t *testing.T) {
	meta := pmcTestMeta()

	pathArg := buildLiteralArg(meta, "/api/users")
	handlerArg := buildIdentArg(meta, "listUsers", "myapp")

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "chi",
		[]*metadata.CallArgument{pathArg, handlerArg})
	edge.Position = meta.StringPool.Get("routes.go:15")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodFromCall:  true,
		PathFromArg:     true,
		PathArgIndex:    0,
		HandlerFromArg:  true,
		HandlerArgIndex: 1,
		MethodExtraction: &MethodExtractionConfig{
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
			},
			UsePrefix: true,
		},
	})

	routeInfo := RouteInfo{}
	found := matcher.ExtractRoute(node, &routeInfo)
	assert.True(t, found)
	assert.Equal(t, "GET", routeInfo.Method)
	assert.Equal(t, "/api/users", routeInfo.Path)
	// Handler is set from the ident arg
	assert.NotEmpty(t, routeInfo.Handler)
}

func TestExtractRoute_EmptyRouteInfoGetsDefaults(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "main", "main", "PostItems", "chi",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/items")})
	edge.Position = meta.StringPool.Get("routes.go:1")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodFromCall: true,
		PathFromArg:    true,
		PathArgIndex:   0,
		MethodExtraction: &MethodExtractionConfig{
			MethodMappings: []MethodMapping{
				{Patterns: []string{"post"}, Method: "POST", Priority: 10},
			},
			UsePrefix:   true,
			UseContains: true,
		},
	})

	routeInfo := RouteInfo{}
	matcher.ExtractRoute(node, &routeInfo)
	// Defaults should be set: the Method from POST in function name, Response/UsedTypes maps
	assert.Equal(t, "POST", routeInfo.Method)
	assert.NotNil(t, routeInfo.Response)
	assert.NotNil(t, routeInfo.UsedTypes)
}

func TestExtractRoute_WithMetadata(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/test")})
	edge.Callee.Meta = meta
	edge.Position = meta.StringPool.Get("main.go:5")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		PathFromArg:  true,
		PathArgIndex: 0,
	})

	routeInfo := RouteInfo{}
	matcher.ExtractRoute(node, &routeInfo)
	assert.Equal(t, meta, routeInfo.Metadata)
}

// ========================================================================
// 6. matchPattern (BasePatternMatcher)
// ========================================================================

func TestBaseMatchPattern_Match(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	assert.True(t, base.matchPattern(`^HandleFunc$`, "HandleFunc"))
}

func TestBaseMatchPattern_NoMatch(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	assert.False(t, base.matchPattern(`^GET$`, "HandleFunc"))
}

func TestBaseMatchPattern_EmptyPattern(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	assert.False(t, base.matchPattern("", "anything"))
}

func TestBaseMatchPattern_InvalidRegex(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	assert.False(t, base.matchPattern("[invalid", "test"))
}

// ========================================================================
// 7. extractMethodFromFunctionNameWithConfig
// ========================================================================

func TestExtractMethodFromFunctionName_EmptyName(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	result := base.extractMethodFromFunctionNameWithConfig("", nil)
	assert.Equal(t, "", result)
}

func TestExtractMethodFromFunctionName_NilConfig_UsesDefault(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	result := base.extractMethodFromFunctionNameWithConfig("getUserProfile", nil)
	// Default config has UsePrefix and UseContains enabled, should detect "get" -> "GET"
	assert.Equal(t, "GET", result)
}

func TestExtractMethodFromFunctionName_CaseSensitive(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	config := &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"Get"}, Method: "GET", Priority: 10},
		},
		UsePrefix:     true,
		CaseSensitive: true,
		DefaultMethod: "POST",
	}

	// Exact case match with word boundary (underscore is not a letter)
	assert.Equal(t, "GET", base.extractMethodFromFunctionNameWithConfig("Get_users", config))
	// No match due to case sensitivity ("get" != "Get")
	assert.Equal(t, "POST", base.extractMethodFromFunctionNameWithConfig("get_users", config))
	// Exact function name matching the pattern exactly (no trailing chars)
	assert.Equal(t, "GET", base.extractMethodFromFunctionNameWithConfig("Get", config))
}

func TestExtractMethodFromFunctionName_PriorityOrdering(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	config := &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"del"}, Method: "DELETE_SHORT", Priority: 5},
			{Patterns: []string{"delete"}, Method: "DELETE_FULL", Priority: 10},
		},
		UseContains:   true,
		DefaultMethod: "GET",
	}

	// Higher priority "delete" mapping should be checked first due to sort
	result := base.extractMethodFromFunctionNameWithConfig("deleteUser", config)
	assert.Equal(t, "DELETE_FULL", result)
}

func TestExtractMethodFromFunctionName_ContainsMatch(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	config := &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"update"}, Method: "PUT", Priority: 10},
		},
		UseContains:   true,
		DefaultMethod: "POST",
	}

	result := base.extractMethodFromFunctionNameWithConfig("handleUserUpdate", config)
	assert.Equal(t, "PUT", result)
}

func TestExtractMethodFromFunctionName_DefaultFallback(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	config := &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"get"}, Method: "GET", Priority: 10},
		},
		UsePrefix:     true,
		DefaultMethod: "POST",
	}

	result := base.extractMethodFromFunctionNameWithConfig("doSomething", config)
	assert.Equal(t, "POST", result)
}

// ========================================================================
// 8. traceVariable
// ========================================================================

func TestTraceVariable_NoContextImpl(t *testing.T) {
	// When contextProvider is not a *ContextProviderImpl, traceVariable returns defaults
	cfg := pmcTestCfg()
	// Use a minimal mock that satisfies ContextProvider but is not ContextProviderImpl
	mockCP := &mockContextProvider{}
	base := NewBasePatternMatcher(cfg, mockCP, nil)

	originVar, originPkg, originType, originFunc := base.traceVariable("x", "main", "main")
	assert.Equal(t, "x", originVar)
	assert.Equal(t, "main", originPkg)
	assert.Nil(t, originType)
	assert.Equal(t, "", originFunc)
}

func TestTraceVariable_WithContextProviderImpl(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	base := NewBasePatternMatcher(cfg, cp, nil)

	// traceVariable with empty metadata should return the inputs unchanged
	originVar, originPkg, originType, _ := base.traceVariable("handler", "main", "main")
	assert.Equal(t, "handler", originVar)
	assert.Equal(t, "main", originPkg)
	assert.Nil(t, originType)
}

// mockContextProvider is a minimal mock for ContextProvider
type mockContextProvider struct{}

func (m *mockContextProvider) GetString(_ int) string { return "" }
func (m *mockContextProvider) GetCalleeInfo(_ TrackerNodeInterface) (string, string, string) {
	return "", "", ""
}
func (m *mockContextProvider) GetArgumentInfo(_ *metadata.CallArgument) string { return "" }

// ========================================================================
// 9. processArguments (tracker.go)
// ========================================================================

func TestProcessArguments_NilEdge(t *testing.T) {
	meta := pmcTestMeta()
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
		limits:                 limits,
	}
	visited := map[string]int{}
	assignIdx := assigmentIndexMap{}

	result := processArguments(tree, meta, nil, nil, visited, &assignIdx, limits)
	assert.Nil(t, result)
}

func TestProcessArguments_WithLiteralArgs(t *testing.T) {
	meta := pmcTestMeta()
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
		limits:                 limits,
	}

	litArg := buildLiteralArg(meta, "/users")
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http",
		[]*metadata.CallArgument{litArg})

	parentNode := &TrackerNode{
		key:           "main.main",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}

	visited := map[string]int{}
	assignIdx := assigmentIndexMap{}

	children := processArguments(tree, meta, parentNode, edge, visited, &assignIdx, limits)
	// The literal arg should be added as a child
	assert.NotNil(t, children)
}

func TestProcessArguments_MaxArgsLimit(t *testing.T) {
	meta := pmcTestMeta()
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 2, // very low limit
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
		limits:                 limits,
	}

	// Create multiple args
	args := []*metadata.CallArgument{
		buildLiteralArg(meta, "/a"),
		buildLiteralArg(meta, "/b"),
		buildLiteralArg(meta, "/c"),
		buildLiteralArg(meta, "/d"),
	}
	edge := buildCallGraphEdge(meta, "main", "main", "Handle", "net/http", args)
	parentNode := &TrackerNode{
		key:           "main.main",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}

	visited := map[string]int{}
	assignIdx := assigmentIndexMap{}

	children := processArguments(tree, meta, parentNode, edge, visited, &assignIdx, limits)
	// Should be limited by MaxArgsPerFunction
	assert.LessOrEqual(t, len(children), limits.MaxArgsPerFunction)
}

// ========================================================================
// 10. processChainRelationships (tracker.go)
// ========================================================================

func TestProcessChainRelationships_NoChains(_ *testing.T) {
	meta := pmcTestMeta()
	meta.BuildCallGraphMaps()

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		roots:                  []*TrackerNode{},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	// Should not panic
	tree.processChainRelationships()
}

func TestProcessChainRelationships_WithChainedCalls(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	// Create a chain: parentEdge -> childEdge (ChainParent set)
	parentEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("Group"), Pkg: sp.Get("chi")},
	}
	parentEdge.Caller.Edge = parentEdge
	parentEdge.Callee.Edge = parentEdge

	childEdge := metadata.CallGraphEdge{
		Caller:      metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee:      metadata.Call{Meta: meta, Name: sp.Get("Use"), Pkg: sp.Get("chi")},
		ChainParent: parentEdge,
	}
	childEdge.Caller.Edge = &childEdge
	childEdge.Callee.Edge = &childEdge

	meta.CallGraph = []metadata.CallGraphEdge{childEdge}
	meta.BuildCallGraphMaps()

	// Create nodes
	parentNode := &TrackerNode{
		key:           parentEdge.Callee.ID(),
		CallGraphEdge: parentEdge,
		Children:      []*TrackerNode{},
	}
	childNode := &TrackerNode{
		key:           childEdge.Callee.ID(),
		CallGraphEdge: &childEdge,
		Children:      []*TrackerNode{},
	}

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		roots:                  []*TrackerNode{parentNode},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{parentEdge.Callee.ID(): parentEdge},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap: map[string]*TrackerNode{
			parentEdge.Callee.ID(): parentNode,
			childEdge.Callee.ID():  childNode,
		},
		idCache: map[string]string{},
	}

	tree.processChainRelationships()
	// Parent should now have the child
	found := false
	for _, c := range parentNode.Children {
		if c == childNode {
			found = true
		}
	}
	assert.True(t, found, "childNode should be added as child of parentNode")
}

// ========================================================================
// 11. traverseTree (indirect)
// ========================================================================

func TestTraverseTree_EmptyNodes(t *testing.T) {
	mapObj := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	result := traverseTree(nil, mapObj, 1, nil)
	assert.False(t, result)
}

func TestTraverseTree_EmptyKeyNode(t *testing.T) {
	// Node with no key should be skipped
	node := &TrackerNode{Children: []*TrackerNode{}}
	mapObj := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	result := traverseTree([]*TrackerNode{node}, mapObj, 1, nil)
	assert.False(t, result)
}

func TestTraverseTree_MatchingAssignment(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)
	node := &TrackerNode{
		key:           "main.HandleFunc",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}

	assignNode := &TrackerNode{
		key:           "main.HandleFunc",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}

	mapObj := &assignmentNodes{
		assignmentIndex: assigmentIndexMap{
			{Name: "main.HandleFunc"}: assignNode,
		},
	}

	result := traverseTree([]*TrackerNode{node}, mapObj, 5, nil)
	assert.False(t, result) // no children to add
}

func TestTraverseTree_LimitExceeded(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func", "pkg", nil)

	node := &TrackerNode{
		key:           "pkg.Func",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}

	mapObj := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	// Already visited more times than limit
	nodeCount := map[string]int{"pkg.Func": 100}
	result := traverseTree([]*TrackerNode{node}, mapObj, 1, nodeCount)
	assert.False(t, result)
}

// ========================================================================
// 12. Assign methods
// ========================================================================

func TestAssignmentNodes_Assign(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func1", "pkg", nil)

	node1 := &TrackerNode{key: "node1", CallGraphEdge: edge, Children: []*TrackerNode{}}
	node2 := &TrackerNode{key: "node2", CallGraphEdge: edge, Children: []*TrackerNode{}}

	an := &assignmentNodes{
		assignmentIndex: assigmentIndexMap{
			{Name: "a"}: node1,
			{Name: "b"}: node2,
		},
	}

	var visited []*TrackerNode
	an.Assign(func(tn *TrackerNode) bool {
		visited = append(visited, tn)
		return false
	})

	assert.Len(t, visited, 2)
}

func TestVariableNodes_Assign(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func1", "pkg", nil)

	node1 := &TrackerNode{key: "v1", CallGraphEdge: edge, Children: []*TrackerNode{}}
	node2 := &TrackerNode{key: "v2", CallGraphEdge: edge, Children: []*TrackerNode{}}
	node3 := &TrackerNode{key: "v3", CallGraphEdge: edge, Children: []*TrackerNode{}}

	vn := &variableNodes{
		variables: map[paramKey][]*TrackerNode{
			{Name: "x", Pkg: "pkg", Container: "main"}: {node1, node2},
			{Name: "y", Pkg: "pkg", Container: "main"}: {node3},
		},
	}

	var visited []*TrackerNode
	vn.Assign(func(tn *TrackerNode) bool {
		visited = append(visited, tn)
		return false
	})

	// Should only get the first element of each slice
	assert.Len(t, visited, 2)
	// The first element of the "x" slice is node1
	assert.Contains(t, visited, node1)
	assert.Contains(t, visited, node3)
}

func TestVariableNodes_Assign_EmptySlice(t *testing.T) {
	vn := &variableNodes{
		variables: map[paramKey][]*TrackerNode{
			{Name: "empty"}: {}, // empty slice
		},
	}

	count := 0
	vn.Assign(func(_ *TrackerNode) bool {
		count++
		return false
	})
	// Empty slice should be skipped
	assert.Equal(t, 0, count)
}

// ========================================================================
// 13. NewTrackerTree
// ========================================================================

func TestNewTrackerTree_EmptyMetadata(t *testing.T) {
	meta := pmcTestMeta()
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}

	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	assert.Empty(t, tree.roots)
}

func TestNewTrackerTree_WithMainCaller(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	// Create a call graph with main as caller
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: sp.Get("main"),
			Pkg:  sp.Get("main"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get("HandleFunc"),
			Pkg:  sp.Get("net/http"),
		},
		Args: []*metadata.CallArgument{
			buildLiteralArg(meta, "/users"),
			buildIdentArg(meta, "handler", "main"),
		},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge

	meta.CallGraph = []metadata.CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}

	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	// Should find "main" as root
	assert.NotEmpty(t, tree.roots, "Expected at least one root node for main function")
}

func TestNewTrackerTree_NonMainCallerIsNotRoot(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	// Non-main caller should not become root
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: sp.Get("setup"),
			Pkg:  sp.Get("myapp"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get("HandleFunc"),
			Pkg:  sp.Get("net/http"),
		},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge

	meta.CallGraph = []metadata.CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}

	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	// "setup" is a root in the call graph, but NewTrackerTree only picks "main"
	assert.Empty(t, tree.roots)
}

func TestNewTrackerTree_WithChainParent(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	parentEdge := metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("Group"), Pkg: sp.Get("chi")},
	}
	parentEdge.Caller.Edge = &parentEdge
	parentEdge.Callee.Edge = &parentEdge

	childEdge := metadata.CallGraphEdge{
		Caller:      metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee:      metadata.Call{Meta: meta, Name: sp.Get("Get"), Pkg: sp.Get("chi")},
		ChainParent: &parentEdge,
		Args: []*metadata.CallArgument{
			buildLiteralArg(meta, "/users"),
		},
	}
	childEdge.Caller.Edge = &childEdge
	childEdge.Callee.Edge = &childEdge

	meta.CallGraph = []metadata.CallGraphEdge{parentEdge, childEdge}
	meta.BuildCallGraphMaps()

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}

	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	// chainParentMap should have been built
	assert.NotEmpty(t, tree.chainParentMap)
}

// ========================================================================
// Additional coverage: GetPriority for all matchers
// ========================================================================

func TestRoutePatternMatcher_GetPriority(t *testing.T) {
	meta := pmcTestMeta()

	tests := []struct {
		name     string
		pattern  RoutePattern
		expected int
	}{
		{"empty", RoutePattern{}, 0},
		{"callRegex only", RoutePattern{BasePattern: BasePattern{CallRegex: "test"}}, 10},
		{"functionNameRegex only", RoutePattern{BasePattern: BasePattern{FunctionNameRegex: "test"}}, 5},
		{"recvTypeRegex only", RoutePattern{BasePattern: BasePattern{RecvTypeRegex: "test"}}, 3},
		{"recvType only", RoutePattern{BasePattern: BasePattern{RecvType: "test"}}, 3},
		{"all", RoutePattern{BasePattern: BasePattern{CallRegex: "a", FunctionNameRegex: "b", RecvTypeRegex: "c"}}, 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newRoutePatternMatcher(meta, tt.pattern)
			assert.Equal(t, tt.expected, m.GetPriority())
		})
	}
}

func TestRoutePatternMatcher_GetPattern(t *testing.T) {
	meta := pmcTestMeta()
	pattern := RoutePattern{BasePattern: BasePattern{CallRegex: "test"}, PathFromArg: true}
	m := newRoutePatternMatcher(meta, pattern)
	result := m.GetPattern()
	rp, ok := result.(RoutePattern)
	require.True(t, ok)
	assert.Equal(t, "test", rp.CallRegex)
}

func TestRequestPatternMatcher_GetPriority(t *testing.T) {
	meta := pmcTestMeta()

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{CallRegex: "test",
		FunctionNameRegex: "handler",
		RecvTypeRegex:     "ctx"},
	})
	assert.Equal(t, 18, m.GetPriority())
}

func TestRequestPatternMatcher_GetPattern_Coverage(t *testing.T) {
	meta := pmcTestMeta()
	pattern := RequestBodyPattern{BasePattern: BasePattern{CallRegex: "BindJSON"}}
	m := newRequestPatternMatcher(meta, pattern)
	result := m.GetPattern()
	rp, ok := result.(RequestBodyPattern)
	require.True(t, ok)
	assert.Equal(t, "BindJSON", rp.CallRegex)
}

// ========================================================================
// Additional: RequestPatternMatcher MatchNode
// ========================================================================

func TestRequestMatchNode_NilNode(t *testing.T) {
	meta := pmcTestMeta()
	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{CallRegex: ".*"}})
	assert.False(t, m.MatchNode(nil))
}

func TestRequestMatchNode_NilEdge(t *testing.T) {
	meta := pmcTestMeta()
	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{CallRegex: ".*"}})
	node := &TrackerNode{}
	assert.False(t, m.MatchNode(node))
}

func TestRequestMatchNode_CallRegexMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin", nil)
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{CallRegex: `^BindJSON$`}})
	assert.True(t, m.MatchNode(node))
}

func TestRequestMatchNode_CallRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin", nil)
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{CallRegex: `^Decode$`}})
	assert.False(t, m.MatchNode(node))
}

func TestRequestMatchNode_FunctionNameRegex(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "handleCreate", "main", "BindJSON", "gin", nil)
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{FunctionNameRegex: `^handle.*$`}})
	assert.True(t, m.MatchNode(node))
}

func TestRequestMatchNode_FunctionNameRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "setup", "main", "BindJSON", "gin", nil)
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{FunctionNameRegex: `^handle.*$`}})
	assert.False(t, m.MatchNode(node))
}

func TestRequestMatchNode_RecvTypeRegex(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "github.com/gin-gonic/gin", nil)
	edge.Callee.RecvType = sp.Get("Context")
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{RecvTypeRegex: `^github\.com/gin-gonic/gin\.Context$`}})
	assert.True(t, m.MatchNode(node))
}

func TestRequestMatchNode_RecvTypeExactMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "github.com/gin-gonic/gin", nil)
	edge.Callee.RecvType = sp.Get("Context")
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{RecvType: "github.com/gin-gonic/gin.Context"}})
	assert.True(t, m.MatchNode(node))
}

func TestRequestMatchNode_RecvTypeExactNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin", nil)
	edge.Callee.RecvType = sp.Get("Handler")
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{RecvType: "gin.Context"}})
	assert.False(t, m.MatchNode(node))
}

func TestRequestMatchNode_InvalidRecvTypeRegex(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "handler", "main", "BindJSON", "gin", nil)
	edge.Callee.RecvType = sp.Get("Context")
	node := buildTrackerNode(edge)

	m := newRequestPatternMatcher(meta, RequestBodyPattern{BasePattern: BasePattern{RecvTypeRegex: `[invalid`}})
	assert.False(t, m.MatchNode(node))
}

// ========================================================================
// Additional: MountPatternMatcher coverage
// ========================================================================

func TestMountMatchNode_NilNode(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{CallRegex: ".*"}, IsMount: true},
	}
	assert.False(t, m.MatchNode(nil))
}

func TestMountMatchNode_NilEdge(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{CallRegex: ".*"}, IsMount: true},
	}
	node := &TrackerNode{}
	assert.False(t, m.MatchNode(node))
}

func TestMountMatchNode_CallRegexMatch(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	edge := buildCallGraphEdge(meta, "main", "main", "Mount", "chi", nil)
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{CallRegex: `^Mount$`}, IsMount: true},
	}
	assert.True(t, m.MatchNode(node))
}

func TestMountMatchNode_IsMountFalse(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	edge := buildCallGraphEdge(meta, "main", "main", "Mount", "chi", nil)
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{CallRegex: `^Mount$`}, IsMount: false},
	}
	// Even though CallRegex matches, IsMount is false so returns false
	assert.False(t, m.MatchNode(node))
}

func TestMountMatchNode_FunctionNameRegex(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	edge := buildCallGraphEdge(meta, "setupAPI", "main", "Mount", "chi", nil)
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{FunctionNameRegex: `^setup.*$`}, IsMount: true},
	}
	assert.True(t, m.MatchNode(node))
}

func TestMountMatchNode_FunctionNameRegexNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	edge := buildCallGraphEdge(meta, "initRoutes", "main", "Mount", "chi", nil)
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{FunctionNameRegex: `^setup.*$`}, IsMount: true},
	}
	assert.False(t, m.MatchNode(node))
}

func TestMountMatchNode_RecvTypeRegex(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Mount", "chi", nil)
	edge.Callee.RecvType = sp.Get("Mux")
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{RecvTypeRegex: `^chi\.Mux$`}, IsMount: true},
	}
	assert.True(t, m.MatchNode(node))
}

func TestMountMatchNode_RecvTypeExact(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Mount", "chi", nil)
	edge.Callee.RecvType = sp.Get("Mux")
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{RecvType: "chi.Mux"}, IsMount: true},
	}
	assert.True(t, m.MatchNode(node))
}

func TestMountMatchNode_RecvTypeExactNoMatch(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)
	sp := meta.StringPool

	edge := buildCallGraphEdge(meta, "main", "main", "Mount", "chi", nil)
	edge.Callee.RecvType = sp.Get("Router")
	node := buildTrackerNode(edge)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{RecvType: "chi.Mux"}, IsMount: true},
	}
	assert.False(t, m.MatchNode(node))
}

func TestMountPatternMatcher_GetPriority(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            MountPattern{BasePattern: BasePattern{CallRegex: "x", FunctionNameRegex: "y", RecvType: "z"}},
	}
	assert.Equal(t, 18, m.GetPriority())
}

func TestMountPatternMatcher_GetPattern_Coverage(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	cp := NewContextProvider(meta)

	pattern := MountPattern{BasePattern: BasePattern{CallRegex: "Mount"}, IsMount: true}
	m := &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, cp, nil),
		pattern:            pattern,
	}
	result := m.GetPattern()
	mp, ok := result.(MountPattern)
	require.True(t, ok)
	assert.Equal(t, "Mount", mp.CallRegex)
}

// ========================================================================
// Additional: isValidHTTPMethod
// ========================================================================

func TestIsValidHTTPMethod_Coverage(t *testing.T) {
	meta := pmcTestMeta()
	matcher := newRoutePatternMatcher(meta, RoutePattern{})

	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE", "CONNECT",
		"get", "post", "put", "delete", "patch", "options", "head", "trace", "connect"}

	for _, m := range validMethods {
		assert.True(t, matcher.isValidHTTPMethod(m), "Expected %q to be valid", m)
	}

	invalidMethods := []string{"INVALID", "FOO", "", "GETS", "POSTING"}
	for _, m := range invalidMethods {
		assert.False(t, matcher.isValidHTTPMethod(m), "Expected %q to be invalid", m)
	}
}

// ========================================================================
// Additional: isLetter
// ========================================================================

func TestIsLetter(t *testing.T) {
	meta := pmcTestMeta()
	cfg := pmcTestCfg()
	base := NewBasePatternMatcher(cfg, NewContextProvider(meta), nil)

	assert.True(t, base.isLetter('a'))
	assert.True(t, base.isLetter('Z'))
	assert.True(t, base.isLetter('m'))
	assert.False(t, base.isLetter('0'))
	assert.False(t, base.isLetter(' '))
	assert.False(t, base.isLetter('_'))
}

// ========================================================================
// Additional: getCachedPatternRegex double-check after write lock
// ========================================================================

func TestGetCachedPatternRegex_CachesResult(t *testing.T) {
	pattern := `^test\d+$`
	re1, err1 := getCachedPatternRegex(pattern)
	require.NoError(t, err1)
	require.NotNil(t, re1)

	// Second call should return cached result
	re2, err2 := getCachedPatternRegex(pattern)
	require.NoError(t, err2)
	assert.Equal(t, re1, re2, "Should return the same cached regex")
}

func TestGetCachedPatternRegex_InvalidPattern(t *testing.T) {
	_, err := getCachedPatternRegex("[invalid")
	assert.Error(t, err)
}

// ========================================================================
// Additional: TrackerNode methods
// ========================================================================

func TestTrackerNode_GetParent_Nil_PMC(t *testing.T) {
	node := &TrackerNode{}
	result := node.GetParent()
	assert.Nil(t, result)
}

func TestTrackerNode_GetParent_NonNil_PMC(t *testing.T) {
	parent := &TrackerNode{key: "parent"}
	child := &TrackerNode{key: "child", Parent: parent}
	result := child.GetParent()
	require.NotNil(t, result)
	assert.Equal(t, "parent", result.GetKey())
}

func TestTrackerNode_GetChildren(t *testing.T) {
	child1 := &TrackerNode{key: "c1"}
	child2 := &TrackerNode{key: "c2"}
	parent := &TrackerNode{key: "p", Children: []*TrackerNode{child1, child2}}

	children := parent.GetChildren()
	assert.Len(t, children, 2)
}

func TestTrackerNode_GetEdge(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func", "pkg", nil)
	node := &TrackerNode{CallGraphEdge: edge}
	assert.Equal(t, edge, node.GetEdge())
}

func TestTrackerNode_GetEdge_Nil_PMC(t *testing.T) {
	node := &TrackerNode{}
	assert.Nil(t, node.GetEdge())
}

func TestTrackerNode_GetArgument_PMC(t *testing.T) {
	meta := pmcTestMeta()
	arg := buildIdentArg(meta, "x", "pkg")
	node := &TrackerNode{CallArgument: arg}
	assert.Equal(t, arg, node.GetArgument())
}

func TestTrackerNode_GetArgument_Nil_PMC(t *testing.T) {
	node := &TrackerNode{}
	assert.Nil(t, node.GetArgument())
}

func TestTrackerNode_GetArgType(t *testing.T) {
	tests := []struct {
		argType  ArgumentType
		expected metadata.ArgumentType
	}{
		{ArgTypeDirectCallee, metadata.ArgTypeDirectCallee},
		{ArgTypeFunctionCall, metadata.ArgTypeFunctionCall},
		{ArgTypeVariable, metadata.ArgTypeVariable},
		{ArgTypeLiteral, metadata.ArgTypeLiteral},
		{ArgTypeSelector, metadata.ArgTypeSelector},
		{ArgTypeComplex, metadata.ArgTypeComplex},
		{ArgTypeUnary, metadata.ArgTypeUnary},
		{ArgTypeBinary, metadata.ArgTypeBinary},
		{ArgTypeIndex, metadata.ArgTypeIndex},
		{ArgTypeComposite, metadata.ArgTypeComposite},
		{ArgTypeTypeAssert, metadata.ArgTypeTypeAssert},
	}

	for _, tt := range tests {
		node := &TrackerNode{ArgType: tt.argType}
		assert.Equal(t, tt.expected, node.GetArgType())
	}
}

func TestTrackerNode_GetArgIndex_PMC(t *testing.T) {
	node := &TrackerNode{ArgIndex: 3}
	assert.Equal(t, 3, node.GetArgIndex())
}

func TestTrackerNode_GetArgContext_PMC(t *testing.T) {
	node := &TrackerNode{ArgContext: "main.HandleFunc"}
	assert.Equal(t, "main.HandleFunc", node.GetArgContext())
}

func TestTrackerNode_GetTypeParamMap(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func", "pkg", nil)
	edge.TypeParamMap = map[string]string{"T": "string"}
	node := &TrackerNode{CallGraphEdge: edge}
	tp := node.GetTypeParamMap()
	assert.Equal(t, "string", tp["T"])
}

func TestTrackerNode_GetRootAssignmentMap(t *testing.T) {
	node := &TrackerNode{
		RootAssignmentMap: map[string][]metadata.Assignment{
			"x": {{VariableName: 1}},
		},
	}
	ram := node.GetRootAssignmentMap()
	assert.Len(t, ram, 1)
	assert.Contains(t, ram, "x")
}

func TestTrackerNode_AddChild_PMC(t *testing.T) {
	parent := &TrackerNode{key: "p", Children: []*TrackerNode{}}
	child := &TrackerNode{key: "c", Children: []*TrackerNode{}}

	parent.AddChild(child)
	assert.Len(t, parent.Children, 1)
	assert.Equal(t, parent, child.Parent)
}

func TestTrackerNode_AddChild_DetachFromPrevious_PMC(t *testing.T) {
	oldParent := &TrackerNode{key: "old", Children: []*TrackerNode{}}
	newParent := &TrackerNode{key: "new", Children: []*TrackerNode{}}
	child := &TrackerNode{key: "c", Children: []*TrackerNode{}}

	oldParent.AddChild(child)
	assert.Len(t, oldParent.Children, 1)

	newParent.AddChild(child)
	assert.Len(t, newParent.Children, 1)
	assert.Empty(t, oldParent.Children)
	assert.Equal(t, newParent, child.Parent)
}

func TestTrackerNode_AddChildren(t *testing.T) {
	parent := &TrackerNode{key: "p", Children: []*TrackerNode{}}
	c1 := &TrackerNode{key: "c1", Children: []*TrackerNode{}}
	c2 := &TrackerNode{key: "c2", Children: []*TrackerNode{}}

	parent.AddChildren([]*TrackerNode{c1, c2})
	assert.Len(t, parent.Children, 2)
	assert.Equal(t, parent, c1.Parent)
	assert.Equal(t, parent, c2.Parent)
}

// ========================================================================
// Additional: classifyArgument
// ========================================================================

func TestClassifyArgument(t *testing.T) {
	meta := pmcTestMeta()

	tests := []struct {
		name     string
		kind     string
		typeStr  string
		expected ArgumentType
	}{
		{"call", metadata.KindCall, "", ArgTypeFunctionCall},
		{"func_lit", metadata.KindFuncLit, "", ArgTypeFunctionCall},
		{"ident", metadata.KindIdent, "", ArgTypeVariable},
		{"ident_func_type", metadata.KindIdent, "func()", ArgTypeFunctionCall},
		{"literal", metadata.KindLiteral, "", ArgTypeLiteral},
		{"selector", metadata.KindSelector, "", ArgTypeSelector},
		{"unary", metadata.KindUnary, "", ArgTypeUnary},
		{"binary", metadata.KindBinary, "", ArgTypeBinary},
		{"index", metadata.KindIndex, "", ArgTypeIndex},
		{"composite_lit", metadata.KindCompositeLit, "", ArgTypeComposite},
		{"type_assert", metadata.KindTypeAssert, "", ArgTypeTypeAssert},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := metadata.NewCallArgument(meta)
			arg.SetKind(tt.kind)
			if tt.typeStr != "" {
				arg.SetType(tt.typeStr)
			}
			result := classifyArgument(arg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ========================================================================
// Additional: ArgumentType.String()
// ========================================================================

func TestArgumentType_String_PMC(t *testing.T) {
	tests := []struct {
		at       ArgumentType
		expected string
	}{
		{ArgTypeDirectCallee, "DirectCallee"},
		{ArgTypeFunctionCall, "FunctionCall"},
		{ArgTypeVariable, "Variable"},
		{ArgTypeLiteral, "Literal"},
		{ArgTypeSelector, "Selector"},
		{ArgTypeComplex, "Complex"},
		{ArgTypeUnary, "Unary"},
		{ArgTypeBinary, "Binary"},
		{ArgTypeIndex, "Index"},
		{ArgTypeComposite, "Composite"},
		{ArgTypeTypeAssert, "TypeAssert"},
		{ArgumentType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.at.String())
		})
	}
}

// ========================================================================
// Additional: TrackerNode.Key() various branches
// ========================================================================

func TestTrackerNode_Key_WithExplicitKey(t *testing.T) {
	node := &TrackerNode{key: "explicit-key"}
	assert.Equal(t, "explicit-key", node.Key())
}

func TestTrackerNode_Key_FromCallArgument(t *testing.T) {
	meta := pmcTestMeta()
	arg := buildIdentArg(meta, "myVar", "mypkg")
	node := &TrackerNode{CallArgument: arg}
	key := node.Key()
	assert.NotEmpty(t, key)
}

func TestTrackerNode_Key_FromCallGraphEdge(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "HandleFunc", "net/http", nil)
	node := &TrackerNode{CallGraphEdge: edge}
	key := node.Key()
	assert.NotEmpty(t, key)
}

func TestTrackerNode_Key_StripsStarPrefix(t *testing.T) {
	node := &TrackerNode{key: "*something"}
	key := node.Key()
	assert.Equal(t, "something", key)
}

// ========================================================================
// Additional: detachChild
// ========================================================================

func TestDetachChild_SingleChild_PMC(t *testing.T) {
	parent := &TrackerNode{key: "p", Children: []*TrackerNode{}}
	child := &TrackerNode{key: "c", Children: []*TrackerNode{}}
	parent.Children = append(parent.Children, child)
	child.Parent = parent

	detachChild(child)
	assert.Empty(t, parent.Children)
}

func TestDetachChild_MultipleChildren(t *testing.T) {
	parent := &TrackerNode{key: "p", Children: []*TrackerNode{}}
	c1 := &TrackerNode{key: "c1", Children: []*TrackerNode{}}
	c2 := &TrackerNode{key: "c2", Children: []*TrackerNode{}}
	c3 := &TrackerNode{key: "c3", Children: []*TrackerNode{}}
	parent.Children = append(parent.Children, c1, c2, c3)
	c1.Parent = parent
	c2.Parent = parent
	c3.Parent = parent

	detachChild(c2)
	assert.Len(t, parent.Children, 2)
	for _, c := range parent.Children {
		assert.NotEqual(t, "c2", c.Key())
	}
}

func TestDetachChild_NilParent_PMC(_ *testing.T) {
	child := &TrackerNode{key: "c"}
	// Should not panic
	detachChild(child)
}

// ========================================================================
// Additional: TrackerTree findNodeByEdgeID
// ========================================================================

func TestFindNodeByEdgeID_InNodeMap(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func", "pkg", nil)
	node := buildTrackerNode(edge)
	edgeID := edge.Callee.ID()

	tree := &TrackerTree{
		meta:    meta,
		nodeMap: map[string]*TrackerNode{edgeID: node},
	}

	found := tree.findNodeByEdgeID(edgeID)
	assert.Equal(t, node, found)
}

func TestFindNodeByEdgeID_NotFound(t *testing.T) {
	meta := pmcTestMeta()
	tree := &TrackerTree{
		meta:    meta,
		roots:   []*TrackerNode{},
		nodeMap: map[string]*TrackerNode{},
	}

	found := tree.findNodeByEdgeID("nonexistent")
	assert.Nil(t, found)
}

// ========================================================================
// Additional: newArgumentNode
// ========================================================================

func TestNewArgumentNode_EmptyID(t *testing.T) {
	result := newArgumentNode(nil, nil, "", nil, nil)
	assert.Nil(t, result)
}

func TestNewArgumentNode_ValidID(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "Func", "pkg", nil)
	arg := buildIdentArg(meta, "x", "pkg")
	parent := buildTrackerNode(edge)

	tree := &TrackerTree{
		meta:    meta,
		nodeMap: map[string]*TrackerNode{},
	}

	result := newArgumentNode(tree, parent, "test-id", edge, arg)
	require.NotNil(t, result)
	assert.Equal(t, edge, result.CallGraphEdge)
	assert.Equal(t, arg, result.CallArgument)
	assert.Equal(t, parent, result.Parent)
}

// ========================================================================
// Additional: TrackerTree.GetNodeCount
// ========================================================================

func TestTrackerTree_GetNodeCount_Empty_PMC(t *testing.T) {
	meta := pmcTestMeta()
	meta.BuildCallGraphMaps()

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		roots:                  []*TrackerNode{},
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	assert.Equal(t, 0, tree.GetNodeCount())
}

// ========================================================================
// Additional: MethodFromCall with default extraction config
// ========================================================================

func TestExtractRouteDetails_MethodFromCall_PostContains(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "main", "main", "PostUsers", "net/http", nil)
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		MethodFromCall: true,
		MethodExtraction: &MethodExtractionConfig{
			MethodMappings: []MethodMapping{
				{Patterns: []string{"get"}, Method: "GET", Priority: 10},
				{Patterns: []string{"post"}, Method: "POST", Priority: 10},
				{Patterns: []string{"delete"}, Method: "DELETE", Priority: 10},
			},
			UsePrefix:   true,
			UseContains: true,
		},
	})

	routeInfo := &RouteInfo{
		File:      "test.go",
		Package:   "main",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	found := matcher.extractRouteDetails(node, routeInfo)
	assert.True(t, found)
	assert.Equal(t, "POST", routeInfo.Method)
}

// ========================================================================
// Additional: ExtractRoute with handler KindFuncLit
// ========================================================================

func TestExtractRoute_HandlerFuncLit(t *testing.T) {
	meta := pmcTestMeta()

	handlerArg := metadata.NewCallArgument(meta)
	handlerArg.SetKind(metadata.KindFuncLit)
	handlerArg.SetName("FuncLit:main.go:20")

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "chi",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/test"), handlerArg})
	edge.Position = meta.StringPool.Get("main.go:10")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		PathFromArg:     true,
		PathArgIndex:    0,
		HandlerFromArg:  true,
		HandlerArgIndex: 1,
	})

	routeInfo := RouteInfo{}
	found := matcher.ExtractRoute(node, &routeInfo)
	assert.True(t, found)
	assert.Equal(t, "/test", routeInfo.Path)
}

// ========================================================================
// Additional: ExtractRoute with nil routeInfo fields
// ========================================================================

func TestExtractRoute_InitializesRouteInfoWhenEmpty(t *testing.T) {
	meta := pmcTestMeta()

	edge := buildCallGraphEdge(meta, "main", "main", "Get", "chi",
		[]*metadata.CallArgument{buildLiteralArg(meta, "/init-test")})
	edge.Position = meta.StringPool.Get("app.go:5")
	node := buildTrackerNode(edge)

	matcher := newRoutePatternMatcher(meta, RoutePattern{
		PathFromArg:  true,
		PathArgIndex: 0,
	})

	// Pass a nil-ish routeInfo (empty file/package)
	routeInfo := &RouteInfo{}
	matcher.ExtractRoute(node, routeInfo)

	// Should have been initialized with defaults
	assert.Equal(t, http.MethodPost, routeInfo.Method)
	assert.NotNil(t, routeInfo.Response)
	assert.NotNil(t, routeInfo.UsedTypes)
	assert.Equal(t, "app.go:5", routeInfo.File)
}
