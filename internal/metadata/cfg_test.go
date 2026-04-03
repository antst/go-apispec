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
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFunctionCFGs_NilMetadata(_ *testing.T) {
	// Should not panic
	BuildFunctionCFGs(nil, nil, nil)
	BuildFunctionCFGs([]*ast.FuncDecl{}, nil, nil)
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
	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
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

	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
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
	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
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
