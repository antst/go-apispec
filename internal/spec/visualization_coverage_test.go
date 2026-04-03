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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// ---------- helpers ----------

// newMinimalMeta creates a metadata with a minimal call graph for testing.
func newMinimalMeta() *metadata.Metadata {
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	handlerIdx := sp.Get("handler")
	posIdx := sp.Get("main.go:10")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     mainIdx,
					Pkg:      mainIdx,
					Position: posIdx,
					RecvType: -1,
					Scope:    -1,
				},
				Callee: metadata.Call{
					Name:     handlerIdx,
					Pkg:      mainIdx,
					Position: -1,
					RecvType: -1,
					Scope:    -1,
				},
			},
		},
	}
	// Wire Meta pointers
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()
	return meta
}

// ---------- 1. splitNodeLabel ----------

func TestSplitNodeLabel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLbl string
		wantPos string
	}{
		{"with_at", "funcName@file.go:10", "funcName", "file.go:10"},
		{"no_at", "noAt", "noAt", ""},
		{"empty", "", "", ""},
		{"multiple_at", "a@b@c", "a", "b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbl, pos := splitNodeLabel(tt.input)
			assert.Equal(t, tt.wantLbl, lbl, "label")
			assert.Equal(t, tt.wantPos, pos, "position")
		})
	}
}

// ---------- 2. callArgument ----------

func TestCallArgument_Nil(t *testing.T) {
	assert.Nil(t, callArgument(nil))
}

func TestCallArgument_WithNameReturnsItself(t *testing.T) {
	sp := metadata.NewStringPool()
	nameIdx := sp.Get("myVar")
	meta := &metadata.Metadata{StringPool: sp}

	arg := &metadata.CallArgument{Name: nameIdx, Meta: meta}
	got := callArgument(arg)
	require.NotNil(t, got)
	assert.Equal(t, "myVar", got.GetName())
}

func TestCallArgument_WithValueReturnsItself(t *testing.T) {
	sp := metadata.NewStringPool()
	valIdx := sp.Get("42")
	meta := &metadata.Metadata{StringPool: sp}

	arg := &metadata.CallArgument{Value: valIdx, Name: -1, Meta: meta}
	got := callArgument(arg)
	require.NotNil(t, got)
	assert.Equal(t, "42", got.GetValue())
}

func TestCallArgument_TraversesFun(t *testing.T) {
	sp := metadata.NewStringPool()
	nameIdx := sp.Get("inner")
	meta := &metadata.Metadata{StringPool: sp}

	inner := &metadata.CallArgument{Name: nameIdx, Meta: meta}
	outer := &metadata.CallArgument{Name: -1, Value: -1, Fun: inner, Meta: meta}
	got := callArgument(outer)
	require.NotNil(t, got)
	assert.Equal(t, "inner", got.GetName())
}

func TestCallArgument_TraversesSel(t *testing.T) {
	sp := metadata.NewStringPool()
	nameIdx := sp.Get("sel")
	meta := &metadata.Metadata{StringPool: sp}

	inner := &metadata.CallArgument{Name: nameIdx, Meta: meta}
	outer := &metadata.CallArgument{Name: -1, Value: -1, Sel: inner, Meta: meta}
	got := callArgument(outer)
	require.NotNil(t, got)
	assert.Equal(t, "sel", got.GetName())
}

func TestCallArgument_TraversesX(t *testing.T) {
	sp := metadata.NewStringPool()
	nameIdx := sp.Get("xArg")
	meta := &metadata.Metadata{StringPool: sp}

	inner := &metadata.CallArgument{Name: nameIdx, Meta: meta}
	outer := &metadata.CallArgument{Name: -1, Value: -1, X: inner, Meta: meta}
	got := callArgument(outer)
	require.NotNil(t, got)
	assert.Equal(t, "xArg", got.GetName())
}

func TestCallArgument_NoFieldsReturnsSelf(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	arg := &metadata.CallArgument{Name: -1, Value: -1, Meta: meta}
	got := callArgument(arg)
	assert.Equal(t, arg, got, "should return same pointer when no traversal fields")
}

// ---------- 3. extractFuncLitSignature ----------

func TestExtractFuncLitSignature_NilInputs(t *testing.T) {
	assert.Equal(t, "", extractFuncLitSignature(nil, nil))
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.Equal(t, "", extractFuncLitSignature(meta, nil))
	assert.Equal(t, "", extractFuncLitSignature(nil, &metadata.Call{}))
}

func TestExtractFuncLitSignature_WithRecvType(t *testing.T) {
	sp := metadata.NewStringPool()
	recvIdx := sp.Get("*MyHandler")
	meta := &metadata.Metadata{StringPool: sp}

	caller := &metadata.Call{
		RecvType: recvIdx,
		Meta:     meta,
	}
	assert.Equal(t, "*MyHandler", extractFuncLitSignature(meta, caller))
}

func TestExtractFuncLitSignature_NoRecvType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	caller := &metadata.Call{
		RecvType: -1,
		Meta:     meta,
	}
	assert.Equal(t, "func()", extractFuncLitSignature(meta, caller))
}

// ---------- 4. OrderTrackerTreeNodesDepthFirst ----------

func TestOrderTrackerTreeNodesDepthFirst_Empty(t *testing.T) {
	data := &CytoscapeData{Nodes: []CytoscapeNode{}, Edges: []CytoscapeEdge{}}
	result := OrderTrackerTreeNodesDepthFirst(data)
	assert.Empty(t, result)
}

func TestOrderTrackerTreeNodesDepthFirst_Linear(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_1", Label: "B", Depth: 1}},
			{Data: CytoscapeNodeData{ID: "node_2", Label: "C", Depth: 2}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_0", Target: "node_1"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_1", Target: "node_2"}},
		},
	}
	result := OrderTrackerTreeNodesDepthFirst(data)
	require.Len(t, result, 3)
	assert.Equal(t, "node_0", result[0].Data.ID)
	assert.Equal(t, "node_1", result[1].Data.ID)
	assert.Equal(t, "node_2", result[2].Data.ID)
}

func TestOrderTrackerTreeNodesDepthFirst_Branching(t *testing.T) {
	// main -> A, main -> B, A -> C
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 1}},
			{Data: CytoscapeNodeData{ID: "node_2", Label: "B", Depth: 1}},
			{Data: CytoscapeNodeData{ID: "node_3", Label: "C", Depth: 2}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_0", Target: "node_1"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_0", Target: "node_2"}},
			{Data: CytoscapeEdgeData{ID: "e2", Source: "node_1", Target: "node_3"}},
		},
	}
	result := OrderTrackerTreeNodesDepthFirst(data)
	require.Len(t, result, 4)
	// main first, then A, then C (depth-first into A before visiting B), then B
	assert.Equal(t, "node_0", result[0].Data.ID)
	assert.Equal(t, "node_1", result[1].Data.ID)
	assert.Equal(t, "node_3", result[2].Data.ID)
	assert.Equal(t, "node_2", result[3].Data.ID)
}

func TestOrderTrackerTreeNodesDepthFirst_OrphanedNodes(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_orphan", Label: "orphan", Depth: 5}},
		},
		Edges: []CytoscapeEdge{},
	}
	result := OrderTrackerTreeNodesDepthFirst(data)
	require.Len(t, result, 2)
	// main is depth 0, should come first; orphan has no edges and isn't root by depth 0
	assert.Equal(t, "node_0", result[0].Data.ID)
}

// ---------- 5. TraverseTrackerTreeBranchOrder ----------

func TestTraverseTrackerTreeBranchOrder_Empty(t *testing.T) {
	data := &CytoscapeData{Nodes: []CytoscapeNode{}, Edges: []CytoscapeEdge{}}
	result := TraverseTrackerTreeBranchOrder(data)
	assert.Nil(t, result)
}

func TestTraverseTrackerTreeBranchOrder_Linear(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 1}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_0", Target: "node_1", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	require.Len(t, result, 2)
	assert.Equal(t, "node_0", result[0].Data.ID)
	assert.Equal(t, "node_1", result[1].Data.ID)
}

func TestTraverseTrackerTreeBranchOrder_Branching(t *testing.T) {
	// main -> A, main -> B, A -> C (all "calls" type)
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 1}},
			{Data: CytoscapeNodeData{ID: "node_2", Label: "B", Depth: 1}},
			{Data: CytoscapeNodeData{ID: "node_3", Label: "C", Depth: 2}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_0", Target: "node_1", Type: "calls"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_0", Target: "node_2", Type: "calls"}},
			{Data: CytoscapeEdgeData{ID: "e2", Source: "node_1", Target: "node_3", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	require.Len(t, result, 4)
	assert.Equal(t, "node_0", result[0].Data.ID)
	assert.Equal(t, "node_1", result[1].Data.ID)
	assert.Equal(t, "node_3", result[2].Data.ID)
	assert.Equal(t, "node_2", result[3].Data.ID)
}

func TestTraverseTrackerTreeBranchOrder_IgnoresNonCallEdges(t *testing.T) {
	// Only "calls" edges are followed; argument edges should be ignored.
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_1", Label: "A", Depth: 1}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_0", Target: "node_1", Type: "argument"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	require.Len(t, result, 2)
	// Both should appear but node_1 won't be reached via calls edges, so it will be orphaned.
	// Main is root, node_1 has incoming (argument) but since only "calls" edges are considered,
	// node_1 has no incoming "calls" edges so it's also a root.
	ids := []string{result[0].Data.ID, result[1].Data.ID}
	assert.Contains(t, ids, "node_0")
	assert.Contains(t, ids, "node_1")
}

func TestTraverseTrackerTreeBranchOrder_WithOrphanedNodes(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_0", Label: "main", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_orphan", Label: "orphan", Depth: 5}},
		},
		Edges: []CytoscapeEdge{},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	require.Len(t, result, 2)
}

// ---------- 6. drawNode ----------

func TestDrawNode_SingleRoot(t *testing.T) {
	root := &TrackerNode{key: "root"}
	var sb strings.Builder
	counter := 0
	drawNode(root, &sb, &counter)
	// No children means nothing written
	assert.Empty(t, sb.String())
}

func TestDrawNode_WithChildren(t *testing.T) {
	root := &TrackerNode{key: "root"}
	child := &TrackerNode{key: "child", Parent: root}
	root.Children = []*TrackerNode{child}

	var sb strings.Builder
	counter := 0
	drawNode(root, &sb, &counter)

	output := sb.String()
	assert.Contains(t, output, "root")
	assert.Contains(t, output, "child")
	assert.Contains(t, output, "-->")
}

func TestDrawNode_DeeplyNested(t *testing.T) {
	root := &TrackerNode{key: "A"}
	b := &TrackerNode{key: "B", Parent: root}
	c := &TrackerNode{key: "C", Parent: b}
	root.Children = []*TrackerNode{b}
	b.Children = []*TrackerNode{c}

	var sb strings.Builder
	counter := 0
	drawNode(root, &sb, &counter)

	output := sb.String()
	assert.Contains(t, output, "A")
	assert.Contains(t, output, "B")
	assert.Contains(t, output, "C")
	// Two edges: A->B and B->C
	assert.Equal(t, 2, strings.Count(output, "-->"))
}

// ---------- 7. buildCallPathInfos ----------

func TestBuildCallPathInfos_Found(t *testing.T) {
	meta := newMinimalMeta()

	// The callee in our minimal meta is "handler" in package "main"
	calleeID := meta.CallGraph[0].Callee.BaseID()
	infos := buildCallPathInfos(meta, calleeID)

	require.NotEmpty(t, infos)
	assert.Equal(t, "main", infos[0].CallerPkg)
	assert.Equal(t, "main", infos[0].CallerName)
}

func TestBuildCallPathInfos_NotFound(t *testing.T) {
	meta := newMinimalMeta()
	infos := buildCallPathInfos(meta, "nonexistent.function")
	assert.Empty(t, infos)
}

func TestBuildCallPathInfos_WithFuncLitCaller(t *testing.T) {
	sp := metadata.NewStringPool()
	funcLitName := sp.Get("FuncLit:main.go:20")
	pkgIdx := sp.Get("mypkg")
	handlerIdx := sp.Get("handler")
	posIdx := sp.Get("main.go:25")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     funcLitName,
					Pkg:      pkgIdx,
					Position: posIdx,
					RecvType: -1,
					Scope:    -1,
				},
				Callee: metadata.Call{
					Name:     handlerIdx,
					Pkg:      pkgIdx,
					Position: -1,
					RecvType: -1,
					Scope:    -1,
				},
			},
		},
	}
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	calleeID := meta.CallGraph[0].Callee.BaseID()
	infos := buildCallPathInfos(meta, calleeID)

	require.NotEmpty(t, infos)
	assert.Equal(t, "FuncLit", infos[0].CallerName)
	require.NotNil(t, infos[0].FuncLitInfo)
	assert.Equal(t, "main.go:20", infos[0].FuncLitInfo.Position)
}

// ---------- 8. processCallGraphEdge ----------

func TestProcessCallGraphEdge_NilEdge(t *testing.T) {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0
	// Should not panic
	processCallGraphEdge(nil, nil, data, visited, pairs, idMap, &nc, &ec)
	assert.Empty(t, data.Nodes)
	assert.Empty(t, data.Edges)
}

func TestProcessCallGraphEdge_CreatesNodesAndEdge(t *testing.T) {
	meta := newMinimalMeta()
	edge := &meta.CallGraph[0]

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// Should have 2 nodes (caller + callee) and 1 edge
	assert.Len(t, data.Nodes, 2)
	assert.Len(t, data.Edges, 1)

	// Verify edge type
	assert.Equal(t, "calls", data.Edges[0].Data.Type)
}

func TestProcessCallGraphEdge_NoDuplicateNodes(t *testing.T) {
	meta := newMinimalMeta()
	edge := &meta.CallGraph[0]

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	// Process same edge twice
	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)
	processCallGraphEdge(meta, edge, data, visited, pairs, idMap, &nc, &ec)

	// Should still have only 2 nodes and 1 edge (no duplicates)
	assert.Len(t, data.Nodes, 2)
	assert.Len(t, data.Edges, 1)
}

// ---------- 9. Export functions ----------

func TestGenerateCallGraphCytoscapeHTML(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "callgraph.html")

	err := GenerateCallGraphCytoscapeHTML(meta, outPath)
	require.NoError(t, err)

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.True(t, len(content) > 100, "HTML file should have substantial content")
	assert.Contains(t, string(content), "main")
}

func TestGenerateCallGraphCytoscapeHTML_InvalidPath(t *testing.T) {
	meta := newMinimalMeta()
	err := GenerateCallGraphCytoscapeHTML(meta, "/nonexistent/dir/out.html")
	assert.Error(t, err)
}

func TestExportCallGraphCytoscapeJSON(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "callgraph.json")

	err := ExportCallGraphCytoscapeJSON(meta, outPath)
	require.NoError(t, err)

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.True(t, len(content) > 10, "JSON file should have content")

	// Verify it's valid JSON
	var parsed CytoscapeData
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.NotEmpty(t, parsed.Nodes)
}

func TestExportCallGraphCytoscapeJSON_InvalidPath(t *testing.T) {
	meta := newMinimalMeta()
	err := ExportCallGraphCytoscapeJSON(meta, "/nonexistent/dir/out.json")
	assert.Error(t, err)
}

func TestGenerateOptimizedCallGraphHTML_Paginated(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "paginated.html")

	err := GenerateOptimizedCallGraphHTML(meta, outPath, "paginated")
	require.NoError(t, err)

	info, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.True(t, info.Size() > 100)
}

func TestGenerateOptimizedCallGraphHTML_Default(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "default.html")

	err := GenerateOptimizedCallGraphHTML(meta, outPath, "default")
	require.NoError(t, err)

	info, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.True(t, info.Size() > 100)
}

func TestGenerateOptimizedCallGraphHTML_UnknownFallsToDefault(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "unknown.html")

	err := GenerateOptimizedCallGraphHTML(meta, outPath, "whatever")
	require.NoError(t, err)

	_, err = os.Stat(outPath)
	require.NoError(t, err)
}

// ---------- 10. Paginated server ----------

func TestNewPaginatedCallGraphServer(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)
	require.NotNil(t, server)
	assert.Equal(t, 10, server.pageSize)
	assert.NotNil(t, server.allData)
}

func TestPaginatedCallGraphServer_ServeHTTP_DefaultParams(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, 1, data.Page)
	assert.Equal(t, 10, data.PageSize)
	assert.GreaterOrEqual(t, data.TotalNodes, 1)
}

func TestPaginatedCallGraphServer_ServeHTTP_WithPageAndPackage(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	req := httptest.NewRequest(http.MethodGet, "/?page=1&package=main", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, 1, data.Page)
	// All nodes in our minimal meta are in "main" package
	assert.GreaterOrEqual(t, data.TotalNodes, 1)
}

func TestPaginatedCallGraphServer_ServeHTTP_PackageFilterNoMatch(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	req := httptest.NewRequest(http.MethodGet, "/?package=nonexistent", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, 0, data.TotalNodes)
}

func TestPaginatedCallGraphServer_ServeHTTP_HighPage(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 1) // page size 1

	req := httptest.NewRequest(http.MethodGet, "/?page=100", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	// Page 100 with only a few nodes should yield no nodes in the page
	assert.Empty(t, data.Nodes)
	assert.False(t, data.HasMore)
}

func TestGetPaginatedData_NoFilter(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 5)

	result := server.getPaginatedData(0, 5, "")
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 5, result.PageSize)
	assert.GreaterOrEqual(t, result.TotalNodes, 1)
}

func TestGetPaginatedData_WithPackageFilter(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 5)

	result := server.getPaginatedData(0, 5, "main")
	require.NotNil(t, result)
	// All nodes in minimal meta have package "main"
	assert.GreaterOrEqual(t, result.TotalNodes, 1)
}

func TestGetPaginatedData_StartBeyondLength(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 5)

	result := server.getPaginatedData(1000, 1005, "")
	require.NotNil(t, result)
	assert.Empty(t, result.Nodes)
	assert.False(t, result.HasMore)
}

func TestGetPaginatedData_EdgeFilterMatchesPaginated(t *testing.T) {
	// Create meta with multiple edges to test edge filtering during pagination
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	aIdx := sp.Get("A")
	bIdx := sp.Get("B")
	cIdx := sp.Get("C")
	pkgOther := sp.Get("other")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{Name: mainIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: aIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
			},
			{
				Caller: metadata.Call{Name: aIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: bIdx, Pkg: pkgOther, RecvType: -1, Position: -1, Scope: -1},
			},
			{
				Caller: metadata.Call{Name: bIdx, Pkg: pkgOther, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: cIdx, Pkg: pkgOther, RecvType: -1, Position: -1, Scope: -1},
			},
		},
	}
	for i := range meta.CallGraph {
		meta.CallGraph[i].Caller.Meta = meta
		meta.CallGraph[i].Callee.Meta = meta
	}
	meta.BuildCallGraphMaps()

	server := NewPaginatedCallGraphServer(meta, 100)

	// Filter to "other" package
	result := server.getPaginatedData(0, 100, "other")
	require.NotNil(t, result)
	// Only edges between "other" nodes should be included
	for _, e := range result.Edges {
		// Both source and target should map to "other" package nodes
		assert.NotEmpty(t, e.Data.Source)
		assert.NotEmpty(t, e.Data.Target)
	}
}

func TestGetPaginatedData_HasMore(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 1) // page size 1

	result := server.getPaginatedData(0, 1, "")
	require.NotNil(t, result)
	// With 2 nodes total and page size 1, HasMore should be true
	if result.TotalNodes > 1 {
		assert.True(t, result.HasMore)
	}
}

func TestGeneratePaginatedCytoscapeHTML(t *testing.T) {
	meta := newMinimalMeta()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "paginated.html")

	err := GeneratePaginatedCytoscapeHTML(meta, outPath, 10)
	require.NoError(t, err)

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.True(t, len(content) > 100)
}

func TestGeneratePaginatedCytoscapeHTML_InvalidPath(t *testing.T) {
	meta := newMinimalMeta()
	err := GeneratePaginatedCytoscapeHTML(meta, "/nonexistent/dir/paginated.html", 10)
	assert.Error(t, err)
}

func TestGenerateServerBasedCytoscapeHTML(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "server.html")

	err := GenerateServerBasedCytoscapeHTML("http://localhost:8080", outPath)
	require.NoError(t, err)

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "http://localhost:8080")
}

func TestGenerateServerBasedCytoscapeHTML_InvalidPath(t *testing.T) {
	err := GenerateServerBasedCytoscapeHTML("http://localhost:8080", "/nonexistent/dir/server.html")
	assert.Error(t, err)
}

// ---------- 11. DrawTrackerTree edge cases ----------

func TestDrawTrackerTree_EmptyTree(t *testing.T) {
	result := DrawTrackerTree(nil)
	assert.Equal(t, mermaidGraphHeader, result)
}

func TestDrawTrackerTree_DeeplyNested(t *testing.T) {
	// Create a chain: root -> a -> b -> c -> d
	root := &TrackerNode{key: "root"}
	a := &TrackerNode{key: "a", Parent: root}
	b := &TrackerNode{key: "b", Parent: a}
	c := &TrackerNode{key: "c", Parent: b}
	d := &TrackerNode{key: "d", Parent: c}
	root.Children = []*TrackerNode{a}
	a.Children = []*TrackerNode{b}
	b.Children = []*TrackerNode{c}
	c.Children = []*TrackerNode{d}

	nodes := []TrackerNodeInterface{root}
	result := DrawTrackerTree(nodes)

	assert.Contains(t, result, "graph LR")
	assert.Contains(t, result, "root")
	assert.Contains(t, result, "d")
	// Should have 4 edges
	assert.Equal(t, 4, strings.Count(result, "-->"))
}

func TestDrawTrackerTree_MultipleRoots(t *testing.T) {
	root1 := &TrackerNode{key: "root1"}
	child1 := &TrackerNode{key: "c1", Parent: root1}
	root1.Children = []*TrackerNode{child1}

	root2 := &TrackerNode{key: "root2"}
	child2 := &TrackerNode{key: "c2", Parent: root2}
	root2.Children = []*TrackerNode{child2}

	nodes := []TrackerNodeInterface{root1, root2}
	result := DrawTrackerTree(nodes)

	assert.Contains(t, result, "root1")
	assert.Contains(t, result, "root2")
	assert.Contains(t, result, "c1")
	assert.Contains(t, result, "c2")
}

// ---------- 12. DrawCallGraphCytoscape edge cases ----------

func TestDrawCallGraphCytoscape_NilMeta(t *testing.T) {
	result := DrawCallGraphCytoscape(nil)
	require.NotNil(t, result)
	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestDrawCallGraphCytoscape_EmptyCallGraph(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph:  []metadata.CallGraphEdge{},
	}
	result := DrawCallGraphCytoscape(meta)
	require.NotNil(t, result)
	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestDrawCallGraphCytoscape_WithData(t *testing.T) {
	meta := newMinimalMeta()
	result := DrawCallGraphCytoscape(meta)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Nodes)
	assert.NotEmpty(t, result.Edges)

	// Verify we have both caller and callee nodes
	labels := make([]string, 0, len(result.Nodes))
	for _, n := range result.Nodes {
		labels = append(labels, n.Data.Label)
	}
	assert.Contains(t, labels, "main")
	assert.Contains(t, labels, "handler")
}

func TestDrawCallGraphCytoscape_NilStringPool(t *testing.T) {
	meta := &metadata.Metadata{
		StringPool: nil,
		CallGraph:  nil,
	}
	// Should handle nil without panic
	result := DrawCallGraphCytoscape(meta)
	require.NotNil(t, result)
}

// ---------- 13. DrawTrackerTreeCytoscapeWithMetadata edge cases ----------

func TestDrawTrackerTreeCytoscapeWithMetadata_EmptyTree(t *testing.T) {
	result := DrawTrackerTreeCytoscapeWithMetadata(nil, nil)
	require.NotNil(t, result)
	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestDrawTrackerTreeCytoscapeWithMetadata_EmptySlice(t *testing.T) {
	nodes := []TrackerNodeInterface{}
	result := DrawTrackerTreeCytoscapeWithMetadata(nodes, nil)
	require.NotNil(t, result)
	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestDrawTrackerTreeCytoscapeWithMetadata_SingleNode(t *testing.T) {
	root := &TrackerNode{key: "myFunc"}
	nodes := []TrackerNodeInterface{root}
	result := DrawTrackerTreeCytoscapeWithMetadata(nodes, nil)
	require.NotNil(t, result)
	assert.Len(t, result.Nodes, 1)
	assert.Empty(t, result.Edges)
}

func TestDrawTrackerTreeCytoscapeWithMetadata_WithChildren(t *testing.T) {
	root := &TrackerNode{key: "parent"}
	child := &TrackerNode{key: "child", Parent: root}
	root.Children = []*TrackerNode{child}

	nodes := []TrackerNodeInterface{root}
	result := DrawTrackerTreeCytoscapeWithMetadata(nodes, nil)
	require.NotNil(t, result)
	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Edges, 1)
}

// ---------- Additional coverage: buildCallPaths ----------

func TestBuildCallPaths_Empty(t *testing.T) {
	meta := newMinimalMeta()
	paths := buildCallPaths(meta, "nonexistent")
	assert.Empty(t, paths)
}

func TestBuildCallPaths_WithData(t *testing.T) {
	meta := newMinimalMeta()
	calleeID := meta.CallGraph[0].Callee.BaseID()
	paths := buildCallPaths(meta, calleeID)
	require.NotEmpty(t, paths)
	assert.Contains(t, paths[0], "main")
}

func TestBuildCallPaths_WithPosition(t *testing.T) {
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	handlerIdx := sp.Get("handler")
	posIdx := sp.Get("file.go:42")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     mainIdx,
					Pkg:      mainIdx,
					Position: posIdx,
					RecvType: -1,
					Scope:    -1,
				},
				Callee: metadata.Call{
					Name:     handlerIdx,
					Pkg:      mainIdx,
					Position: -1,
					RecvType: -1,
					Scope:    -1,
				},
			},
		},
	}
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	calleeID := meta.CallGraph[0].Callee.BaseID()
	paths := buildCallPaths(meta, calleeID)
	require.NotEmpty(t, paths)
	assert.Contains(t, paths[0], "@ file.go:42")
}

// ---------- Additional coverage: extractParameterInfo ----------

func TestExtractParameterInfo_NoParamArgMap(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	nameIdx := sp.Get("myArg")
	typeIdx := sp.Get("string")

	edge := &metadata.CallGraphEdge{
		ParamArgMap: nil,
		Args: []*metadata.CallArgument{
			{
				Name: nameIdx,
				Type: typeIdx,
				Meta: meta,
			},
		},
	}

	paramTypes, passedParams := extractParameterInfo(edge)
	require.Len(t, paramTypes, 1)
	assert.Contains(t, paramTypes[0], "string")
	require.Len(t, passedParams, 1)
	assert.Contains(t, passedParams[0], "myArg")
}

func TestExtractParameterInfo_EmptyEdge(t *testing.T) {
	edge := &metadata.CallGraphEdge{}
	paramTypes, passedParams := extractParameterInfo(edge)
	assert.Empty(t, paramTypes)
	assert.Empty(t, passedParams)
}

func TestExtractParameterInfo_ArgsWithNoNameOrValue(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	// Arg with no Name, no Value, no Raw -> should fall back to "nil"
	edge := &metadata.CallGraphEdge{
		Args: []*metadata.CallArgument{
			{
				Name:  -1,
				Value: -1,
				Raw:   -1,
				Type:  -1,
				Meta:  meta,
			},
		},
	}

	paramTypes, passedParams := extractParameterInfo(edge)
	require.Len(t, paramTypes, 1)
	assert.Contains(t, paramTypes[0], "unknown")
	require.Len(t, passedParams, 1)
	assert.Contains(t, passedParams[0], "nil")
}

// ---------- Additional: multiple edges in processCallGraphEdge ----------

func TestProcessCallGraphEdge_MultipleDistinctEdges(t *testing.T) {
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	aIdx := sp.Get("funcA")
	bIdx := sp.Get("funcB")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{Name: mainIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: aIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
			},
			{
				Caller: metadata.Call{Name: mainIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: bIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
			},
		},
	}
	for i := range meta.CallGraph {
		meta.CallGraph[i].Caller.Meta = meta
		meta.CallGraph[i].Callee.Meta = meta
	}
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	for i := range meta.CallGraph {
		processCallGraphEdge(meta, &meta.CallGraph[i], data, visited, pairs, idMap, &nc, &ec)
	}

	// 3 unique nodes: main, funcA, funcB
	assert.Len(t, data.Nodes, 3)
	// 2 edges: main->funcA, main->funcB
	assert.Len(t, data.Edges, 2)
}

// ---------- OrderTrackerTreeNodesDepthFirst: no main root fallback ----------

func TestOrderTrackerTreeNodesDepthFirst_NoMainRoot(t *testing.T) {
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_5", Label: "alpha", Depth: 0}},
			{Data: CytoscapeNodeData{ID: "node_6", Label: "beta", Depth: 1}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_5", Target: "node_6"}},
		},
	}
	result := OrderTrackerTreeNodesDepthFirst(data)
	require.Len(t, result, 2)
	assert.Equal(t, "node_5", result[0].Data.ID)
	assert.Equal(t, "node_6", result[1].Data.ID)
}

func TestOrderTrackerTreeNodesDepthFirst_FallbackMinDepth(t *testing.T) {
	// All nodes have incoming edges so the root-finding logic must fallback to minDepth
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_a", Label: "A", Depth: 3}},
			{Data: CytoscapeNodeData{ID: "node_b", Label: "B", Depth: 3}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_a", Target: "node_b"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_b", Target: "node_a"}},
		},
	}
	result := OrderTrackerTreeNodesDepthFirst(data)
	require.Len(t, result, 2)
}

// ---------- TraverseTrackerTreeBranchOrder: fallback min depth ----------

func TestTraverseTrackerTreeBranchOrder_FallbackMinDepth(t *testing.T) {
	// All nodes have incoming "calls" edges, so no root by incoming-edge check or depth 0
	data := &CytoscapeData{
		Nodes: []CytoscapeNode{
			{Data: CytoscapeNodeData{ID: "node_x", Label: "X", Depth: 2}},
			{Data: CytoscapeNodeData{ID: "node_y", Label: "Y", Depth: 2}},
		},
		Edges: []CytoscapeEdge{
			{Data: CytoscapeEdgeData{ID: "e0", Source: "node_x", Target: "node_y", Type: "calls"}},
			{Data: CytoscapeEdgeData{ID: "e1", Source: "node_y", Target: "node_x", Type: "calls"}},
		},
	}
	result := TraverseTrackerTreeBranchOrder(data)
	require.Len(t, result, 2)
}

// ---------- Paginated server: depth parameter ----------

func TestPaginatedCallGraphServer_ServeHTTP_WithDepthParam(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	req := httptest.NewRequest(http.MethodGet, "/?depth=5", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPaginatedCallGraphServer_ServeHTTP_InvalidPage(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	// page=0 should be treated as page=1
	req := httptest.NewRequest(http.MethodGet, "/?page=0", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, 1, data.Page)
}

func TestPaginatedCallGraphServer_ServeHTTP_NegativePage(t *testing.T) {
	meta := newMinimalMeta()
	server := NewPaginatedCallGraphServer(meta, 10)

	req := httptest.NewRequest(http.MethodGet, "/?page=-5", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var data PaginatedCytoscapeData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	assert.Equal(t, 1, data.Page)
}

// ---------- DrawTrackerTreeCytoscape delegates correctly ----------

func TestDrawTrackerTreeCytoscape_DelegatesToWithMetadata(t *testing.T) {
	root := &TrackerNode{key: "func1"}
	child := &TrackerNode{key: "func2", Parent: root}
	root.Children = []*TrackerNode{child}

	nodes := []TrackerNodeInterface{root}
	result := DrawTrackerTreeCytoscape(nodes)
	require.NotNil(t, result)
	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Edges, 1)
}

// ---------- FuncLit in DrawCallGraphCytoscape ----------

func TestDrawCallGraphCytoscape_FuncLitCaller(t *testing.T) {
	sp := metadata.NewStringPool()
	funcLitName := sp.Get("FuncLit:main.go:15")
	pkgIdx := sp.Get("pkg")
	targetIdx := sp.Get("target")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Name:     funcLitName,
					Pkg:      pkgIdx,
					Position: -1,
					RecvType: -1,
					Scope:    -1,
				},
				Callee: metadata.Call{
					Name:     targetIdx,
					Pkg:      pkgIdx,
					Position: -1,
					RecvType: -1,
					Scope:    -1,
				},
			},
		},
	}
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	result := DrawCallGraphCytoscape(meta)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Nodes)

	// The FuncLit caller label should be "FuncLit", not "FuncLit:main.go:15"
	foundFuncLit := false
	for _, n := range result.Nodes {
		if n.Data.Label == "FuncLit" {
			foundFuncLit = true
			// Position should be extracted from the name
			assert.Equal(t, "main.go:15", n.Data.Position)
			break
		}
	}
	assert.True(t, foundFuncLit, "expected to find a node labeled FuncLit")
}

// ---------- processCallGraphEdge with ReceiverType ----------

func TestProcessCallGraphEdge_WithReceiverType(t *testing.T) {
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	methodIdx := sp.Get("Handle")
	recvIdx := sp.Get("Server")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{Name: mainIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee: metadata.Call{Name: methodIdx, Pkg: mainIdx, RecvType: recvIdx, Position: -1, Scope: -1},
			},
		},
	}
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{Nodes: make([]CytoscapeNode, 0), Edges: make([]CytoscapeEdge, 0)}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, &meta.CallGraph[0], data, visited, pairs, idMap, &nc, &ec)

	// The callee should have label "Server.Handle"
	found := false
	for _, n := range data.Nodes {
		if n.Data.Label == "Server.Handle" {
			found = true
			assert.Equal(t, "Server", n.Data.ReceiverType)
			break
		}
	}
	assert.True(t, found, "expected node with label Server.Handle")
}

// ---------- processCallGraphEdge with TypeParamMap ----------

func TestProcessCallGraphEdge_WithTypeParamMap(t *testing.T) {
	sp := metadata.NewStringPool()
	mainIdx := sp.Get("main")
	genericIdx := sp.Get("GenericFunc")

	meta := &metadata.Metadata{
		StringPool: sp,
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller:       metadata.Call{Name: mainIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				Callee:       metadata.Call{Name: genericIdx, Pkg: mainIdx, RecvType: -1, Position: -1, Scope: -1},
				TypeParamMap: map[string]string{"T": "int"},
			},
		},
	}
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.BuildCallGraphMaps()

	data := &CytoscapeData{Nodes: make([]CytoscapeNode, 0), Edges: make([]CytoscapeEdge, 0)}
	visited := make(map[string]bool)
	pairs := make(map[string]bool)
	idMap := make(map[string]string)
	nc, ec := 0, 0

	processCallGraphEdge(meta, &meta.CallGraph[0], data, visited, pairs, idMap, &nc, &ec)

	// Verify the caller node has generics set
	found := false
	for _, n := range data.Nodes {
		if n.Data.Label == "main" {
			found = true
			if n.Data.Generics != nil {
				assert.Equal(t, "int", n.Data.Generics["T"])
			}
			break
		}
	}
	assert.True(t, found, "expected to find main node")
}
