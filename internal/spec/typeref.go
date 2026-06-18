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
// Phase 1 (issue: typed type model): the model + parser, additive and unused by
// production code. Later phases migrate the consumers and ultimately build
// TypeRef directly from go/types at the metadata boundary.
type TypeRef struct {
	Kind TypeKind
	// Named/Basic: the package qualifier (Named only) and the type name.
	Pkg  string
	Name string
	// Slice/Array/Pointer: the element type. Map: the value type.
	Elem *TypeRef
	// Map: the key type.
	Key *TypeRef
	// Named: generic type arguments, e.g. the User in APIResponse[User].
	Args []*TypeRef
	// Array: the length; -1 when unspecified (e.g. [...]T or a const expr).
	Len int
}

// TypeKind tags the shape of a TypeRef.
type TypeKind uint8

const (
	// KindBasic is a builtin/primitive scalar (int, string, bool, …) or an
	// unrecognized opaque type carried verbatim in Name.
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
	// KindInterface is interface{} / any.
	KindInterface
)

// ParseTypeRef parses a Go type string into a TypeRef. It accepts both the
// go/types form (dot-separated, bracket generics — "pkg.APIResponse[pkg.User]")
// and the metadata-internal TypeSep form ("pkg-->APIResponse-->pkg.User"),
// normalizing both to the same tree. Unrecognized input is returned as an
// opaque KindBasic so callers never need to guard against a nil result.
func ParseTypeRef(s string) *TypeRef {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return &TypeRef{Kind: KindBasic}
	case s == "interface{}" || s == "any":
		return &TypeRef{Kind: KindInterface, Name: s}
	case strings.HasPrefix(s, "*"):
		return &TypeRef{Kind: KindPointer, Elem: ParseTypeRef(s[1:])}
	case strings.HasPrefix(s, "[]"):
		return &TypeRef{Kind: KindSlice, Elem: ParseTypeRef(s[2:])}
	case strings.HasPrefix(s, "map["):
		return parseMap(s)
	case strings.HasPrefix(s, "["):
		return parseArray(s)
	default:
		return parseNamed(s)
	}
}

// parseMap parses "map[K]V"; the key may itself be a map/generic, so the
// key-closing bracket is found by depth, not the first ']'.
func parseMap(s string) *TypeRef {
	inner := s[len("map["):]
	depth, end := 1, -1
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return &TypeRef{Kind: KindBasic, Name: s} // malformed — keep opaque
	}
	return &TypeRef{
		Kind: KindMap,
		Key:  ParseTypeRef(inner[:end]),
		Elem: ParseTypeRef(inner[end+1:]),
	}
}

// parseArray parses "[N]Elem" (and "[]Elem" already handled by the caller). A
// non-numeric length (e.g. a const expr) yields Len == -1.
func parseArray(s string) *TypeRef {
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return &TypeRef{Kind: KindBasic, Name: s}
	}
	length := -1
	if n, err := strconv.Atoi(strings.TrimSpace(s[1:end])); err == nil {
		length = n
	}
	return &TypeRef{Kind: KindArray, Len: length, Elem: ParseTypeRef(s[end+1:])}
}

// parseNamed parses a named type, possibly a generic instantiation, in either
// the bracket form (Base[Arg, …]) or the TypeSep form (pkg-->Name-->Arg…).
func parseNamed(s string) *TypeRef {
	// Bracket generic, when the '[' is not preceded by a TypeSep-qualified base
	// (mirrors TypeParts so "pkg-->T-->[]X" style strings take the TypeSep path).
	if open := strings.IndexByte(s, '['); open > 0 && !strings.Contains(s[:open], TypeSep) {
		base := s[:open]
		args := strings.TrimSuffix(s[open+1:], "]")
		ref := splitPkgName(base)
		ref.Kind = KindNamed
		for _, a := range splitTopLevelCommas(args) {
			ref.Args = append(ref.Args, ParseTypeRef(a))
		}
		return ref
	}
	// TypeSep form: pkg-->Name(-->Arg)*.
	if parts := strings.Split(s, TypeSep); len(parts) > 1 {
		ref := &TypeRef{Kind: KindNamed, Pkg: parts[0], Name: parts[1]}
		for _, a := range parts[2:] {
			ref.Args = append(ref.Args, ParseTypeRef(a))
		}
		return ref
	}
	// Plain pkg.Name or a primitive.
	ref := splitPkgName(s)
	if ref.Pkg == "" && metadata.IsPrimitiveType(ref.Name) {
		ref.Kind = KindBasic
	} else {
		ref.Kind = KindNamed
	}
	return ref
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
// bracket depth 0, so Pair[map[string]int, []User] yields two arguments.
func splitTopLevelCommas(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
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

// String renders the TypeRef in canonical go/types form (dot-separated,
// bracket generics) — the inverse of ParseTypeRef for canonical input and the
// normal form for the TypeSep input.
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
		if t.Len >= 0 {
			return "[" + strconv.Itoa(t.Len) + "]" + t.Elem.String()
		}
		return "[]" + t.Elem.String()
	case KindMap:
		return "map[" + t.Key.String() + "]" + t.Elem.String()
	case KindInterface:
		if t.Name != "" {
			return t.Name
		}
		return "interface{}"
	default: // KindNamed, KindBasic
		name := t.Name
		if t.Pkg != "" {
			name = t.Pkg + "." + t.Name
		}
		if len(t.Args) == 0 {
			return name
		}
		args := make([]string, len(t.Args))
		for i, a := range t.Args {
			args[i] = a.String()
		}
		return name + "[" + strings.Join(args, ",") + "]"
	}
}
