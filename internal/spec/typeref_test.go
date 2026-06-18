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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTypeRef_Canonical(t *testing.T) {
	// input → canonical String(). Canonical inputs round-trip to themselves;
	// TypeSep inputs and `any` normalize to the dot/bracket / interface{} form.
	cases := map[string]string{
		"":                               "",
		"int":                            "int",
		"string":                         "string",
		"User":                           "User",
		"pkg.User":                       "pkg.User",
		"github.com/org/pkg.User":        "github.com/org/pkg.User",
		"*pkg.User":                      "*pkg.User",
		"[]pkg.User":                     "[]pkg.User",
		"[]*pkg.User":                    "[]*pkg.User",
		"[][]int":                        "[][]int",
		"[16]byte":                       "[16]byte",
		"map[string]pkg.Money":           "map[string]pkg.Money",
		"map[string][]pkg.Money":         "map[string][]pkg.Money",
		"map[string]*pkg.Money":          "map[string]*pkg.Money",
		"map[pkg.Key]pkg.Val":            "map[pkg.Key]pkg.Val",
		"map[map[string]int]bool":        "map[map[string]int]bool",
		"interface{}":                    "interface{}",
		"any":                            "interface{}",
		"pkg.APIResponse[pkg.User]":      "pkg.APIResponse[pkg.User]",
		"pkg.Pair[string,int]":           "pkg.Pair[string,int]",
		"pkg.Box[[]pkg.User]":            "pkg.Box[[]pkg.User]",
		"pkg.Box[map[string]int]":        "pkg.Box[map[string]int]",
		"pkg.Pair[pkg.A,pkg.B]":          "pkg.Pair[pkg.A,pkg.B]",
		"map[string][][]pkg.M":           "map[string][][]pkg.M",
		"[]map[string]*pkg.T":            "[]map[string]*pkg.T",
		"pkg.Outer[pkg.Inner[pkg.Leaf]]": "pkg.Outer[pkg.Inner[pkg.Leaf]]",
		// TypeSep (metadata-internal) forms normalize to canonical dot/bracket.
		"pkg-->User":                   "pkg.User",
		"pkg-->APIResponse-->pkg.User": "pkg.APIResponse[pkg.User]",
		"pkg-->APIResponse[pkg.User]":  "pkg.APIResponse[pkg.User]",
	}
	for in, want := range cases {
		assert.Equal(t, want, ParseTypeRef(in).String(), "input %q", in)
	}
}

func TestParseTypeRef_Structure(t *testing.T) {
	// Primitive vs named discrimination, including a qualified primitive.
	assert.Equal(t, KindBasic, ParseTypeRef("int").Kind)
	assert.Equal(t, KindBasic, ParseTypeRef("time.Time").Kind) // qualified primitive
	assert.Equal(t, KindNamed, ParseTypeRef("pkg.User").Kind)
	assert.Equal(t, KindNamed, ParseTypeRef("User").Kind)
	// A package-local type whose name collides with a builtin keeps its $ref
	// (Named), it is not mistaken for the primitive.
	bt := ParseTypeRef("mypkg.byte")
	assert.Equal(t, KindNamed, bt.Kind)
	assert.Equal(t, "mypkg", bt.Pkg)

	// Map value/key recursively parsed; nested slice has no name mangling.
	m := ParseTypeRef("map[string][]pkg.Money")
	require.Equal(t, KindMap, m.Kind)
	assert.Equal(t, KindBasic, m.Key.Kind)
	require.Equal(t, KindSlice, m.Elem.Kind)
	assert.Equal(t, "Money", m.Elem.Elem.Name)

	// Generic instantiation binds the concrete arg (both bracket and TypeSep).
	for _, in := range []string{"pkg.APIResponse[pkg.User]", "pkg-->APIResponse-->pkg.User", "pkg-->APIResponse[pkg.User]"} {
		g := ParseTypeRef(in)
		require.Equal(t, KindNamed, g.Kind, in)
		assert.Equal(t, "APIResponse", g.Name, in)
		require.Len(t, g.Args, 1, in)
		assert.Equal(t, "User", g.Args[0].Name, in)
	}

	// Multi-arg generic with a nested map arg, split on the top-level comma only.
	p := ParseTypeRef("pkg.Pair[map[string]int,[]pkg.User]")
	require.Len(t, p.Args, 2)
	assert.Equal(t, KindMap, p.Args[0].Kind)
	assert.Equal(t, KindSlice, p.Args[1].Kind)

	a := ParseTypeRef("[16]byte")
	require.Equal(t, KindArray, a.Kind)
	assert.Equal(t, 16, a.Len)

	ptr := ParseTypeRef("*pkg.User")
	require.Equal(t, KindPointer, ptr.Kind)
	assert.Equal(t, "User", ptr.Elem.Name)
}

func TestParseTypeRef_OpaqueForms(t *testing.T) {
	// Function / channel / field-bearing struct & interface types are carried
	// opaque (KindBasic, whole string in Name), never decomposed into a garbage
	// named/generic tree, and round-trip verbatim.
	for _, in := range []string{
		"func([]int) error",
		"func(http.ResponseWriter, *http.Request)",
		"chan int",
		"<-chan string",
		"chan<- int",
		"struct{X int}",
		"interface{Reader}",
	} {
		r := ParseTypeRef(in)
		assert.Equal(t, KindBasic, r.Kind, "input %q", in)
		assert.Empty(t, r.Pkg, "input %q must not fabricate a package", in)
		assert.Equal(t, in, r.String(), "opaque round-trip %q", in)
	}
}

func TestParseTypeRef_EdgeCases(t *testing.T) {
	// Malformed / incomplete forms → opaque, never a structured node with an
	// empty or degraded element.
	for _, in := range []string{"map[string", "[5", "map[string]", "[16]", "map[]int", "[-5]int", "[99999999999999999999]int"} {
		assert.Equal(t, KindBasic, ParseTypeRef(in).Kind, "input %q", in)
	}

	// A trailing token after the generic close doesn't corrupt the args (the
	// outer brackets are matched by depth, not TrimSuffix).
	g := ParseTypeRef("pkg.Box[pkg.User]extra")
	require.Len(t, g.Args, 1)
	assert.Equal(t, "User", g.Args[0].Name)

	// An unbalanced generic bracket degrades to the bare named type with no
	// args rather than a corrupted arg list.
	u := ParseTypeRef("pkg.Box[pkg.User")
	require.Equal(t, KindNamed, u.Kind)
	assert.Equal(t, "Box", u.Name)
	assert.Empty(t, u.Args)
}

func TestTypeRef_Qualify(t *testing.T) {
	// Bare field types gain the enclosing package on their named elements only.
	m := ParseTypeRef("map[string][]Money")
	m.Qualify("pkg")
	assert.Equal(t, "map[string][]pkg.Money", m.String())

	// Primitives and already-qualified types are untouched; the package only
	// reaches the element of a slice, not the slice itself.
	s := ParseTypeRef("[]other.Thing")
	s.Qualify("pkg")
	assert.Equal(t, "[]other.Thing", s.String())

	prim := ParseTypeRef("[][]int")
	prim.Qualify("pkg")
	assert.Equal(t, "[][]int", prim.String())

	// Generic args are qualified too.
	g := ParseTypeRef("Box[Item]")
	g.Qualify("pkg")
	assert.Equal(t, "pkg.Box[pkg.Item]", g.String())

	// Nil / empty-pkg are no-ops.
	var nilRef *TypeRef
	nilRef.Qualify("pkg")
	g.Qualify("")
}

func TestTypeRef_StringNil(t *testing.T) {
	var t0 *TypeRef
	assert.Equal(t, "", t0.String())
}
