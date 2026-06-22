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
	"gopkg.in/yaml.v3"
)

// fieldExprs parses src and returns each struct field's type expression keyed
// by field name. When check is true it also type-checks (importing real
// packages) and returns the types.Info, so selector package paths and constant
// array lengths resolve.
func fieldExprs(t *testing.T, src string, check bool) (map[string]ast.Expr, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	require.NoError(t, err)

	var info *types.Info
	if check {
		info = &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
		_, _ = conf.Check("x", fset, []*ast.File{f}, info) // tolerate undefined types
	}

	// Collect only the fields of the top-level struct declarations — not nested
	// function params/results or interface methods, whose names could otherwise
	// overwrite a struct field's entry and make a test assert the wrong type.
	exprs := map[string]ast.Expr{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					exprs[name.Name] = field.Type
				}
			}
		}
	}
	return exprs, info
}

func TestTypeRefFromExpr_Kinds(t *testing.T) {
	src := `package x
type S struct {
	A int
	B *User
	C []User
	D [16]byte
	E map[string]int
	F interface{}
	G interface{ Read() error }
	H struct{ X int }
	I func(a, b int) (string, error)
	J chan int
	Q <-chan int
	R chan<- Event
	K Box[User]
	L Pair[K, V]
	M [][]int
	N **User
	O map[string][]*User
	P qual.Type
	T1 [0x10]byte
	T2 [1_000]int
}`
	exprs, _ := fieldExprs(t, src, false)

	cases := []struct {
		field string
		kind  RefKind
		str   string
	}{
		{"A", RefBasic, "int"},
		{"B", RefPointer, "*User"},
		{"C", RefSlice, "[]User"},
		{"D", RefArray, "[16]byte"}, // length preserved (getTypeName dropped it)
		{"E", RefMap, "map[string]int"},
		{"F", RefInterface, "interface{}"},
		{"G", RefInterface, "interface{}"}, // non-empty interface still opaque any
		{"H", RefStruct, "struct{}"},
		{"I", RefFunc, "func"}, // never a string, so never mis-split on its commas
		{"J", RefChan, "chan"}, // chan is opaque — direction and element dropped
		{"Q", RefChan, "chan"}, // <-chan int
		{"R", RefChan, "chan"}, // chan<- Event
		{"K", RefNamed, "Box[User]"},
		{"L", RefNamed, "Pair[K,V]"}, // IndexListExpr — getTypeName produced ""
		{"M", RefSlice, "[][]int"},
		{"N", RefPointer, "**User"},
		{"O", RefMap, "map[string][]*User"},
		{"P", RefNamed, "qual.Type"},  // selector, no type info → source qualifier
		{"T1", RefArray, "[16]byte"},  // hex literal length, no type info (ParseInt base 0)
		{"T2", RefArray, "[1000]int"}, // underscore-separated literal length
	}
	for _, c := range cases {
		ref := TypeRefFromExpr(exprs[c.field], nil)
		require.NotNil(t, ref, c.field)
		assert.Equal(t, c.kind, ref.Kind, "field %s kind", c.field)
		assert.Equal(t, c.str, ref.String(), "field %s string", c.field)
	}

	// Structural spot-checks the round-trip string can't fully convey.
	require.Len(t, TypeRefFromExpr(exprs["D"], nil).Args, 0)
	assert.Equal(t, 16, TypeRefFromExpr(exprs["D"], nil).Len)
	l := TypeRefFromExpr(exprs["L"], nil)
	require.Len(t, l.Args, 2)
	assert.Equal(t, "K", l.Args[0].Name)
	assert.Equal(t, "V", l.Args[1].Name)
	p := TypeRefFromExpr(exprs["P"], nil)
	assert.Equal(t, "qual", p.Pkg)
	assert.Equal(t, "Type", p.Name)

	// Variadic ...T (valid only in signatures, so constructed directly) is a
	// slice, matching how go/types lowers it.
	va := TypeRefFromExpr(&ast.Ellipsis{Elt: &ast.Ident{Name: "Order"}}, nil)
	require.Equal(t, RefSlice, va.Kind)
	assert.Equal(t, "[]Order", va.String())
}

func TestTypeRefFromExpr_WithInfo(t *testing.T) {
	src := `package x
import (
	"time"
	"net/http"
)
const N = 8
type S struct {
	A time.Time
	B time.Duration
	C [N]byte
	D [4]int
	E []time.Time
	F http.Header
}`
	exprs, info := fieldExprs(t, src, true)
	require.NotNil(t, info)

	// time.Time is a recognized primitive; its package path is resolved.
	a := TypeRefFromExpr(exprs["A"], info)
	assert.Equal(t, RefBasic, a.Kind)
	assert.Equal(t, "time", a.Pkg)
	assert.Equal(t, "time.Time", a.String())

	// time.Duration is a named type, not a primitive — keeps its $ref.
	b := TypeRefFromExpr(exprs["B"], info)
	assert.Equal(t, RefNamed, b.Kind)
	assert.Equal(t, "time.Duration", b.String())

	// Constant array length resolved through the type-checker, and a literal.
	assert.Equal(t, 8, TypeRefFromExpr(exprs["C"], info).Len)
	assert.Equal(t, 4, TypeRefFromExpr(exprs["D"], info).Len)
	assert.Equal(t, "[]time.Time", TypeRefFromExpr(exprs["E"], info).String())

	// A multi-segment import path (ident "http" ≠ path "net/http") proves the
	// package comes from type info, not the source-level qualifier.
	f := TypeRefFromExpr(exprs["F"], info)
	assert.Equal(t, RefNamed, f.Kind)
	assert.Equal(t, "net/http", f.Pkg)
	assert.Equal(t, "net/http.Header", f.String())
}

func TestTypeRefFromExpr_TypeParam(t *testing.T) {
	exprs, info := fieldExprs(t, `package x
type Local int
type Box[T any] struct {
	V T
	W []T
	X Local
	U Undefined
}`, true)
	require.NotNil(t, info)

	// A generic type parameter is its own kind, not a package type — so Qualify
	// must not turn T into pkg.T.
	v := TypeRefFromExpr(exprs["V"], info)
	require.Equal(t, RefParam, v.Kind)
	assert.Equal(t, "T", v.Name)
	v.Qualify("pkg")
	assert.Equal(t, "T", v.String())

	w := TypeRefFromExpr(exprs["W"], info)
	require.Equal(t, RefSlice, w.Kind)
	assert.Equal(t, RefParam, w.Elem.Kind)
	w.Qualify("pkg")
	assert.Equal(t, "[]T", w.String())

	// A defined non-parameter type and an undefined identifier both classify as
	// named (exercising isTypeParam's non-param and nil-object branches).
	assert.Equal(t, RefNamed, TypeRefFromExpr(exprs["X"], info).Kind)
	assert.Equal(t, RefNamed, TypeRefFromExpr(exprs["U"], info).Kind)

	// Without type info a bare T cannot be told apart from a package-local type.
	assert.Equal(t, RefNamed, TypeRefFromExpr(exprs["V"], nil).Kind)
}

func TestTypeRefFromExpr_NilAndUnrecognized(t *testing.T) {
	assert.Nil(t, TypeRefFromExpr(nil, nil))
	assert.Nil(t, TypeRefFromExpr(&ast.BasicLit{Kind: token.INT, Value: "1"}, nil)) // not a type expr

	// A parenthesized type unwraps to its inner type.
	paren := TypeRefFromExpr(&ast.ParenExpr{X: &ast.Ident{Name: "int"}}, nil)
	require.NotNil(t, paren)
	assert.Equal(t, "int", paren.String())

	// A generic whose base is not a type (here a literal) yields no usable type.
	assert.Nil(t, TypeRefFromExpr(&ast.IndexExpr{X: &ast.BasicLit{Kind: token.INT, Value: "1"}, Index: &ast.Ident{Name: "T"}}, nil))

	// A generic whose base is recognized but not a NAMED type — a composite,
	// a primitive, or a type parameter (all ill-typed) — is nil, never a
	// fabricated "[T]"/"int[T]" identifier.
	tp := &ast.Ident{Name: "T"}
	assert.Nil(t, TypeRefFromExpr(&ast.IndexExpr{X: &ast.ArrayType{Elt: &ast.Ident{Name: "int"}}, Index: tp}, nil))                // ([]int)[T]
	assert.Nil(t, TypeRefFromExpr(&ast.IndexExpr{X: &ast.Ident{Name: "int"}, Index: tp}, nil))                                     // int[T]
	assert.Nil(t, TypeRefFromExpr(&ast.IndexListExpr{X: &ast.StarExpr{X: &ast.Ident{Name: "Foo"}}, Indices: []ast.Expr{tp}}, nil)) // (*Foo)[T]
	// A named base with an unrecognized argument also yields nil, not an empty arg.
	assert.Nil(t, TypeRefFromExpr(&ast.IndexExpr{X: &ast.Ident{Name: "Foo"}, Index: &ast.BasicLit{Kind: token.INT, Value: "1"}}, nil))
	// An already-instantiated base (the ill-typed nested Foo[A][B]) is nil, not a
	// collapsed Foo[A,B].
	fooAB := &ast.IndexExpr{X: &ast.IndexExpr{X: &ast.Ident{Name: "Foo"}, Index: &ast.Ident{Name: "A"}}, Index: &ast.Ident{Name: "B"}}
	assert.Nil(t, TypeRefFromExpr(fooAB, nil))

	// An array whose length is a non-constant, non-literal expression with no
	// type info is unresolved: Len -1, rendered [...] (distinct from a genuine [0]).
	arr := TypeRefFromExpr(&ast.ArrayType{Len: &ast.Ident{Name: "N"}, Elt: &ast.Ident{Name: "byte"}}, nil)
	require.Equal(t, RefArray, arr.Kind)
	assert.Equal(t, -1, arr.Len)
	assert.Equal(t, "[...]byte", arr.String())

	// An inferred-length array [...]T (only valid in composite literals, so
	// constructed directly) renders [...]T, not a wrong [0]T.
	ell := TypeRefFromExpr(&ast.ArrayType{Len: &ast.Ellipsis{}, Elt: &ast.Ident{Name: "int"}}, nil)
	require.Equal(t, RefArray, ell.Kind)
	assert.Equal(t, -1, ell.Len)
	assert.Equal(t, "[...]int", ell.String())

	// A composite whose inner type is unrecognized propagates nil instead of
	// fabricating a "*" / "[]" / "map[]" shell (the documented contract).
	lit := &ast.BasicLit{Kind: token.INT, Value: "1"}
	assert.Nil(t, TypeRefFromExpr(&ast.StarExpr{X: lit}, nil))
	assert.Nil(t, TypeRefFromExpr(&ast.ArrayType{Elt: lit}, nil))
	assert.Nil(t, TypeRefFromExpr(&ast.MapType{Key: &ast.Ident{Name: "string"}, Value: lit}, nil))

	// A selector whose base is not a plain package ident (a.b.C) has no simple
	// qualifier; it degrades to the trailing name with no package.
	nested := &ast.SelectorExpr{
		X:   &ast.SelectorExpr{X: &ast.Ident{Name: "a"}, Sel: &ast.Ident{Name: "b"}},
		Sel: &ast.Ident{Name: "C"},
	}
	y := TypeRefFromExpr(nested, nil)
	require.Equal(t, RefNamed, y.Kind)
	assert.Equal(t, "C", y.Name)
	assert.Empty(t, y.Pkg)
}

func TestTypeRef_Qualify(t *testing.T) {
	exprs, _ := fieldExprs(t, `package x
type S struct {
	A map[string][]Money
	B []other.Thing
	C [][]int
	D Box[Item]
	E chan Event
}`, false)

	a := TypeRefFromExpr(exprs["A"], nil)
	a.Qualify("pkg")
	assert.Equal(t, "map[string][]pkg.Money", a.String())

	b := TypeRefFromExpr(exprs["B"], nil)
	b.Qualify("pkg")
	assert.Equal(t, "[]other.Thing", b.String()) // already qualified — untouched

	c := TypeRefFromExpr(exprs["C"], nil)
	c.Qualify("pkg")
	assert.Equal(t, "[][]int", c.String()) // primitives — untouched

	d := TypeRefFromExpr(exprs["D"], nil)
	d.Qualify("pkg")
	assert.Equal(t, "pkg.Box[pkg.Item]", d.String()) // base and args both qualified

	e := TypeRefFromExpr(exprs["E"], nil)
	e.Qualify("pkg")
	assert.Equal(t, "chan", e.String()) // chan is opaque — not descended into

	var nilRef *TypeRef
	nilRef.Qualify("pkg") // no panic
	before := d.String()
	d.Qualify("") // empty pkg — no-op
	assert.Equal(t, before, d.String())
}

func TestTypeRefFromType(t *testing.T) {
	exprs, info := fieldExprs(t, `package x
import (
	"time"
	"net/http"
)
type Alias = int
type Box[T any] struct{ V T }
type S struct {
	A int
	B *S
	C []int
	D [4]byte
	E map[string]int
	F time.Time
	G time.Duration
	H http.Header
	I Box[int]
	J interface{}
	K func(int) error
	L chan int
	M Alias
	N struct{ X int }
	Err error
}`, true)
	require.NotNil(t, info)

	ref := func(field string) *TypeRef {
		typ := info.TypeOf(exprs[field])
		require.NotNil(t, typ, field)
		return TypeRefFromType(typ)
	}

	assert.Equal(t, "int", ref("A").String())
	assert.Equal(t, RefPointer, ref("B").Kind)
	assert.Equal(t, "[]int", ref("C").String())
	d := ref("D")
	assert.Equal(t, RefArray, d.Kind)
	assert.Equal(t, 4, d.Len)
	assert.Equal(t, RefMap, ref("E").Kind)
	assert.Equal(t, RefBasic, ref("F").Kind) // time.Time is a recognized primitive
	assert.Equal(t, "time.Time", ref("F").String())
	g := ref("G")
	assert.Equal(t, RefNamed, g.Kind)
	assert.Equal(t, "time.Duration", g.String())
	assert.Equal(t, "net/http", ref("H").Pkg) // full import path from go/types
	i := ref("I")
	require.Equal(t, RefNamed, i.Kind)
	require.Len(t, i.Args, 1)
	assert.Equal(t, "int", i.Args[0].String())
	assert.Equal(t, RefInterface, ref("J").Kind)
	assert.Equal(t, RefFunc, ref("K").Kind)
	assert.Equal(t, RefChan, ref("L").Kind)
	assert.Equal(t, "int", ref("M").String()) // alias resolved through Unalias
	assert.Equal(t, RefParam, ref("V").Kind)  // type parameter of Box
	assert.Equal(t, RefStruct, ref("N").Kind)
	// builtin `error` is a pkg-less *types.Named (Obj().Pkg()==nil) and a
	// recognized primitive — exercises namedTypeRef's nil-package branch.
	e := ref("Err")
	assert.Equal(t, RefBasic, e.Kind)
	assert.Equal(t, "error", e.String())

	// Defensive branches: an unrepresentable type (a tuple), and composites whose
	// element/key is unrepresentable, yield nil rather than a fabricated node.
	tuple := types.NewTuple()
	intT := types.Typ[types.Int]
	assert.Nil(t, TypeRefFromType(nil))
	assert.Nil(t, TypeRefFromType(types.Typ[types.Invalid]))  // unresolved/erroneous
	assert.Nil(t, TypeRefFromType(tuple))                     // default arm
	assert.Nil(t, TypeRefFromType(types.NewArray(tuple, 3)))  // array elem nil
	assert.Nil(t, TypeRefFromType(types.NewMap(tuple, intT))) // map key nil
	assert.Nil(t, TypeRefFromType(types.NewMap(intT, tuple))) // map value nil
	assert.Nil(t, TypeRefFromType(types.NewPointer(tuple)))   // pointer elem nil (via wrap)

	// An UNTYPED basic resolves to its DEFAULT typed form (not a "untyped X" name).
	assert.Equal(t, &TypeRef{Kind: RefBasic, Name: "int"}, TypeRefFromType(types.Typ[types.UntypedInt]))
	assert.Equal(t, &TypeRef{Kind: RefBasic, Name: "string"}, TypeRefFromType(types.Typ[types.UntypedString]))
	assert.Equal(t, &TypeRef{Kind: RefBasic, Name: "bool"}, TypeRefFromType(types.Typ[types.UntypedBool]))
	assert.Equal(t, &TypeRef{Kind: RefBasic, Name: "float64"}, TypeRefFromType(types.Typ[types.UntypedFloat]))
	assert.Nil(t, TypeRefFromType(types.Typ[types.UntypedNil])) // untyped nil has no typed default → no schema
}

func TestTypeRefFromExpr_InfoFallback(t *testing.T) {
	// When type info is present but the type is unresolved (TypeOf is nil/invalid),
	// TypeRefFromExpr falls back to the AST path — which still uses info to resolve
	// the selector's package and the array length.
	exprs, info := fieldExprs(t, `package x
import "time"
type S struct {
	A time.Undefined
	B [3]time.Nope
}`, true)
	require.NotNil(t, info)

	// Selector to an undefined member: the AST fallback resolves the package
	// import path via info (exercises selectorParts' info branch).
	a := TypeRefFromExpr(exprs["A"], info)
	require.Equal(t, RefNamed, a.Kind)
	assert.Equal(t, "time", a.Pkg)
	assert.Equal(t, "Undefined", a.Name)

	// Array of an undefined element: the AST fallback resolves the constant
	// length via info (exercises arrayLen's info path).
	b := TypeRefFromExpr(exprs["B"], info)
	require.Equal(t, RefArray, b.Kind)
	assert.Equal(t, 3, b.Len)
}

func TestTypeRefFromExpr_PackageIdent(t *testing.T) {
	// A package identifier (the "http" in http.Header) is not a type; it must not
	// become a fabricated RefNamed. It surfaces as a sub-expression when the
	// call-graph builder recurses into selectors.
	exprs, info := fieldExprs(t, `package x
import "net/http"
type S struct{ F http.Header }`, true)
	require.NotNil(t, info)
	sel, ok := exprs["F"].(*ast.SelectorExpr)
	require.True(t, ok)
	assert.Nil(t, TypeRefFromExpr(sel.X, info)) // sel.X is the "http" package ident
	// The selector itself still resolves correctly.
	assert.Equal(t, "net/http.Header", TypeRefFromExpr(sel, info).String())
}

func TestTypeRef_ConstructorsAgree(t *testing.T) {
	// With type info, TypeRefFromExpr routes through TypeRefFromType, so the two
	// constructors produce identical trees and the prior divergences (bare vs
	// full package path, `any` Kind) are gone.
	exprs, info := fieldExprs(t, `package x
type Local struct{ X int }
type S struct {
	A Local
	B any
	C []Local
}`, true)
	require.NotNil(t, info)

	// Same-package type now carries the full package path (matching go/types),
	// not a bare name — and equals what TypeRefFromType produces.
	a := TypeRefFromExpr(exprs["A"], info)
	require.Equal(t, RefNamed, a.Kind)
	assert.Equal(t, "x", a.Pkg)
	assert.Equal(t, TypeRefFromType(info.TypeOf(exprs["A"])), a)

	// `any` resolves to the interface kind through go/types (not RefBasic{"any"}).
	b := TypeRefFromExpr(exprs["B"], info)
	assert.Equal(t, RefInterface, b.Kind)
	assert.Equal(t, TypeRefFromType(info.TypeOf(exprs["B"])), b)

	// Composite of a same-package type agrees too.
	c := TypeRefFromExpr(exprs["C"], info)
	assert.Equal(t, TypeRefFromType(info.TypeOf(exprs["C"])), c)
}

func TestTypeRef_YAMLRoundTrip(t *testing.T) {
	// A tree exercising every field: a named generic with args, slice/map/pointer
	// nesting, a concrete array length, a genuine [0], and the inferred (-1)
	// sentinel — round-trips through default struct marshaling, no custom code.
	orig := &TypeRef{
		Kind: RefNamed, Pkg: "pkg", Name: "APIResponse",
		Args: []*TypeRef{
			{Kind: RefMap,
				Key:  &TypeRef{Kind: RefBasic, Name: "string"},
				Elem: &TypeRef{Kind: RefSlice, Elem: &TypeRef{Kind: RefPointer, Elem: &TypeRef{Kind: RefNamed, Pkg: "pkg", Name: "User"}}}},
			{Kind: RefArray, Len: 16, Elem: &TypeRef{Kind: RefBasic, Name: "byte"}},
			{Kind: RefArray, Len: 0, Elem: &TypeRef{Kind: RefBasic, Name: "int"}},  // genuine [0]int
			{Kind: RefArray, Len: -1, Elem: &TypeRef{Kind: RefBasic, Name: "int"}}, // inferred [...]int
			{Kind: RefInterface},
		},
	}
	data, err := yaml.Marshal(orig)
	require.NoError(t, err)
	var back TypeRef
	require.NoError(t, yaml.Unmarshal(data, &back))
	assert.Equal(t, orig, &back)                  // structurally identical
	assert.Equal(t, orig.String(), back.String()) // and renders identically
}

func TestCallArgument_ResolvedTypeRef(t *testing.T) {
	// Phase 3: SetResolvedType keeps the structured ResolvedTypeRef in lockstep
	// with the resolved-type string, and the field round-trips like the declared
	// TypeRef (default struct marshaling, no pooling).
	meta := &Metadata{StringPool: NewStringPool()}
	arg := &CallArgument{Meta: meta, Kind: -1, Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1}

	arg.SetResolvedType("pkg.User")
	assert.Equal(t, "pkg.User", arg.GetResolvedType())
	require.NotNil(t, arg.ResolvedTypeRef)
	assert.Equal(t, ParseTypeRef("pkg.User"), arg.ResolvedTypeRef)

	data, err := yaml.Marshal(arg)
	require.NoError(t, err)
	assert.Contains(t, string(data), "resolved_type_ref:")
	var back CallArgument
	require.NoError(t, yaml.Unmarshal(data, &back))
	assert.Equal(t, arg.ResolvedTypeRef, back.ResolvedTypeRef)

	// An empty resolved type leaves the ref unset (omitempty / graceful degradation).
	arg.SetResolvedType("")
	assert.Nil(t, arg.ResolvedTypeRef)
}

func TestTypeRef_StringNil(t *testing.T) {
	var t0 *TypeRef
	assert.Equal(t, "", t0.String())
	// chan is opaque — it always renders as the bare keyword.
	assert.Equal(t, "chan", (&TypeRef{Kind: RefChan}).String())
}
