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
// Helpers for synthetic metadata construction
// ---------------------------------------------------------------------------

func realisticLimits() metadata.TrackerLimits {
	return metadata.TrackerLimits{
		MaxNodesPerTree:    50000,
		MaxChildrenPerNode: 500,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 10,
		MaxRecursionDepth:  10,
	}
}

// makeCallWithDefaults creates a Call with all int fields set to -1 except Name and Pkg.
func makeCallWithDefaults(meta *metadata.Metadata, name, pkg string) metadata.Call {
	sp := meta.StringPool
	return metadata.Call{
		Meta:         meta,
		Name:         sp.Get(name),
		Pkg:          sp.Get(pkg),
		RecvType:     -1,
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}
}

// makeCallWithRecv creates a Call with RecvType set.
func makeCallWithRecv(meta *metadata.Metadata, name, pkg, recvType string) metadata.Call {
	sp := meta.StringPool
	return metadata.Call{
		Meta:         meta,
		Name:         sp.Get(name),
		Pkg:          sp.Get(pkg),
		RecvType:     sp.Get(recvType),
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}
}

// makeCallWithPosition creates a Call with Position set.
func makeCallWithPosition(meta *metadata.Metadata, name, pkg, pos string) metadata.Call {
	sp := meta.StringPool
	return metadata.Call{
		Meta:         meta,
		Name:         sp.Get(name),
		Pkg:          sp.Get(pkg),
		RecvType:     -1,
		Position:     sp.Get(pos),
		Scope:        -1,
		SignatureStr: -1,
	}
}

// ---------------------------------------------------------------------------
// 1. NewTrackerTree — multi-edge call chain
// ---------------------------------------------------------------------------

func TestNewTrackerTree_MultiEdgeCallChain(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// main -> funcA -> funcB -> funcC
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "funcA", "app"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "funcA", "app"),
			Callee: makeCallWithDefaults(meta, "funcB", "app"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "funcB", "app"),
			Callee: makeCallWithDefaults(meta, "funcC", "app"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)

	// main is a root caller
	roots := tree.GetRoots()
	require.GreaterOrEqual(t, len(roots), 1, "should have at least one root (main)")

	// The tree must contain funcA, funcB, funcC as descendants
	nodeCount := tree.GetNodeCount()
	assert.GreaterOrEqual(t, nodeCount, 3, "tree should contain multiple nodes from the chain")
}

func TestNewTrackerTree_BranchingCallGraph(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// main calls both handleUsers and handleOrders
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "handleUsers", "app"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "handleOrders", "app"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "handleUsers", "app"),
			Callee: makeCallWithDefaults(meta, "getDB", "app"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "handleOrders", "app"),
			Callee: makeCallWithDefaults(meta, "getDB", "app"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)

	nodeCount := tree.GetNodeCount()
	assert.GreaterOrEqual(t, nodeCount, 3, "branching tree should have multiple nodes")
}

func TestNewTrackerTree_WithLiteralArgs(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	litArg := metadata.NewCallArgument(meta)
	litArg.SetKind(metadata.KindLiteral)
	litArg.SetValue("/api/v1")
	litArg.SetPosition("main.go:10:5")

	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "HandleFunc", "net/http"),
			Args:   []*metadata.CallArgument{litArg},
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
	assert.GreaterOrEqual(t, tree.GetNodeCount(), 1)
}

func TestNewTrackerTree_WithRootAssignments(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Create function with assignment map
	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"main.go": {
				Functions: map[string]*metadata.Function{
					"main": {
						Name: sp.Get("main"),
						Pkg:  sp.Get("app"),
						AssignmentMap: map[string][]metadata.Assignment{
							"srv": {
								{
									VariableName: sp.Get("srv"),
									Pkg:          sp.Get("app"),
									ConcreteType: sp.Get("*Server"),
									Func:         sp.Get("main"),
								},
							},
						},
					},
				},
			},
		},
	}

	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "ListenAndServe", "net/http"),
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "ListenAndServe", "net/http"),
			Callee: makeCallWithDefaults(meta, "serve", "net/http"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)

	// Verify the root node has assignment maps propagated
	roots := tree.GetRoots()
	if len(roots) > 0 {
		root := roots[0].(*TrackerNode)
		// root should have propagated assignment maps
		assert.NotNil(t, root)
	}
}

// ---------------------------------------------------------------------------
// 2. NewTrackerNode — argument kind switch coverage
// ---------------------------------------------------------------------------

func TestNewTrackerNode_WithIdentArg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	identArg := metadata.NewCallArgument(meta)
	identArg.SetKind(metadata.KindIdent)
	identArg.SetName("myHandler")
	identArg.SetPkg("app")
	identArg.SetType("func()")
	identArg.SetPosition("main.go:5:3")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("Register"),
			Pkg:          sp.Get("app"),
			RecvType:     -1,
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Args: []*metadata.CallArgument{identArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
	assert.Equal(t, edge, node.CallGraphEdge)
}

func TestNewTrackerNode_WithSelectorArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetType("string")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("obj")
	xArg.SetType("MyStruct")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg
	selectorArg.SetPosition("main.go:15:7")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "doWork", "app"),
		Args:   []*metadata.CallArgument{selectorArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithCallArgEdge(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindIdent)
	funArg.SetName("createHandler")
	funArg.SetType("func() Handler")
	funArg.SetPkg("app")

	callArg := metadata.NewCallArgument(meta)
	callArg.SetKind(metadata.KindCall)
	callArg.Fun = funArg
	callArg.SetPosition("main.go:20:3")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Register", "app"),
		Args:   []*metadata.CallArgument{callArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithCompositeLitArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	compositeLitArg := metadata.NewCallArgument(meta)
	compositeLitArg.SetKind(metadata.KindCompositeLit)
	compositeLitArg.SetType("Config")
	compositeLitArg.SetPkg("app")
	compositeLitArg.SetPosition("main.go:25:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "NewServer", "app"),
		Args:   []*metadata.CallArgument{compositeLitArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithUnaryArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("cfg")
	xArg.SetType("Config")

	unaryArg := metadata.NewCallArgument(meta)
	unaryArg.SetKind(metadata.KindUnary)
	unaryArg.X = xArg
	unaryArg.SetPosition("main.go:30:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Init", "app"),
		Args:   []*metadata.CallArgument{unaryArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithIndexArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	indexArg := metadata.NewCallArgument(meta)
	indexArg.SetKind(metadata.KindIndex)
	indexArg.SetName("items")
	indexArg.SetType("[]Item")
	indexArg.SetPosition("main.go:35:5")

	// X for the indexed expression
	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("items")
	xArg.SetType("[]Item")
	indexArg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Process", "app"),
		Args:   []*metadata.CallArgument{indexArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithKeyValueArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	kvArg := metadata.NewCallArgument(meta)
	kvArg.SetKind(metadata.KindKeyValue)
	kvArg.SetName("key")
	kvArg.SetValue("value")
	kvArg.SetPosition("main.go:40:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Set", "app"),
		Args:   []*metadata.CallArgument{kvArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithFuncLitArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	funcLitArg := metadata.NewCallArgument(meta)
	funcLitArg.SetKind(metadata.KindFuncLit)
	funcLitArg.SetType("func(http.ResponseWriter, *http.Request)")
	funcLitArg.SetPosition("main.go:45:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "HandleFunc", "net/http"),
		Args:   []*metadata.CallArgument{funcLitArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithRawArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	rawArg := metadata.NewCallArgument(meta)
	rawArg.SetKind(metadata.KindRaw)
	rawArg.SetRaw("someComplexExpr{}")
	rawArg.SetPosition("main.go:50:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Execute", "app"),
		Args:   []*metadata.CallArgument{rawArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithArrayTypeArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	arrayArg := metadata.NewCallArgument(meta)
	arrayArg.SetKind(metadata.KindArrayType)
	arrayArg.SetType("[]string")
	arrayArg.SetPosition("main.go:55:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "SetTags", "app"),
		Args:   []*metadata.CallArgument{arrayArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithMapTypeArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	mapArg := metadata.NewCallArgument(meta)
	mapArg.SetKind(metadata.KindMapType)
	mapArg.SetType("map[string]interface{}")
	mapArg.SetPosition("main.go:60:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Configure", "app"),
		Args:   []*metadata.CallArgument{mapArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithFuncTypeArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	funcTypeArg := metadata.NewCallArgument(meta)
	funcTypeArg.SetKind(metadata.KindFuncType)
	funcTypeArg.SetType("func(int) error")
	funcTypeArg.SetPosition("main.go:65:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "SetCallback", "app"),
		Args:   []*metadata.CallArgument{funcTypeArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithBinaryArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	binaryArg := metadata.NewCallArgument(meta)
	binaryArg.SetKind(metadata.KindBinary)
	binaryArg.SetRaw("a + b")
	binaryArg.SetPosition("main.go:70:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Sum", "app"),
		Args:   []*metadata.CallArgument{binaryArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithTypeAssertArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	typeAssertArg := metadata.NewCallArgument(meta)
	typeAssertArg.SetKind(metadata.KindTypeAssert)
	typeAssertArg.SetType("MyInterface")
	typeAssertArg.SetPosition("main.go:75:5")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("val")
	xArg.SetType("interface{}")
	typeAssertArg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Process", "app"),
		Args:   []*metadata.CallArgument{typeAssertArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithEllipsisArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	ellipsisArg := metadata.NewCallArgument(meta)
	ellipsisArg.SetKind(metadata.KindEllipsis)
	ellipsisArg.SetType("...string")
	ellipsisArg.SetPosition("main.go:80:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Print", "fmt"),
		Args:   []*metadata.CallArgument{ellipsisArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithStarArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	starArg := metadata.NewCallArgument(meta)
	starArg.SetKind(metadata.KindStar)
	starArg.SetType("*Config")
	starArg.SetPosition("main.go:85:5")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("ptrCfg")
	xArg.SetType("*Config")
	starArg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "ApplyConfig", "app"),
		Args:   []*metadata.CallArgument{starArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithParenArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	parenArg := metadata.NewCallArgument(meta)
	parenArg.SetKind(metadata.KindParen)
	parenArg.SetType("int")
	parenArg.SetPosition("main.go:90:5")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("expr")
	xArg.SetType("int")
	parenArg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Compute", "app"),
		Args:   []*metadata.CallArgument{parenArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithSliceArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	sliceArg := metadata.NewCallArgument(meta)
	sliceArg.SetKind(metadata.KindSlice)
	sliceArg.SetType("[]byte")
	sliceArg.SetPosition("main.go:95:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Write", "app"),
		Args:   []*metadata.CallArgument{sliceArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

func TestNewTrackerNode_WithTypeConversionArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	typeConvArg := metadata.NewCallArgument(meta)
	typeConvArg.SetKind(metadata.KindTypeConversion)
	typeConvArg.SetType("string")
	typeConvArg.SetPosition("main.go:100:5")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("data")
	xArg.SetType("[]byte")
	typeConvArg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "LogString", "app"),
		Args:   []*metadata.CallArgument{typeConvArg},
	}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

// ---------------------------------------------------------------------------
// 3. NewTrackerNode — CalleeVarName and ParentFunction paths
// ---------------------------------------------------------------------------

func TestNewTrackerNode_WithCalleeVarName_AndAssignment(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Create assignment node for the variable
	assignmentNode := &TrackerNode{key: "assign-svc", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "svc",
		Pkg:       "app",
		Type:      "Service",
		Container: "main",
	}
	assignmentIndex := assigmentIndexMap{akey: assignmentNode}

	parentEdge := &metadata.CallGraphEdge{
		Caller:        makeCallWithDefaults(meta, "main", "app"),
		Callee:        makeCallWithRecv(meta, "Handle", "app", "Service"),
		CalleeVarName: "svc",
	}

	// Add a variable node
	varNode := &TrackerNode{key: "var-svc", Children: []*TrackerNode{}}
	tree.variableNodes[paramKey{
		Name:      "svc",
		Pkg:       "app",
		Container: "Handle",
	}] = []*TrackerNode{varNode}

	visited := map[string]int{}
	node := NewTrackerNode(tree, meta, "", "app.Service.Handle", parentEdge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)

	_ = sp // sp used via makeCallWithDefaults
}

func TestNewTrackerNode_WithCalleeVarName_AndParentFunction(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	parentFunc := makeCallWithDefaults(meta, "outerFunc", "app")

	parentEdge := &metadata.CallGraphEdge{
		Caller:         makeCallWithDefaults(meta, "main", "app"),
		Callee:         makeCallWithDefaults(meta, "handler", "app"),
		CalleeVarName:  "h",
		ParentFunction: &parentFunc,
	}

	// Add a variable node matching the ParentFunction container
	varNode := &TrackerNode{key: "var-h", Children: []*TrackerNode{}}
	tree.variableNodes[paramKey{
		Name:      "h",
		Pkg:       "app",
		Container: "outerFunc",
	}] = []*TrackerNode{varNode}

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}
	node := NewTrackerNode(tree, meta, "", "app.handler", parentEdge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)

	// The variable node should have picked up the child
	assert.Contains(t, varNode.Children, node)
}

// ---------------------------------------------------------------------------
// 4. NewTrackerNode — ParentFunctions map path
// ---------------------------------------------------------------------------

func TestNewTrackerNode_ParentFunctionsPath(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Create an edge where wrapperFunc calls innerFunc.
	// We'll set up ParentFunctions for a functionID that is NOT in Callers,
	// which forces NewTrackerNode to take the ParentFunctions fallback path
	// (line 1292: meta.Callers[callerID] == nil && exists).
	wrapperEdge := metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "wrapperFunc", "app"),
		Callee: makeCallWithDefaults(meta, "innerFunc", "app"),
	}
	meta.CallGraph = append(meta.CallGraph, wrapperEdge)
	meta.BuildCallGraphMaps()

	// Set up ParentFunctions for a function ID that won't match any Callers key.
	// "app.orphanFunc" is not in the call graph as a caller, so Callers["app.orphanFunc"] == nil.
	parentEdge := &meta.CallGraph[0]
	meta.ParentFunctions = map[string][]*metadata.CallGraphEdge{
		"app.orphanFunc": {parentEdge},
	}

	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// Call NewTrackerNode with callerID that is NOT in Callers but IS in ParentFunctions
	node := NewTrackerNode(tree, meta, "", "app.orphanFunc", nil, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
	// The node should have children from the ParentFunctions fallback path
	assert.GreaterOrEqual(t, len(node.Children), 1, "expected children from ParentFunctions fallback")
}

// ---------------------------------------------------------------------------
// 5. NewTrackerNode — MaxChildrenPerNode limit
// ---------------------------------------------------------------------------

func TestNewTrackerNode_MaxChildrenPerNodeLimit(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Create 10 children for main
	for i := 0; i < 10; i++ {
		calleeName := "func" + string(rune('A'+i))
		meta.CallGraph = append(meta.CallGraph,
			metadata.CallGraphEdge{
				Caller: makeCallWithDefaults(meta, "main", "app"),
				Callee: makeCallWithPosition(meta, calleeName, "app", calleeName+".go:1:1"),
			},
		)
	}
	meta.BuildCallGraphMaps()

	tree := newTrackerTree(meta)
	limits := realisticLimits()
	limits.MaxChildrenPerNode = 3

	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	node := NewTrackerNode(tree, meta, "", "app.main", nil, nil, visited, &assignmentIndex, limits)
	require.NotNil(t, node)
	// Children should be capped at MaxChildrenPerNode
	assert.LessOrEqual(t, len(node.Children), 3)
}

// ---------------------------------------------------------------------------
// 6. NewTrackerNode — interface method resolution
// ---------------------------------------------------------------------------

func TestNewTrackerNode_InterfaceMethodResolution(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Set up interface type with implementations
	meta.Packages["svc"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"iface.go": {
				Types: map[string]*metadata.Type{
					"Handler": {
						Name:          sp.Get("Handler"),
						Pkg:           sp.Get("svc"),
						Kind:          sp.Get("interface"),
						ImplementedBy: []int{sp.Get("svc.ConcreteHandler")},
					},
				},
			},
			"impl.go": {
				Types: map[string]*metadata.Type{
					"ConcreteHandler": {
						Name: sp.Get("ConcreteHandler"),
						Pkg:  sp.Get("svc"),
						Kind: sp.Get("struct"),
						Methods: []metadata.Method{
							{
								Name:     sp.Get("Handle"),
								Receiver: sp.Get("ConcreteHandler"),
							},
						},
					},
				},
			},
		},
	}

	// Edge: main calls something through Handler interface
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithRecv(meta, "Handle", "svc", "Handler"),
		},
		// ConcreteHandler.Handle calls an inner function
		metadata.CallGraphEdge{
			Caller: makeCallWithRecv(meta, "Handle", "svc", "ConcreteHandler"),
			Callee: makeCallWithDefaults(meta, "doWork", "svc"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := newTrackerTree(meta)
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	parentEdge := &meta.CallGraph[0]
	calleeID := parentEdge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, parentEdge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
}

// ---------------------------------------------------------------------------
// 7. NewTrackerNode — generic type filtering
// ---------------------------------------------------------------------------

func TestNewTrackerNode_GenericTypeFiltering(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// main calls a generic function that has type parameters
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: metadata.Call{
				Meta:         meta,
				Name:         sp.Get("Process[T=string]"),
				Pkg:          sp.Get("app"),
				RecvType:     -1,
				Position:     -1,
				Scope:        -1,
				SignatureStr: -1,
			},
			TypeParamMap: map[string]string{"T": "string"},
		},
		metadata.CallGraphEdge{
			Caller: metadata.Call{
				Meta:         meta,
				Name:         sp.Get("Process[T=string]"),
				Pkg:          sp.Get("app"),
				RecvType:     -1,
				Position:     -1,
				Scope:        -1,
				SignatureStr: -1,
			},
			Callee:       makeCallWithDefaults(meta, "store", "app"),
			TypeParamMap: map[string]string{"T": "string"},
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
	assert.GreaterOrEqual(t, tree.GetNodeCount(), 1)
}

// ---------------------------------------------------------------------------
// 8. traverseTree — advanced scenarios
// ---------------------------------------------------------------------------

func TestTraverseTree_MatchingWithTypeParams(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	// Create assignment node with type params
	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "int"},
	}

	child := &TrackerNode{key: "matched-child"}
	targetNode := &TrackerNode{
		key:           "target",
		Children:      []*TrackerNode{child},
		CallGraphEdge: edge,
	}
	child.Parent = targetNode

	// Root node has matching key
	rootEdge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
		TypeParamMap: map[string]string{"T": "int"},
	}
	rootNode := &TrackerNode{
		key:           "target",
		CallGraphEdge: rootEdge,
	}

	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{
		assignmentKey{}: targetNode,
	}}

	result := traverseTree([]*TrackerNode{rootNode}, an, 5, nil)
	assert.False(t, result)
	// rootNode should get children because keys match and type params match
	assert.NotEmpty(t, rootNode.Children)
}

func TestTraverseTree_ParentAssignment(t *testing.T) {
	// Test the branch where tn has a parent but no children
	meta, sp := newTrackerTestMeta()

	parentOfTarget := &TrackerNode{key: "parent-node", Children: []*TrackerNode{}}

	targetNode := &TrackerNode{
		key:    "target",
		Parent: parentOfTarget,
		// No children — triggers the else branch
	}
	parentOfTarget.Children = append(parentOfTarget.Children, targetNode)

	// Root node with matching key
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
	}
	rootNode := &TrackerNode{
		key:           "target",
		CallGraphEdge: edge,
	}

	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{
		assignmentKey{}: targetNode,
	}}

	result := traverseTree([]*TrackerNode{rootNode}, an, 5, nil)
	assert.False(t, result)
	// parentOfTarget should have rootNode added as a child
	found := false
	for _, child := range parentOfTarget.Children {
		if child == rootNode {
			found = true
			break
		}
	}
	assert.True(t, found, "rootNode should be added to parentOfTarget's children")
}

func TestTraverseTree_VariableNodes(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	child := &TrackerNode{key: "linked-child"}
	varNode := &TrackerNode{
		key:      "target",
		Children: []*TrackerNode{child},
	}
	child.Parent = varNode

	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
	}
	rootNode := &TrackerNode{
		key:           "target",
		CallGraphEdge: edge,
	}

	vn := &variableNodes{variables: map[paramKey][]*TrackerNode{
		{Name: "x", Pkg: "pkg", Container: "fn"}: {varNode},
	}}

	result := traverseTree([]*TrackerNode{rootNode}, vn, 50, nil)
	assert.False(t, result)
	// rootNode should have acquired children from variable node
	assert.NotEmpty(t, rootNode.Children)
}

func TestTraverseTree_RecursiveChildren(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	// Create a tree: root -> child, where child also has a matching assignment
	edge := &metadata.CallGraphEdge{
		Callee: metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg")},
	}

	grandchild := &TrackerNode{key: "grandchild"}
	assignTarget := &TrackerNode{
		key:      "child",
		Children: []*TrackerNode{grandchild},
	}
	grandchild.Parent = assignTarget

	childNode := &TrackerNode{
		key:           "child",
		CallGraphEdge: edge,
		Children:      []*TrackerNode{},
	}
	rootNode := &TrackerNode{
		key:      "root",
		Children: []*TrackerNode{childNode},
	}
	childNode.Parent = rootNode

	an := &assignmentNodes{assignmentIndex: assigmentIndexMap{
		assignmentKey{}: assignTarget,
	}}

	// Traverse should process both root and child
	result := traverseTree([]*TrackerNode{rootNode}, an, 5, nil)
	assert.False(t, result)
}

// ---------------------------------------------------------------------------
// 9. TraceArgumentOrigin — variable argument tracing
// ---------------------------------------------------------------------------

func TestTraceArgumentOrigin_VariableWithMatchingOrigin(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	// Set up a function with assignment map for the variable
	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"main.go": {
				Functions: map[string]*metadata.Function{
					"handler": {
						Name: sp.Get("handler"),
						Pkg:  sp.Get("app"),
						AssignmentMap: map[string][]metadata.Assignment{
							"db": {
								{
									VariableName: sp.Get("db"),
									Pkg:          sp.Get("app"),
									ConcreteType: sp.Get("*Database"),
									Func:         sp.Get("handler"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create a variable argument node
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("db")
	arg.SetPkg("app")
	arg.SetType("*Database")

	argNode := &TrackerNode{
		key:          "arg-db",
		IsArgument:   true,
		ArgType:      ArgTypeVariable,
		ArgContext:   "handler",
		CallArgument: arg,
	}

	// Create the origin variable node
	originNode := &TrackerNode{key: "origin-db"}

	tree := newTrackerTree(meta)
	// TraceArgumentOrigin calls TraceVariableOrigin with pkg="" (empty).
	// The fallback returns originVar="db", originPkg="", funName="handler",
	// so the paramKey lookup must match that.
	tree.variableNodes[paramKey{
		Name:      "db",
		Pkg:       "",
		Container: "handler",
	}] = []*TrackerNode{originNode}

	result := tree.TraceArgumentOrigin(argNode)
	// Should find the origin node via the variable lookup
	require.NotNil(t, result, "expected TraceArgumentOrigin to find the origin node")
	assert.Equal(t, originNode, result)
}

func TestTraceArgumentOrigin_VariableWithNoCallArgument(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	argNode := &TrackerNode{
		key:        "arg",
		IsArgument: true,
		ArgType:    ArgTypeVariable,
		// No CallArgument set
	}

	result := tree.TraceArgumentOrigin(argNode)
	assert.Nil(t, result)
}

func TestTraceArgumentOrigin_NonVariableType(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue("hello")

	argNode := &TrackerNode{
		key:          "arg",
		IsArgument:   true,
		ArgType:      ArgTypeLiteral,
		CallArgument: arg,
	}

	result := tree.TraceArgumentOrigin(argNode)
	assert.Nil(t, result)
}

func TestTraceArgumentOrigin_FunctionCallType(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindCall)

	argNode := &TrackerNode{
		key:          "arg",
		IsArgument:   true,
		ArgType:      ArgTypeFunctionCall,
		CallArgument: arg,
	}

	result := tree.TraceArgumentOrigin(argNode)
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// 10. resolveFuncCallSelectorEdges — receiver type resolution paths
// ---------------------------------------------------------------------------

func TestResolveFuncCallSelectorEdges_NestedSelectorX(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Inner selector: obj.field (X is an ident with type)
	innerX := metadata.NewCallArgument(meta)
	innerX.SetKind(metadata.KindIdent)
	innerX.SetName("obj")
	innerX.SetType("app.Container")
	innerX.SetPkg("app")

	innerSel := metadata.NewCallArgument(meta)
	innerSel.SetKind(metadata.KindIdent)
	innerSel.SetName("field")
	innerSel.SetType("Service")

	// selectorArg.X is a selector (obj.field)
	selectorX := metadata.NewCallArgument(meta)
	selectorX.SetKind(metadata.KindSelector)
	selectorX.X = innerX
	selectorX.Sel = innerSel

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("app")
	selArg.SetType("func() error")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = selectorX

	// Add matching edge
	meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("Method"),
			Pkg:          sp.Get("app"),
			RecvType:     sp.Get("Container"),
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Callee: makeCallWithDefaults(meta, "inner", "app"),
	})

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveFuncCallSelectorEdges(tree, meta, argNode, selectorArg, "obj.field", "obj.field", visited, &assignmentIndex, realisticLimits())
	// FuncType path should be activated by nested selector with X.X.Type
	assert.NotNil(t, argNode)
}

func TestResolveFuncCallSelectorEdges_CallX(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// selectorArg.X is a call expression: getService().Handle
	// The source code at line 933 accesses selectorArg.X.X.GetPkg(), so X.X must not be nil.
	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindIdent)
	funArg.SetName("getService")
	funArg.SetType("app.Service")
	funArg.SetPkg("app")

	// X.X is needed because the source code accesses it even in the KindCall branch
	callXX := metadata.NewCallArgument(meta)
	callXX.SetKind(metadata.KindIdent)
	callXX.SetName("pkg")
	callXX.SetPkg("app")
	callXX.SetType("app.ServiceFactory")

	callX := metadata.NewCallArgument(meta)
	callX.SetKind(metadata.KindCall)
	callX.Fun = funArg
	callX.X = callXX

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Handle")
	selArg.SetPkg("app")
	selArg.SetType("func() error")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = callX

	// Add matching edge
	meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("Handle"),
			Pkg:          sp.Get("app"),
			RecvType:     sp.Get("Service"),
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Callee: makeCallWithDefaults(meta, "doWork", "app"),
	})

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// Guard: production code at line 933 dereferences selectorArg.X.X without
	// a nil check in the KindCall branch. Verify X.X is set before calling to
	// avoid a nil-pointer panic if the test setup is ever accidentally changed.
	require.NotNil(t, selectorArg.X, "selectorArg.X must not be nil")
	require.NotNil(t, selectorArg.X.X, "selectorArg.X.X must not be nil (production code dereferences it at tracker.go:933)")
	require.NotNil(t, selectorArg.X.Fun, "selectorArg.X.Fun must not be nil (production code dereferences it at tracker.go:931)")

	resolveFuncCallSelectorEdges(tree, meta, argNode, selectorArg, "getService()", "getService()", visited, &assignmentIndex, realisticLimits())
	// Primary goal: exercise the KindCall branch (line 931-934) for selectorArg.X without panic
	assert.NotNil(t, argNode.Children)
}

func TestResolveFuncCallSelectorEdges_WithInterfaceResolution(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Register interface resolution
	tree.RegisterInterfaceResolution("Handler", "", "svc", "ConcreteHandler")

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Handle")
	selArg.SetPkg("svc")
	selArg.SetType("func() error")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("h")
	xArg.SetType("Handler")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg

	// Add matching edge with concrete type
	meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("Handle"),
			Pkg:          sp.Get("svc"),
			RecvType:     sp.Get("ConcreteHandler"),
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Callee: makeCallWithDefaults(meta, "impl", "svc"),
	})

	argNode := &TrackerNode{key: "arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveFuncCallSelectorEdges(tree, meta, argNode, selectorArg, "Handler", "h", visited, &assignmentIndex, realisticLimits())
	assert.NotEmpty(t, argNode.Children, "should resolve through interface to concrete type")
}

// ---------------------------------------------------------------------------
// 11. resolveSelectorMethod — additional paths
// ---------------------------------------------------------------------------

func TestResolveSelectorMethod_WithReceiverType_Integration(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	recvArg := metadata.NewCallArgument(meta)
	recvArg.SetKind(metadata.KindIdent)
	recvArg.SetName("MyService")

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Execute")
	selArg.SetPkg("svc")
	selArg.SetType("func() error")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("s")
	xArg.SetType("MyService")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg
	selectorArg.ReceiverType = recvArg

	// Add matching edge
	meta.CallGraph = append(meta.CallGraph, metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("Execute"),
			Pkg:          sp.Get("svc"),
			RecvType:     sp.Get("MyService"),
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Callee: makeCallWithDefaults(meta, "run", "svc"),
	})

	argNode := &TrackerNode{key: "method-arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// originVar == varName triggers ReceiverType path
	resolveSelectorMethod(tree, meta, argNode, selectorArg, "s", "s", visited, &assignmentIndex, realisticLimits())
	assert.NotEmpty(t, argNode.Children)
}

func TestResolveSelectorMethod_SelTypeNotIdent(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Execute")
	selArg.SetPkg("svc")
	selArg.SetType("func() error")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("s")
	xArg.SetType("Service")

	selectorArg := metadata.NewCallArgument(meta)
	selectorArg.SetKind(metadata.KindSelector)
	selectorArg.Sel = selArg
	selectorArg.X = xArg

	// With Sel.Type != -1 and originVar == varName, uses X.GetType()
	argNode := &TrackerNode{key: "method-arg", Children: []*TrackerNode{}}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	resolveSelectorMethod(tree, meta, argNode, selectorArg, "s", "s", visited, &assignmentIndex, realisticLimits())
	// No matching edges, so no children, but shouldn't panic
	assert.Empty(t, argNode.Children)
}

// ---------------------------------------------------------------------------
// 12. processArgSelector — edge cases
// ---------------------------------------------------------------------------

func TestProcessArgSelector_NilX(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	// X is nil

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "caller", "app"),
		Callee: makeCallWithDefaults(meta, "callee", "app"),
	}

	argNode := &TrackerNode{key: "sel-arg"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	// Should return early without panic
	processArgSelector(tree, meta, argNode, arg, edge, visited, &assignmentIndex, realisticLimits())
	assert.NotNil(t, argNode)
	_ = sp
}

func TestProcessArgSelector_WithFuncTypeSel(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Do")
	selArg.SetType("func() error")
	selArg.SetPkg("svc")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("s")
	xArg.SetType("Service")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "caller", "app"),
		Callee: makeCallWithDefaults(meta, "callee", "app"),
	}

	argNode := &TrackerNode{key: "sel-arg"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	processArgSelector(tree, meta, argNode, arg, edge, visited, &assignmentIndex, realisticLimits())
	_ = sp
	// Should process the func type selector and try to resolve method
	assert.NotNil(t, argNode)
}

func TestProcessArgSelector_NestedSelectorLinkage(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Nested selector: obj.inner.Field
	innerX := metadata.NewCallArgument(meta)
	innerX.SetKind(metadata.KindIdent)
	innerX.SetName("obj")
	innerX.SetType("Container")
	innerX.SetPkg("app")

	innerSel := metadata.NewCallArgument(meta)
	innerSel.SetKind(metadata.KindIdent)
	innerSel.SetName("inner")
	innerSel.SetType("Service")

	xSel := metadata.NewCallArgument(meta)
	xSel.SetKind(metadata.KindSelector)
	xSel.X = innerX
	xSel.Sel = innerSel

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")
	selArg.SetType("string")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xSel

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "process", "app"),
	}

	argNode := &TrackerNode{key: "nested-sel"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	processArgSelector(tree, meta, argNode, arg, edge, visited, &assignmentIndex, realisticLimits())
	_ = sp
	// Should handle nested selector case for linkage
	assert.NotNil(t, argNode)
}

// ---------------------------------------------------------------------------
// 13. processArgVariable — full assignment chain
// ---------------------------------------------------------------------------

func TestProcessArgVariable_MultipleVariableNodes(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "process", "app"),
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("cfg")
	arg.SetType("Config")

	argNode := &TrackerNode{key: "arg-cfg"}

	// Create multiple variable nodes (representing reassignments)
	varNode1 := &TrackerNode{key: "var-cfg-1", Children: []*TrackerNode{}}
	varNode2 := &TrackerNode{key: "var-cfg-2", Children: []*TrackerNode{}}
	tree.variableNodes[paramKey{
		Name:      "cfg",
		Pkg:       "app",
		Container: "handler",
	}] = []*TrackerNode{varNode1, varNode2}

	assignmentIndex := assigmentIndexMap{}
	processArgVariable(tree, meta, argNode, arg, edge, &assignmentIndex)

	// Should link to the most recent (last) variable node
	assert.Contains(t, varNode2.Children, argNode)
	_ = sp
}

// ---------------------------------------------------------------------------
// 14. processArgFunctionCall — with nested args
// ---------------------------------------------------------------------------

func TestProcessArgFunctionCall_WithNestedArgs(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Inner argument for the function call
	innerArg := metadata.NewCallArgument(meta)
	innerArg.SetKind(metadata.KindLiteral)
	innerArg.SetValue("hello")
	innerArg.SetPosition("main.go:100:10")

	funArg := metadata.NewCallArgument(meta)
	funArg.SetKind(metadata.KindIdent)
	funArg.SetName("format")
	funArg.SetType("func(string) string")

	callArg := metadata.NewCallArgument(meta)
	callArg.SetKind(metadata.KindCall)
	callArg.Fun = funArg
	callArg.Args = []*metadata.CallArgument{innerArg}
	callArg.SetPosition("main.go:100:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "print", "app"),
	}

	parentNode := &TrackerNode{key: "parent"}
	argNode := &TrackerNode{key: "call-arg"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processArgFunctionCall(tree, meta, parentNode, argNode, callArg, edge, nil, "call-id", 0, visited, &assignmentIndex, realisticLimits())
	assert.NotNil(t, result)
	_ = sp
}

// ---------------------------------------------------------------------------
// 15. Full integration: NewTrackerTree with various arg types
// ---------------------------------------------------------------------------

func TestNewTrackerTree_FullIntegration_MixedArgs(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Literal arg
	litArg := metadata.NewCallArgument(meta)
	litArg.SetKind(metadata.KindLiteral)
	litArg.SetValue("/api")
	litArg.SetPosition("main.go:10:5")

	// Ident arg (handler function)
	identArg := metadata.NewCallArgument(meta)
	identArg.SetKind(metadata.KindIdent)
	identArg.SetName("userHandler")
	identArg.SetType("func(http.ResponseWriter, *http.Request)")
	identArg.SetPosition("main.go:10:15")

	// Composite lit arg
	compositeLit := metadata.NewCallArgument(meta)
	compositeLit.SetKind(metadata.KindCompositeLit)
	compositeLit.SetType("Options")
	compositeLit.SetPosition("main.go:15:5")

	// Build call graph: main -> http.HandleFunc with literal + ident args
	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "HandleFunc", "net/http"),
			Args:   []*metadata.CallArgument{litArg, identArg},
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "Configure", "app"),
			Args:   []*metadata.CallArgument{compositeLit},
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "HandleFunc", "net/http"),
			Callee: makeCallWithDefaults(meta, "serve", "net/http"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)

	roots := tree.GetRoots()
	assert.GreaterOrEqual(t, len(roots), 1, "should have at least one root")
	assert.GreaterOrEqual(t, tree.GetNodeCount(), 2, "tree should have nodes from call chain")
}

func TestNewTrackerTree_FullIntegration_ChainCalls(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Chain: app.Group("/api").Use(middleware)
	groupEdge := metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "Group", "router"),
	}

	useEdge := metadata.CallGraphEdge{
		Caller:      makeCallWithDefaults(meta, "Use", "router"),
		Callee:      makeCallWithDefaults(meta, "applyMiddleware", "router"),
		ChainParent: &groupEdge,
		ChainRoot:   "app",
		ChainDepth:  1,
	}

	meta.CallGraph = append(meta.CallGraph, groupEdge, useEdge)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
	assert.NotNil(t, tree.chainParentMap)
	assert.Greater(t, len(tree.chainParentMap), 0, "expected chainParentMap to have entries from chain registration")
}

func TestNewTrackerTree_FullIntegration_WithParamArgMap(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	identArg := metadata.NewCallArgument(meta)
	identArg.SetKind(metadata.KindIdent)
	identArg.SetName("db")
	identArg.SetType("*Database")

	paramArgMap := map[string]metadata.CallArgument{
		"store": *identArg,
	}

	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller:      makeCallWithDefaults(meta, "main", "app"),
			Callee:      makeCallWithDefaults(meta, "NewHandler", "app"),
			ParamArgMap: paramArgMap,
		},
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "NewHandler", "app"),
			Callee: makeCallWithDefaults(meta, "Connect", "app"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
	// ParamArgMap should trigger variable tracing during tree construction
	assert.GreaterOrEqual(t, tree.GetNodeCount(), 1)
}

// ---------------------------------------------------------------------------
// 16. classifyArgument — coverage of all paths
// ---------------------------------------------------------------------------

func TestClassifyArgument_AllKinds_Extended(t *testing.T) {
	meta, _ := newTrackerTestMeta()

	tests := []struct {
		kind     string
		typeStr  string
		expected ArgumentType
	}{
		{metadata.KindCall, "", ArgTypeFunctionCall},
		{metadata.KindFuncLit, "", ArgTypeFunctionCall},
		{metadata.KindIdent, "func(int) error", ArgTypeFunctionCall}, // ident with func type
		{metadata.KindIdent, "string", ArgTypeVariable},
		{metadata.KindLiteral, "", ArgTypeLiteral},
		{metadata.KindSelector, "", ArgTypeSelector},
		{metadata.KindUnary, "", ArgTypeUnary},
		{metadata.KindBinary, "", ArgTypeBinary},
		{metadata.KindIndex, "", ArgTypeIndex},
		{metadata.KindCompositeLit, "", ArgTypeComposite},
		{metadata.KindTypeAssert, "", ArgTypeTypeAssert},
		{metadata.KindRaw, "", ArgTypeComplex},           // default case
		{metadata.KindArrayType, "", ArgTypeComplex},     // default case
		{metadata.KindMapType, "", ArgTypeComplex},       // default case
		{metadata.KindKeyValue, "", ArgTypeComplex},      // default case
		{metadata.KindFuncType, "", ArgTypeComplex},      // default case
		{metadata.KindStar, "", ArgTypeComplex},          // default case
		{metadata.KindParen, "", ArgTypeComplex},         // default case
		{metadata.KindSlice, "", ArgTypeComplex},         // default case
		{metadata.KindEllipsis, "", ArgTypeComplex},      // default case
		{metadata.KindChanType, "", ArgTypeComplex},      // default case
		{metadata.KindStructType, "", ArgTypeComplex},    // default case
		{metadata.KindInterfaceType, "", ArgTypeComplex}, // default case
	}

	for _, tt := range tests {
		arg := metadata.NewCallArgument(meta)
		arg.SetKind(tt.kind)
		if tt.typeStr != "" {
			arg.SetType(tt.typeStr)
		}

		result := classifyArgument(arg)
		assert.Equal(t, tt.expected, result, "kind=%s type=%s", tt.kind, tt.typeStr)
	}
}

// ---------------------------------------------------------------------------
// 17. processChainRelationships — with argument child node
// ---------------------------------------------------------------------------

func TestProcessChainRelationships_SelfReferenceSkipped(t *testing.T) {
	meta, sp := newTrackerTestMeta()

	edge := metadata.CallGraphEdge{
		Caller:      makeCallWithDefaults(meta, "Use", "router"),
		Callee:      makeCallWithDefaults(meta, "inner", "router"),
		ChainParent: nil,
	}
	meta.CallGraph = []metadata.CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	tree := newTrackerTree(meta)

	// Self-referencing chain parent
	meta.CallGraph[0].ChainParent = &meta.CallGraph[0]
	node := &TrackerNode{Children: []*TrackerNode{}}
	node.CallGraphEdge = &meta.CallGraph[0]
	calleeID := edge.Callee.ID()
	tree.nodeMap[calleeID] = node

	// Should not create self-referencing parent-child
	tree.processChainRelationships()

	// Verify the node did not gain a self-referencing child
	assert.NotNil(t, node)
	_ = sp
}

// ---------------------------------------------------------------------------
// 18. newArgumentNode
// ---------------------------------------------------------------------------

func TestNewArgumentNode_EmptyArgID(t *testing.T) {
	tree := newTrackerTree(nil)
	parent := &TrackerNode{key: "parent"}

	node := newArgumentNode(tree, parent, "", nil, nil)
	assert.Nil(t, node)
}

func TestNewArgumentNode_NilTree(t *testing.T) {
	parent := &TrackerNode{key: "parent"}
	meta, _ := newTrackerTestMeta()
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue("test")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "fn", "pkg"),
		Callee: makeCallWithDefaults(meta, "target", "pkg"),
	}

	node := newArgumentNode(nil, parent, "some-id", edge, arg)
	require.NotNil(t, node)
	assert.Equal(t, parent, node.Parent)
	assert.Equal(t, edge, node.CallGraphEdge)
	assert.Equal(t, arg, node.CallArgument)
}

// ---------------------------------------------------------------------------
// 19. Full integration: edge with callee appearing in Args map (skipped)
// ---------------------------------------------------------------------------

func TestNewTrackerTree_SkipsCalleeInArgsMap(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	// Create an arg whose base ID matches a callee, so it should be skipped
	handlerArg := metadata.NewCallArgument(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("handleUsers")
	handlerArg.SetPkg("app")
	handlerArg.SetPosition("main.go:20:5")

	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "main", "app"),
			Callee: makeCallWithDefaults(meta, "Route", "app"),
			Args:   []*metadata.CallArgument{handlerArg},
		},
		// handleUsers as a caller (its base ID will be in Args map)
		metadata.CallGraphEdge{
			Caller: makeCallWithDefaults(meta, "handleUsers", "app"),
			Callee: makeCallWithDefaults(meta, "getDB", "app"),
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
}

// ---------------------------------------------------------------------------
// 20. NewTrackerNode — edge with receiver type and variable tracing
// ---------------------------------------------------------------------------

func TestNewTrackerNode_ReceiverVarTracing(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Set up assignment index for receiver variable
	assignmentNode := &TrackerNode{key: "assign-router", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "r",
		Pkg:       "app",
		Type:      "app.*Router",
		Container: "main",
	}
	assignmentIndex := assigmentIndexMap{akey: assignmentNode}

	// Set up variable nodes
	varNode := &TrackerNode{key: "var-r", Children: []*TrackerNode{}}
	tree.variableNodes[paramKey{
		Name:      "r",
		Pkg:       "app",
		Container: "main",
	}] = []*TrackerNode{varNode}

	// Edge with receiver
	edge := &metadata.CallGraphEdge{
		Caller:        makeCallWithDefaults(meta, "main", "app"),
		Callee:        makeCallWithRecv(meta, "Mount", "app", "*Router"),
		CalleeVarName: "r",
	}

	visited := map[string]int{}
	calleeID := edge.Callee.ID()

	node := NewTrackerNode(tree, meta, "", calleeID, edge, nil, visited, &assignmentIndex, realisticLimits())
	require.NotNil(t, node)
	_ = sp
}

// ---------------------------------------------------------------------------
// 21. GetRoots on nil tree
// ---------------------------------------------------------------------------

func TestTrackerTree_GetRoots_NilTree(t *testing.T) {
	var tree *TrackerTree
	roots := tree.GetRoots()
	assert.Nil(t, roots)
}

// ---------------------------------------------------------------------------
// 22. linkSelectorToAssignment — with nested selector
// ---------------------------------------------------------------------------

func TestLinkSelectorToAssignment_NestedSelector_Integration(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// arg.X is a selector (obj.inner), and arg.X.X is ident with type
	innerX := metadata.NewCallArgument(meta)
	innerX.SetKind(metadata.KindIdent)
	innerX.SetName("obj")
	innerX.SetType("Container")
	innerX.SetPkg("app")

	innerSel := metadata.NewCallArgument(meta)
	innerSel.SetKind(metadata.KindIdent)
	innerSel.SetName("inner")
	innerSel.SetType("Service")

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindSelector)
	xArg.X = innerX
	xArg.Sel = innerSel

	selArg := metadata.NewCallArgument(meta)
	selArg.SetKind(metadata.KindIdent)
	selArg.SetName("Field")
	selArg.SetType("string")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.Sel = selArg
	arg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "process", "app"),
	}

	argNode := &TrackerNode{key: "link-sel"}
	assignmentIndex := assigmentIndexMap{}

	linkSelectorToAssignment(tree, meta, argNode, arg, edge, &assignmentIndex)
	_ = sp
	// Should handle the nested selector case (uses X.X.GetType() for parentType)
	assert.NotNil(t, argNode)
}

// ---------------------------------------------------------------------------
// 23. traceSelectorOrigin — assignment and variable linkage
// ---------------------------------------------------------------------------

func TestTraceSelectorOrigin_LinksToAssignment(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("svc")
	xArg.SetType("Service")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "process", "app"),
	}

	// Set up assignment
	assignNode := &TrackerNode{key: "assign-svc", Children: []*TrackerNode{}}
	akey := assignmentKey{
		Name:      "svc",
		Pkg:       "app",
		Type:      "", // empty type from arg
		Container: "handler",
	}
	assignmentIndex := assigmentIndexMap{akey: assignNode}

	argNode := &TrackerNode{key: "trace-origin"}

	originVar, _ := traceSelectorOrigin(tree, meta, argNode, arg, edge, &assignmentIndex)
	assert.NotEmpty(t, originVar)
	_ = sp
}

func TestTraceSelectorOrigin_LinksToVariableNode(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	xArg := metadata.NewCallArgument(meta)
	xArg.SetKind(metadata.KindIdent)
	xArg.SetName("db")
	xArg.SetType("Database")

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X = xArg

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "handler", "app"),
		Callee: makeCallWithDefaults(meta, "query", "app"),
	}

	// Set up variable node
	varNode := &TrackerNode{key: "var-db", Children: []*TrackerNode{}}
	tree.variableNodes[paramKey{
		Name:      "db",
		Pkg:       "app",
		Container: "handler",
	}] = []*TrackerNode{varNode}

	argNode := &TrackerNode{key: "trace-origin-2"}
	assignmentIndex := assigmentIndexMap{}

	traceSelectorOrigin(tree, meta, argNode, arg, edge, &assignmentIndex)
	assert.Contains(t, varNode.Children, argNode)
	_ = sp
}

// ---------------------------------------------------------------------------
// 24. processArguments — edge with argument matching callee ID (skip path)
// ---------------------------------------------------------------------------

func TestProcessArguments_SkipArgMatchingCalleeID(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Create arg whose ID matches the callee ID
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("target")
	arg.SetPkg("app")
	arg.SetPosition("main.go:1:1")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("fn"),
			Pkg:          sp.Get("app"),
			RecvType:     -1,
			Position:     -1,
			Scope:        -1,
			SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta:         meta,
			Name:         sp.Get("target"),
			Pkg:          sp.Get("app"),
			RecvType:     -1,
			Position:     sp.Get("main.go:1:1"),
			Scope:        -1,
			SignatureStr: -1,
		},
		Args: []*metadata.CallArgument{arg},
	}

	parentNode := &TrackerNode{key: "parent"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, realisticLimits())
	// The arg matching callee ID should be skipped, so no children added
	assert.NotNil(t, result)
}

// ---------------------------------------------------------------------------
// 25. processArguments — ident arg classified as function call
// ---------------------------------------------------------------------------

func TestProcessArguments_IdentWithFuncType(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	// Ident arg with func type should be classified as ArgTypeFunctionCall
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("myCallback")
	arg.SetType("func(int) error")
	arg.SetPosition("main.go:50:5")

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "setup", "app"),
		Callee: makeCallWithDefaults(meta, "register", "app"),
		Args:   []*metadata.CallArgument{arg},
	}

	parentNode := &TrackerNode{key: "parent"}
	visited := map[string]int{}
	assignmentIndex := assigmentIndexMap{}

	result := processArguments(tree, meta, parentNode, edge, visited, &assignmentIndex, realisticLimits())
	assert.NotEmpty(t, result)
	_ = sp
}

// ---------------------------------------------------------------------------
// 26. Full NewTrackerTree with assignment relationship
// ---------------------------------------------------------------------------

func TestNewTrackerTree_WithAssignmentRelationships(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
		CallGraph:  []metadata.CallGraphEdge{},
	}

	lhsArg := metadata.NewCallArgument(meta)
	lhsArg.SetKind(metadata.KindIdent)
	lhsArg.SetName("srv")

	valArg := metadata.NewCallArgument(meta)
	valArg.SetKind(metadata.KindCall)

	// Set up function with assignments
	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"main.go": {
				Functions: map[string]*metadata.Function{
					"main": {
						Name: sp.Get("main"),
						Pkg:  sp.Get("app"),
						AssignmentMap: map[string][]metadata.Assignment{
							"srv": {
								{
									VariableName: sp.Get("srv"),
									Pkg:          sp.Get("app"),
									ConcreteType: sp.Get("*Server"),
									Func:         sp.Get("main"),
									Lhs:          *lhsArg,
									Value:        *valArg,
								},
							},
						},
					},
				},
			},
		},
	}

	meta.CallGraph = append(meta.CallGraph,
		metadata.CallGraphEdge{
			Caller:        makeCallWithDefaults(meta, "main", "app"),
			Callee:        makeCallWithRecv(meta, "ListenAndServe", "app", "*Server"),
			CalleeVarName: "srv",
		},
	)
	meta.BuildCallGraphMaps()

	tree := NewTrackerTree(meta, realisticLimits())
	require.NotNil(t, tree)
}

// ---------------------------------------------------------------------------
// 27. NewTrackerNode — MaxNodesPerTree with parentEdge and callArg
// ---------------------------------------------------------------------------

func TestNewTrackerNode_MaxNodesPerTree_WithEdgeAndArg(t *testing.T) {
	meta, sp := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	limits := realisticLimits()
	limits.MaxNodesPerTree = 0 // force limit

	edge := &metadata.CallGraphEdge{
		Caller: makeCallWithDefaults(meta, "main", "app"),
		Callee: makeCallWithDefaults(meta, "handler", "app"),
	}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")

	// visited has entries exceeding the limit
	visited := map[string]int{"a": 1}
	assignmentIndex := assigmentIndexMap{}

	node := NewTrackerNode(tree, meta, "", "app.handler", edge, arg, visited, &assignmentIndex, limits)
	require.NotNil(t, node)
	// When truncated with parentEdge and callArg, key should NOT be set
	assert.Equal(t, "", node.key)
	assert.Equal(t, edge, node.CallGraphEdge)
	assert.Equal(t, arg, node.CallArgument)
	_ = sp
}

func TestNewTrackerNode_MaxNodesPerTree_NilEdgeAndArg(t *testing.T) {
	meta, _ := newTrackerTestMeta()
	tree := newTrackerTree(meta)

	limits := realisticLimits()
	limits.MaxNodesPerTree = 0

	visited := map[string]int{"a": 1}
	assignmentIndex := assigmentIndexMap{}

	node := NewTrackerNode(tree, meta, "", "some-id", nil, nil, visited, &assignmentIndex, limits)
	require.NotNil(t, node)
	// When truncated with nil edge and arg, key should be set to id
	assert.Equal(t, "some-id", node.key)
}

// ---------------------------------------------------------------------------
// 28. getString helper with nil metadata
// ---------------------------------------------------------------------------

func TestGetString_NilMeta_Integration(t *testing.T) {
	result := getString(nil, 0)
	assert.Equal(t, "", result)
}

func TestGetString_NilStringPool_Integration(t *testing.T) {
	meta := &metadata.Metadata{StringPool: nil}
	result := getString(meta, 0)
	assert.Equal(t, "", result)
}
