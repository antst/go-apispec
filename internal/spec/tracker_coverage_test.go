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

// helper to create a simple metadata + string pool for tracker tests
func newTrackerTestMeta() (*metadata.Metadata, *metadata.StringPool) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}
	return meta, sp
}

func defaultTrackerLimits() metadata.TrackerLimits {
	return metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 100,
		MaxArgsPerFunction: 50,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  5,
	}
}

// ---------------------------------------------------------------------------
// 1. NewTrackerTree basic creation
// ---------------------------------------------------------------------------

func TestNewTrackerTree_EmptyMeta(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	limits := defaultTrackerLimits()
	tree := NewTrackerTree(meta, limits)

	require.NotNil(t, tree)
	assert.Equal(t, 0, tree.GetNodeCount())
	assert.NotNil(t, tree.GetMetadata())
	assert.Equal(t, meta, tree.GetMetadata())
}

func TestNewTrackerTree_NilStringPool(t *testing.T) {
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}
	limits := defaultTrackerLimits()
	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
}

// ---------------------------------------------------------------------------
// 2. TrackerNode accessors
// ---------------------------------------------------------------------------

func TestTrackerNode_GetKey_ManualKey(t *testing.T) {
	nd := &TrackerNode{key: "mykey"}
	assert.Equal(t, "mykey", nd.GetKey())
}

func TestTrackerNode_GetKey_EmptyNode(t *testing.T) {
	nd := &TrackerNode{}
	// With no key, no CallArgument, no CallGraphEdge => key stays empty
	assert.Equal(t, "", nd.GetKey())
}

func TestTrackerNode_GetKey_FromCallGraphEdge(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get("handler"),
			Pkg:  sp.Get("main"),
		},
		Caller: metadata.Call{
			Meta: meta,
			Name: sp.Get("caller"),
			Pkg:  sp.Get("main"),
		},
	}
	nd := &TrackerNode{}
	nd.CallGraphEdge = edge
	key := nd.GetKey()
	assert.NotEmpty(t, key)
	assert.Contains(t, key, "handler")
}

func TestTrackerNode_GetParent_Nil(t *testing.T) {
	nd := &TrackerNode{}
	assert.Nil(t, nd.GetParent())
}

func TestTrackerNode_GetParent_NonNil(t *testing.T) {
	parent := &TrackerNode{key: "parent"}
	child := &TrackerNode{key: "child", Parent: parent}
	result := child.GetParent()
	require.NotNil(t, result)
	assert.Equal(t, "parent", result.GetKey())
}

func TestTrackerNode_GetChildren_Empty(t *testing.T) {
	nd := &TrackerNode{}
	children := nd.GetChildren()
	assert.Empty(t, children)
}

func TestTrackerNode_GetChildren_WithChildren(t *testing.T) {
	child1 := &TrackerNode{key: "c1"}
	child2 := &TrackerNode{key: "c2"}
	nd := &TrackerNode{key: "parent", Children: []*TrackerNode{child1, child2}}
	children := nd.GetChildren()
	assert.Len(t, children, 2)
}

func TestTrackerNode_GetEdge_Nil(t *testing.T) {
	nd := &TrackerNode{}
	assert.Nil(t, nd.GetEdge())
}

func TestTrackerNode_GetEdge_NonNil(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
	}
	nd := &TrackerNode{}
	nd.CallGraphEdge = edge
	assert.Equal(t, edge, nd.GetEdge())
}

func TestTrackerNode_GetArgument_Nil(t *testing.T) {
	nd := &TrackerNode{}
	assert.Nil(t, nd.GetArgument())
}

func TestTrackerNode_GetArgument_NonNil(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")
	nd := &TrackerNode{}
	nd.CallArgument = arg
	assert.Equal(t, arg, nd.GetArgument())
}

func TestTrackerNode_GetArgType_AllTypes(t *testing.T) {
	tests := []struct {
		localType    ArgumentType
		expectedMeta metadata.ArgumentType
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
		nd := &TrackerNode{ArgType: tt.localType}
		assert.Equal(t, tt.expectedMeta, nd.GetArgType(), "for ArgType %v", tt.localType)
	}
}

func TestTrackerNode_GetArgType_Default(t *testing.T) {
	// Use an out-of-range ArgumentType to hit default case
	nd := &TrackerNode{ArgType: ArgumentType(999)}
	assert.Equal(t, metadata.ArgTypeComplex, nd.GetArgType())
}

func TestTrackerNode_GetArgIndex(t *testing.T) {
	nd := &TrackerNode{ArgIndex: 3}
	assert.Equal(t, 3, nd.GetArgIndex())
}

func TestTrackerNode_GetArgContext(t *testing.T) {
	nd := &TrackerNode{ArgContext: "myFunc"}
	assert.Equal(t, "myFunc", nd.GetArgContext())
}

func TestTrackerNode_GetTypeParamMap_Empty(t *testing.T) {
	nd := &TrackerNode{}
	params := nd.GetTypeParamMap()
	assert.NotNil(t, params)
	assert.Empty(t, params)
}

func TestTrackerNode_GetTypeParamMap_FromEdge(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "string"},
	}
	nd := &TrackerNode{}
	nd.CallGraphEdge = edge
	params := nd.GetTypeParamMap()
	assert.Equal(t, "string", params["T"])
}

func TestTrackerNode_GetRootAssignmentMap_Nil(t *testing.T) {
	nd := &TrackerNode{}
	assert.Nil(t, nd.GetRootAssignmentMap())
}

func TestTrackerNode_GetRootAssignmentMap_NonNil(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	assignments := map[string][]metadata.Assignment{
		"x": {
			{
				VariableName: sp.Get("x"),
				Pkg:          sp.Get("main"),
			},
		},
	}
	_ = meta // ensure meta is used
	nd := &TrackerNode{RootAssignmentMap: assignments}
	result := nd.GetRootAssignmentMap()
	require.NotNil(t, result)
	assert.Len(t, result["x"], 1)
}

// ---------------------------------------------------------------------------
// 3. ArgumentType String
// ---------------------------------------------------------------------------

func TestArgumentType_String(t *testing.T) {
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
		{ArgumentType(999), "Unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.at.String(), "for type %d", tt.at)
	}
}

// ---------------------------------------------------------------------------
// 4. AddChild / AddChildren
// ---------------------------------------------------------------------------

func TestTrackerNode_AddChild(t *testing.T) {
	parent := &TrackerNode{key: "parent"}
	child := &TrackerNode{key: "child"}

	parent.AddChild(child)

	assert.Len(t, parent.Children, 1)
	assert.Equal(t, parent, child.Parent)
}

func TestTrackerNode_AddChild_Reparent(t *testing.T) {
	parent1 := &TrackerNode{key: "parent1"}
	parent2 := &TrackerNode{key: "parent2"}
	child := &TrackerNode{key: "child"}

	parent1.AddChild(child)
	assert.Len(t, parent1.Children, 1)

	parent2.AddChild(child)
	assert.Equal(t, parent2, child.Parent)
	assert.Len(t, parent2.Children, 1)
	// Old parent should have no children after detach
	assert.Len(t, parent1.Children, 0)
}

func TestTrackerNode_AddChildren_Multiple(t *testing.T) {
	parent := &TrackerNode{key: "parent"}
	c1 := &TrackerNode{key: "c1"}
	c2 := &TrackerNode{key: "c2"}
	c3 := &TrackerNode{key: "c3"}

	parent.AddChildren([]*TrackerNode{c1, c2, c3})

	assert.Len(t, parent.Children, 3)
	assert.Equal(t, parent, c1.Parent)
	assert.Equal(t, parent, c2.Parent)
	assert.Equal(t, parent, c3.Parent)
}

func TestDetachChild(t *testing.T) {
	parent := &TrackerNode{key: "p"}
	c1 := &TrackerNode{key: "c1"}
	c2 := &TrackerNode{key: "c2"}

	parent.AddChildren([]*TrackerNode{c1, c2})
	assert.Len(t, parent.Children, 2)

	detachChild(c1)
	assert.Len(t, parent.Children, 1)
	assert.Equal(t, "c2", parent.Children[0].key)
}

func TestDetachChild_SingleChild(t *testing.T) {
	parent := &TrackerNode{key: "p"}
	child := &TrackerNode{key: "c"}

	parent.AddChild(child)
	assert.Len(t, parent.Children, 1)

	detachChild(child)
	assert.Len(t, parent.Children, 0)
}

func TestDetachChild_NilParent(_ *testing.T) {
	child := &TrackerNode{key: "c"}
	// Should not panic
	detachChild(child)
}

// ---------------------------------------------------------------------------
// 5. TrackerTree — FindNodeByKey
// ---------------------------------------------------------------------------

func TestTrackerTree_FindNodeByKey_Found(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	limits := defaultTrackerLimits()

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 limits,
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	child := &TrackerNode{key: "target"}
	root := &TrackerNode{key: "root", Children: []*TrackerNode{child}}
	child.Parent = root
	tree.roots = []*TrackerNode{root}

	found := tree.FindNodeByKey("target")
	require.NotNil(t, found)
	assert.Equal(t, "target", found.GetKey())
}

func TestTrackerTree_FindNodeByKey_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	limits := defaultTrackerLimits()

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 limits,
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	root := &TrackerNode{key: "root"}
	tree.roots = []*TrackerNode{root}

	found := tree.FindNodeByKey("nonexistent")
	assert.Nil(t, found)
}

func TestTrackerTree_FindNodeByKey_EmptyTree(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	limits := defaultTrackerLimits()

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 limits,
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	found := tree.FindNodeByKey("anything")
	assert.Nil(t, found)
}

func TestTrackerTree_FindNodeByKey_Root(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	root := &TrackerNode{key: "root"}
	tree.roots = []*TrackerNode{root}

	found := tree.FindNodeByKey("root")
	require.NotNil(t, found)
	assert.Equal(t, "root", found.GetKey())
}

// ---------------------------------------------------------------------------
// 6. TrackerTree — TraverseTree
// ---------------------------------------------------------------------------

func TestTrackerTree_TraverseTree_CountNodes(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	c1 := &TrackerNode{key: "c1"}
	c2 := &TrackerNode{key: "c2"}
	root := &TrackerNode{key: "root", Children: []*TrackerNode{c1, c2}}
	tree.roots = []*TrackerNode{root}

	var count int
	tree.TraverseTree(func(_ TrackerNodeInterface) bool {
		count++
		return true
	})
	assert.Equal(t, 3, count)
}

func TestTrackerTree_TraverseTree_EarlyStop(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	c1 := &TrackerNode{key: "c1"}
	c2 := &TrackerNode{key: "c2"}
	root := &TrackerNode{key: "root", Children: []*TrackerNode{c1, c2}}
	tree.roots = []*TrackerNode{root}

	var visited []string
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		visited = append(visited, node.GetKey())
		// Stop after visiting root
		return node.GetKey() != "root"
	})
	assert.Equal(t, []string{"root"}, visited)
}

func TestTrackerTree_TraverseTree_EmptyTree(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	var count int
	tree.TraverseTree(func(_ TrackerNodeInterface) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// 7. TrackerTree — GetNodeCount
// ---------------------------------------------------------------------------

func TestTrackerTree_GetNodeCount_Empty(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	assert.Equal(t, 0, tree.GetNodeCount())
}

func TestTrackerTree_GetNodeCount_WithNodes(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	grandchild := &TrackerNode{key: "gc"}
	child := &TrackerNode{key: "c", Children: []*TrackerNode{grandchild}}
	root := &TrackerNode{key: "r", Children: []*TrackerNode{child}}
	tree.roots = []*TrackerNode{root}

	assert.Equal(t, 3, tree.GetNodeCount())
}

func TestTrackerTree_GetNodeCount_MultipleRoots(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	root1 := &TrackerNode{key: "r1"}
	root2 := &TrackerNode{key: "r2", Children: []*TrackerNode{{key: "c"}}}
	tree.roots = []*TrackerNode{root1, root2}

	assert.Equal(t, 3, tree.GetNodeCount())
}

// ---------------------------------------------------------------------------
// 8. TrackerTree — GetMetadata, GetLimits
// ---------------------------------------------------------------------------

func TestTrackerTree_GetMetadata(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	assert.Equal(t, meta, tree.GetMetadata())
}

func TestTrackerTree_GetLimits(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    500,
		MaxChildrenPerNode: 50,
		MaxArgsPerFunction: 25,
		MaxNestedArgsDepth: 5,
		MaxRecursionDepth:  3,
	}
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 limits,
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	got := tree.GetLimits()
	assert.Equal(t, 500, got.MaxNodesPerTree)
	assert.Equal(t, 50, got.MaxChildrenPerNode)
	assert.Equal(t, 25, got.MaxArgsPerFunction)
	assert.Equal(t, 5, got.MaxNestedArgsDepth)
}

// ---------------------------------------------------------------------------
// 9. TrackerTree — GetFunctionContext
// ---------------------------------------------------------------------------

func TestTrackerTree_GetFunctionContext_Found(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	fn := &metadata.Function{
		Name: sp.Get("myHandler"),
		Pkg:  sp.Get("handlers"),
	}
	meta.Packages["handlers"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"handler.go": {
				Functions: map[string]*metadata.Function{
					"myHandler": fn,
				},
			},
		},
	}

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	gotFn, gotPkg, gotFile := tree.GetFunctionContext("myHandler")
	require.NotNil(t, gotFn)
	assert.Equal(t, fn, gotFn)
	assert.Equal(t, "handlers", gotPkg)
	assert.Equal(t, "handler.go", gotFile)
}

func TestTrackerTree_GetFunctionContext_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	meta.Packages["main"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"main.go": {
				Functions: map[string]*metadata.Function{},
			},
		},
	}

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	gotFn, gotPkg, gotFile := tree.GetFunctionContext("nonexistent")
	assert.Nil(t, gotFn)
	assert.Equal(t, "", gotPkg)
	assert.Equal(t, "", gotFile)
}

func TestTrackerTree_GetFunctionContext_EmptyName(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	gotFn, gotPkg, gotFile := tree.GetFunctionContext("")
	assert.Nil(t, gotFn)
	assert.Equal(t, "", gotPkg)
	assert.Equal(t, "", gotFile)
}

// ---------------------------------------------------------------------------
// 10. TrackerTree — RegisterInterfaceResolution / GetInterfaceResolutions / ResolveInterface
// ---------------------------------------------------------------------------

func TestTrackerTree_InterfaceResolution_RegisterAndGet(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	tree.RegisterInterfaceResolution("Handler", "Server", "mypkg", "MyHandler")

	resolutions := tree.GetInterfaceResolutions()
	require.Len(t, resolutions, 1)

	key := interfaceKey{InterfaceType: "Handler", StructType: "Server", Pkg: "mypkg"}
	assert.Equal(t, "MyHandler", resolutions[key])
}

func TestTrackerTree_InterfaceResolution_Empty(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	resolutions := tree.GetInterfaceResolutions()
	assert.Empty(t, resolutions)
}

func TestTrackerTree_ResolveInterface_Found(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	tree.RegisterInterfaceResolution("Writer", "Logger", "io", "FileWriter")

	result := tree.ResolveInterface("Writer", "Logger", "io")
	assert.Equal(t, "FileWriter", result)
}

func TestTrackerTree_ResolveInterface_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.ResolveInterface("Reader", "Conn", "net")
	// Returns original interface type when not found
	assert.Equal(t, "Reader", result)
}

func TestTrackerTree_RegisterInterfaceResolution_Overwrite(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	tree.RegisterInterfaceResolution("Handler", "App", "pkg", "First")
	tree.RegisterInterfaceResolution("Handler", "App", "pkg", "Second")

	result := tree.ResolveInterface("Handler", "App", "pkg")
	assert.Equal(t, "Second", result)
}

// ---------------------------------------------------------------------------
// 11. TrackerTree — TraceArgumentOrigin
// ---------------------------------------------------------------------------

func TestTrackerTree_TraceArgumentOrigin_NilNode(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.TraceArgumentOrigin(nil)
	assert.Nil(t, result)
}

func TestTrackerTree_TraceArgumentOrigin_NotArgument(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	nd := &TrackerNode{key: "nd", IsArgument: false}
	result := tree.TraceArgumentOrigin(nd)
	assert.Nil(t, result)
}

func TestTrackerTree_TraceArgumentOrigin_NonVariableArgType(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	nd := &TrackerNode{
		key:        "nd",
		IsArgument: true,
		ArgType:    ArgTypeLiteral,
	}
	result := tree.TraceArgumentOrigin(nd)
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// 12. TrackerTree — FindVariableNodes
// ---------------------------------------------------------------------------

func TestTrackerTree_FindVariableNodes_Empty(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.FindVariableNodes()
	assert.Empty(t, result)
}

func TestTrackerTree_FindVariableNodes_WithNodes(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	n1 := &TrackerNode{key: "n1"}
	n2 := &TrackerNode{key: "n2"}
	n3 := &TrackerNode{key: "n3"}

	tree := &TrackerTree{
		meta:      meta,
		positions: map[string]bool{},
		limits:    defaultTrackerLimits(),
		variableNodes: map[paramKey][]*TrackerNode{
			{Name: "x", Pkg: "main", Container: "func1"}: {n1, n2},
			{Name: "y", Pkg: "main", Container: "func2"}: {n3},
		},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.FindVariableNodes()
	assert.Len(t, result, 3)
}

// ---------------------------------------------------------------------------
// 13. TrackerTree — GetRoots
// ---------------------------------------------------------------------------

func TestTrackerTree_GetRoots_Empty(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	roots := tree.GetRoots()
	assert.Empty(t, roots)
}

func TestTrackerTree_GetRoots_WithRoots(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	r1 := &TrackerNode{key: "r1"}
	r2 := &TrackerNode{key: "r2"}

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		roots:                  []*TrackerNode{r1, r2},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	roots := tree.GetRoots()
	assert.Len(t, roots, 2)
}

func TestTrackerTree_GetRoots_Nil(t *testing.T) {
	var tree *TrackerTree
	roots := tree.GetRoots()
	assert.Nil(t, roots)
}

// ---------------------------------------------------------------------------
// 14. TrackerTree — SyncInterfaceResolutionsFromMetadata
// ---------------------------------------------------------------------------

func TestTrackerTree_SyncInterfaceResolutionsFromMetadata_NilMeta(t *testing.T) {
	tree := &TrackerTree{
		meta:                   nil,
		interfaceResolutionMap: map[interfaceKey]string{},
	}
	// Should not panic
	tree.SyncInterfaceResolutionsFromMetadata()
	assert.Empty(t, tree.interfaceResolutionMap)
}

func TestTrackerTree_SyncInterfaceResolutionsFromMetadata_EmptyResolutions(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	tree.SyncInterfaceResolutionsFromMetadata()
	assert.Empty(t, tree.interfaceResolutionMap)
}

// ---------------------------------------------------------------------------
// 15. ResolveInterfaceFromMetadata
// ---------------------------------------------------------------------------

func TestTrackerTree_ResolveInterfaceFromMetadata_FoundInLocal(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	tree.RegisterInterfaceResolution("Svc", "App", "main", "MySvc")

	result := tree.ResolveInterfaceFromMetadata("Svc", "App", "main")
	assert.Equal(t, "MySvc", result)
}

func TestTrackerTree_ResolveInterfaceFromMetadata_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.ResolveInterfaceFromMetadata("Unknown", "Struct", "pkg")
	// Falls back to the interface type itself
	assert.Equal(t, "Unknown", result)
}

// ---------------------------------------------------------------------------
// 16. classifyArgument
// ---------------------------------------------------------------------------

func TestClassifyArgument_AllKinds(t *testing.T) {
	meta, _ := newTrackerTestMeta()

	tests := []struct {
		kind     string
		extra    func(*metadata.CallArgument)
		expected ArgumentType
	}{
		{metadata.KindCall, nil, ArgTypeFunctionCall},
		{metadata.KindFuncLit, nil, ArgTypeFunctionCall},
		{metadata.KindIdent, nil, ArgTypeVariable},
		{metadata.KindIdent, func(a *metadata.CallArgument) { a.SetType("func() error") }, ArgTypeFunctionCall},
		{metadata.KindLiteral, nil, ArgTypeLiteral},
		{metadata.KindSelector, nil, ArgTypeSelector},
		{metadata.KindUnary, nil, ArgTypeUnary},
		{metadata.KindBinary, nil, ArgTypeBinary},
		{metadata.KindIndex, nil, ArgTypeIndex},
		{metadata.KindCompositeLit, nil, ArgTypeComposite},
		{metadata.KindTypeAssert, nil, ArgTypeTypeAssert},
		{"unknown_thing", nil, ArgTypeComplex},
	}

	for _, tt := range tests {
		a := metadata.NewCallArgument(meta)
		a.SetKind(tt.kind)
		if tt.extra != nil {
			tt.extra(a)
		}
		result := classifyArgument(a)
		assert.Equal(t, tt.expected, result, "for kind %s", tt.kind)
	}
}

// ---------------------------------------------------------------------------
// 17. getString helper
// ---------------------------------------------------------------------------

func TestGetString_NilMeta_Tracker(t *testing.T) {
	result := getString(nil, 0)
	assert.Equal(t, "", result)
}

func TestGetString_NilStringPool_Tracker(t *testing.T) {
	meta := &metadata.Metadata{StringPool: nil}
	result := getString(meta, 0)
	assert.Equal(t, "", result)
}

func TestGetString_ValidIndex(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	idx := sp.Get("hello")
	result := getString(meta, idx)
	assert.Equal(t, "hello", result)
}

// ---------------------------------------------------------------------------
// 18. interfaceKey / assignmentKey String
// ---------------------------------------------------------------------------

func TestInterfaceKey_String(t *testing.T) {
	k := interfaceKey{InterfaceType: "Handler", StructType: "Server", Pkg: "pkg"}
	assert.Equal(t, "pkgServerHandler", k.String())
}

func TestAssignmentKey_String(t *testing.T) {
	k := assignmentKey{Name: "x", Pkg: "main", Type: "int", Container: "func"}
	assert.Equal(t, "mainintxfunc", k.String())
}

// ---------------------------------------------------------------------------
// 19. TrackerNode — TypeParams with parent propagation
// ---------------------------------------------------------------------------

func TestTrackerNode_TypeParams_Propagation(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	parentEdge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("parent"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "string"},
	}
	childEdge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("child"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"U": "int"},
	}

	parent := &TrackerNode{}
	parent.CallGraphEdge = parentEdge

	child := &TrackerNode{Parent: parent}
	child.CallGraphEdge = childEdge

	params := child.TypeParams()
	assert.Equal(t, "string", params["T"])
	assert.Equal(t, "int", params["U"])
}

func TestTrackerNode_TypeParams_CycleProtection(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "bool"},
	}

	// Create a cycle: node -> parent -> node (should not infinite loop)
	node1 := &TrackerNode{}
	node1.CallGraphEdge = edge
	node2 := &TrackerNode{Parent: node1}
	node1.Parent = node2

	// Should not panic or infinite loop
	params := node1.TypeParams()
	assert.Equal(t, "bool", params["T"])
}

// ---------------------------------------------------------------------------
// 20. findNodeByEdgeID
// ---------------------------------------------------------------------------

func TestTrackerTree_FindNodeByEdgeID_InCache(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("cached"), Pkg: sp.Get("pkg")},
	}
	cachedNode := &TrackerNode{key: "cached"}
	cachedNode.CallGraphEdge = edge

	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{"pkg.cached": cachedNode},
		idCache:                map[string]string{},
	}

	result := tree.findNodeByEdgeID("pkg.cached")
	require.NotNil(t, result)
	assert.Equal(t, "cached", result.key)
}

func TestTrackerTree_FindNodeByEdgeID_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}

	result := tree.findNodeByEdgeID("nonexistent")
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// 21. getTrackerNode pool
// ---------------------------------------------------------------------------

func TestGetTrackerNode_ReturnsCleanNode(t *testing.T) {
	node := getTrackerNode()
	require.NotNil(t, node)
	assert.Equal(t, "", node.key)
	assert.Nil(t, node.Parent)
	assert.Nil(t, node.Children)
	assert.Nil(t, node.CallGraphEdge)
	assert.Nil(t, node.CallArgument)
}

func TestGetTrackerNode_PoolReuse(t *testing.T) {
	// Get a node, modify it, put it back, get another
	node1 := getTrackerNode()
	node1.key = "dirty"
	node1.ArgIndex = 42
	trackerNodePool.Put(node1)

	node2 := getTrackerNode()
	// Should be clean even if recycled
	assert.Equal(t, "", node2.key)
	assert.Equal(t, 0, node2.ArgIndex)
}

// ---------------------------------------------------------------------------
// 22. filterChildren
// ---------------------------------------------------------------------------

func TestFilterChildren_AllMatch(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "string"},
	}
	c1 := &TrackerNode{}
	c1.CallGraphEdge = edge
	c2 := &TrackerNode{}
	c2.CallGraphEdge = edge

	result := filterChildren([]*TrackerNode{c1, c2}, map[string]string{"T": "string"})
	assert.Len(t, result, 2)
}

func TestFilterChildren_Empty(t *testing.T) {
	result := filterChildren([]*TrackerNode{}, map[string]string{"T": "string"})
	assert.Empty(t, result)
}
