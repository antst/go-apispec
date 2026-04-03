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

package metadata

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// ---------- helpers ----------

// makePkg builds a minimal *packages.Package with the given import path and
// AST files that contain the listed import paths.
func makePkg(pkgPath string, imports ...string) *packages.Package {
	specs := make([]*ast.ImportSpec, 0, len(imports))
	for _, imp := range imports {
		specs = append(specs, &ast.ImportSpec{
			Path: &ast.BasicLit{Value: `"` + imp + `"`},
		})
	}
	file := &ast.File{
		Name:    ast.NewIdent("pkg"),
		Imports: specs,
	}
	return &packages.Package{
		PkgPath: pkgPath,
		Syntax:  []*ast.File{file},
		GoFiles: []string{"a.go"},
		Imports: make(map[string]*packages.Package),
	}
}

// makePkgWithMultipleFiles builds a package with multiple AST files.
func makePkgWithMultipleFiles(pkgPath string, fileImports ...[]string) *packages.Package {
	files := make([]*ast.File, 0, len(fileImports))
	for _, imps := range fileImports {
		specs := make([]*ast.ImportSpec, 0, len(imps))
		for _, imp := range imps {
			specs = append(specs, &ast.ImportSpec{
				Path: &ast.BasicLit{Value: `"` + imp + `"`},
			})
		}
		files = append(files, &ast.File{
			Name:    ast.NewIdent("pkg"),
			Imports: specs,
		})
	}
	return &packages.Package{
		PkgPath: pkgPath,
		Syntax:  files,
		GoFiles: []string{"a.go"},
		Imports: make(map[string]*packages.Package),
	}
}

// ---------- contains ----------

func TestContains(t *testing.T) {
	fd := NewFrameworkDetector()

	assert.True(t, fd.contains([]string{"a", "b", "c"}, "b"))
	assert.True(t, fd.contains([]string{"a", "b", "c"}, "a"))
	assert.True(t, fd.contains([]string{"a", "b", "c"}, "c"))
	assert.False(t, fd.contains([]string{"a", "b", "c"}, "d"))
	assert.False(t, fd.contains([]string{}, "a"))
	assert.False(t, fd.contains(nil, "a"))
}

// ---------- isTestMockPackage ----------

func TestIsTestMockPackage(t *testing.T) {
	fd := NewFrameworkDetector()

	tests := []struct {
		pkgPath string
		want    bool
	}{
		{"myproject/mock/handler", true},
		{"myproject/mocks/handler", true},
		{"myproject/test/handler", true},
		{"myproject/tests/handler", true},
		{"myproject/fake/handler", true},
		{"myproject/fakes/handler", true},
		{"myproject/stub/handler", true},
		{"myproject/stubs/handler", true},
		{"myproject/handler_mock", true},
		{"myproject/handler_test", true},
		{"myproject/handler_fake", true},
		{"myproject/handler_stub", true},
		{"myproject/mocked", true},
		{"myproject/handler", false},
		{"myproject/services", false},
		{"myproject/api", false},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			assert.Equal(t, tt.want, fd.isTestMockPackage(tt.pkgPath))
		})
	}
}

func TestIsTestMockPackage_CaseInsensitive(t *testing.T) {
	fd := NewFrameworkDetector()
	// Uppercase should still match since the code lowercases
	assert.True(t, fd.isTestMockPackage("myproject/MOCK/handler"))
	assert.True(t, fd.isTestMockPackage("myproject/MOCKED"))
}

// ---------- findCommonPrefix ----------

func TestFindCommonPrefix(t *testing.T) {
	fd := NewFrameworkDetector()

	tests := []struct {
		a, b, want string
	}{
		{"github.com/user/project/pkg1", "github.com/user/project/pkg2", "github.com/user/project/pkg"},
		{"abc", "abcdef", "abc"},
		{"abcdef", "abc", "abc"},
		{"", "abc", ""},
		{"abc", "", ""},
		{"abc", "xyz", ""},
		{"same", "same", "same"},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, fd.findCommonPrefix(tt.a, tt.b))
		})
	}
}

// ---------- detectProjectRoot ----------

func TestDetectProjectRoot_Empty(t *testing.T) {
	fd := NewFrameworkDetector()
	assert.Equal(t, "", fd.detectProjectRoot())
}

func TestDetectProjectRoot_SinglePackage(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.packages["myproject/handlers"] = makePkg("myproject/handlers")
	// Single package: common prefix is the full path
	root := fd.detectProjectRoot()
	// "myproject/handlers" => common prefix is "myproject/handlers"
	// Since len >= 3 and contains no ".", it returns it
	assert.Equal(t, "myproject/handlers", root)
}

func TestDetectProjectRoot_MultiplePackages(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.packages["myproject/handlers"] = makePkg("myproject/handlers")
	fd.packages["myproject/models"] = makePkg("myproject/models")
	fd.packages["myproject/services"] = makePkg("myproject/services")

	root := fd.detectProjectRoot()
	assert.Equal(t, "myproject/", root)
}

func TestDetectProjectRoot_DomainPackages(t *testing.T) {
	// When packages start with a domain like "github.com/user/project/..."
	// the common prefix will contain "." => falls to fallback logic
	fd := NewFrameworkDetector()
	fd.packages["github.com/user/project/handlers"] = makePkg("github.com/user/project/handlers")
	fd.packages["github.com/user/project/models"] = makePkg("github.com/user/project/models")

	root := fd.detectProjectRoot()
	// Common prefix is "github.com/user/project/" which contains "." so it falls to
	// looking for packages without dots in the first part. Since "github.com" has a dot,
	// none match, so returns "".
	assert.Equal(t, "", root)
}

func TestDetectProjectRoot_NonDomainFirstPart(t *testing.T) {
	// When common prefix contains "." but some packages have first part without "."
	fd := NewFrameworkDetector()
	fd.packages["myapp/handlers"] = makePkg("myapp/handlers")
	fd.packages["other.com/lib"] = makePkg("other.com/lib")

	root := fd.detectProjectRoot()
	// Common prefix between "myapp/handlers" and "other.com/lib" => "" (short)
	// Then fallback: look for packages without "." in first part
	// "myapp/handlers" has "myapp" without "." and len(parts) >= 2 => returns "myapp"
	// OR "other.com/lib" has "other.com" with "." => skip
	// Since map iteration is non-deterministic, let's just verify it returns "myapp"
	// Actually, with empty common prefix, len < 3 => falls through
	// Then iterates packages looking for parts[0] without "."
	// "myapp" qualifies => returns "myapp"
	assert.Equal(t, "myapp", root)
}

// ---------- fallbackProjectPackageDetection ----------

func TestFallbackProjectPackageDetection(t *testing.T) {
	fd := NewFrameworkDetector()

	tests := []struct {
		importPath string
		want       bool
		reason     string
	}{
		// Project patterns matching
		{"myproject/internal/handler", true, "contains /internal/"},
		{"myproject/pkg/utils", true, "contains /pkg/"},
		{"myproject/api/v1", true, "contains /api/"},
		{"myproject/handlers/auth", true, "contains /handlers/"},
		{"myproject/models/user", true, "contains /models/"},
		{"myproject/services/auth", true, "contains /services/"},
		{"myproject/middleware/cors", true, "contains /middleware/"},

		// Hyphen/underscore in first part
		{"my-project/something", true, "first part has hyphen"},
		{"my_project/something", true, "first part has underscore"},

		// Simple two-part without domain
		{"myproject/models", true, "two-part non-domain"},

		// Three slashes, not vendor
		{"some/path/deep/nested", true, "slash count >= 2, no vendor"},

		// Vendor should be excluded
		{"some/vendor/package", false, "contains vendor/"},

		// Single-word package (no slashes)
		{"fmt", false, "no slashes, not a project package"},

		// Standard library-like single part
		{"strings", false, "single word"},
	}

	for _, tt := range tests {
		t.Run(tt.importPath, func(t *testing.T) {
			assert.Equal(t, tt.want, fd.fallbackProjectPackageDetection(tt.importPath),
				"reason: %s", tt.reason)
		})
	}
}

// ---------- isProjectRelatedPackage ----------

func TestIsProjectRelatedPackage(t *testing.T) {
	fd := NewFrameworkDetector()

	// Test/mock package should be excluded
	assert.False(t, fd.isProjectRelatedPackage("myproject/mock/handler"))

	// External packages should be excluded
	assert.False(t, fd.isProjectRelatedPackage("github.com/gin-gonic/gin/internal"))
	assert.False(t, fd.isProjectRelatedPackage("golang.org/x/net"))
	assert.False(t, fd.isProjectRelatedPackage("github.com/stretchr/testify"))

	// Non-external, non-test packages should delegate to isIntelligentProjectPackage
	// With empty packages map, detectProjectRoot returns "" => fallback
	assert.True(t, fd.isProjectRelatedPackage("myproject/internal/handler"))
}

// ---------- isIntelligentProjectPackage ----------

func TestIsIntelligentProjectPackage_WithProjectRoot(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.packages["myproject/handlers"] = makePkg("myproject/handlers")
	fd.packages["myproject/models"] = makePkg("myproject/models")

	// "myproject/" is the common root
	assert.True(t, fd.isIntelligentProjectPackage("myproject/services"))
	assert.False(t, fd.isIntelligentProjectPackage("otherpkg/services"))
}

func TestIsIntelligentProjectPackage_FallbackPath(t *testing.T) {
	fd := NewFrameworkDetector()
	// No packages => empty root => fallback
	assert.True(t, fd.isIntelligentProjectPackage("my-project/handler"))
}

// ---------- isPackageImportedByProject ----------

func TestIsPackageImportedByProject(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("myproject/handlers", "myproject/models", "myproject/utils")
	fd.packages["myproject/handlers"] = pkg

	assert.True(t, fd.isPackageImportedByProject("myproject/models"))
	assert.True(t, fd.isPackageImportedByProject("myproject/utils"))
	assert.False(t, fd.isPackageImportedByProject("myproject/services"))
}

func TestIsPackageImportedByProject_EmptyPackages(t *testing.T) {
	fd := NewFrameworkDetector()
	assert.False(t, fd.isPackageImportedByProject("anything"))
}

// ---------- detectFrameworkType ----------

func TestDetectFrameworkType(t *testing.T) {
	fd := NewFrameworkDetector()

	tests := []struct {
		name     string
		imports  []string
		wantType string
	}{
		{"gin framework", []string{"github.com/gin-gonic/gin"}, "gin"},
		{"gin contrib", []string{"github.com/gin-contrib/cors"}, "gin"},
		{"echo framework", []string{"github.com/labstack/echo/v4"}, "echo"},
		{"fiber framework", []string{"github.com/gofiber/fiber/v2"}, "fiber"},
		{"chi framework", []string{"github.com/go-chi/chi/v5"}, "chi"},
		{"mux framework", []string{"github.com/gorilla/mux"}, "mux"},
		{"http framework", []string{"net/http"}, "http"},
		{"fasthttp framework", []string{"github.com/valyala/fasthttp"}, "fasthttp"},
		{"no framework", []string{"fmt", "os"}, ""},
		{"no imports", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := makePkg("test/pkg", tt.imports...)
			got := fd.detectFrameworkType(pkg)
			assert.Equal(t, tt.wantType, got)
		})
	}
}

func TestDetectFrameworkType_DisabledFramework(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.DisableFramework("gin")

	pkg := makePkg("test/pkg", "github.com/gin-gonic/gin")
	assert.Equal(t, "", fd.detectFrameworkType(pkg))
}

func TestDetectFrameworkType_MultipleFiles(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkgWithMultipleFiles("test/pkg",
		[]string{"fmt"},
		[]string{"github.com/gin-gonic/gin"},
	)
	assert.Equal(t, "gin", fd.detectFrameworkType(pkg))
}

func TestDetectFrameworkType_NilImportPath(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Imports: []*ast.ImportSpec{
			{Path: nil}, // nil path
		},
	}
	pkg := &packages.Package{
		PkgPath: "test/pkg",
		Syntax:  []*ast.File{file},
		Imports: make(map[string]*packages.Package),
	}
	assert.Equal(t, "", fd.detectFrameworkType(pkg))
}

// ---------- buildDependencyGraph ----------

func TestBuildDependencyGraph(t *testing.T) {
	fd := NewFrameworkDetector()

	pkgA := makePkg("project/a", "project/b", "project/c")
	pkgB := makePkg("project/b", "project/c")
	pkgC := makePkg("project/c")

	// Note: buildDependencyGraph re-initializes reverseDependencyGraph[pkgPath]
	// for each package in pkgs. If pkgA is processed first and adds reverse deps
	// for project/b, those get overwritten when pkgB is processed. So we process
	// in an order where dependencies come before dependents.
	fd.buildDependencyGraph([]*packages.Package{pkgC, pkgB, pkgA})

	// Check forward graph
	assert.Contains(t, fd.dependencyGraph["project/a"], "project/b")
	assert.Contains(t, fd.dependencyGraph["project/a"], "project/c")
	assert.Contains(t, fd.dependencyGraph["project/b"], "project/c")
	assert.Empty(t, fd.dependencyGraph["project/c"])

	// Check reverse graph: only entries added AFTER the pkg's own initialization survive.
	// With order [C, B, A]: C is initialized, then B is initialized, then B adds
	// reverse dep for C. Then A is initialized, then A adds reverse deps for B and C.
	assert.Contains(t, fd.reverseDependencyGraph["project/b"], "project/a")
	assert.Contains(t, fd.reverseDependencyGraph["project/c"], "project/a")
	assert.Contains(t, fd.reverseDependencyGraph["project/c"], "project/b")
}

func TestBuildDependencyGraph_EmptyImportPath(t *testing.T) {
	fd := NewFrameworkDetector()

	// Create a package with an empty import path
	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Imports: []*ast.ImportSpec{
			{Path: &ast.BasicLit{Value: `""`}},
			{Path: &ast.BasicLit{Value: `"valid/import"`}},
		},
	}
	pkg := &packages.Package{
		PkgPath: "test/pkg",
		Syntax:  []*ast.File{file},
		Imports: make(map[string]*packages.Package),
	}

	fd.buildDependencyGraph([]*packages.Package{pkg})
	// Empty import path should be skipped
	assert.Equal(t, []string{"valid/import"}, fd.dependencyGraph["test/pkg"])
}

func TestBuildDependencyGraph_NilImportPath(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Imports: []*ast.ImportSpec{
			{Path: nil},
		},
	}
	pkg := &packages.Package{
		PkgPath: "test/pkg",
		Syntax:  []*ast.File{file},
		Imports: make(map[string]*packages.Package),
	}

	fd.buildDependencyGraph([]*packages.Package{pkg})
	assert.Empty(t, fd.dependencyGraph["test/pkg"])
}

// ---------- dependsOnFrameworkPackage ----------

func TestDependsOnFrameworkPackage_Direct(t *testing.T) {
	fd := NewFrameworkDetector()

	fd.dependencyGraph["project/handler"] = []string{"project/framework"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	assert.True(t, fd.dependsOnFrameworkPackage("project/handler", frameworkPkgs))
}

func TestDependsOnFrameworkPackage_NoDependency(t *testing.T) {
	fd := NewFrameworkDetector()

	fd.dependencyGraph["project/handler"] = []string{"project/utils"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	assert.False(t, fd.dependsOnFrameworkPackage("project/handler", frameworkPkgs))
}

func TestDependsOnFrameworkPackage_Transitive(t *testing.T) {
	fd := NewFrameworkDetector()

	fd.dependencyGraph["project/handler"] = []string{"project/service"}
	fd.dependencyGraph["project/service"] = []string{"project/framework"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	assert.True(t, fd.dependsOnFrameworkPackage("project/handler", frameworkPkgs))
}

// ---------- hasTransitiveFrameworkDependency ----------

func TestHasTransitiveFrameworkDependency_Cycle(t *testing.T) {
	fd := NewFrameworkDetector()

	// Create a cycle: a -> b -> c -> a
	fd.dependencyGraph["project/a"] = []string{"project/b"}
	fd.dependencyGraph["project/b"] = []string{"project/c"}
	fd.dependencyGraph["project/c"] = []string{"project/a"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	visited := make(map[string]bool)
	// Should not loop forever
	assert.False(t, fd.hasTransitiveFrameworkDependency("project/a", frameworkPkgs, visited))
}

func TestHasTransitiveFrameworkDependency_DeepTransitive(t *testing.T) {
	fd := NewFrameworkDetector()

	fd.dependencyGraph["project/a"] = []string{"project/b"}
	fd.dependencyGraph["project/b"] = []string{"project/c"}
	fd.dependencyGraph["project/c"] = []string{"project/d"}
	fd.dependencyGraph["project/d"] = []string{"project/framework"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	visited := make(map[string]bool)
	assert.True(t, fd.hasTransitiveFrameworkDependency("project/a", frameworkPkgs, visited))
}

func TestHasTransitiveFrameworkDependency_AlreadyVisited(t *testing.T) {
	fd := NewFrameworkDetector()

	fd.dependencyGraph["project/a"] = []string{"project/framework"}

	frameworkPkgs := map[string]*FrameworkDependency{
		"project/framework": {PackagePath: "project/framework"},
	}

	visited := map[string]bool{"project/a": true}
	// Already visited => returns false immediately
	assert.False(t, fd.hasTransitiveFrameworkDependency("project/a", frameworkPkgs, visited))
}

// ---------- analyzeFileContents ----------

func TestAnalyzeFileContents(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("HandleRequest"),
				Type: &ast.FuncType{},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("ProcessData"),
				Type: &ast.FuncType{},
			},
			&ast.GenDecl{
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("MyStruct"),
						Type: &ast.StructType{
							Fields: &ast.FieldList{},
						},
					},
				},
			},
		},
	}

	dep := &FrameworkDependency{
		Functions: make([]string, 0),
		Types:     make([]string, 0),
	}

	fd.analyzeFileContents(file, dep)

	assert.Contains(t, dep.Functions, "HandleRequest")
	assert.Contains(t, dep.Functions, "ProcessData")
	assert.Contains(t, dep.Types, "MyStruct")
}

func TestAnalyzeFileContents_NoDuplicates(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("HandleRequest"),
				Type: &ast.FuncType{},
			},
		},
	}

	dep := &FrameworkDependency{
		Functions: []string{"HandleRequest"}, // already exists
		Types:     make([]string, 0),
	}

	fd.analyzeFileContents(file, dep)

	// Should not duplicate
	count := 0
	for _, f := range dep.Functions {
		if f == "HandleRequest" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestAnalyzeFileContents_NilNode(t *testing.T) {
	fd := NewFrameworkDetector()

	// File with no declarations (the Inspect callback will get nil)
	file := &ast.File{
		Name:  ast.NewIdent("pkg"),
		Decls: []ast.Decl{},
	}

	dep := &FrameworkDependency{
		Functions: make([]string, 0),
		Types:     make([]string, 0),
	}

	fd.analyzeFileContents(file, dep)
	assert.Empty(t, dep.Functions)
	assert.Empty(t, dep.Types)
}

// ---------- analyzePackageContents ----------

func TestAnalyzePackageContents_WithMetadata(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("Foo"),
				Type: &ast.FuncType{},
			},
		},
	}

	pkg := &packages.Package{
		PkgPath: "myproject/handler",
		Syntax:  []*ast.File{file},
		GoFiles: []string{"handler.go", "helper.go"},
		Imports: map[string]*packages.Package{
			"fmt": {},
			"os":  {},
		},
		Errors: nil,
	}

	dep := &FrameworkDependency{
		Files:     make([]string, 0),
		Functions: make([]string, 0),
		Types:     make([]string, 0),
		Metadata:  make(map[string]interface{}),
	}

	info := &types.Info{}
	pkgsMetadata := map[string]map[string]*ast.File{
		"myproject/handler": {
			"handler.go": file,
		},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file: info,
	}

	fd.analyzePackageContents(pkg, dep, pkgsMetadata, fileToInfo)

	assert.Contains(t, dep.Files, "handler.go")
	assert.Contains(t, dep.Functions, "Foo")
	assert.Equal(t, 0, dep.Metadata["syntax_errors"])
	assert.Equal(t, 2, dep.Metadata["imports_count"])
	assert.Equal(t, 2, dep.Metadata["files_count"])
}

func TestAnalyzePackageContents_NoPkgsMetadata(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := &packages.Package{
		PkgPath: "myproject/handler",
		GoFiles: []string{"handler.go"},
		Imports: make(map[string]*packages.Package),
	}

	dep := &FrameworkDependency{
		Files:     make([]string, 0),
		Functions: make([]string, 0),
		Types:     make([]string, 0),
		Metadata:  make(map[string]interface{}),
	}

	fd.analyzePackageContents(pkg, dep, nil, nil)

	// No files should be added since pkgsMetadata is nil
	assert.Empty(t, dep.Files)
	assert.Equal(t, 0, dep.Metadata["syntax_errors"])
}

func TestAnalyzePackageContents_FileNotInInfo(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("Bar"),
				Type: &ast.FuncType{},
			},
		},
	}

	pkg := &packages.Package{
		PkgPath: "myproject/handler",
		GoFiles: []string{"handler.go"},
		Imports: make(map[string]*packages.Package),
	}

	dep := &FrameworkDependency{
		Files:     make([]string, 0),
		Functions: make([]string, 0),
		Types:     make([]string, 0),
		Metadata:  make(map[string]interface{}),
	}

	pkgsMetadata := map[string]map[string]*ast.File{
		"myproject/handler": {
			"handler.go": file,
		},
	}
	// fileToInfo has no entry for this file => analyzeFileContents is NOT called
	fileToInfo := map[*ast.File]*types.Info{}

	fd.analyzePackageContents(pkg, dep, pkgsMetadata, fileToInfo)

	assert.Contains(t, dep.Files, "handler.go")
	// Functions should NOT be found since fileToInfo doesn't include the file
	assert.Empty(t, dep.Functions)
}

// ---------- findImportsRecursivelyWithDepth ----------

func TestFindImportsRecursivelyWithDepth_MaxDepth(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.config.MaxImportDepth = 1

	pkg := makePkg("project/handler", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models", "project/deep"),
		"project/deep":    makePkg("project/deep"),
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	// "project/models" should be found at depth 0
	// "project/deep" should NOT be found because depth=1 would exceed MaxImportDepth=1
	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/models")
	assert.NotContains(t, paths, "project/deep")
}

func TestFindImportsRecursivelyWithDepth_AtMaxDepth(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.config.MaxImportDepth = 2

	pkg := makePkg("project/handler", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models"),
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	// Start at depth >= MaxImportDepth => should immediately return
	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 2)

	assert.Empty(t, result)
}

func TestFindImportsRecursivelyWithDepth_SkipStdlib(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("project/handler", "fmt", "os", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models"),
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	// "fmt" and "os" are stdlib (no "/" or ".") => skipped
	assert.NotContains(t, paths, "fmt")
	assert.NotContains(t, paths, "os")
	assert.Contains(t, paths, "project/models")
}

func TestFindImportsRecursivelyWithDepth_SkipProcessed(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("project/handler", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models"),
	}

	imported := make(map[string]bool)
	processed := map[string]bool{"project/models": true} // already processed
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	assert.Empty(t, result)
}

func TestFindImportsRecursivelyWithDepth_SkipAlreadyImported(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("project/handler", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models"),
	}

	imported := map[string]bool{"project/models": true} // already imported
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	assert.Empty(t, result)
}

func TestFindImportsRecursivelyWithDepth_IncludeExternalPackages(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.config.IncludeExternalPackages = true

	pkg := makePkg("project/handler", "github.com/gin-gonic/gin")

	available := map[string]*packages.Package{
		"project/handler": pkg,
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "github.com/gin-gonic/gin")
}

func TestFindImportsRecursivelyWithDepth_PackageNotAvailable(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("project/handler", "project/models")

	// project/models is NOT in available packages
	available := map[string]*packages.Package{
		"project/handler": pkg,
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	require.Len(t, result, 1)
	assert.Equal(t, "project/models", result[0].PackagePath)
	assert.Equal(t, "imported", result[0].FrameworkType)
	// Should have metadata noting it's not in original analysis
	assert.Equal(t, "package not in original analysis", result[0].Metadata["note"])
	assert.Equal(t, "project/handler", result[0].Metadata["imported_by"])
}

func TestFindImportsRecursivelyWithDepth_NilImportPath(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("pkg"),
		Imports: []*ast.ImportSpec{
			{Path: nil},
		},
	}
	pkg := &packages.Package{
		PkgPath: "test/pkg",
		Syntax:  []*ast.File{file},
		Imports: make(map[string]*packages.Package),
	}

	available := map[string]*packages.Package{"test/pkg": pkg}
	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(pkg, available, imported, processed, &result, 0)

	assert.Empty(t, result)
}

// ---------- findImportsRecursively (wrapper) ----------

func TestFindImportsRecursively(t *testing.T) {
	fd := NewFrameworkDetector()

	pkg := makePkg("project/handler", "project/models")

	available := map[string]*packages.Package{
		"project/handler": pkg,
		"project/models":  makePkg("project/models"),
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursively(pkg, available, imported, processed, &result)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/models")
}

// ---------- findImportedPackages ----------

func TestFindImportedPackages(t *testing.T) {
	fd := NewFrameworkDetector()

	handlerPkg := makePkg("project/handler", "project/models", "project/utils")
	modelsPkg := makePkg("project/models")
	utilsPkg := makePkg("project/utils")

	directFramework := map[string]*FrameworkDependency{
		"project/handler": {PackagePath: "project/handler"},
	}

	pkgs := []*packages.Package{handlerPkg, modelsPkg, utilsPkg}
	processed := map[string]bool{
		"project/handler": true, // already processed as direct
	}

	result := fd.findImportedPackages(directFramework, pkgs, processed)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/models")
	assert.Contains(t, paths, "project/utils")
}

func TestFindImportedPackages_NoFrameworkPackages(t *testing.T) {
	fd := NewFrameworkDetector()

	directFramework := map[string]*FrameworkDependency{}
	pkgs := []*packages.Package{}
	processed := make(map[string]bool)

	result := fd.findImportedPackages(directFramework, pkgs, processed)
	assert.Empty(t, result)
}

func TestFindImportedPackages_FrameworkPkgNotInAvailable(t *testing.T) {
	fd := NewFrameworkDetector()

	directFramework := map[string]*FrameworkDependency{
		"project/handler": {PackagePath: "project/handler"},
	}

	// handler not in the pkgs list
	pkgs := []*packages.Package{}
	processed := make(map[string]bool)

	result := fd.findImportedPackages(directFramework, pkgs, processed)
	assert.Empty(t, result)
}

// ---------- findAllFrameworkPackages ----------

func TestFindAllFrameworkPackages(t *testing.T) {
	fd := NewFrameworkDetector()

	handlerPkg := makePkg("project/handler", "github.com/gin-gonic/gin", "project/models")
	modelsPkg := makePkg("project/models")
	servicePkg := makePkg("project/service", "project/handler")

	pkgs := []*packages.Package{handlerPkg, modelsPkg, servicePkg}

	// Build dependency graph first (as AnalyzeFrameworkDependencies would)
	for _, pkg := range pkgs {
		fd.packages[pkg.PkgPath] = pkg
	}
	fd.buildDependencyGraph(pkgs)

	result := fd.findAllFrameworkPackages(pkgs, nil, nil)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	// handler is direct gin framework
	assert.Contains(t, paths, "project/handler")

	// service depends on handler => indirect
	assert.Contains(t, paths, "project/service")
}

func TestFindAllFrameworkPackages_SkipsMocks(t *testing.T) {
	fd := NewFrameworkDetector()

	mockPkg := makePkg("project/mock/handler", "github.com/gin-gonic/gin")
	normalPkg := makePkg("project/handler", "github.com/gin-gonic/gin")

	pkgs := []*packages.Package{mockPkg, normalPkg}

	for _, pkg := range pkgs {
		fd.packages[pkg.PkgPath] = pkg
	}
	fd.buildDependencyGraph(pkgs)

	result := fd.findAllFrameworkPackages(pkgs, nil, nil)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/handler")
	assert.NotContains(t, paths, "project/mock/handler")
}

func TestFindAllFrameworkPackages_SkipsMocksInDependentPhase(t *testing.T) {
	fd := NewFrameworkDetector()

	handlerPkg := makePkg("project/handler", "github.com/gin-gonic/gin")
	mockDep := makePkg("project/mock_service", "project/handler")

	pkgs := []*packages.Package{handlerPkg, mockDep}

	for _, pkg := range pkgs {
		fd.packages[pkg.PkgPath] = pkg
	}
	fd.buildDependencyGraph(pkgs)

	result := fd.findAllFrameworkPackages(pkgs, nil, nil)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/handler")
	// mock_service matches pattern "_mock" => should be skipped
	assert.NotContains(t, paths, "project/mock_service")
}

// ---------- AnalyzeFrameworkDependencies ----------

func TestAnalyzeFrameworkDependencies(t *testing.T) {
	fd := NewFrameworkDetector()

	handlerPkg := makePkg("project/handler", "github.com/gin-gonic/gin")
	modelsPkg := makePkg("project/models")

	pkgs := []*packages.Package{handlerPkg, modelsPkg}

	list, err := fd.AnalyzeFrameworkDependencies(pkgs, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, list)

	assert.Equal(t, list.TotalPackages, len(list.AllPackages))
	assert.True(t, list.DirectPackages > 0)
	assert.True(t, list.TotalPackages >= list.DirectPackages)

	// Framework types should include "gin"
	_, hasGin := list.FrameworkTypes["gin"]
	assert.True(t, hasGin)
}

func TestAnalyzeFrameworkDependencies_EmptyPackages(t *testing.T) {
	fd := NewFrameworkDetector()

	list, err := fd.AnalyzeFrameworkDependencies([]*packages.Package{}, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, list)
	assert.Equal(t, 0, list.TotalPackages)
	assert.Equal(t, 0, list.DirectPackages)
	assert.Equal(t, 0, list.IndirectPackages)
}

func TestAnalyzeFrameworkDependencies_WithDependentPackages(t *testing.T) {
	fd := NewFrameworkDetector()

	handlerPkg := makePkg("project/handler", "github.com/gin-gonic/gin")
	servicePkg := makePkg("project/service", "project/handler")

	pkgs := []*packages.Package{handlerPkg, servicePkg}

	list, err := fd.AnalyzeFrameworkDependencies(pkgs, nil, nil, nil)
	require.NoError(t, err)

	assert.True(t, list.IndirectPackages > 0 || list.DirectPackages > 0)
}

// ---------- GetFrameworkPackages ----------

func TestGetFrameworkPackages(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", FrameworkType: "gin", IsDirect: true},
			{PackagePath: "project/api", FrameworkType: "gin", IsDirect: true},
			{PackagePath: "project/echo_handler", FrameworkType: "echo", IsDirect: true},
			{PackagePath: "project/service", FrameworkType: "dependent", IsDirect: false},
			{PackagePath: "project/models", FrameworkType: "imported", IsDirect: false},
		},
	}

	result := list.GetFrameworkPackages()

	// "dependent" should be excluded
	_, hasDep := result["dependent"]
	assert.False(t, hasDep)

	// "gin" should have 2 packages, sorted
	ginPkgs := result["gin"]
	require.Len(t, ginPkgs, 2)
	assert.Equal(t, "project/api", ginPkgs[0].PackagePath)
	assert.Equal(t, "project/handler", ginPkgs[1].PackagePath)

	// "echo" should have 1 package
	echoPkgs := result["echo"]
	require.Len(t, echoPkgs, 1)

	// "imported" should be included
	importedPkgs := result["imported"]
	require.Len(t, importedPkgs, 1)
}

func TestGetFrameworkPackages_Empty(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{},
	}

	result := list.GetFrameworkPackages()
	assert.Empty(t, result)
}

// ---------- GetImportedPackages ----------

func TestGetImportedPackages(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", FrameworkType: "gin"},
			{PackagePath: "project/models", FrameworkType: "imported"},
			{PackagePath: "project/utils", FrameworkType: "imported"},
			{PackagePath: "project/service", FrameworkType: "dependent"},
		},
	}

	result := list.GetImportedPackages()
	require.Len(t, result, 2)
	assert.Equal(t, "project/models", result[0].PackagePath)
	assert.Equal(t, "project/utils", result[1].PackagePath)
}

func TestGetImportedPackages_Empty(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", FrameworkType: "gin"},
		},
	}

	result := list.GetImportedPackages()
	assert.Nil(t, result)
}

// ---------- GetDirectPackages ----------

func TestGetDirectPackages(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", IsDirect: true},
			{PackagePath: "project/service", IsDirect: false},
			{PackagePath: "project/api", IsDirect: true},
		},
	}

	result := list.GetDirectPackages()
	require.Len(t, result, 2)
	assert.Equal(t, "project/handler", result[0].PackagePath)
	assert.Equal(t, "project/api", result[1].PackagePath)
}

func TestGetDirectPackages_NoDirect(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/service", IsDirect: false},
		},
	}

	result := list.GetDirectPackages()
	assert.Nil(t, result)
}

// ---------- GetIndirectPackages ----------

func TestGetIndirectPackages(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", IsDirect: true},
			{PackagePath: "project/service", IsDirect: false},
			{PackagePath: "project/models", IsDirect: false},
		},
	}

	result := list.GetIndirectPackages()
	require.Len(t, result, 2)
	assert.Equal(t, "project/service", result[0].PackagePath)
	assert.Equal(t, "project/models", result[1].PackagePath)
}

func TestGetIndirectPackages_NoIndirect(t *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{PackagePath: "project/handler", IsDirect: true},
		},
	}

	result := list.GetIndirectPackages()
	assert.Nil(t, result)
}

// ---------- PrintDependencyList ----------

func TestPrintDependencyList(_ *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{
			{
				PackagePath:   "project/handler",
				FrameworkType: "gin",
				IsDirect:      true,
				Files:         []string{"handler.go"},
				Functions:     []string{"HandleRequest"},
			},
			{
				PackagePath:   "project/models",
				FrameworkType: "imported",
				IsDirect:      false,
				Files:         []string{"model.go"},
				Functions:     []string{"NewModel"},
			},
			{
				PackagePath:   "project/service",
				FrameworkType: "dependent",
				IsDirect:      false,
				Files:         []string{},
				Functions:     []string{},
			},
		},
		FrameworkTypes: map[string][]string{
			"gin":       {"project/handler"},
			"imported":  {"project/models"},
			"dependent": {"project/service"},
		},
		TotalPackages:    3,
		DirectPackages:   1,
		IndirectPackages: 2,
	}

	// Should not panic
	list.PrintDependencyList()
}

func TestPrintDependencyList_Empty(_ *testing.T) {
	list := &FrameworkDependencyList{
		AllPackages:      []*FrameworkDependency{},
		FrameworkTypes:   make(map[string][]string),
		TotalPackages:    0,
		DirectPackages:   0,
		IndirectPackages: 0,
	}

	// Should not panic
	list.PrintDependencyList()
}

func TestPrintDependencyList_MissingDep(_ *testing.T) {
	// Test the case where pkgPath in FrameworkTypes doesn't match any AllPackages entry
	list := &FrameworkDependencyList{
		AllPackages: []*FrameworkDependency{},
		FrameworkTypes: map[string][]string{
			"gin": {"project/nonexistent"},
		},
		TotalPackages: 0,
	}

	// Should not panic even if dep is nil
	list.PrintDependencyList()
}

// ---------- DisableFramework with nil map ----------

func TestDisableFramework_NilDisabledMap(t *testing.T) {
	config := FrameworkDetectorConfig{
		FrameworkPatterns:  DefaultFrameworkDetectorConfig().FrameworkPatterns,
		DisabledFrameworks: nil, // nil map
	}
	fd := NewFrameworkDetectorWithConfig(config)

	// Should initialize the map and not panic
	fd.DisableFramework("http")
	assert.True(t, fd.config.DisabledFrameworks["http"])
}

// ---------- AddFrameworkPattern with nil map ----------

func TestAddFrameworkPattern_NilMap(t *testing.T) {
	config := FrameworkDetectorConfig{
		FrameworkPatterns: nil,
	}
	fd := NewFrameworkDetectorWithConfig(config)

	fd.AddFrameworkPattern("custom", []string{"custom/framework"})
	assert.Equal(t, []string{"custom/framework"}, fd.config.FrameworkPatterns["custom"])
}

// ---------- detectProjectRoot edge cases ----------

func TestDetectProjectRoot_ShortPrefix(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.packages["ab"] = makePkg("ab")
	fd.packages["ac"] = makePkg("ac")

	// Common prefix is "a", which has length < 3 and no "."
	// Then looks for packages without "." in first part and len(parts) >= 2
	// "ab" split by "/" => ["ab"] (len 1), "ac" split => ["ac"] (len 1)
	// Neither has len >= 2, so returns ""
	root := fd.detectProjectRoot()
	assert.Equal(t, "", root)
}

func TestDetectProjectRoot_EmptyCommonPrefix(t *testing.T) {
	// When common prefix becomes "" during iteration, the loop should break early
	fd := NewFrameworkDetector()
	fd.packages["abc/handler"] = makePkg("abc/handler")
	fd.packages["xyz/service"] = makePkg("xyz/service")

	root := fd.detectProjectRoot()
	// Common prefix is "" => short => fallback finds "abc" or "xyz" (both have no dot, len >= 2)
	assert.NotEmpty(t, root)
}

func TestDetectProjectRoot_CommonPrefixContainsDotButHasNonDomainPkg(t *testing.T) {
	fd := NewFrameworkDetector()
	// One package with domain, one without
	fd.packages["github.com/user/proj"] = makePkg("github.com/user/proj")
	fd.packages["myapp/handler"] = makePkg("myapp/handler")

	root := fd.detectProjectRoot()
	// Common prefix between them: empty or very short (no common prefix)
	// Falls to fallback: looks for first package without "." in parts[0]
	// "myapp/handler" => parts[0] = "myapp" (no dot), len >= 2 => returns "myapp"
	assert.Equal(t, "myapp", root)
}

// ---------- full integration: findAllFrameworkPackages with all phases ----------

func TestFindAllFrameworkPackages_AllPhases(t *testing.T) {
	fd := NewFrameworkDetector()

	// Phase 1: direct framework package
	ginHandler := makePkg("project/handler", "github.com/gin-gonic/gin", "project/models")
	// Phase 2: depends on framework package
	service := makePkg("project/service", "project/handler")
	// Phase 3: imported by framework package (project/models)
	models := makePkg("project/models")
	// Not related
	unrelated := makePkg("project/unrelated")

	pkgs := []*packages.Package{ginHandler, service, models, unrelated}
	for _, pkg := range pkgs {
		fd.packages[pkg.PkgPath] = pkg
	}
	fd.buildDependencyGraph(pkgs)

	result := fd.findAllFrameworkPackages(pkgs, nil, nil)

	paths := make(map[string]string)
	for _, d := range result {
		paths[d.PackagePath] = d.FrameworkType
	}

	// Direct gin framework package
	assert.Equal(t, "gin", paths["project/handler"])
	// Depends on direct framework package
	assert.Equal(t, "dependent", paths["project/service"])
	// Imported by framework package
	assert.Equal(t, "imported", paths["project/models"])
}

// ---------- findImportsRecursivelyWithDepth: recursive depth traversal ----------

func TestFindImportsRecursivelyWithDepth_RecursiveTraversal(t *testing.T) {
	fd := NewFrameworkDetector()
	fd.config.MaxImportDepth = 3

	// Chain: handler -> models -> dto
	handler := makePkg("project/handler", "project/models")
	models := makePkg("project/models", "project/dto")
	dto := makePkg("project/dto")

	available := map[string]*packages.Package{
		"project/handler": handler,
		"project/models":  models,
		"project/dto":     dto,
	}

	imported := make(map[string]bool)
	processed := make(map[string]bool)
	var result []*FrameworkDependency

	fd.findImportsRecursivelyWithDepth(handler, available, imported, processed, &result, 0)

	paths := make([]string, 0, len(result))
	for _, d := range result {
		paths = append(paths, d.PackagePath)
	}
	assert.Contains(t, paths, "project/models")
	assert.Contains(t, paths, "project/dto")
}

// ---------- isProjectRelatedPackage with various scenarios ----------

func TestIsProjectRelatedPackage_ExternalPrefixes(t *testing.T) {
	fd := NewFrameworkDetector()

	// These match default ExternalPrefixes
	assert.False(t, fd.isProjectRelatedPackage("golang.org/x/net"))
	assert.False(t, fd.isProjectRelatedPackage("google.golang.org/grpc"))
	assert.False(t, fd.isProjectRelatedPackage("go.uber.org/zap"))
	assert.False(t, fd.isProjectRelatedPackage("gorm.io/gorm"))
	assert.False(t, fd.isProjectRelatedPackage("gopkg.in/yaml.v3"))
	assert.False(t, fd.isProjectRelatedPackage("k8s.io/client-go"))
	assert.False(t, fd.isProjectRelatedPackage("sigs.k8s.io/controller-runtime"))
	assert.False(t, fd.isProjectRelatedPackage("github.com/google/uuid"))
}

// ---------- fallbackProjectPackageDetection edge cases ----------

func TestFallbackProjectPackageDetection_VendorExcluded(t *testing.T) {
	fd := NewFrameworkDetector()
	assert.False(t, fd.fallbackProjectPackageDetection("some/vendor/pkg"))
}

func TestFallbackProjectPackageDetection_TwoPartNoDomain(t *testing.T) {
	fd := NewFrameworkDetector()
	// Two parts, first part has no dot => project package
	assert.True(t, fd.fallbackProjectPackageDetection("myapp/handler"))
}

func TestFallbackProjectPackageDetection_TwoPartWithDomain(t *testing.T) {
	fd := NewFrameworkDetector()
	// Two parts, first part has a dot => not matching the two-part rule
	// But has 1 slash, not >= 2 => only two parts with dot in first part
	assert.False(t, fd.fallbackProjectPackageDetection("example.com/pkg"))
}

func TestFallbackProjectPackageDetection_ThreeSlashes(t *testing.T) {
	fd := NewFrameworkDetector()
	// 3 slashes but no vendor => true
	assert.True(t, fd.fallbackProjectPackageDetection("some/deep/path/package"))
}

func TestFallbackProjectPackageDetection_SinglePart(t *testing.T) {
	fd := NewFrameworkDetector()
	// No slashes => doesn't match any rule => false
	assert.False(t, fd.fallbackProjectPackageDetection("singlepackage"))
}

// ---------- AnalyzeFrameworkDependencies with full metadata ----------

func TestAnalyzeFrameworkDependencies_WithMetadata(t *testing.T) {
	fd := NewFrameworkDetector()

	file := &ast.File{
		Name: ast.NewIdent("handler"),
		Imports: []*ast.ImportSpec{
			{Path: &ast.BasicLit{Value: `"github.com/gin-gonic/gin"`}},
		},
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("Handle"),
				Type: &ast.FuncType{},
			},
			&ast.GenDecl{
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("Handler"),
						Type: &ast.StructType{Fields: &ast.FieldList{}},
					},
				},
			},
		},
	}

	pkg := &packages.Package{
		PkgPath: "project/handler",
		Syntax:  []*ast.File{file},
		GoFiles: []string{"handler.go"},
		Imports: map[string]*packages.Package{
			"github.com/gin-gonic/gin": {},
		},
	}

	info := &types.Info{}
	pkgsMetadata := map[string]map[string]*ast.File{
		"project/handler": {"handler.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}

	list, err := fd.AnalyzeFrameworkDependencies([]*packages.Package{pkg}, pkgsMetadata, fileToInfo, nil)
	require.NoError(t, err)
	require.NotNil(t, list)

	// Should have the direct framework package
	require.True(t, list.DirectPackages >= 1)

	// Find the handler dep
	var handlerDep *FrameworkDependency
	for _, d := range list.AllPackages {
		if d.PackagePath == "project/handler" {
			handlerDep = d
			break
		}
	}
	require.NotNil(t, handlerDep)
	assert.Equal(t, "gin", handlerDep.FrameworkType)
	assert.True(t, handlerDep.IsDirect)
	assert.Contains(t, handlerDep.Files, "handler.go")
	assert.Contains(t, handlerDep.Functions, "Handle")
	assert.Contains(t, handlerDep.Types, "Handler")
}
