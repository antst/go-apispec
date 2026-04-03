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
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/antst/go-apispec/internal/profiler"
)

// ---------------------------------------------------------------------------
// stringSliceFlag.Set
// ---------------------------------------------------------------------------

func TestStringSliceFlag_Set(t *testing.T) {
	var s stringSliceFlag

	if err := s.Set("alpha"); err != nil {
		t.Fatalf("Set(alpha) returned error: %v", err)
	}
	if err := s.Set("beta"); err != nil {
		t.Fatalf("Set(beta) returned error: %v", err)
	}

	if len(s) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(s))
	}
	if s[0] != "alpha" || s[1] != "beta" {
		t.Errorf("unexpected values: %v", s)
	}

	got := s.String()
	if got != "alpha,beta" {
		t.Errorf("String() = %q, want %q", got, "alpha,beta")
	}
}

func TestStringSliceFlag_SetEmpty(t *testing.T) {
	var s stringSliceFlag

	if err := s.Set(""); err != nil {
		t.Fatalf("Set('') returned error: %v", err)
	}
	if len(s) != 1 {
		t.Fatalf("expected 1 element, got %d", len(s))
	}
}

// ---------------------------------------------------------------------------
// extractVCSInfo
// ---------------------------------------------------------------------------

func TestExtractVCSInfo_Full(t *testing.T) {
	// Save originals and restore after test
	origCommit, origBuildDate := Commit, BuildDate
	defer func() { Commit, BuildDate = origCommit, origBuildDate }()

	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abcdef1234567890"},
		{Key: "vcs.time", Value: "2026-01-15T10:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}

	hasVCS, isModified := extractVCSInfo(settings)
	if !hasVCS {
		t.Error("expected hasVCSInfo=true")
	}
	if !isModified {
		t.Error("expected isModified=true")
	}
	if Commit != "abcdef1" {
		t.Errorf("Commit = %q, want %q", Commit, "abcdef1")
	}
	if BuildDate != "2026-01-15T10:00:00Z" {
		t.Errorf("BuildDate = %q, want %q", BuildDate, "2026-01-15T10:00:00Z")
	}
}

func TestExtractVCSInfo_ShortRevision(t *testing.T) {
	origCommit := Commit
	defer func() { Commit = origCommit }()

	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc"},
	}

	hasVCS, isModified := extractVCSInfo(settings)
	if !hasVCS {
		t.Error("expected hasVCSInfo=true")
	}
	if isModified {
		t.Error("expected isModified=false for missing vcs.modified")
	}
	if Commit != "abc" {
		t.Errorf("Commit = %q, want %q", Commit, "abc")
	}
}

func TestExtractVCSInfo_NotModified(t *testing.T) {
	settings := []debug.BuildSetting{
		{Key: "vcs.modified", Value: "false"},
	}

	_, isModified := extractVCSInfo(settings)
	if isModified {
		t.Error("expected isModified=false when vcs.modified=false")
	}
}

func TestExtractVCSInfo_Empty(t *testing.T) {
	hasVCS, isModified := extractVCSInfo(nil)
	if hasVCS {
		t.Error("expected hasVCSInfo=false for nil settings")
	}
	if isModified {
		t.Error("expected isModified=false for nil settings")
	}
}

func TestExtractVCSInfo_UnrelatedKeys(t *testing.T) {
	settings := []debug.BuildSetting{
		{Key: "GOARCH", Value: "amd64"},
		{Key: "GOOS", Value: "linux"},
	}
	hasVCS, isModified := extractVCSInfo(settings)
	if hasVCS {
		t.Error("expected hasVCSInfo=false for non-VCS settings")
	}
	if isModified {
		t.Error("expected isModified=false for non-VCS settings")
	}
}

// ---------------------------------------------------------------------------
// runGenerationWithProfiling — nil profiler path
// ---------------------------------------------------------------------------

func TestRunGenerationWithProfiling_NilProfiler(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:   tempDir,
		OutputFile: "openapi.json",
		Title:      "Test API",
		APIVersion: "1.0.0",
	}

	spec, eng, err := runGenerationWithProfiling(config, nil)
	if err != nil {
		t.Fatalf("runGenerationWithProfiling with nil profiler failed: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestRunGenerationWithProfiling_ProfilerNoMetrics(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:   tempDir,
		OutputFile: "openapi.json",
		Title:      "Test API",
		APIVersion: "1.0.0",
	}

	// Create a profiler without enabling custom metrics (GetMetrics() returns nil)
	prof := profiler.NewProfiler(&profiler.ProfilerConfig{})

	spec, eng, err := runGenerationWithProfiling(config, prof)
	if err != nil {
		t.Fatalf("runGenerationWithProfiling with nil metrics failed: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
}

// ---------------------------------------------------------------------------
// generatePerformanceAnalysis
// ---------------------------------------------------------------------------

func TestGeneratePerformanceAnalysis_NilMetrics(t *testing.T) {
	// Profiler without custom metrics enabled => GetMetrics() == nil
	prof := profiler.NewProfiler(&profiler.ProfilerConfig{})

	config := &CLIConfig{
		ProfileOutputDir:   t.TempDir(),
		ProfileMetricsPath: "metrics.json",
	}

	err := generatePerformanceAnalysis(prof, config)
	if err != nil {
		t.Fatalf("expected nil error for nil metrics, got: %v", err)
	}
}

func TestGeneratePerformanceAnalysis_WithMetrics(t *testing.T) {
	tempDir := t.TempDir()

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     tempDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	// Add a metric so there is something to analyze
	mc := prof.GetMetrics()
	if mc == nil {
		t.Fatal("expected non-nil metrics collector after Start()")
	}
	mc.SetGauge("test.gauge", 42.0, "count", map[string]string{"test": "true"})

	config := &CLIConfig{
		ProfileOutputDir:   tempDir,
		ProfileMetricsPath: "metrics.json",
		Verbose:            true,
	}

	err := generatePerformanceAnalysis(prof, config)
	if err != nil {
		t.Fatalf("generatePerformanceAnalysis failed: %v", err)
	}

	// Verify the metrics file was written
	metricsPath := filepath.Join(tempDir, "metrics.json")
	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		t.Error("expected metrics file to be created")
	}
}

func TestGeneratePerformanceAnalysis_NonVerbose(t *testing.T) {
	tempDir := t.TempDir()

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     tempDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	config := &CLIConfig{
		ProfileOutputDir:   tempDir,
		ProfileMetricsPath: "metrics.json",
		Verbose:            false,
	}

	err := generatePerformanceAnalysis(prof, config)
	if err != nil {
		t.Fatalf("generatePerformanceAnalysis non-verbose failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeOutput — JSON to file
// ---------------------------------------------------------------------------

func TestWriteOutput_JSONToFile(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    filepath.Join(tempDir, "output.json"),
		OutputFlagSet: true,
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err != nil {
		t.Fatalf("writeOutput JSON failed: %v", err)
	}

	// Verify file exists and contains valid JSON
	data, err := os.ReadFile(filepath.Join(tempDir, "output.json"))
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := parsed["openapi"]; !ok {
		t.Error("JSON output missing 'openapi' key")
	}
}

func TestWriteOutput_YAMLToFile(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    filepath.Join(tempDir, "output.yaml"),
		OutputFlagSet: true,
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err != nil {
		t.Fatalf("writeOutput YAML failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "output.yaml"))
	if err != nil {
		t.Fatalf("failed to read YAML output file: %v", err)
	}
	if !strings.Contains(string(data), "openapi:") {
		t.Error("YAML output missing 'openapi:' key")
	}
}

func TestWriteOutput_YMLExtension(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    filepath.Join(tempDir, "output.yml"),
		OutputFlagSet: true,
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err != nil {
		t.Fatalf("writeOutput YML failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "output.yml"))
	if err != nil {
		t.Fatalf("failed to read .yml output file: %v", err)
	}
	if !strings.Contains(string(data), "openapi:") {
		t.Error("YML output missing 'openapi:' key")
	}
}

func TestWriteOutput_StdoutJSON(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    "openapi.json", // default — triggers stdout path
		OutputFlagSet: false,
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = writeOutput(spec, config, eng)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("writeOutput stdout JSON failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Should be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("stdout output is not valid JSON: %v\nOutput:\n%s", err, output[:min(200, len(output))])
	}
}

func TestWriteOutput_BadPath(t *testing.T) {
	tempDir := createTestGoProject(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    filepath.Join("/nonexistent_dir_xyz", "sub", "output.json"),
		OutputFlagSet: true,
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err == nil {
		t.Fatal("expected error writing to non-existent directory")
	}
	if !strings.Contains(err.Error(), "failed to create output file") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// run — minimal end-to-end
// ---------------------------------------------------------------------------

func TestRun_MinimalProject(t *testing.T) {
	tempDir := createTestGoProject(t)
	outputFile := filepath.Join(tempDir, "spec.json")

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    outputFile,
		OutputFlagSet: true,
		Title:         "Run Test API",
		APIVersion:    "1.0.0",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// Verify output was written
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if len(data) == 0 {
		t.Error("output file is empty")
	}
}

func TestRun_WithCustomMetrics(t *testing.T) {
	tempDir := createTestGoProject(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Profiled API",
		APIVersion:         "1.0.0",
		CustomMetrics:      true,
		ProfileOutputDir:   profileDir,
		ProfileMetricsPath: "metrics.json",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with custom metrics failed: %v", err)
	}

	// Verify metrics file was created
	metricsPath := filepath.Join(profileDir, "metrics.json")
	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		t.Error("expected metrics.json to be created")
	}
}

func TestRun_InvalidDir(t *testing.T) {
	config := &CLIConfig{
		InputDir:   "/nonexistent_directory_xyz",
		OutputFile: "openapi.json",
		Title:      "Test",
		APIVersion: "1.0.0",
	}

	err := run(config)
	if err == nil {
		t.Fatal("expected run to fail for nonexistent directory")
	}
}

func TestToSortedYAML(t *testing.T) {
	data := map[string]interface{}{
		"z": "last",
		"a": "first",
		"m": []string{"one", "two"},
	}
	node, err := toSortedYAML(data)
	if err != nil {
		t.Fatalf("toSortedYAML: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	// Verify keys are sorted in document content
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		t.Fatal("expected document node with content")
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("expected mapping node")
	}
	if mapping.Content[0].Value != "a" {
		t.Errorf("expected first key 'a', got %q", mapping.Content[0].Value)
	}
}

func TestSortYAMLNode_NilNode(_ *testing.T) {
	sortYAMLNode(nil) // should not panic
}

func TestSortYAMLNode_SequenceNode(t *testing.T) {
	// Sequence nodes should recurse into children
	node := &yaml.Node{
		Kind: yaml.SequenceNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "b"},
			{Kind: yaml.ScalarNode, Value: "a"},
		},
	}
	sortYAMLNode(node)
	// Sequence order should be preserved (not sorted)
	if node.Content[0].Value != "b" {
		t.Errorf("sequence should preserve order, got %q first", node.Content[0].Value)
	}
}

func TestDetectVersionInfo_Defaults(t *testing.T) {
	// Reset to defaults
	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	detectVersionInfo()

	// After detection, at least GoVersion should be set if built with Go
	if GoVersion == "unknown" {
		t.Log("GoVersion still unknown — likely running in unusual build env")
	}
}

func TestWriteOutput_StdoutDefaultJSON(t *testing.T) {
	spec := map[string]interface{}{"openapi": "3.1.1"}
	config := &CLIConfig{
		OutputFile:    "openapi.json", // matches DefaultOutputFile
		OutputFlagSet: false,          // not explicitly set → stdout
	}
	// Capture stdout output — just verify it doesn't error
	err := writeOutput(spec, config, nil)
	if err != nil {
		t.Fatalf("writeOutput stdout json: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// createTestGoProject creates a minimal Go project in a temp dir and returns its path.
func createTestGoProject(t *testing.T) string {
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
