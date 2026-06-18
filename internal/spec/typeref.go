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
	"strconv"
	"strings"

	"github.com/antst/go-apispec/internal/metadata"
)

// TypeRef is a structured, recursive representation of a Go type. It replaces
// the ad-hoc parsing of type *strings* that was scattered across schema
// generation (TypeParts, qualifyElementType, the []/map[/* prefix checks, the
// schema-component name mangling). A type string is parsed once into a TypeRef
// tree and then walked, rather than re-split at every use — eliminating the
// double-separator (TypeSep "-->" vs ".") fragility and the name-mangling bugs
// (pkg..Money, [][]int → _int, unbound generic T-any).
//
// Phase 1 (typed type model): the model + parser, additive and unused by
// production code. Later phases migrate the consumers and ultimately build
// TypeRef directly from go/types at the metadata boundary.
type TypeRef struct {
	Kind TypeKind
	// Named/Basic: the package qualifier and the type name. Pkg is set for
	// KindNamed and also for KindBasic when the input is a qualified primitive
	// (e.g. time.Time), so that String() can render it correctly. For an opaque
	// form (func/chan/struct{…}/interface{Foo}) the whole string is Name.
	Pkg  string
	Name string
	// Slice/Array/Pointer: the element type. Map: the value type.
	Elem *TypeRef
	// Map: the key type.
	Key *TypeRef
	// Named: generic type arguments of an *instantiation*, e.g. the User in
	// APIResponse[User]. These are positional: Args[i] is the concrete type for
	// the i-th declared type parameter.
	Args []*TypeRef
	// Named: type-*parameter* names of an unbound generic *declaration*, e.g. the
	// T in APIResponse[T any] or the K, V in Pair[K comparable, V any]. A given
	// TypeRef is one or the other — an instantiation has Args, a declaration has
	// Params — and field substitution zips an instantiation's Args against the
	// declaration's Params (or the type's metadata TypeParams) by position.
	Params []string
	// Array: the length (always ≥ 0; non-numeric/negative lengths make the whole
	// form opaque).
	Len int
}

// TypeKind tags the shape of a TypeRef.
type TypeKind uint8

const (
	// KindBasic is a builtin/primitive scalar (int, string, bool, …) or an
	// unrecognized/opaque type (func/chan/struct{…}/interface{Foo}/malformed)
	// carried verbatim in Name.
	KindBasic TypeKind = iota
	// KindNamed is a package-qualified named type, optionally a generic
	// instantiation when Args is non-empty.
	KindNamed
	// KindSlice is []Elem.
	KindSlice
	// KindArray is [Len]Elem.
	KindArray
	// KindMap is map[Key]Elem.
	KindMap
	// KindPointer is *Elem.
	KindPointer
	// KindInterface is the empty interface (interface{} / any).
	KindInterface
)

// ParseTypeRef parses a Go type string into a TypeRef. It accepts both the
// go/types form (dot-separated, bracket generics — "pkg.APIResponse[pkg.User]")
// and the metadata-internal TypeSep form ("pkg-->APIResponse-->pkg.User"),
// normalizing both to the same tree. Forms that don't decompose into the schema
// model — function, channel, field-bearing struct/interface types — are carried
// verbatim as an opaque KindBasic. Structurally malformed input is handled on a
// best-effort basis: unbalanced generic brackets degrade to a bare named type
// (no Args), while other malformed forms (invalid array lengths, empty map key
// or value) also become opaque KindBasic. Callers never receive a nil result.
func ParseTypeRef(s string) *TypeRef {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return &TypeRef{Kind: KindBasic}
	case s == "interface{}" || s == "any":
		return &TypeRef{Kind: KindInterface}
	case strings.HasPrefix(s, "*"):
		return &TypeRef{Kind: KindPointer, Elem: ParseTypeRef(s[1:])}
	case strings.HasPrefix(s, "[]"):
		return &TypeRef{Kind: KindSlice, Elem: ParseTypeRef(s[2:])}
	case strings.HasPrefix(s, "map["):
		return parseMap(s)
	case strings.HasPrefix(s, "["):
		return parseArray(s)
	case isOpaqueForm(s):
		return &TypeRef{Kind: KindBasic, Name: s}
	default:
		return parseNamed(s)
	}
}

// isOpaqueForm reports whether s is a Go type that the schema model carries
// verbatim rather than decomposing — function, channel, and field-bearing
// struct/interface types. Decomposing these by the named/generic rules would
// fabricate a garbage tree (e.g. "func([]int) error" splitting at the slice's
// bracket), so they stay opaque.
func isOpaqueForm(s string) bool {
	for _, p := range []string{"func(", "func ", "func[", "chan ", "chan<-", "<-chan", "struct{", "struct ", "interface{"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parseMap parses "map[K]V"; the key may itself be a map/generic, so the
// key-closing bracket is found by depth, not the first ']'. An empty key or
// value (malformed/incomplete) yields an opaque KindBasic.
func parseMap(s string) *TypeRef {
	inner := s[len("map["):]
	depth, end := 1, -1
	for i := 0; i < len(inner) && end < 0; i++ {
		switch inner[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i
			}
		}
	}
	if end < 0 || strings.TrimSpace(inner[:end]) == "" || strings.TrimSpace(inner[end+1:]) == "" {
		return &TypeRef{Kind: KindBasic, Name: s}
	}
	return &TypeRef{
		Kind: KindMap,
		Key:  ParseTypeRef(inner[:end]),
		Elem: ParseTypeRef(inner[end+1:]),
	}
}

// parseArray parses "[N]Elem". A missing bracket, empty element, or a length
// that is not a non-negative integer (negative, overflow, or a const-expr
// placeholder) makes the whole form opaque rather than silently degrading to a
// slice.
func parseArray(s string) *TypeRef {
	end := strings.IndexByte(s, ']')
	if end < 0 || strings.TrimSpace(s[end+1:]) == "" {
		return &TypeRef{Kind: KindBasic, Name: s}
	}
	n, err := strconv.Atoi(strings.TrimSpace(s[1:end]))
	if err != nil || n < 0 {
		return &TypeRef{Kind: KindBasic, Name: s}
	}
	return &TypeRef{Kind: KindArray, Len: n, Elem: ParseTypeRef(s[end+1:])}
}

// parseNamed parses a named type, possibly a generic instantiation, in either
// the bracket form (Base[Arg, …]) or the TypeSep form (pkg-->Name(-->Arg)* —
// where Name may itself carry [Arg]).
func parseNamed(s string) *TypeRef {
	// Bracket generic, when the '[' is not preceded by a TypeSep-qualified base
	// (mirrors TypeParts so "pkg-->T[...]" style strings take the TypeSep path).
	if open := strings.IndexByte(s, '['); open > 0 && !strings.Contains(s[:open], TypeSep) {
		ref := splitPkgName(s[:open])
		ref.Kind = KindNamed
		ref.addGenericArgs(bracketArgList(s[open:]))
		return ref
	}
	// TypeSep form: pkg-->Name(-->Arg)*.
	if parts := strings.Split(s, TypeSep); len(parts) > 1 {
		name := parts[1]
		var bracket string
		if b := strings.IndexByte(name, '['); b >= 0 {
			bracket, name = name[b:], name[:b]
		}
		ref := &TypeRef{Kind: KindNamed, Pkg: parts[0], Name: name}
		ref.addGenericArgs(parts[2:])
		ref.addGenericArgs(bracketArgList(bracket))
		return ref
	}
	// Plain pkg.Name or a primitive. A qualified type can still be primitive —
	// metadata.IsPrimitiveType("time.Time") is true — but the unqualified-name
	// fallback applies only when there is no package, so a package-local type
	// named after a builtin (mypkg.byte) stays Named and keeps its $ref.
	ref := splitPkgName(s)
	if metadata.IsPrimitiveType(s) || (ref.Pkg == "" && metadata.IsPrimitiveType(ref.Name)) {
		ref.Kind = KindBasic
	} else {
		ref.Kind = KindNamed
	}
	return ref
}

// bracketArgList extracts the comma-separated arguments from a "[a, b, c]"
// string, matching the outer brackets by depth so a trailing token after the
// close (or a missing close) doesn't corrupt the list.
func bracketArgList(s string) []string {
	if !strings.HasPrefix(s, "[") {
		return nil
	}
	depth, end := 0, -1
	for i := 0; i < len(s) && end < 0; i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i
			}
		}
	}
	if end < 0 {
		return nil // unbalanced — no args
	}
	return splitTopLevelCommas(s[1:end])
}

// addGenericArgs distributes a generic bracket's tokens between Args and
// Params: a type-parameter declaration ("T any") records its name in Params,
// while a concrete instantiation argument ("pkg.User", "func(int) error") is
// parsed into Args. The two never mix on a single type — a string is either a
// declaration or an instantiation — but routing each token independently keeps
// a malformed mix from corrupting the other slice.
func (t *TypeRef) addGenericArgs(tokens []string) {
	for _, tok := range tokens {
		if name, ok := typeParamDecl(tok); ok {
			t.Params = append(t.Params, name)
		} else {
			t.Args = append(t.Args, ParseTypeRef(tok))
		}
	}
}

// typeParamDecl reports whether tok is a type-*parameter* declaration of the
// form "<ident> <constraint>" (the "T any" in APIResponse[T any]), returning
// the parameter name. The head before the first space must be a plain
// identifier, which distinguishes a declaration from a concrete argument that
// merely contains a space, such as "func(int) error".
func typeParamDecl(tok string) (string, bool) {
	sp := strings.IndexByte(tok, ' ')
	if sp <= 0 {
		return "", false
	}
	for i := 0; i < sp; i++ {
		if !isIdentChar(tok[i]) {
			return "", false
		}
	}
	return tok[:sp], true
}

// isIdentChar reports whether c may appear in a Go identifier (ASCII subset,
// sufficient for the type-parameter names go/types emits).
func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// splitPkgName splits "github.com/org/pkg.Name" at the last dot into the
// package qualifier and the type name. Names with no dot have an empty Pkg.
func splitPkgName(s string) *TypeRef {
	if dot := strings.LastIndexByte(s, '.'); dot > 0 {
		return &TypeRef{Pkg: s[:dot], Name: s[dot+1:]}
	}
	return &TypeRef{Name: s}
}

// splitTopLevelCommas splits a generic argument list on commas that sit at
// bracket/paren depth 0, so Pair[map[string]int, []User] yields two arguments
// and Box[func(int, error) error] yields one. The depths have a floor so stray
// closing tokens can't drive them negative.
func splitTopLevelCommas(s string) []string {
	var out []string
	brackets, parens, start := 0, 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '(':
			parens++
		case ')':
			if parens > 0 {
				parens--
			}
		case ',':
			if brackets == 0 && parens == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// Qualify attaches pkg to every unqualified, non-primitive Named node in the
// tree (recursively), turning a bare field type like []Money into []pkg.Money —
// the package-context rewrite qualifyElementType performs. Already-qualified
// nodes, primitives, and opaque/interface forms are left untouched.
func (t *TypeRef) Qualify(pkg string) {
	if t == nil || pkg == "" {
		return
	}
	switch t.Kind {
	case KindNamed:
		if t.Pkg == "" {
			t.Pkg = pkg
		}
		for _, a := range t.Args {
			a.Qualify(pkg)
		}
	case KindSlice, KindArray, KindPointer:
		t.Elem.Qualify(pkg)
	case KindMap:
		t.Key.Qualify(pkg)
		t.Elem.Qualify(pkg)
	}
}

// String renders the TypeRef in canonical go/types form (dot-separated, bracket
// generics) — the inverse of ParseTypeRef for canonical input and the normal
// form for TypeSep input.
func (t *TypeRef) String() string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case KindPointer:
		return "*" + t.Elem.String()
	case KindSlice:
		return "[]" + t.Elem.String()
	case KindArray:
		return "[" + strconv.Itoa(t.Len) + "]" + t.Elem.String()
	case KindMap:
		return "map[" + t.Key.String() + "]" + t.Elem.String()
	case KindInterface:
		return "interface{}"
	default: // KindNamed, KindBasic
		name := t.Name
		if t.Pkg != "" {
			name = t.Pkg + "." + t.Name
		}
		switch {
		case len(t.Args) > 0:
			args := make([]string, len(t.Args))
			for i, a := range t.Args {
				args[i] = a.String()
			}
			return name + "[" + strings.Join(args, ",") + "]"
		case len(t.Params) > 0:
			return name + "[" + strings.Join(t.Params, ",") + "]"
		default:
			return name
		}
	}
}
