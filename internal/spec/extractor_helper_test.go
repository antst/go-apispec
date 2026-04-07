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
	result := rm.resolveParamArgType(child, "data")
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
	result := rm.resolveParamArgType(child, "data")
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
	result := rm.resolveParamArgType(child, "data")
	assert.Empty(t, result)
}

func TestResolveParamArgType_NilParent(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	child := &TrackerNode{key: "child"}

	rm := ext.responseMatchers[0].(*ResponsePatternMatcherImpl)
	result := rm.resolveParamArgType(child, "data")
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
	result := rm.resolveParamArgType(child, "data")
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
	result := rm.resolveParamArgType(child, "data")
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
		},
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	ext.expandHelperFunctionResponses(routeNode, route)

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
	ext.expandHelperFunctionResponses(routeNode, route)
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
	ext.expandHelperFunctionResponses(routeNode, route)
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

	ext.expandHelperFunctionResponses(routeNode, route)
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

	ext.expandHelperFunctionResponses(routeNode, route)
	assert.Empty(t, route.Response)
}
