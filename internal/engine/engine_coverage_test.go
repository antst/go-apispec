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

package engine

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"os"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/antst/go-apispec/internal/metadata"
	intspec "github.com/antst/go-apispec/internal/spec"
	"github.com/antst/go-apispec/spec"
)

// ---------------------------------------------------------------------------
// VerboseLogger tests
// ---------------------------------------------------------------------------

// captureStdout captures stdout during fn execution and returns the output.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestVerboseLogger_Print_Enabled(t *testing.T) {
	vl := NewVerboseLogger(true)
	out := captureStdout(func() {
		vl.Print("hello")
	})
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

func TestVerboseLogger_Print_Disabled(t *testing.T) {
	vl := NewVerboseLogger(false)
	out := captureStdout(func() {
		vl.Print("hello")
	})
	if out != "" {
		t.Errorf("expected empty output when verbose=false, got %q", out)
	}
}

func TestVerboseLogger_Printf_Enabled(t *testing.T) {
	vl := NewVerboseLogger(true)
	out := captureStdout(func() {
		vl.Printf("count=%d", 42)
	})
	if out != "count=42" {
		t.Errorf("expected 'count=42', got %q", out)
	}
}

func TestVerboseLogger_Printf_Disabled(t *testing.T) {
	vl := NewVerboseLogger(false)
	out := captureStdout(func() {
		vl.Printf("count=%d", 42)
	})
	if out != "" {
		t.Errorf("expected empty output when verbose=false, got %q", out)
	}
}

func TestVerboseLogger_Println_Enabled(t *testing.T) {
	vl := NewVerboseLogger(true)
	out := captureStdout(func() {
		vl.Println("line")
	})
	if out != "line\n" {
		t.Errorf("expected 'line\\n', got %q", out)
	}
}

func TestVerboseLogger_Println_Disabled(t *testing.T) {
	vl := NewVerboseLogger(false)
	out := captureStdout(func() {
		vl.Println("line")
	})
	if out != "" {
		t.Errorf("expected empty output when verbose=false, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// matchesPattern tests
// ---------------------------------------------------------------------------

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.py", false},
		{"**/*.go", "internal/pkg/main.go", true},
		{"cmd/*", "cmd/server", true},
		{"cmd/*", "pkg/server", false},
		{"foo", "foo", true},
		{"foo", "bar", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.value), func(t *testing.T) {
			got := matchesPattern(tt.pattern, tt.value)
			if got != tt.want {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isCGOProblematic tests
// ---------------------------------------------------------------------------

func TestIsCGOProblematic(t *testing.T) {
	tests := []struct {
		pkgPath string
		want    bool
	}{
		// Direct substring matches via strings.Contains fallback:
		{"github.com/mattn/go-sqlite3", true},
		{"github.com/foo/sqlite3", true},
		// The contains fallback strips first "*/" from pattern, so
		// "*/tensorflow/*" -> "tensorflow/*" which contains literal "*"
		// and won't match. But single-segment paths match the glob:
		{"x/tensorflow/y", true},     // matches */tensorflow/* glob
		{"x/govips/y", true},         // matches */govips/* glob
		{"x/opencv/y", true},         // matches */opencv/* glob
		{"x/ffmpeg/y", true},         // matches */ffmpeg/* glob
		{"x/graft/tensorflow", true}, // matches */graft/tensorflow pattern
		// Non-CGO packages:
		{"github.com/foo/bar/baz", false},
		{"github.com/foo/myapp/cmd", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			got := isCGOProblematic(tt.pkgPath)
			if got != tt.want {
				t.Errorf("isCGOProblematic(%q) = %v, want %v", tt.pkgPath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// matchesPackagePattern tests
// ---------------------------------------------------------------------------

func TestMatchesPackagePattern(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		pkgPath  string
		want     bool
	}{
		{"exact match on last segment", []string{"cmd"}, "github.com/foo/cmd", true},
		{"wildcard single segment", []string{"*/cmd"}, "foo/cmd", true},
		{"no match", []string{"internal"}, "github.com/foo/cmd", false},
		{"empty patterns", nil, "github.com/foo/cmd", false},
		{"multiple patterns first matches", []string{"cmd", "internal"}, "github.com/foo/cmd", true},
		{"multiple patterns second matches", []string{"internal", "cmd"}, "github.com/foo/cmd", true},
		{"double star", []string{"**/cmd"}, "github.com/foo/cmd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPackagePattern(tt.patterns, tt.pkgPath)
			if got != tt.want {
				t.Errorf("matchesPackagePattern(%v, %q) = %v, want %v", tt.patterns, tt.pkgPath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// shouldIncludePackage tests
// ---------------------------------------------------------------------------

func TestShouldIncludePackage_NoPatterns(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	// No include/exclude patterns => include everything
	if !e.shouldIncludePackage("github.com/foo/bar") {
		t.Error("expected package to be included when no patterns set")
	}
}

func TestShouldIncludePackage_CGOSkip(t *testing.T) {
	e := NewEngine(&EngineConfig{SkipCGOPackages: true})
	if e.shouldIncludePackage("github.com/mattn/go-sqlite3") {
		t.Error("expected CGO package to be excluded")
	}
}

func TestShouldIncludePackage_CGONotSkip(t *testing.T) {
	e := NewEngine(&EngineConfig{SkipCGOPackages: false})
	if !e.shouldIncludePackage("github.com/mattn/go-sqlite3") {
		t.Error("expected CGO package to be included when SkipCGOPackages=false")
	}
}

func TestShouldIncludePackage_AutoExcludeTests(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeTests: true})
	tests := []struct {
		pkg  string
		want bool
	}{
		{"github.com/foo/bar_test", false},
		{"github.com/foo/bar_tests", false},
		{"github.com/foo/bar", true},
	}
	for _, tt := range tests {
		got := e.shouldIncludePackage(tt.pkg)
		if got != tt.want {
			t.Errorf("shouldIncludePackage(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestShouldIncludePackage_AutoExcludeMocks(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeMocks: true})
	tests := []struct {
		pkg  string
		want bool
	}{
		{"github.com/foo/mock", false},
		{"github.com/foo/mocks", false},
		{"github.com/foo/fake", false},
		{"github.com/foo/fakes", false},
		{"github.com/foo/stub", false},
		{"github.com/foo/stubs", false},
		{"github.com/foo/service", true},
	}
	for _, tt := range tests {
		got := e.shouldIncludePackage(tt.pkg)
		if got != tt.want {
			t.Errorf("shouldIncludePackage(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestShouldIncludePackage_ExcludeTakesPrecedence(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludePackages: []string{"**/handler"},
		ExcludePackages: []string{"**/handler"},
	})
	if e.shouldIncludePackage("github.com/foo/handler") {
		t.Error("expected exclude to take precedence over include")
	}
}

func TestShouldIncludePackage_IncludeOnly(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludePackages: []string{"cmd"},
	})
	if !e.shouldIncludePackage("github.com/foo/cmd") {
		t.Error("expected package matching include pattern to be included")
	}
	if e.shouldIncludePackage("github.com/foo/internal") {
		t.Error("expected package not matching include pattern to be excluded")
	}
}

func TestShouldIncludePackage_ExcludeOnly(t *testing.T) {
	e := NewEngine(&EngineConfig{
		ExcludePackages: []string{"internal"},
	})
	if e.shouldIncludePackage("github.com/foo/internal") {
		t.Error("expected excluded package to be excluded")
	}
	if !e.shouldIncludePackage("github.com/foo/cmd") {
		t.Error("expected non-excluded package to be included")
	}
}

// The shouldIncludePackage function also considers IncludeFiles/ExcludeFiles
// to decide whether "no patterns specified" short-circuit fires.
func TestShouldIncludePackage_FileFiltersDoNotExcludePackage(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles: []string{"*.go"},
	})
	// File filters set, but no package patterns => include all packages
	if !e.shouldIncludePackage("github.com/foo/bar") {
		t.Error("expected package to be included when only file patterns set")
	}
}

// ---------------------------------------------------------------------------
// shouldIncludeFile tests
// ---------------------------------------------------------------------------

func TestShouldIncludeFile_NoPatterns(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	if !e.shouldIncludeFile("handler.go") {
		t.Error("expected file to be included with no patterns")
	}
}

func TestShouldIncludeFile_AutoExcludeTestFiles(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeTests: true})
	tests := []struct {
		file string
		want bool
	}{
		{"handler_test.go", false},
		{"internal/test/helper.go", false},
		{"internal/tests/helper.go", false},
		{"handler.go", true},
	}
	for _, tt := range tests {
		got := e.shouldIncludeFile(tt.file)
		if got != tt.want {
			t.Errorf("shouldIncludeFile(%q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestShouldIncludeFile_AutoExcludeMockFiles(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeMocks: true})
	tests := []struct {
		file string
		want bool
	}{
		{"service_mock.go", false},
		{"service_fake.go", false},
		{"service_stub.go", false},
		{"service_mocks.go", false},
		{"service_fakes.go", false},
		{"service_stubs.go", false},
		{"service.go", true},
	}
	for _, tt := range tests {
		got := e.shouldIncludeFile(tt.file)
		if got != tt.want {
			t.Errorf("shouldIncludeFile(%q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestShouldIncludeFile_ExcludePattern(t *testing.T) {
	e := NewEngine(&EngineConfig{
		ExcludeFiles: []string{"*_generated.go"},
	})
	if e.shouldIncludeFile("types_generated.go") {
		t.Error("expected generated file to be excluded")
	}
	if !e.shouldIncludeFile("types.go") {
		t.Error("expected normal file to be included")
	}
}

func TestShouldIncludeFile_IncludePattern(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles: []string{"handler*.go"},
	})
	if !e.shouldIncludeFile("handler.go") {
		t.Error("expected handler.go to be included")
	}
	if !e.shouldIncludeFile("handler_user.go") {
		t.Error("expected handler_user.go to be included")
	}
	if e.shouldIncludeFile("service.go") {
		t.Error("expected service.go to be excluded when include pattern set")
	}
}

func TestShouldIncludeFile_ExcludeTakesPrecedence(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles: []string{"*.go"},
		ExcludeFiles: []string{"secret.go"},
	})
	if e.shouldIncludeFile("secret.go") {
		t.Error("expected exclude to take precedence")
	}
	if !e.shouldIncludeFile("handler.go") {
		t.Error("expected handler.go to be included")
	}
}

func TestShouldIncludeFile_AutoExcludeDisabled(t *testing.T) {
	e := NewEngine(&EngineConfig{
		AutoExcludeTests: false,
		AutoExcludeMocks: false,
	})
	if !e.shouldIncludeFile("handler_test.go") {
		t.Error("expected test file to be included when auto-exclude disabled")
	}
	if !e.shouldIncludeFile("service_mock.go") {
		t.Error("expected mock file to be included when auto-exclude disabled")
	}
}

// ---------------------------------------------------------------------------
// ModuleRoot / GetMetadata / GetConfig tests
// ---------------------------------------------------------------------------

func TestModuleRoot(t *testing.T) {
	cfg := &EngineConfig{}
	cfg.moduleRoot = "/some/root"
	e := &Engine{config: cfg}
	if e.ModuleRoot() != "/some/root" {
		t.Errorf("expected /some/root, got %s", e.ModuleRoot())
	}
}

func TestModuleRoot_Empty(t *testing.T) {
	e := NewEngine(nil)
	if e.ModuleRoot() != "" {
		t.Errorf("expected empty module root, got %s", e.ModuleRoot())
	}
}

func TestGetMetadata_Nil(t *testing.T) {
	e := NewEngine(nil)
	if e.GetMetadata() != nil {
		t.Error("expected nil metadata for fresh engine")
	}
}

func TestGetMetadata_Set(t *testing.T) {
	e := NewEngine(nil)
	m := &metadata.Metadata{}
	e.metadata = m
	if e.GetMetadata() != m {
		t.Error("expected GetMetadata to return the set metadata")
	}
}

func TestGetConfig(t *testing.T) {
	cfg := &EngineConfig{Title: "My API"}
	e := NewEngine(cfg)
	got := e.GetConfig()
	if got == nil {
		t.Fatal("expected non-nil config")
	}
	if got.Title != "My API" {
		t.Errorf("expected title 'My API', got %q", got.Title)
	}
}

func TestGetConfig_NilInput(t *testing.T) {
	e := NewEngine(nil)
	got := e.GetConfig()
	if got == nil {
		t.Fatal("expected non-nil config even with nil input")
	}
	if got.Title != DefaultTitle {
		t.Errorf("expected default title %q, got %q", DefaultTitle, got.Title)
	}
}

// ---------------------------------------------------------------------------
// mergeIncludeExcludePatterns tests
// ---------------------------------------------------------------------------

func TestMergeIncludeExcludePatterns_AllFields(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles:     []string{"inc_file"},
		IncludePackages:  []string{"inc_pkg"},
		IncludeFunctions: []string{"inc_func"},
		IncludeTypes:     []string{"inc_type"},
		ExcludeFiles:     []string{"exc_file"},
		ExcludePackages:  []string{"exc_pkg"},
		ExcludeFunctions: []string{"exc_func"},
		ExcludeTypes:     []string{"exc_type"},
	})

	cfg := &spec.APISpecConfig{}
	e.mergeIncludeExcludePatterns(cfg)

	assertSliceContains(t, cfg.Include.Files, "inc_file")
	assertSliceContains(t, cfg.Include.Packages, "inc_pkg")
	assertSliceContains(t, cfg.Include.Functions, "inc_func")
	assertSliceContains(t, cfg.Include.Types, "inc_type")
	assertSliceContains(t, cfg.Exclude.Files, "exc_file")
	assertSliceContains(t, cfg.Exclude.Packages, "exc_pkg")
	assertSliceContains(t, cfg.Exclude.Functions, "exc_func")
	assertSliceContains(t, cfg.Exclude.Types, "exc_type")
}

func TestMergeIncludeExcludePatterns_Empty(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	cfg := &spec.APISpecConfig{
		Include: intspec.IncludeExclude{Files: []string{"existing"}},
	}
	e.mergeIncludeExcludePatterns(cfg)

	// Should not change existing values
	if len(cfg.Include.Files) != 1 || cfg.Include.Files[0] != "existing" {
		t.Errorf("expected existing include files to be preserved, got %v", cfg.Include.Files)
	}
}

func TestMergeIncludeExcludePatterns_Appends(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludeFiles: []string{"new_file"},
	})
	cfg := &spec.APISpecConfig{
		Include: intspec.IncludeExclude{Files: []string{"existing"}},
	}
	e.mergeIncludeExcludePatterns(cfg)

	if len(cfg.Include.Files) != 2 {
		t.Fatalf("expected 2 include files, got %d", len(cfg.Include.Files))
	}
	assertSliceContains(t, cfg.Include.Files, "existing")
	assertSliceContains(t, cfg.Include.Files, "new_file")
}

// ---------------------------------------------------------------------------
// filterValidPackages tests
// ---------------------------------------------------------------------------

func TestFilterValidPackages_AllValid(t *testing.T) {
	e := NewEngine(nil)
	logger := NewVerboseLogger(false)

	pkgs := []*packages.Package{
		{PkgPath: "github.com/foo/bar"},
		{PkgPath: "github.com/foo/baz"},
	}
	result, err := e.filterValidPackages(pkgs, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 packages, got %d", len(result))
	}
}

func TestFilterValidPackages_SomeErrors(t *testing.T) {
	e := NewEngine(nil)
	logger := NewVerboseLogger(false)

	pkgs := []*packages.Package{
		{PkgPath: "github.com/foo/good"},
		{
			PkgPath: "github.com/foo/bad",
			Errors:  []packages.Error{{Msg: "some error"}},
		},
	}
	result, err := e.filterValidPackages(pkgs, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 valid package, got %d", len(result))
	}
	if result[0].PkgPath != "github.com/foo/good" {
		t.Errorf("expected good package, got %s", result[0].PkgPath)
	}
}

func TestFilterValidPackages_AllErrors(t *testing.T) {
	e := NewEngine(nil)
	logger := NewVerboseLogger(false)

	pkgs := []*packages.Package{
		{
			PkgPath: "github.com/foo/bad1",
			Errors:  []packages.Error{{Msg: "error1"}},
		},
		{
			PkgPath: "github.com/foo/bad2",
			Errors:  []packages.Error{{Msg: "error2"}},
		},
	}
	_, err := e.filterValidPackages(pkgs, logger)
	if err == nil {
		t.Fatal("expected error when all packages have errors")
	}
	if !contains(err.Error(), "no valid packages found") {
		t.Errorf("expected 'no valid packages found' error, got: %s", err.Error())
	}
}

func TestFilterValidPackages_VerboseOutput(t *testing.T) {
	e := NewEngine(nil)
	logger := NewVerboseLogger(true)

	pkgs := []*packages.Package{
		{PkgPath: "github.com/foo/good"},
		{
			PkgPath: "github.com/foo/bad",
			Errors:  []packages.Error{{Msg: "broken"}},
		},
	}

	out := captureStdout(func() {
		_, _ = e.filterValidPackages(pkgs, logger)
	})

	if !contains(out, "Warning") {
		t.Errorf("expected verbose output to contain 'Warning', got %q", out)
	}
	if !contains(out, "broken") {
		t.Errorf("expected verbose output to contain error message 'broken', got %q", out)
	}
}

// ---------------------------------------------------------------------------
// isAutoExcludedPackage tests
// ---------------------------------------------------------------------------

func TestIsAutoExcludedPackage_Tests(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeTests: true, AutoExcludeMocks: false})
	tests := []struct {
		pkg  string
		want bool
	}{
		{"github.com/foo/pkg_test", true},
		{"github.com/foo/pkg_tests", true},
		{"github.com/foo/PKG_TEST", true}, // case insensitive
		{"github.com/foo/pkg", false},
	}
	for _, tt := range tests {
		got := e.isAutoExcludedPackage(tt.pkg)
		if got != tt.want {
			t.Errorf("isAutoExcludedPackage(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestIsAutoExcludedPackage_Mocks(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeTests: false, AutoExcludeMocks: true})
	tests := []struct {
		pkg  string
		want bool
	}{
		{"github.com/foo/mock", true},
		{"github.com/foo/mocks", true},
		{"github.com/foo/fake", true},
		{"github.com/foo/fakes", true},
		{"github.com/foo/stub", true},
		{"github.com/foo/stubs", true},
		{"github.com/foo/MOCK", true}, // case insensitive
		{"github.com/foo/service", false},
	}
	for _, tt := range tests {
		got := e.isAutoExcludedPackage(tt.pkg)
		if got != tt.want {
			t.Errorf("isAutoExcludedPackage(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestIsAutoExcludedPackage_BothDisabled(t *testing.T) {
	e := NewEngine(&EngineConfig{AutoExcludeTests: false, AutoExcludeMocks: false})
	if e.isAutoExcludedPackage("github.com/foo/mock") {
		t.Error("expected mock package to be included when auto-exclude disabled")
	}
	if e.isAutoExcludedPackage("github.com/foo/bar_test") {
		t.Error("expected test package to be included when auto-exclude disabled")
	}
}

// ---------------------------------------------------------------------------
// generateDiagram tests (without actually generating)
// ---------------------------------------------------------------------------

func TestGenerateDiagram_EmptyPath(t *testing.T) {
	e := NewEngine(&EngineConfig{DiagramPath: ""})
	err := e.generateDiagram(&metadata.Metadata{})
	if err != nil {
		t.Errorf("expected nil error for empty diagram path, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// autoIncludeFrameworkPackages tests
// ---------------------------------------------------------------------------

func TestAutoIncludeFrameworkPackages_Nil(_ *testing.T) {
	e := NewEngine(&EngineConfig{})
	logger := NewVerboseLogger(false)
	// Should not panic
	e.autoIncludeFrameworkPackages(nil, logger)
}

func TestAutoIncludeFrameworkPackages_EmptyList(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	logger := NewVerboseLogger(false)
	e.autoIncludeFrameworkPackages(&metadata.FrameworkDependencyList{}, logger)
	if len(e.config.IncludePackages) != 0 {
		t.Errorf("expected no include packages, got %v", e.config.IncludePackages)
	}
}

func TestAutoIncludeFrameworkPackages_AddsPackages(t *testing.T) {
	e := NewEngine(&EngineConfig{})
	logger := NewVerboseLogger(false)
	list := &metadata.FrameworkDependencyList{
		AllPackages: []*metadata.FrameworkDependency{
			{PackagePath: "github.com/foo/handler", FrameworkType: "gin", IsDirect: true},
			{PackagePath: "github.com/foo/routes", FrameworkType: "gin", IsDirect: false},
		},
	}
	e.autoIncludeFrameworkPackages(list, logger)
	if len(e.config.IncludePackages) != 2 {
		t.Fatalf("expected 2 include packages, got %d", len(e.config.IncludePackages))
	}
	assertSliceContains(t, e.config.IncludePackages, "github.com/foo/handler")
	assertSliceContains(t, e.config.IncludePackages, "github.com/foo/routes")
}

func TestAutoIncludeFrameworkPackages_NoDuplicates(t *testing.T) {
	e := NewEngine(&EngineConfig{
		IncludePackages: []string{"github.com/foo/handler"},
	})
	logger := NewVerboseLogger(false)
	list := &metadata.FrameworkDependencyList{
		AllPackages: []*metadata.FrameworkDependency{
			{PackagePath: "github.com/foo/handler", FrameworkType: "gin", IsDirect: true},
			{PackagePath: "github.com/foo/new", FrameworkType: "gin", IsDirect: true},
		},
	}
	e.autoIncludeFrameworkPackages(list, logger)
	// Should only add the new one
	if len(e.config.IncludePackages) != 2 {
		t.Errorf("expected 2 include packages (1 existing + 1 new), got %d: %v",
			len(e.config.IncludePackages), e.config.IncludePackages)
	}
}

// ---------------------------------------------------------------------------
// mergeConfigWithDefaults tests
// ---------------------------------------------------------------------------

func TestMergeConfigWithDefaults_EmptyUser(t *testing.T) {
	user := &spec.APISpecConfig{}
	defaults := &spec.APISpecConfig{
		Defaults: intspec.Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "text/plain",
			ResponseStatus:      200,
		},
	}
	defaults.Framework.RoutePatterns = []intspec.RoutePattern{{BasePattern: intspec.BasePattern{CallRegex: "test"}}}
	defaults.Framework.RequestBodyPatterns = []intspec.RequestBodyPattern{{BasePattern: intspec.BasePattern{CallRegex: "rb"}}}
	defaults.Framework.ResponsePatterns = []intspec.ResponsePattern{{BasePattern: intspec.BasePattern{CallRegex: "rp"}}}
	defaults.Framework.ParamPatterns = []intspec.ParamPattern{{BasePattern: intspec.BasePattern{CallRegex: "pp"}}}
	defaults.Framework.MountPatterns = []intspec.MountPattern{{BasePattern: intspec.BasePattern{CallRegex: "mp"}}}
	defaults.Framework.ContentTypePatterns = []intspec.ContentTypePattern{{BasePattern: intspec.BasePattern{CallRegex: "ct"}}}

	mergeConfigWithDefaults(user, defaults)

	if len(user.Framework.RoutePatterns) != 1 {
		t.Errorf("expected route patterns from defaults, got %d", len(user.Framework.RoutePatterns))
	}
	if len(user.Framework.RequestBodyPatterns) != 1 {
		t.Errorf("expected request body patterns from defaults")
	}
	if len(user.Framework.ResponsePatterns) != 1 {
		t.Errorf("expected response patterns from defaults")
	}
	if len(user.Framework.ParamPatterns) != 1 {
		t.Errorf("expected param patterns from defaults")
	}
	if len(user.Framework.MountPatterns) != 1 {
		t.Errorf("expected mount patterns from defaults")
	}
	if len(user.Framework.ContentTypePatterns) != 1 {
		t.Errorf("expected content type patterns from defaults")
	}
	if user.Defaults.RequestContentType != "application/json" {
		t.Errorf("expected request content type from defaults")
	}
	if user.Defaults.ResponseContentType != "text/plain" {
		t.Errorf("expected response content type from defaults")
	}
	if user.Defaults.ResponseStatus != 200 {
		t.Errorf("expected response status from defaults")
	}
}

func TestMergeConfigWithDefaults_UserOverrides(t *testing.T) {
	user := &spec.APISpecConfig{
		Defaults: intspec.Defaults{
			RequestContentType:  "text/xml",
			ResponseContentType: "text/html",
			ResponseStatus:      201,
		},
	}
	user.Framework.RoutePatterns = []intspec.RoutePattern{{BasePattern: intspec.BasePattern{CallRegex: "user_route"}}}

	defaults := &spec.APISpecConfig{
		Defaults: intspec.Defaults{
			RequestContentType:  "application/json",
			ResponseContentType: "text/plain",
			ResponseStatus:      200,
		},
	}
	defaults.Framework.RoutePatterns = []intspec.RoutePattern{{BasePattern: intspec.BasePattern{CallRegex: "default_route"}}}

	mergeConfigWithDefaults(user, defaults)

	// User values should be preserved
	if user.Defaults.RequestContentType != "text/xml" {
		t.Errorf("expected user request content type to be preserved")
	}
	if user.Defaults.ResponseContentType != "text/html" {
		t.Errorf("expected user response content type to be preserved")
	}
	if user.Defaults.ResponseStatus != 201 {
		t.Errorf("expected user response status to be preserved")
	}
	if user.Framework.RoutePatterns[0].CallRegex != "user_route" {
		t.Errorf("expected user route patterns to be preserved")
	}
}

// ---------------------------------------------------------------------------
// applyConfigDefaults tests
// ---------------------------------------------------------------------------

func TestApplyConfigDefaults(t *testing.T) {
	e := NewEngine(&EngineConfig{
		Title:          "Test API",
		Description:    "A test",
		APIVersion:     "3.0.0",
		TermsOfService: "https://example.com/tos",
		ContactName:    "Tester",
		ContactURL:     "https://example.com",
		ContactEmail:   "test@example.com",
		LicenseName:    "MIT",
		LicenseURL:     "https://opensource.org/licenses/MIT",
		ShortNames:     true,
		ShortNamesSet:  true,
	})

	cfg := &spec.APISpecConfig{}
	e.applyConfigDefaults(cfg)

	if cfg.Info.Title != "Test API" {
		t.Errorf("expected title 'Test API', got %q", cfg.Info.Title)
	}
	if cfg.Info.Description != "A test" {
		t.Errorf("expected description 'A test', got %q", cfg.Info.Description)
	}
	if cfg.Info.Version != "3.0.0" {
		t.Errorf("expected version '3.0.0', got %q", cfg.Info.Version)
	}
	if cfg.Info.TermsOfService != "https://example.com/tos" {
		t.Errorf("expected terms of service")
	}
	if cfg.Info.Contact == nil {
		t.Fatal("expected contact to be set")
	}
	if cfg.Info.Contact.Name != "Tester" {
		t.Errorf("expected contact name 'Tester', got %q", cfg.Info.Contact.Name)
	}
	if cfg.Info.License == nil {
		t.Fatal("expected license to be set")
	}
	if cfg.Info.License.Name != "MIT" {
		t.Errorf("expected license name 'MIT', got %q", cfg.Info.License.Name)
	}
	if cfg.ShortNames == nil || !*cfg.ShortNames {
		t.Error("expected ShortNames to be true")
	}
}

func TestApplyConfigDefaults_DoesNotOverride(t *testing.T) {
	e := NewEngine(&EngineConfig{
		Title:       "Engine Title",
		Description: "Engine Desc",
	})

	existingContact := &intspec.Contact{Name: "Existing"}
	existingLicense := &intspec.License{Name: "Existing License"}
	cfg := &spec.APISpecConfig{
		Info: intspec.Info{
			Title:       "Existing Title",
			Description: "Existing Desc",
			Version:     "9.9.9",
			Contact:     existingContact,
			License:     existingLicense,
		},
	}
	e.applyConfigDefaults(cfg)

	if cfg.Info.Title != "Existing Title" {
		t.Errorf("expected existing title to be preserved, got %q", cfg.Info.Title)
	}
	if cfg.Info.Description != "Existing Desc" {
		t.Errorf("expected existing description to be preserved")
	}
	if cfg.Info.Version != "9.9.9" {
		t.Errorf("expected existing version to be preserved")
	}
	if cfg.Info.Contact != existingContact {
		t.Error("expected existing contact to be preserved")
	}
	if cfg.Info.License != existingLicense {
		t.Error("expected existing license to be preserved")
	}
}

func TestApplyConfigDefaults_ShortNamesNotSet(t *testing.T) {
	e := NewEngine(&EngineConfig{
		ShortNamesSet: false,
	})
	cfg := &spec.APISpecConfig{}
	e.applyConfigDefaults(cfg)
	if cfg.ShortNames != nil {
		t.Error("expected ShortNames to remain nil when ShortNamesSet=false")
	}
}

// ---------------------------------------------------------------------------
// NewEngine defaults merging tests
// ---------------------------------------------------------------------------

func TestNewEngine_DefaultsMerging(t *testing.T) {
	// When fields are empty, they should get defaults
	e := NewEngine(&EngineConfig{})
	c := e.GetConfig()

	if c.InputDir != DefaultInputDir {
		t.Errorf("expected default InputDir")
	}
	if c.OutputFile != DefaultOutputFile {
		t.Errorf("expected default OutputFile")
	}
	if c.Title != DefaultTitle {
		t.Errorf("expected default Title")
	}
	if c.APIVersion != DefaultAPIVersion {
		t.Errorf("expected default APIVersion")
	}
	if c.ContactName != DefaultContactName {
		t.Errorf("expected default ContactName")
	}
	if c.ContactURL != DefaultContactURL {
		t.Errorf("expected default ContactURL")
	}
	if c.ContactEmail != DefaultContactEmail {
		t.Errorf("expected default ContactEmail")
	}
	if c.OpenAPIVersion != DefaultOpenAPIVersion {
		t.Errorf("expected default OpenAPIVersion")
	}
	if c.MaxNodesPerTree != DefaultMaxNodesPerTree {
		t.Errorf("expected default MaxNodesPerTree")
	}
	if c.MaxChildrenPerNode != DefaultMaxChildrenPerNode {
		t.Errorf("expected default MaxChildrenPerNode")
	}
	if c.MaxArgsPerFunction != DefaultMaxArgsPerFunction {
		t.Errorf("expected default MaxArgsPerFunction")
	}
	if c.MaxNestedArgsDepth != DefaultMaxNestedArgsDepth {
		t.Errorf("expected default MaxNestedArgsDepth")
	}
}

// ---------------------------------------------------------------------------
// filterToFrameworkPackages tests
// ---------------------------------------------------------------------------

func TestFilterToFrameworkPackages_EmptyInput(t *testing.T) {
	e := NewEngine(nil)
	pkgsMeta := make(map[string]map[string]*ast.File)
	fileToInfo := make(map[*ast.File]*types.Info)
	importPaths := make(map[string]string)
	list := &metadata.FrameworkDependencyList{}

	rPkgs, rFiles, rImports := e.filterToFrameworkPackages(pkgsMeta, fileToInfo, importPaths, list)
	if len(rPkgs) != 0 || len(rFiles) != 0 || len(rImports) != 0 {
		t.Error("expected empty results for empty input")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertSliceContains(t *testing.T, slice []string, val string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("expected slice to contain %q, got %v", val, slice)
}
