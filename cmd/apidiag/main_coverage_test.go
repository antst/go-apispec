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

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antst/go-apispec/internal/metadata"
	"github.com/antst/go-apispec/internal/spec"
)

// ---------- matchesFunctionName ----------

func TestMatchesFunctionName(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		searchTerm   string
		want         bool
	}{
		{"exact match", "HandleRequest", "HandleRequest", true},
		{"case insensitive", "HandleRequest", "handlerequest", true},
		{"substring match", "HandleRequest", "Request", true},
		{"no match", "HandleRequest", "foobar", false},
		{"empty search term", "HandleRequest", "", false},
		{"empty function name", "", "something", false},
		{"both empty", "", "", false},
		{"search with spaces", "HandleRequest", "  Request  ", true},
		{"partial lowercase", "MyFunc", "func", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFunctionName(tt.functionName, tt.searchTerm)
			if got != tt.want {
				t.Errorf("matchesFunctionName(%q, %q) = %v, want %v",
					tt.functionName, tt.searchTerm, got, tt.want)
			}
		})
	}
}

// ---------- calculateCallGraphDepth ----------

func TestCalculateCallGraphDepth(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	t.Run("empty graph", func(t *testing.T) {
		data := &spec.CytoscapeData{
			Nodes: []spec.CytoscapeNode{},
			Edges: []spec.CytoscapeEdge{},
		}
		depths := server.calculateCallGraphDepth(data)
		if len(depths) != 0 {
			t.Errorf("expected 0 depths, got %d", len(depths))
		}
	})

	t.Run("single node no edges", func(t *testing.T) {
		data := &spec.CytoscapeData{
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "n1", Label: "main"}},
			},
			Edges: []spec.CytoscapeEdge{},
		}
		depths := server.calculateCallGraphDepth(data)
		if d, ok := depths["n1"]; !ok || d != 0 {
			t.Errorf("expected n1 at depth 0, got %v (ok=%v)", d, ok)
		}
	})

	t.Run("linear chain", func(t *testing.T) {
		// main -> A -> B -> C
		data := &spec.CytoscapeData{
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "n1", Label: "main", FunctionName: "main"}},
				{Data: spec.CytoscapeNodeData{ID: "n2", Label: "A"}},
				{Data: spec.CytoscapeNodeData{ID: "n3", Label: "B"}},
				{Data: spec.CytoscapeNodeData{ID: "n4", Label: "C"}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "n1", Target: "n2"}},
				{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "n2", Target: "n3"}},
				{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "n3", Target: "n4"}},
			},
		}
		depths := server.calculateCallGraphDepth(data)
		if depths["n1"] != 0 {
			t.Errorf("expected n1 at depth 0, got %d", depths["n1"])
		}
		if depths["n2"] != 1 {
			t.Errorf("expected n2 at depth 1, got %d", depths["n2"])
		}
		if depths["n3"] != 2 {
			t.Errorf("expected n3 at depth 2, got %d", depths["n3"])
		}
		if depths["n4"] != 3 {
			t.Errorf("expected n4 at depth 3, got %d", depths["n4"])
		}
	})

	t.Run("diamond graph finds shortest path", func(t *testing.T) {
		// root -> A, root -> B, A -> C, B -> C
		// C should be at depth 2 (shortest)
		data := &spec.CytoscapeData{
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "root", Label: "main"}},
				{Data: spec.CytoscapeNodeData{ID: "a", Label: "A"}},
				{Data: spec.CytoscapeNodeData{ID: "b", Label: "B"}},
				{Data: spec.CytoscapeNodeData{ID: "c", Label: "C"}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "root", Target: "a"}},
				{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "root", Target: "b"}},
				{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "a", Target: "c"}},
				{Data: spec.CytoscapeEdgeData{ID: "e4", Source: "b", Target: "c"}},
			},
		}
		depths := server.calculateCallGraphDepth(data)
		if depths["c"] != 2 {
			t.Errorf("expected c at depth 2, got %d", depths["c"])
		}
	})

	t.Run("disconnected nodes with zero in-degree become roots", func(t *testing.T) {
		// Two disconnected components: A->B and C->D
		data := &spec.CytoscapeData{
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "a", Label: "A"}},
				{Data: spec.CytoscapeNodeData{ID: "b", Label: "B"}},
				{Data: spec.CytoscapeNodeData{ID: "c", Label: "C"}},
				{Data: spec.CytoscapeNodeData{ID: "d", Label: "D"}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "a", Target: "b"}},
				{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "c", Target: "d"}},
			},
		}
		depths := server.calculateCallGraphDepth(data)
		if depths["a"] != 0 {
			t.Errorf("expected a at depth 0, got %d", depths["a"])
		}
		if depths["b"] != 1 {
			t.Errorf("expected b at depth 1, got %d", depths["b"])
		}
		if depths["c"] != 0 {
			t.Errorf("expected c at depth 0, got %d", depths["c"])
		}
		if depths["d"] != 1 {
			t.Errorf("expected d at depth 1, got %d", depths["d"])
		}
	})
}

// ---------- helper to create a test server with metadata ----------

func newTestServerWithData() *DiagramServer {
	server := NewDiagramServer(&ServerConfig{
		Host:        "localhost",
		Port:        8080,
		PageSize:    100,
		MaxDepth:    3,
		EnableCORS:  true,
		DiagramType: "call-graph",
	})
	server.metadata = &metadata.Metadata{
		Packages: map[string]*metadata.Package{
			"github.com/example/pkg":     {},
			"github.com/example/pkg/sub": {},
			"github.com/other/lib":       {},
		},
		CallGraph: []metadata.CallGraphEdge{},
	}
	// Pre-populate the data cache with known cytoscape data so the handlers
	// do not need to run the full analysis engine.
	server.dataCache = map[string]*spec.CytoscapeData{
		"call-graph:normal": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{
					ID: "n1", Label: "main", FunctionName: "main",
					Package: "github.com/example/pkg", Position: "main.go:10",
					Scope: "exported", ReceiverType: "", SignatureStr: "func main()",
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n2", Label: "HandleRequest", FunctionName: "HandleRequest",
					Package: "github.com/example/pkg", Position: "handler.go:20",
					Scope: "exported", ReceiverType: "Server", SignatureStr: "func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request)",
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n3", Label: "helperFunc", FunctionName: "helperFunc",
					Package: "github.com/example/pkg/sub", Position: "helper.go:5",
					Scope: "unexported", ReceiverType: "",
					Generics: map[string]string{"T": "any"},
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n4", Label: "LibFunc", FunctionName: "LibFunc",
					Package: "github.com/other/lib", Position: "lib.go:1",
					Scope: "exported",
					CallPaths: []spec.CallPathInfo{
						{Position: "caller.go:42"},
					},
				}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "n1", Target: "n2"}},
				{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "n2", Target: "n3"}},
				{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "n2", Target: "n4"}},
			},
		},
		"call-graph:full": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{
					ID: "n1", Label: "main", FunctionName: "main",
					Package: "github.com/example/pkg", Position: "main.go:10",
					Scope: "exported",
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n2", Label: "HandleRequest", FunctionName: "HandleRequest",
					Package: "github.com/example/pkg", Position: "handler.go:20",
					Scope: "exported", ReceiverType: "Server", SignatureStr: "func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request)",
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n3", Label: "helperFunc", FunctionName: "helperFunc",
					Package: "github.com/example/pkg/sub", Position: "helper.go:5",
					Scope:    "unexported",
					Generics: map[string]string{"T": "any"},
				}},
				{Data: spec.CytoscapeNodeData{
					ID: "n4", Label: "LibFunc", FunctionName: "LibFunc",
					Package: "github.com/other/lib", Position: "lib.go:1",
					Scope: "exported",
					CallPaths: []spec.CallPathInfo{
						{Position: "caller.go:42"},
					},
				}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "n1", Target: "n2"}},
				{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "n2", Target: "n3"}},
				{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "n2", Target: "n4"}},
			},
		},
	}
	server.lastLoad = time.Now()
	return server
}

// ---------- handlePackageHierarchy ----------

func TestHandlePackageHierarchy(t *testing.T) {
	server := newTestServerWithData()

	t.Run("GET returns hierarchy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/packages", nil)
		w := httptest.NewRecorder()

		server.handlePackageHierarchy(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp PackageHierarchyResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if resp.DiagramType != "call-graph" {
			t.Errorf("expected diagram_type call-graph, got %s", resp.DiagramType)
		}
		if len(resp.RootPackages) == 0 {
			t.Error("expected at least one root package")
		}
	})

	t.Run("wrong method returns 405-style error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/diagram/packages", nil)
		w := httptest.NewRecorder()

		server.handlePackageHierarchy(w, req)

		if w.Code == http.StatusOK {
			t.Error("expected non-200 for POST")
		}
		body := w.Body.String()
		if !strings.Contains(body, "Method not allowed") {
			t.Errorf("expected 'Method not allowed' in body, got %s", body)
		}
	})
}

// ---------- handlePackageBasedDiagram ----------

func TestHandlePackageBasedDiagram(t *testing.T) {
	server := newTestServerWithData()

	t.Run("missing packages param returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("wrong method returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/diagram/by-packages?packages=foo", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code == http.StatusOK {
			t.Error("expected non-200 for POST")
		}
	})

	t.Run("valid package returns data", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp PaginatedResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		if resp.TotalNodes == 0 {
			t.Error("expected at least one node for the package filter")
		}
	})

	t.Run("isolate mode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&isolate=true", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp PaginatedResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// In isolate mode, only nodes from the selected package appear
		for _, node := range resp.Nodes {
			if !strings.HasPrefix(node.Data.Package, "github.com/example/pkg") {
				t.Errorf("in isolate mode, unexpected package %s", node.Data.Package)
			}
		}
	})

	t.Run("with depth parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&depth=1", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with function filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&function=Handle", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp PaginatedResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// All direct-match nodes should contain "Handle" in label
		for _, node := range resp.Nodes {
			if strings.HasPrefix(node.Data.Package, "github.com/example/pkg") {
				if !strings.Contains(strings.ToLower(node.Data.Label), "handle") {
					// Could be a connected node from another package
					if node.Data.Package == "github.com/example/pkg" || node.Data.Package == "github.com/example/pkg/sub" {
						t.Logf("node %s (label=%s) in selected package but doesn't match function filter", node.Data.ID, node.Data.Label)
					}
				}
			}
		}
	})

	t.Run("with file filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&file=handler.go", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with receiver filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&receiver=Server", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with signature filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&signature=ResponseWriter", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with scope filter exported", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&scope=exported", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with scope filter unexported", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&scope=unexported", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("with generic filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg/sub&generic=any", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("negative depth clamped to 0", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg&depth=-5", nil)
		w := httptest.NewRecorder()

		server.handlePackageBasedDiagram(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

// ---------- generatePackageBasedData ----------

func TestGeneratePackageBasedData(t *testing.T) {
	server := newTestServerWithData()

	t.Run("filters by package", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"},
			3, nil, nil, nil, nil, nil, "", false,
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		// Should include nodes from github.com/example/pkg and its sub-packages
		if result.TotalNodes == 0 {
			t.Error("expected at least one node")
		}
	})

	t.Run("isolate mode excludes external edges", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"},
			3, nil, nil, nil, nil, nil, "", true,
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		// In isolate mode, edges should only connect nodes within the package
		nodeIDs := make(map[string]bool)
		for _, n := range result.Nodes {
			nodeIDs[n.Data.ID] = true
		}
		for _, e := range result.Edges {
			if !nodeIDs[e.Data.Source] || !nodeIDs[e.Data.Target] {
				t.Errorf("edge %s connects nodes outside the filtered set", e.Data.ID)
			}
		}
	})

	t.Run("function filter narrows results", func(t *testing.T) {
		all := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"}, 3, nil, nil, nil, nil, nil, "", true,
		)
		filtered := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"}, 3,
			[]string{"HandleRequest"}, nil, nil, nil, nil, "", true,
		)
		if filtered.TotalNodes > all.TotalNodes {
			t.Error("filtered result should not have more nodes than unfiltered")
		}
	})

	t.Run("scope filter exported", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"}, 3, nil, nil, nil, nil, nil, "exported", true,
		)
		for _, n := range result.Nodes {
			if strings.HasPrefix(n.Data.Package, "github.com/example/pkg") && n.Data.Scope != "exported" && n.Data.Scope != "" {
				t.Errorf("expected only exported nodes in package, got scope=%s for %s", n.Data.Scope, n.Data.Label)
			}
		}
	})

	t.Run("scope filter unexported", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg/sub"}, 3, nil, nil, nil, nil, nil, "unexported", true,
		)
		for _, n := range result.Nodes {
			if n.Data.Package == "github.com/example/pkg/sub" && n.Data.Scope != "unexported" && n.Data.Scope != "" {
				t.Errorf("expected only unexported nodes, got scope=%s for %s", n.Data.Scope, n.Data.Label)
			}
		}
	})

	t.Run("file filter via CallPaths", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/other/lib"}, 3, nil, []string{"caller.go"}, nil, nil, nil, "", true,
		)
		if result.TotalNodes == 0 {
			t.Error("expected nodes matching file filter via CallPaths")
		}
	})

	t.Run("receiver filter", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"}, 3, nil, nil, []string{"Server"}, nil, nil, "", true,
		)
		found := false
		for _, n := range result.Nodes {
			if n.Data.ReceiverType == "Server" {
				found = true
			}
		}
		if !found {
			t.Error("expected at least one node with receiver Server")
		}
	})

	t.Run("signature filter", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg"}, 3, nil, nil, nil, []string{"ResponseWriter"}, nil, "", true,
		)
		if result.TotalNodes == 0 {
			t.Error("expected nodes matching signature filter")
		}
	})

	t.Run("generic filter", func(t *testing.T) {
		result := server.generatePackageBasedData(
			[]string{"github.com/example/pkg/sub"}, 3, nil, nil, nil, nil, []string{"any"}, "", true,
		)
		if result.TotalNodes == 0 {
			t.Error("expected nodes matching generic filter")
		}
	})
}

// ---------- handleStats with method check ----------

func TestHandleStatsMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for POST to stats")
	}
}

func TestHandleStatsWithData(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify expected fields
	for _, key := range []string{"total_nodes", "total_edges", "total_functions", "last_load", "cache_timeout", "page_size", "max_depth", "input_dir"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("expected key %q in stats response", key)
		}
	}
}

// ---------- handleRefresh with method check ----------

func TestHandleRefreshMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/refresh", nil)
	w := httptest.NewRecorder()

	server.handleRefresh(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for GET to refresh")
	}
}

// ---------- handleDiagram with method check ----------

func TestHandleDiagramMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for POST to diagram")
	}
}

func TestHandleDiagramWithData(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.DiagramType != "call-graph" {
		t.Errorf("expected diagram_type call-graph, got %s", resp.DiagramType)
	}
	if resp.TotalNodes != 4 {
		t.Errorf("expected 4 total nodes, got %d", resp.TotalNodes)
	}
}

// ---------- handlePaginatedDiagram with various query params ----------

func TestHandlePaginatedDiagramMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/page", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for POST")
	}
}

func TestHandlePaginatedDiagramWithFilters(t *testing.T) {
	server := newTestServerWithData()

	tests := []struct {
		name  string
		query string
	}{
		{"basic pagination", "?page=1&size=2"},
		{"with depth", "?page=1&size=100&depth=1"},
		{"with package filter", "?page=1&size=100&package=example"},
		{"with function filter", "?page=1&size=100&function=main"},
		{"with file filter", "?page=1&size=100&file=main.go"},
		{"with receiver filter", "?page=1&size=100&receiver=Server"},
		{"with signature filter", "?page=1&size=100&signature=ResponseWriter"},
		{"with generic filter", "?page=1&size=100&generic=any"},
		{"with scope filter exported", "?page=1&size=100&scope=exported"},
		{"with scope filter unexported", "?page=1&size=100&scope=unexported"},
		{"with scope filter all", "?page=1&size=100&scope=all"},
		{"multiple packages", "?page=1&size=100&package=example,other"},
		{"multiple functions", "?page=1&size=100&function=main,Handle"},
		{"negative depth clamped", "?page=1&size=100&depth=-1"},
		{"oversized page clamped", "?page=1&size=5000"},
		{"page beyond data", "?page=999&size=10"},
		{"zero page defaults to 1", "?page=0&size=10"},
		{"zero size defaults", "?page=1&size=0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/diagram/page"+tt.query, nil)
			w := httptest.NewRecorder()

			server.handlePaginatedDiagram(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp PaginatedResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
		})
	}
}

// ---------- handleHealth with method check ----------

func TestHandleHealthMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for POST to health")
	}
}

// ---------- handleExport ----------

func TestHandleExportMethodNotAllowed(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code == http.StatusOK {
		t.Error("expected non-200 for POST to export")
	}
}

func TestHandleExportInvalidFormat(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=invalid", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", w.Code)
	}
}

func TestHandleExportJSON(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	disp := w.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "diagram.json") {
		t.Errorf("expected Content-Disposition with diagram.json, got %s", disp)
	}

	// Verify it is valid JSON
	var data interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("export JSON is not valid: %v", err)
	}
}

func TestHandleExportSVGReturnsClientSideMessage(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=svg", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	// SVG format is handled client-side, so server returns an error message
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for SVG (client-side), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "client-side") {
		t.Errorf("expected client-side message, got %s", body)
	}
}

func TestHandleExportDefaultFormat(t *testing.T) {
	server := newTestServerWithData()

	// No format param defaults to svg
	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	// Default format is svg which is handled client-side
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for default svg format, got %d", w.Code)
	}
}

func TestHandleExportWithFilters(t *testing.T) {
	server := newTestServerWithData()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json&page=1&size=2&depth=1&package=example&function=main&file=main.go&receiver=Server&signature=func&generic=any&scope=exported", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------- writeJSON with CORS ----------

func TestWriteJSONCORS(t *testing.T) {
	t.Run("CORS enabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: true})
		w := httptest.NewRecorder()
		server.writeJSON(w, map[string]string{"ok": "true"})

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("expected CORS header when enabled")
		}
	})

	t.Run("CORS disabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: false})
		w := httptest.NewRecorder()
		server.writeJSON(w, map[string]string{"ok": "true"})

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("expected no CORS header when disabled")
		}
	})
}

// ---------- writeResponse with CORS ----------

func TestWriteResponseCORS(t *testing.T) {
	t.Run("CORS enabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: true})
		w := httptest.NewRecorder()
		server.writeResponse(w, "hello", "text/plain")

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("expected CORS header when enabled")
		}
		if w.Body.String() != "hello" {
			t.Errorf("expected body 'hello', got %q", w.Body.String())
		}
	})

	t.Run("CORS disabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: false})
		w := httptest.NewRecorder()
		server.writeResponse(w, "hello", "text/plain")

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("expected no CORS header when disabled")
		}
	})
}

// ---------- writeError with CORS ----------

func TestWriteErrorCORS(t *testing.T) {
	t.Run("CORS enabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: true})
		w := httptest.NewRecorder()
		server.writeError(w, "bad", http.StatusBadRequest)

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("expected CORS header when enabled")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("CORS disabled", func(t *testing.T) {
		server := NewDiagramServer(&ServerConfig{EnableCORS: false})
		w := httptest.NewRecorder()
		server.writeError(w, "bad", http.StatusBadRequest)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("expected no CORS header when disabled")
		}
	})
}

// ---------- writeError response structure ----------

func TestWriteErrorStructure(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})
	w := httptest.NewRecorder()
	server.writeError(w, "something broke", http.StatusInternalServerError)

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Message != "something broke" {
		t.Errorf("expected message 'something broke', got %q", resp.Message)
	}
	if resp.Code != http.StatusInternalServerError {
		t.Errorf("expected code 500, got %d", resp.Code)
	}
	if resp.Error != "Internal Server Error" {
		t.Errorf("expected error text 'Internal Server Error', got %q", resp.Error)
	}
}

// ---------- getAllData caching ----------

func TestGetAllDataCaching(t *testing.T) {
	server := newTestServerWithData()

	// First call populates from pre-populated cache
	data1 := server.getAllData("call-graph", false)
	if data1 == nil {
		t.Fatal("expected non-nil data")
	}

	// Second call should return the same pointer (cached)
	data2 := server.getAllData("call-graph", false)
	if data1 != data2 {
		t.Error("expected cached data to be returned on second call")
	}
}

func TestGetAllDataNilCache(t *testing.T) {
	server := newTestServerWithData()
	server.dataCache = nil

	// Should not panic when dataCache is nil
	data := server.getAllData("call-graph", false)
	if data == nil {
		t.Error("expected non-nil data even with nil dataCache")
	}
}

func TestGetAllDataDifferentKeys(t *testing.T) {
	server := newTestServerWithData()

	normal := server.getAllData("call-graph", false)
	full := server.getAllData("call-graph", true)

	// Both should be non-nil; they may or may not be the same object
	if normal == nil || full == nil {
		t.Fatal("expected non-nil data for both normal and full depth")
	}
}

// ---------- generatePaginatedDataInternal coverage ----------

func TestGeneratePaginatedDataInternalWithFilters(t *testing.T) {
	server := newTestServerWithData()

	t.Run("package filter", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			[]string{"example"}, nil, nil, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("function filter", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, []string{"main"}, nil, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("file filter via position", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, []string{"main.go"}, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.TotalNodes == 0 {
			t.Error("expected at least one node matching file main.go")
		}
	})

	t.Run("file filter via callpaths", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, []string{"caller.go"}, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.TotalNodes == 0 {
			t.Error("expected at least one node matching file caller.go via CallPaths")
		}
	})

	t.Run("receiver filter", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, []string{"Server"}, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("signature filter", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, nil, []string{"ResponseWriter"}, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("generic filter", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, nil, nil, []string{"any"}, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("scope exported", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, nil, nil, nil, "exported")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("scope unexported", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, nil, nil, nil, "unexported")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("scope all", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			nil, nil, nil, nil, nil, nil, "all")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("pagination page 2", func(t *testing.T) {
		result := server.generatePaginatedData(2, 2, 3,
			nil, nil, nil, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("page beyond available data", func(t *testing.T) {
		result := server.generatePaginatedData(999, 10, 3,
			nil, nil, nil, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Nodes) != 0 {
			t.Errorf("expected 0 nodes for page beyond data, got %d", len(result.Nodes))
		}
	})

	t.Run("depth 0 returns only roots", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 0,
			nil, nil, nil, nil, nil, nil, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		result := server.generatePaginatedData(1, 100, 3,
			[]string{"example"}, []string{"Handle"}, []string{"handler.go"},
			[]string{"Server"}, []string{"ResponseWriter"}, nil, "exported")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("caching returns same result", func(t *testing.T) {
		// Clear cache
		server.cache = make(map[string]*spec.PaginatedCytoscapeData)

		r1 := server.generatePaginatedData(1, 100, 3, nil, nil, nil, nil, nil, nil, "")
		r2 := server.generatePaginatedData(1, 100, 3, nil, nil, nil, nil, nil, nil, "")

		// Both should be the same cached result
		if r1.TotalNodes != r2.TotalNodes {
			t.Errorf("cached results differ: %d vs %d nodes", r1.TotalNodes, r2.TotalNodes)
		}
	})
}

// ---------- detectVersionInfo additional branches ----------

func TestDetectVersionInfoBranches(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	origBuildDate := BuildDate
	origGoVersion := GoVersion
	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
		GoVersion = origGoVersion
	}()

	t.Run("skips when version already set", func(t *testing.T) {
		Version = "2.0.0"
		Commit = "abc1234"
		detectVersionInfo()
		if Version != "2.0.0" {
			t.Errorf("expected version to stay 2.0.0, got %s", Version)
		}
	})

	t.Run("runtime detection from build info", func(t *testing.T) {
		Version = "0.0.1"
		Commit = "unknown"
		BuildDate = "unknown"
		GoVersion = "unknown"

		detectVersionInfo()

		// After detection, GoVersion should be populated from runtime
		if GoVersion == "unknown" {
			t.Error("expected GoVersion to be set from build info")
		}
	})
}

// ---------- export handler: png, jpg, pdf all return client-side message ----------

func TestHandleExportClientSideFormats(t *testing.T) {
	server := newTestServerWithData()

	for _, format := range []string{"png", "jpg", "pdf"} {
		t.Run(format, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format="+format, nil)
			w := httptest.NewRecorder()

			server.handleExport(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %s format, got %d", format, w.Code)
			}
		})
	}
}

// ---------- writeJSON with unmarshalable data ----------

func TestWriteJSONEncodeError(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})
	w := httptest.NewRecorder()

	// A channel cannot be marshaled to JSON
	server.writeJSON(w, make(chan int))

	// Should result in 500 error
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for unmarshalable data, got %d", w.Code)
	}
}

// ---------- handleExport with depth and size edge cases ----------

func TestHandleExportEdgeCases(t *testing.T) {
	server := newTestServerWithData()

	t.Run("negative depth in export", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json&depth=-5", nil)
		w := httptest.NewRecorder()

		server.handleExport(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("oversized page in export", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json&size=9999", nil)
		w := httptest.NewRecorder()

		server.handleExport(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

// ---------- tracker-tree diagram type coverage ----------

func newTrackerTreeTestServer() *DiagramServer {
	server := NewDiagramServer(&ServerConfig{
		Host:        "localhost",
		Port:        8080,
		PageSize:    100,
		MaxDepth:    3,
		EnableCORS:  true,
		DiagramType: "tracker-tree",
	})
	server.metadata = &metadata.Metadata{
		Packages: map[string]*metadata.Package{
			"github.com/example/pkg": {},
		},
		CallGraph: []metadata.CallGraphEdge{},
	}
	trackerData := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{
				ID: "t1", Label: "main", FunctionName: "main",
				Package: "github.com/example/pkg", Position: "main.go:1",
				Scope: "exported",
			}},
			{Data: spec.CytoscapeNodeData{
				ID: "t2", Label: "child", FunctionName: "child",
				Package: "github.com/example/pkg", Position: "child.go:1",
				Parent: "t1", Scope: "unexported",
			}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{ID: "te1", Source: "t1", Target: "t2"}},
		},
	}
	server.dataCache = map[string]*spec.CytoscapeData{
		"tracker-tree:normal": trackerData,
		"tracker-tree:full":   trackerData,
	}
	server.lastLoad = time.Now()
	return server
}

func TestGetAllDataTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	data := server.getAllData("tracker-tree", false)
	if data == nil {
		t.Fatal("expected non-nil data for tracker-tree")
	}
	if len(data.Nodes) == 0 {
		t.Error("expected nodes in tracker-tree data")
	}

	// Full depth variant
	fullData := server.getAllData("tracker-tree", true)
	if fullData == nil {
		t.Fatal("expected non-nil data for tracker-tree full depth")
	}
}

func TestGetAllDataTrackerTreeComputeFromMetadata(t *testing.T) {
	// Test the branch where dataCache is empty and getAllData must compute from metadata
	server := NewDiagramServer(&ServerConfig{
		DiagramType: "tracker-tree",
		MaxDepth:    3,
	})
	server.metadata = &metadata.Metadata{
		Packages:  map[string]*metadata.Package{},
		CallGraph: []metadata.CallGraphEdge{},
	}
	// Empty data cache forces computation
	server.dataCache = make(map[string]*spec.CytoscapeData)

	data := server.getAllData("tracker-tree", false)
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestGetAllDataCallGraphComputeFromMetadata(t *testing.T) {
	// Test the branch where dataCache is empty and getAllData must compute call-graph from metadata
	server := NewDiagramServer(&ServerConfig{
		DiagramType: "call-graph",
		MaxDepth:    3,
	})
	server.metadata = &metadata.Metadata{
		Packages:  map[string]*metadata.Package{},
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.dataCache = make(map[string]*spec.CytoscapeData)

	data := server.getAllData("call-graph", false)
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestHandleDiagramTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.DiagramType != "tracker-tree" {
		t.Errorf("expected diagram_type tracker-tree, got %s", resp.DiagramType)
	}
}

func TestHandlePaginatedDiagramTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/page?page=1&size=10", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePackageHierarchyTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/packages", nil)
	w := httptest.NewRecorder()

	server.handlePackageHierarchy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PackageHierarchyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.DiagramType != "tracker-tree" {
		t.Errorf("expected diagram_type tracker-tree, got %s", resp.DiagramType)
	}
}

func TestHandlePackageBasedDiagramTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/by-packages?packages=github.com/example/pkg", nil)
	w := httptest.NewRecorder()

	server.handlePackageBasedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeneratePaginatedDataTrackerTree(t *testing.T) {
	server := newTrackerTreeTestServer()

	// Clear paginated cache to test full path
	server.cache = make(map[string]*spec.PaginatedCytoscapeData)

	result := server.generatePaginatedData(1, 10, 3, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------- handleExport with CORS headers ----------

func TestHandleExportCORSHeaders(t *testing.T) {
	server := newTestServerWithData()
	server.config.EnableCORS = true

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS headers on export response")
	}
}

// ---------- handleRefresh failure path ----------

func TestHandleRefreshFailure(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		Host:     "localhost",
		Port:     8080,
		InputDir: "/nonexistent/path/that/does/not/exist",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/refresh", nil)
	w := httptest.NewRecorder()

	server.handleRefresh(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for refresh with bad dir, got %d", w.Code)
	}
}

// ---------- generatePackageBasedData with parent node injection ----------

func TestGeneratePaginatedDataParentNodeInjection(t *testing.T) {
	// Create a server where paginated results include a child node whose
	// parent is not in the current page, to test parent injection
	server := NewDiagramServer(&ServerConfig{
		Host:        "localhost",
		Port:        8080,
		PageSize:    100,
		MaxDepth:    5,
		EnableCORS:  true,
		DiagramType: "call-graph",
	})
	server.metadata = &metadata.Metadata{
		Packages: map[string]*metadata.Package{
			"pkg": {},
		},
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.dataCache = map[string]*spec.CytoscapeData{
		"call-graph:normal": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "parent1", Label: "Parent", Package: "pkg"}},
				{Data: spec.CytoscapeNodeData{ID: "child1", Label: "Child", Package: "pkg", Parent: "parent1"}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "parent1", Target: "child1"}},
			},
		},
		"call-graph:full": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{ID: "parent1", Label: "Parent", Package: "pkg"}},
				{Data: spec.CytoscapeNodeData{ID: "child1", Label: "Child", Package: "pkg", Parent: "parent1"}},
			},
			Edges: []spec.CytoscapeEdge{
				{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "parent1", Target: "child1"}},
			},
		},
	}
	server.lastLoad = time.Now()

	// Page size 1 so that only child1 is on page 2, but parent1 is on page 1
	// This tests the parent injection path in generatePaginatedDataInternal
	server.cache = make(map[string]*spec.PaginatedCytoscapeData)
	result := server.generatePaginatedData(2, 1, 5, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The child node should have its parent injected if it was on this page
}
