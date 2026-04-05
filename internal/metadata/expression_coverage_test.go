package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/cfg"
)

// ------------------------------------------------------------
// ExprToCallArgument — nil and fallback
// ------------------------------------------------------------

func TestExprToCallArgument_Nil(t *testing.T) {
	meta := newTestMeta()
	arg := ExprToCallArgument(nil, nil, "pkg", nil, meta)
	assert.NotNil(t, arg)
	assert.Equal(t, KindRaw, arg.GetKind())
}

func TestExprToCallArgument_Fallback(t *testing.T) {
	// An expression type not explicitly handled falls to the default case
	meta := newTestMeta()
	fset := token.NewFileSet()
	// ast.BadExpr is not handled by ExprToCallArgument
	bad := &ast.BadExpr{}
	arg := ExprToCallArgument(bad, nil, "pkg", fset, meta)
	assert.NotNil(t, arg)
	assert.Equal(t, KindRaw, arg.GetKind())
}

// ------------------------------------------------------------
// handleIdent — various paths
// ------------------------------------------------------------

func TestHandleIdent_UntypedNil(t *testing.T) {
	src := `package testpkg

func f() {
	var x *int = nil
	_ = x
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the nil ident
	var nilIdent *ast.Ident
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "nil" {
			nilIdent = id
		}
		return true
	})
	require.NotNil(t, nilIdent)

	arg := handleIdent(nilIdent, info, "testpkg", fset, meta)
	assert.Equal(t, KindIdent, arg.GetKind())
	assert.Equal(t, "nil", arg.GetType())
	assert.Equal(t, "", arg.GetPkg())
}

func TestHandleIdent_PackageIdent(t *testing.T) {
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the fmt ident (the package reference in fmt.Println)
	var fmtIdent *ast.Ident
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "fmt" {
				fmtIdent = id
			}
		}
		return true
	})
	require.NotNil(t, fmtIdent)

	arg := handleIdent(fmtIdent, info, "testpkg", fset, meta)
	assert.Equal(t, KindIdent, arg.GetKind())
	assert.Equal(t, "fmt", arg.GetPkg())
	assert.Equal(t, "fmt", arg.GetName())
}

func TestHandleIdent_NoInfo(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ident := &ast.Ident{Name: "myVar"}
	arg := handleIdent(ident, nil, "pkg", fset, meta)
	assert.Equal(t, KindIdent, arg.GetKind())
	assert.Equal(t, "myVar", arg.GetName())
	assert.Equal(t, "pkg", arg.GetPkg())
}

func TestHandleIdent_ObjectWithPkg(t *testing.T) {
	src := `package testpkg

var x int

func f() {
	y := x
	_ = y
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the x reference in y := x
	var xIdent *ast.Ident
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if id, ok := as.Rhs[0].(*ast.Ident); ok && id.Name == "x" {
				xIdent = id
			}
		}
		return true
	})
	require.NotNil(t, xIdent)

	arg := handleIdent(xIdent, info, "testpkg", fset, meta)
	assert.Equal(t, KindIdent, arg.GetKind())
	assert.Equal(t, "x", arg.GetName())
}

// ------------------------------------------------------------
// handleCallExpr — type conversion vs function call
// ------------------------------------------------------------

func TestHandleCallExpr_TypeConversion(t *testing.T) {
	src := `package testpkg

type MyType int

func f() {
	x := MyType(42)
	_ = x
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	arg := handleCallExpr(callExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindTypeConversion, arg.GetKind())
}

func TestHandleCallExpr_FunctionCall(t *testing.T) {
	src := `package testpkg

func foo(x int) int { return x }

func f() {
	y := foo(42)
	_ = y
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "foo" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	arg := handleCallExpr(callExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindCall, arg.GetKind())
	assert.NotNil(t, arg.Fun)
	assert.Equal(t, 1, len(arg.Args))
}

func TestHandleCallExpr_MethodCall(t *testing.T) {
	src := `package testpkg

type S struct{}
func (s S) Do(x int) {}

func f() {
	s := S{}
	s.Do(42)
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

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

	arg := handleCallExpr(callExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindCall, arg.GetKind())
}

// ------------------------------------------------------------
// handleArrayType — with and without Len
// ------------------------------------------------------------

func TestHandleArrayType_Slice(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	arr := &ast.ArrayType{
		Elt: &ast.Ident{Name: "int"},
		Len: nil, // slice
	}
	arg := handleArrayType(arr, nil, "pkg", fset, meta)
	assert.Equal(t, KindArrayType, arg.GetKind())
	assert.NotNil(t, arg.X)
	assert.Equal(t, "", arg.GetValue()) // no length
}

func TestHandleArrayType_FixedLength(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	arr := &ast.ArrayType{
		Elt: &ast.Ident{Name: "byte"},
		Len: &ast.BasicLit{Kind: token.INT, Value: "32"},
	}
	arg := handleArrayType(arr, nil, "pkg", fset, meta)
	assert.Equal(t, KindArrayType, arg.GetKind())
	assert.Equal(t, "32", arg.GetValue())
}

// ------------------------------------------------------------
// handleSliceExpr — various combinations
// ------------------------------------------------------------

func TestHandleSliceExpr_LowOnly(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	slice := &ast.SliceExpr{
		X:    &ast.Ident{Name: "s"},
		Low:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		High: nil,
	}
	arg := handleSliceExpr(slice, nil, "pkg", fset, meta)
	assert.Equal(t, KindSlice, arg.GetKind())
	assert.Equal(t, 1, len(arg.Args))
}

func TestHandleSliceExpr_LowHighMax(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	slice := &ast.SliceExpr{
		X:    &ast.Ident{Name: "s"},
		Low:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		High: &ast.BasicLit{Kind: token.INT, Value: "5"},
		Max:  &ast.BasicLit{Kind: token.INT, Value: "10"},
	}
	arg := handleSliceExpr(slice, nil, "pkg", fset, meta)
	assert.Equal(t, KindSlice, arg.GetKind())
	assert.Equal(t, 3, len(arg.Args))
}

func TestHandleSliceExpr_NoLowNoHigh(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	slice := &ast.SliceExpr{
		X: &ast.Ident{Name: "s"},
	}
	arg := handleSliceExpr(slice, nil, "pkg", fset, meta)
	assert.Equal(t, KindSlice, arg.GetKind())
	assert.Equal(t, 0, len(arg.Args))
}

// ------------------------------------------------------------
// handleChanType — all directions
// ------------------------------------------------------------

func TestHandleChanType_Send(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ch := &ast.ChanType{Dir: ast.SEND, Value: &ast.Ident{Name: "int"}}
	arg := handleChanType(ch, nil, "pkg", fset, meta)
	assert.Equal(t, KindChanType, arg.GetKind())
	assert.Equal(t, "send", arg.GetValue())
}

func TestHandleChanType_Recv(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ch := &ast.ChanType{Dir: ast.RECV, Value: &ast.Ident{Name: "int"}}
	arg := handleChanType(ch, nil, "pkg", fset, meta)
	assert.Equal(t, KindChanType, arg.GetKind())
	assert.Equal(t, "recv", arg.GetValue())
}

func TestHandleChanType_Bidir(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ch := &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: &ast.Ident{Name: "int"}}
	arg := handleChanType(ch, nil, "pkg", fset, meta)
	assert.Equal(t, KindChanType, arg.GetKind())
	assert.Equal(t, "bidir", arg.GetValue())
}

// ------------------------------------------------------------
// handleStructType — embedded and named fields
// ------------------------------------------------------------

func TestHandleStructType_EmbeddedField(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	st := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{Type: &ast.Ident{Name: "io.Reader"}}, // embedded
			},
		},
	}
	arg := handleStructType(st, nil, "pkg", fset, meta)
	assert.Equal(t, KindStructType, arg.GetKind())
	assert.Equal(t, 1, len(arg.Args))
	assert.Equal(t, KindEmbed, arg.Args[0].GetKind())
}

func TestHandleStructType_MultipleNamedFields(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	st := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{{Name: "X"}, {Name: "Y"}},
					Type:  &ast.Ident{Name: "int"},
				},
			},
		},
	}
	arg := handleStructType(st, nil, "pkg", fset, meta)
	assert.Equal(t, KindStructType, arg.GetKind())
	assert.Equal(t, 2, len(arg.Args)) // X and Y
	assert.Equal(t, KindField, arg.Args[0].GetKind())
	assert.Equal(t, "X", arg.Args[0].GetName())
	assert.Equal(t, "Y", arg.Args[1].GetName())
}

// ------------------------------------------------------------
// callArgToString / CallArgToString — coverage of all kinds
// ------------------------------------------------------------

func TestCallArgToString_Nil(t *testing.T) {
	assert.Equal(t, "", CallArgToString(nil))
}

func TestCallArgToString_Ident_WithType(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName("x")
	arg.SetType("int")

	// With parent = non-nil, should return type
	parent := NewCallArgument(meta)
	parent.SetKind(KindCall)
	result := callArgToString(arg, parent)
	assert.Equal(t, "int", result)
}

func TestCallArgToString_Ident_WithoutType(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName("x")
	// Type is -1 (unset)
	result := callArgToString(arg, nil)
	assert.Equal(t, "x", result)
}

func TestCallArgToString_Literal(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindLiteral)
	arg.SetValue(`"hello"`)
	result := CallArgToString(arg)
	assert.Equal(t, "hello", result)
}

func TestCallArgToString_Selector(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("pkg")

	sel := NewCallArgument(meta)
	sel.SetKind(KindIdent)
	sel.SetName("Func")

	arg := NewCallArgument(meta)
	arg.SetKind(KindSelector)
	arg.X = x
	arg.Sel = sel
	result := CallArgToString(arg)
	assert.Equal(t, "pkg.Func", result)
}

func TestCallArgToString_SelectorNilX(t *testing.T) {
	meta := newTestMeta()
	sel := NewCallArgument(meta)
	sel.SetKind(KindIdent)
	sel.SetName("Func")

	arg := NewCallArgument(meta)
	arg.SetKind(KindSelector)
	arg.Sel = sel
	result := CallArgToString(arg)
	assert.Equal(t, "Func", result)
}

func TestCallArgToString_Call(t *testing.T) {
	meta := newTestMeta()
	fun := NewCallArgument(meta)
	fun.SetKind(KindIdent)
	fun.SetName("foo")

	argItem := NewCallArgument(meta)
	argItem.SetKind(KindLiteral)
	argItem.SetValue("42")

	arg := NewCallArgument(meta)
	arg.SetKind(KindCall)
	arg.Fun = fun
	arg.Args = []*CallArgument{argItem}
	result := CallArgToString(arg)
	assert.Equal(t, "foo(42)", result)
}

func TestCallArgToString_CallNilFun(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindCall)
	result := CallArgToString(arg)
	assert.Equal(t, "call()", result)
}

func TestCallArgToString_Unary(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("p")

	arg := NewCallArgument(meta)
	arg.SetKind(KindUnary)
	arg.SetValue("&")
	arg.X = x
	result := CallArgToString(arg)
	assert.Equal(t, "&p", result)
}

func TestCallArgToString_UnaryNilX(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindUnary)
	arg.SetValue("-")
	result := CallArgToString(arg)
	assert.Equal(t, "-", result)
}

func TestCallArgToString_Binary(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("a")

	y := NewCallArgument(meta)
	y.SetKind(KindIdent)
	y.SetName("b")

	arg := NewCallArgument(meta)
	arg.SetKind(KindBinary)
	arg.SetValue("+")
	arg.X = x
	arg.Fun = y
	result := CallArgToString(arg)
	assert.Equal(t, "a + b", result)
}

func TestCallArgToString_BinaryNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindBinary)
	arg.SetValue("+")
	result := CallArgToString(arg)
	assert.Equal(t, "+", result)
}

func TestCallArgToString_Index(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("arr")

	idx := NewCallArgument(meta)
	idx.SetKind(KindLiteral)
	idx.SetValue("0")

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	arg.X = x
	arg.Fun = idx
	result := CallArgToString(arg)
	assert.Equal(t, "arr[0]", result)
}

func TestCallArgToString_IndexNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	result := CallArgToString(arg)
	assert.Equal(t, "index", result)
}

func TestCallArgToString_IndexList(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("gen")

	idx1 := NewCallArgument(meta)
	idx1.SetKind(KindIdent)
	idx1.SetName("int")

	idx2 := NewCallArgument(meta)
	idx2.SetKind(KindIdent)
	idx2.SetName("string")

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndexList)
	arg.X = x
	arg.Args = []*CallArgument{idx1, idx2}
	result := CallArgToString(arg)
	assert.Equal(t, "gen[int, string]", result)
}

func TestCallArgToString_IndexListNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindIndexList)
	result := CallArgToString(arg)
	assert.Equal(t, "index_list", result)
}

func TestCallArgToString_Paren(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("expr")

	arg := NewCallArgument(meta)
	arg.SetKind(KindParen)
	arg.X = x
	result := CallArgToString(arg)
	assert.Equal(t, "(expr)", result)
}

func TestCallArgToString_ParenNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindParen)
	result := CallArgToString(arg)
	assert.Equal(t, "()", result)
}

func TestCallArgToString_Star(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("T")

	arg := NewCallArgument(meta)
	arg.SetKind(KindStar)
	arg.X = x
	result := CallArgToString(arg)
	assert.Equal(t, "*T", result)
}

func TestCallArgToString_StarNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindStar)
	result := CallArgToString(arg)
	assert.Equal(t, "*", result)
}

func TestCallArgToString_ArrayType(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("int")

	arg := NewCallArgument(meta)
	arg.SetKind(KindArrayType)
	arg.X = x
	arg.SetValue("5")
	result := CallArgToString(arg)
	assert.Equal(t, "[5]int", result)
}

func TestCallArgToString_ArrayTypeNilX(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindArrayType)
	arg.SetValue("3")
	result := CallArgToString(arg)
	assert.Equal(t, "[3]", result)
}

func TestCallArgToString_Slice(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("s")

	low := NewCallArgument(meta)
	low.SetKind(KindLiteral)
	low.SetValue("1")

	high := NewCallArgument(meta)
	high.SetKind(KindLiteral)
	high.SetValue("3")

	arg := NewCallArgument(meta)
	arg.SetKind(KindSlice)
	arg.X = x
	arg.Args = []*CallArgument{low, high}
	result := CallArgToString(arg)
	assert.Equal(t, "s[1:3]", result)
}

func TestCallArgToString_SliceInsufficient(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindSlice)
	result := CallArgToString(arg)
	assert.Equal(t, "slice", result)
}

func TestCallArgToString_CompositeLit(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("MyStruct")

	field := NewCallArgument(meta)
	field.SetKind(KindLiteral)
	field.SetValue("42")

	arg := NewCallArgument(meta)
	arg.SetKind(KindCompositeLit)
	arg.X = x
	arg.Args = []*CallArgument{field}
	result := CallArgToString(arg)
	assert.Equal(t, "MyStruct{42}", result)
}

func TestCallArgToString_CompositeLitNilX(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindCompositeLit)
	result := CallArgToString(arg)
	assert.Equal(t, "{}", result)
}

func TestCallArgToString_KeyValue(t *testing.T) {
	meta := newTestMeta()
	key := NewCallArgument(meta)
	key.SetKind(KindIdent)
	key.SetName("Name")

	val := NewCallArgument(meta)
	val.SetKind(KindLiteral)
	val.SetValue(`"Alice"`)

	arg := NewCallArgument(meta)
	arg.SetKind(KindKeyValue)
	arg.X = key
	arg.Fun = val
	result := CallArgToString(arg)
	assert.Equal(t, "Name: Alice", result)
}

func TestCallArgToString_KeyValueNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindKeyValue)
	result := CallArgToString(arg)
	assert.Equal(t, "key: value", result)
}

func TestCallArgToString_TypeAssert(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("val")

	typ := NewCallArgument(meta)
	typ.SetKind(KindIdent)
	typ.SetName("string")

	arg := NewCallArgument(meta)
	arg.SetKind(KindTypeAssert)
	arg.X = x
	arg.Fun = typ
	result := CallArgToString(arg)
	assert.Equal(t, "val.(string)", result)
}

func TestCallArgToString_TypeAssertNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindTypeAssert)
	result := CallArgToString(arg)
	assert.Equal(t, "type_assert", result)
}

func TestCallArgToString_FuncLit(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindFuncLit)
	arg.SetName("FuncLit:test.go:10:5")
	result := CallArgToString(arg)
	assert.Equal(t, "FuncLit:test.go:10:5", result)
}

func TestCallArgToString_ChanType(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("int")

	arg := NewCallArgument(meta)
	arg.SetKind(KindChanType)
	arg.X = x
	result := CallArgToString(arg)
	assert.Equal(t, "chan int", result)
}

func TestCallArgToString_ChanTypeNilX(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindChanType)
	result := CallArgToString(arg)
	assert.Equal(t, "chan", result)
}

func TestCallArgToString_MapType(t *testing.T) {
	meta := newTestMeta()
	key := NewCallArgument(meta)
	key.SetKind(KindIdent)
	key.SetName("string")

	val := NewCallArgument(meta)
	val.SetKind(KindIdent)
	val.SetName("int")

	arg := NewCallArgument(meta)
	arg.SetKind(KindMapType)
	arg.X = key
	arg.Fun = val
	result := CallArgToString(arg)
	assert.Equal(t, "map[string]int", result)
}

func TestCallArgToString_MapTypeNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindMapType)
	result := CallArgToString(arg)
	assert.Equal(t, "map", result)
}

func TestCallArgToString_StructType(t *testing.T) {
	meta := newTestMeta()
	field := NewCallArgument(meta)
	field.SetKind(KindIdent)
	field.SetName("X")

	arg := NewCallArgument(meta)
	arg.SetKind(KindStructType)
	arg.Args = []*CallArgument{field}
	result := CallArgToString(arg)
	assert.Equal(t, "struct{X}", result)
}

func TestCallArgToString_InterfaceType(t *testing.T) {
	meta := newTestMeta()
	method := NewCallArgument(meta)
	method.SetKind(KindInterfaceMethod)
	method.SetName("Do")

	arg := NewCallArgument(meta)
	arg.SetKind(KindInterfaceType)
	arg.Args = []*CallArgument{method}
	result := CallArgToString(arg)
	assert.Contains(t, result, "interface{")
}

func TestCallArgToString_Ellipsis(t *testing.T) {
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("int")

	arg := NewCallArgument(meta)
	arg.SetKind(KindEllipsis)
	arg.X = x
	result := CallArgToString(arg)
	assert.Equal(t, "...int", result)
}

func TestCallArgToString_EllipsisNil(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindEllipsis)
	result := CallArgToString(arg)
	assert.Equal(t, "...", result)
}

func TestCallArgToString_FuncType(t *testing.T) {
	meta := newTestMeta()
	param := NewCallArgument(meta)
	param.SetKind(KindIdent)
	param.SetName("int")

	ret := NewCallArgument(meta)
	ret.SetKind(KindIdent)
	ret.SetName("string")

	results := NewCallArgument(meta)
	results.SetKind(KindFuncResults)
	results.Args = []*CallArgument{ret}

	arg := NewCallArgument(meta)
	arg.SetKind(KindFuncType)
	arg.Args = []*CallArgument{param}
	arg.Fun = results
	result := CallArgToString(arg)
	assert.Contains(t, result, "func(")
	assert.Contains(t, result, "int")
	assert.Contains(t, result, "string")
}

func TestCallArgToString_FuncTypeWithTParams(t *testing.T) {
	meta := newTestMeta()
	param := NewCallArgument(meta)
	param.SetKind(KindIdent)
	param.SetName("x")

	ret := NewCallArgument(meta)
	ret.SetKind(KindIdent)
	ret.SetName("T")

	results := NewCallArgument(meta)
	results.SetKind(KindFuncResults)
	results.Args = []*CallArgument{ret}

	tparam := CallArgument{Meta: meta, Kind: -1, Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1}
	tparam.SetKind(KindIdent)
	tparam.SetName("T")
	tparam.SetType("any")

	arg := NewCallArgument(meta)
	arg.SetKind(KindFuncType)
	arg.Args = []*CallArgument{param}
	arg.TParams = []CallArgument{tparam}
	arg.Fun = results
	result := CallArgToString(arg)
	assert.Contains(t, result, "[T any]")
}

func TestCallArgToString_FuncTypeNilFun(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind(KindFuncType)
	result := CallArgToString(arg)
	assert.Equal(t, "func()", result)
}

func TestCallArgToString_FuncResults(t *testing.T) {
	meta := newTestMeta()
	ret1 := NewCallArgument(meta)
	ret1.SetKind(KindIdent)
	ret1.SetName("int")

	ret2 := NewCallArgument(meta)
	ret2.SetKind(KindIdent)
	ret2.SetName("error")

	arg := NewCallArgument(meta)
	arg.SetKind(KindFuncResults)
	arg.Args = []*CallArgument{ret1, ret2}
	result := CallArgToString(arg)
	assert.Equal(t, "int, error", result)
}

func TestCallArgToString_Default(t *testing.T) {
	meta := newTestMeta()
	arg := NewCallArgument(meta)
	arg.SetKind("unknown_kind")
	arg.SetRaw("raw_value")
	result := CallArgToString(arg)
	assert.Equal(t, "raw_value", result)
}

func TestCallArgToString_SelectorStarPrefix(t *testing.T) {
	// Selector where X resolves to a name starting with *
	meta := newTestMeta()
	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("*Receiver")

	sel := NewCallArgument(meta)
	sel.SetKind(KindIdent)
	sel.SetName("Method")

	arg := NewCallArgument(meta)
	arg.SetKind(KindSelector)
	arg.X = x
	arg.Sel = sel
	result := CallArgToString(arg)
	assert.Equal(t, "Receiver.Method", result)
}

// ------------------------------------------------------------
// io.go — WriteSplitMetadata and LoadSplitMetadata error paths
// ------------------------------------------------------------

func TestWriteSplitMetadata_StringPoolError(t *testing.T) {
	meta := newTestMeta()
	// Write to a non-existent directory
	err := WriteSplitMetadata(meta, "/nonexistent/dir/metadata.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write string pool")
}

func TestWriteSplitMetadata_PackagesError(t *testing.T) {
	meta := newTestMeta()
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "meta")

	// Write string pool so it succeeds, then make packages path a directory
	// with a sub-entry so os.Remove and OpenFile both fail
	pkgDir := basePath + "-packages.yaml"
	err := os.MkdirAll(filepath.Join(pkgDir, "sub"), 0755)
	require.NoError(t, err)

	err = WriteSplitMetadata(meta, basePath+".yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write packages")
}

func TestWriteSplitMetadata_CallGraphError(t *testing.T) {
	meta := newTestMeta()
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "meta")

	// Make call graph path a directory with a sub-entry so write fails
	cgDir := basePath + "-call-graph.yaml"
	err := os.MkdirAll(filepath.Join(cgDir, "sub"), 0755)
	require.NoError(t, err)

	err = WriteSplitMetadata(meta, basePath+".yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write call graph")
}

func TestWriteYAML_RemoveError(t *testing.T) {
	// Write to a path where Remove would fail but OpenFile still works
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "test.yaml")
	err := WriteYAML("hello", filename)
	assert.NoError(t, err)
}

func TestLoadSplitMetadata_PackagesError(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "meta")

	// Create only string pool file
	err := WriteYAML(NewStringPool(), basePath+"-string-pool.yaml")
	require.NoError(t, err)

	_, err = LoadSplitMetadata(basePath + ".yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load packages")
}

func TestLoadSplitMetadata_CallGraphError(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "meta")

	// Create string pool and packages files
	err := WriteYAML(NewStringPool(), basePath+"-string-pool.yaml")
	require.NoError(t, err)
	err = WriteYAML(map[string]*Package{}, basePath+"-packages.yaml")
	require.NoError(t, err)

	_, err = LoadSplitMetadata(basePath + ".yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load call graph")
}

// ------------------------------------------------------------
// cfg.go — mapBlockKind, extractCaseValues, annotateBranches
// ------------------------------------------------------------

func TestMapBlockKind_AllKnownKinds(t *testing.T) {
	tests := []struct {
		kind     cfg.BlockKind
		expected string
	}{
		{cfg.KindIfThen, "if-then"},
		{cfg.KindIfElse, "if-else"},
		{cfg.KindIfDone, ""},
		{cfg.KindSwitchCaseBody, "switch-case"},
		{cfg.KindSwitchNextCase, ""},
		{cfg.KindSwitchDone, ""},
		{cfg.KindForBody, ""},
		{cfg.KindForDone, ""},
		{cfg.KindSelectCaseBody, "select-case"},
		{cfg.KindSelectDone, ""},
		{0, ""}, // default/entry block
	}
	for _, tt := range tests {
		result := mapBlockKind(tt.kind)
		assert.Equal(t, tt.expected, result)
	}
}

func TestExtractCaseValues_NonCaseClause(t *testing.T) {
	// Not a CaseClause
	stmt := &ast.ExprStmt{X: &ast.Ident{Name: "x"}}
	assert.Nil(t, extractCaseValues(stmt))
}

func TestExtractCaseValues_NilStmt(t *testing.T) {
	assert.Nil(t, extractCaseValues(nil))
}

func TestExtractCaseValues_IntLiterals(t *testing.T) {
	cc := &ast.CaseClause{
		List: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "42"},
			&ast.BasicLit{Kind: token.INT, Value: "99"},
		},
	}
	values := extractCaseValues(cc)
	assert.Equal(t, []string{"42", "99"}, values)
}

func TestExtractCaseValues_StringLiterals(t *testing.T) {
	cc := &ast.CaseClause{
		List: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: `"GET"`},
			&ast.BasicLit{Kind: token.STRING, Value: `"POST"`},
		},
	}
	values := extractCaseValues(cc)
	assert.Equal(t, []string{"GET", "POST"}, values)
}

func TestExtractCaseValues_MixedExprs(t *testing.T) {
	// Non-BasicLit expressions should be skipped
	cc := &ast.CaseClause{
		List: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: `"A"`},
			&ast.Ident{Name: "SomeConst"}, // not a BasicLit
		},
	}
	values := extractCaseValues(cc)
	assert.Equal(t, []string{"A"}, values)
}

func TestExtractCaseValues_DefaultCase(t *testing.T) {
	// Default case has nil List
	cc := &ast.CaseClause{List: nil}
	values := extractCaseValues(cc)
	assert.Nil(t, values)
}

func TestBuildFunctionCFGs_NilBody(_ *testing.T) {
	// A FuncDecl with nil Body should be skipped
	meta := newTestMeta()
	fset := token.NewFileSet()
	decl := &ast.FuncDecl{
		Name: &ast.Ident{Name: "noBody"},
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: nil,
	}
	// Should not panic
	BuildFunctionCFGs([]*ast.FuncDecl{decl}, fset, meta)
}

func TestBuildFunctionCFGs_WithMatchingEdges(t *testing.T) {
	src := `package main

func handler(method string) {
	if method == "GET" {
		println("get")
	} else {
		println("other")
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "handler" {
			funcDecl = fn
		}
	}
	require.NotNil(t, funcDecl)

	meta := newTestMeta()
	// Add an edge with a position that might match
	pos := fset.Position(funcDecl.Body.Pos()).String()
	meta.CallGraph = []CallGraphEdge{
		{
			Position: meta.StringPool.Get(pos),
			Caller:   Call{Meta: meta, Name: meta.StringPool.Get("handler")},
			Callee:   Call{Meta: meta, Name: meta.StringPool.Get("println")},
		},
	}

	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
	// Just verify it doesn't panic and runs correctly
}

func TestBuildFunctionCFGs_SwitchWithEdges(t *testing.T) {
	src := `package main

func dispatch(method string) {
	switch method {
	case "GET":
		println("get")
	case "POST":
		println("post")
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
		}
	}
	require.NotNil(t, funcDecl)

	meta := newTestMeta()
	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
}

// ------------------------------------------------------------
// ExprToCallArgument — full round-trip for various expression types
// ------------------------------------------------------------

func TestExprToCallArgument_BasicLit(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	lit := &ast.BasicLit{Kind: token.STRING, Value: `"hello"`}
	arg := ExprToCallArgument(lit, nil, "pkg", fset, meta)
	assert.Equal(t, KindLiteral, arg.GetKind())
	assert.Equal(t, `"hello"`, arg.GetValue())
}

func TestExprToCallArgument_FuncLit(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	fset.AddFile("test.go", 1, 100)
	fl := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{},
	}
	arg := ExprToCallArgument(fl, nil, "pkg", fset, meta)
	assert.Equal(t, KindFuncLit, arg.GetKind())
	assert.Contains(t, arg.GetName(), "FuncLit:")
}

func TestExprToCallArgument_MapType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	mapExpr := &ast.MapType{
		Key:   &ast.Ident{Name: "string"},
		Value: &ast.Ident{Name: "int"},
	}
	arg := ExprToCallArgument(mapExpr, nil, "pkg", fset, meta)
	assert.Equal(t, KindMapType, arg.GetKind())
}

func TestExprToCallArgument_InterfaceType(t *testing.T) {
	meta := newTestMeta()
	iface := &ast.InterfaceType{
		Methods: &ast.FieldList{},
	}
	arg := ExprToCallArgument(iface, nil, "pkg", nil, meta)
	assert.Equal(t, KindInterfaceType, arg.GetKind())
}

func TestExprToCallArgument_FuncType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	funcType := &ast.FuncType{
		Params: &ast.FieldList{
			List: []*ast.Field{
				{Type: &ast.Ident{Name: "int"}},
			},
		},
		Results: &ast.FieldList{
			List: []*ast.Field{
				{Type: &ast.Ident{Name: "error"}},
			},
		},
	}
	arg := ExprToCallArgument(funcType, nil, "pkg", fset, meta)
	assert.Equal(t, KindFuncType, arg.GetKind())
	assert.Equal(t, 1, len(arg.Args))
	assert.NotNil(t, arg.Fun)
}

func TestExprToCallArgument_Ellipsis(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ellipsis := &ast.Ellipsis{Elt: &ast.Ident{Name: "int"}}
	arg := ExprToCallArgument(ellipsis, nil, "pkg", fset, meta)
	assert.Equal(t, KindEllipsis, arg.GetKind())
}

func TestExprToCallArgument_StarExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	star := &ast.StarExpr{X: &ast.Ident{Name: "int"}}
	arg := ExprToCallArgument(star, nil, "pkg", fset, meta)
	assert.Equal(t, KindStar, arg.GetKind())
}

func TestExprToCallArgument_KeyValueExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	kv := &ast.KeyValueExpr{
		Key:   &ast.Ident{Name: "Name"},
		Value: &ast.BasicLit{Kind: token.STRING, Value: `"test"`},
	}
	arg := ExprToCallArgument(kv, nil, "pkg", fset, meta)
	assert.Equal(t, KindKeyValue, arg.GetKind())
}

func TestExprToCallArgument_CompositeLit(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	comp := &ast.CompositeLit{
		Type: &ast.Ident{Name: "MyStruct"},
		Elts: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "42"},
		},
	}
	arg := ExprToCallArgument(comp, nil, "pkg", fset, meta)
	assert.Equal(t, KindCompositeLit, arg.GetKind())
	assert.Equal(t, 1, len(arg.Args))
}

func TestExprToCallArgument_UnaryExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	unary := &ast.UnaryExpr{
		Op: token.AND,
		X:  &ast.Ident{Name: "x"},
	}
	arg := ExprToCallArgument(unary, nil, "pkg", fset, meta)
	assert.Equal(t, KindUnary, arg.GetKind())
	assert.Equal(t, "&", arg.GetValue())
}

func TestExprToCallArgument_BinaryExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	bin := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "a"},
		Op: token.ADD,
		Y:  &ast.Ident{Name: "b"},
	}
	arg := ExprToCallArgument(bin, nil, "pkg", fset, meta)
	assert.Equal(t, KindBinary, arg.GetKind())
	assert.Equal(t, "+", arg.GetValue())
}

func TestExprToCallArgument_TypeAssertExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ta := &ast.TypeAssertExpr{
		X:    &ast.Ident{Name: "x"},
		Type: &ast.Ident{Name: "MyType"},
	}
	arg := ExprToCallArgument(ta, nil, "pkg", fset, meta)
	assert.Equal(t, KindTypeAssert, arg.GetKind())
}

func TestExprToCallArgument_IndexExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	idx := &ast.IndexExpr{
		X:     &ast.Ident{Name: "arr"},
		Index: &ast.BasicLit{Kind: token.INT, Value: "0"},
	}
	arg := ExprToCallArgument(idx, nil, "pkg", fset, meta)
	assert.Equal(t, KindIndex, arg.GetKind())
}

func TestExprToCallArgument_ParenExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	paren := &ast.ParenExpr{X: &ast.Ident{Name: "x"}}
	arg := ExprToCallArgument(paren, nil, "pkg", fset, meta)
	assert.Equal(t, KindParen, arg.GetKind())
}

func TestExprToCallArgument_ChanType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ch := &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: &ast.Ident{Name: "int"}}
	arg := ExprToCallArgument(ch, nil, "pkg", fset, meta)
	assert.Equal(t, KindChanType, arg.GetKind())
}

func TestExprToCallArgument_StructType(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	st := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{Names: []*ast.Ident{{Name: "X"}}, Type: &ast.Ident{Name: "int"}},
			},
		},
	}
	arg := ExprToCallArgument(st, nil, "pkg", fset, meta)
	assert.Equal(t, KindStructType, arg.GetKind())
}

// Test handleIdent with type-checked code to exercise the obj.Type() path
func TestHandleIdent_TypeChecked(t *testing.T) {
	src := `package testpkg

func f() {
	x := 42
	y := x
	_ = y
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	// Find the x usage in y := x
	var xUseIdent *ast.Ident
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if id, ok := as.Rhs[0].(*ast.Ident); ok && id.Name == "x" {
				xUseIdent = id
			}
		}
		return true
	})
	require.NotNil(t, xUseIdent)

	arg := handleIdent(xUseIdent, info, "testpkg", fset, meta)
	assert.Equal(t, KindIdent, arg.GetKind())
	assert.Equal(t, "x", arg.GetName())
	assert.NotEqual(t, "", arg.GetType())
}

// Test handleFuncType with TypeParams (generics)
func TestHandleFuncType_WithTypeParams(t *testing.T) {
	src := `package testpkg

func GenericFunc[T any](x T) T { return x }
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var funcDecl *ast.FuncDecl
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcDecl = fd
		}
	}
	require.NotNil(t, funcDecl)

	arg := handleFuncType(funcDecl.Type, info, "testpkg", fset, meta)
	assert.Equal(t, KindFuncType, arg.GetKind())
	assert.NotEmpty(t, arg.TParams)
}

func TestHandleFuncType_NoParamsNoResults(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ft := &ast.FuncType{
		Params: nil,
	}
	arg := handleFuncType(ft, nil, "pkg", fset, meta)
	assert.Equal(t, KindFuncType, arg.GetKind())
	assert.Nil(t, arg.Args)
}

// Test the handleSelector path with type-checked code
func TestHandleSelector_TypeChecked(t *testing.T) {
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			selExpr = se
		}
		return true
	})
	require.NotNil(t, selExpr)

	arg := handleSelector(selExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindSelector, arg.GetKind())
	assert.NotNil(t, arg.X)
	assert.NotNil(t, arg.Sel)
}

// Test handleSelector with method call for ReceiverType coverage
func TestHandleSelector_MethodWithReceiver(t *testing.T) {
	src := `package testpkg

type MyStruct struct{}
func (m *MyStruct) Do() {}

func f() {
	s := &MyStruct{}
	s.Do()
}
`
	fset, file, info := parseWithInfo(t, src)
	meta := newTestMeta()

	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok && se.Sel.Name == "Do" {
			selExpr = se
		}
		return true
	})
	require.NotNil(t, selExpr)

	arg := handleSelector(selExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindSelector, arg.GetKind())
	assert.NotNil(t, arg.ReceiverType)
}

// Test annotateBranches with assignment position matching
func TestAnnotateBranches_WithAssignmentMatch(t *testing.T) {
	src := `package main

func f(x int) int {
	if x > 0 {
		y := x + 1
		return y
	}
	return 0
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	var funcDecl *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fn
		}
	}
	require.NotNil(t, funcDecl)

	meta := newTestMeta()
	sp := meta.StringPool

	// Find the assignment position
	var assignPos string
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			assignPos = fset.Position(as.Pos()).String()
		}
		return true
	})

	// Create an assignment at that position
	assign := Assignment{
		Position: sp.Get(assignPos),
	}
	meta.CallGraph = []CallGraphEdge{
		{
			Caller: Call{Meta: meta, Name: sp.Get("main")},
			Callee: Call{Meta: meta, Name: sp.Get("f")},
			AssignmentMap: map[string][]Assignment{
				"y": {assign},
			},
		},
	}

	BuildFunctionCFGs([]*ast.FuncDecl{funcDecl}, fset, meta)
}

// Test ExprToCallArgument dispatches IndexListExpr via the switch
func TestExprToCallArgument_IndexListExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	ilExpr := &ast.IndexListExpr{
		X:       &ast.Ident{Name: "GenericFunc"},
		Indices: []ast.Expr{&ast.Ident{Name: "int"}, &ast.Ident{Name: "string"}},
	}
	arg := ExprToCallArgument(ilExpr, nil, "pkg", fset, meta)
	assert.Equal(t, KindIndexList, arg.GetKind())
}

// Test ExprToCallArgument dispatches SliceExpr via the switch
func TestExprToCallArgument_SliceExpr(t *testing.T) {
	meta := newTestMeta()
	fset := token.NewFileSet()
	slExpr := &ast.SliceExpr{
		X:    &ast.Ident{Name: "s"},
		Low:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		High: &ast.BasicLit{Kind: token.INT, Value: "5"},
	}
	arg := ExprToCallArgument(slExpr, nil, "pkg", fset, meta)
	assert.Equal(t, KindSlice, arg.GetKind())
}

// Test WriteYAML overwriting existing file
func TestWriteYAML_OverwriteExisting(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "test.yaml")

	// Write first time
	err := WriteYAML("first", filename)
	assert.NoError(t, err)

	// Overwrite
	err = WriteYAML("second", filename)
	assert.NoError(t, err)

	// Verify content
	var result string
	err = LoadYAML(filename, &result)
	assert.NoError(t, err)
	assert.Equal(t, "second", result)
}

// Test handleInterfaceType with no method names
func TestHandleInterfaceType_EmbeddedOnly(t *testing.T) {
	meta := newTestMeta()
	iface := &ast.InterfaceType{
		Methods: &ast.FieldList{
			List: []*ast.Field{
				{Type: &ast.Ident{Name: "io.Reader"}}, // embedded, no Names
			},
		},
	}
	arg := handleInterfaceType(iface, meta)
	assert.Equal(t, KindInterfaceType, arg.GetKind())
	// The embedded entry won't have Names, so methods[0] should be nil
	assert.Nil(t, arg.Args[0])
}
