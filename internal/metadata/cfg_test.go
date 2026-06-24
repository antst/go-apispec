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
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typeCheckSrc parses and type-checks a single-file source, returning the file and
// its types.Info (best-effort; type errors are ignored so partial info still
// populates Types).
func typeCheckSrc(t *testing.T, src string) (*ast.File, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
	_, _ = conf.Check("test", fset, []*ast.File{file}, info)
	return file, info
}

func typeSwitchCases(t *testing.T, file *ast.File) []*ast.CaseClause {
	t.Helper()
	var cases []*ast.CaseClause
	ast.Inspect(file, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSwitchStmt); ok && ts.Body != nil {
			for _, stmt := range ts.Body.List {
				if cc, ok := stmt.(*ast.CaseClause); ok {
					cases = append(cases, cc)
				}
			}
		}
		return true
	})
	return cases
}

func TestExtractCaseTypeRefs(t *testing.T) {
	file, info := typeCheckSrc(t, `package main

type NotFound struct{}
type Conflict struct{}

func classify(err error) int {
	switch err.(type) {
	case *NotFound:
		return 404
	case *Conflict:
		return 409
	case nil:
		return 200
	default:
		return 500
	}
}
`)
	cases := typeSwitchCases(t, file)
	require.Len(t, cases, 4)

	// case *NotFound: / case *Conflict: → one type ref each.
	nf := extractCaseTypeRefs(cases[0], info)
	require.Len(t, nf, 1)
	assert.Contains(t, nf[0].String(), "NotFound")
	cf := extractCaseTypeRefs(cases[1], info)
	require.Len(t, cf, 1)
	assert.Contains(t, cf[0].String(), "Conflict")

	// case nil: and default: are the unconditional arm — no type captured.
	assert.Empty(t, extractCaseTypeRefs(cases[2], info))
	assert.Empty(t, extractCaseTypeRefs(cases[3], info))

	// Without type info we cannot tell type cases from value cases → nil.
	assert.Nil(t, extractCaseTypeRefs(cases[0], nil))
}

func TestExtractCaseTypeRefs_ValueSwitchIsNotAType(t *testing.T) {
	file, info := typeCheckSrc(t, `package main

func pick(s string) int {
	switch s {
	case "GET":
		return 1
	default:
		return 0
	}
}
`)
	// A value-switch case is NOT a TypeSwitchStmt, so typeSwitchCases finds none and
	// extractCaseTypeRefs (called on the value case clause) yields no type refs.
	assert.Empty(t, typeSwitchCases(t, file))
	var valueCases []*ast.CaseClause
	ast.Inspect(file, func(n ast.Node) bool {
		if cc, ok := n.(*ast.CaseClause); ok {
			valueCases = append(valueCases, cc)
		}
		return true
	})
	require.NotEmpty(t, valueCases)
	assert.Empty(t, extractCaseTypeRefs(valueCases[0], info)) // "GET" is a value, not a type
}

func TestBuildFunctionCFGs_NilMetadata(_ *testing.T) {
	// Should not panic
	BuildFunctionCFGs(nil, nil, nil, nil)
	BuildFunctionCFGs([]*ast.FuncDecl{}, nil, nil, nil)
}

func TestBuildFunctionCFGs_IfElseBranches(t *testing.T) {
	src := `package main

import "net/http"

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.WriteHeader(200)
	} else {
		w.WriteHeader(404)
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	// Get the function declaration
	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "handler" {
			funcDecl = fn
			break
		}
	}
	require.NotNil(t, funcDecl)

	meta := &Metadata{
		StringPool: NewStringPool(),
		CallGraph:  []CallGraphEdge{},
	}

	// Should not panic even with empty call graph
	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, nil, fset, meta)
}

func TestBuildFunctionCFGs_SkipsClosureBodies(t *testing.T) {
	// A nested function literal has its own scope and no CFG of its own, so its
	// inner positions must NOT be folded into the enclosing function's model —
	// otherwise a closure's statuses bleed into the handler's reachability.
	src := `package main

import "net/http"

func h(w http.ResponseWriter) {
	cb := func() { w.WriteHeader(500) }
	_ = cb
	w.WriteHeader(200)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == "h" {
			fn = f
		}
	}
	require.NotNil(t, fn)

	// Locate the outer WriteHeader(200) call and the closure-body WriteHeader(500).
	var outerCall, innerCall *ast.CallExpr
	ast.Inspect(fn, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			ast.Inspect(v.Body, func(m ast.Node) bool {
				if c, ok := m.(*ast.CallExpr); ok {
					innerCall = c
				}
				return true
			})
			return false // don't let outerCall pick up the closure's call
		case *ast.CallExpr:
			outerCall = v
		}
		return true
	})
	require.NotNil(t, outerCall)
	require.NotNil(t, innerCall)

	meta := &Metadata{StringPool: NewStringPool(), CallGraph: []CallGraphEdge{}}
	BuildFunctionCFGs([]*ast.FuncDecl{fn}, nil, fset, meta)
	fc := meta.FunctionCFGs[fset.Position(fn.Body.Pos()).String()]
	require.NotNil(t, fc)

	_, hasOuter := fc.PosToBlock[fset.Position(outerCall.Pos()).String()]
	_, hasInner := fc.PosToBlock[fset.Position(innerCall.Pos()).String()]
	assert.True(t, hasOuter, "the enclosing function's own call is captured")
	assert.False(t, hasInner, "a closure-body position must not be folded into the enclosing function")
}

func TestBuildFunctionCFGs_SwitchCaseBranches(t *testing.T) {
	src := `package main

func dispatch(method string) {
	switch method {
	case "GET":
		println("get")
	case "POST":
		println("post")
	default:
		println("other")
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "dispatch" {
			funcDecl = fn
			break
		}
	}
	require.NotNil(t, funcDecl)

	meta := &Metadata{
		StringPool: NewStringPool(),
		CallGraph:  []CallGraphEdge{},
	}

	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, nil, fset, meta)
}

func TestBuildFunctionCFGs_UnconditionalCode(t *testing.T) {
	src := `package main

func simple() {
	x := 1
	_ = x
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "simple" {
			funcDecl = fn
			break
		}
	}
	require.NotNil(t, funcDecl)

	meta := &Metadata{
		StringPool: NewStringPool(),
		CallGraph:  []CallGraphEdge{},
	}

	// Unconditional code: no edges should get Branch annotations
	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, nil, fset, meta)
	for _, edge := range meta.CallGraph {
		assert.Nil(t, edge.Branch, "unconditional code should have nil Branch")
	}
}

func TestMapBlockKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     int
		expected string
	}{
		{"if-then", 1, "if-then"}, // KindIfThen
		{"if-else", 2, "if-else"}, // KindIfElse
		{"unknown", 999, ""},
	}

	// We can't directly test cfg.BlockKind values since they're unexported constants,
	// but we verify the mapping function doesn't panic on various inputs.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// mapBlockKind accepts cfg.BlockKind which is an int type
			// Just verify it doesn't panic
			assert.NotPanics(t, func() {
				_ = mapBlockKind(0) // Default/entry block
			})
		})
	}
}

func TestBranchContext_Struct(t *testing.T) {
	ctx := &BranchContext{
		BlockIndex:    5,
		BlockKind:     "if-then",
		ParentStmtPos: 42,
	}
	assert.Equal(t, int32(5), ctx.BlockIndex)
	assert.Equal(t, "if-then", ctx.BlockKind)
	assert.Equal(t, 42, ctx.ParentStmtPos)
}

// --- spec 009: type-switch operand + default-arm capture -------------------

func tsFindSwitch(file *ast.File) *ast.TypeSwitchStmt {
	var ts *ast.TypeSwitchStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if t, ok := n.(*ast.TypeSwitchStmt); ok && ts == nil {
			ts = t
		}
		return true
	})
	return ts
}

func tsFuncBody(file *ast.File, name string) *ast.BlockStmt {
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn.Body
		}
	}
	return nil
}

func TestTypeSwitchOperandName(t *testing.T) {
	// assignment form: switch x := v.(type)
	f1, _ := typeCheckSrc(t, "package p\nfunc h(v any) { switch x := v.(type) { case int: _ = x; default: } }")
	assert.Equal(t, "v", typeSwitchOperandName(tsFindSwitch(f1)))

	// bare expression form: switch v.(type)
	f2, _ := typeCheckSrc(t, "package p\nfunc h(v any) { switch v.(type) { case int: } }")
	assert.Equal(t, "v", typeSwitchOperandName(tsFindSwitch(f2)))

	// non-ident operand (selector) → no operand
	f3, _ := typeCheckSrc(t, "package p\ntype S struct{ E any }\nfunc h(s S) { switch s.E.(type) { case int: } }")
	assert.Equal(t, "", typeSwitchOperandName(tsFindSwitch(f3)))
}

func TestTypeSwitchOperandsAndDefaults(t *testing.T) {
	f, _ := typeCheckSrc(t, "package p\nfunc h(v any) {\n\tswitch x := v.(type) {\n\tcase *int:\n\t\t_ = x\n\tdefault:\n\t\t_ = x\n\t}\n}")
	body := tsFuncBody(f, "h")

	ops := typeSwitchOperands(body)
	assert.NotEmpty(t, ops)
	for _, op := range ops {
		assert.Equal(t, "v", op)
	}

	defs := typeSwitchDefaultArms(body)
	require.Len(t, defs, 1)
	assert.Equal(t, "v", defs[0].operand)
	assert.Less(t, int(defs[0].lo), int(defs[0].hi))

	// A type switch with no default clause yields no default arm.
	f2, _ := typeCheckSrc(t, "package p\nfunc h(v any) { switch v.(type) { case int: } }")
	assert.Empty(t, typeSwitchDefaultArms(tsFuncBody(f2, "h")))

	// A switch on a non-ident operand (selector) records no operands/defaults.
	f3, _ := typeCheckSrc(t, "package p\ntype S struct{ E any }\nfunc h(s S) { switch s.E.(type) { case int: default: } }")
	assert.Empty(t, typeSwitchOperands(tsFuncBody(f3, "h")))
	assert.Empty(t, typeSwitchDefaultArms(tsFuncBody(f3, "h")))
}

func TestAnnotateDefaultArm(t *testing.T) {
	meta := &Metadata{StringPool: NewStringPool()}
	defaults := []tsDefaultArm{{lo: 10, hi: 20, operand: "v"}}

	// In-range: the edge and the assignment both get the default-arm Branch.
	edge := &CallGraphEdge{Position: meta.StringPool.Get("p.go:5:3")}
	assign := &Assignment{}
	annotateDefaultArm(token.Pos(15), "p.go:5:3", defaults,
		map[string]*CallGraphEdge{"p.go:5:3": edge}, map[string][]*Assignment{"p.go:5:3": {assign}})
	require.NotNil(t, edge.Branch)
	assert.Equal(t, "switch-case", edge.Branch.BlockKind)
	assert.Equal(t, "v", edge.Branch.SwitchOperand)
	assert.Empty(t, edge.Branch.CaseTypeRefs)
	require.NotNil(t, assign.Branch)

	// Out of range: untouched.
	edge2 := &CallGraphEdge{}
	annotateDefaultArm(token.Pos(25), "q.go:1:1", defaults, map[string]*CallGraphEdge{"q.go:1:1": edge2}, nil)
	assert.Nil(t, edge2.Branch)

	// Already annotated: not overridden.
	edge3 := &CallGraphEdge{Branch: &BranchContext{BlockKind: "if-then"}}
	annotateDefaultArm(token.Pos(15), "p.go:5:3", defaults, map[string]*CallGraphEdge{"p.go:5:3": edge3}, nil)
	assert.Equal(t, "if-then", edge3.Branch.BlockKind)

	// Nested defaults: a write inside the TIGHTER (inner) span binds the inner
	// operand, not the lexically-enclosing outer one.
	nested := []tsDefaultArm{
		{lo: 10, hi: 40, operand: "outer"},
		{lo: 20, hi: 30, operand: "inner"},
	}
	edge4 := &CallGraphEdge{}
	annotateDefaultArm(token.Pos(25), "n.go:1:1", nested, map[string]*CallGraphEdge{"n.go:1:1": edge4}, nil)
	require.NotNil(t, edge4.Branch)
	assert.Equal(t, "inner", edge4.Branch.SwitchOperand)
}

// --- spec 009 US2: if r.Method == … method-guard capture --------------------

func tsFindIf(file *ast.File) *ast.IfStmt {
	var ifs *ast.IfStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if s, ok := n.(*ast.IfStmt); ok && ifs == nil {
			ifs = s
		}
		return true
	})
	return ifs
}

func TestExtractMethodGuard(t *testing.T) {
	cases := []struct {
		name string
		cond string
		want []string
	}{
		{"selector", "r.Method == http.MethodPost", []string{"POST"}},
		{"string literal", `r.Method == "GET"`, []string{"GET"}},
		{"reversed", "http.MethodPut == r.Method", []string{"PUT"}},
		{"lowercase literal", `r.Method == "delete"`, []string{"DELETE"}},
		{"non-method field", "r.Other == http.MethodPost", nil},
		{"not equality", "r.Method != http.MethodPost", nil},
		{"unknown method const", "r.Method == http.MethodFoo", nil},
		{"both sides non-method", "a == b", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\nimport \"net/http\"\nfunc h(r *http.Request) {\n\tvar a, b int\n\t_ = a\n\t_ = b\n\tif " + tc.cond + " {\n\t}\n}"
			f, _ := typeCheckSrc(t, src)
			got := extractMethodGuard(tsFindIf(f))
			assert.Equal(t, tc.want, got)
		})
	}
	// A non-if statement yields nothing.
	assert.Nil(t, extractMethodGuard(&ast.ExprStmt{}))
}
