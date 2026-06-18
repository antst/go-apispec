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
	// TypeSep inputs normalize to the dot/bracket form.
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
		"interface{}":                    "interface{}",
		"any":                            "any",
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
	}
	for in, want := range cases {
		assert.Equal(t, want, ParseTypeRef(in).String(), "input %q", in)
	}
}

func TestParseTypeRef_Structure(t *testing.T) {
	// Primitive vs named discrimination.
	assert.Equal(t, KindBasic, ParseTypeRef("int").Kind)
	assert.Equal(t, KindNamed, ParseTypeRef("pkg.User").Kind)
	assert.Equal(t, KindNamed, ParseTypeRef("User").Kind) // non-primitive, unqualified

	// Map value/key are recursively parsed.
	m := ParseTypeRef("map[string][]pkg.Money")
	require.Equal(t, KindMap, m.Kind)
	assert.Equal(t, KindBasic, m.Key.Kind)
	require.Equal(t, KindSlice, m.Elem.Kind)
	assert.Equal(t, KindNamed, m.Elem.Elem.Kind)
	assert.Equal(t, "pkg", m.Elem.Elem.Pkg)
	assert.Equal(t, "Money", m.Elem.Elem.Name)

	// Nested slice: [][]int → slice of slice of int (no name mangling).
	s := ParseTypeRef("[][]int")
	require.Equal(t, KindSlice, s.Kind)
	require.Equal(t, KindSlice, s.Elem.Kind)
	assert.Equal(t, KindBasic, s.Elem.Elem.Kind)
	assert.Equal(t, "int", s.Elem.Elem.Name)

	// Generic instantiation binds the concrete arg.
	g := ParseTypeRef("pkg.APIResponse[pkg.User]")
	require.Equal(t, KindNamed, g.Kind)
	assert.Equal(t, "APIResponse", g.Name)
	require.Len(t, g.Args, 1)
	assert.Equal(t, "User", g.Args[0].Name)

	// Multi-arg generic with a nested map arg, split on the top-level comma only.
	p := ParseTypeRef("pkg.Pair[map[string]int,[]pkg.User]")
	require.Len(t, p.Args, 2)
	assert.Equal(t, KindMap, p.Args[0].Kind)
	assert.Equal(t, KindSlice, p.Args[1].Kind)

	// Fixed-size array length is captured.
	a := ParseTypeRef("[16]byte")
	require.Equal(t, KindArray, a.Kind)
	assert.Equal(t, 16, a.Len)

	// Pointer wraps the element.
	ptr := ParseTypeRef("*pkg.User")
	require.Equal(t, KindPointer, ptr.Kind)
	assert.Equal(t, "User", ptr.Elem.Name)
}

func TestTypeRef_StringNil(t *testing.T) {
	var t0 *TypeRef
	assert.Equal(t, "", t0.String())
}

func TestParseTypeRef_EdgeCases(t *testing.T) {
	// Malformed (unbalanced) and incomplete (no value/element) forms → opaque,
	// never a structured node with an empty element.
	for _, in := range []string{"map[string", "[5", "map[string]", "[16]"} {
		assert.Equal(t, KindBasic, ParseTypeRef(in).Kind, "input %q", in)
	}

	// Array with a non-numeric length → Len -1, rendered as a plain slice.
	a := ParseTypeRef("[N]byte")
	require.Equal(t, KindArray, a.Kind)
	assert.Equal(t, -1, a.Len)
	assert.Equal(t, "[]byte", a.String())

	// A package-qualified primitive (time.Time → string/date-time mapping) is
	// Basic, not Named — so it isn't turned into a $ref component.
	tt := ParseTypeRef("time.Time")
	assert.Equal(t, KindBasic, tt.Kind)
	assert.Equal(t, "time.Time", tt.String())
}
