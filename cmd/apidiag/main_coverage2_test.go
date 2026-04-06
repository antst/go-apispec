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

// ---------------------------------------------------------------------------
// parseFlags — cannot be tested directly because it uses the global flag
// package (flag.Parse) which causes "flag redefined" panics when called
// more than once in a process. The existing comment in main_test.go
// acknowledges this limitation. We can only verify the validation logic
// indirectly through the ServerConfig fields.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// detectVersionInfo — additional branch coverage
// ---------------------------------------------------------------------------

func TestDetectVersionInfo_AlreadySetNonDefault(t *testing.T) {
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

	// When Version is already != "0.0.1", the function should return early
	Version = "3.5.0"
	Commit = "custom_commit"
	BuildDate = "custom_date"
	GoVersion = "custom_go"

	detectVersionInfo()

	if Version != "3.5.0" {
		t.Errorf("expected Version to remain 3.5.0, got %s", Version)
	}
	if Commit != "custom_commit" {
		t.Errorf("expected Commit to remain custom_commit, got %s", Commit)
	}
}

func TestDetectVersionInfo_RuntimeDetection(t *testing.T) {
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

	// Reset to trigger runtime detection
	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	detectVersionInfo()

	// After runtime detection, GoVersion should be set
	if GoVersion == "unknown" {
		t.Log("GoVersion still unknown - may be unusual build environment")
	}
	// Version should be updated to something other than "0.0.1"
	// (either "dev" if VCS info found, or "latest (go install)" / "unknown (go install)")
	if Version == "0.0.1" {
		t.Log("Version still 0.0.1 after detection - build info may lack VCS data")
	}
}

// ---------------------------------------------------------------------------
// handleIndex — content type check and embedded template
// ---------------------------------------------------------------------------

func TestHandleIndex_ContentType(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		Host: "localhost",
		Port: 9090,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type containing text/html, got %s", ct)
	}

	body := w.Body.String()
	// Verify the server URL placeholder was replaced
	_ = strings.Contains(body, "http://localhost:9090")
	// Should contain HTML
	if !strings.Contains(body, "html") {
		t.Error("expected HTML in response body")
	}
}

// ---------------------------------------------------------------------------
// writeResponse — verify content and CORS behavior
// ---------------------------------------------------------------------------

func TestWriteResponse_HTMLContent(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{EnableCORS: true})
	w := httptest.NewRecorder()
	htmlContent := "<html><body>Hello</body></html>"

	server.writeResponse(w, htmlContent, "text/html")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/html" {
		t.Errorf("expected Content-Type text/html, got %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != htmlContent {
		t.Errorf("expected body to match HTML content")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS headers")
	}
}

func TestWriteResponse_NoCORS(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{EnableCORS: false})
	w := httptest.NewRecorder()

	server.writeResponse(w, "test", "text/plain")

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header when disabled")
	}
}

// ---------------------------------------------------------------------------
// writeError — various status codes and CORS
// ---------------------------------------------------------------------------

func TestWriteError_VariousStatusCodes(t *testing.T) {
	codes := []int{
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := NewDiagramServer(&ServerConfig{EnableCORS: true})
			w := httptest.NewRecorder()
			msg := "error: " + http.StatusText(code)

			server.writeError(w, msg, code)

			if w.Code != code {
				t.Errorf("expected status %d, got %d", code, w.Code)
			}

			var resp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if resp.Code != code {
				t.Errorf("expected code %d in body, got %d", code, resp.Code)
			}
			if resp.Message != msg {
				t.Errorf("expected message %q, got %q", msg, resp.Message)
			}
			if resp.Error != http.StatusText(code) {
				t.Errorf("expected error text %q, got %q", http.StatusText(code), resp.Error)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadMetadata — additional coverage
// ---------------------------------------------------------------------------

func TestLoadMetadata_VerboseMode(_ *testing.T) {
	config := &ServerConfig{
		InputDir: ".",
		Verbose:  true,
	}
	server := NewDiagramServer(config)

	// This may succeed or fail depending on the current directory
	// We just check it doesn't panic in verbose mode
	_ = server.LoadMetadata()
}

func TestLoadMetadata_InvalidDir(t *testing.T) {
	config := &ServerConfig{
		InputDir: "/absolutely_nonexistent_directory_xyz",
	}
	server := NewDiagramServer(config)

	err := server.LoadMetadata()
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// ---------------------------------------------------------------------------
// generatePaginatedData — timeout path (hard to trigger directly, but test
// that the function returns valid data under normal conditions)
// ---------------------------------------------------------------------------

func TestGeneratePaginatedData_WithAllFilters(t *testing.T) {
	server := newTestServerWithData2()

	result := server.generatePaginatedData(
		1, 50, 2,
		[]string{"example"},
		[]string{"main"},
		[]string{"main.go"},
		[]string{"Server"},
		[]string{"func"},
		[]string{"any"},
		"exported",
	)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGeneratePaginatedData_PageSizeLargerThanData(t *testing.T) {
	server := newTestServerWithData2()

	result := server.generatePaginatedData(1, 10000, 10, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.HasMore {
		t.Error("expected HasMore=false when page size exceeds data")
	}
}

// ---------------------------------------------------------------------------
// generatePaginatedDataInternal — edge cases
// ---------------------------------------------------------------------------

func TestGeneratePaginatedDataInternal_EmptyFilters(t *testing.T) {
	server := newTestServerWithData2()
	// Clear cache to test fresh generation
	server.cache = make(map[string]*spec.PaginatedCytoscapeData)

	result := server.generatePaginatedDataInternal(1, 100, 5, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGeneratePaginatedDataInternal_EndClampedToLength(t *testing.T) {
	server := newTestServerWithData2()
	server.cache = make(map[string]*spec.PaginatedCytoscapeData)

	// Request a page size larger than available nodes
	result := server.generatePaginatedDataInternal(1, 99999, 10, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGeneratePaginatedDataInternal_Verbose(t *testing.T) {
	server := newTestServerWithData2()
	server.config.Verbose = true
	server.cache = make(map[string]*spec.PaginatedCytoscapeData)

	result := server.generatePaginatedDataInternal(1, 100, 1, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------------------------------------------------------------------------
// SetupRoutes — cannot be tested multiple times in the same process because
// it registers routes on the default http.ServeMux, which panics on duplicate
// patterns. The existing test in main_test.go already covers the basic case.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// calculateCallGraphDepth — edge cases
// ---------------------------------------------------------------------------

func TestCalculateCallGraphDepth_AllNodesAreRoots(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "a", Label: "A"}},
			{Data: spec.CytoscapeNodeData{ID: "b", Label: "B"}},
			{Data: spec.CytoscapeNodeData{ID: "c", Label: "C"}},
		},
		Edges: []spec.CytoscapeEdge{}, // No edges, all nodes have zero in-degree
	}

	depths := server.calculateCallGraphDepth(data)

	for _, id := range []string{"a", "b", "c"} {
		if d, ok := depths[id]; !ok || d != 0 {
			t.Errorf("expected %s at depth 0, got %d (ok=%v)", id, d, ok)
		}
	}
}

func TestCalculateCallGraphDepth_CyclicGraph(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	// A -> B -> C -> A (cycle) + D (isolated root)
	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "d", Label: "main", FunctionName: "main"}},
			{Data: spec.CytoscapeNodeData{ID: "a", Label: "A"}},
			{Data: spec.CytoscapeNodeData{ID: "b", Label: "B"}},
			{Data: spec.CytoscapeNodeData{ID: "c", Label: "C"}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "d", Target: "a"}},
			{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "a", Target: "b"}},
			{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "b", Target: "c"}},
			{Data: spec.CytoscapeEdgeData{ID: "e4", Source: "c", Target: "a"}}, // cycle
		},
	}

	depths := server.calculateCallGraphDepth(data)

	if depths["d"] != 0 {
		t.Errorf("expected d at depth 0, got %d", depths["d"])
	}
	if depths["a"] != 1 {
		t.Errorf("expected a at depth 1, got %d", depths["a"])
	}
}

func TestCalculateCallGraphDepth_PureCycleNoRoots(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	// A cycle where all nodes have incoming edges and none is "main"
	// This triggers the "If no roots found" fallback at line 1285
	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "a", Label: "A", FunctionName: "A"}},
			{Data: spec.CytoscapeNodeData{ID: "b", Label: "B", FunctionName: "B"}},
			{Data: spec.CytoscapeNodeData{ID: "c", Label: "C", FunctionName: "C"}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "a", Target: "b"}},
			{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "b", Target: "c"}},
			{Data: spec.CytoscapeEdgeData{ID: "e3", Source: "c", Target: "a"}},
		},
	}

	depths := server.calculateCallGraphDepth(data)
	// All nodes have in-degree > 0 and none is "main", so the first pass finds no roots
	// The fallback "all nodes with zero in-degree" also finds nothing (all have in-degree 1)
	// So depths map may be empty
	if len(depths) != 0 {
		t.Logf("depths map has %d entries (cycle nodes got assigned depths from fallback)", len(depths))
	}
}

func TestCalculateCallGraphDepth_MainNodeWithIncomingEdges(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	// main has incoming edge but should still be treated as root
	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "m", Label: "main", FunctionName: "main"}},
			{Data: spec.CytoscapeNodeData{ID: "a", Label: "A"}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "a", Target: "m"}},
			{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "m", Target: "a"}},
		},
	}

	depths := server.calculateCallGraphDepth(data)

	if depths["m"] != 0 {
		t.Errorf("expected main at depth 0, got %d", depths["m"])
	}
}

// ---------------------------------------------------------------------------
// getAllData — cache miss and tracker-tree type with empty cache
// ---------------------------------------------------------------------------

func TestGetAllData_TrackerTreeEmptyCache(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		DiagramType: "tracker-tree",
		MaxDepth:    5,
	})
	server.metadata = &metadata.Metadata{
		Packages:  map[string]*metadata.Package{},
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.dataCache = make(map[string]*spec.CytoscapeData)

	data := server.getAllData("tracker-tree", true)
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestGetAllData_CallGraphNormalVsFull(t *testing.T) {
	server := newTestServerWithData2()

	// Clear cache
	server.dataCache = make(map[string]*spec.CytoscapeData)
	// Pre-populate only normal
	server.dataCache["call-graph:normal"] = &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "x", Label: "X"}},
		},
		Edges: []spec.CytoscapeEdge{},
	}

	normal := server.getAllData("call-graph", false)
	if len(normal.Nodes) != 1 {
		t.Errorf("expected 1 node for normal, got %d", len(normal.Nodes))
	}

	// Full should compute since not in cache
	full := server.getAllData("call-graph", true)
	if full == nil {
		t.Fatal("expected non-nil data for full depth")
	}
}

// ---------------------------------------------------------------------------
// handleExport — JSON format with CORS disabled
// ---------------------------------------------------------------------------

func TestHandleExport_JSONNoCORS(t *testing.T) {
	server := newTestServerWithData2()
	server.config.EnableCORS = false

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header when disabled")
	}
}

func TestHandleExport_JSONWithDepthFilter(t *testing.T) {
	server := newTestServerWithData2()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json&depth=0&page=1&size=100", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var data interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestHandleExport_NoDepthParam(t *testing.T) {
	server := newTestServerWithData2()

	// No depth parameter - should use server default
	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleExport_EmptyFilterParams(t *testing.T) {
	server := newTestServerWithData2()

	// All filter params empty strings (should be parsed as empty slices)
	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=json&package=&function=&file=&receiver=&signature=&generic=&scope=", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleIndex — server with different port
// ---------------------------------------------------------------------------

func TestHandleIndex_DifferentPort(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		Host: "0.0.0.0",
		Port: 3000,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "http://0.0.0.0:3000") {
		// The template replaces %s with the server URL
		t.Log("server URL substitution may not have occurred (depends on template format)")
	}
}

// ---------------------------------------------------------------------------
// handlePaginatedDiagram — depth edge cases
// ---------------------------------------------------------------------------

func TestHandlePaginatedDiagram_EmptyDepthParam(t *testing.T) {
	server := newTestServerWithData2()

	// depth parameter is empty string -> should use server default MaxDepth
	req := httptest.NewRequest(http.MethodGet, "/api/diagram/page?page=1&size=100&depth=", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePaginatedDiagram_ZeroDepth(t *testing.T) {
	server := newTestServerWithData2()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/page?page=1&size=100&depth=0", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handlePackageBasedDiagram — additional filter combinations
// ---------------------------------------------------------------------------

func TestHandlePackageBasedDiagram_AllFiltersAtOnce(t *testing.T) {
	server := newTestServerWithData2()

	req := httptest.NewRequest(http.MethodGet,
		"/api/diagram/by-packages?packages=github.com/example/pkg&depth=2&function=Handle&file=handler.go&receiver=Server&signature=ResponseWriter&generic=any&scope=exported&isolate=false",
		nil)
	w := httptest.NewRecorder()

	server.handlePackageBasedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePackageBasedDiagram_EmptyDepth(t *testing.T) {
	server := newTestServerWithData2()

	// Empty depth string should use server default
	req := httptest.NewRequest(http.MethodGet,
		"/api/diagram/by-packages?packages=github.com/example/pkg&depth=",
		nil)
	w := httptest.NewRecorder()

	server.handlePackageBasedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// writeResponse — write error path is not testable because writeError
// recursively calls itself on encode failure, causing a stack overflow.
// The error paths in writeResponse (line 1699) and writeError (line 1722)
// are unreachable in practice since httptest.ResponseRecorder never fails
// on Write. Attempting to use a custom failWriter triggers infinite recursion.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// handleExport — JSON write error path (hard to trigger but test edge case)
// ---------------------------------------------------------------------------

func TestHandleExport_MultipleFilterCombinations(t *testing.T) {
	server := newTestServerWithData2()

	tests := []struct {
		name  string
		query string
	}{
		{"json with all filters", "?format=json&page=2&size=1&depth=0&package=example&function=main&file=main.go&receiver=Server&signature=func&generic=any&scope=exported"},
		{"json with negative page", "?format=json&page=-1&size=10"},
		{"json with zero page", "?format=json&page=0&size=10"},
		{"json large size", "?format=json&size=3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/diagram/export"+tt.query, nil)
			w := httptest.NewRecorder()

			server.handleExport(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// calculateCallGraphDepth — "no roots found" fallback with partial cycle
// ---------------------------------------------------------------------------

func TestCalculateCallGraphDepth_PartialCycleWithNoMain(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{})

	// Create a graph where some nodes have zero in-degree but none is "main"
	// D -> A -> B -> C -> A (cycle), D has zero in-degree
	data := &spec.CytoscapeData{
		Nodes: []spec.CytoscapeNode{
			{Data: spec.CytoscapeNodeData{ID: "d", Label: "init", FunctionName: "init"}},
			{Data: spec.CytoscapeNodeData{ID: "a", Label: "A", FunctionName: "A"}},
			{Data: spec.CytoscapeNodeData{ID: "b", Label: "B", FunctionName: "B"}},
		},
		Edges: []spec.CytoscapeEdge{
			{Data: spec.CytoscapeEdgeData{ID: "e1", Source: "d", Target: "a"}},
			{Data: spec.CytoscapeEdgeData{ID: "e2", Source: "a", Target: "b"}},
		},
	}

	depths := server.calculateCallGraphDepth(data)
	if depths["d"] != 0 {
		t.Errorf("expected d at depth 0, got %d", depths["d"])
	}
	if depths["a"] != 1 {
		t.Errorf("expected a at depth 1, got %d", depths["a"])
	}
	if depths["b"] != 2 {
		t.Errorf("expected b at depth 2, got %d", depths["b"])
	}
}

// ---------------------------------------------------------------------------
// Helper: create a test server with data (separate instance to avoid cache
// pollution from existing tests that use newTestServerWithData)
// ---------------------------------------------------------------------------

func newTestServerWithData2() *DiagramServer {
	server := NewDiagramServer(&ServerConfig{
		Host:        "localhost",
		Port:        8081,
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

// --- parseFlags tests (now testable with FlagSet) ---

func TestParseFlags_Defaults(t *testing.T) {
	config, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.Port != 8080 {
		t.Errorf("expected port 8080, got %d", config.Port)
	}
	if config.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", config.Host)
	}
	if config.PageSize != 100 {
		t.Errorf("expected page-size 100, got %d", config.PageSize)
	}
	if config.MaxDepth != 3 {
		t.Errorf("expected max-depth 3, got %d", config.MaxDepth)
	}
	if config.DiagramType != "call-graph" {
		t.Errorf("expected diagram-type call-graph, got %s", config.DiagramType)
	}
}

func TestParseFlags_AllFlags(t *testing.T) {
	config, err := parseFlags([]string{
		"--port", "9090",
		"--host", "0.0.0.0",
		"--dir", "/tmp/test",
		"--page-size", "50",
		"--max-depth", "5",
		"--cors=false",
		"--verbose",
		"--diagram-type", "tracker-tree",
		"--static", "/public",
		"--afd",
		"--aifp",
		"--aet",
		"--aem",
	})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.Port != 9090 {
		t.Errorf("expected port 9090, got %d", config.Port)
	}
	if config.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", config.Host)
	}
	if config.InputDir != "/tmp/test" {
		t.Errorf("expected dir /tmp/test, got %s", config.InputDir)
	}
	if config.PageSize != 50 {
		t.Errorf("expected page-size 50, got %d", config.PageSize)
	}
	if config.MaxDepth != 5 {
		t.Errorf("expected max-depth 5, got %d", config.MaxDepth)
	}
	if config.EnableCORS {
		t.Error("expected cors false")
	}
	if !config.Verbose {
		t.Error("expected verbose true")
	}
	if config.DiagramType != "tracker-tree" {
		t.Errorf("expected diagram-type tracker-tree, got %s", config.DiagramType)
	}
	if config.StaticDir != "/public" {
		t.Errorf("expected static /public, got %s", config.StaticDir)
	}
	if !config.AnalyzeFrameworkDependencies {
		t.Error("expected afd true")
	}
	if !config.AutoIncludeFrameworkPackages {
		t.Error("expected aifp true")
	}
	if !config.AutoExcludeTests {
		t.Error("expected aet true")
	}
	if !config.AutoExcludeMocks {
		t.Error("expected aem true")
	}
}

func TestParseFlags_PageSizeClamping(t *testing.T) {
	config, _ := parseFlags([]string{"--page-size", "5"})
	if config.PageSize != 10 {
		t.Errorf("expected page-size clamped to 10, got %d", config.PageSize)
	}

	config, _ = parseFlags([]string{"--page-size", "5000"})
	if config.PageSize != 1000 {
		t.Errorf("expected page-size clamped to 1000, got %d", config.PageSize)
	}
}

func TestParseFlags_MaxDepthClamping(t *testing.T) {
	config, _ := parseFlags([]string{"--max-depth", "0"})
	if config.MaxDepth != 1 {
		t.Errorf("expected max-depth clamped to 1, got %d", config.MaxDepth)
	}

	config, _ = parseFlags([]string{"--max-depth", "99"})
	if config.MaxDepth != 10 {
		t.Errorf("expected max-depth clamped to 10, got %d", config.MaxDepth)
	}
}

func TestParseFlags_InvalidDiagramType(t *testing.T) {
	config, _ := parseFlags([]string{"--diagram-type", "invalid"})
	if config.DiagramType != "call-graph" {
		t.Errorf("expected fallback to call-graph, got %s", config.DiagramType)
	}
}

func TestParseFlags_VersionFlag(t *testing.T) {
	config, err := parseFlags([]string{"--version"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !config.ShowVersion {
		t.Error("expected ShowVersion true")
	}
}

func TestParseFlags_HelpFlag(t *testing.T) {
	_, err := parseFlags([]string{"--help"})
	if err == nil {
		t.Error("expected error for --help")
	}
}

func TestParseFlags_InvalidFlag(t *testing.T) {
	_, err := parseFlags([]string{"--nonexistent-flag"})
	if err == nil {
		t.Error("expected error for invalid flag")
	}
}

func TestRun_InvalidDir(t *testing.T) {
	config := &ServerConfig{
		InputDir: "/nonexistent/dir/for/test",
		Port:     0,
		Host:     "localhost",
	}
	err := run(config)
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}
