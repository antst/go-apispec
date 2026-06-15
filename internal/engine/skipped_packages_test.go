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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestModulePathFromGoMod(t *testing.T) {
	cases := map[string]string{
		"module example.com/app\n\ngo 1.22\n":        "example.com/app",
		"// header comment\nmodule github.com/x/y\n": "github.com/x/y",
		"module github.com/x/y // inline comment\n":  "github.com/x/y",
		"\tmodule\tgithub.com/tabbed/m\n":            "github.com/tabbed/m",
		"module \"example.com/quoted\"\n":            "example.com/quoted",
		"go 1.22\nrequire foo v1.0.0\n":              "", // no module directive
		"modulefoo bar\n":                            "", // not the module directive
		"":                                           "",
	}
	for src, want := range cases {
		assert.Equal(t, want, modulePathFromGoMod([]byte(src)), "src=%q", src)
	}
}

func TestModuleImportPath(t *testing.T) {
	// No module root configured.
	assert.Equal(t, "", NewEngine(nil).moduleImportPath())

	// Module root with a readable go.mod.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.22\n"), 0o644))
	e := NewEngine(nil)
	e.config.moduleRoot = dir
	assert.Equal(t, "example.com/app", e.moduleImportPath())

	// Module root without a go.mod.
	e.config.moduleRoot = t.TempDir()
	assert.Equal(t, "", e.moduleImportPath())
}

func TestFilterValidPackages_RecordsInModuleSkipped(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.22\n"), 0o644))
	e := NewEngine(nil)
	e.config.moduleRoot = dir

	pkgs := []*packages.Package{
		{PkgPath: "example.com/app/handlers"},                                                    // valid in-module
		{PkgPath: "example.com/app", Errors: []packages.Error{{Msg: "undefined: Main"}}},         // broken in-module (exact match)
		{PkgPath: "example.com/app/broken", Errors: []packages.Error{{Msg: "undefined: X"}}},     // broken in-module (prefix)
		{PkgPath: "github.com/third/party", Errors: []packages.Error{{Msg: "third-party boom"}}}, // broken third-party (ignored)
	}

	result, err := e.filterValidPackages(pkgs, NewVerboseLogger(false))
	require.NoError(t, err)
	require.Len(t, result, 1, "only the error-free package is valid")
	assert.Equal(t, "example.com/app/handlers", result[0].PkgPath)

	skipped := e.SkippedPackages()
	require.Len(t, skipped, 2, "only in-module broken packages are recorded")
	got := map[string]string{}
	for _, sp := range skipped {
		got[sp.Package] = sp.Reason
	}
	assert.Equal(t, "undefined: Main", got["example.com/app"])
	assert.Equal(t, "undefined: X", got["example.com/app/broken"])
	assert.NotContains(t, got, "github.com/third/party", "third-party errors are not reported")
}

func TestSkippedPackages_EmptyByDefault(t *testing.T) {
	assert.Empty(t, NewEngine(nil).SkippedPackages())
}

// TestSkippedPackages_ReturnsDefensiveCopy ensures callers can't mutate the
// engine's internal diagnostics state through the returned slice.
func TestSkippedPackages_ReturnsDefensiveCopy(t *testing.T) {
	e := NewEngine(nil)
	e.skipped = []SkippedPackage{{Package: "a/b", Reason: "x"}}

	got := e.SkippedPackages()
	require.Len(t, got, 1)
	got[0].Package = "MUTATED"
	got = append(got, SkippedPackage{Package: "extra"})
	_ = got

	again := e.SkippedPackages()
	require.Len(t, again, 1, "append on the returned slice must not grow internal state")
	assert.Equal(t, "a/b", again[0].Package, "element mutation must not leak into the engine")
}
