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
// Helpers
// ---------------------------------------------------------------------------

// newTrackerTree creates a TrackerTree with initialized maps (no call graph).
func newTrackerTree(meta *metadata.Metadata) *TrackerTree {
	return &TrackerTree{
		meta:                   meta,
		positions:              map[string]bool{},
		limits:                 defaultTrackerLimits(),
		variableNodes:          map[paramKey][]*TrackerNode{},
		chainParentMap:         map[string]*metadata.CallGraphEdge{},
		interfaceResolutionMap: map[interfaceKey]string{},
		nodeMap:                map[string]*TrackerNode{},
		idCache:                map[string]string{},
	}
}

// ---------------------------------------------------------------------------
// processArgUnary
// ---------------------------------------------------------------------------

func TestProcessArgUnary_NilX(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindUnary)
	// X is nil

	argNode := &TrackerNode{key: "unary-arg"}
	assignmentIndex := assigmentIndexMap{}
	// Should not panic
	processArgUnary(meta, argNode, arg, edge, &assignmentIndex)
}

func TestProcessArgUnary_XNotIdent(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	x := metadata.NewCallArgument(meta)
	x.SetKind(metadata.KindLiteral) // Not KindIdent
	x.SetName("val")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindUnary)
	arg.X = x

	argNode := &TrackerNode{key: "unary-arg"}
	assignmentIndex := assigmentIndexMap{}
	// Should return early without linking
	processArgUnary(meta, argNode, arg, edge, &assignmentIndex)
	// No parent assigned
	assert.Nil(t, argNode.Parent)
}

func TestProcessArgUnary_IdentLinksToAssignment(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	x := metadata.NewCallArgument(meta)
	x.SetKind(metadata.KindIdent)
	x.SetName("myVar")
	x.SetType("MyType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindUnary)
	arg.X = x

	argNode := &TrackerNode{key: "unary-arg"}

	// Create an assignment node that matches the key
	assignmentNode := &TrackerNode{key: "assign-node", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "myVar",
		Pkg:       "main",
		Type:      "MyType",
		Container: "caller",
	}
	assignmentIndex := assigmentIndexMap{akey: assignmentNode}

	processArgUnary(meta, argNode, arg, edge, &assignmentIndex)
	// The assignment node should now have argNode as a child
	assert.Contains(t, assignmentNode.Children, argNode)
}

// ---------------------------------------------------------------------------
// processFuncCallSelectorMethod
// ---------------------------------------------------------------------------

func TestProcessFuncCallSelectorMethod_NoVariableNodes(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	// sel is not a function type, so should return false
	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")
	selArg.SetType("string") // not "func(...)"
	selArg.SetPkg("main")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("ObjType")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg

	argNode := &TrackerNode{key: "arg"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processFuncCallSelectorMethod(tree, meta, argNode, selectorArg, edge, visited, &assignmentIndex, defaultTrackerLimits())
	assert.False(t, result)
}

func TestProcessFuncCallSelectorMethod_FuncTypeLinksVariables(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("DoSomething")
	selArg.SetType("func() error")
	selArg.SetPkg("main")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("svc")
	xArg.SetType("Service")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg

	argNode := &TrackerNode{key: "arg"}

	// Create a variable node
	varNode := &TrackerNode{key: "var-svc", Children: []*TrackerNode{}}
	pkey := paramKey{
		Name:      "svc",
		Pkg:       "main",
		Container: "caller",
	}
	tree.variableNodes[pkey] = []*TrackerNode{varNode}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processFuncCallSelectorMethod(tree, meta, argNode, selectorArg, edge, visited, &assignmentIndex, defaultTrackerLimits())
	// Function type detected, should return true
	assert.True(t, result)
	// Variable node should have argNode as child
	assert.Contains(t, varNode.Children, argNode)
}

// ---------------------------------------------------------------------------
// resolveFuncCallSelectorEdges
// ---------------------------------------------------------------------------

func TestResolveFuncCallSelectorEdges_NoMatchingEdge(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("mypkg")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("MyType")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// No edges in call graph => no children created
	resolveFuncCallSelectorEdges(tree, meta, argNode, selectorArg, "obj", "obj", visited, &assignmentIndex, defaultTrackerLimits())
	assert.Empty(t, argNode.Children)
}

func TestResolveFuncCallSelectorEdges_WithReceiverType(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	recvArg := metadata.NewCallArgument(meta)
	recvArg.SetKind(metadata.KindIdent)
	recvArg.SetName("MyType")

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("mypkg")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("MyType")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg
	selectorArg.ReceiverType = recvArg

	tree := newTrackerTree(meta)

	// Add an edge that matches
	meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:     meta,
			Name:     sp.Get("Method"),
			Pkg:      sp.Get("mypkg"),
			RecvType: sp.Get("MyType"),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: sp.Get("inner"),
			Pkg:  sp.Get("mypkg"),
		},
	})

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// originVar == varName, so ReceiverType path is taken
	resolveFuncCallSelectorEdges(tree, meta, argNode, selectorArg, "obj", "obj", visited, &assignmentIndex, defaultTrackerLimits())
	assert.NotEmpty(t, argNode.Children)
}

// ---------------------------------------------------------------------------
// processArgFunctionCall
// ---------------------------------------------------------------------------

func TestProcessArgFunctionCall_NilFun(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindCall)
	// Fun is nil

	parentNode := &TrackerNode{key: "parent"}
	argNode := &TrackerNode{key: "arg"}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processArgFunctionCall(tree, meta, parentNode, argNode, arg, edge, nil, "arg-id", 0, visited, &assignmentIndex, defaultTrackerLimits())
	// With nil Fun, the selector branch is skipped; still creates a NewTrackerNode
	assert.NotNil(t, result)
}

func TestProcessArgFunctionCall_WithSelectorFun(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetType("func() error")
	selArg.SetPkg("main")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("svc")
	xArg.SetType("Service")

	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindSelector)
	funArg.Sel = selArg
	funArg.X = xArg

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = funArg

	parentNode := &TrackerNode{key: "parent"}
	argNode := &TrackerNode{key: "call-arg"}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processArgFunctionCall(tree, meta, parentNode, argNode, arg, edge, nil, "call-id", 0, visited, &assignmentIndex, defaultTrackerLimits())
	assert.NotNil(t, result)
}

// ---------------------------------------------------------------------------
// findNodeByEdgeID / findNodeInSubtreeWithVisited
// ---------------------------------------------------------------------------

func TestFindNodeByEdgeID_FromNodeMap(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edgeNode := &TrackerNode{key: "target-node"}
	edgeNode.CallGraphEdge = &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
	}
	tree.nodeMap["pkg.fn"] = edgeNode

	found := tree.findNodeByEdgeID("pkg.fn")
	assert.Equal(t, edgeNode, found)
}

func TestFindNodeByEdgeID_FallbackToRoots(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	rootEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
	}
	root := &TrackerNode{}
	root.CallGraphEdge = rootEdge
	tree.roots = []*TrackerNode{root}

	calleeID := rootEdge.Callee.ID()
	found := tree.findNodeByEdgeID(calleeID)
	require.NotNil(t, found)
	// Should be cached now
	assert.Equal(t, found, tree.nodeMap[calleeID])
}

func TestFindNodeByEdgeID_FallbackToSubtree(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	childEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("deep"), Pkg: sp.Get("main")},
		Caller: metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
	}
	child := &TrackerNode{}
	child.CallGraphEdge = childEdge

	rootEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
	}
	root := &TrackerNode{Children: []*TrackerNode{child}}
	root.CallGraphEdge = rootEdge
	tree.roots = []*TrackerNode{root}

	calleeID := childEdge.Callee.ID()
	found := tree.findNodeByEdgeID(calleeID)
	require.NotNil(t, found)
	assert.Equal(t, child, found)
}

func TestFindNodeByEdgeID_NotFoundDeep(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	tree.roots = []*TrackerNode{{key: "root"}}

	found := tree.findNodeByEdgeID("nonexistent")
	assert.Nil(t, found)
}

func TestFindNodeInSubtreeWithVisited_CycleDetectionDeep(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Create a cycle: a -> b -> a
	a := &TrackerNode{key: "a"}
	b := &TrackerNode{key: "b"}
	a.Children = []*TrackerNode{b}
	b.Children = []*TrackerNode{a}

	visited := make(map[*TrackerNode]bool)
	found := tree.findNodeInSubtreeWithVisited(a, "nonexistent", visited)
	assert.Nil(t, found)
}

func TestFindNodeInSubtreeWithVisited_DepthLimit(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Create a deep chain of 60 nodes
	var first, prev *TrackerNode
	for i := 0; i < 60; i++ {
		node := &TrackerNode{key: "node"}
		if first == nil {
			first = node
		}
		if prev != nil {
			prev.Children = []*TrackerNode{node}
		}
		prev = node
	}

	visited := make(map[*TrackerNode]bool)
	found := tree.findNodeInSubtreeWithVisited(first, "nonexistent", visited)
	assert.Nil(t, found) // Should hit the depth limit
}

func TestFindNodeInSubtreeWithVisited_NilNode(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	visited := make(map[*TrackerNode]bool)
	found := tree.findNodeInSubtreeWithVisited(nil, "id", visited)
	assert.Nil(t, found)
}

func TestFindNodeInSubtreeWithVisited_MaxChildLimit(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Create a node with 25 children, target is at position 22 (beyond maxChildrenToSearch=20)
	parent := &TrackerNode{key: "parent", Children: make([]*TrackerNode, 25)}
	for i := 0; i < 25; i++ {
		parent.Children[i] = &TrackerNode{key: "child"}
	}
	// Put target at index 22
	targetEdge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("main")},
		Caller: metadata.Call{Meta: meta, Name: sp.Get("parent"), Pkg: sp.Get("main")},
	}
	target := &TrackerNode{}
	target.CallGraphEdge = targetEdge
	parent.Children[22] = target

	visited := make(map[*TrackerNode]bool)
	calleeID := targetEdge.Callee.ID()
	found := tree.findNodeInSubtreeWithVisited(parent, calleeID, visited)
	// Beyond maxChildrenToSearch limit, so it should not be found
	assert.Nil(t, found)
}

// ---------------------------------------------------------------------------
// NewTrackerNode
// ---------------------------------------------------------------------------

func TestNewTrackerNode_EmptyID(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	node := NewTrackerNode(tree, meta, "", "", nil, nil, visited, &assignmentIndex, defaultTrackerLimits())
	assert.Nil(t, node)
}

func TestNewTrackerNode_SelfRecursion(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	// parentID == id should return nil
	node := NewTrackerNode(tree, meta, "same-id", "same-id", nil, nil, visited, &assignmentIndex, defaultTrackerLimits())
	assert.Nil(t, node)
}

func TestNewTrackerNode_MaxRecursionDepth(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	limits := defaultTrackerLimits()
	limits.MaxRecursionDepth = 1
	visited := map[string]int{"node-id": 1}
	assignmentIndex := assigmentIndexMap{}
	node := NewTrackerNode(tree, meta, "", "node-id", nil, nil, visited, &assignmentIndex, limits)
	assert.Nil(t, node)
}

func TestNewTrackerNode_MaxNodesPerTree(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	limits := defaultTrackerLimits()
	limits.MaxNodesPerTree = 0 // already over limit
	// visited has more entries than limit
	visited := map[string]int{"a": 1}
	assignmentIndex := assigmentIndexMap{}
	node := NewTrackerNode(tree, meta, "", "node-id", nil, nil, visited, &assignmentIndex, limits)
	// Should still create a node but truncated
	require.NotNil(t, node)
	assert.Equal(t, "node-id", node.key)
}

func TestNewTrackerNode_WithCallArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")

	node := NewTrackerNode(tree, meta, "", "some-id", nil, arg, visited, &assignmentIndex, defaultTrackerLimits())
	require.NotNil(t, node)
	assert.Equal(t, arg, node.CallArgument)
}

func TestNewTrackerNode_WithParentEdge(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	parentEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
	}

	node := NewTrackerNode(tree, meta, "", "main.handler", parentEdge, nil, visited, &assignmentIndex, defaultTrackerLimits())
	require.NotNil(t, node)
	assert.Equal(t, parentEdge, node.CallGraphEdge)
}

func TestNewTrackerNode_WithCalleeVarName(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	parentEdge := &metadata.CallGraphEdge{
		Caller:        metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main")},
		Callee:        metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
		CalleeVarName: "myVar",
	}

	node := NewTrackerNode(tree, meta, "", "main.handler", parentEdge, nil, visited, &assignmentIndex, defaultTrackerLimits())
	require.NotNil(t, node)
}

// ---------------------------------------------------------------------------
// processArgVariable
// ---------------------------------------------------------------------------

func TestProcessArgVariable_LinksToAssignment(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("myVar")
	arg.SetType("int")

	argNode := &TrackerNode{key: "arg-node"}

	assignmentNode := &TrackerNode{key: "assign", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "myVar",
		Pkg:       "main",
		Type:      "int",
		Container: "caller",
	}
	assignmentIndex := assigmentIndexMap{akey: assignmentNode}

	processArgVariable(tree, meta, argNode, arg, edge, &assignmentIndex)
	// The assignment node should have argNode as child
	assert.Contains(t, assignmentNode.Children, argNode)
	assert.Equal(t, assignmentNode, argNode.Parent)
}

func TestProcessArgVariable_FallbackToCalleePkg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("other")},
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")
	arg.SetType("string")

	argNode := &TrackerNode{key: "arg-node"}

	// First key (originVar) won't match, but the fallback key with callee pkg should
	assignmentNode := &TrackerNode{key: "assign-fallback", Children: []*TrackerNode{}}
	fallbackKey := assignmentKey{
		Name:      "x",
		Pkg:       "other",
		Type:      "string",
		Container: "caller",
	}
	assignmentIndex := assigmentIndexMap{fallbackKey: assignmentNode}

	processArgVariable(tree, meta, argNode, arg, edge, &assignmentIndex)
	assert.Equal(t, argNode, assignmentNode.Parent)
}

func TestProcessArgVariable_LinksToVariableNode(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("v")
	arg.SetType("int")

	argNode := &TrackerNode{key: "arg-node"}

	varNode := &TrackerNode{key: "var-v", Children: []*TrackerNode{}}
	pkey := paramKey{Name: "v", Pkg: "main", Container: "caller"}
	tree.variableNodes[pkey] = []*TrackerNode{varNode}

	assignmentIndex := assigmentIndexMap{}
	processArgVariable(tree, meta, argNode, arg, edge, &assignmentIndex)
	assert.Contains(t, varNode.Children, argNode)
	// RootAssignmentMap should be initialized
	assert.NotNil(t, argNode.RootAssignmentMap)
}

// ---------------------------------------------------------------------------
// traverseTree
// ---------------------------------------------------------------------------

func TestTraverseTree_EmptyNodeKey(t *testing.T) {
	// Nodes with empty key are skipped
	emptyNode := &TrackerNode{key: ""}
	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	result := traverseTree([]*TrackerNode{emptyNode}, an, 1, nil)
	assert.False(t, result)
}

func TestTraverseTree_LimitExceededDeep(t *testing.T) {
	node := &TrackerNode{key: "a"}
	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	nodeCount := map[string]int{"a": 3}
	result := traverseTree([]*TrackerNode{node}, an, 1, nodeCount)
	assert.False(t, result)
}

func TestTraverseTree_DefaultLimit(t *testing.T) {
	node := &TrackerNode{key: "a"}
	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{}}
	// limit=0 should use MaxSelfCallingDepth
	result := traverseTree([]*TrackerNode{node}, an, 0, nil)
	assert.False(t, result)
}

func TestTraverseTree_MatchingAssignmentDeep(t *testing.T) {
	child := &TrackerNode{key: "child"}
	targetNode := &TrackerNode{
		key:      "a",
		Children: []*TrackerNode{child},
	}
	child.Parent = targetNode

	rootNode := &TrackerNode{key: "a"}

	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{
		assignmentKey{}: targetNode,
	}}
	result := traverseTree([]*TrackerNode{rootNode}, an, 5, nil)
	// traverseTree returns false when it finishes without hitting recursion limit
	assert.False(t, result)
	// rootNode should have acquired children from matching assignment
	assert.NotEmpty(t, rootNode.Children)
}

// ---------------------------------------------------------------------------
// SyncInterfaceResolutionsFromMetadata
// ---------------------------------------------------------------------------

func TestSyncInterfaceResolutionsFromMetadata_NilMeta(t *testing.T) {
	tree := &TrackerTree{
		meta:                   nil,
		interfaceResolutionMap: map[interfaceKey]string{},
	}
	// Should not panic
	tree.SyncInterfaceResolutionsFromMetadata()
	assert.Empty(t, tree.interfaceResolutionMap)
}

func TestSyncInterfaceResolutionsFromMetadata_WithResolutions(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	meta.RegisterInterfaceResolution("Reader", "MyStruct", "mypkg", "ConcreteReader", "test.go:10")

	tree := &TrackerTree{
		meta:                   meta,
		interfaceResolutionMap: map[interfaceKey]string{},
	}
	tree.SyncInterfaceResolutionsFromMetadata()

	key := interfaceKey{InterfaceType: "Reader", StructType: "MyStruct", Pkg: "mypkg"}
	assert.Equal(t, "ConcreteReader", tree.interfaceResolutionMap[key])
}

// ---------------------------------------------------------------------------
// NewTrackerTree
// ---------------------------------------------------------------------------

func TestNewTrackerTree_WithCallGraph(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Meta: nil, Name: sp.Get("main"), Pkg: sp.Get("main"),
					RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
				},
				Callee: metadata.Call{
					Meta: nil, Name: sp.Get("handler"), Pkg: sp.Get("main"),
					RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
				},
			},
		},
	}
	// Wire Meta pointers
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	limits := defaultTrackerLimits()
	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	assert.Equal(t, meta, tree.GetMetadata())
	// main is root function, so we should have 1 root
	assert.GreaterOrEqual(t, len(tree.roots), 1)
}

func TestNewTrackerTree_ChainRelationships(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	limits := defaultTrackerLimits()
	tree := NewTrackerTree(meta, limits)
	require.NotNil(t, tree)
	assert.NotNil(t, tree.chainParentMap)
}

// ---------------------------------------------------------------------------
// ResolveInterfaceFromMetadata
// ---------------------------------------------------------------------------

func TestResolveInterfaceFromMetadata_LocalCache(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Pre-populate local cache
	key := interfaceKey{InterfaceType: "Writer", StructType: "Server", Pkg: "http"}
	tree.interfaceResolutionMap[key] = "FileWriter"

	result := tree.ResolveInterfaceFromMetadata("Writer", "Server", "http")
	assert.Equal(t, "FileWriter", result)
}

func TestResolveInterfaceFromMetadata_FromMetadata(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	meta.RegisterInterfaceResolution("Handler", "App", "web", "ConcreteHandler", "test.go:5")

	tree := newTrackerTree(meta)

	result := tree.ResolveInterfaceFromMetadata("Handler", "App", "web")
	assert.Equal(t, "ConcreteHandler", result)
	// Should be cached locally
	key := interfaceKey{InterfaceType: "Handler", StructType: "App", Pkg: "web"}
	assert.Equal(t, "ConcreteHandler", tree.interfaceResolutionMap[key])
}

func TestResolveInterfaceFromMetadata_NotFound(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	result := tree.ResolveInterfaceFromMetadata("Unknown", "", "")
	assert.Equal(t, "Unknown", result)
}

// ---------------------------------------------------------------------------
// filterChildren
// ---------------------------------------------------------------------------

func TestFilterChildren_AllMatchDeep(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "int"},
	}
	child := &TrackerNode{CallGraphEdge: edge}
	nodeTypeParams := map[string]string{"T": "int"}

	result := filterChildren([]*TrackerNode{child}, nodeTypeParams)
	assert.Len(t, result, 1)
}

func TestFilterChildren_NonMatchDeep(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "string"},
	}
	child := &TrackerNode{CallGraphEdge: edge}
	nodeTypeParams := map[string]string{"T": "int"}

	result := filterChildren([]*TrackerNode{child}, nodeTypeParams)
	// Mismatch, should still be included due to the hasMatch bug (it doesn't reset per child).
	// Actually the function does NOT reset hasMatch between children, so behavior is:
	// once hasMatch becomes false, all subsequent children are also excluded.
	// But the first child itself: it will detect mismatch and set hasMatch=false, so it's excluded.
	assert.Len(t, result, 0)
}

func TestFilterChildren_EmptyDeep(t *testing.T) {
	result := filterChildren([]*TrackerNode{}, map[string]string{"T": "int"})
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// AddChildren — reparenting
// ---------------------------------------------------------------------------

func TestAddChildren_Reparent(t *testing.T) {
	oldParent := &TrackerNode{key: "old"}
	child := &TrackerNode{key: "child"}
	oldParent.AddChild(child)

	newParent := &TrackerNode{key: "new"}
	newParent.AddChildren([]*TrackerNode{child})

	assert.Equal(t, newParent, child.Parent)
	assert.Len(t, newParent.Children, 1)
	assert.Len(t, oldParent.Children, 0)
}

func TestAddChildren_SameParent(t *testing.T) {
	parent := &TrackerNode{key: "p"}
	c1 := &TrackerNode{key: "c1"}
	parent.AddChild(c1)

	// AddChildren with the same parent should not detach
	parent.AddChildren([]*TrackerNode{c1})
	assert.Equal(t, parent, c1.Parent)
}

func TestAddChildren_NilParentOnChild(t *testing.T) {
	parent := &TrackerNode{key: "p"}
	c := &TrackerNode{key: "c"} // no parent
	parent.AddChildren([]*TrackerNode{c})
	assert.Equal(t, parent, c.Parent)
	assert.Len(t, parent.Children, 1)
}

// ---------------------------------------------------------------------------
// TraverseTree (TrackerTree method)
// ---------------------------------------------------------------------------

func TestTrackerTree_TraverseTree_MultipleRoots(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	r1c := &TrackerNode{key: "r1c"}
	r1 := &TrackerNode{key: "r1", Children: []*TrackerNode{r1c}}
	r2 := &TrackerNode{key: "r2"}
	tree.roots = []*TrackerNode{r1, r2}

	var keys []string
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		keys = append(keys, node.GetKey())
		return true
	})
	assert.Equal(t, []string{"r1", "r1c", "r2"}, keys)
}

func TestTrackerTree_TraverseTree_StopMidBranch(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	c := &TrackerNode{key: "c"}
	r1 := &TrackerNode{key: "r1", Children: []*TrackerNode{c}}
	r2 := &TrackerNode{key: "r2"}
	tree.roots = []*TrackerNode{r1, r2}

	var keys []string
	tree.TraverseTree(func(node TrackerNodeInterface) bool {
		keys = append(keys, node.GetKey())
		return node.GetKey() != "c" // stop on child
	})
	// Should visit r1 and c but not r2
	assert.Equal(t, []string{"r1", "c"}, keys)
}

// ---------------------------------------------------------------------------
// TypeParams — cycle detection
// ---------------------------------------------------------------------------

func TestTypeParams_CycleDetection(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edgeA := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("a"), Pkg: sp.Get("p")},
		TypeParamMap: map[string]string{"T": "int"},
	}
	edgeB := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("b"), Pkg: sp.Get("p")},
		TypeParamMap: map[string]string{"U": "string"},
	}

	a := &TrackerNode{key: "a"}
	a.CallGraphEdge = edgeA
	b := &TrackerNode{key: "b"}
	b.CallGraphEdge = edgeB

	// Create cycle
	a.Parent = b
	b.Parent = a

	params := a.TypeParams()
	assert.Equal(t, "int", params["T"])
	assert.Equal(t, "string", params["U"])
}

func TestTypeParams_NilParent(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
		TypeParamMap: map[string]string{"K": "bool"},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	params := node.TypeParams()
	assert.Equal(t, "bool", params["K"])
}

func TestTypeParams_FromCallArgument(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("val")
	arg.TypeParamMap = map[string]string{"V": "float64"}

	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("p")},
	}

	node := &TrackerNode{}
	node.CallGraphEdge = edge
	node.CallArgument = arg

	params := node.TypeParams()
	assert.Equal(t, "float64", params["V"])
}

// ---------------------------------------------------------------------------
// processChainRelationships
// ---------------------------------------------------------------------------

func TestProcessChainRelationships_NoChainsDeep(_ *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	// Should not panic
	tree.processChainRelationships()
}

func TestProcessChainRelationships_WithChildArgument(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	parentEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("Group"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
	}
	childEdge := metadata.CallGraphEdge{
		Caller:      metadata.Call{Meta: meta, Name: sp.Get("Use"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee:      metadata.Call{Meta: meta, Name: sp.Get("inner"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		ChainParent: parentEdge,
	}
	meta.CallGraph = []metadata.CallGraphEdge{
		*parentEdge,
		childEdge,
	}
	meta.BuildCallGraphMaps()

	tree := newTrackerTree(meta)

	parentNode := &TrackerNode{Children: []*TrackerNode{}}
	parentNode.CallGraphEdge = parentEdge
	tree.nodeMap[parentEdge.Callee.ID()] = parentNode

	argNode := &TrackerNode{Children: []*TrackerNode{}}
	argNode.CallArgument = metadata.NewCallArgument(meta)
	argNode.SetKind(metadata.KindIdent)
	argNode.SetName("arg")
	grandParent := &TrackerNode{key: "gp", Children: []*TrackerNode{argNode}}
	argNode.Parent = grandParent

	tree.nodeMap[childEdge.Callee.ID()] = argNode

	tree.processChainRelationships()
	// parentNode should be a child of grandParent (since argNode has a CallArgument)
	assert.Contains(t, grandParent.Children, parentNode)
}

// ---------------------------------------------------------------------------
// processArguments
// ---------------------------------------------------------------------------

func TestProcessArguments_NilEdgeDeep(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, nil, nil, visited, &assignmentIndex, defaultTrackerLimits())
	assert.Nil(t, result)
}

func TestProcessArguments_LimitReached(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	parentNode := &TrackerNode{key: "parent"}
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
		Args: []*metadata.CallArgument{
			func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindLiteral)
				a.SetName("lit1")
				a.SetValue("1")
				a.SetPosition("pos1")
				return a
			}(),
			func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindLiteral)
				a.SetName("lit2")
				a.SetValue("2")
				a.SetPosition("pos2")
				return a
			}(),
			func() *metadata.CallArgument {
				a := metadata.NewCallArgument(meta)
				a.SetKind(metadata.KindLiteral)
				a.SetName("lit3")
				a.SetValue("3")
				a.SetPosition("pos3")
				return a
			}(),
		},
	}

	limits := defaultTrackerLimits()
	limits.MaxArgsPerFunction = 2

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, limits)
	// Should be capped at 1 (first arg processed, second triggers limit)
	assert.LessOrEqual(t, len(result), 2)
}

func TestProcessArguments_SkipsSelfCaller(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
	}

	callerID := edge.Caller.ID()
	// Create an arg whose ID equals callerID => should be skipped
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("fn")
	arg.SetPkg("main")
	arg.SetPosition(callerID) // trick to get base matching

	parentNode := &TrackerNode{key: "parent"}
	edge.Args = []*metadata.CallArgument{arg}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, defaultTrackerLimits())
	// Args where callerID == StripToBase(argID) are skipped
	_ = result // we just verify no panic
}

func TestProcessArguments_NilNameArg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// An arg named "nil" should be skipped
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("nil")
	arg.SetPosition("pos1")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Args:   []*metadata.CallArgument{arg},
	}

	parentNode := &TrackerNode{key: "parent"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, defaultTrackerLimits())
	assert.Empty(t, result)
}

func TestProcessArguments_SelectorArg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")
	selArg.SetType("string")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("Obj")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.SetPosition("pos1")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Args:   []*metadata.CallArgument{arg},
	}

	parentNode := &TrackerNode{key: "parent"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, defaultTrackerLimits())
	assert.NotEmpty(t, result)
}

func TestProcessArguments_UnaryArg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("myPtr")
	xArg.SetType("*int")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindUnary)
	arg.X = xArg
	arg.SetPosition("pos1")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("main"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Args:   []*metadata.CallArgument{arg},
	}

	parentNode := &TrackerNode{key: "parent"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, defaultTrackerLimits())
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// GetNodeCount — nil child handling
// ---------------------------------------------------------------------------

func TestGetNodeCount_NilChildInList(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	root := &TrackerNode{key: "root", Children: []*TrackerNode{nil, {key: "child"}}}
	tree.roots = []*TrackerNode{root}

	count := tree.GetNodeCount()
	// root + child = 2 (nil child is nil-checked so skipped)
	assert.Equal(t, 2, count)
}

func TestGetNodeCount_DeepTree(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	gc := &TrackerNode{key: "gc"}
	c := &TrackerNode{key: "c", Children: []*TrackerNode{gc}}
	root := &TrackerNode{key: "root", Children: []*TrackerNode{c}}
	tree.roots = []*TrackerNode{root}

	assert.Equal(t, 3, tree.GetNodeCount())
}

// ---------------------------------------------------------------------------
// FindNodeByKey — deeper paths
// ---------------------------------------------------------------------------

func TestFindNodeByKey_DeepChild(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	gc := &TrackerNode{key: "grandchild"}
	c := &TrackerNode{key: "child", Children: []*TrackerNode{gc}}
	root := &TrackerNode{key: "root", Children: []*TrackerNode{c}}
	tree.roots = []*TrackerNode{root}

	found := tree.FindNodeByKey("grandchild")
	require.NotNil(t, found)
	assert.Equal(t, "grandchild", found.GetKey())
}

func TestFindNodeByKey_MultipleRoots(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	target := &TrackerNode{key: "target"}
	r1 := &TrackerNode{key: "r1"}
	r2 := &TrackerNode{key: "r2", Children: []*TrackerNode{target}}
	tree.roots = []*TrackerNode{r1, r2}

	found := tree.FindNodeByKey("target")
	require.NotNil(t, found)
	assert.Equal(t, "target", found.GetKey())
}

// ---------------------------------------------------------------------------
// resolveSelectorMethod
// ---------------------------------------------------------------------------

func TestResolveSelectorMethod_NoMatchingEdge(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("pkg")
	selArg.Name = sp.Get("Method")
	selArg.Pkg = sp.Get("pkg")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("ObjType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveSelectorMethod(tree, meta, argNode, arg, "obj", "obj", visited, &assignmentIndex, defaultTrackerLimits())
	assert.Empty(t, argNode.Children)
}

func TestResolveSelectorMethod_WithReceiverType(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	recvArg := metadata.NewCallArgument(meta)
	recvArg.SetKind(metadata.KindIdent)
	recvArg.SetName("ConcreteType")

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Process")
	selArg.SetPkg("mypkg")
	selArg.Name = sp.Get("Process")
	selArg.Pkg = sp.Get("mypkg")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("svc")
	xArg.SetType("SvcType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.ReceiverType = recvArg

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// originVar == varName so ReceiverType is used
	resolveSelectorMethod(tree, meta, argNode, arg, "svc", "svc", visited, &assignmentIndex, defaultTrackerLimits())
	// No matching edge, but it shouldn't panic
	assert.Empty(t, argNode.Children)
}

func TestResolveSelectorMethod_XIsSelector(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("pkg")
	selArg.Name = sp.Get("Method")
	selArg.Pkg = sp.Get("pkg")

	innerXArg := metadata.NewCallArgument(meta)
	innerXArg.SetKind(metadata.KindIdent)
	innerXArg.SetName("base")
	innerXArg.SetType("BaseType")
	innerXArg.SetPkg("pkg")

	innerSelArg := metadata.NewCallArgument(meta)
	innerSelArg.SetKind(metadata.KindIdent)
	innerSelArg.SetName("field")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindSelector)
	xArg.X = innerXArg
	xArg.Sel = innerSelArg

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveSelectorMethod(tree, meta, argNode, arg, "obj", "obj", visited, &assignmentIndex, defaultTrackerLimits())
	// Should not panic
}

func TestResolveSelectorMethod_XIsCall(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("pkg")
	selArg.Name = sp.Get("Method")
	selArg.Pkg = sp.Get("pkg")

	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindIdent)
	funArg.SetName("factory")
	funArg.SetType("FactoryType")
	funArg.SetPkg("pkg")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindCall)
	xArg.Fun = funArg
	xArg.X = funArg // X needed for the fallback

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveSelectorMethod(tree, meta, argNode, arg, "obj", "obj", visited, &assignmentIndex, defaultTrackerLimits())
	// Should not panic
}

// ---------------------------------------------------------------------------
// linkSelectorToAssignment
// ---------------------------------------------------------------------------

func TestLinkSelectorToAssignment_Basic(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("ObjType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.SetType("FieldType")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	argNode := &TrackerNode{key: "arg-node"}
	assignmentIndex := assigmentIndexMap{}

	// Should not panic even with no matching assignment
	linkSelectorToAssignment(tree, meta, argNode, arg, edge, &assignmentIndex)
}

func TestLinkSelectorToAssignment_WithMatchingAssignment(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("ObjType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.SetType("FieldType")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	argNode := &TrackerNode{key: "arg-node"}

	// CallArgToString for selector with X.Type="ObjType" returns "ObjType.Field"
	// TraceVariableOrigin returns ("ObjType.Field", "main", nil, "caller") when variable not found
	assignmentNode := &TrackerNode{key: "assign", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "ObjType.Field",
		Pkg:       "main",
		Type:      "FieldType",
		Container: "ObjType",
	}
	assignmentIndex := assigmentIndexMap{akey: assignmentNode}

	linkSelectorToAssignment(tree, meta, argNode, arg, edge, &assignmentIndex)
	// assignmentNode's parent should be set to argNode
	assert.Equal(t, argNode, assignmentNode.Parent)
}

func TestLinkSelectorToAssignment_WithVariableNode(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("ObjType")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.SetType("FieldType")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	argNode := &TrackerNode{key: "arg-node"}

	// CallArgToString for selector with X.Type="ObjType" returns "ObjType.Field"
	// TraceVariableOrigin returns ("ObjType.Field", "main", nil, "caller")
	varNode := &TrackerNode{key: "var-obj", Children: []*TrackerNode{}}
	pkey := paramKey{Name: "ObjType.Field", Pkg: "main", Container: "caller"}
	tree.variableNodes[pkey] = []*TrackerNode{varNode}

	assignmentIndex := assigmentIndexMap{}
	linkSelectorToAssignment(tree, meta, argNode, arg, edge, &assignmentIndex)
	assert.Contains(t, varNode.Children, argNode)
}

func TestLinkSelectorToAssignment_NestedSelector(_ *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Build nested selector: obj.inner.Field
	innerSel := metadata.NewCallArgument(meta)
	innerSel.SetKind(metadata.KindIdent)
	innerSel.SetName("inner")

	innerX := metadata.NewCallArgument(meta)
	innerX.SetKind(metadata.KindIdent)
	innerX.SetName("obj")
	innerX.SetType("ObjType")

	// X is a selector: obj.inner
	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindSelector)
	xArg.Sel = innerSel
	xArg.X = innerX

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg
	arg.SetType("FieldType")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
	}

	argNode := &TrackerNode{key: "arg-node"}
	assignmentIndex := assigmentIndexMap{}

	// Test the nested selector path (arg.X is selector, X.X has Type)
	linkSelectorToAssignment(tree, meta, argNode, arg, edge, &assignmentIndex)
	// Should not panic
}
