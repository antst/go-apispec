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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// ---------------------------------------------------------------------------
// ensureParentFunctionNode
// ---------------------------------------------------------------------------

func TestEnsureParentFunctionNode_NilParentFunc(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	nc := 0

	result := ensureParentFunctionNode(meta, nil, data, visited, &nc)
	assert.Equal(t, "", result)
	assert.Empty(t, data.Nodes)
}

func TestEnsureParentFunctionNode_NewNode(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	nc := 0

	parentFunc := &metadata.Call{
		Meta:         meta,
		Name:         sp.Get("handleRequest"),
		Pkg:          sp.Get("mypkg"),
		RecvType:     sp.Get("Server"),
		Position:     -1,
		Scope:        -1,
		SignatureStr: sp.Get("func()"),
	}

	result := ensureParentFunctionNode(meta, parentFunc, data, visited, &nc)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "Server.handleRequest", data.Nodes[0].Data.Label)
	assert.Equal(t, "function", data.Nodes[0].Data.Type)
	assert.Equal(t, "true", data.Nodes[0].Data.IsParentFunction)
	assert.Equal(t, "mypkg", data.Nodes[0].Data.Package)
}

func TestEnsureParentFunctionNode_AlreadyExists(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	parentFunc := &metadata.Call{
		Meta:         meta,
		Name:         sp.Get("handleRequest"),
		Pkg:          sp.Get("mypkg"),
		RecvType:     -1,
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}

	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{
				Data: CytoscapeNodeData{
					ID:           "node_1",
					Label:        "handleRequest",
					FunctionName: "handleRequest",
					Package:      "mypkg",
				},
			},
		},
		Edges: make([]CytoscapeEdge, 0),
	}

	// Mark as already visited
	visited := map[string]bool{
		parentFunc.BaseID(): true,
	}
	nc := 1

	result := ensureParentFunctionNode(meta, parentFunc, data, visited, &nc)
	assert.Equal(t, "node_1", result)
	// Should mark existing node as parent function
	assert.Equal(t, "true", data.Nodes[0].Data.IsParentFunction)
	// Should not add new nodes
	assert.Len(t, data.Nodes, 1)
}

func TestEnsureParentFunctionNode_MainFunction(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	nc := 0

	parentFunc := &metadata.Call{
		Meta:         meta,
		Name:         sp.Get("main"),
		Pkg:          sp.Get("main"),
		RecvType:     -1,
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}

	result := ensureParentFunctionNode(meta, parentFunc, data, visited, &nc)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "root function", data.Nodes[0].Data.Position)
}

// ---------------------------------------------------------------------------
// drawNodeCytoscapeWithDepth
// ---------------------------------------------------------------------------

func TestDrawNodeCytoscapeWithDepth_NilNode(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	result := drawNodeCytoscapeWithDepth(nil, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.Equal(t, "", result)
}

func TestDrawNodeCytoscapeWithDepth_SimpleNode(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	node := &TrackerNode{key: "main.handler"}
	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, 0, data.Nodes[0].Data.Depth)
}

func TestDrawNodeCytoscapeWithDepth_WithChildren(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	child := &TrackerNode{key: "child"}
	parent := &TrackerNode{key: "parent", Children: []*TrackerNode{child}}
	child.Parent = parent

	result := drawNodeCytoscapeWithDepth(parent, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	assert.Len(t, data.Nodes, 2)
	assert.Len(t, data.Edges, 1)
	assert.Equal(t, "calls", data.Edges[0].Data.Type)
}

func TestDrawNodeCytoscapeWithDepth_MergesNodes(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	// Two different occurrences of the same base node
	node1 := &TrackerNode{key: "pkg.func@pos1"}
	_ = drawNodeCytoscapeWithDepth(node1, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)

	node2 := &TrackerNode{key: "pkg.func@pos2"}
	_ = drawNodeCytoscapeWithDepth(node2, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 1)

	// Should have only 1 node (merged)
	assert.Len(t, data.Nodes, 1)
	// Position should contain both positions
	assert.Contains(t, data.Nodes[0].Data.Position, "pos1")
	assert.Contains(t, data.Nodes[0].Data.Position, "pos2")
}

func TestDrawNodeCytoscapeWithDepth_ArgumentNode(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("myArg")
	arg.SetType("string")

	node := &TrackerNode{
		key:          "arg-node",
		IsArgument:   true,
		ArgType:      ArgTypeVariable,
		ArgIndex:     0,
		ArgContext:   "caller.callee",
		CallArgument: arg,
	}

	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 2)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "argument", data.Nodes[0].Data.Type)
	assert.Equal(t, 2, data.Nodes[0].Data.Depth)
}

func TestDrawNodeCytoscapeWithDepth_FunctionNodeWithMetadata(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: sp.Get("main.go:10"), Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("http"),
			RecvType: sp.Get("Server"), Position: sp.Get("handler.go:5"), Scope: -1,
			SignatureStr: sp.Get("func(w, r)"),
		},
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, meta, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "function", data.Nodes[0].Data.Type)
	assert.Equal(t, "handler", data.Nodes[0].Data.FunctionName)
	assert.Equal(t, "http", data.Nodes[0].Data.Package)
	assert.Equal(t, "Server", data.Nodes[0].Data.ReceiverType)
}

func TestDrawNodeCytoscapeWithDepth_FunctionNodeWithoutMetadata(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
	}

	node := &TrackerNode{}
	node.CallGraphEdge = edge

	// Pass nil metadata => fallback path
	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "function", data.Nodes[0].Data.Type)
	// Should use fallback format
	assert.True(t, strings.HasPrefix(data.Nodes[0].Data.FunctionName, "func_"))
}

func TestDrawNodeCytoscapeWithDepth_CallArgumentNode(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")
	arg.SetType("int")

	node := &TrackerNode{
		key:          "call-arg",
		CallArgument: arg,
		// No IsArgument flag, no CallGraphEdge
	}

	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "call_argument", data.Nodes[0].Data.Type)
}

func TestDrawNodeCytoscapeWithDepth_GenericNode(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	// A TrackerNode with no CallGraphEdge, no CallArgument, no IsArgument
	node := &TrackerNode{key: "generic-node"}

	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "generic", data.Nodes[0].Data.Type)
}

func TestDrawNodeCytoscapeWithDepth_WithRootAssignments(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	node := &TrackerNode{
		key: "node-with-assignments",
		RootAssignmentMap: map[string][]metadata.Assignment{
			"x": {
				{VariableName: sp.Get("x"), Pkg: sp.Get("main")},
				{VariableName: sp.Get("x"), Pkg: sp.Get("main")},
			},
		},
	}

	_ = meta
	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, 2, data.Nodes[0].Data.RootAssignments["x"])
}

func TestDrawNodeCytoscapeWithDepth_WithTypeParams(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	edge := &metadata.CallGraphEdge{
		Callee:       metadata.Call{Meta: meta, Name: sp.Get("fn"), Pkg: sp.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Caller:       metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		TypeParamMap: map[string]string{"T": "int", "U": "string"},
	}
	node := &TrackerNode{}
	node.CallGraphEdge = edge

	result := drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)
	assert.NotEmpty(t, result)
	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "int", data.Nodes[0].Data.Generics["T"])
	assert.Equal(t, "string", data.Nodes[0].Data.Generics["U"])
}

func TestDrawNodeCytoscapeWithDepth_PreventsDuplicateEdges(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	child := &TrackerNode{key: "child"}
	parent := &TrackerNode{key: "parent", Children: []*TrackerNode{child, child}}

	_ = drawNodeCytoscapeWithDepth(parent, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)

	// Should only have 1 edge despite 2 children with same key
	assert.Len(t, data.Edges, 1)
}

func TestDrawNodeCytoscapeWithDepth_ArgumentEdgeType(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	child := &TrackerNode{key: "child"}
	parent := &TrackerNode{
		key:        "parent",
		IsArgument: true,
		Children:   []*TrackerNode{child},
	}

	_ = drawNodeCytoscapeWithDepth(parent, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)

	require.Len(t, data.Edges, 1)
	assert.Equal(t, "argument", data.Edges[0].Data.Type)
}

func TestDrawNodeCytoscapeWithDepth_PackagePrefixTrimming(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("mypkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("mypkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
	}
	// node key includes package prefix
	node := &TrackerNode{key: "mypkg.handler"}
	node.CallGraphEdge = edge

	_ = drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, meta, 0)

	require.Len(t, data.Nodes, 1)
	// Should have package prefix trimmed from label
	assert.False(t, strings.HasPrefix(data.Nodes[0].Data.Label, "mypkg."))
}

func TestDrawNodeCytoscapeWithDepth_MainSuffixTrimming(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	baseKeyToNodeIndex := make(map[string]int)
	edgeSet := make(map[string]bool)
	childrenProcessed := make(map[string]bool)
	ec, nc := 0, 0

	node := &TrackerNode{key: "pkg.main"}

	_ = drawNodeCytoscapeWithDepth(node, data, nodeMap, baseKeyToNodeIndex, edgeSet, childrenProcessed, &ec, &nc, nil, 0)

	require.Len(t, data.Nodes, 1)
	assert.Equal(t, "main", data.Nodes[0].Data.Label)
}

// ---------------------------------------------------------------------------
// processCallGraphEdge
// ---------------------------------------------------------------------------

func TestProcessCallGraphEdge_WithBranch(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: sp.Get("main.go:1"), Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Branch: &metadata.BranchContext{
			BlockKind:  "switch-case",
			CaseValues: []string{"GET", "POST"},
		},
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	require.Len(t, data.Edges, 1)
	assert.Equal(t, "switch-case", data.Edges[0].Data.BranchKind)
	assert.Equal(t, "GET, POST", data.Edges[0].Data.BranchLabel)
}

func TestProcessCallGraphEdge_WithFuncLitCaller(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	parentCall := &metadata.Call{
		Meta:         meta,
		Name:         sp.Get("handleRequest"),
		Pkg:          sp.Get("mypkg"),
		RecvType:     -1,
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("FuncLit:main.go:20"), Pkg: sp.Get("mypkg"),
			RecvType: -1, Position: sp.Get("main.go:20"), Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("target"), Pkg: sp.Get("mypkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		ParentFunction: parentCall,
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// Should have 3 nodes: funclit caller, target callee, parent function
	require.GreaterOrEqual(t, len(data.Nodes), 2)

	// Find the FuncLit node
	var funcLitNode *CytoscapeNodeData
	for i := range data.Nodes {
		if data.Nodes[i].Data.Label == "FuncLit" {
			funcLitNode = &data.Nodes[i].Data
			break
		}
	}
	if funcLitNode != nil {
		assert.Equal(t, "FuncLit", funcLitNode.Label)
		assert.NotEmpty(t, funcLitNode.Parent)
	}
}

func TestProcessCallGraphEdge_WithFuncLitCallee(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	parentCall := &metadata.Call{
		Meta:         meta,
		Name:         sp.Get("setup"),
		Pkg:          sp.Get("mypkg"),
		RecvType:     -1,
		Position:     -1,
		Scope:        -1,
		SignatureStr: -1,
	}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: sp.Get("main.go:5"), Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("FuncLit:setup.go:15"), Pkg: sp.Get("mypkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		ParentFunction: parentCall,
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// Find FuncLit callee node
	found := false
	for _, n := range data.Nodes {
		if n.Data.Label == "FuncLit" {
			found = true
			assert.NotEmpty(t, n.Data.Parent)
			break
		}
	}
	assert.True(t, found, "should have a FuncLit callee node")
}

func TestProcessCallGraphEdge_WithReceiverTypeDeep(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("Handle"), Pkg: sp.Get("http"),
			RecvType: sp.Get("Router"), Position: -1, Scope: -1, SignatureStr: -1,
		},
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// Find callee node with receiver type
	var calleeNode *CytoscapeNodeData
	for i := range data.Nodes {
		if data.Nodes[i].Data.FunctionName == "Handle" {
			calleeNode = &data.Nodes[i].Data
			break
		}
	}
	require.NotNil(t, calleeNode)
	assert.Equal(t, "Router", calleeNode.ReceiverType)
	assert.Equal(t, "Router.Handle", calleeNode.Label)
}

func TestProcessCallGraphEdge_DifferentBranchesCreateSeparateEdges(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	edge1 := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("process"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Branch: &metadata.BranchContext{
			BlockKind:  "switch-case",
			CaseValues: []string{"GET"},
		},
	}
	edge2 := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("process"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Branch: &metadata.BranchContext{
			BlockKind:  "switch-case",
			CaseValues: []string{"POST"},
		},
	}

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge1, data, visited, pairs, idMap, &nc, &ec)
	processCallGraphEdge(meta, edge2, data, visited, pairs, idMap, &nc, &ec)

	// Should have 2 edges (different branch labels)
	assert.Len(t, data.Edges, 2)
}

func TestProcessCallGraphEdge_WithTypeParamMapDeep(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta, Name: sp.Get("main"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: metadata.Call{
			Meta: meta, Name: sp.Get("generic"), Pkg: sp.Get("main"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		TypeParamMap: map[string]string{"T": "int"},
	}
	meta.CallGraph = []metadata.CallGraphEdge{*edge}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// At least the caller node should have generics
	var hasGenerics bool
	for _, n := range data.Nodes {
		if n.Data.Generics != nil && n.Data.Generics["T"] == "int" {
			hasGenerics = true
			break
		}
	}
	assert.True(t, hasGenerics, "should have a node with generics")
}

// ---------------------------------------------------------------------------
// extractParameterInfo
// ---------------------------------------------------------------------------

func TestExtractParameterInfo_WithParamArgMap(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("myVar")
	arg.SetType("string")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("caller"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("callee"), Pkg: sp.Get("main")},
		ParamArgMap: map[string]metadata.CallArgument{
			"param1": *arg,
		},
	}

	paramTypes, passedParams := extractParameterInfo(edge)
	assert.NotEmpty(t, paramTypes)
	assert.NotEmpty(t, passedParams)
	// Should contain "param1"
	found := false
	for _, p := range passedParams {
		if strings.Contains(p, "param1") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestExtractParameterInfo_WithParamArgMap_EmptyParamName(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("val")
	arg.SetType("int")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
		ParamArgMap: map[string]metadata.CallArgument{
			"": *arg,
		},
	}

	_, passedParams := extractParameterInfo(edge)
	// Empty param name should still produce output without ":"
	assert.NotEmpty(t, passedParams)
	assert.Equal(t, "val", passedParams[0])
}

func TestExtractParameterInfo_WithParamArgMap_NoType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")
	// No type set and no resolved type => "unknown"

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
		ParamArgMap: map[string]metadata.CallArgument{
			"p": *arg,
		},
	}

	paramTypes, _ := extractParameterInfo(edge)
	found := false
	for _, pt := range paramTypes {
		if strings.Contains(pt, "unknown") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestExtractParameterInfo_FromArgs_Fallback(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue("42")
	arg.SetType("int")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
		// No ParamArgMap => use Args fallback
		Args: []*metadata.CallArgument{arg},
	}

	paramTypes, passedParams := extractParameterInfo(edge)
	assert.NotEmpty(t, paramTypes)
	assert.NotEmpty(t, passedParams)
	assert.Contains(t, paramTypes[0], "arg0:int")
	assert.Contains(t, passedParams[0], "42")
}

func TestExtractParameterInfo_FromArgs_NoValue(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	// No value, no name, no raw => "nil"

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
		Args:   []*metadata.CallArgument{arg},
	}

	_, passedParams := extractParameterInfo(edge)
	assert.NotEmpty(t, passedParams)
	assert.Contains(t, passedParams[0], "nil")
}

func TestExtractParameterInfo_FromArgs_NoType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("x")
	// No type, no resolved type => "unknown"

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("c"), Pkg: sp.Get("p")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("f"), Pkg: sp.Get("p")},
		Args:   []*metadata.CallArgument{arg},
	}

	paramTypes, passedParams := extractParameterInfo(edge)
	assert.Contains(t, paramTypes[0], "unknown")
	assert.Contains(t, passedParams[0], "x")
}

// ---------------------------------------------------------------------------
// TraverseTrackerTreeBranchOrder — additional cases
// ---------------------------------------------------------------------------

func TestTraverseTrackerTreeBranchOrder_NoRootByIncoming(t *testing.T) {
	// All nodes have incoming edges but none with depth 0, use min depth fallback
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 3}},
			{Data: CytoscapeNodeData{ID: "node_2", Label: "B", Depth: 5}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_1", Target: "node_2", Type: "calls"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_2", Target: "node_1", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	assert.Len(t, result, 2)
}

func TestTraverseTrackerTreeBranchOrder_MinDepthFallbackWithMain(t *testing.T) {
	// All nodes have incoming, min depth node is "main" (should be first)
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_1", Label: "other", Depth: 2}},
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 2}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_1", Target: "node_0", Type: "calls"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_0", Target: "node_1", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	assert.Len(t, result, 2)
	// node_0 (main) should appear first
	assert.Equal(t, "node_0", result[0].Data.ID)
}

func TestTraverseTrackerTreeBranchOrder_OtherRootsNoMain(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_2", Label: "B", Depth: 1}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_1", Target: "node_2", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	assert.Len(t, result, 2)
	assert.Equal(t, "node_1", result[0].Data.ID)
}
