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

package metadata_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/packages/packagestest"

	"github.com/antst/go-apispec/internal/metadata"
)

// metaFromModule type-checks a single-module source map and returns the
// generated metadata, mirroring the load flow the engine uses.
func metaFromModule(t *testing.T, files map[string]interface{}) *metadata.Metadata {
	t.Helper()
	fset := token.NewFileSet()
	exported := packagestest.Export(t, packagestest.GOPATH, []packagestest.Module{{Name: "app", Files: files}})
	defer exported.Cleanup()

	exported.Config.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
	exported.Config.Fset = fset
	exported.Config.Tests = false

	pkgs, err := packages.Load(exported.Config, "./...")
	require.NoError(t, err)

	pkgsMetadata := map[string]map[string]*ast.File{}
	fileToInfo := map[*ast.File]*types.Info{}
	importPaths := map[string]string{}
	for _, pkg := range pkgs {
		if pkg.PkgPath == "" {
			continue
		}
		pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)
		for i, f := range pkg.Syntax {
			if i < len(pkg.GoFiles) {
				pkgsMetadata[pkg.PkgPath][pkg.GoFiles[i]] = f
				fileToInfo[f] = pkg.TypesInfo
				importPaths[pkg.GoFiles[i]] = pkg.PkgPath
			}
		}
	}
	return metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)
}

// findType looks up a type by short name across every file in a package.
func findType(meta *metadata.Metadata, pkgPath, name string) *metadata.Type {
	pkg, ok := meta.Packages[pkgPath]
	if !ok {
		return nil
	}
	for _, file := range pkg.Files {
		if t, ok := file.Types[name]; ok {
			return t
		}
	}
	return nil
}

// TestProcessLocalTypes verifies that named types declared inside function
// bodies are captured as real types (so a request/response bound to them emits
// a real component, not a dangling $ref), and that a function-local type never
// shadows a package-level type of the same name.
func TestProcessLocalTypes(t *testing.T) {
	meta := metaFromModule(t, map[string]interface{}{
		"app.go": `package app

// Conf is a package-level type whose name a local type also uses.
type Conf struct{ A string }

func handler() {
	// Login is a function-local named type — the kind that previously
	// resolved to a dangling $ref.
	type Login struct {
		Email    string ` + "`json:\"email\"`" + `
		Password string ` + "`json:\"password\"`" + `
	}
	_ = Login{}
}

func other() {
	// A function-local type reusing a package-level name must NOT overwrite
	// the package-level Conf.
	type Conf struct{ B int }
	_ = Conf{}
}
`,
	})

	const pkg = "app"

	// The function-local Login type is captured with its fields.
	login := findType(meta, pkg, "Login")
	require.NotNil(t, login, "function-local type Login must be captured")
	names := map[string]bool{}
	for _, f := range login.Fields {
		names[meta.StringPool.GetString(f.Name)] = true
	}
	require.True(t, names["Email"] && names["Password"], "local type fields captured")

	// The package-level Conf is preserved, not shadowed by the local one.
	conf := findType(meta, pkg, "Conf")
	require.NotNil(t, conf)
	fieldNames := make([]string, 0, len(conf.Fields))
	for _, f := range conf.Fields {
		fieldNames = append(fieldNames, meta.StringPool.GetString(f.Name))
	}
	require.Equal(t, []string{"A"}, fieldNames, "package-level Conf not shadowed by local Conf")
}

// TestMethodDocCommentsCaptured covers issue #45: doc comments on method
// declarations are captured into the type's Method records (previously
// processFunctions skipped methods, so method handler summaries were empty).
func TestMethodDocCommentsCaptured(t *testing.T) {
	meta := metaFromModule(t, map[string]interface{}{
		"h.go": `package app

type Handler struct{}

// GetUser returns a user by ID.
func (h *Handler) GetUser() {}
`,
	})
	typ := findType(meta, "app", "Handler")
	require.NotNil(t, typ)
	var got string
	for _, m := range typ.Methods {
		if meta.StringPool.GetString(m.Name) == "GetUser" {
			got = meta.StringPool.GetString(m.Comments)
		}
	}
	require.Contains(t, got, "returns a user by ID")
}

// TestProcessLocalTypes_CrossFileShadow verifies the shadow guard is
// package-scoped: a function-local type in one file must not overwrite a
// package-level type of the same name declared in a *different* file (the
// guard checks allTypes, not just the current file's f.Types).
func TestProcessLocalTypes_CrossFileShadow(t *testing.T) {
	meta := metaFromModule(t, map[string]interface{}{
		// Package-level Dup lives in a.go.
		"a.go": `package app

type Dup struct{ A string }
`,
		// A function-local Dup of the same name lives in b.go.
		"b.go": `package app

func makeLocal() {
	type Dup struct{ B int }
	_ = Dup{}
}
`,
	})

	dup := findType(meta, "app", "Dup")
	require.NotNil(t, dup)
	names := make([]string, 0, len(dup.Fields))
	for _, f := range dup.Fields {
		names = append(names, meta.StringPool.GetString(f.Name))
	}
	require.Equal(t, []string{"A"}, names, "package-level Dup must survive a same-named function-local type in another file")
}
