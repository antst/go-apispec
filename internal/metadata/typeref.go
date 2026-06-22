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
	"go/constant"
	"go/types"
	"strconv"
	"strings"
)

// TypeRef is a structured, recursive representation of a Go type, built once
// from the syntax tree (and type info) at the point where the structure is
// still intact, then carried as a tree rather than flattened to a string and
// re-parsed downstream.
//
// It replaces getTypeName's string flattening, which is lossy and ambiguous:
// getTypeName collapses every function to "func", every struct to "struct{}",
// every array to a slice (the length is dropped), and has no case for
// multi-parameter generics at all. A consumer that re-parses such a string
// cannot recover what was thrown away, and a string parser that tries to is an
// open-ended reimplementation of Go's type grammar. Building the tree from the
// AST sidesteps all of it: *ast.FuncType, *ast.IndexListExpr, and
// *ast.ArrayType.Len each answer the question directly and unambiguously.
//
// The yaml tags let the tree round-trip through the metadata model's
// serialization via default struct marshaling — no custom marshaler and no
// string parser. Kind distinguishes RefArray from RefSlice, so Len carries an
// omitempty 0 unambiguously (a genuine [0]T is a RefArray with Len 0).
type TypeRef struct {
	Kind RefKind `yaml:"kind,omitempty"`
	// Named: the package import path and type name. Basic: Name only (and Pkg
	// for a qualified primitive such as time.Time, so String renders it whole).
	Pkg  string `yaml:"pkg,omitempty"`
	Name string `yaml:"name,omitempty"`
	// Slice/Array/Pointer: the element type. Map: the value type.
	Elem *TypeRef `yaml:"elem,omitempty"`
	// Map: the key type.
	Key *TypeRef `yaml:"key,omitempty"`
	// Named: generic instantiation arguments, e.g. the User in APIResponse[User]
	// or the K, V in Pair[K, V] — positional, one per declared type parameter.
	Args []*TypeRef `yaml:"args,omitempty"`
	// Array: the length. >= 0 is a known constant length; -1 means inferred or
	// not statically resolved — [...]T, or a const-expression length built
	// without type info — and renders as [...]. A genuine [0]T keeps Len 0.
	Len int `yaml:"len,omitempty"`
}

// RefKind tags the shape of a TypeRef.
type RefKind uint8

const (
	// RefBasic is a builtin/primitive scalar (int, string, bool, time.Time, …).
	RefBasic RefKind = iota
	// RefNamed is a package-qualified named type, a generic instantiation when
	// Args is non-empty.
	RefNamed
	// RefSlice is []Elem.
	RefSlice
	// RefArray is [Len]Elem.
	RefArray
	// RefMap is map[Key]Elem.
	RefMap
	// RefPointer is *Elem.
	RefPointer
	// RefInterface is an interface type; schema-wise it is treated as any.
	RefInterface
	// RefFunc is a function type — opaque (no schema).
	RefFunc
	// RefStruct is an inline/anonymous struct type — opaque here; its fields
	// are captured separately as a nested type.
	RefStruct
	// RefChan is a channel type — opaque (no schema).
	RefChan
	// RefParam is a generic type-parameter reference (the T in Box[T]). It is not
	// a package type, so Qualify must leave it alone; detected only with type info.
	RefParam
)

// TypeRefFromExpr builds a TypeRef from a type expression. info may be nil; it
// is used to resolve a selector's package path, evaluate a constant array
// length, and distinguish a generic type parameter from a package-local type —
// all of which degrade gracefully when absent (without info a bare identifier
// is treated as a named type, never a type parameter). Unrecognized nodes
// return nil so callers can detect "no usable type" rather than a fabricated one.
//
//nolint:gocyclo // flat type switch over many AST node kinds — low real complexity
func TypeRefFromExpr(e ast.Expr, info *types.Info) *TypeRef {
	// Prefer the fully-resolved go/types type when available: it is the single
	// source of truth (resolves aliases, instantiations, and package import
	// paths) and guarantees TypeRefFromExpr and TypeRefFromType agree for the
	// same type. Fall back to the AST when type info is absent, the type is
	// unresolved (external fragments, undefined identifiers), OR go/types returns
	// a shape TypeRefFromType intentionally does not represent (tuples and other
	// default-arm kinds, for which it returns nil). This also lets the constructor
	// handle value expressions, whose type comes from info.
	if e != nil && info != nil {
		if t := info.TypeOf(e); t != nil {
			if ref := TypeRefFromType(t); ref != nil {
				return ref
			}
		}
	}
	switch t := e.(type) {
	case nil:
		return nil
	case *ast.ParenExpr:
		return TypeRefFromExpr(t.X, info)
	case *ast.Ident:
		switch {
		case isPackageName(t, info):
			return nil // a package identifier (the "http" in http.Header) is not a type
		case IsPrimitiveType(t.Name):
			return &TypeRef{Kind: RefBasic, Name: t.Name}
		case isTypeParam(t, info):
			return &TypeRef{Kind: RefParam, Name: t.Name}
		default:
			return &TypeRef{Kind: RefNamed, Name: t.Name}
		}
	case *ast.SelectorExpr:
		return namedOrBasic(selectorParts(t, info))
	case *ast.StarExpr:
		return wrap(RefPointer, TypeRefFromExpr(t.X, info))
	case *ast.ArrayType:
		elem := TypeRefFromExpr(t.Elt, info)
		if elem == nil {
			return nil
		}
		if t.Len == nil {
			return &TypeRef{Kind: RefSlice, Elem: elem}
		}
		return &TypeRef{Kind: RefArray, Len: arrayLen(t.Len, info), Elem: elem}
	case *ast.MapType:
		key, val := TypeRefFromExpr(t.Key, info), TypeRefFromExpr(t.Value, info)
		if key == nil || val == nil {
			return nil
		}
		return &TypeRef{Kind: RefMap, Key: key, Elem: val}
	case *ast.InterfaceType:
		return &TypeRef{Kind: RefInterface}
	case *ast.StructType:
		return &TypeRef{Kind: RefStruct}
	case *ast.FuncType:
		return &TypeRef{Kind: RefFunc}
	case *ast.ChanType: // opaque (no schema); direction and element carry no schema meaning
		return &TypeRef{Kind: RefChan}
	case *ast.Ellipsis: // variadic ...T is a []T (go/types lowers it the same way)
		return wrap(RefSlice, TypeRefFromExpr(t.Elt, info))
	case *ast.IndexExpr: // single-arg generic instantiation: Base[Arg]
		return namedWithArgs(t.X, []ast.Expr{t.Index}, info)
	case *ast.IndexListExpr: // multi-arg generic instantiation: Base[A, B]
		return namedWithArgs(t.X, t.Indices, info)
	default:
		return nil
	}
}

// namedWithArgs builds a generic instantiation: an un-instantiated named base
// type carrying its positional type arguments. Anything else is reachable only
// from ill-typed source and yields nil (never a fabricated identifier): a
// non-named base (int[T], ([]int)[T]), a base that is already instantiated (the
// nested Foo[A][B], which must not collapse to Foo[A,B]), or an unrecognized
// argument.
func namedWithArgs(base ast.Expr, args []ast.Expr, info *types.Info) *TypeRef {
	ref := TypeRefFromExpr(base, info)
	if ref == nil || ref.Kind != RefNamed || len(ref.Args) > 0 {
		return nil
	}
	for _, a := range args {
		arg := TypeRefFromExpr(a, info)
		if arg == nil {
			return nil
		}
		ref.Args = append(ref.Args, arg)
	}
	return ref
}

// wrap builds a single-element composite (pointer/slice), propagating a nil
// element as nil so an unrecognized inner type yields no usable type rather
// than a fabricated "*"/"[]" shell.
func wrap(kind RefKind, elem *TypeRef) *TypeRef {
	if elem == nil {
		return nil
	}
	return &TypeRef{Kind: kind, Elem: elem}
}

// isTypeParam reports whether id refers to a generic type parameter, which must
// not be qualified or treated as a package type. Requires type info; without it
// a bare identifier is indistinguishable from a package-local type.
func isTypeParam(id *ast.Ident, info *types.Info) bool {
	if info == nil {
		return false
	}
	obj := info.ObjectOf(id)
	if obj == nil {
		return false
	}
	_, ok := obj.Type().(*types.TypeParam)
	return ok
}

// isPackageName reports whether id refers to an imported package rather than a
// type (the "http" in http.Header). Such an identifier has no type and must not
// become a fabricated RefNamed. Requires type info.
func isPackageName(id *ast.Ident, info *types.Info) bool {
	if info == nil {
		return false
	}
	_, ok := info.ObjectOf(id).(*types.PkgName)
	return ok
}

// selectorParts resolves a qualified type "pkg.Name" to its import path and
// name. With type info the package's full import path is used; without it the
// source-level qualifier is the best available.
func selectorParts(t *ast.SelectorExpr, info *types.Info) (pkg, name string) {
	name = t.Sel.Name
	x, ok := t.X.(*ast.Ident)
	if !ok {
		return "", name
	}
	if info != nil {
		if p, ok := info.ObjectOf(x).(*types.PkgName); ok {
			return p.Imported().Path(), name
		}
	}
	return x.Name, name
}

// arrayLen evaluates a constant array length, preferring the type-checker's
// constant value (which resolves named/const-expression lengths) and falling
// back to a literal integer. Returns -1 when the length is inferred ([...]) or
// cannot be statically determined.
func arrayLen(e ast.Expr, info *types.Info) int {
	if info != nil {
		if tv, ok := info.Types[e]; ok && tv.Value != nil {
			if n, ok := constant.Int64Val(tv.Value); ok && fitsLen(n) {
				return int(n)
			}
		}
	}
	// Fallback without type info: ParseInt(base 0) honors Go's 0x/0o/0b/_ literal
	// forms, which strconv.Atoi would reject or misread (e.g. "010" as 10).
	if lit, ok := e.(*ast.BasicLit); ok {
		if n, err := strconv.ParseInt(lit.Value, 0, 64); err == nil && fitsLen(n) {
			return int(n)
		}
	}
	return -1 // inferred ([...]T) or not statically resolvable
}

// fitsLen reports whether a 64-bit array length is a non-negative value that
// survives the int conversion without truncation (relevant on 32-bit builds).
func fitsLen(n int64) bool {
	return n >= 0 && int64(int(n)) == n
}

// TypeRefFromType builds a TypeRef from a go/types type — the resolved-type
// counterpart to TypeRefFromExpr, for sites that have a types.Type but no
// syntax (e.g. a value's inferred type). go/types has already resolved aliases,
// constants, and instantiations, so array lengths and generic arguments are
// always concrete here. A nil type, or a child that does not resolve, yields nil.
func TypeRefFromType(t types.Type) *TypeRef {
	switch u := t.(type) {
	case nil:
		return nil
	case *types.Basic:
		if u.Kind() == types.Invalid {
			return nil // unresolved/erroneous type — no usable representation
		}
		return &TypeRef{Kind: RefBasic, Name: u.Name()}
	case *types.Named:
		return namedTypeRef(u)
	case *types.Alias:
		return TypeRefFromType(types.Unalias(u))
	case *types.Pointer:
		return wrap(RefPointer, TypeRefFromType(u.Elem()))
	case *types.Slice:
		return wrap(RefSlice, TypeRefFromType(u.Elem()))
	case *types.Array:
		elem := TypeRefFromType(u.Elem())
		if elem == nil {
			return nil
		}
		// fitsLen guards the int64->int conversion (matches the AST path); an
		// out-of-range length degrades to the inferred sentinel, not a wrap.
		ln := -1
		if fitsLen(u.Len()) {
			ln = int(u.Len())
		}
		return &TypeRef{Kind: RefArray, Len: ln, Elem: elem}
	case *types.Map:
		key, val := TypeRefFromType(u.Key()), TypeRefFromType(u.Elem())
		if key == nil || val == nil {
			return nil
		}
		return &TypeRef{Kind: RefMap, Key: key, Elem: val}
	case *types.Interface:
		return &TypeRef{Kind: RefInterface}
	case *types.Signature:
		return &TypeRef{Kind: RefFunc}
	case *types.Struct:
		return &TypeRef{Kind: RefStruct}
	case *types.Chan:
		return &TypeRef{Kind: RefChan}
	case *types.TypeParam:
		return &TypeRef{Kind: RefParam, Name: u.Obj().Name()}
	default:
		return nil
	}
}

// namedOrBasic classifies a resolved (pkg, name) pair as a primitive (RefBasic,
// e.g. time.Time) or a package type (RefNamed). Shared by the AST and go/types
// constructors so both classify the same type identically.
func namedOrBasic(pkg, name string) *TypeRef {
	full := name
	if pkg != "" {
		full = pkg + "." + name
	}
	if IsPrimitiveType(full) {
		return &TypeRef{Kind: RefBasic, Pkg: pkg, Name: name}
	}
	return &TypeRef{Kind: RefNamed, Pkg: pkg, Name: name}
}

// namedTypeRef builds the TypeRef for a *types.Named, resolving package identity,
// the primitive special-cases (via namedOrBasic), and generic instantiation
// arguments. An unresolvable type argument bails the whole node to nil, matching
// the AST twin namedWithArgs.
func namedTypeRef(n *types.Named) *TypeRef {
	obj := n.Obj()
	pkg := ""
	if obj.Pkg() != nil {
		pkg = obj.Pkg().Path()
	}
	ref := namedOrBasic(pkg, obj.Name())
	if ref.Kind != RefNamed {
		return ref // a primitive (error, time.Time, …) carries no type arguments
	}
	if args := n.TypeArgs(); args != nil {
		for i := 0; i < args.Len(); i++ {
			a := TypeRefFromType(args.At(i))
			if a == nil {
				return nil
			}
			ref.Args = append(ref.Args, a)
		}
	}
	return ref
}

// Qualify attaches pkg to every unqualified named node in the tree, turning a
// bare field type like []Money into []pkg.Money. Already-qualified nodes,
// primitives, and opaque kinds are left untouched.
func (t *TypeRef) Qualify(pkg string) {
	if t == nil || pkg == "" {
		return
	}
	switch t.Kind {
	case RefNamed:
		if t.Pkg == "" {
			t.Pkg = pkg
		}
		for _, a := range t.Args {
			a.Qualify(pkg)
		}
	case RefSlice, RefArray, RefPointer:
		t.Elem.Qualify(pkg)
	case RefMap:
		t.Key.Qualify(pkg)
		t.Elem.Qualify(pkg)
	}
}

// SubstituteParams returns a copy of the tree with every RefParam node replaced by
// its binding (keyed by parameter name); unbound params and all other nodes are
// preserved. Used to instantiate a generic struct's field types with the concrete
// type arguments — e.g. a field []K under bindings{K: User} becomes []User,
// including params nested inside Elem/Key/Args (decision D3). A no-op when there
// are no bindings.
func (t *TypeRef) SubstituteParams(bindings map[string]*TypeRef) *TypeRef {
	if t == nil || len(bindings) == 0 {
		return t
	}
	if t.Kind == RefParam {
		if b, ok := bindings[t.Name]; ok {
			// INVARIANT: the binding pointer is returned as-is, so every occurrence
			// of the same type parameter shares one *TypeRef node. Callers must treat
			// a substituted tree as READ-ONLY (the schema pipeline only reads
			// .String()) — mutating a node in place (e.g. Qualify) would corrupt every
			// sibling that bound the same argument. Clone first if you need to mutate.
			return b
		}
		return t
	}
	c := *t
	c.Elem = t.Elem.SubstituteParams(bindings)
	c.Key = t.Key.SubstituteParams(bindings)
	if t.Args != nil {
		c.Args = make([]*TypeRef, len(t.Args))
		for i, a := range t.Args {
			c.Args[i] = a.SubstituteParams(bindings)
		}
	}
	return &c
}

// NamedLeaf unwraps pointer, slice, array, and map wrappers and returns the first
// non-container node — the named/basic leaf a container ultimately holds (the map
// value for a map). Returns the receiver for a leaf, nil for nil.
func (t *TypeRef) NamedLeaf() *TypeRef {
	for t != nil && (t.Kind == RefPointer || t.Kind == RefSlice || t.Kind == RefArray || t.Kind == RefMap) {
		t = t.Elem
	}
	return t
}

// ParseTypeRef parses a Go-type string into a TypeRef — the inverse of String().
// It accepts the canonical String() form and the equivalent variants the schema
// pipeline produces: the "-->" package separator, short or bare package
// qualifiers, and primitive scalars. Package-qualified names keep whatever
// qualifier the string carried (typeByRef's name-only fallback resolves short
// forms); the round-trip String(ParseTypeRef(s)) == s holds for canonical input.
//
// This lets every remaining string-only producer (origin-traced body/param
// types, alias targets, config and wrapper-override type strings) reach the
// TypeRef tree, so the string-based schema generator could be retired and every
// schema now derives from the tree.
//
//nolint:gocyclo // one case per type-syntax form; a flat dispatch is clearest here
func ParseTypeRef(s string) *TypeRef {
	s = strings.TrimSpace(strings.ReplaceAll(s, "-->", "."))
	if s == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(s, "*"):
		return wrapElem(RefPointer, s[1:])
	case strings.HasPrefix(s, "[]"):
		return wrapElem(RefSlice, s[2:])
	case strings.HasPrefix(s, "[...]"):
		if e := ParseTypeRef(s[5:]); e != nil {
			return &TypeRef{Kind: RefArray, Len: -1, Elem: e}
		}
		return nil
	case strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "[]"):
		// [N]T fixed array — the bracket holds a non-negative decimal length.
		// Reject n < 0: a negative length is not valid Go, and Len < 0 is already
		// the reserved sentinel for the inferred-length form ("[...]T", Len -1), so
		// accepting "[-5]int" would silently masquerade as an inferred-length array.
		if end := strings.IndexByte(s, ']'); end > 1 {
			if n, err := strconv.Atoi(strings.TrimSpace(s[1:end])); err == nil && n >= 0 {
				if e := ParseTypeRef(s[end+1:]); e != nil {
					return &TypeRef{Kind: RefArray, Len: n, Elem: e}
				}
			}
		}
		return nil
	case strings.HasPrefix(s, "map["):
		return parseMapRef(s)
	case s == "interface{}" || s == "any":
		return &TypeRef{Kind: RefInterface}
	case s == "struct{}":
		return &TypeRef{Kind: RefStruct}
	case s == "func" || strings.HasPrefix(s, "func(") || strings.HasPrefix(s, "func ") || strings.HasPrefix(s, "func["):
		// "func[" is a GENERIC function type ("func[T any](T) T"); the analysis
		// elsewhere already treats the "func[" prefix as a function (tracker.go,
		// context_provider.go). Recognize it here too so an opaque generic func is
		// classified RefFunc, not misparsed as a nameable RefNamed (which would let
		// canAddRefSchemaForType emit a dangling $ref).
		return &TypeRef{Kind: RefFunc}
	case s == "chan" || strings.HasPrefix(s, "chan ") || strings.HasPrefix(s, "chan<-") || strings.HasPrefix(s, "<-chan"):
		return &TypeRef{Kind: RefChan}
	case IsPrimitiveType(s):
		return &TypeRef{Kind: RefBasic, Name: s}
	default:
		return parseNamedRef(s)
	}
}

// wrapElem parses elemStr and wraps it in a single-element container kind
// (pointer or slice). Returns nil when the element does not parse.
func wrapElem(kind RefKind, elemStr string) *TypeRef {
	e := ParseTypeRef(elemStr)
	if e == nil {
		return nil
	}
	return &TypeRef{Kind: kind, Elem: e}
}

// parseMapRef parses "map[K]V", scanning balanced brackets to find the key's
// closing "]" (so map[map[a]b]c keys parse correctly).
func parseMapRef(s string) *TypeRef {
	rest := s[len("map["):]
	depth, end := 0, -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				end = i
			} else {
				depth--
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}
	key := ParseTypeRef(rest[:end])
	val := ParseTypeRef(rest[end+1:])
	if key == nil || val == nil {
		return nil
	}
	return &TypeRef{Kind: RefMap, Key: key, Elem: val}
}

// parseNamedRef parses a (possibly generic, possibly package-qualified) named
// type: "pkg/path.Name", "pkg.Name[Arg1,Arg2]", or a bare "Name".
func parseNamedRef(s string) *TypeRef {
	var args []*TypeRef
	// A mid-string '[' opens a generic argument list (leading '[' was an array,
	// handled earlier). Everything up to it is the base name.
	if open := strings.IndexByte(s, '['); open > 0 && strings.HasSuffix(s, "]") {
		for _, a := range splitTopLevelCommas(s[open+1 : len(s)-1]) {
			ref := ParseTypeRef(a)
			if ref == nil {
				return nil
			}
			args = append(args, ref)
		}
		s = s[:open]
	}
	ref := &TypeRef{Kind: RefNamed, Name: s, Args: args}
	// Split a package qualifier off the LAST dot (import paths contain dots, but
	// the type name never does); a dotless string is an unqualified name.
	if dot := strings.LastIndexByte(s, '.'); dot > 0 {
		ref.Pkg = s[:dot]
		ref.Name = s[dot+1:]
	}
	return ref
}

// splitTopLevelCommas splits generic arguments on commas that are not nested
// inside a bracketed sub-argument (so Map[K,V] inside Outer[Map[K,V],T] stays
// whole).
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

// String renders the TypeRef as the project's canonical metadata identifier:
// dot-separated using the package's full import path, so a qualified type reads
// "net/http.Header" (an import-path form, not literal Go source). Opaque kinds
// render to their bare keyword (func, struct{}, interface{}, chan).
func (t *TypeRef) String() string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case RefPointer:
		return "*" + t.Elem.String()
	case RefSlice:
		return "[]" + t.Elem.String()
	case RefArray:
		if t.Len < 0 {
			return "[...]" + t.Elem.String()
		}
		return "[" + strconv.Itoa(t.Len) + "]" + t.Elem.String()
	case RefMap:
		return "map[" + t.Key.String() + "]" + t.Elem.String()
	case RefInterface:
		return "interface{}"
	case RefStruct:
		return "struct{}"
	case RefFunc:
		return "func"
	case RefChan:
		return "chan"
	default: // RefNamed, RefBasic
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
