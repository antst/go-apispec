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

package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestResolvedPathThreadsTypeRef is the Phase-3 parse-free gate (SC-001).
//
// The type-RESOLUTION subsystem lives in extractor.go and pattern_matchers.go;
// every schemaForType call in those two files is a resolved body/param/return
// position. Phase 3 threads a structured *TypeRef into each of them so the schema
// generator consumes the tree directly instead of re-parsing the resolved string
// (schemaForType walks a non-nil canonical ref via schemaForRefTree, never
// reaching schemaFromParsedString's ParseTypeRef — see its doc comment).
//
// This gate asserts that invariant structurally: no schemaForType call on the
// resolution path passes a bare `nil` as its ref (3rd) argument. A nil there is a
// regression — it re-opens the resolved-string re-parse this phase closed. The
// legitimately-nil string-only callers (struct-field recursion, named-underlying
// expansion, config overrides) live in mapper.go / wrapper_specialisation.go and
// are out of scope here by construction.
//
// It is AST-based (not line- or text-matched) so it survives refactors: it tracks
// the call by name and the argument by position, and fails loudly if the call
// shape changes or the calls disappear (guarding against a silent rename).
func TestResolvedPathThreadsTypeRef(t *testing.T) {
	const refArgIndex = 2 // schemaForType(usedTypes, goType, ref, meta, cfg, visited)
	files := []string{"extractor.go", "pattern_matchers.go"}

	found := 0
	for _, file := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "schemaForType" {
				return true
			}
			found++
			if len(call.Args) <= refArgIndex {
				t.Errorf("%s:%d: schemaForType called with %d args, expected the ref at index %d",
					file, fset.Position(call.Pos()).Line, len(call.Args), refArgIndex)
				return true
			}
			if nilLit, ok := call.Args[refArgIndex].(*ast.Ident); ok && nilLit.Name == "nil" {
				t.Errorf("%s:%d: schemaForType on the resolution path passes nil ref — "+
					"resolved positions MUST thread a *TypeRef (SC-001); re-parsing the "+
					"resolved string is forbidden", file, fset.Position(call.Pos()).Line)
			}
			return true
		})
	}

	if found == 0 {
		t.Fatal("no schemaForType calls found in the resolution files — did the call rename? " +
			"the parse-free gate is no longer guarding anything")
	}
	t.Logf("verified %d schemaForType call(s) on the resolution path thread a non-nil TypeRef", found)
}
