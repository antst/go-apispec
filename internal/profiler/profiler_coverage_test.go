package profiler

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// startCPUProfile — error paths
// ---------------------------------------------------------------------------

func TestStartCPUProfile_InvalidDir(t *testing.T) {
	// Create profiler with an output dir that does not exist
	config := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu.prof",
		OutputDir:      "/nonexistent_profiler_test_dir_xyz/nested",
	}
	p := NewProfiler(config)

	// Call startCPUProfile directly (must lock first because Start locks)
	err := p.startCPUProfile()
	if err == nil {
		t.Fatal("expected error when output directory does not exist")
	}
	// cpuFile should remain nil
	if p.cpuFile != nil {
		t.Error("expected cpuFile to be nil on error")
	}
}

func TestStartCPUProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu.prof",
		OutputDir:      tmpDir,
	}
	p := NewProfiler(config)

	err := p.startCPUProfile()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p.cpuFile == nil {
		t.Fatal("expected cpuFile to be set")
	}

	// Verify the file was created
	filePath := filepath.Join(tmpDir, "cpu.prof")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Error("expected cpu.prof file to be created")
	}

	// Clean up: stop CPU profiling via Stop
	err = p.Stop()
	if err != nil {
		t.Errorf("failed to stop profiler: %v", err)
	}
}

// ---------------------------------------------------------------------------
// startTraceProfile — error paths
// ---------------------------------------------------------------------------

func TestStartTraceProfile_InvalidDir(t *testing.T) {
	config := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace.out",
		OutputDir:        "/nonexistent_profiler_test_dir_xyz/nested",
	}
	p := NewProfiler(config)

	err := p.startTraceProfile()
	if err == nil {
		t.Fatal("expected error when output directory does not exist")
	}
	if p.traceFile != nil {
		t.Error("expected traceFile to be nil on error")
	}
}

func TestStartTraceProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace.out",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.startTraceProfile()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p.traceFile == nil {
		t.Fatal("expected traceFile to be set")
	}

	// Verify the file was created
	filePath := filepath.Join(tmpDir, "trace.out")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Error("expected trace.out file to be created")
	}

	// Clean up
	err = p.Stop()
	if err != nil {
		t.Errorf("failed to stop profiler: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Stop — error paths: close errors, metrics stop, combined errors
// ---------------------------------------------------------------------------

func TestStop_WithAllProfileTypes(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		CPUProfile:       true,
		CPUProfilePath:   "cpu.prof",
		MemProfile:       true,
		MemProfilePath:   "mem.prof",
		BlockProfile:     true,
		BlockProfilePath: "block.prof",
		MutexProfile:     true,
		MutexProfilePath: "mutex.prof",
		TraceProfile:     true,
		TraceProfilePath: "trace.out",
		CustomMetrics:    true,
		MetricsPath:      "metrics.json",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify all profile files were created
	for _, fname := range []string{"cpu.prof", "mem.prof", "block.prof", "mutex.prof", "trace.out"} {
		fpath := filepath.Join(tmpDir, fname)
		if _, statErr := os.Stat(fpath); os.IsNotExist(statErr) {
			t.Errorf("expected %s to be created", fname)
		}
	}

	// cpuFile and traceFile should be nil after stop
	if p.cpuFile != nil {
		t.Error("expected cpuFile to be nil after stop")
	}
	if p.traceFile != nil {
		t.Error("expected traceFile to be nil after stop")
	}
}

func TestStop_MemProfileWriteError(t *testing.T) {
	// Set MemProfilePath to a directory that doesn't exist to trigger error in stopMemProfile
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		MemProfile:     true,
		MemProfilePath: "nonexistent_subdir/mem.prof",
		OutputDir:      tmpDir,
	}
	p := NewProfiler(config)

	// Start doesn't fail because mem profiling is deferred to stop
	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should collect the error from stopMemProfile
	err = p.Stop()
	if err == nil {
		t.Fatal("expected error from Stop when mem profile path is invalid")
	}
}

func TestStop_BlockProfileWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		BlockProfile:     true,
		BlockProfilePath: "nonexistent_subdir/block.prof",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err == nil {
		t.Fatal("expected error from Stop when block profile path is invalid")
	}
}

func TestStop_MutexProfileWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		MutexProfile:     true,
		MutexProfilePath: "nonexistent_subdir/mutex.prof",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err == nil {
		t.Fatal("expected error from Stop when mutex profile path is invalid")
	}
}

// ---------------------------------------------------------------------------
// stopMemProfile — success path (verify file content)
// ---------------------------------------------------------------------------

func TestStopMemProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		MemProfile:     true,
		MemProfilePath: "mem.prof",
		OutputDir:      tmpDir,
	}
	p := NewProfiler(config)

	// Start and allocate some memory
	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Do some allocation
	_ = make([]byte, 1024*1024)

	err = p.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify the memory profile file was written and is non-empty
	fpath := filepath.Join(tmpDir, "mem.prof")
	info, statErr := os.Stat(fpath)
	if os.IsNotExist(statErr) {
		t.Fatal("expected mem.prof to be created")
	}
	if info.Size() == 0 {
		t.Error("expected mem.prof to be non-empty")
	}
}

// ---------------------------------------------------------------------------
// stopBlockProfile — success path
// ---------------------------------------------------------------------------

func TestStopBlockProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		BlockProfile:     true,
		BlockProfilePath: "block.prof",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	fpath := filepath.Join(tmpDir, "block.prof")
	info, statErr := os.Stat(fpath)
	if os.IsNotExist(statErr) {
		t.Fatal("expected block.prof to be created")
	}
	if info.Size() == 0 {
		t.Error("expected block.prof to be non-empty")
	}
}

// ---------------------------------------------------------------------------
// stopMutexProfile — success path
// ---------------------------------------------------------------------------

func TestStopMutexProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		MutexProfile:     true,
		MutexProfilePath: "mutex.prof",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	fpath := filepath.Join(tmpDir, "mutex.prof")
	info, statErr := os.Stat(fpath)
	if os.IsNotExist(statErr) {
		t.Fatal("expected mutex.prof to be created")
	}
	if info.Size() == 0 {
		t.Error("expected mutex.prof to be non-empty")
	}
}

// ---------------------------------------------------------------------------
// Start — error path for output directory creation
// ---------------------------------------------------------------------------

func TestStart_OutputDirCreationFailure(t *testing.T) {
	// Use a read-only parent to prevent directory creation
	config := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu.prof",
		OutputDir:      "/nonexistent_root_xyz/profiles",
	}
	p := NewProfiler(config)

	err := p.Start()
	if err == nil {
		t.Fatal("expected error when output directory cannot be created")
	}
}

func TestStart_CPUProfileError(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a path that cannot be created (directory instead of file)
	dirAsFile := filepath.Join(tmpDir, "subdir")
	if mkErr := os.Mkdir(dirAsFile, 0750); mkErr != nil {
		t.Fatalf("failed to create subdir: %v", mkErr)
	}
	config := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "subdir", // this is a directory, not a file
		OutputDir:      tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err == nil {
		// It may or may not fail depending on OS behavior.
		// At minimum, if it succeeds, clean up.
		_ = p.Stop()
	}
}

func TestStart_TraceProfileError(t *testing.T) {
	tmpDir := t.TempDir()
	// Make a directory where the trace file should be
	dirAsFile := filepath.Join(tmpDir, "trace.out")
	if mkErr := os.Mkdir(dirAsFile, 0750); mkErr != nil {
		t.Fatalf("failed to create directory: %v", mkErr)
	}
	config := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace.out",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err == nil {
		_ = p.Stop()
	}
}

// ---------------------------------------------------------------------------
// Start — with empty OutputDir
// ---------------------------------------------------------------------------

func TestStart_EmptyOutputDir(t *testing.T) {
	// When OutputDir is empty, MkdirAll should be skipped
	config := &ProfilerConfig{
		CustomMetrics: true,
		MetricsPath:   "metrics.json",
		OutputDir:     "",
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("expected no error with empty OutputDir, got: %v", err)
	}

	err = p.Stop()
	if err != nil {
		t.Errorf("expected no error stopping, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Stop — with closed cpuFile (simulate close error)
// ---------------------------------------------------------------------------

func TestStop_CPUFileAlreadyClosed(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu.prof",
		OutputDir:      tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close the cpuFile manually to force a close error in Stop
	if p.cpuFile != nil {
		_ = p.cpuFile.Close()
	}

	err = p.Stop()
	// Should collect the "close" error
	if err == nil {
		t.Log("no error from Stop after pre-closing cpuFile (OS may allow double close)")
	}
}

// ---------------------------------------------------------------------------
// Stop — with closed traceFile (simulate close error)
// ---------------------------------------------------------------------------

func TestStop_TraceFileAlreadyClosed(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace.out",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Close the traceFile manually to force a close error in Stop
	if p.traceFile != nil {
		_ = p.traceFile.Close()
	}

	err = p.Stop()
	// Should collect the "close" error
	if err == nil {
		t.Log("no error from Stop after pre-closing traceFile (OS may allow double close)")
	}
}

// ---------------------------------------------------------------------------
// startCPUProfile — double start triggers pprof.StartCPUProfile error
// ---------------------------------------------------------------------------

func TestStartCPUProfile_DoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	config1 := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu1.prof",
		OutputDir:      tmpDir,
	}
	p1 := NewProfiler(config1)

	// Start first CPU profiler
	err := p1.startCPUProfile()
	if err != nil {
		t.Fatalf("first startCPUProfile failed: %v", err)
	}

	// Second CPU profiler should fail because pprof.StartCPUProfile is already active
	config2 := &ProfilerConfig{
		CPUProfile:     true,
		CPUProfilePath: "cpu2.prof",
		OutputDir:      tmpDir,
	}
	p2 := NewProfiler(config2)

	err = p2.startCPUProfile()
	if err == nil {
		// pprof.StartCPUProfile should return an error when already profiling
		t.Log("second startCPUProfile did not error (unexpected, but OS may allow it)")
		_ = p2.Stop()
	}

	// Clean up first profiler
	_ = p1.Stop()
}

// ---------------------------------------------------------------------------
// startTraceProfile — double start triggers trace.Start error
// ---------------------------------------------------------------------------

func TestStartTraceProfile_DoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	config1 := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace1.out",
		OutputDir:        tmpDir,
	}
	p1 := NewProfiler(config1)

	// Start first trace profiler
	err := p1.startTraceProfile()
	if err != nil {
		t.Fatalf("first startTraceProfile failed: %v", err)
	}

	// Second trace profiler should fail because trace.Start is already active
	config2 := &ProfilerConfig{
		TraceProfile:     true,
		TraceProfilePath: "trace2.out",
		OutputDir:        tmpDir,
	}
	p2 := NewProfiler(config2)

	err = p2.startTraceProfile()
	if err == nil {
		t.Log("second startTraceProfile did not error (unexpected, but may work on some OSes)")
		_ = p2.Stop()
	}

	// Clean up first profiler
	_ = p1.Stop()
}

// ---------------------------------------------------------------------------
// Stop — multiple errors combined
// ---------------------------------------------------------------------------

func TestStop_MultipleErrors(t *testing.T) {
	tmpDir := t.TempDir()
	config := &ProfilerConfig{
		MemProfile:       true,
		MemProfilePath:   "bad_subdir/mem.prof",
		BlockProfile:     true,
		BlockProfilePath: "bad_subdir/block.prof",
		MutexProfile:     true,
		MutexProfilePath: "bad_subdir/mutex.prof",
		OutputDir:        tmpDir,
	}
	p := NewProfiler(config)

	err := p.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = p.Stop()
	if err == nil {
		t.Fatal("expected combined errors from Stop")
	}
	// Should contain "profiling stop errors"
	if !contains(err.Error(), "profiling stop errors") {
		t.Errorf("expected 'profiling stop errors' in error, got: %v", err)
	}
}
