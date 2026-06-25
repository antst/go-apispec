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

// ---------------------------------------------------------------------------
// Test helpers specific to this file
// ---------------------------------------------------------------------------

// makeSelectorArg builds a selector CallArgument representing pkg.name
// (e.g., "http.StatusBadRequest"). The X sub-arg is an ident for the package,
// and the Sel sub-arg is an ident for the selector name.
func makeSelectorArg(meta *metadata.Metadata, xPkg, xName, selName string) metadata.CallArgument {
	x := metadata.NewCallArgument(meta)
	x.SetKind(metadata.KindIdent)
	x.SetName(xName)
	x.SetPkg(xPkg)

	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindIdent)
	sel.SetName(selName)

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X = x
	arg.Sel = sel
	return *arg
}

// makeHelperEdge creates a CallGraphEdge whose Callee has the given name/pkg
// and whose ParamArgMap is populated from the given map.
func makeHelperEdge(meta *metadata.Metadata, calleeName, calleePkg string, paramArgs map[string]metadata.CallArgument) metadata.CallGraphEdge {
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("main"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get(calleeName),
			Pkg:  meta.StringPool.Get(calleePkg),
		},
		ParamArgMap: paramArgs,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	return edge
}

// newTestExtractor creates an Extractor backed by a MockTrackerTree using the
// default chi configuration. The returned tree can have roots added to it.
func newTestExtractor(meta *metadata.Metadata) (*Extractor, *MockTrackerTree) {
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}
	tree := NewMockTrackerTree(meta, limits)
	cfg := DefaultChiConfig()
	ext := NewExtractor(tree, cfg)
	return ext, tree
}

// ===========================================================================
// 1. resolveParamArgStatus
// ===========================================================================

func TestResolveParamArgStatus_FoundInParent(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Build: grandparent → parent (with ParamArgMap["code" → StatusBadRequest]) → child
	statusArg := makeSelectorArg(meta, "net/http", "http", "StatusBadRequest")

	parentEdge := makeHelperEdge(meta, "writeJSONError", "helpers", map[string]metadata.CallArgument{
		"code": statusArg,
	})
	grandparent := &TrackerNode{key: "grandparent"}
	parent := &TrackerNode{
		key:           "parent",
		Parent:        grandparent,
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	// Get the response pattern matcher from the extractor
	require.NotEmpty(t, ext.responseMatchers, "expected at least one response matcher")
	rm, ok := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	require.True(t, ok)

	status, found := rm.resolveParamArgStatus(child, "code")
	assert.True(t, found)
	assert.Equal(t, 400, status)
}

func TestResolveParamArgStatus_NotFound(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	parentEdge := makeHelperEdge(meta, "helper", "pkg", map[string]metadata.CallArgument{
		"other": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.False(t, found)
	assert.Equal(t, 0, status)
}

func TestResolveParamArgStatus_NilParent(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	child := &TrackerNode{key: "child"}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.False(t, found)
	assert.Equal(t, 0, status)
}

func TestResolveParamArgStatus_ParentEdgeNil(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	parent := &TrackerNode{key: "parent"} // no edge
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.False(t, found)
	assert.Equal(t, 0, status)
}

func TestResolveParamArgStatus_EmptyParamArgMap(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	parentEdge := makeHelperEdge(meta, "helper", "pkg", map[string]metadata.CallArgument{})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.False(t, found)
	assert.Equal(t, 0, status)
}

func TestResolveParamArgStatus_UnresolvableStatus(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// ParamArgMap has an ident arg that doesn't resolve to a known status
	unknownArg := makeCallArg(meta)
	unknownArg.SetKind(metadata.KindIdent)
	unknownArg.SetName("someVar")
	unknownArg.SetType("int")

	parentEdge := makeHelperEdge(meta, "helper", "pkg", map[string]metadata.CallArgument{
		"code": *unknownArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.False(t, found)
	assert.Equal(t, 0, status)
}

func TestResolveParamArgStatus_GrandparentHasMapping(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Parent has no mapping, grandparent does
	statusArg := makeSelectorArg(meta, "net/http", "http", "StatusNotFound")

	grandparentEdge := makeHelperEdge(meta, "outerHelper", "pkg", map[string]metadata.CallArgument{
		"code": statusArg,
	})
	grandparent := &TrackerNode{
		key:           "grandparent",
		CallGraphEdge: &grandparentEdge,
	}
	parent := &TrackerNode{
		key:    "parent",
		Parent: grandparent,
		// no edge or empty ParamArgMap
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	status, found := rm.resolveParamArgStatus(child, "code")
	assert.True(t, found)
	assert.Equal(t, 404, status)
}

// ===========================================================================
// 2. resolveParamArgType
// ===========================================================================

func TestResolveParamArgType_FoundType(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("user")
	dataArg.SetType("User")
	dataArg.SetPkg("models")

	parentEdge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"data": *dataArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	assert.NotEmpty(t, result)
	assert.NotEqual(t, "interface{}", result)
}

func TestResolveParamArgType_SkipsInterface(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// arg has type "interface{}" — should be skipped
	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("data")
	dataArg.SetType("interface{}")

	parentEdge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"data": *dataArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	assert.Empty(t, result)
}

func TestResolveParamArgType_SkipsAny(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("data")
	dataArg.SetType("any")

	parentEdge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"data": *dataArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	assert.Empty(t, result)
}

func TestResolveParamArgType_NilParent(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	child := &TrackerNode{key: "child"}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	assert.Empty(t, result)
}

func TestResolveParamArgType_ParamNotFound(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("user")
	dataArg.SetType("User")
	dataArg.SetPkg("models")

	parentEdge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"something_else": *dataArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	assert.Empty(t, result)
}

func TestResolveParamArgType_FallbackToRawType(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Arg where GetArgumentInfo returns "" but GetType returns something useful.
	// This happens when pkg is empty and type is a simple name.
	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("u")
	dataArg.SetType("MyStruct")
	// No pkg set — GetArgumentInfo for ident with no pkg may return empty or just the type

	parentEdge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"data": *dataArg,
	})
	parent := &TrackerNode{
		key:           "parent",
		CallGraphEdge: &parentEdge,
	}
	child := &TrackerNode{
		key:    "child",
		Parent: parent,
	}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result, _ := rm.resolveParamArgType(child, "data")
	// Should get some type back (either via GetArgumentInfo or fallback)
	assert.NotEmpty(t, result)
	assert.NotEqual(t, "interface{}", result)
	assert.NotEqual(t, "any", result)
}

// ===========================================================================
// 3. resolveArgToStatusCode
// ===========================================================================

func TestResolveArgToStatusCode_KnownStatus(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	arg := makeSelectorArg(meta, "net/http", "http", "StatusBadRequest")
	status, ok := ext.resolveArgToStatusCode(&arg)
	assert.True(t, ok)
	assert.Equal(t, 400, status)
}

func TestResolveArgToStatusCode_LiteralNumber(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	arg := makeLiteralArg(meta, "201")
	status, ok := ext.resolveArgToStatusCode(arg)
	assert.True(t, ok)
	assert.Equal(t, 201, status)
}

func TestResolveArgToStatusCode_UnknownString(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("someVariable")
	arg.SetType("int")
	status, ok := ext.resolveArgToStatusCode(arg)
	assert.False(t, ok)
	assert.Equal(t, 0, status)
}

func TestResolveArgToStatusCode_StatusOK(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	arg := makeSelectorArg(meta, "net/http", "http", "StatusOK")
	status, ok := ext.resolveArgToStatusCode(&arg)
	assert.True(t, ok)
	assert.Equal(t, 200, status)
}

func TestResolveArgToStatusCode_StatusInternalServerError(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	arg := makeSelectorArg(meta, "net/http", "http", "StatusInternalServerError")
	status, ok := ext.resolveArgToStatusCode(&arg)
	assert.True(t, ok)
	assert.Equal(t, 500, status)
}

// ===========================================================================
// 4. collectHelperCallGroups
// ===========================================================================

func TestCollectHelperCallGroups_BasicGrouping(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Two children with same callee BaseID (same name/pkg), different ParamArgMaps
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	edge2 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
	})

	root := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1},
			{key: "call2", CallGraphEdge: &edge2},
		},
	}

	groups := ext.collectHelperCallGroups(root)
	// Both edges have the same callee "helpers-->respondJSON", so 1 group with 2 calls
	assert.Len(t, groups, 1)
	for _, calls := range groups {
		assert.Len(t, calls, 2)
	}
}

func TestCollectHelperCallGroups_DifferentCallees(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	edge2 := makeHelperEdge(meta, "respondError", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
	})

	root := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1},
			{key: "call2", CallGraphEdge: &edge2},
		},
	}

	groups := ext.collectHelperCallGroups(root)
	assert.Len(t, groups, 2)
}

func TestCollectHelperCallGroups_SkipsEdgesWithoutParamArgMap(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Edge with no ParamArgMap
	edgeNoMap := makeEdge(meta, "handler", "main", "fmt.Println", "fmt", nil)

	root := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edgeNoMap},
		},
	}

	groups := ext.collectHelperCallGroups(root)
	assert.Empty(t, groups)
}

func TestCollectHelperCallGroups_EmptyChildren(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	root := &TrackerNode{
		key: "route",
	}

	groups := ext.collectHelperCallGroups(root)
	assert.Empty(t, groups)
}

func TestCollectHelperCallGroups_NestedChildren(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Nested: root → child1 → grandchild1 (same callee as child2)
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	edge2 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusNotFound"),
	})

	grandchild := &TrackerNode{key: "grandchild", CallGraphEdge: &edge2}
	root := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{
				key:           "call1",
				CallGraphEdge: &edge1,
				Children:      []*TrackerNode{grandchild},
			},
		},
	}

	groups := ext.collectHelperCallGroups(root)
	assert.Len(t, groups, 1)
	for _, calls := range groups {
		assert.Len(t, calls, 2)
	}
}

// ===========================================================================
// 5. findStatusParamAndSchema
// ===========================================================================

func TestFindStatusParamAndSchema_Found(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	statusArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": statusArg,
		"data": *makeIdentArg(meta, "user", "User"),
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge1},
	}

	// Route must have a response for status 200 with a schema
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
		Metadata: meta,
	}

	paramName, schema, contentType := ext.findStatusParamAndSchema(calls, route)
	assert.Equal(t, "code", paramName)
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, "application/json", contentType)
}

func TestFindStatusParamAndSchema_NoMatchingResponse(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	statusArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": statusArg,
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge1},
	}

	// Route has no response for 200
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	paramName, schema, _ := ext.findStatusParamAndSchema(calls, route)
	assert.Empty(t, paramName)
	assert.Nil(t, schema)
}

func TestFindStatusParamAndSchema_ResponseWithoutSchema(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	statusArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": statusArg,
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge1},
	}

	// Route has response for 200 but without schema
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      nil,
			},
		},
		Metadata: meta,
	}

	paramName, schema, _ := ext.findStatusParamAndSchema(calls, route)
	assert.Empty(t, paramName)
	assert.Nil(t, schema)
}

func TestFindStatusParamAndSchema_MultipleCallsPicksFirst(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	edge2 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge1},
		{node: &TrackerNode{key: "call2"}, edge: &edge2},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
		Metadata: meta,
	}

	paramName, schema, _ := ext.findStatusParamAndSchema(calls, route)
	assert.Equal(t, "code", paramName)
	assert.NotNil(t, schema)
}

// ===========================================================================
// 6. findBodyParamName
// ===========================================================================

func TestFindBodyParamName_FindsDataParam(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("user")
	dataArg.SetType("User")
	dataArg.SetPkg("models")

	wArg := makeCallArg(meta)
	wArg.SetKind(metadata.KindIdent)
	wArg.SetName("w")
	wArg.SetType("http.ResponseWriter")

	codeArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")

	edge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"w":    *wArg,
		"code": codeArg,
		"data": *dataArg,
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	bodyParam := ext.findBodyParamName(calls, route, "code", &Schema{Type: "object"})
	assert.Equal(t, "data", bodyParam)
}

func TestFindBodyParamName_SkipsWriter(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	wArg := makeCallArg(meta)
	wArg.SetKind(metadata.KindIdent)
	wArg.SetName("w")
	wArg.SetType("http.ResponseWriter")

	codeArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")

	edge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"w":      *wArg,
		"code":   codeArg,
		"writer": *wArg, // also skipped
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	bodyParam := ext.findBodyParamName(calls, route, "code", &Schema{Type: "object"})
	assert.Empty(t, bodyParam)
}

func TestFindBodyParamName_SkipsLiteralParam(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	literalArg := makeLiteralArg(meta, "something went wrong")
	codeArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")

	edge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": codeArg,
		"msg":  *literalArg,
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	bodyParam := ext.findBodyParamName(calls, route, "code", &Schema{Type: "object"})
	assert.Empty(t, bodyParam)
}

func TestFindBodyParamName_EmptyCalls(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	bodyParam := ext.findBodyParamName(nil, route, "code", &Schema{Type: "object"})
	assert.Empty(t, bodyParam)
}

func TestFindBodyParamName_SkipsStatusCodeParam(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// All params are either "code" (status param) or resolve to status codes
	codeArg := makeSelectorArg(meta, "net/http", "http", "StatusOK")
	anotherCode := makeSelectorArg(meta, "net/http", "http", "StatusBadRequest")

	edge := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code":    codeArg,
		"altCode": anotherCode, // also resolves to a status code → skipped
	})

	calls := []helperCall{
		{node: &TrackerNode{key: "call1"}, edge: &edge},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	bodyParam := ext.findBodyParamName(calls, route, "code", &Schema{Type: "object"})
	assert.Empty(t, bodyParam)
}

// ===========================================================================
// 7. expandHelperFunctionResponses
// ===========================================================================

func TestExpandHelperFunctionResponses_AddsNewResponses(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Two calls to the same helper with different status codes
	dataArg1 := makeCallArg(meta)
	dataArg1.SetKind(metadata.KindIdent)
	dataArg1.SetName("user")
	dataArg1.SetType("User")
	dataArg1.SetPkg("models")

	dataArg2 := makeCallArg(meta)
	dataArg2.SetKind(metadata.KindIdent)
	dataArg2.SetName("errResp")
	dataArg2.SetType("ErrorResponse")
	dataArg2.SetPkg("models")

	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
		"data": *dataArg1,
	})
	edge2 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
		"data": *dataArg2,
	})

	// Helper nodes need a child that matches a response pattern (WriteHeader)
	// so helperContainsResponsePattern returns true.
	writeHeaderEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
		},
	}
	whChild := &TrackerNode{key: "net/http.ResponseWriter.WriteHeader", CallGraphEdge: writeHeaderEdge}

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1, Children: []*TrackerNode{whChild}},
			{key: "call2", CallGraphEdge: &edge2, Children: []*TrackerNode{whChild}},
		},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	ext.expandHelperFunctionResponses(routeNode, route, nil)

	// Should have added a 400 response
	assert.Contains(t, route.Response, "400")
}

func TestExpandHelperFunctionResponses_SkipsSingleCall(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1},
		},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
		Metadata: meta,
	}

	initialCount := len(route.Response)
	ext.expandHelperFunctionResponses(routeNode, route, nil)
	assert.Equal(t, initialCount, len(route.Response), "should not add responses for single-call groups")
}

func TestExpandHelperFunctionResponses_SkipsExistingStatus(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Two calls to the same helper, both status codes already have responses
	edge1 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	edge2 := makeHelperEdge(meta, "respondJSON", "helpers", map[string]metadata.CallArgument{
		"code": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
	})

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1},
			{key: "call2", CallGraphEdge: &edge2},
		},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {
				StatusCode:  200,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
			"400": {
				StatusCode:  400,
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
		Metadata: meta,
	}

	initialCount := len(route.Response)
	ext.expandHelperFunctionResponses(routeNode, route, nil)
	assert.Equal(t, initialCount, len(route.Response), "should not overwrite existing responses")
}

func TestExpandHelperFunctionResponses_NoChildren(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	routeNode := &TrackerNode{key: "route"}
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	ext.expandHelperFunctionResponses(routeNode, route, nil)
	assert.Empty(t, route.Response)
}

func TestExpandHelperFunctionResponses_NoStatusParamFound(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	// Two calls to the same helper but params don't resolve to status codes
	unknownArg := makeCallArg(meta)
	unknownArg.SetKind(metadata.KindIdent)
	unknownArg.SetName("x")
	unknownArg.SetType("int")

	edge1 := makeHelperEdge(meta, "doSomething", "helpers", map[string]metadata.CallArgument{
		"x": *unknownArg,
	})
	edge2 := makeHelperEdge(meta, "doSomething", "helpers", map[string]metadata.CallArgument{
		"x": *unknownArg,
	})

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "call1", CallGraphEdge: &edge1},
			{key: "call2", CallGraphEdge: &edge2},
		},
	}

	route := &RouteInfo{
		Response: map[string]*ResponseInfo{},
		Metadata: meta,
	}

	ext.expandHelperFunctionResponses(routeNode, route, nil)
	assert.Empty(t, route.Response)
}

// ===========================================================================
// 8. inferStatusParamFromCalls (issue #27 fallback path)
// ===========================================================================

func TestInferStatusParamFromCalls_PicksStatusArg(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	bodyArg := makeCallArg(meta)
	bodyArg.SetKind(metadata.KindIdent)
	bodyArg.SetName("v")
	bodyArg.SetType("interface{}")

	edge := makeHelperEdge(meta, "WriteJSON", "helpers", map[string]metadata.CallArgument{
		"w":      {Meta: meta},
		"status": makeSelectorArg(meta, "net/http", "http", "StatusCreated"),
		"v":      *bodyArg,
	})
	calls := []helperCall{{node: &TrackerNode{key: "c1", CallGraphEdge: &edge}, edge: &edge}}

	got := ext.inferStatusParamFromCalls(calls)
	assert.Equal(t, "status", got)
}

func TestInferStatusParamFromCalls_NoStatusArg(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	dataArg := makeCallArg(meta)
	dataArg.SetKind(metadata.KindIdent)
	dataArg.SetName("payload")
	dataArg.SetType("User")
	dataArg.SetPkg("models")

	edge := makeHelperEdge(meta, "Send", "helpers", map[string]metadata.CallArgument{
		"data": *dataArg,
	})
	calls := []helperCall{{node: &TrackerNode{key: "c1", CallGraphEdge: &edge}, edge: &edge}}

	got := ext.inferStatusParamFromCalls(calls)
	assert.Empty(t, got)
}

// ===========================================================================
// 9. helperFallbackEdges (issue #27 — defensive write filter)
// ===========================================================================

// fallbackTestRig builds a route → WriteJSON helper tree where the helper has
// both an unconditional WriteHeader (status param) and a conditional
// WriteHeader(500) inside an `if err != nil { ... }` branch. Returns the
// route node and the conditional edge for assertions.
func fallbackTestRig(meta *metadata.Metadata) (*TrackerNode, *metadata.CallGraphEdge) {
	// The write edges live in WriteJSON's body, so their Caller is WriteJSON
	// (classifyHelperWrites scopes collection to the host callee via sameFunc).
	inHelper := metadata.Call{Meta: meta, Name: meta.StringPool.Get("WriteJSON"), Pkg: meta.StringPool.Get("helpers")}
	condWHEdge := &metadata.CallGraphEdge{
		Caller: inHelper,
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("file.go:10:5"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
	}
	succWHEdge := &metadata.CallGraphEdge{
		Caller: inHelper,
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("file.go:20:5"),
		},
	}

	helperEdge := makeHelperEdge(meta, "WriteJSON", "helpers", map[string]metadata.CallArgument{
		"status": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
	})
	helperNode := &TrackerNode{
		key:           "helper",
		CallGraphEdge: &helperEdge,
		Children: []*TrackerNode{
			{key: "wh-cond", CallGraphEdge: condWHEdge},
			{key: "wh-succ", CallGraphEdge: succWHEdge},
		},
	}
	routeNode := &TrackerNode{
		key:      "route",
		Children: []*TrackerNode{helperNode},
	}
	return routeNode, condWHEdge
}

func TestHelperFallbackEdges_FlagsConditionalWhenUnconditionalExists(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	routeNode, condEdge := fallbackTestRig(meta)

	fallback := ext.helperFallbackEdges(routeNode)
	assert.True(t, fallback[condEdge.Callee.ID()],
		"conditional write inside a helper that also has an unconditional write must be flagged as a fallback")
}

func TestHelperFallbackEdges_NoUnconditional_KeepsAll(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	inHelper := metadata.Call{Meta: meta, Name: meta.StringPool.Get("WriteEither"), Pkg: meta.StringPool.Get("helpers")}
	condWH1 := &metadata.CallGraphEdge{
		Caller: inHelper,
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("f.go:1:1"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
	}
	condWH2 := &metadata.CallGraphEdge{
		Caller: inHelper,
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("f.go:2:1"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-else"},
	}
	helperEdge := makeHelperEdge(meta, "WriteEither", "helpers", map[string]metadata.CallArgument{
		"x": {Meta: meta},
	})
	helperNode := &TrackerNode{
		key:           "helper",
		CallGraphEdge: &helperEdge,
		Children: []*TrackerNode{
			{key: "a", CallGraphEdge: condWH1},
			{key: "b", CallGraphEdge: condWH2},
		},
	}
	routeNode := &TrackerNode{key: "route", Children: []*TrackerNode{helperNode}}

	fallback := ext.helperFallbackEdges(routeNode)
	assert.Empty(t, fallback, "no unconditional write → nothing to flag")
}

func TestHelperFallbackEdges_SkipsRouteNode(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	cond := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("h.go:1:1"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
	}
	uncond := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("h.go:2:1"),
		},
	}
	routeOwnEdge := makeHelperEdge(meta, "HandleFunc", "net/http", map[string]metadata.CallArgument{
		"path": {Meta: meta},
	})
	routeNode := &TrackerNode{
		key:           "route",
		CallGraphEdge: &routeOwnEdge,
		Children: []*TrackerNode{
			{key: "c", CallGraphEdge: cond},
			{key: "u", CallGraphEdge: uncond},
		},
	}

	fallback := ext.helperFallbackEdges(routeNode)
	assert.Empty(t, fallback,
		"branches at the route level are handler control flow, not fallbacks — must not flag")
}

func TestHelperFallbackEdges_SkipsResponsePrimitives(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	statusEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("p.go:1:1"),
		},
		ParamArgMap: map[string]metadata.CallArgument{
			"statusCode": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
		},
	}

	condChild := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("Write"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("p.go:2:1"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
	}
	uncondChild := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("Write"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("p.go:3:1"),
		},
	}
	primitive := &TrackerNode{
		key:           "primitive",
		CallGraphEdge: statusEdge,
		Children: []*TrackerNode{
			{key: "wc", CallGraphEdge: condChild},
			{key: "wu", CallGraphEdge: uncondChild},
		},
	}
	routeNode := &TrackerNode{key: "route", Children: []*TrackerNode{primitive}}

	fallback := ext.helperFallbackEdges(routeNode)
	assert.Empty(t, fallback,
		"response-pattern primitives are not user-defined helpers — must not classify their children")
}

func TestHelperFallbackEdges_NilRoute(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	assert.Empty(t, ext.helperFallbackEdges(nil))
}

// ===========================================================================
// 10. expandHelperFunctionResponses — inferStatusParam fallback (issue #27)
// ===========================================================================

// When the primary response pass skipped a helper's status (the body
// argument bottomed out at "invalid type" so the schema seed is missing),
// expandHelperFunctionResponses must still populate per-status schemas from
// each call's caller arg via the inferStatusParamFromCalls fallback path.
func TestExpandHelperFunctionResponses_InferStatusParamFromCalls(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	bodyArg1 := makeCallArg(meta)
	bodyArg1.SetKind(metadata.KindIdent)
	bodyArg1.SetName("user")
	bodyArg1.SetType("User")
	bodyArg1.SetPkg("models")

	bodyArg2 := makeCallArg(meta)
	bodyArg2.SetKind(metadata.KindIdent)
	bodyArg2.SetName("errResp")
	bodyArg2.SetType("ErrorResponse")
	bodyArg2.SetPkg("models")

	edge1 := makeHelperEdge(meta, "WriteJSON", "helpers", map[string]metadata.CallArgument{
		"status": makeSelectorArg(meta, "net/http", "http", "StatusOK"),
		"v":      *bodyArg1,
	})
	edge2 := makeHelperEdge(meta, "WriteJSON", "helpers", map[string]metadata.CallArgument{
		"status": makeSelectorArg(meta, "net/http", "http", "StatusBadRequest"),
		"v":      *bodyArg2,
	})

	// Helper must contain a response-pattern child for the expansion to fire.
	writeHeaderEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
		},
	}
	whChild := &TrackerNode{key: "wh", CallGraphEdge: writeHeaderEdge}

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "c1", CallGraphEdge: &edge1, Children: []*TrackerNode{whChild}},
			{key: "c2", CallGraphEdge: &edge2, Children: []*TrackerNode{whChild}},
		},
	}

	// route.Response intentionally EMPTY — primary pass found nothing to seed.
	route := &RouteInfo{
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	ext.expandHelperFunctionResponses(routeNode, route, nil)

	// Both status codes should now be populated from per-call body args.
	assert.Contains(t, route.Response, "200",
		"200 should be filled from caller's user arg via inferStatusParamFromCalls fallback")
	assert.Contains(t, route.Response, "400",
		"400 should be filled from caller's errResp arg via inferStatusParamFromCalls fallback")
	if r := route.Response["200"]; r != nil {
		assert.NotEmpty(t, r.ContentType,
			"contentType must fall back to defaults when no seed exists")
	}
}

// ===========================================================================
// 11. expandHelperFunctionResponses — fallback edge filter (issue #27)
// ===========================================================================

func TestExpandHelperFunctionResponses_FallbackEdgesFiltered(t *testing.T) {
	// A WriteHeader "helper" group containing only fallback (if-then) edges
	// should not synthesize a phantom response on the caller.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	fallback500Edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("h.go:10:5"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
		ParamArgMap: map[string]metadata.CallArgument{
			"statusCode": makeSelectorArg(meta, "net/http", "http", "StatusInternalServerError"),
		},
	}
	fallback500Edge2 := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta, Name: meta.StringPool.Get("WriteHeader"),
			Pkg: meta.StringPool.Get("net/http"), RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get("h.go:11:5"),
		},
		Branch: &metadata.BranchContext{BlockKind: "if-then"},
		ParamArgMap: map[string]metadata.CallArgument{
			"statusCode": makeSelectorArg(meta, "net/http", "http", "StatusInternalServerError"),
		},
	}

	routeNode := &TrackerNode{
		key: "route",
		Children: []*TrackerNode{
			{key: "wh1", CallGraphEdge: fallback500Edge, Children: []*TrackerNode{
				{key: "wh1-child", CallGraphEdge: fallback500Edge},
			}},
			{key: "wh2", CallGraphEdge: fallback500Edge2, Children: []*TrackerNode{
				{key: "wh2-child", CallGraphEdge: fallback500Edge2},
			}},
		},
	}
	route := &RouteInfo{Response: map[string]*ResponseInfo{}, Metadata: meta}
	fbEdges := map[string]bool{
		fallback500Edge.Callee.ID():  true,
		fallback500Edge2.Callee.ID(): true,
	}

	ext.expandHelperFunctionResponses(routeNode, route, fbEdges)
	_, has500 := route.Response["500"]
	assert.False(t, has500, "all calls in the group were filtered as fallback edges — must NOT add 500")
}

// ===========================================================================
// 12. ExtractResponse — "invalid type" sentinel detection (issue #27)
// ===========================================================================

// When the body argument's resolved type bottoms out at go/types' "invalid
// type" sentinel (e.g., `Write(append(data, '\n'))` where `append` is a
// builtin without a defined type), ExtractResponse must clear the body type
// so it can't fabricate a `$ref` to a non-existent schema name and so
// expandHelperFunctionResponses can fill the schema from the caller's
// ParamArgMap arg instead.
func TestExtractResponse_InvalidTypeSentinel_ClearsBody(t *testing.T) {
	meta := newTestMeta()

	// Argument that mimics go/types' fallback for an unresolvable builtin:
	// a KindCall arg whose name string contains "invalid type".
	bodyArg := metadata.NewCallArgument(meta)
	bodyArg.SetKind(metadata.KindCall)
	bodyArg.SetName("invalid type")
	bodyArg.SetPkg("mypkg")
	// arg.Fun is required so callArgToString doesn't bail to "call(...)".
	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindIdent)
	funArg.SetName("append")
	bodyArg.Fun = funArg

	edge := makeEdge(meta, "handler", "main", "Write", "http", []*metadata.CallArgument{bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus: 200,
		TypeFromArg:   true,
		TypeArgIndex:  0,
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
	resp := firstResponse(matcher.ExtractResponse(node, route))

	require.NotNil(t, resp)
	assert.Empty(t, resp.BodyType,
		"`invalid type` sentinel must clear bodyType — otherwise a `$ref` to "+
			"`<pkg>.invalid-type` would leak into the spec (issue #27)")
}
