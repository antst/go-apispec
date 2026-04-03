// Copyright 2025-2026 Anton Starikov
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
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SetRepoRoot ---

func TestSetRepoRoot_Empty(t *testing.T) {
	// Reset for safety
	repoRoot = ""
	SetRepoRoot("")
	assert.Equal(t, "", repoRoot)
}

func TestSetRepoRoot_WithTrailingSlash(t *testing.T) {
	SetRepoRoot("/some/path/")
	assert.Equal(t, "/some/path/", repoRoot)
	repoRoot = "" // cleanup
}

func TestSetRepoRoot_WithoutTrailingSlash(t *testing.T) {
	SetRepoRoot("/some/path")
	assert.Equal(t, "/some/path/", repoRoot)
	repoRoot = "" // cleanup
}

// --- getTypeName ---

func TestGetTypeName_Nil(t *testing.T) {
	assert.Equal(t, "", getTypeName(nil, nil))
}

func TestGetTypeName_Ident(t *testing.T) {
	ident := &ast.Ident{Name: "MyType"}
	assert.Equal(t, "MyType", getTypeName(ident, nil))
}

func TestGetTypeName_StarExpr(t *testing.T) {
	star := &ast.StarExpr{X: &ast.Ident{Name: "Foo"}}
	assert.Equal(t, "*Foo", getTypeName(star, nil))
}

func TestGetTypeName_ArrayType(t *testing.T) {
	arr := &ast.ArrayType{Elt: &ast.Ident{Name: "int"}}
	assert.Equal(t, "[]int", getTypeName(arr, nil))
}

func TestGetTypeName_MapType(t *testing.T) {
	m := &ast.MapType{
		Key:   &ast.Ident{Name: "string"},
		Value: &ast.Ident{Name: "int"},
	}
	assert.Equal(t, "map[string]int", getTypeName(m, nil))
}

func TestGetTypeName_InterfaceType(t *testing.T) {
	it := &ast.InterfaceType{Methods: &ast.FieldList{}}
	assert.Equal(t, "interface{}", getTypeName(it, nil))
}

func TestGetTypeName_StructType(t *testing.T) {
	st := &ast.StructType{Fields: &ast.FieldList{}}
	assert.Equal(t, "struct{}", getTypeName(st, nil))
}

func TestGetTypeName_FuncType(t *testing.T) {
	ft := &ast.FuncType{}
	assert.Equal(t, "func", getTypeName(ft, nil))
}

func TestGetTypeName_ChanType_Bidirectional(t *testing.T) {
	ch := &ast.ChanType{
		Dir:   ast.SEND | ast.RECV,
		Value: &ast.Ident{Name: "int"},
	}
	assert.Equal(t, "chan int", getTypeName(ch, nil))
}

func TestGetTypeName_ChanType_Send(t *testing.T) {
	ch := &ast.ChanType{
		Dir:   ast.SEND,
		Value: &ast.Ident{Name: "int"},
	}
	assert.Equal(t, "chan<- int", getTypeName(ch, nil))
}

func TestGetTypeName_ChanType_Recv(t *testing.T) {
	ch := &ast.ChanType{
		Dir:   ast.RECV,
		Value: &ast.Ident{Name: "int"},
	}
	assert.Equal(t, "<-chan int", getTypeName(ch, nil))
}

func TestGetTypeName_IndexExpr(t *testing.T) {
	ie := &ast.IndexExpr{
		X:     &ast.Ident{Name: "List"},
		Index: &ast.Ident{Name: "T"},
	}
	assert.Equal(t, "List[T]", getTypeName(ie, nil))
}

func TestGetTypeName_SelectorExpr_NoInfo(t *testing.T) {
	se := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "pkg"},
		Sel: &ast.Ident{Name: "Type"},
	}
	// With empty info (no entries), falls through to x.Name + "." + t.Sel.Name
	info := &types.Info{
		Uses: make(map[*ast.Ident]types.Object),
	}
	assert.Equal(t, "pkg.Type", getTypeName(se, info))
}

func TestGetTypeName_SelectorExpr_NonIdent(t *testing.T) {
	se := &ast.SelectorExpr{
		X:   &ast.StarExpr{X: &ast.Ident{Name: "pkg"}},
		Sel: &ast.Ident{Name: "Type"},
	}
	// When X is not *ast.Ident, returns just Sel name
	assert.Equal(t, "Type", getTypeName(se, nil))
}

func TestGetTypeName_TypeSpec_WithTypeParams(t *testing.T) {
	ts := &ast.TypeSpec{
		Name: &ast.Ident{Name: "Container"},
		TypeParams: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: "T"}, {Name: "U"}},
				},
			},
		},
		Type: &ast.StructType{Fields: &ast.FieldList{}},
	}
	// TypeSpec with type params returns Name + formatted param list
	result := getTypeName(ts, nil)
	assert.Contains(t, result, "Container")
	assert.Contains(t, result, "T")
	assert.Contains(t, result, "U")
}

func TestGetTypeName_TypeSpec_NoTypeParams(t *testing.T) {
	ts := &ast.TypeSpec{
		Name: &ast.Ident{Name: "Simple"},
		Type: &ast.StructType{Fields: &ast.FieldList{}},
	}
	assert.Equal(t, "Simple", getTypeName(ts, nil))
}

func TestGetTypeName_FieldList(t *testing.T) {
	fl := &ast.FieldList{
		List: []*ast.Field{
			{Names: []*ast.Ident{{Name: "A"}, {Name: "B"}}},
		},
	}
	result := getTypeName(fl, nil)
	assert.Equal(t, "[A, B]", result)
}

func TestGetTypeName_UnknownNode(t *testing.T) {
	// An ast.BadExpr is not handled, should return ""
	bad := &ast.BadExpr{}
	assert.Equal(t, "", getTypeName(bad, nil))
}

func TestGetTypeName_SelectorExpr_WithInfo(t *testing.T) {
	// Parse real code to get proper types.Info
	src := `package test
import "fmt"
var _ = fmt.Println
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)

	conf := types.Config{
		Importer: nil,
		Error:    func(_ error) {},
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	_, _ = conf.Check("test", fset, []*ast.File{file}, info)

	// Find the selector expression fmt.Println
	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			selExpr = se
			return false
		}
		return true
	})

	if selExpr != nil {
		result := getTypeName(selExpr, info)
		// Even if importer is nil, info.ObjectOf may be populated;
		// we just check it doesn't crash
		assert.NotEmpty(t, result)
	}
}

// --- getPosition ---

func TestGetPosition_InvalidPos(t *testing.T) {
	fset := token.NewFileSet()
	result := getPosition(token.NoPos, fset)
	assert.Equal(t, "", result)
}

func TestGetPosition_NilFset(t *testing.T) {
	result := getPosition(token.Pos(1), nil)
	assert.Equal(t, "", result)
}

func TestGetPosition_ValidPos(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
func main() {}
`
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)
	// Use position of the func decl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			result := getPosition(fn.Pos(), fset)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "test.go")
		}
	}
}

func TestGetPosition_StripsRepoRoot(t *testing.T) {
	oldRoot := repoRoot
	defer func() { repoRoot = oldRoot }()

	fset := token.NewFileSet()
	src := `package main
func main() {}
`
	file, err := parser.ParseFile(fset, "/my/project/test.go", src, 0)
	require.NoError(t, err)

	SetRepoRoot("/my/project")
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			result := getPosition(fn.Pos(), fset)
			assert.NotEmpty(t, result)
			assert.NotContains(t, result, "/my/project/")
			assert.Contains(t, result, "test.go")
		}
	}
}

// --- getFuncPosition ---

func TestGetFuncPosition_Nil(t *testing.T) {
	fset := token.NewFileSet()
	assert.Equal(t, "", getFuncPosition(nil, fset))
}

func TestGetFuncPosition_Valid(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
func hello() {}
`
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			result := getFuncPosition(fn, fset)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "test.go")
		}
	}
}

// --- getVarPosition ---

func TestGetVarPosition_Nil(t *testing.T) {
	fset := token.NewFileSet()
	assert.Equal(t, "", getVarPosition(nil, fset))
}

func TestGetVarPosition_Valid(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
var x int
`
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						result := getVarPosition(name, fset)
						assert.NotEmpty(t, result)
						assert.Contains(t, result, "test.go")
					}
				}
			}
		}
	}
}

// --- getComments ---

func TestGetComments_Nil(t *testing.T) {
	assert.Equal(t, "", getComments(nil))
}

func TestGetComments_FuncDecl_WithDoc(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
// Hello is a function.
func Hello() {}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			result := getComments(fn)
			assert.Contains(t, result, "Hello is a function")
		}
	}
}

func TestGetComments_FuncDecl_NoDoc(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
func Hello() {}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			result := getComments(fn)
			assert.Equal(t, "", result)
		}
	}
}

func TestGetComments_GenDecl_WithDoc(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
// MyType is a type.
type MyType struct{}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok {
			result := getComments(gd)
			assert.Contains(t, result, "MyType is a type")
		}
	}
}

func TestGetComments_GenDecl_NoDoc(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
type MyType struct{}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok {
			result := getComments(gd)
			assert.Equal(t, "", result)
		}
	}
}

func TestGetComments_Field_WithDocAndComment(t *testing.T) {
	fset := token.NewFileSet()
	src := `package main
type S struct {
	// Field doc
	Name string // inline comment
}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	ast.Inspect(file, func(n ast.Node) bool {
		if st, ok := n.(*ast.StructType); ok {
			for _, field := range st.Fields.List {
				if len(field.Names) > 0 && field.Names[0].Name == "Name" {
					result := getComments(field)
					assert.Contains(t, result, "Field doc")
					assert.Contains(t, result, "inline comment")
				}
			}
		}
		return true
	})
}

func TestGetComments_Field_NoDocNoComment(t *testing.T) {
	field := &ast.Field{
		Names: []*ast.Ident{{Name: "X"}},
		Type:  &ast.Ident{Name: "int"},
	}
	result := getComments(field)
	assert.Equal(t, "", result)
}

func TestGetComments_ValueSpec_WithDoc(t *testing.T) {
	fset := token.NewFileSet()
	// Use a parenthesized block so the comment attaches to the ValueSpec, not the GenDecl
	src := `package main
var (
	// myVar is important.
	myVar int
)
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					result := getComments(vs)
					assert.Contains(t, result, "myVar is important")
				}
			}
		}
	}
}

func TestGetComments_ValueSpec_NoDoc(t *testing.T) {
	vs := &ast.ValueSpec{
		Names: []*ast.Ident{{Name: "x"}},
		Type:  &ast.Ident{Name: "int"},
	}
	result := getComments(vs)
	assert.Equal(t, "", result)
}

func TestGetComments_UnhandledNodeType(t *testing.T) {
	// ast.BasicLit is not one of the handled types
	bl := &ast.BasicLit{Kind: token.INT, Value: "42"}
	result := getComments(bl)
	assert.Equal(t, "", result)
}

// --- getFieldTag ---

func TestGetFieldTag_Nil(t *testing.T) {
	assert.Equal(t, "", getFieldTag(nil))
}

func TestGetFieldTag_NilTag(t *testing.T) {
	field := &ast.Field{}
	assert.Equal(t, "", getFieldTag(field))
}

func TestGetFieldTag_BacktickTag(t *testing.T) {
	field := &ast.Field{
		Tag: &ast.BasicLit{
			Kind:  token.STRING,
			Value: "`json:\"name\"`",
		},
	}
	result := getFieldTag(field)
	assert.Equal(t, "json:\"name\"", result)
}

func TestGetFieldTag_DoubleQuoteTag(t *testing.T) {
	field := &ast.Field{
		Tag: &ast.BasicLit{
			Kind:  token.STRING,
			Value: "\"json:\\\"name\\\"\"",
		},
	}
	result := getFieldTag(field)
	assert.Equal(t, "json:\\\"name\\\"", result)
}

func TestGetFieldTag_SingleCharTag(t *testing.T) {
	// Tag that is too short to strip
	field := &ast.Field{
		Tag: &ast.BasicLit{
			Kind:  token.STRING,
			Value: "x",
		},
	}
	result := getFieldTag(field)
	assert.Equal(t, "x", result)
}

func TestGetFieldTag_EmptyBacktickTag(t *testing.T) {
	field := &ast.Field{
		Tag: &ast.BasicLit{
			Kind:  token.STRING,
			Value: "``",
		},
	}
	result := getFieldTag(field)
	assert.Equal(t, "", result)
}

// --- isExported ---

func TestIsExported_Empty(t *testing.T) {
	assert.False(t, isExported(""))
}

func TestIsExported_Uppercase(t *testing.T) {
	assert.True(t, isExported("Hello"))
}

func TestIsExported_Lowercase(t *testing.T) {
	assert.False(t, isExported("hello"))
}

func TestIsExported_Underscore(t *testing.T) {
	assert.False(t, isExported("_private"))
}

func TestIsExported_SingleChar(t *testing.T) {
	assert.True(t, isExported("A"))
	assert.False(t, isExported("a"))
}

// --- getScope ---

func TestGetScope_Exported(t *testing.T) {
	assert.Equal(t, defaultScopeExported, getScope("Hello"))
}

func TestGetScope_Unexported(t *testing.T) {
	assert.Equal(t, defaultScopeUnexported, getScope("hello"))
}

// --- getImportPath ---

func TestGetImportPath_Nil(t *testing.T) {
	assert.Equal(t, "", getImportPath(nil))
}

func TestGetImportPath_NilPath(t *testing.T) {
	imp := &ast.ImportSpec{}
	assert.Equal(t, "", getImportPath(imp))
}

func TestGetImportPath_Valid(t *testing.T) {
	imp := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"fmt"`,
		},
	}
	assert.Equal(t, "fmt", getImportPath(imp))
}

func TestGetImportPath_LongPath(t *testing.T) {
	imp := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"github.com/foo/bar"`,
		},
	}
	assert.Equal(t, "github.com/foo/bar", getImportPath(imp))
}

// --- getImportAlias ---

func TestGetImportAlias_Nil(t *testing.T) {
	assert.Equal(t, "", getImportAlias(nil))
}

func TestGetImportAlias_NoAlias(t *testing.T) {
	imp := &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`},
	}
	assert.Equal(t, "", getImportAlias(imp))
}

func TestGetImportAlias_WithAlias(t *testing.T) {
	imp := &ast.ImportSpec{
		Name: &ast.Ident{Name: "f"},
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`},
	}
	assert.Equal(t, "f", getImportAlias(imp))
}

func TestGetImportAlias_DotImport(t *testing.T) {
	imp := &ast.ImportSpec{
		Name: &ast.Ident{Name: "."},
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`},
	}
	assert.Equal(t, ".", getImportAlias(imp))
}

func TestGetImportAlias_BlankImport(t *testing.T) {
	imp := &ast.ImportSpec{
		Name: &ast.Ident{Name: "_"},
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"net/http/pprof"`},
	}
	assert.Equal(t, "_", getImportAlias(imp))
}
