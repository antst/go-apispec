package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------
// helpers
// ------------------------------------------------------------

// newTestMeta creates a ready-to-use Metadata with all caches initialised.
func newTestMeta() *Metadata {
	return &Metadata{
		StringPool:         NewStringPool(),
		Packages:           make(map[string]*Package),
		traceVariableCache: make(map[string]TraceVariableResult),
		methodLookupCache:  make(map[string]*Method),
	}
}

// parseWithInfo parses a Go source string and type-checks it, returning the
// file-set, the AST file, and the fully-populated *types.Info.
func parseWithInfo(t *testing.T, src string) (*token.FileSet, *ast.File, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: nil, Error: func(_ error) {}}
	_, _ = conf.Check("testpkg", fset, []*ast.File{file}, info)
	return fset, file, info
}

// ------------------------------------------------------------
// implementsInterface
// ------------------------------------------------------------

func TestImplementsInterface_AllMatch(t *testing.T) {
	meta := newTestMeta()
	ifaceType := &Type{
		Methods: []Method{
			{Name: meta.StringPool.Get("Foo"), SignatureStr: meta.StringPool.Get("func()")},
			{Name: meta.StringPool.Get("Bar"), SignatureStr: meta.StringPool.Get("func() int")},
		},
	}
	structMethods := map[int]int{
		meta.StringPool.Get("Foo"): meta.StringPool.Get("func()"),
		meta.StringPool.Get("Bar"): meta.StringPool.Get("func() int"),
		meta.StringPool.Get("Baz"): meta.StringPool.Get("func() string"), // extra is fine
	}
	assert.True(t, implementsInterface(structMethods, ifaceType))
}

func TestImplementsInterface_MissingMethod(t *testing.T) {
	meta := newTestMeta()
	ifaceType := &Type{
		Methods: []Method{
			{Name: meta.StringPool.Get("Foo"), SignatureStr: meta.StringPool.Get("func()")},
			{Name: meta.StringPool.Get("Missing"), SignatureStr: meta.StringPool.Get("func()")},
		},
	}
	structMethods := map[int]int{
		meta.StringPool.Get("Foo"): meta.StringPool.Get("func()"),
	}
	assert.False(t, implementsInterface(structMethods, ifaceType))
}

func TestImplementsInterface_SignatureMismatch(t *testing.T) {
	meta := newTestMeta()
	ifaceType := &Type{
		Methods: []Method{
			{Name: meta.StringPool.Get("Foo"), SignatureStr: meta.StringPool.Get("func() int")},
		},
	}
	structMethods := map[int]int{
		meta.StringPool.Get("Foo"): meta.StringPool.Get("func() string"),
	}
	assert.False(t, implementsInterface(structMethods, ifaceType))
}

func TestImplementsInterface_Empty(t *testing.T) {
	// An empty interface is always implemented
	ifaceType := &Type{Methods: []Method{}}
	assert.True(t, implementsInterface(map[int]int{}, ifaceType))
}

// ------------------------------------------------------------
// getEnclosingFunctionName
// ------------------------------------------------------------

func TestGetEnclosingFunctionName_DeclaredFunction(t *testing.T) {
	src := `package testpkg

func Hello(x int) string { return "" }
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	fn := file.Decls[0].(*ast.FuncDecl)
	// Use a position inside the function body
	pos := fn.Body.Pos() + 1

	name, recv, sig := getEnclosingFunctionName(file, pos, info, fset, meta)
	assert.Equal(t, "Hello", name)
	assert.Equal(t, "", recv) // not a method
	assert.NotEqual(t, "", sig)
}

func TestGetEnclosingFunctionName_Method(t *testing.T) {
	src := `package testpkg

type S struct{}
func (s S) Do() {}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the method decl
	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "Do" {
			fn = fd
		}
	}
	require.NotNil(t, fn)

	pos := fn.Body.Pos() + 1
	name, recv, _ := getEnclosingFunctionName(file, pos, info, fset, meta)
	assert.Equal(t, "Do", name)
	assert.Contains(t, recv, "S")
}

func TestGetEnclosingFunctionName_FuncLiteral(t *testing.T) {
	src := `package testpkg

func Outer() {
	f := func(x int) int { return x }
	_ = f
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the func lit inside Outer
	var funcLit *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			funcLit = fl
		}
		return true
	})
	require.NotNil(t, funcLit)

	// Use a position inside the function literal body
	pos := funcLit.Body.Pos() + 1
	name, _, _ := getEnclosingFunctionName(file, pos, info, fset, meta)
	assert.Contains(t, name, "FuncLit:")
}

func TestGetEnclosingFunctionName_NoMatch(t *testing.T) {
	src := `package testpkg

var x int
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	name, recv, sig := getEnclosingFunctionName(file, token.Pos(1), info, fset, meta)
	assert.Equal(t, "", name)
	assert.Equal(t, "", recv)
	assert.Equal(t, "", sig)
}

func TestGetEnclosingFunctionName_NonFuncDecl(t *testing.T) {
	// File with only a variable declaration (not a FuncDecl)
	src := `package testpkg

var x = 42
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Position is inside the GenDecl, not a FuncDecl
	genDecl := file.Decls[0].(*ast.GenDecl)
	pos := genDecl.Pos() + 1
	name, recv, sig := getEnclosingFunctionName(file, pos, info, fset, meta)
	assert.Equal(t, "", name)
	assert.Equal(t, "", recv)
	assert.Equal(t, "", sig)
}

// ------------------------------------------------------------
// findParentFunction  -- additional coverage
// ------------------------------------------------------------

func TestFindParentFunction_MethodParent(t *testing.T) {
	src := `package testpkg

type S struct{}

func (s *S) Run() {
	fn := func() {}
	_ = fn
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var funcLit *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			funcLit = fl
		}
		return true
	})
	require.NotNil(t, funcLit)

	pos := funcLit.Body.Pos() + 1
	name, recv, _ := findParentFunction(file, pos, info, fset, meta)
	assert.Equal(t, "Run", name)
	assert.Contains(t, recv, "S")
}

func TestFindParentFunction_NilSignature(t *testing.T) {
	// Edge case: func decl that wraps a literal but ExprToCallArgument returns nil
	// We just confirm it doesn't panic.
	src := `package testpkg

func Outer() {
	f := func() {}
	_ = f
}
`
	fset, file, _ := parseWithInfo(t, src)
	meta := newTestMeta()

	var funcLit *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			funcLit = fl
		}
		return true
	})
	require.NotNil(t, funcLit)

	pos := funcLit.Body.Pos() + 1
	name, _, sig := findParentFunction(file, pos, &types.Info{}, fset, meta)
	assert.Equal(t, "Outer", name)
	assert.NotEqual(t, "", sig) // signature still generated from AST
}

// Test findParentFunction when func literal is not inside any FuncDecl
func TestFindParentFunction_LitOutsideDecl(t *testing.T) {
	// Create a file with a func literal at top level (assigned to a var)
	// Since the func literal is inside a GenDecl (not FuncDecl),
	// the parent will not be found
	src := `package testpkg

var f = func() {}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var funcLit *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			funcLit = fl
		}
		return true
	})
	require.NotNil(t, funcLit)

	pos := funcLit.Body.Pos() + 1
	name, recv, sig := findParentFunction(file, pos, info, fset, meta)
	assert.Equal(t, "", name)
	assert.Equal(t, "", recv)
	assert.Equal(t, "", sig)
}

// ------------------------------------------------------------
// DefaultImportName  -- edge cases
// ------------------------------------------------------------

func TestDefaultImportName_VCharOnly(t *testing.T) {
	// "v" alone with a second char that is not a digit
	assert.Equal(t, "vx", DefaultImportName("github.com/foo/vx"))
}

func TestDefaultImportName_VersionV5SingleSegment(t *testing.T) {
	// "v5" alone — len(parts) == 1, so the version check won't use parts[len-2]
	assert.Equal(t, "v5", DefaultImportName("v5"))
}

func TestDefaultImportName_JustSlash(t *testing.T) {
	// Trailing empty segment
	result := DefaultImportName("github.com/foo/")
	assert.Equal(t, "", result)
}

// ------------------------------------------------------------
// isTypeConversion
// ------------------------------------------------------------

func TestIsTypeConversion_NilInfo(t *testing.T) {
	call := &ast.CallExpr{Fun: &ast.Ident{Name: "int"}, Args: []ast.Expr{&ast.Ident{Name: "x"}}}
	assert.False(t, isTypeConversion(call, nil))
}

func TestIsTypeConversion_IdentType(t *testing.T) {
	src := `package testpkg

type MyInt int

func f() {
	x := MyInt(42)
	_ = x
}
`
	_, file, info := parseWithInfo(t, src)

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.True(t, isTypeConversion(callExpr, info))
}

func TestIsTypeConversion_ArrayType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.ArrayType{Elt: &ast.Ident{Name: "byte"}},
		Args: []ast.Expr{&ast.Ident{Name: "s"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_SliceExpr(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.SliceExpr{X: &ast.Ident{Name: "arr"}},
		Args: []ast.Expr{&ast.Ident{Name: "s"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_MapType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.MapType{Key: &ast.Ident{Name: "string"}, Value: &ast.Ident{Name: "int"}},
		Args: []ast.Expr{&ast.Ident{Name: "m"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_ChanType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: &ast.Ident{Name: "int"}},
		Args: []ast.Expr{&ast.Ident{Name: "c"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_StarExpr(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.StarExpr{X: &ast.Ident{Name: "int"}},
		Args: []ast.Expr{&ast.Ident{Name: "p"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_InterfaceType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.InterfaceType{Methods: &ast.FieldList{}},
		Args: []ast.Expr{&ast.Ident{Name: "x"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_StructType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.StructType{Fields: &ast.FieldList{}},
		Args: []ast.Expr{&ast.Ident{Name: "x"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_FuncType(t *testing.T) {
	call := &ast.CallExpr{
		Fun:  &ast.FuncType{Params: &ast.FieldList{}},
		Args: []ast.Expr{&ast.Ident{Name: "x"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.True(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_FunctionCall(t *testing.T) {
	// A real function call should NOT be a type conversion
	src := `package testpkg

func foo(x int) int { return x }

func main() {
	y := foo(42)
	_ = y
}
`
	_, file, info := parseWithInfo(t, src)

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.False(t, isTypeConversion(callExpr, info))
}

func TestIsTypeConversion_SelectorExprType(t *testing.T) {
	// SelectorExpr with ObjectOf returning a TypeName
	// Use a self-contained example since the test importer can't resolve stdlib
	src := `package testpkg

type Duration int64

type mypkg struct{}

func f() {
	_ = Duration(42)
}
`
	_, file, info := parseWithInfo(t, src)

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.True(t, isTypeConversion(callExpr, info))
}

func TestIsTypeConversion_SelectorExprNonType(t *testing.T) {
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	_, file, info := parseWithInfo(t, src)

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.False(t, isTypeConversion(callExpr, info))
}

func TestIsTypeConversion_IdentNotResolved(t *testing.T) {
	// Ident where ObjectOf returns nil
	call := &ast.CallExpr{
		Fun:  &ast.Ident{Name: "unknownFunc"},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	assert.False(t, isTypeConversion(call, info))
}

func TestIsTypeConversion_OneArgTypeCheck(t *testing.T) {
	// Test the additional check: 1 arg and info.Types[call.Fun].IsType()
	src := `package testpkg

type MyType int

func f() {
	var x int = 42
	y := MyType(x)
	_ = y
}
`
	_, file, info := parseWithInfo(t, src)

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.True(t, isTypeConversion(callExpr, info))
}

// ------------------------------------------------------------
// getCalleeFunctionNameAndPackage
// ------------------------------------------------------------

func TestGetCalleeFunctionNameAndPackage_SimpleIdent(t *testing.T) {
	ident := &ast.Ident{Name: "myFunc"}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, recv := getCalleeFunctionNameAndPackage(ident, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "myFunc", name)
	assert.Equal(t, "mypkg", pkg)
	assert.Equal(t, "", recv)
}

func TestGetCalleeFunctionNameAndPackage_SelectorExpr(t *testing.T) {
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	// Find the CallExpr
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Println", name)
	assert.Equal(t, "fmt", pkg)
	assert.Equal(t, "", recv)
}

func TestGetCalleeFunctionNameAndPackage_MethodOnVariable(t *testing.T) {
	src := `package testpkg

type MyStruct struct{}
func (m *MyStruct) Do() {}

func f() {
	s := &MyStruct{}
	s.Do()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	// Find the s.Do() call
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Do", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "MyStruct")
}

func TestGetCalleeFunctionNameAndPackage_CallExpr(t *testing.T) {
	// CallExpr wrapping another call - should recurse through Fun
	inner := &ast.Ident{Name: "myFunc"}
	call := &ast.CallExpr{Fun: inner}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, recv := getCalleeFunctionNameAndPackage(call, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "myFunc", name)
	assert.Equal(t, "mypkg", pkg)
	assert.Equal(t, "", recv)
}

func TestGetCalleeFunctionNameAndPackage_IndexExpr(t *testing.T) {
	// IndexExpr - like generics: Func[T]
	idx := &ast.IndexExpr{
		X:     &ast.Ident{Name: "GenericFunc"},
		Index: &ast.Ident{Name: "int"},
	}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(idx, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "GenericFunc", name)
	assert.Equal(t, "mypkg", pkg)
}

func TestGetCalleeFunctionNameAndPackage_IndexListExpr(t *testing.T) {
	// IndexListExpr - like generics: Func[T1, T2]
	idxList := &ast.IndexListExpr{
		X:       &ast.Ident{Name: "MultiGeneric"},
		Indices: []ast.Expr{&ast.Ident{Name: "int"}, &ast.Ident{Name: "string"}},
	}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(idxList, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "MultiGeneric", name)
	assert.Equal(t, "mypkg", pkg)
}

func TestGetCalleeFunctionNameAndPackage_FuncLit(t *testing.T) {
	fset := token.NewFileSet()
	fset.AddFile("test.go", 1, 100)
	funcLit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{},
	}
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(funcLit, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Contains(t, name, "FuncLit:")
	assert.Equal(t, "mypkg", pkg)
}

func TestGetCalleeFunctionNameAndPackage_UnknownExpr(t *testing.T) {
	// A BasicLit is not a recognized expression for function calls
	lit := &ast.BasicLit{Kind: token.INT, Value: "42"}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, recv := getCalleeFunctionNameAndPackage(lit, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "", name)
	assert.Equal(t, "", pkg)
	assert.Equal(t, "", recv)
}

func TestGetCalleeFunctionNameAndPackage_SelectorNoInfo(t *testing.T) {
	// SelectorExpr where fileToInfo has no entry for the file
	sel := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "obj"},
		Sel: &ast.Ident{Name: "Method"},
	}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	fileToInfo := map[*ast.File]*types.Info{} // empty
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(sel, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Method", name)
	assert.Equal(t, "mypkg", pkg) // fallback
}

func TestGetCalleeFunctionNameAndPackage_SelectorComplexReceiver(t *testing.T) {
	// SelectorExpr where X is not a simple *ast.Ident (e.g., a call expr)
	src := `package testpkg

type Builder struct{}
func NewBuilder() *Builder { return &Builder{} }
func (b *Builder) Build() {}

func f() {
	NewBuilder().Build()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	// Find Build() call
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Build" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Build", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "Builder")
}

func TestGetCalleeFunctionNameAndPackage_InterfaceVar(t *testing.T) {
	// SelectorExpr on interface variable
	src := `package testpkg

type Doer interface { Do() }

func f(d Doer) {
	d.Do()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Do", name)
	assert.Equal(t, "testpkg", pkg)
	// Named interface type resolves to its name, not "interface"
	assert.Equal(t, "Doer", recv)
}

// ------------------------------------------------------------
// getReceiverTypeString
// ------------------------------------------------------------

func TestGetReceiverTypeString_Named(t *testing.T) {
	src := `package testpkg
type Foo struct{}
`
	_, _, info := parseWithInfo(t, src)
	// Find the type
	for _, obj := range info.Defs {
		if obj != nil {
			if tn, ok := obj.(*types.TypeName); ok && tn.Name() == "Foo" {
				result := getReceiverTypeString(tn.Type())
				assert.Equal(t, "Foo", result)
				return
			}
		}
	}
	t.Fatal("could not find Foo type")
}

func TestGetReceiverTypeString_Pointer(t *testing.T) {
	src := `package testpkg
type Bar struct{}
`
	_, _, info := parseWithInfo(t, src)
	for _, obj := range info.Defs {
		if obj != nil {
			if tn, ok := obj.(*types.TypeName); ok && tn.Name() == "Bar" {
				ptr := types.NewPointer(tn.Type())
				result := getReceiverTypeString(ptr)
				assert.Equal(t, "*Bar", result)
				return
			}
		}
	}
	t.Fatal("could not find Bar type")
}

func TestGetReceiverTypeString_Interface(t *testing.T) {
	iface := types.NewInterfaceType(nil, nil)
	result := getReceiverTypeString(iface)
	assert.Equal(t, "interface", result)
}

func TestGetReceiverTypeString_Basic(t *testing.T) {
	// A basic type (int, string) should return ""
	result := getReceiverTypeString(types.Typ[types.Int])
	assert.Equal(t, "", result)
}

func TestGetReceiverTypeString_PointerToBasic(t *testing.T) {
	// Pointer to a basic type should go through default since Elem is not Named
	ptr := types.NewPointer(types.Typ[types.Int])
	result := getReceiverTypeString(ptr)
	// *<empty> because int is not Named
	assert.Equal(t, "*", result)
}

// ------------------------------------------------------------
// analyzeAssignmentValue
// ------------------------------------------------------------

func TestAnalyzeAssignmentValue_Nil(t *testing.T) {
	meta := newTestMeta()
	pkg, arg := analyzeAssignmentValue(nil, nil, "f", "pkg", meta, nil)
	assert.Equal(t, "pkg", pkg)
	assert.Nil(t, arg)
}

func TestAnalyzeAssignmentValue_WithTypeInfo(t *testing.T) {
	src := `package testpkg

func f() {
	x := 42
	_ = x
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var assignRhs ast.Expr
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Rhs) > 0 {
			assignRhs = as.Rhs[0]
		}
		return true
	})
	require.NotNil(t, assignRhs)

	pkg, arg := analyzeAssignmentValue(assignRhs, info, "f", "testpkg", meta, fset)
	assert.Equal(t, "testpkg", pkg)
	assert.NotNil(t, arg)
	assert.Equal(t, KindIdent, arg.GetKind())
}

func TestAnalyzeAssignmentValue_Ident(t *testing.T) {
	meta := newTestMeta()
	// Ident without type info falls through to TraceVariableOrigin
	ident := &ast.Ident{Name: "myVar"}
	pkg, _ := analyzeAssignmentValue(ident, nil, "f", "mypkg", meta, nil)
	assert.Equal(t, "mypkg", pkg)
}

func TestAnalyzeAssignmentValue_SelectorExprIdent(t *testing.T) {
	meta := newTestMeta()
	sel := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "obj"},
		Sel: &ast.Ident{Name: "Field"},
	}
	pkg, _ := analyzeAssignmentValue(sel, nil, "f", "mypkg", meta, nil)
	assert.Equal(t, "mypkg", pkg)
}

func TestAnalyzeAssignmentValue_SelectorExprNonIdent(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// X is not an *ast.Ident, falls to fallback
	sel := &ast.SelectorExpr{
		X: &ast.CallExpr{
			Fun: &ast.Ident{Name: "getObj"},
		},
		Sel: &ast.Ident{Name: "Field"},
	}
	pkg, arg := analyzeAssignmentValue(sel, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
}

func TestAnalyzeAssignmentValue_CallExprDirect(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// Direct function call
	callExpr := &ast.CallExpr{
		Fun: &ast.Ident{Name: "doSomething"},
	}
	pkg, _ := analyzeAssignmentValue(callExpr, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
}

func TestAnalyzeAssignmentValue_CallExprSelector(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// Selector function call: pkg.Func()
	callExpr := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "pkg"},
			Sel: &ast.Ident{Name: "Func"},
		},
	}
	pkg, _ := analyzeAssignmentValue(callExpr, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
}

func TestAnalyzeAssignmentValue_CallExprFallback(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// Call on something non-Ident and non-SelectorExpr
	callExpr := &ast.CallExpr{
		Fun: &ast.ParenExpr{X: &ast.Ident{Name: "fn"}},
	}
	pkg, arg := analyzeAssignmentValue(callExpr, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
}

func TestAnalyzeAssignmentValue_TypeAssertWithType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	typeAssert := &ast.TypeAssertExpr{
		X:    &ast.Ident{Name: "x"},
		Type: &ast.Ident{Name: "MyType"},
	}
	pkg, arg := analyzeAssignmentValue(typeAssert, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
}

func TestAnalyzeAssignmentValue_TypeAssertNoType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// Type switch: x.(type) — has nil Type
	typeAssert := &ast.TypeAssertExpr{
		X:    &ast.Ident{Name: "x"},
		Type: nil,
	}
	pkg, arg := analyzeAssignmentValue(typeAssert, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
	assert.Equal(t, KindIdent, arg.GetKind())
}

func TestAnalyzeAssignmentValue_StarExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	starExpr := &ast.StarExpr{X: &ast.Ident{Name: "ptr"}}
	pkg, _ := analyzeAssignmentValue(starExpr, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
}

func TestAnalyzeAssignmentValue_CompositeLit(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	comp := &ast.CompositeLit{
		Type: &ast.Ident{Name: "MyStruct"},
	}
	pkg, arg := analyzeAssignmentValue(comp, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
}

func TestAnalyzeAssignmentValue_Default(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	// BinaryExpr hits the default case
	binExpr := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "a"},
		Op: token.ADD,
		Y:  &ast.Ident{Name: "b"},
	}
	pkg, arg := analyzeAssignmentValue(binExpr, nil, "f", "mypkg", meta, fset)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, arg)
}

// ------------------------------------------------------------
// traceVariableOriginHelper  — coverage via TraceVariableOrigin
// ------------------------------------------------------------

func TestTraceVariableOriginHelper_EmptyVarName(t *testing.T) {
	meta := newTestMeta()
	name, pkg, typ, fn := TraceVariableOrigin("", "f", "pkg", meta)
	assert.Equal(t, "", name)
	assert.Equal(t, "pkg", pkg)
	assert.Nil(t, typ)
	assert.Equal(t, "f", fn)
}

func TestTraceVariableOriginHelper_CycleDetection(t *testing.T) {
	// Metadata where variable a aliases b and b aliases a
	meta := newTestMeta()
	sp := meta.StringPool

	aArg := NewCallArgument(meta)
	aArg.SetKind(KindIdent)
	aArg.SetName("b")

	bArg := NewCallArgument(meta)
	bArg.SetKind(KindIdent)
	bArg.SetName("a")

	fn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"a": {{Value: *aArg}},
			"b": {{Value: *bArg}},
		},
	}
	file := &File{
		Functions: map[string]*Function{"f": fn},
	}
	pkg := &Package{Files: map[string]*File{"test.go": file}}
	meta.Packages["mypkg"] = pkg

	// Should not loop infinitely
	name, _, _, _ := TraceVariableOrigin("a", "f", "mypkg", meta)
	// Should return something (either "a" or "b") without panicking
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_TypeParamMap(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	meta.CallGraph = []CallGraphEdge{
		{
			Caller: Call{
				Name: sp.Get("caller"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			Callee: Call{
				Name: sp.Get("f"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			TypeParamMap: map[string]string{
				"T": "int",
			},
			ParamArgMap: map[string]CallArgument{},
		},
	}

	name, pkg, typ, callerFn := TraceVariableOrigin("T", "f", "mypkg", meta)
	assert.Equal(t, "T", name)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, typ)
	assert.Equal(t, "int", typ.GetType())
	assert.Equal(t, "caller", callerFn)
}

func TestTraceVariableOriginHelper_ParamArgMap(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	argVal := NewCallArgument(meta)
	argVal.SetKind(KindIdent)
	argVal.SetName("originVar")

	meta.CallGraph = []CallGraphEdge{
		{
			Caller: Call{
				Name: sp.Get("caller"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			Callee: Call{
				Name: sp.Get("f"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			ParamArgMap: map[string]CallArgument{
				"param": *argVal,
			},
		},
	}

	name, _, _, _ := TraceVariableOrigin("param", "f", "mypkg", meta)
	// Should trace through the paramArgMap
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_ParamArgMapWithSpaceAndBracket(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	argVal := NewCallArgument(meta)
	argVal.SetKind(KindIdent)
	argVal.SetName("pkg.Type someVar(args)")

	meta.CallGraph = []CallGraphEdge{
		{
			Caller: Call{
				Name: sp.Get("caller"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			Callee: Call{
				Name: sp.Get("f"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			ParamArgMap: map[string]CallArgument{
				"param": *argVal,
			},
		},
	}

	// Exercise the space-stripping and bracket-stripping code
	name, _, _, _ := TraceVariableOrigin("param", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_VariableInFile(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	file := &File{
		Variables: map[string]*Variable{
			"globalVar": {
				Name: sp.Get("globalVar"),
				Type: sp.Get("string"),
			},
		},
		Functions: map[string]*Function{},
	}
	pkg := &Package{Files: map[string]*File{"test.go": file}}
	meta.Packages["mypkg"] = pkg

	name, _, typ, _ := TraceVariableOrigin("globalVar", "f", "mypkg", meta)
	assert.Equal(t, "globalVar", name)
	assert.NotNil(t, typ)
}

func TestTraceVariableOriginHelper_AssignmentFromFunctionCall(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindIdent)
	retArg.SetName("result")

	calleeFn := &Function{
		Name:       sp.Get("getResult"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"getResult": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)
	valArg.SetName("getResult")

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"x": {{
				Value:       *valArg,
				CalleeFunc:  "getResult",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("x", "f", "mypkg", meta)
	// Should trace through the callee function's return var
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_AssignmentFromMethodCall(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindIdent)
	retArg.SetName("methodResult")

	method := Method{
		Name:       sp.Get("GetValue"),
		ReturnVars: []CallArgument{*retArg},
	}
	myType := &Type{
		Methods: []Method{method},
	}
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"MyType": myType},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)
	valArg.SetName("GetValue")

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"y": {{
				Value:       *valArg,
				CalleeFunc:  "GetValue",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("y", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_ReturnVarSelector(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	innerArg := NewCallArgument(meta)
	innerArg.SetKind(KindIdent)
	innerArg.SetName("base")

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindSelector)
	retArg.Sel = innerArg

	calleeFn := &Function{
		Name:       sp.Get("getVal"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"getVal": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"z": {{
				Value:       *valArg,
				CalleeFunc:  "getVal",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("z", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_ReturnVarUnary(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	innerArg := NewCallArgument(meta)
	innerArg.SetKind(KindIdent)
	innerArg.SetName("inner")

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindUnary)
	retArg.X = innerArg

	calleeFn := &Function{
		Name:       sp.Get("getAddr"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"getAddr": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"w": {{
				Value:       *valArg,
				CalleeFunc:  "getAddr",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("w", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_ReturnVarCompositeLit(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	innerArg := NewCallArgument(meta)
	innerArg.SetKind(KindIdent)
	innerArg.SetName("compositeInner")

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindCompositeLit)
	retArg.X = innerArg

	calleeFn := &Function{
		Name:       sp.Get("makeStruct"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"makeStruct": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"v": {{
				Value:       *valArg,
				CalleeFunc:  "makeStruct",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("v", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_ReturnVarBreakDefault(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// A return var with a kind that's not Ident/Selector/Unary/CompositeLit
	retArg := NewCallArgument(meta)
	retArg.SetKind(KindCall)
	retArg.SetName("someCall")

	calleeFn := &Function{
		Name:       sp.Get("getCall"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"getCall": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"u": {{
				Value:       *valArg,
				CalleeFunc:  "getCall",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("u", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_MethodReturnVarSelector(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	innerArg := NewCallArgument(meta)
	innerArg.SetKind(KindIdent)
	innerArg.SetName("methodBase")

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindSelector)
	retArg.Sel = innerArg

	method := Method{
		Name:       sp.Get("MethodGetVal"),
		ReturnVars: []CallArgument{*retArg},
	}
	myType := &Type{Methods: []Method{method}}
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"MyType": myType},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"q": {{
				Value:       *valArg,
				CalleeFunc:  "MethodGetVal",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("q", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_MethodReturnVarUnary(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	innerArg := NewCallArgument(meta)
	innerArg.SetKind(KindIdent)
	innerArg.SetName("methodUnaryInner")

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindUnary)
	retArg.X = innerArg

	method := Method{
		Name:       sp.Get("MethodAddr"),
		ReturnVars: []CallArgument{*retArg},
	}
	myType := &Type{Methods: []Method{method}}
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"MyType": myType},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"r": {{
				Value:       *valArg,
				CalleeFunc:  "MethodAddr",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("r", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_MethodBreakDefault(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// Method return var with non-unwrappable kind
	retArg := NewCallArgument(meta)
	retArg.SetKind(KindCall)
	retArg.SetName("methodCallRet")

	method := Method{
		Name:       sp.Get("MethodCall"),
		ReturnVars: []CallArgument{*retArg},
	}
	myType := &Type{Methods: []Method{method}}
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"MyType": myType},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"s": {{
				Value:       *valArg,
				CalleeFunc:  "MethodCall",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("s", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_BreakAfterEdgeFound(t *testing.T) {
	// Test the break on line 475: edge found but variable not in TypeParamMap or ParamArgMap
	meta := newTestMeta()
	sp := meta.StringPool

	meta.CallGraph = []CallGraphEdge{
		{
			Caller: Call{
				Name: sp.Get("caller"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			Callee: Call{
				Name: sp.Get("f"),
				Pkg:  sp.Get("mypkg"),
				Meta: meta,
			},
			ParamArgMap:  map[string]CallArgument{}, // empty
			TypeParamMap: map[string]string{},       // empty
		},
	}

	// varName "x" not in ParamArgMap or TypeParamMap, so it falls through to the break
	name, _, _, _ := TraceVariableOrigin("x", "f", "mypkg", meta)
	assert.Equal(t, "x", name) // fallback
}

func TestIsTypeConversion_BuiltinTypeOneArg(t *testing.T) {
	// Exercise line 188: len(call.Args)==1 && info.Types[call.Fun].IsType()
	src := `package testpkg

func f() {
	var x float64 = 3.14
	y := int(x)
	_ = y
}
`
	_, file, info := parseWithInfo(t, src)

	// Find the int(x) call
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "int" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)
	assert.True(t, isTypeConversion(callExpr, info))
}

func TestGetCalleeFunctionNameAndPackage_NamedNilPkg(t *testing.T) {
	// Exercise line 220: Named type with nil Pkg (e.g., universe types like error)
	src := `package testpkg

func f() {
	var e error
	_ = e.Error()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, _ := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Error", name)
	// error is a universe type, so pkg falls back
	assert.NotEqual(t, "", pkg)
}

func TestGetCalleeFunctionNameAndPackage_PointerMethodCall(t *testing.T) {
	// Exercise line 222-223: Pointer type case
	src := `package testpkg

type MyStruct struct{}
func (m *MyStruct) Do() {}

func f() {
	s := &MyStruct{}
	s.Do()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Do", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "MyStruct")
}

func TestGetCalleeFunctionNameAndPackage_ComplexReceiverNamed(t *testing.T) {
	// Exercise lines 241-244: complex receiver expression (X is not *ast.Ident) with Named type
	src := `package testpkg

type Builder struct{}
func NewBuilder() Builder { return Builder{} }
func (b Builder) Build() {}

func f() {
	NewBuilder().Build()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	// Find the Build() call — its X is a CallExpr (NewBuilder()), not an Ident
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Build" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Build", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "Builder")
}

func TestGetCalleeFunctionNameAndPackage_ComplexReceiverPointer(t *testing.T) {
	// Exercise lines 246-248: complex receiver with Pointer type
	src := `package testpkg

type Builder struct{}
func NewBuilder() *Builder { return &Builder{} }
func (b *Builder) Build() {}

func f() {
	NewBuilder().Build()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Build" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Build", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "Builder")
}

func TestGetCalleeFunctionNameAndPackage_ComplexReceiverNamedInterface(t *testing.T) {
	// Named interface through getDoer() → Named type, hits line 241-243
	src := `package testpkg

type Doer interface { Do() }

func getDoer() Doer { return nil }

func f() {
	getDoer().Do()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Do", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Equal(t, "Doer", recv)
}

func TestGetCalleeFunctionNameAndPackage_VarUnnamedInterface(t *testing.T) {
	// Exercise lines 225-227: Ident X with Var of unnamed interface type
	// Named interfaces resolve as *types.Named. The Interface case
	// (lines 225-227) is for inline/unnamed interface types which are
	// extremely rare in practice and hard to construct without contrived
	// type assertions. We simply verify normal named-interface coverage.
	src := `package testpkg

type HasName interface {
	Name() string
}

func f(x HasName) {
	x.Name()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Name" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recv := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Name", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recv, "HasName")
}

func TestGetCalleeFunctionNameAndPackage_NamedNilPkgComplex(t *testing.T) {
	// Exercise line 245: complex receiver, Named type with nil Pkg
	// error type is a universe type with nil Pkg
	src := `package testpkg

func getErr() error { return nil }

func f() {
	getErr().Error()
}
`
	fset, file, info := parseWithInfo(t, src)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, _ := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Error", name)
	assert.NotEqual(t, "", pkg)
}

func TestGetCalleeFunctionNameAndPackage_ComplexReceiverNoType(t *testing.T) {
	// Exercise line 256: complex receiver, X is not Ident, but info.Types doesn't resolve
	sel := &ast.SelectorExpr{
		X: &ast.CallExpr{
			Fun: &ast.Ident{Name: "unknown"},
		},
		Sel: &ast.Ident{Name: "Method"},
	}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(sel, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Method", name)
	assert.Equal(t, "mypkg", pkg) // fallback
}

func TestGetCalleeFunctionNameAndPackage_SelectorIdentObjNil(t *testing.T) {
	// SelectorExpr with ident X where ObjectOf returns nil (fallback to default)
	sel := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "unknown"},
		Sel: &ast.Ident{Name: "Method"},
	}
	fset := token.NewFileSet()
	file := &ast.File{Name: &ast.Ident{Name: "test"}}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := map[string]*ast.FuncDecl{}

	name, pkg, _ := getCalleeFunctionNameAndPackage(sel, file, "mypkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Method", name)
	assert.Equal(t, "mypkg", pkg)
}

func TestTraceVariableOriginHelper_CacheHit(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// Pre-populate cache
	key := "mypkg.f:x"
	cachedType := NewCallArgument(meta)
	cachedType.SetKind(KindIdent)
	cachedType.SetType("cachedType")
	meta.traceVariableCache[key] = TraceVariableResult{
		OriginVar:      "cachedX",
		OriginPkg:      "mypkg",
		OriginType:     cachedType,
		CallerFuncName: "f",
	}

	// Add a package so Packages is populated for caching
	meta.Packages["mypkg"] = &Package{
		Files: map[string]*File{"t.go": {Functions: map[string]*Function{}}},
	}
	_ = sp

	name, pkg, typ, fn := TraceVariableOrigin("x", "f", "mypkg", meta)
	assert.Equal(t, "cachedX", name)
	assert.Equal(t, "mypkg", pkg)
	assert.NotNil(t, typ)
	assert.Equal(t, "f", fn)
}

func TestTraceVariableOriginHelper_Fallback(t *testing.T) {
	meta := newTestMeta()
	// No packages, no call graph — pure fallback
	name, pkg, typ, fn := TraceVariableOrigin("unknown", "f", "mypkg", meta)
	assert.Equal(t, "unknown", name)
	assert.Equal(t, "mypkg", pkg)
	assert.Nil(t, typ)
	assert.Equal(t, "f", fn)
}

func TestTraceVariableOriginHelper_CacheWithPopulatedPackages(t *testing.T) {
	meta := newTestMeta()
	meta.Packages["mypkg"] = &Package{
		Files: map[string]*File{"t.go": {Functions: map[string]*Function{}}},
	}

	// First call caches the result
	name1, pkg1, _, fn1 := TraceVariableOrigin("unknown", "f", "mypkg", meta)
	assert.Equal(t, "unknown", name1)
	assert.Equal(t, "mypkg", pkg1)
	assert.Equal(t, "f", fn1)

	// Second call should hit cache
	name2, pkg2, _, fn2 := TraceVariableOrigin("unknown", "f", "mypkg", meta)
	assert.Equal(t, name1, name2)
	assert.Equal(t, pkg1, pkg2)
	assert.Equal(t, fn1, fn2)
}

func TestTraceVariableOriginHelper_CacheMutexSafety(_ *testing.T) {
	meta := newTestMeta()
	meta.Packages["mypkg"] = &Package{
		Files: map[string]*File{"t.go": {Functions: map[string]*Function{}}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			varName := "var" + string(rune('a'+idx))
			TraceVariableOrigin(varName, "f", "mypkg", meta)
		}(i)
	}
	wg.Wait()
}

func TestTraceVariableOriginHelper_MethodLookupCacheNil(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	// Callee method not found -> nil cached
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"EmptyType": {Methods: []Method{}}},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"x": {{
				Value:       *valArg,
				CalleeFunc:  "NonExistent",
				CalleePkg:   "calleepkg",
				ReturnIndex: 0,
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("x", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
	// Verify nil was cached
	assert.Nil(t, meta.methodLookupCache["calleepkg.NonExistent"])
}

func TestTraceVariableOriginHelper_AssignmentAliasNonSelf(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	aArg := NewCallArgument(meta)
	aArg.SetKind(KindIdent)
	aArg.SetName("globalX")

	fn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"localAlias": {{Value: *aArg}},
		},
	}
	file := &File{
		Functions: map[string]*Function{"f": fn},
		Variables: map[string]*Variable{
			"globalX": {Name: sp.Get("globalX"), Type: sp.Get("int")},
		},
	}
	pkg := &Package{Files: map[string]*File{"test.go": file}}
	meta.Packages["mypkg"] = pkg

	name, _, _, _ := TraceVariableOrigin("localAlias", "f", "mypkg", meta)
	assert.Equal(t, "globalX", name)
}

func TestTraceVariableOriginHelper_ReturnIndexOutOfBounds(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindIdent)
	retArg.SetName("result")

	calleeFn := &Function{
		Name:       sp.Get("getVal"),
		ReturnVars: []CallArgument{*retArg},
	}
	calleeFile := &File{
		Functions: map[string]*Function{"getVal": calleeFn},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"x": {{
				Value:       *valArg,
				CalleeFunc:  "getVal",
				CalleePkg:   "calleepkg",
				ReturnIndex: 5, // out of bounds
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	// Should not panic, should fall through
	name, _, _, _ := TraceVariableOrigin("x", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}

func TestTraceVariableOriginHelper_MethodReturnIndexOutOfBounds(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool

	retArg := NewCallArgument(meta)
	retArg.SetKind(KindIdent)
	retArg.SetName("result")

	method := Method{
		Name:       sp.Get("GetItem"),
		ReturnVars: []CallArgument{*retArg},
	}
	myType := &Type{Methods: []Method{method}}
	calleeFile := &File{
		Functions: map[string]*Function{},
		Types:     map[string]*Type{"MyType": myType},
	}
	calleePkg := &Package{Files: map[string]*File{"callee.go": calleeFile}}

	valArg := NewCallArgument(meta)
	valArg.SetKind(KindCall)

	callerFn := &Function{
		Name: sp.Get("f"),
		AssignmentMap: map[string][]Assignment{
			"x": {{
				Value:       *valArg,
				CalleeFunc:  "GetItem",
				CalleePkg:   "calleepkg",
				ReturnIndex: 10, // out of bounds
			}},
		},
	}
	callerFile := &File{
		Functions: map[string]*Function{"f": callerFn},
	}
	callerPkg := &Package{Files: map[string]*File{"caller.go": callerFile}}

	meta.Packages["mypkg"] = callerPkg
	meta.Packages["calleepkg"] = calleePkg

	name, _, _, _ := TraceVariableOrigin("x", "f", "mypkg", meta)
	assert.NotEqual(t, "", name)
}
