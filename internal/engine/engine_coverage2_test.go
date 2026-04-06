package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
	"github.com/antst/go-apispec/spec"
)

// ---------------------------------------------------------------------------
// appendFrameworkPatterns tests
// ---------------------------------------------------------------------------

func TestAppendFrameworkPatterns_BothEmpty(t *testing.T) {
	dst := &spec.APISpecConfig{}
	src := &spec.APISpecConfig{}
	appendFrameworkPatterns(dst, src)

	assert.Empty(t, dst.Framework.RoutePatterns)
	assert.Empty(t, dst.Framework.RequestBodyPatterns)
	assert.Empty(t, dst.Framework.ResponsePatterns)
	assert.Empty(t, dst.Framework.ParamPatterns)
	assert.Empty(t, dst.Framework.MountPatterns)
	assert.Empty(t, dst.Framework.ContentTypePatterns)
}

func TestAppendFrameworkPatterns_AppendsAll(t *testing.T) {
	dst := spec.DefaultGinConfig()
	src := spec.DefaultChiConfig()

	origRouteLen := len(dst.Framework.RoutePatterns)
	origReqBodyLen := len(dst.Framework.RequestBodyPatterns)
	origRespLen := len(dst.Framework.ResponsePatterns)
	origParamLen := len(dst.Framework.ParamPatterns)
	origMountLen := len(dst.Framework.MountPatterns)
	origCTLen := len(dst.Framework.ContentTypePatterns)

	appendFrameworkPatterns(dst, src)

	assert.Len(t, dst.Framework.RoutePatterns, origRouteLen+len(src.Framework.RoutePatterns))
	assert.Len(t, dst.Framework.RequestBodyPatterns, origReqBodyLen+len(src.Framework.RequestBodyPatterns))
	assert.Len(t, dst.Framework.ResponsePatterns, origRespLen+len(src.Framework.ResponsePatterns))
	assert.Len(t, dst.Framework.ParamPatterns, origParamLen+len(src.Framework.ParamPatterns))
	assert.Len(t, dst.Framework.MountPatterns, origMountLen+len(src.Framework.MountPatterns))
	assert.Len(t, dst.Framework.ContentTypePatterns, origCTLen+len(src.Framework.ContentTypePatterns))
}

func TestAppendFrameworkPatterns_DstHasSrcEmpty(t *testing.T) {
	dst := spec.DefaultEchoConfig()
	src := &spec.APISpecConfig{}

	origRouteLen := len(dst.Framework.RoutePatterns)
	appendFrameworkPatterns(dst, src)

	// Should remain unchanged
	assert.Len(t, dst.Framework.RoutePatterns, origRouteLen)
}

func TestAppendFrameworkPatterns_DstEmptySrcHas(t *testing.T) {
	dst := &spec.APISpecConfig{}
	src := spec.DefaultFiberConfig()

	appendFrameworkPatterns(dst, src)

	assert.Len(t, dst.Framework.RoutePatterns, len(src.Framework.RoutePatterns))
	assert.Len(t, dst.Framework.RequestBodyPatterns, len(src.Framework.RequestBodyPatterns))
	assert.Len(t, dst.Framework.ResponsePatterns, len(src.Framework.ResponsePatterns))
	assert.Len(t, dst.Framework.ParamPatterns, len(src.Framework.ParamPatterns))
	assert.Len(t, dst.Framework.MountPatterns, len(src.Framework.MountPatterns))
	assert.Len(t, dst.Framework.ContentTypePatterns, len(src.Framework.ContentTypePatterns))
}

// ---------------------------------------------------------------------------
// defaultConfigForFramework tests
// ---------------------------------------------------------------------------

func TestDefaultConfigForFramework_AllKnown(t *testing.T) {
	frameworks := []struct {
		name     string
		expected *spec.APISpecConfig
	}{
		{"gin", spec.DefaultGinConfig()},
		{"chi", spec.DefaultChiConfig()},
		{"echo", spec.DefaultEchoConfig()},
		{"fiber", spec.DefaultFiberConfig()},
		{"mux", spec.DefaultMuxConfig()},
	}

	for _, tc := range frameworks {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultConfigForFramework(tc.name)
			require.NotNil(t, got, "defaultConfigForFramework(%q) returned nil", tc.name)
			// Verify the route patterns match what we expect from the named config
			assert.Equal(t, len(tc.expected.Framework.RoutePatterns), len(got.Framework.RoutePatterns),
				"route pattern count mismatch for %s", tc.name)
		})
	}
}

func TestDefaultConfigForFramework_Unknown(t *testing.T) {
	got := defaultConfigForFramework("unknown_framework")
	require.NotNil(t, got)
	// Unknown frameworks should fall back to HTTP config
	expected := spec.DefaultHTTPConfig()
	assert.Equal(t, len(expected.Framework.RoutePatterns), len(got.Framework.RoutePatterns))
}

func TestDefaultConfigForFramework_EmptyString(t *testing.T) {
	got := defaultConfigForFramework("")
	require.NotNil(t, got)
	expected := spec.DefaultHTTPConfig()
	assert.Equal(t, len(expected.Framework.RoutePatterns), len(got.Framework.RoutePatterns))
}

// ---------------------------------------------------------------------------
// generateDiagram tests
// ---------------------------------------------------------------------------

func TestGenerateDiagram_PaginatedRelativePath(t *testing.T) {
	tempDir := t.TempDir()

	e := NewEngine(&EngineConfig{
		DiagramPath:      "diagram.html",
		PaginatedDiagram: true,
		DiagramPageSize:  50,
	})
	e.config.moduleRoot = tempDir

	meta := &metadata.Metadata{
		Packages: make(map[string]*metadata.Package),
	}

	err := e.generateDiagram(meta)
	require.NoError(t, err)

	// Verify the diagram file was created
	diagramFile := filepath.Join(tempDir, "diagram.html")
	_, err = os.Stat(diagramFile)
	assert.NoError(t, err, "expected diagram.html to be created")
}

func TestGenerateDiagram_NonPaginatedRelativePath(t *testing.T) {
	tempDir := t.TempDir()

	e := NewEngine(&EngineConfig{
		DiagramPath:      "diagram.html",
		PaginatedDiagram: false,
	})
	e.config.moduleRoot = tempDir

	meta := &metadata.Metadata{
		Packages: make(map[string]*metadata.Package),
	}

	err := e.generateDiagram(meta)
	require.NoError(t, err)

	diagramFile := filepath.Join(tempDir, "diagram.html")
	_, err = os.Stat(diagramFile)
	assert.NoError(t, err, "expected diagram.html to be created (non-paginated)")
}

func TestGenerateDiagram_AbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	diagramPath := filepath.Join(tempDir, "abs_diagram.html")

	e := NewEngine(&EngineConfig{
		DiagramPath:      diagramPath,
		PaginatedDiagram: true,
		DiagramPageSize:  10,
	})
	e.config.moduleRoot = tempDir

	meta := &metadata.Metadata{
		Packages: make(map[string]*metadata.Package),
	}

	err := e.generateDiagram(meta)
	require.NoError(t, err)

	_, err = os.Stat(diagramPath)
	assert.NoError(t, err, "expected absolute diagram path to be created")
}

// ---------------------------------------------------------------------------
// findModuleRoot tests
// ---------------------------------------------------------------------------

func TestFindModuleRoot_DirectDir(t *testing.T) {
	tempDir := t.TempDir()
	goModPath := filepath.Join(tempDir, "go.mod")
	require.NoError(t, os.WriteFile(goModPath, []byte("module test\n\ngo 1.21\n"), 0644))

	e := NewEngine(nil)
	root, err := e.findModuleRoot(tempDir)
	require.NoError(t, err)
	assert.Equal(t, tempDir, root)
}

func TestFindModuleRoot_ChildDir(t *testing.T) {
	tempDir := t.TempDir()
	goModPath := filepath.Join(tempDir, "go.mod")
	require.NoError(t, os.WriteFile(goModPath, []byte("module test\n\ngo 1.21\n"), 0644))

	childDir := filepath.Join(tempDir, "internal", "pkg")
	require.NoError(t, os.MkdirAll(childDir, 0755))

	e := NewEngine(nil)
	root, err := e.findModuleRoot(childDir)
	require.NoError(t, err)
	assert.Equal(t, tempDir, root)
}

func TestFindModuleRoot_NoGoMod(t *testing.T) {
	tempDir := t.TempDir()

	e := NewEngine(nil)
	_, err := e.findModuleRoot(tempDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no go.mod found")
}

// ---------------------------------------------------------------------------
// matchesPackagePattern additional tests
// ---------------------------------------------------------------------------

func TestMatchesPackagePattern_EmptyPkgPath(t *testing.T) {
	got := matchesPackagePattern([]string{"foo"}, "")
	assert.False(t, got)
}

func TestMatchesPackagePattern_SingleSegmentPkg(t *testing.T) {
	// When pkgPath has no "/" the last segment is the full path
	got := matchesPackagePattern([]string{"main"}, "main")
	assert.True(t, got)
}

func TestMatchesPackagePattern_WildcardOnFullPath(t *testing.T) {
	got := matchesPackagePattern([]string{"github.com/foo/*"}, "github.com/foo/bar")
	assert.True(t, got)
}

// ---------------------------------------------------------------------------
// loadOrDetectConfig tests
// ---------------------------------------------------------------------------

func TestLoadOrDetectConfig_DirectConfig(t *testing.T) {
	directCfg := spec.DefaultGinConfig()
	e := NewEngine(&EngineConfig{
		APISpecConfig: directCfg,
	})

	cfg, err := e.loadOrDetectConfig()
	require.NoError(t, err)
	assert.Equal(t, directCfg, cfg)
}

func TestLoadOrDetectConfig_AutoDetect(t *testing.T) {
	tempDir := t.TempDir()

	// Create go.mod
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	// Create a Go file importing net/http so framework detection finds HTTP
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{})
	e.config.moduleRoot = tempDir

	cfg, err := e.loadOrDetectConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	// Should have route patterns from auto-detected framework
	assert.NotEmpty(t, cfg.Framework.RoutePatterns)
}

func TestLoadOrDetectConfig_WithConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create go.mod
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	// Create a Go file importing net/http
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	// Create a config file
	configPath := filepath.Join(tempDir, "apispec.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`framework:
  routePatterns:
    - callRegex: "^HandleFunc$"
      pathFromArg: true
      handlerFromArg: true
      pathArgIndex: 0
      methodArgIndex: -1
      handlerArgIndex: 1
      recvTypeRegex: "^net/http(\\.\\*ServeMux)?$"
defaults:
  requestContentType: "application/json"
  responseContentType: "application/json"
  responseStatus: 200
`), 0644))

	e := NewEngine(&EngineConfig{
		ConfigFile: configPath,
	})
	e.config.moduleRoot = tempDir

	cfg, err := e.loadOrDetectConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Framework.RoutePatterns)
	assert.Equal(t, "application/json", cfg.Defaults.RequestContentType)
}

func TestLoadOrDetectConfig_BadConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create go.mod
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	// Create a Go file importing net/http
	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{
		ConfigFile: "/nonexistent/config.yaml",
	})
	e.config.moduleRoot = tempDir

	_, err := e.loadOrDetectConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// ---------------------------------------------------------------------------
// GenerateOpenAPI additional coverage tests
// ---------------------------------------------------------------------------

func TestGenerateOpenAPI_WithSplitMetadata(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	eng := NewEngine(&EngineConfig{
		InputDir:      tempDir,
		WriteMetadata: true,
		SplitMetadata: true,
	})

	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify split metadata files were created
	for _, suffix := range []string{"-string-pool.yaml", "-packages.yaml", "-call-graph.yaml"} {
		path := filepath.Join(tempDir, "metadata"+suffix)
		_, statErr := os.Stat(path)
		assert.NoError(t, statErr, "expected split metadata file to exist: %s", path)
	}
}

func TestGenerateOpenAPI_WithOutputConfig(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	eng := NewEngine(&EngineConfig{
		InputDir:     tempDir,
		OutputConfig: "effective_config.yaml",
	})

	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the effective config file was created
	configPath := filepath.Join(tempDir, "effective_config.yaml")
	_, err = os.Stat(configPath)
	assert.NoError(t, err, "expected effective config file to be created")
}

func TestGenerateOpenAPI_WithDiagramNonPaginated(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	diagramPath := filepath.Join(tempDir, "diagram.html")
	eng := NewEngine(&EngineConfig{
		InputDir:         tempDir,
		DiagramPath:      diagramPath,
		PaginatedDiagram: false,
	})

	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	_, err = os.Stat(diagramPath)
	assert.NoError(t, err, "expected diagram file to be created")
}

func TestGenerateOpenAPI_WithRelativeDiagramPath(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	eng := NewEngine(&EngineConfig{
		InputDir:         tempDir,
		DiagramPath:      "my_diagram.html",
		PaginatedDiagram: true,
		DiagramPageSize:  25,
	})

	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	diagramFile := filepath.Join(tempDir, "my_diagram.html")
	_, err = os.Stat(diagramFile)
	assert.NoError(t, err, "expected relative diagram file to be created in module root")
}

func TestGenerateOpenAPI_WithRelativeOutputConfig(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("users"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	eng := NewEngine(&EngineConfig{
		InputDir:     tempDir,
		OutputConfig: "out_config.yaml",
	})

	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	configFile := filepath.Join(tempDir, "out_config.yaml")
	_, err = os.Stat(configFile)
	assert.NoError(t, err, "expected output config file to be created")

	// Read it back and verify it's valid YAML
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

// ---------------------------------------------------------------------------
// GenerateMetadataOnlyWithLogger additional coverage
// ---------------------------------------------------------------------------

func TestGenerateMetadataOnlyWithLogger_InvalidDir(t *testing.T) {
	e := NewEngine(&EngineConfig{
		InputDir: "/non/existent/path",
	})
	logger := NewVerboseLogger(false)

	_, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input directory does not exist")
}

func TestGenerateMetadataOnlyWithLogger_NoGoMod(t *testing.T) {
	tempDir := t.TempDir()

	e := NewEngine(&EngineConfig{
		InputDir: tempDir,
	})
	logger := NewVerboseLogger(false)

	_, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find Go module")
}

func TestGenerateMetadataOnlyWithLogger_ValidProject(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{
		InputDir: tempDir,
	})
	logger := NewVerboseLogger(false)

	meta, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.NoError(t, err)
	require.NotNil(t, meta)
}

func TestGenerateMetadataOnlyWithLogger_DisableFrameworkAnalysis(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{
		InputDir:                     tempDir,
		AnalyzeFrameworkDependencies: false,
		AutoIncludeFrameworkPackages: false,
	})
	logger := NewVerboseLogger(false)

	meta, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.NoError(t, err)
	require.NotNil(t, meta)
	// FrameworkDependencyList should be nil since analysis was disabled
	assert.Nil(t, meta.FrameworkDependencyList)
}

func TestGenerateMetadataOnlyWithLogger_VerboseOutput(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{
		InputDir: tempDir,
		Verbose:  true,
	})
	logger := NewVerboseLogger(true)

	out := captureStdout(func() {
		meta, err := e.GenerateMetadataOnlyWithLogger(logger)
		require.NoError(t, err)
		require.NotNil(t, meta)
	})

	// Verbose mode should produce some output about framework analysis
	assert.NotEmpty(t, out)
}

// ---------------------------------------------------------------------------
// analyzeFrameworkDependencies tests
// ---------------------------------------------------------------------------

func TestAnalyzeFrameworkDependencies_SkipHTTPFramework(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	e := NewEngine(&EngineConfig{
		InputDir:                     tempDir,
		SkipHTTPFramework:            true,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: true,
	})
	logger := NewVerboseLogger(false)

	meta, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.NoError(t, err)
	require.NotNil(t, meta)
	// Verify the framework dependency list was populated and does not contain net/http
	if meta.FrameworkDependencyList != nil {
		for _, pkg := range meta.FrameworkDependencyList.AllPackages {
			assert.NotEqual(t, "net/http", pkg.PackagePath, "net/http should be excluded by SkipHTTPFramework")
		}
	}
}

// ---------------------------------------------------------------------------
// autoIncludeFrameworkPackages verbose output tests
// ---------------------------------------------------------------------------

func TestAutoIncludeFrameworkPackages_VerboseOutput(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	logger := NewVerboseLogger(true)
	list := &metadata.FrameworkDependencyList{
		AllPackages: []*metadata.FrameworkDependency{
			{PackagePath: "github.com/foo/handler", FrameworkType: "gin", IsDirect: true},
			{PackagePath: "github.com/foo/routes", FrameworkType: "chi", IsDirect: false},
		},
	}

	out := captureStdout(func() {
		e.autoIncludeFrameworkPackages(list, logger)
	})

	assert.Contains(t, out, "Auto-including framework packages")
	assert.Contains(t, out, "Added 2 framework packages")
	assert.Contains(t, out, "github.com/foo/handler")
	assert.Contains(t, out, "(direct)")
	assert.Contains(t, out, "(indirect)")
}

// ---------------------------------------------------------------------------
// generateDiagram error path tests
// ---------------------------------------------------------------------------

func TestGenerateDiagram_PaginatedError(t *testing.T) {
	// Use a path in a non-existent deep directory to cause write failure
	e := NewEngine(&EngineConfig{
		DiagramPath:      "/nonexistent/deep/dir/diagram.html",
		PaginatedDiagram: true,
		DiagramPageSize:  10,
	})
	// Set moduleRoot so the absolute path check passes
	e.config.moduleRoot = "/nonexistent"

	meta := &metadata.Metadata{
		Packages: make(map[string]*metadata.Package),
	}

	err := e.generateDiagram(meta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate paginated diagram")
}

func TestGenerateDiagram_NonPaginatedError(t *testing.T) {
	e := NewEngine(&EngineConfig{
		DiagramPath:      "/nonexistent/deep/dir/diagram.html",
		PaginatedDiagram: false,
	})
	e.config.moduleRoot = "/nonexistent"

	meta := &metadata.Metadata{
		Packages: make(map[string]*metadata.Package),
	}

	err := e.generateDiagram(meta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate diagram")
}

// ---------------------------------------------------------------------------
// GenerateOpenAPI error path tests
// ---------------------------------------------------------------------------

func TestGenerateOpenAPI_DiagramError(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	eng := NewEngine(&EngineConfig{
		InputDir:         tempDir,
		DiagramPath:      "/nonexistent/deep/nested/path/diagram.html",
		PaginatedDiagram: true,
	})

	_, err := eng.GenerateOpenAPI()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate paginated diagram")
}

func TestGenerateOpenAPI_MetadataWriteError(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	// Create a non-empty directory at the metadata file path so os.Remove fails
	// with "directory not empty" (which is not os.ErrNotExist)
	metadataPath := filepath.Join(tempDir, DefaultMetadataFile)
	require.NoError(t, os.MkdirAll(metadataPath, 0755))
	// Add a file inside so the directory is non-empty
	require.NoError(t, os.WriteFile(filepath.Join(metadataPath, "blocker.txt"), []byte("x"), 0644))

	eng := NewEngine(&EngineConfig{
		InputDir:      tempDir,
		WriteMetadata: true,
		SplitMetadata: false,
	})

	_, err := eng.GenerateOpenAPI()
	require.Error(t, err)
}

func TestGenerateOpenAPI_SplitMetadataWriteError(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	// The split metadata writes to <base>-string-pool.yaml etc.
	// Create a non-empty directory at the string pool path to block writing
	basePath := filepath.Join(tempDir, "metadata") // DefaultMetadataFile "metadata.yaml" minus ".yaml"
	stringPoolPath := basePath + "-string-pool.yaml"
	require.NoError(t, os.MkdirAll(stringPoolPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stringPoolPath, "blocker.txt"), []byte("x"), 0644))

	eng := NewEngine(&EngineConfig{
		InputDir:      tempDir,
		WriteMetadata: true,
		SplitMetadata: true,
	})

	_, err := eng.GenerateOpenAPI()
	require.Error(t, err)
}

func TestGenerateOpenAPI_OutputConfigWriteError(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	// Use a path in a non-existent directory to force write failure
	eng := NewEngine(&EngineConfig{
		InputDir:     tempDir,
		OutputConfig: "/nonexistent/deep/nested/config.yaml",
	})

	_, err := eng.GenerateOpenAPI()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write effective config")
}

// ---------------------------------------------------------------------------
// loadOrDetectConfig error path tests
// ---------------------------------------------------------------------------

func TestLoadOrDetectConfig_DetectError(t *testing.T) {
	// Use a non-existent directory for moduleRoot to trigger Detect() error
	e := NewEngine(&EngineConfig{})
	e.config.moduleRoot = "/nonexistent/module/root"

	_, err := e.loadOrDetectConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to detect framework")
}

// ---------------------------------------------------------------------------
// GenerateMetadataOnlyWithLogger error in framework analysis
// ---------------------------------------------------------------------------

func TestGenerateMetadataOnlyWithLogger_FrameworkAnalysisNoAutoInclude(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	// Enable framework analysis but disable auto-include
	e := NewEngine(&EngineConfig{
		InputDir:                     tempDir,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: false,
	})
	logger := NewVerboseLogger(false)

	meta, err := e.GenerateMetadataOnlyWithLogger(logger)
	require.NoError(t, err)
	require.NotNil(t, meta)
	// FrameworkDependencyList should still be set even without auto-include
	assert.NotNil(t, meta.FrameworkDependencyList)
}
