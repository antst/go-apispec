package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/antst/go-apispec/internal/engine"
	"github.com/antst/go-apispec/internal/profiler"
)

// ---------------------------------------------------------------------------
// toSortedYAML — error paths
// ---------------------------------------------------------------------------

// badYAMLMarshaler triggers an error in yaml.Marshal via the Marshaler interface.
type badYAMLMarshaler struct{}

func (b badYAMLMarshaler) MarshalYAML() (interface{}, error) {
	return nil, os.ErrInvalid
}

func TestToSortedYAML_MarshalError(t *testing.T) {
	_, err := toSortedYAML(badYAMLMarshaler{})
	if err == nil {
		t.Fatal("expected error from marshaling")
	}
}

func TestToSortedYAML_NestedMaps(t *testing.T) {
	data := map[string]interface{}{
		"z": map[string]interface{}{
			"z_inner": "last",
			"a_inner": "first",
		},
		"a": "top",
		"m": map[string]interface{}{
			"b": "second",
			"a": "first",
		},
	}

	node, err := toSortedYAML(data)
	if err != nil {
		t.Fatalf("toSortedYAML failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		t.Fatal("expected document node with content")
	}

	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("expected mapping node")
	}
	// Top-level keys should be sorted: a, m, z
	if mapping.Content[0].Value != "a" {
		t.Errorf("expected first key 'a', got %q", mapping.Content[0].Value)
	}
	if mapping.Content[2].Value != "m" {
		t.Errorf("expected second key 'm', got %q", mapping.Content[2].Value)
	}
	if mapping.Content[4].Value != "z" {
		t.Errorf("expected third key 'z', got %q", mapping.Content[4].Value)
	}

	// Verify nested map "m" has sorted keys: a, b
	mValue := mapping.Content[3] // value node for key "m"
	if mValue.Kind != yaml.MappingNode {
		t.Fatal("expected 'm' value to be a mapping node")
	}
	if mValue.Content[0].Value != "a" {
		t.Errorf("expected nested key 'a' first in 'm', got %q", mValue.Content[0].Value)
	}
	if mValue.Content[2].Value != "b" {
		t.Errorf("expected nested key 'b' second in 'm', got %q", mValue.Content[2].Value)
	}

	// Verify nested map "z" has sorted keys: a_inner, z_inner
	zValue := mapping.Content[5] // value node for key "z"
	if zValue.Kind != yaml.MappingNode {
		t.Fatal("expected 'z' value to be a mapping node")
	}
	if zValue.Content[0].Value != "a_inner" {
		t.Errorf("expected nested key 'a_inner' first in 'z', got %q", zValue.Content[0].Value)
	}
	if zValue.Content[2].Value != "z_inner" {
		t.Errorf("expected nested key 'z_inner' second in 'z', got %q", zValue.Content[2].Value)
	}
}

func TestToSortedYAML_EmptyMap(t *testing.T) {
	data := map[string]interface{}{}
	node, err := toSortedYAML(data)
	if err != nil {
		t.Fatalf("toSortedYAML failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
}

func TestToSortedYAML_SimpleScalar(t *testing.T) {
	data := "hello"
	node, err := toSortedYAML(data)
	if err != nil {
		t.Fatalf("toSortedYAML failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
}

// ---------------------------------------------------------------------------
// writeOutput — error paths and additional output modes
// ---------------------------------------------------------------------------

func TestWriteOutput_FileYAML(t *testing.T) {
	// Test the YAML file output path by setting OutputFile to a .yaml extension
	// with OutputFlagSet=true to trigger the file-write branch.
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "openapi.yaml")

	spec := map[string]interface{}{"openapi": "3.1.1"}
	config := &CLIConfig{
		OutputFile:    outputPath,
		OutputFlagSet: true,
	}

	err := writeOutput(spec, config, nil)
	if err != nil {
		t.Fatalf("writeOutput file YAML failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read YAML output: %v", err)
	}
	output := string(data)
	if !strings.Contains(output, "openapi") {
		t.Error("expected output to contain 'openapi'")
	}

	// Verify output is valid YAML, not JSON
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
	if parsed["openapi"] != "3.1.1" {
		t.Errorf("expected openapi 3.1.1, got %v", parsed["openapi"])
	}
}

func TestWriteOutput_FileYAMLSorted(t *testing.T) {
	tempDir := createTestGoProject2(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    filepath.Join(tempDir, "sorted.yaml"),
		OutputFlagSet: true,
		Title:         "Sorted API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err != nil {
		t.Fatalf("writeOutput YAML sorted failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "sorted.yaml"))
	if err != nil {
		t.Fatalf("failed to read YAML output: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "openapi:") {
		t.Error("YAML output missing 'openapi:' key")
	}
}

func TestWriteOutput_FileJSONWriteError(t *testing.T) {
	// Use a read-only file to cause write error
	tempDir := createTestGoProject2(t)

	outputPath := filepath.Join(tempDir, "readonly.json")
	// Create the file first, then make it read-only
	if err := os.WriteFile(outputPath, []byte(""), 0444); err != nil {
		t.Fatalf("failed to create read-only file: %v", err)
	}

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    outputPath,
		OutputFlagSet: true,
		Title:         "Test",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	err = writeOutput(spec, config, eng)
	if err == nil {
		t.Fatal("expected error writing to read-only file, but got nil")
	}
}

// ---------------------------------------------------------------------------
// detectVersionInfo — additional branches
// ---------------------------------------------------------------------------

func TestDetectVersionInfo_VersionAlreadySet(t *testing.T) {
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

	Version = "2.5.0"
	Commit = "abc1234"
	BuildDate = "2026-01-01"
	GoVersion = "go1.24"

	detectVersionInfo()

	if Version != "2.5.0" {
		t.Errorf("expected Version to remain 2.5.0, got %s", Version)
	}
	if Commit != "abc1234" {
		t.Errorf("expected Commit to remain abc1234, got %s", Commit)
	}
}

func TestDetectVersionInfo_RuntimePath(t *testing.T) {
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

	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	detectVersionInfo()

	// GoVersion should be set from runtime (debug.ReadBuildInfo succeeds in tests)
	if GoVersion == "unknown" {
		t.Error("expected GoVersion to be set after detectVersionInfo(), got 'unknown'")
	}
	// Version may stay "0.0.1" in CI builds without VCS data — skip rather than fail
	if Version == "0.0.1" {
		t.Skip("Version still 0.0.1 — build info lacks VCS data (expected in some CI environments)")
	}
}

func TestDetectVersionInfo_DevelVersion(t *testing.T) {
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

	// Set Version to "0.0.1" so detectVersionInfo() doesn't early-return.
	// This exercises the full path including ReadBuildInfo and fallbacks.
	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	detectVersionInfo()

	// After detectVersionInfo, Version should be updated from "0.0.1"
	// (either to "dev", "latest (go install)", or "unknown (go install)")
	if Version == "0.0.1" {
		t.Error("expected Version to be updated from '0.0.1' after detectVersionInfo()")
	}
	// GoVersion should be populated from runtime
	if GoVersion == "unknown" {
		t.Skip("GoVersion still 'unknown' — debug.ReadBuildInfo may not provide it in this environment")
	}
}

// ---------------------------------------------------------------------------
// run — profiling paths
// ---------------------------------------------------------------------------

func TestRun_WithCPUProfile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:         tempDir,
		OutputFile:       outputFile,
		OutputFlagSet:    true,
		Title:            "CPU Profile Test",
		APIVersion:       "1.0.0",
		CPUProfile:       true,
		ProfileOutputDir: profileDir,
		ProfileCPUPath:   "cpu.prof",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with CPU profile failed: %v", err)
	}

	// Verify CPU profile was created
	cpuPath := filepath.Join(profileDir, "cpu.prof")
	if _, err := os.Stat(cpuPath); os.IsNotExist(err) {
		t.Error("expected cpu.prof to be created")
	}
}

func TestRun_WithMemProfile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:         tempDir,
		OutputFile:       outputFile,
		OutputFlagSet:    true,
		Title:            "Mem Profile Test",
		APIVersion:       "1.0.0",
		MemProfile:       true,
		ProfileOutputDir: profileDir,
		ProfileMemPath:   "mem.prof",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with mem profile failed: %v", err)
	}

	memPath := filepath.Join(profileDir, "mem.prof")
	if _, err := os.Stat(memPath); os.IsNotExist(err) {
		t.Error("expected mem.prof to be created")
	}
}

func TestRun_WithBlockProfile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Block Profile Test",
		APIVersion:         "1.0.0",
		BlockProfile:       true,
		ProfileOutputDir:   profileDir,
		ProfileBlockPath:   "block.prof",
		ProfileMetricsPath: "metrics.json",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with block profile failed: %v", err)
	}

	// Verify block profile was created
	blockPath := filepath.Join(profileDir, "block.prof")
	if _, err := os.Stat(blockPath); os.IsNotExist(err) {
		t.Error("expected block.prof to be created")
	}
}

func TestRun_WithMutexProfile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Mutex Profile Test",
		APIVersion:         "1.0.0",
		MutexProfile:       true,
		ProfileOutputDir:   profileDir,
		ProfileMutexPath:   "mutex.prof",
		ProfileMetricsPath: "metrics.json",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with mutex profile failed: %v", err)
	}

	// Verify mutex profile was created
	mutexPath := filepath.Join(profileDir, "mutex.prof")
	if _, err := os.Stat(mutexPath); os.IsNotExist(err) {
		t.Error("expected mutex.prof to be created")
	}
}

func TestRun_WithTraceProfile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Trace Profile Test",
		APIVersion:         "1.0.0",
		TraceProfile:       true,
		ProfileOutputDir:   profileDir,
		ProfileTracePath:   "trace.out",
		ProfileMetricsPath: "metrics.json",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with trace profile failed: %v", err)
	}

	// Verify trace profile was created
	tracePath := filepath.Join(profileDir, "trace.out")
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		t.Error("expected trace.out to be created")
	}
}

func TestRun_WithAllProfiles(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "All Profiles Test",
		APIVersion:         "1.0.0",
		CPUProfile:         true,
		MemProfile:         true,
		BlockProfile:       true,
		MutexProfile:       true,
		TraceProfile:       true,
		CustomMetrics:      true,
		ProfileOutputDir:   profileDir,
		ProfileCPUPath:     "cpu.prof",
		ProfileMemPath:     "mem.prof",
		ProfileBlockPath:   "block.prof",
		ProfileMutexPath:   "mutex.prof",
		ProfileTracePath:   "trace.out",
		ProfileMetricsPath: "metrics.json",
		Verbose:            true,
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with all profiles failed: %v", err)
	}

	// Verify profile files
	for _, fname := range []string{"cpu.prof", "mem.prof", "block.prof", "mutex.prof", "trace.out", "metrics.json"} {
		fpath := filepath.Join(profileDir, fname)
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			t.Errorf("expected %s to be created", fname)
		}
	}
}

func TestRun_ProfilingStartError(t *testing.T) {
	tempDir := createTestGoProject2(t)

	config := &CLIConfig{
		InputDir:         tempDir,
		OutputFile:       filepath.Join(tempDir, "spec.json"),
		OutputFlagSet:    true,
		Title:            "Test",
		APIVersion:       "1.0.0",
		CPUProfile:       true,
		ProfileOutputDir: "/nonexistent_root_xyz/profiles",
		ProfileCPUPath:   "cpu.prof",
	}

	err := run(config)
	if err == nil {
		t.Fatal("expected error when profiling start fails")
	}
	if !strings.Contains(err.Error(), "failed to start profiling") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_GenerationError(t *testing.T) {
	config := &CLIConfig{
		InputDir:      "/nonexistent_directory_xyz",
		OutputFile:    "openapi.json",
		OutputFlagSet: true,
		Title:         "Test",
		APIVersion:    "1.0.0",
	}

	err := run(config)
	if err == nil {
		t.Fatal("expected error when generation fails")
	}
}

// ---------------------------------------------------------------------------
// runGenerationWithProfiling — with active metrics
// ---------------------------------------------------------------------------

func TestRunGenerationWithProfiling_WithActiveMetrics(t *testing.T) {
	tempDir := createTestGoProject2(t)
	profileDir := filepath.Join(tempDir, "profiles")

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     profileDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	config := &CLIConfig{
		InputDir:   tempDir,
		OutputFile: "openapi.json",
		Title:      "Profiled API",
		APIVersion: "1.0.0",
	}

	spec, eng, err := runGenerationWithProfiling(config, prof)
	if err != nil {
		t.Fatalf("runGenerationWithProfiling failed: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}

	// Check that the success gauge was recorded
	mc := prof.GetMetrics()
	if mc == nil {
		t.Fatal("expected non-nil metrics collector")
	}
	metrics := mc.GetMetrics()
	foundSuccessGauge := false
	for _, m := range metrics {
		if m.Name == "generation.success" {
			foundSuccessGauge = true
			break
		}
	}
	if !foundSuccessGauge {
		t.Error("expected generation.success gauge to be recorded")
	}
}

func TestRunGenerationWithProfiling_GenerationError(t *testing.T) {
	profileDir := t.TempDir()

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     profileDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	config := &CLIConfig{
		InputDir:   "/nonexistent_directory_xyz",
		OutputFile: "openapi.json",
		Title:      "Test",
		APIVersion: "1.0.0",
	}

	_, _, err := runGenerationWithProfiling(config, prof)
	if err == nil {
		t.Fatal("expected error from generation failure")
	}
}

// ---------------------------------------------------------------------------
// generatePerformanceAnalysis — error writing metrics
// ---------------------------------------------------------------------------

func TestGeneratePerformanceAnalysis_WriteError(t *testing.T) {
	profileDir := t.TempDir()

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     profileDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	config := &CLIConfig{
		ProfileOutputDir:   "/nonexistent_root_xyz",
		ProfileMetricsPath: "metrics.json",
	}

	err := generatePerformanceAnalysis(prof, config)
	if err == nil {
		t.Fatal("expected error when metrics write path is invalid")
	}
	if !strings.Contains(err.Error(), "failed to write metrics") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGeneratePerformanceAnalysis_VerboseWithIssues(t *testing.T) {
	profileDir := t.TempDir()

	profConfig := &profiler.ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     profileDir,
	}
	prof := profiler.NewProfiler(profConfig)
	if err := prof.Start(); err != nil {
		t.Fatalf("failed to start profiler: %v", err)
	}
	defer func() {
		_ = prof.Stop()
	}()

	// Add some metrics to make the analyzer report issues
	mc := prof.GetMetrics()
	mc.SetGauge("test.memory.alloc", 999999999.0, "bytes", map[string]string{"test": "true"})

	config := &CLIConfig{
		ProfileOutputDir:   profileDir,
		ProfileMetricsPath: "metrics.json",
		Verbose:            true,
	}

	err := generatePerformanceAnalysis(prof, config)
	if err != nil {
		t.Fatalf("generatePerformanceAnalysis failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// parseFlags — additional coverage
// ---------------------------------------------------------------------------

func TestParseFlags_VersionFlag(t *testing.T) {
	config, err := parseFlags([]string{"--version"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !config.ShowVersion {
		t.Error("expected ShowVersion to be true")
	}
}

func TestParseFlags_ShortVersionFlag(t *testing.T) {
	config, err := parseFlags([]string{"-V"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !config.ShowVersion {
		t.Error("expected ShowVersion to be true with -V")
	}
}

func TestParseFlags_DiagramPageSizeClamp(t *testing.T) {
	// Test page size below minimum
	config, err := parseFlags([]string{"-dps", "10"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.DiagramPageSize != 50 {
		t.Errorf("expected DiagramPageSize clamped to 50, got %d", config.DiagramPageSize)
	}

	// Test page size above maximum
	config, err = parseFlags([]string{"-dps", "1000"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.DiagramPageSize != 500 {
		t.Errorf("expected DiagramPageSize clamped to 500, got %d", config.DiagramPageSize)
	}
}

func TestParseFlags_OutputResolvesToAbsolute(t *testing.T) {
	config, err := parseFlags([]string{"-o", "relative/output.json"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !filepath.IsAbs(config.OutputFile) {
		t.Errorf("expected absolute output path, got %s", config.OutputFile)
	}
	if !config.OutputFlagSet {
		t.Error("expected OutputFlagSet to be true")
	}
}

func TestParseFlags_DiagramPathResolvesToAbsolute(t *testing.T) {
	config, err := parseFlags([]string{"-g", "diagram.html"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !filepath.IsAbs(config.DiagramPath) {
		t.Errorf("expected absolute diagram path, got %s", config.DiagramPath)
	}
}

func TestParseFlags_OutputConfigResolvesToAbsolute(t *testing.T) {
	config, err := parseFlags([]string{"-oc", "config_out.yaml"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !filepath.IsAbs(config.OutputConfig) {
		t.Errorf("expected absolute output config path, got %s", config.OutputConfig)
	}
}

func TestParseFlags_AbsoluteOutputNotModified(t *testing.T) {
	absPath := "/tmp/absolute_output.json"
	config, err := parseFlags([]string{"-o", absPath})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.OutputFile != absPath {
		t.Errorf("expected output to remain %s, got %s", absPath, config.OutputFile)
	}
}

func TestParseFlags_ProfilingFlags(t *testing.T) {
	config, err := parseFlags([]string{
		"--cpu-profile",
		"--mem-profile",
		"--block-profile",
		"--mutex-profile",
		"--trace-profile",
		"--custom-metrics",
		"--profile-dir", "/tmp/profiles",
		"--cpu-profile-path", "my_cpu.prof",
		"--mem-profile-path", "my_mem.prof",
		"--block-profile-path", "my_block.prof",
		"--mutex-profile-path", "my_mutex.prof",
		"--trace-profile-path", "my_trace.out",
		"--metrics-path", "my_metrics.json",
	})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !config.CPUProfile {
		t.Error("expected CPUProfile=true")
	}
	if !config.MemProfile {
		t.Error("expected MemProfile=true")
	}
	if !config.BlockProfile {
		t.Error("expected BlockProfile=true")
	}
	if !config.MutexProfile {
		t.Error("expected MutexProfile=true")
	}
	if !config.TraceProfile {
		t.Error("expected TraceProfile=true")
	}
	if !config.CustomMetrics {
		t.Error("expected CustomMetrics=true")
	}
	if config.ProfileOutputDir != "/tmp/profiles" {
		t.Errorf("expected profile dir /tmp/profiles, got %s", config.ProfileOutputDir)
	}
	if config.ProfileCPUPath != "my_cpu.prof" {
		t.Errorf("expected cpu path my_cpu.prof, got %s", config.ProfileCPUPath)
	}
}

func TestParseFlags_IncludeExcludeFlags(t *testing.T) {
	config, err := parseFlags([]string{
		"--include-file", "*.go",
		"--include-package", "main",
		"--include-function", "Handle",
		"--include-type", "Server",
		"--exclude-file", "test_*.go",
		"--exclude-package", "vendor",
		"--exclude-function", "deprecated",
		"--exclude-type", "Mock",
	})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if len(config.IncludeFiles) != 1 || config.IncludeFiles[0] != "*.go" {
		t.Errorf("unexpected IncludeFiles: %v", config.IncludeFiles)
	}
	if len(config.ExcludePackages) != 1 || config.ExcludePackages[0] != "vendor" {
		t.Errorf("unexpected ExcludePackages: %v", config.ExcludePackages)
	}
}

func TestParseFlags_ShortNamesExplicitSet(t *testing.T) {
	config, err := parseFlags([]string{"--short-names=false"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.ShortNames {
		t.Error("expected ShortNames=false")
	}
	if !config.ShortNamesSet {
		t.Error("expected ShortNamesSet=true when explicitly set")
	}
}

func TestParseFlags_InvalidFlag(t *testing.T) {
	_, err := parseFlags([]string{"--nonexistent-flag"})
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}

func TestParseFlags_HelpFlag(t *testing.T) {
	_, err := parseFlags([]string{"-help"})
	if err == nil {
		t.Fatal("expected error (flag.ErrHelp) for --help")
	}
}

func TestParseFlags_VerboseFlag(t *testing.T) {
	config, err := parseFlags([]string{"--verbose"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !config.Verbose {
		t.Error("expected Verbose=true")
	}
}

func TestParseFlags_DefaultValues(t *testing.T) {
	config, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if config.InputDir != engine.DefaultInputDir {
		t.Errorf("expected default InputDir %q, got %q", engine.DefaultInputDir, config.InputDir)
	}
	if config.Title != engine.DefaultTitle {
		t.Errorf("expected default Title %q, got %q", engine.DefaultTitle, config.Title)
	}
	if config.APIVersion != engine.DefaultAPIVersion {
		t.Errorf("expected default APIVersion %q, got %q", engine.DefaultAPIVersion, config.APIVersion)
	}
	if config.OpenAPIVersion != engine.DefaultOpenAPIVersion {
		t.Errorf("expected default OpenAPIVersion %q, got %q", engine.DefaultOpenAPIVersion, config.OpenAPIVersion)
	}
	if config.MaxNodesPerTree != engine.DefaultMaxNodesPerTree {
		t.Errorf("expected default MaxNodesPerTree %d, got %d", engine.DefaultMaxNodesPerTree, config.MaxNodesPerTree)
	}
}

// ---------------------------------------------------------------------------
// writeOutput — JSON to stdout (verify output is valid JSON)
// ---------------------------------------------------------------------------

func TestWriteOutput_StdoutJSONContent(t *testing.T) {
	spec := map[string]interface{}{
		"openapi": "3.1.1",
		"info": map[string]interface{}{
			"title":   "Test",
			"version": "1.0.0",
		},
	}
	config := &CLIConfig{
		OutputFile:    engine.DefaultOutputFile,
		OutputFlagSet: false,
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe: %v", pipeErr)
	}
	os.Stdout = w

	err := writeOutput(spec, config, nil)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("writeOutput failed: %v", err)
	}

	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("stdout output is not valid JSON: %v", err)
	}
	if parsed["openapi"] != "3.1.1" {
		t.Errorf("expected openapi 3.1.1, got %v", parsed["openapi"])
	}
}

// ---------------------------------------------------------------------------
// writeOutput — relative path joined with ModuleRoot
// ---------------------------------------------------------------------------

func TestWriteOutput_RelativePathJoinedWithModuleRoot(t *testing.T) {
	tempDir := createTestGoProject2(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    "relative_output.json", // relative path
		OutputFlagSet: true,                   // triggers file path branch
		Title:         "Test API",
		APIVersion:    "1.0.0",
	}

	spec, eng, err := runGeneration(config)
	if err != nil {
		t.Fatalf("runGeneration failed: %v", err)
	}

	// Because OutputFile is a relative path and OutputFlagSet is true,
	// parseFlags would have made it absolute. But since we construct
	// CLIConfig directly, the relative path goes through
	// filepath.Join(genEngine.ModuleRoot(), outputPath) in writeOutput.
	err = writeOutput(spec, config, eng)
	if err != nil {
		t.Fatalf("writeOutput with relative path failed: %v", err)
	}

	// Verify the output file was created at the expected location
	expectedPath := filepath.Join(eng.ModuleRoot(), "relative_output.json")
	if _, statErr := os.Stat(expectedPath); os.IsNotExist(statErr) {
		t.Errorf("expected output file at %s, but it does not exist", expectedPath)
	}
}

// ---------------------------------------------------------------------------
// run — output write error path
// ---------------------------------------------------------------------------

func TestRun_OutputWriteError(t *testing.T) {
	tempDir := createTestGoProject2(t)

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    "/nonexistent_root_xyz/output.json",
		OutputFlagSet: true,
		Title:         "Test",
		APIVersion:    "1.0.0",
	}

	err := run(config)
	if err == nil {
		t.Fatal("expected error when output file path is invalid")
	}
}

func TestRun_YAMLOutputFile(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.yaml")

	config := &CLIConfig{
		InputDir:      tempDir,
		OutputFile:    outputFile,
		OutputFlagSet: true,
		Title:         "YAML Test API",
		APIVersion:    "1.0.0",
	}

	err := run(config)
	if err != nil {
		t.Fatalf("run with YAML output failed: %v", err)
	}

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read YAML output: %v", err)
	}
	if !strings.Contains(string(data), "openapi:") {
		t.Error("YAML output missing 'openapi:' key")
	}
}

// ---------------------------------------------------------------------------
// run — performance analysis error path (logged, not returned)
// ---------------------------------------------------------------------------

func TestRun_WithCustomMetricsInvalidMetricsPath(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	// Create a directory where the metrics file should be, causing the write to fail
	metricsDir := filepath.Join(profileDir, "metrics.json")
	if err := os.MkdirAll(metricsDir, 0750); err != nil {
		t.Fatalf("failed to create metrics directory: %v", err)
	}

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Metrics Error Test",
		APIVersion:         "1.0.0",
		CustomMetrics:      true,
		ProfileOutputDir:   profileDir,
		ProfileMetricsPath: "metrics.json", // this is a directory, not a file
	}

	// This should succeed overall because the performance analysis error
	// is only logged, not returned
	err := run(config)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// run — profiler stop error (logged in defer, not returned)
// ---------------------------------------------------------------------------

func TestRun_ProfilerStopError(t *testing.T) {
	tempDir := createTestGoProject2(t)
	outputFile := filepath.Join(tempDir, "spec.json")
	profileDir := filepath.Join(tempDir, "profiles")

	config := &CLIConfig{
		InputDir:           tempDir,
		OutputFile:         outputFile,
		OutputFlagSet:      true,
		Title:              "Stop Error Test",
		APIVersion:         "1.0.0",
		MemProfile:         true,
		ProfileOutputDir:   profileDir,
		ProfileMemPath:     "nonexistent_subdir/mem.prof", // will fail on stop
		ProfileMetricsPath: "metrics.json",
	}

	// This should succeed overall because profiler stop error is logged
	err := run(config)
	if err != nil {
		t.Fatalf("run returned error (profiler stop error may have propagated): %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func createTestGoProject2(t *testing.T) string {
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
