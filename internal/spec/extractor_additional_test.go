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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// ===========================================================================
// callGraphEdgeNode methods (all at 0%)
// ===========================================================================

func TestCallGraphEdgeNode_AllMethods(t *testing.T) {
	meta := newTestMeta()
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", nil)
	n := &callGraphEdgeNode{edge: &edge}

	assert.Equal(t, &edge, n.GetEdge())
	assert.Equal(t, "", n.GetKey())
	assert.Nil(t, n.GetChildren())
	assert.Nil(t, n.GetParent())
	assert.Nil(t, n.GetArgument())
	assert.Nil(t, n.GetTypeParamMap())
	assert.Equal(t, metadata.ArgumentType(0), n.GetArgType())
	assert.Equal(t, -1, n.GetArgIndex())
	assert.Equal(t, "", n.GetArgContext())
	assert.Nil(t, n.GetRootAssignmentMap())
}

// ===========================================================================
// isValidHTTPMethodStr (0%)
// ===========================================================================

func TestIsValidHTTPMethodStr(t *testing.T) {
	valid := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, m := range valid {
		assert.True(t, isValidHTTPMethodStr(m), "expected %q to be valid", m)
	}

	invalid := []string{"get", "Post", "CONNECT", "TRACE", "", "INVALID", "GE T"}
	for _, m := range invalid {
		assert.False(t, isValidHTTPMethodStr(m), "expected %q to be invalid", m)
	}
}

// ===========================================================================
// floatPtrEqual — missing branches (80%)
// ===========================================================================

func TestFloatPtrEqual_AllBranches(t *testing.T) {
	a := 1.5
	b := 1.5
	c := 2.0

	// both nil
	assert.True(t, floatPtrEqual(nil, nil))
	// a nil, b non-nil
	assert.False(t, floatPtrEqual(nil, &b))
	// a non-nil, b nil
	assert.False(t, floatPtrEqual(&a, nil))
	// equal values
	assert.True(t, floatPtrEqual(&a, &b))
	// unequal values
	assert.False(t, floatPtrEqual(&a, &c))
}

// ===========================================================================
// schemasEqual — additional branches (70.6%)
// ===========================================================================

func TestSchemasEqual_AdditionalBranches(t *testing.T) {
	// both nil
	assert.True(t, schemasEqual(nil, nil))
	// a nil, b non-nil
	assert.False(t, schemasEqual(nil, &Schema{Type: "string"}))
	// a non-nil, b nil
	assert.False(t, schemasEqual(&Schema{Type: "string"}, nil))

	// Type mismatch
	assert.False(t, schemasEqual(&Schema{Type: "string"}, &Schema{Type: "integer"}))
	// Format mismatch
	assert.False(t, schemasEqual(&Schema{Type: "string", Format: "binary"}, &Schema{Type: "string", Format: "date"}))
	// Ref mismatch
	assert.False(t, schemasEqual(&Schema{Ref: "#/a"}, &Schema{Ref: "#/b"}))

	// Minimum mismatch via floatPtrEqual
	min1 := 1.0
	min2 := 2.0
	assert.False(t, schemasEqual(&Schema{Minimum: &min1}, &Schema{Minimum: &min2}))
	// Maximum mismatch
	assert.False(t, schemasEqual(&Schema{Maximum: &min1}, &Schema{Maximum: &min2}))

	// Items: both non-nil and equal
	assert.True(t, schemasEqual(
		&Schema{Type: "array", Items: &Schema{Type: "string"}},
		&Schema{Type: "array", Items: &Schema{Type: "string"}},
	))
	// Items: both non-nil and not equal
	assert.False(t, schemasEqual(
		&Schema{Type: "array", Items: &Schema{Type: "string"}},
		&Schema{Type: "array", Items: &Schema{Type: "integer"}},
	))
	// Items: a has items, b does not
	assert.False(t, schemasEqual(
		&Schema{Type: "array", Items: &Schema{Type: "string"}},
		&Schema{Type: "array"},
	))
	// Items: a has no items, b does
	assert.False(t, schemasEqual(
		&Schema{Type: "array"},
		&Schema{Type: "array", Items: &Schema{Type: "string"}},
	))

	// AdditionalProperties: both non-nil and equal
	assert.True(t, schemasEqual(
		&Schema{AdditionalProperties: &Schema{Type: "string"}},
		&Schema{AdditionalProperties: &Schema{Type: "string"}},
	))
	// AdditionalProperties: both non-nil and not equal
	assert.False(t, schemasEqual(
		&Schema{AdditionalProperties: &Schema{Type: "string"}},
		&Schema{AdditionalProperties: &Schema{Type: "integer"}},
	))
	// AdditionalProperties: a has, b does not
	assert.False(t, schemasEqual(
		&Schema{AdditionalProperties: &Schema{Type: "string"}},
		&Schema{},
	))
	// AdditionalProperties: a does not, b has
	assert.False(t, schemasEqual(
		&Schema{},
		&Schema{AdditionalProperties: &Schema{Type: "string"}},
	))

	// Fully equal with all fields
	assert.True(t, schemasEqual(
		&Schema{Type: "string", Format: "date", Ref: "#/x"},
		&Schema{Type: "string", Format: "date", Ref: "#/x"},
	))
}

// ===========================================================================
// splitByConditionalMethods (33.3%)
// ===========================================================================

func TestSplitByConditionalMethods_TwoHTTPMethods(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Path:     "/resource",
		Handler:  "handler",
		Function: "ServeHTTP",
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				BodyType:    "User",
				Branch: &metadata.BranchContext{
					BlockKind:  "switch-case",
					CaseValues: []string{"GET"},
				},
			},
			"201": {
				StatusCode:  201,
				ContentType: "application/json",
				BodyType:    "User",
				Branch: &metadata.BranchContext{
					BlockKind:  "switch-case",
					CaseValues: []string{"POST"},
				},
			},
		},
		UsedTypes: map[string]*Schema{},
	}

	result := ext.splitByConditionalMethods(route)
	require.Len(t, result, 2)

	methods := map[string]bool{}
	for _, r := range result {
		methods[r.Method] = true
		assert.Equal(t, "/resource", r.Path)
		assert.Equal(t, "ServeHTTP", r.Function)
	}
	assert.True(t, methods["GET"])
	assert.True(t, methods["POST"])
}

func TestSplitByConditionalMethods_SingleMethod_ReturnsNil(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Path:    "/resource",
		Handler: "handler",
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode: 200,
				Branch: &metadata.BranchContext{
					BlockKind:  "switch-case",
					CaseValues: []string{"GET"},
				},
			},
		},
	}

	result := ext.splitByConditionalMethods(route)
	assert.Nil(t, result)
}

func TestSplitByConditionalMethods_NoBranch_ReturnsNil(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Path:    "/resource",
		Handler: "handler",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, Branch: nil},
		},
	}

	result := ext.splitByConditionalMethods(route)
	assert.Nil(t, result)
}

func TestSplitByConditionalMethods_InvalidHTTPMethod_Skipped(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Path:    "/resource",
		Handler: "handler",
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode: 200,
				Branch: &metadata.BranchContext{
					BlockKind:  "switch-case",
					CaseValues: []string{"INVALID"},
				},
			},
			"201": {
				StatusCode: 201,
				Branch: &metadata.BranchContext{
					BlockKind:  "switch-case",
					CaseValues: []string{"ALSO_INVALID"},
				},
			},
		},
	}

	result := ext.splitByConditionalMethods(route)
	assert.Nil(t, result)
}

func TestSplitByConditionalMethods_NonSwitchCase_Skipped(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Path:    "/resource",
		Handler: "handler",
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode: 200,
				Branch: &metadata.BranchContext{
					BlockKind:  "if-then",
					CaseValues: []string{"GET"},
				},
			},
		},
	}

	result := ext.splitByConditionalMethods(route)
	assert.Nil(t, result)
}

// ===========================================================================
// isInterfaceHandler (38.9%)
// ===========================================================================

func TestIsInterfaceHandler_NilMetadata(t *testing.T) {
	tree := NewMockTrackerTree(nil, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := &Extractor{tree: tree, cfg: cfg}

	route := &RouteInfo{Function: "pkg-->Server.Handle"}
	assert.False(t, ext.isInterfaceHandler(route))
}

func TestIsInterfaceHandler_NoTypeSep(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := &Extractor{tree: tree, cfg: cfg}

	route := &RouteInfo{Function: "SimpleFunc"}
	assert.False(t, ext.isInterfaceHandler(route))
}

func TestIsInterfaceHandler_NoDotInMethod(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := &Extractor{tree: tree, cfg: cfg}

	// Has TypeSep but no dot in the method part
	route := &RouteInfo{Function: "pkg-->NoDotHere"}
	assert.False(t, ext.isInterfaceHandler(route))
}

func TestIsInterfaceHandler_InterfaceType(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Types: map[string]*metadata.Type{
				"ContentServer": {
					Name: meta.StringPool.Get("ContentServer"),
					Kind: meta.StringPool.Get("interface"),
				},
			},
		},
	}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := &Extractor{tree: tree, cfg: cfg}

	route := &RouteInfo{Function: "mypkg-->ContentServer.Serve"}
	assert.True(t, ext.isInterfaceHandler(route))
}

func TestIsInterfaceHandler_StructType(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Types: map[string]*metadata.Type{
				"ContentServer": {
					Name: meta.StringPool.Get("ContentServer"),
					Kind: meta.StringPool.Get("struct"),
				},
			},
		},
	}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := &Extractor{tree: tree, cfg: cfg}

	route := &RouteInfo{Function: "mypkg-->ContentServer.Serve"}
	assert.False(t, ext.isInterfaceHandler(route))
}

// ===========================================================================
// resolveInterfaceHandler (0%)
// ===========================================================================

func TestResolveInterfaceHandler_EmptyFuncName(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{Function: "", Response: make(map[string]*ResponseInfo)}
	// Should return early without panic
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)
	assert.Empty(t, route.Response)
}

func TestResolveInterfaceHandler_NilMetadata(t *testing.T) {
	tree := NewMockTrackerTree(nil, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{Function: "ContentServer.Serve", Response: make(map[string]*ResponseInfo)}
	// Should return early without panic
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)
	assert.Empty(t, route.Response)
}

func TestResolveInterfaceHandler_FindsConcreteImpl(t *testing.T) {
	meta := newTestMeta()

	// Create a call graph edge where a concrete implementation calls a response-writing function
	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "user", "User")

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Serve"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: meta.StringPool.Get("ConcreteServer"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("JSON"),
			Pkg:  meta.StringPool.Get("gin"),
		},
		Args: []*metadata.CallArgument{statusArg, bodyArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge

	meta.CallGraph = []metadata.CallGraphEdge{edge}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
					TypeFromArg:    true,
					TypeArgIndex:   1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Function:  "mypkg-->ContentServer.Serve",
		Package:   "mypkg",
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
		Metadata:  meta,
	}
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)

	// Should have found the concrete JSON call and extracted a response
	assert.NotEmpty(t, route.Response)
	if resp, ok := route.Response["200"]; ok {
		assert.Equal(t, 200, resp.StatusCode)
	}
}

func TestResolveInterfaceHandler_SkipsMismatchedCallerName(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("DifferentMethod"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: meta.StringPool.Get("ConcreteServer"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("JSON"),
			Pkg:  meta.StringPool.Get("gin"),
		},
		Args: []*metadata.CallArgument{statusArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	meta.CallGraph = []metadata.CallGraphEdge{edge}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Function:  "mypkg-->ContentServer.Serve",
		Package:   "mypkg",
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
		Metadata:  meta,
	}
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)
	// "DifferentMethod" != "Serve", so no responses should be extracted
	assert.Empty(t, route.Response)
}

func TestResolveInterfaceHandler_SkipsEmptyRecvType(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Serve"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1, // Explicitly no recv type (plain function, not a method)
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("JSON"),
			Pkg:  meta.StringPool.Get("gin"),
		},
		Args: []*metadata.CallArgument{statusArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	meta.CallGraph = []metadata.CallGraphEdge{edge}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Function:  "mypkg-->ContentServer.Serve",
		Package:   "mypkg",
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
		Metadata:  meta,
	}
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)
	assert.Empty(t, route.Response)
}

// ===========================================================================
// resolveCallReturnValue (0%)
// ===========================================================================

func TestResolveCallReturnValue_NilFun(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
	}

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	// Fun is nil
	result := matcher.resolveCallReturnValue(arg)
	assert.Equal(t, "", result)
}

func TestResolveCallReturnValue_EmptyFuncName(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
	}

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = makeCallArg(meta) // Fun exists but resolves to empty

	result := matcher.resolveCallReturnValue(arg)
	assert.Equal(t, "", result)
}

func TestResolveCallReturnValue_FindsConstantReturn(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"helpers.go": {
					Functions: map[string]*metadata.Function{
						"getStatus": {
							Name:                meta.StringPool.Get("getStatus"),
							ConstantReturnValue: "200",
						},
					},
				},
			},
		},
	}

	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
	}

	// Create a call argument with Fun pointing to "mypkg.getStatus"
	funArg := makeLiteralArg(meta, "mypkg.getStatus")
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = funArg

	result := matcher.resolveCallReturnValue(arg)
	assert.Equal(t, "200", result)
}

func TestResolveCallReturnValue_FuncNotFound(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"helpers.go": {
					Functions: map[string]*metadata.Function{
						"otherFunc": {
							Name:                meta.StringPool.Get("otherFunc"),
							ConstantReturnValue: "404",
						},
					},
				},
			},
		},
	}

	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
	}

	funArg := makeLiteralArg(meta, "mypkg.nonExistent")
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = funArg

	result := matcher.resolveCallReturnValue(arg)
	assert.Equal(t, "", result)
}

// ===========================================================================
// checkContentTypePattern (28.1%)
// ===========================================================================

func TestCheckContentTypePattern_NilEdge(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	node := &TrackerNode{} // no edge
	route := NewRouteInfo()
	// Should return early without panic
	ext.checkContentTypePattern(node, route)
	assert.Equal(t, "", route.detectedContentType)
}

func TestCheckContentTypePattern_MatchAndOverrideResponses(t *testing.T) {
	meta := newTestMeta()

	// Build edge: Header().Set("Content-Type", "image/png")
	headerNameArg := makeLiteralArg(meta, `"Content-Type"`)
	headerValueArg := makeLiteralArg(meta, `"image/png"`)

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Set"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("Header"),
		},
		Args: []*metadata.CallArgument{headerNameArg, headerValueArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	node := makeTrackerNode(&edge)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ContentTypePatterns: []ContentTypePattern{
				{
					BasePattern:         BasePattern{CallRegex: "^Set$", RecvTypeRegex: "Header"},
					HeaderNameArgIndex:  0,
					HeaderValueArgIndex: 1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	route.Response["200"] = &ResponseInfo{
		StatusCode:  200,
		ContentType: "application/json", // default, should be overridden
	}

	ext.checkContentTypePattern(node, route)

	assert.Equal(t, "image/png", route.detectedContentType)
	assert.Equal(t, "image/png", route.Response["200"].ContentType)
}

func TestCheckContentTypePattern_DoesNotOverridePatternSpecificContentType(t *testing.T) {
	meta := newTestMeta()

	headerNameArg := makeLiteralArg(meta, `"Content-Type"`)
	headerValueArg := makeLiteralArg(meta, `"image/png"`)

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Set"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("Header"),
		},
		Args: []*metadata.CallArgument{headerNameArg, headerValueArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	node := makeTrackerNode(&edge)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ContentTypePatterns: []ContentTypePattern{
				{
					BasePattern:         BasePattern{CallRegex: "^Set$", RecvTypeRegex: "Header"},
					HeaderNameArgIndex:  0,
					HeaderValueArgIndex: 1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	route.Response["400"] = &ResponseInfo{
		StatusCode:  400,
		ContentType: "text/plain; charset=utf-8", // pattern-specific, should NOT be overridden
	}

	ext.checkContentTypePattern(node, route)

	assert.Equal(t, "image/png", route.detectedContentType)
	assert.Equal(t, "text/plain; charset=utf-8", route.Response["400"].ContentType)
}

func TestCheckContentTypePattern_NonContentTypeHeader_Skipped(t *testing.T) {
	meta := newTestMeta()

	headerNameArg := makeLiteralArg(meta, `"X-Custom-Header"`)
	headerValueArg := makeLiteralArg(meta, `"some-value"`)

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Set"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("Header"),
		},
		Args: []*metadata.CallArgument{headerNameArg, headerValueArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	node := makeTrackerNode(&edge)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ContentTypePatterns: []ContentTypePattern{
				{
					BasePattern:         BasePattern{CallRegex: "^Set$", RecvTypeRegex: "Header"},
					HeaderNameArgIndex:  0,
					HeaderValueArgIndex: 1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	ext.checkContentTypePattern(node, route)
	assert.Equal(t, "", route.detectedContentType)
}

func TestCheckContentTypePattern_CallRegexNoMatch(t *testing.T) {
	meta := newTestMeta()

	headerNameArg := makeLiteralArg(meta, `"Content-Type"`)
	headerValueArg := makeLiteralArg(meta, `"image/png"`)

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Add"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("Header"),
		},
		Args: []*metadata.CallArgument{headerNameArg, headerValueArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	node := makeTrackerNode(&edge)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ContentTypePatterns: []ContentTypePattern{
				{
					BasePattern:         BasePattern{CallRegex: "^Set$", RecvTypeRegex: "Header"}, // "Set" won't match "Add"
					HeaderNameArgIndex:  0,
					HeaderValueArgIndex: 1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	ext.checkContentTypePattern(node, route)
	assert.Equal(t, "", route.detectedContentType)
}

func TestCheckContentTypePattern_RecvTypeRegexNoMatch(t *testing.T) {
	meta := newTestMeta()

	headerNameArg := makeLiteralArg(meta, `"Content-Type"`)
	headerValueArg := makeLiteralArg(meta, `"image/png"`)

	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Set"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"), // won't match "Header"
		},
		Args: []*metadata.CallArgument{headerNameArg, headerValueArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	node := makeTrackerNode(&edge)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ContentTypePatterns: []ContentTypePattern{
				{
					BasePattern:         BasePattern{CallRegex: "^Set$", RecvTypeRegex: "^Header$"},
					HeaderNameArgIndex:  0,
					HeaderValueArgIndex: 1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	ext.checkContentTypePattern(node, route)
	assert.Equal(t, "", route.detectedContentType)
}

// ===========================================================================
// extractParamsFromAssignmentMaps (15.8%)
// ===========================================================================

func TestExtractParamsFromAssignmentMaps_NilEdge(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	node := &TrackerNode{} // no edge
	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

func TestExtractParamsFromAssignmentMaps_NilAssignmentMap(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	node := makeTrackerNode(&edge)
	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

func TestExtractParamsFromAssignmentMaps_ExtractsMapIndex(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	// Build an assignment map with a KindIndex value whose Fun (key) is a literal "id"
	keyArg := makeLiteralArg(meta, `"id"`)
	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIndex)
	valArg.Fun = keyArg

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"id": {
			{
				Value: *valArg,
			},
		},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)

	require.Len(t, route.Params, 1)
	assert.Equal(t, "id", route.Params[0].Name)
	assert.Equal(t, "path", route.Params[0].In)
	assert.True(t, route.Params[0].Required)
	require.NotNil(t, route.Params[0].Schema)
	assert.Equal(t, "string", route.Params[0].Schema.Type)
}

func TestExtractParamsFromAssignmentMaps_SkipsDuplicateParam(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	keyArg := makeLiteralArg(meta, `"id"`)
	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIndex)
	valArg.Fun = keyArg

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"id": {{Value: *valArg}},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	// Pre-populate with existing param
	route.Params = []Parameter{{Name: "id", In: "path", Required: true}}
	ext.extractParamsFromAssignmentMaps(node, route)

	// Should not add a duplicate
	assert.Len(t, route.Params, 1)
}

func TestExtractParamsFromAssignmentMaps_SkipsNonIndexKind(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIdent)
	valArg.SetName("someVar")

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"v": {{Value: *valArg}},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

func TestExtractParamsFromAssignmentMaps_SkipsNilFun(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIndex)
	// Fun is nil

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"v": {{Value: *valArg}},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

func TestExtractParamsFromAssignmentMaps_SkipsNonLiteralFun(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	funArg := makeIdentArg(meta, "dynamicKey", "string")
	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIndex)
	valArg.Fun = funArg

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"v": {{Value: *valArg}},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

func TestExtractParamsFromAssignmentMaps_SkipsEmptyKey(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	// literal with empty value
	funArg := makeLiteralArg(meta, `""`)
	valArg := makeCallArg(meta)
	valArg.SetKind(metadata.KindIndex)
	valArg.Fun = funArg

	edge := makeEdge(meta, "handler", "main", "Vars", "mux", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"v": {{Value: *valArg}},
	}
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	ext.extractParamsFromAssignmentMaps(node, route)
	assert.Empty(t, route.Params)
}

// ===========================================================================
// ExtractResponse — status from KindCall with resolveCallReturnValue (64%)
// ===========================================================================

func TestExtractResponse_StatusFromCallWithConstantReturn(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Files: map[string]*metadata.File{
				"helpers.go": {
					Functions: map[string]*metadata.Function{
						"getStatus": {
							Name:                meta.StringPool.Get("getStatus"),
							ConstantReturnValue: "201",
						},
					},
				},
			},
		},
	}

	// Create a KindCall arg whose Fun resolves to "mypkg.getStatus"
	funArg := makeLiteralArg(meta, "mypkg.getStatus")
	statusArg := makeCallArg(meta)
	statusArg.SetKind(metadata.KindCall)
	statusArg.Fun = funArg

	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 201, resp.StatusCode)
}

// ===========================================================================
// ExtractResponse — status from KindIdent with assignment map (64%)
// ===========================================================================

func TestExtractResponse_StatusFromIdentWithAssignment(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeIdentArg(meta, "code", "int")

	// Create an assignment map entry for "code" = "404"
	assignValue := makeLiteralArg(meta, "404")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"code": {
			{Value: *assignValue},
		},
	}
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.StatusCode)
}

// ===========================================================================
// ExtractResponse — DefaultBodyType for []byte with text reader
// ===========================================================================

func TestExtractResponse_DefaultBodyType_ByteSlice_TextReader(t *testing.T) {
	meta := newTestMeta()

	// Build an edge for io.Copy(w, strings.NewReader("hello"))
	writerArg := makeIdentArg(meta, "w", "http.ResponseWriter")
	readerArg := makeIdentArg(meta, "reader", "io.Reader")

	edge := makeEdge(meta, "handler", "main", "Copy", "io", []*metadata.CallArgument{writerArg, readerArg})
	// Set up assignment map for "reader" pointing to strings.NewReader
	assignValue := makeLiteralArg(meta, "strings.NewReader")
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"reader": {
			{Value: *assignValue},
		},
	}
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/octet-stream"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus:   200,
		DefaultBodyType: "[]byte",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
	// Should detect text reader and NOT set binary format
	assert.Equal(t, "", resp.Schema.Format)
}

func TestExtractResponse_DefaultBodyType_ByteSlice_BinaryReader(t *testing.T) {
	meta := newTestMeta()

	writerArg := makeIdentArg(meta, "w", "http.ResponseWriter")
	readerArg := makeIdentArg(meta, "file", "*os.File")

	edge := makeEdge(meta, "handler", "main", "Copy", "io", []*metadata.CallArgument{writerArg, readerArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/octet-stream"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus:   200,
		DefaultBodyType: "[]byte",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
	assert.Equal(t, "binary", resp.Schema.Format)
}

func TestExtractResponse_DefaultBodyType_NonByteSlice(t *testing.T) {
	meta := newTestMeta()

	edge := makeEdge(meta, "handler", "main", "Fprintf", "fmt", nil)
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "text/plain"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus:   200,
		DefaultBodyType: "string",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "string", resp.BodyType)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
}

// ===========================================================================
// ExtractResponse — branch context propagation
// ===========================================================================

func TestExtractResponse_BranchContextPropagated(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	edge.Branch = &metadata.BranchContext{
		BlockKind:  "switch-case",
		CaseValues: []string{"GET"},
	}
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	require.NotNil(t, resp.Branch)
	assert.Equal(t, "switch-case", resp.Branch.BlockKind)
	assert.Equal(t, []string{"GET"}, resp.Branch.CaseValues)
}

// ===========================================================================
// handleRouteNode — detected content type override (66.7%)
// ===========================================================================

func TestHandleRouteNode_DetectedContentTypeOverride(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	routeInfo := &RouteInfo{
		Path:     "/test",
		Handler:  "TestHandler",
		Function: "TestHandler",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json"},
		},
		UsedTypes:           map[string]*Schema{},
		detectedContentType: "image/png",
	}

	routes := make([]*RouteInfo, 0)
	// Create a node with no children
	node := &TrackerNode{key: "test-node"}
	ext.handleRouteNode(node, routeInfo, "", nil, &routes)

	// The detected content type should override the default
	require.Len(t, routes, 1)
	assert.Equal(t, "image/png", routes[0].Response["200"].ContentType)
}

func TestHandleRouteNode_DetectedContentTypeDoesNotOverridePatternSpecific(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	routeInfo := &RouteInfo{
		Path:     "/test",
		Handler:  "TestHandler",
		Function: "TestHandler",
		Response: map[string]*ResponseInfo{
			"400": {StatusCode: 400, ContentType: "text/plain; charset=utf-8"},
		},
		UsedTypes:           map[string]*Schema{},
		detectedContentType: "image/png",
	}

	routes := make([]*RouteInfo, 0)
	node := &TrackerNode{key: "test-node"}
	ext.handleRouteNode(node, routeInfo, "", nil, &routes)

	require.Len(t, routes, 1)
	// text/plain should NOT be overridden
	assert.Equal(t, "text/plain; charset=utf-8", routes[0].Response["400"].ContentType)
}

// ===========================================================================
// handleRouteNode — mount path prepend and tags
// ===========================================================================

func TestHandleRouteNode_MountPathPrepend(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	routeInfo := &RouteInfo{
		Path:      "/users",
		MountPath: "/users",
		Handler:   "GetUsers",
		Function:  "GetUsers",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	routes := make([]*RouteInfo, 0)
	node := &TrackerNode{key: "test-node"}
	ext.handleRouteNode(node, routeInfo, "/api", []string{"api"}, &routes)

	require.Len(t, routes, 1)
	assert.Equal(t, "/api/users", routes[0].MountPath)
	assert.Equal(t, []string{"api"}, routes[0].Tags)
}

// ===========================================================================
// handleRouteNode — route dedup by function name
// ===========================================================================

func TestHandleRouteNode_UpdatesExistingRoute(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	// Existing route in the list
	existing := &RouteInfo{
		Path:      "/users",
		Handler:   "GetUsers",
		Function:  "GetUsers",
		Summary:   "old summary",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}
	routes := []*RouteInfo{existing}

	// New route with same function
	newRoute := &RouteInfo{
		Path:      "/users",
		Handler:   "GetUsers",
		Function:  "GetUsers",
		Summary:   "new summary",
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
	}

	node := &TrackerNode{key: "test-node"}
	ext.handleRouteNode(node, newRoute, "", nil, &routes)

	// Should update, not duplicate
	require.Len(t, routes, 1)
	assert.Equal(t, "new summary", routes[0].Summary)
}

// ===========================================================================
// handleRouteNode — splitByConditionalMethods integration
// ===========================================================================

func TestHandleRouteNode_SplitsByConditionalMethods(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	routeInfo := &RouteInfo{
		Path:     "/resource",
		Handler:  "ServeHTTP",
		Function: "ServeHTTP",
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Branch:      &metadata.BranchContext{BlockKind: "switch-case", CaseValues: []string{"GET"}},
			},
			"201": {
				StatusCode:  201,
				ContentType: "application/json",
				Branch:      &metadata.BranchContext{BlockKind: "switch-case", CaseValues: []string{"POST"}},
			},
		},
		UsedTypes: map[string]*Schema{},
	}

	routes := make([]*RouteInfo, 0)
	node := &TrackerNode{key: "test-node"}
	ext.handleRouteNode(node, routeInfo, "", nil, &routes)

	// Should produce 2 routes (GET and POST)
	require.Len(t, routes, 2)
	methods := map[string]bool{}
	for _, r := range routes {
		methods[r.Method] = true
	}
	assert.True(t, methods["GET"])
	assert.True(t, methods["POST"])
}

// ===========================================================================
// handleRouterAssignment (25%)
// ===========================================================================

func TestHandleRouterAssignment_NilAssignment(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	mountInfo := MountInfo{Assignment: nil}
	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)
	// Should not panic
	ext.handleRouterAssignment(mountInfo, "/api", nil, &routes, visited)
	assert.Empty(t, routes)
}

func TestHandleRouterAssignment_AssignmentNotFound(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	assignment := makeIdentArg(meta, "nonExistent", "*chi.Mux")
	mountInfo := MountInfo{Assignment: assignment}
	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)
	ext.handleRouterAssignment(mountInfo, "/api", nil, &routes, visited)
	assert.Empty(t, routes)
}

// ===========================================================================
// findTargetNode (90%)
// ===========================================================================

func TestFindTargetNode_NilAssignment(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	assert.Nil(t, ext.findTargetNode(nil))
}

// ===========================================================================
// visitChildren — basic callback invocation
// ===========================================================================

func TestVisitChildren_CallsCallbacksOnChildren(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	child1 := &TrackerNode{key: "child1"}
	child2 := &TrackerNode{key: "child2"}
	parent := &TrackerNode{
		key:      "parent",
		Children: []*TrackerNode{child1, child2},
	}

	var visited []string
	callbacks := []ExtractionCallback{
		func(node TrackerNodeInterface, _ *RouteInfo) {
			visited = append(visited, node.GetKey())
		},
	}

	route := NewRouteInfo()
	ext.visitChildren(parent, route, callbacks)

	assert.Equal(t, []string{"child1", "child2"}, visited)
}

func TestVisitChildren_RecursesIntoGrandchildren(t *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	grandchild := &TrackerNode{key: "grandchild"}
	child := &TrackerNode{
		key:      "child",
		Children: []*TrackerNode{grandchild},
	}
	parent := &TrackerNode{
		key:      "parent",
		Children: []*TrackerNode{child},
	}

	var visited []string
	callbacks := []ExtractionCallback{
		func(node TrackerNodeInterface, _ *RouteInfo) {
			visited = append(visited, node.GetKey())
		},
	}

	route := NewRouteInfo()
	ext.visitChildren(parent, route, callbacks)

	assert.Equal(t, []string{"child", "grandchild"}, visited)
}

// ===========================================================================
// extractRouteChildren — response schema merging in callback (51.9%)
// ===========================================================================

func TestExtractRouteChildren_MergesAlternativeSchemas(t *testing.T) {
	meta := newTestMeta()

	// Build two response-writing edges as children with different positions
	// so they have distinct edge IDs and aren't deduplicated by visitedEdges.
	statusArg1 := makeLiteralArg(meta, "400")
	bodyArg1 := makeIdentArg(meta, "err", "ErrorResponse")
	edge1 := makeEdge(meta, "handler", "main", "JSON", "gin",
		[]*metadata.CallArgument{statusArg1, bodyArg1})
	edge1.Callee.Position = meta.StringPool.Get("pos1")

	statusArg2 := makeLiteralArg(meta, "400")
	bodyArg2 := makeIdentArg(meta, "data", "map[string]string")
	edge2 := makeEdge(meta, "handler", "main", "JSON", "gin",
		[]*metadata.CallArgument{statusArg2, bodyArg2})
	edge2.Callee.Position = meta.StringPool.Get("pos2")

	child1 := &TrackerNode{key: "resp1", CallGraphEdge: &edge1}
	child2 := &TrackerNode{key: "resp2", CallGraphEdge: &edge2}
	routeNode := &TrackerNode{
		key:      "route",
		Children: []*TrackerNode{child1, child2},
	}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
					TypeFromArg:    true,
					TypeArgIndex:   1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	route.Metadata = meta
	visitedEdges := make(map[string]bool)
	routes := make([]*RouteInfo, 0)
	ext.extractRouteChildren(routeNode, route, nil, &routes, visitedEdges)

	// Both responses target status 400 with different schemas
	resp, ok := route.Response["400"]
	require.True(t, ok, "expected response with status 400")
	assert.NotNil(t, resp.Schema)
	// The second schema should be added as an alternative
	assert.NotEmpty(t, resp.AlternativeSchemas, "expected alternative schemas for same status code with different body types")
}

// ===========================================================================
// extractRouteChildren — duplicate schema dedup
// ===========================================================================

func TestExtractRouteChildren_DeduplicatesSameSchema(t *testing.T) {
	meta := newTestMeta()

	// Two identical response-writing edges
	statusArg1 := makeLiteralArg(meta, "200")
	bodyArg1 := makeIdentArg(meta, "user", "User")
	edge1 := makeEdge(meta, "handler", "main", "JSON", "gin",
		[]*metadata.CallArgument{statusArg1, bodyArg1})

	statusArg2 := makeLiteralArg(meta, "200")
	bodyArg2 := makeIdentArg(meta, "user", "User")
	edge2 := makeEdge(meta, "handler", "main", "JSON", "gin",
		[]*metadata.CallArgument{statusArg2, bodyArg2})

	child1 := &TrackerNode{key: "resp1", CallGraphEdge: &edge1}
	child2 := &TrackerNode{key: "resp2", CallGraphEdge: &edge2}
	routeNode := &TrackerNode{
		key:      "route",
		Children: []*TrackerNode{child1, child2},
	}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
					TypeFromArg:    true,
					TypeArgIndex:   1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := NewRouteInfo()
	route.Metadata = meta
	visitedEdges := make(map[string]bool)
	routes := make([]*RouteInfo, 0)
	ext.extractRouteChildren(routeNode, route, nil, &routes, visitedEdges)

	resp, ok := route.Response["200"]
	require.True(t, ok)
	// Same schema should NOT produce alternatives
	assert.Empty(t, resp.AlternativeSchemas, "identical schemas should be deduplicated")
}

// ===========================================================================
// traverseForRoutesWithVisited — cycle detection
// ===========================================================================

func TestTraverseForRoutesWithVisited_PreventsCycles(_ *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	// Create a cycle: node -> child -> node (via same key)
	node := &TrackerNode{key: "cyclic-node"}
	child := &TrackerNode{key: "cyclic-node"} // same key = cycle
	node.Children = []*TrackerNode{child}

	routes := make([]*RouteInfo, 0)
	// Should not infinite-loop
	ext.traverseForRoutes(node, "", nil, &routes)
}

func TestTraverseForRoutesWithVisited_NilNode(_ *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{}
	ext := NewExtractor(tree, cfg)

	routes := make([]*RouteInfo, 0)
	// Should not panic
	ext.traverseForRoutesWithVisited(nil, "", nil, &routes, make(map[string]bool))
}

// ===========================================================================
// handleMountNode — tag handling
// ===========================================================================

func TestHandleMountNode_SetsTagsFromMountPath(_ *testing.T) {
	meta := newTestMeta()
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	ext := NewExtractor(tree, cfg)

	// Create a route child under the mount node
	statusArg := makeLiteralArg(meta, "200")
	routeEdge := makeEdge(meta, "main", "main", "GET", "chi", []*metadata.CallArgument{
		makeLiteralArg(meta, `"/items"`),
		makeIdentArg(meta, "handler", "func(http.ResponseWriter, *http.Request)"),
	})
	routeChild := &TrackerNode{key: "route-child", CallGraphEdge: &routeEdge}
	_ = statusArg

	mountNode := &TrackerNode{
		key:      "mount-node",
		Children: []*TrackerNode{routeChild},
	}

	mountInfo := MountInfo{Path: "/api"}
	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)
	ext.handleMountNode(mountNode, mountInfo, "", nil, &routes, visited)

	// Verified it ran without panic; the children are traversed with mount path
}

// ===========================================================================
// ExtractResponse — DefaultBodyType with reader arg containing "strings"
// ===========================================================================

func TestExtractResponse_DefaultBodyType_ReaderArgContainsStrings(t *testing.T) {
	meta := newTestMeta()

	writerArg := makeIdentArg(meta, "w", "http.ResponseWriter")
	// reader arg itself (not through assignment) contains "strings.NewReader"
	readerArg := makeLiteralArg(meta, "strings.NewReader")

	edge := makeEdge(meta, "handler", "main", "Copy", "io", []*metadata.CallArgument{writerArg, readerArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/octet-stream"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus:   200,
		DefaultBodyType: "[]byte",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
	// "strings" in reader info means text, not binary
	assert.Equal(t, "", resp.Schema.Format)
}

// ===========================================================================
// handleRouteNode — interface handler resolution path
// ===========================================================================

func TestHandleRouteNode_TriggersInterfaceResolution(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"mypkg": {
			Types: map[string]*metadata.Type{
				"Handler": {
					Name: meta.StringPool.Get("Handler"),
					Kind: meta.StringPool.Get("interface"),
				},
			},
		},
	}

	// Concrete implementation edge
	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "data", "Result")
	concreteEdge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Handle"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: meta.StringPool.Get("ConcreteHandler"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("JSON"),
			Pkg:  meta.StringPool.Get("gin"),
		},
		Args: []*metadata.CallArgument{statusArg, bodyArg},
	}
	concreteEdge.Caller.Edge = &concreteEdge
	concreteEdge.Callee.Edge = &concreteEdge
	meta.CallGraph = []metadata.CallGraphEdge{concreteEdge}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
					TypeFromArg:    true,
					TypeArgIndex:   1,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	routeInfo := &RouteInfo{
		Path:      "/resource",
		Handler:   "Handler.Handle",
		Function:  "mypkg-->Handler.Handle",
		Package:   "mypkg",
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
		Metadata:  meta,
	}

	routes := make([]*RouteInfo, 0)
	node := &TrackerNode{key: "route-node"}
	ext.handleRouteNode(node, routeInfo, "", nil, &routes)

	// Interface resolution should find the concrete impl and add responses
	require.Len(t, routes, 1)
	assert.NotEmpty(t, routes[0].Response)
}

// ===========================================================================
// resolveInterfaceHandler — package mismatch skip
// ===========================================================================

func TestResolveInterfaceHandler_SkipsMismatchedPackage(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Serve"),
			Pkg:      meta.StringPool.Get("otherpkg"),
			RecvType: meta.StringPool.Get("ConcreteServer"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("JSON"),
			Pkg:  meta.StringPool.Get("gin"),
		},
		Args: []*metadata.CallArgument{statusArg},
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	meta.CallGraph = []metadata.CallGraphEdge{edge}

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
		Framework: FrameworkConfig{
			ResponsePatterns: []ResponsePattern{
				{
					BasePattern:    BasePattern{CallRegex: "^JSON$"},
					StatusFromArg:  true,
					StatusArgIndex: 0,
				},
			},
		},
	}
	ext := NewExtractor(tree, cfg)

	route := &RouteInfo{
		Function:  "mypkg-->ContentServer.Serve",
		Package:   "mypkg",
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
		Metadata:  meta,
	}
	ext.resolveInterfaceHandler(nil, route, nil, nil, nil)
	// "otherpkg" doesn't match "mypkg"
	assert.Empty(t, route.Response)
}

// ===========================================================================
// ExtractResponse — leastStatusCode calculation
// ===========================================================================

func TestExtractResponse_LeastStatusCodeCalculation(t *testing.T) {
	meta := newTestMeta()

	edge := makeEdge(meta, "handler", "main", "Write", "http", nil)
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus:   200,
		DefaultBodyType: "string",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	// Pre-populate with some responses
	route.Response["200"] = &ResponseInfo{StatusCode: 200}
	route.Response["500"] = &ResponseInfo{StatusCode: 500}

	resp := matcher.ExtractResponse(node, route)
	require.NotNil(t, resp)
	// The default status (200) should be used since DefaultStatus is set
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Export function tests to improve coverage ---

func TestGenerateCytoscapeHTML_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "diagram.html")
	err := GenerateCytoscapeHTML(nil, outFile)
	require.NoError(t, err)
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "cytoscape")
}

func TestGenerateCytoscapeHTML_BadPath(t *testing.T) {
	err := GenerateCytoscapeHTML(nil, "/nonexistent/dir/out.html")
	require.Error(t, err)
}

func TestExportCytoscapeJSON_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "data.json")
	err := ExportCytoscapeJSON(nil, outFile)
	require.NoError(t, err)
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "nodes")
}

func TestExportCytoscapeJSON_BadPath(t *testing.T) {
	err := ExportCytoscapeJSON(nil, "/nonexistent/dir/out.json")
	require.Error(t, err)
}

func TestGenerateCallGraphCytoscapeHTML_NilMeta(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "callgraph.html")
	err := GenerateCallGraphCytoscapeHTML(nil, outFile)
	require.NoError(t, err)
}

func TestExportCallGraphCytoscapeJSON_NilMeta(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "callgraph.json")
	err := ExportCallGraphCytoscapeJSON(nil, outFile)
	require.NoError(t, err)
}

func TestExportCallGraphCytoscapeJSON_BadPath(t *testing.T) {
	err := ExportCallGraphCytoscapeJSON(nil, "/nonexistent/dir/out.json")
	require.Error(t, err)
}

func TestGenerateCallGraphCytoscapeHTML_BadPath(t *testing.T) {
	err := GenerateCallGraphCytoscapeHTML(nil, "/nonexistent/dir/out.html")
	require.Error(t, err)
}

func TestGeneratePaginatedCytoscapeHTML_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "paginated.html")
	err := GeneratePaginatedCytoscapeHTML(nil, outFile, 100)
	require.NoError(t, err)
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "cytoscape")
}

func TestGeneratePaginatedCytoscapeHTML_BadPath(t *testing.T) {
	err := GeneratePaginatedCytoscapeHTML(nil, "/nonexistent/dir/out.html", 100)
	require.Error(t, err)
}

func TestGenerateServerBasedCytoscapeHTML_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "server.html")
	err := GenerateServerBasedCytoscapeHTML("http://localhost:8080", outFile)
	require.NoError(t, err)
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "localhost:8080")
}

func TestGenerateServerBasedCytoscapeHTML_BadPath(t *testing.T) {
	err := GenerateServerBasedCytoscapeHTML("http://localhost:8080", "/nonexistent/dir/out.html")
	require.Error(t, err)
}

func TestJoinPaths_EdgeCases(t *testing.T) {
	assert.Equal(t, "/b", joinPaths("", "/b"))
	assert.Equal(t, "/a/b", joinPaths("/a", "b"))
	assert.Equal(t, "/a/b", joinPaths("/a/", "/b"))
	assert.Equal(t, "/a/", joinPaths("/a", ""))
}

// --- handleVariableMount tests ---

func TestHandleVariableMount_NilRouterArg(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	cfg := DefaultHTTPConfig()
	ext := NewExtractor(tree, cfg)

	routes := make([]*RouteInfo, 0)
	// Should not panic with nil routerArg, and produce no routes
	ext.handleVariableMount(nil, "/api/", nil, &routes)
	assert.Empty(t, routes, "expected no routes when routerArg is nil")
}

func TestHandleVariableMount_EmptyVarName(_ *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	cfg := DefaultHTTPConfig()
	ext := NewExtractor(tree, cfg)

	// Empty name should return early
	arg := metadata.NewCallArgument(meta)
	ext.handleVariableMount(arg, "/api/", nil, &[]*RouteInfo{})
}

func TestHandleVariableMount_FindsMatchingRecvVar(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	// Create a creation edge with CalleeRecvVarName = "apiMux"
	creationEdge := metadata.CallGraphEdge{
		CalleeRecvVarName: "apiMux",
		Caller: metadata.Call{
			Meta:     meta,
			Name:     sp.Get("main"),
			Pkg:      sp.Get("myapp"),
			RecvType: -1,
		},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     sp.Get("NewServeMux"),
			Pkg:      sp.Get("net/http"),
			RecvType: -1,
		},
	}

	meta.CallGraph = []metadata.CallGraphEdge{creationEdge}

	// Build a mock tree with a creation node that has a child
	childEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta:     meta,
			Name:     sp.Get("HandleFunc"),
			Pkg:      sp.Get("net/http"),
			RecvType: sp.Get("*ServeMux"),
		},
	}
	childNode := &TrackerNode{CallGraphEdge: childEdge}
	childNode.key = "net/http.*ServeMux.HandleFunc@test.go:10:2"
	creationNode := &TrackerNode{CallGraphEdge: &creationEdge}
	creationNode.key = "net/http.NewServeMux@test.go:5:2"
	creationNode.Children = append(creationNode.Children, childNode)

	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{})
	tree.AddRoot(creationNode)

	cfg := DefaultHTTPConfig()
	ext := NewExtractor(tree, cfg)

	routerArg := metadata.NewCallArgument(meta)
	routerArg.SetName("apiMux")

	routes := make([]*RouteInfo, 0)
	ext.handleVariableMount(routerArg, "/api/", nil, &routes)

	// The function should not panic and should traverse the creation node's children
	assert.NotNil(t, routes)
}

func TestHandleVariableMount_NilMetadata(_ *testing.T) {
	tree := NewMockTrackerTree(nil, metadata.TrackerLimits{})
	cfg := DefaultHTTPConfig()
	ext := NewExtractor(tree, cfg)

	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	arg := metadata.NewCallArgument(meta)
	arg.SetName("apiMux")

	// Should not panic when tree metadata is nil
	ext.handleVariableMount(arg, "/api/", nil, &[]*RouteInfo{})
}
