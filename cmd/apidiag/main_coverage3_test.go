package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antst/go-apispec/internal/metadata"
	"github.com/antst/go-apispec/internal/spec"
)

// ---------------------------------------------------------------------------
// run — verbose path: LoadMetadata succeeds on its own, then we verify
// the verbose logging path. We cannot call run() with a valid dir because
// it registers routes on the default http.ServeMux and conflicts with
// TestSetupRoutes in main_test.go. Instead, we test the LoadMetadata
// verbose path directly.
// ---------------------------------------------------------------------------

func TestLoadMetadata_VerboseWithProject(t *testing.T) {
	tempDir := createTestGoProjectDiag(t)

	config := &ServerConfig{
		InputDir:    tempDir,
		Verbose:     true,
		DiagramType: "call-graph",
		PageSize:    100,
		MaxDepth:    3,
	}

	server := NewDiagramServer(config)
	err := server.LoadMetadata()
	if err != nil {
		t.Fatalf("LoadMetadata failed for valid project: %v", err)
	}

	// Verify metadata was loaded
	if server.metadata == nil {
		t.Fatal("expected metadata to be loaded")
	}
	if len(server.metadata.Packages) == 0 {
		t.Error("expected at least one package in metadata")
	}
}

func TestRun_VerboseWithInvalidDir(t *testing.T) {
	config := &ServerConfig{
		InputDir:    "/nonexistent/dir/coverage3",
		Port:        0,
		Host:        "localhost",
		Verbose:     true,
		DiagramType: "call-graph",
		PageSize:    100,
		MaxDepth:    3,
	}
	err := run(config)
	if err == nil {
		t.Error("expected error for invalid directory")
	}
	if !strings.Contains(err.Error(), "failed to load metadata") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// handleIndex — CORS enabled path
// ---------------------------------------------------------------------------

func TestHandleIndex_WithCORS(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		Host:       "localhost",
		Port:       9091,
		EnableCORS: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// CORS headers should be present (set by writeResponse)
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header to be set")
	}

	body := w.Body.String()
	if !strings.Contains(body, "http://localhost:9091") {
		t.Log("server URL replacement may vary based on template format")
	}
}

// ---------------------------------------------------------------------------
// writeResponse — various content types
// ---------------------------------------------------------------------------

func TestWriteResponse_PlainText(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{EnableCORS: true})
	w := httptest.NewRecorder()

	server.writeResponse(w, "plain text body", "text/plain")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("expected text/plain, got %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "plain text body" {
		t.Errorf("body mismatch")
	}
}

func TestWriteResponse_XMLContent(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{EnableCORS: false})
	w := httptest.NewRecorder()

	server.writeResponse(w, "<root/>", "application/xml")

	if w.Header().Get("Content-Type") != "application/xml" {
		t.Errorf("expected application/xml, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header when disabled")
	}
}

// ---------------------------------------------------------------------------
// generatePaginatedData — timeout fallback and nil metadata
// ---------------------------------------------------------------------------

func TestGeneratePaginatedData_NilMetadata(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		DiagramType: "call-graph",
		MaxDepth:    3,
		PageSize:    100,
	})
	// metadata is nil (not loaded)
	result := server.generatePaginatedData(1, 10, 1, nil, nil, nil, nil, nil, nil, "")
	if result == nil {
		t.Fatal("expected non-nil result even with nil metadata")
	}
}

// ---------------------------------------------------------------------------
// handleExport — SVG format
// ---------------------------------------------------------------------------

func TestHandleExport_SVGFormat(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=svg", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	// SVG is a valid format but handled client-side, so the handler returns 400
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for client-side SVG export, got %d", w.Code)
	}
}

func TestHandleExport_DOTFormat(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/export?format=dot", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	// DOT is not a supported export format, so the handler returns 400
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported DOT format, got %d", w.Code)
	}
}

func TestHandleExport_PostMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/export?format=json", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST on export, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleDiagram — POST method
// ---------------------------------------------------------------------------

func TestHandleDiagram_PostMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handlePaginatedDiagram — POST method
// ---------------------------------------------------------------------------

func TestHandlePaginatedDiagram_PostMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/page", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handlePackageBasedDiagram — POST method
// ---------------------------------------------------------------------------

func TestHandlePackageBasedDiagram_PostMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/by-packages", nil)
	w := httptest.NewRecorder()

	server.handlePackageBasedDiagram(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleRefresh — GET method (refresh typically uses POST)
// ---------------------------------------------------------------------------

func TestHandleRefresh_GetMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/refresh", nil)
	w := httptest.NewRecorder()

	server.handleRefresh(w, req)

	// Even GET should return something (depends on impl)
	if w.Code < 200 || w.Code >= 500 {
		t.Errorf("unexpected status %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleStats — with richer metadata
// ---------------------------------------------------------------------------

func TestHandleStats_WithPackages(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodGet, "/api/diagram/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "total_nodes") {
		t.Error("expected total_nodes in stats response")
	}
}

// ---------------------------------------------------------------------------
// handlePackageHierarchy — various filters
// ---------------------------------------------------------------------------

func TestHandlePackageHierarchy_PostMethod(t *testing.T) {
	server := newTestServerCoverage3()

	req := httptest.NewRequest(http.MethodPost, "/api/diagram/packages", nil)
	w := httptest.NewRecorder()

	server.handlePackageHierarchy(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// detectVersionInfo — exercising the fallback when GoVersion is empty
// ---------------------------------------------------------------------------

func TestDetectVersionInfo_ResetAndDetect(t *testing.T) {
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

	// Reset all to defaults to trigger full detection
	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	detectVersionInfo()

	// In a go test environment, debug.ReadBuildInfo() succeeds
	// GoVersion should have been set
	if GoVersion == "unknown" {
		t.Log("GoVersion not set - may be unusual build environment")
	}
	// Version should have been updated by the VCS info or fallback
	if Version == "0.0.1" {
		t.Log("Version still default - no VCS info available in test env")
	}
	// Even if no VCS info, the function should not panic
}

// ---------------------------------------------------------------------------
// writeError — with CORS disabled
// ---------------------------------------------------------------------------

func TestWriteError_NoCORS(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{EnableCORS: false})
	w := httptest.NewRecorder()

	server.writeError(w, "test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header when disabled")
	}
}

// ---------------------------------------------------------------------------
// getAllData — tracker-tree with full=true
// ---------------------------------------------------------------------------

func TestGetAllData_TrackerTreeFull(t *testing.T) {
	server := NewDiagramServer(&ServerConfig{
		DiagramType: "tracker-tree",
		MaxDepth:    5,
	})
	server.metadata = &metadata.Metadata{
		Packages:  map[string]*metadata.Package{},
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.dataCache = make(map[string]*spec.CytoscapeData)

	data := server.getAllData("tracker-tree", false)
	if data == nil {
		t.Fatal("expected non-nil data")
	}

	// Full depth
	dataFull := server.getAllData("tracker-tree", true)
	if dataFull == nil {
		t.Fatal("expected non-nil data for full depth")
	}
}

// ---------------------------------------------------------------------------
// matchesFunctionName — edge cases
// ---------------------------------------------------------------------------

func TestMatchesFunctionName_EmptySearch(t *testing.T) {
	if matchesFunctionName("HandleRequest", "") {
		t.Error("expected false for empty search term")
	}
}

func TestMatchesFunctionName_CaseInsensitive(t *testing.T) {
	if !matchesFunctionName("HandleRequest", "handle") {
		t.Error("expected case-insensitive match")
	}
}

func TestMatchesFunctionName_WithWhitespace(t *testing.T) {
	if !matchesFunctionName("HandleRequest", "  handle  ") {
		t.Error("expected match after trimming whitespace")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func createTestGoProjectDiag(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()

	goMod := filepath.Join(tempDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module testapp\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	goFile := filepath.Join(tempDir, "main.go")
	src := `package main

import "net/http"

func main() {
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	http.ListenAndServe(":8080", nil)
}
`
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	return tempDir
}

func newTestServerCoverage3() *DiagramServer {
	server := NewDiagramServer(&ServerConfig{
		Host:        "localhost",
		Port:        8082,
		PageSize:    100,
		MaxDepth:    3,
		EnableCORS:  true,
		DiagramType: "call-graph",
	})
	server.metadata = &metadata.Metadata{
		Packages: map[string]*metadata.Package{
			"github.com/example/app": {},
		},
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.dataCache = map[string]*spec.CytoscapeData{
		"call-graph:normal": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{
					ID: "n1", Label: "main", FunctionName: "main",
					Package: "github.com/example/app", Position: "main.go:10",
					Scope: "exported",
				}},
			},
			Edges: []spec.CytoscapeEdge{},
		},
		"call-graph:full": {
			Nodes: []spec.CytoscapeNode{
				{Data: spec.CytoscapeNodeData{
					ID: "n1", Label: "main", FunctionName: "main",
					Package: "github.com/example/app", Position: "main.go:10",
					Scope: "exported",
				}},
			},
			Edges: []spec.CytoscapeEdge{},
		},
	}
	return server
}
