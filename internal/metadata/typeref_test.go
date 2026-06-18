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
		{"P", RefNamed, "qual.Type"}, // selector, no type info → source qualifier
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

func TestTypeRefFromExpr_NilAndUnrecognized(t *testing.T) {
	assert.Nil(t, TypeRefFromExpr(nil, nil))
	assert.Nil(t, TypeRefFromExpr(&ast.BasicLit{Kind: token.INT, Value: "1"}, nil)) // not a type expr

	// A parenthesized type unwraps to its inner type.
	paren := TypeRefFromExpr(&ast.ParenExpr{X: &ast.Ident{Name: "int"}}, nil)
	require.NotNil(t, paren)
	assert.Equal(t, "int", paren.String())

	// A generic whose base is not a type (here a literal) yields no usable type.
	assert.Nil(t, TypeRefFromExpr(&ast.IndexExpr{X: &ast.BasicLit{Kind: token.INT, Value: "1"}, Index: &ast.Ident{Name: "T"}}, nil))

	// An array whose length is a non-constant, non-literal expression and no
	// type info to resolve it falls back to length 0 (still an array).
	arr := TypeRefFromExpr(&ast.ArrayType{Len: &ast.Ident{Name: "N"}, Elt: &ast.Ident{Name: "byte"}}, nil)
	require.Equal(t, RefArray, arr.Kind)
	assert.Equal(t, 0, arr.Len)

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

func TestTypeRef_StringNil(t *testing.T) {
	var t0 *TypeRef
	assert.Equal(t, "", t0.String())
	// chan is opaque — it always renders as the bare keyword.
	assert.Equal(t, "chan", (&TypeRef{Kind: RefChan}).String())
}
