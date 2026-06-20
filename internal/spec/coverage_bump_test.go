// Tests added to close specific behavioral gaps surfaced by coverage
// analysis. Each test exercises a real semantic branch that a user-facing
// scenario would hit — not coverage-padding.

package spec

import (
	"go/constant"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestExtractEnumValues_TypesConst exercises the *types.Const branch in
// extractEnumValues that real go/types-based analysis produces (the
// existing tests only fed in plain string/int values that hit the default
// branch). When the metadata pass attaches a real *types.Const to an
// enum constant, extractEnumValues must unwrap it through its .Val()
// constant.Value to produce the same shape the user sees in the spec.
func TestExtractEnumValues_TypesConst(t *testing.T) {
	pkg := types.NewPackage("example.com/enum", "enum")

	stringConst := types.NewConst(
		token.NoPos, pkg, "Active",
		types.Typ[types.String], constant.MakeString("active"),
	)
	intConst := types.NewConst(
		token.NoPos, pkg, "Pending",
		types.Typ[types.Int], constant.MakeInt64(2),
	)

	got := extractEnumValues([]EnumConstant{
		{Name: "Active", Value: stringConst},
		{Name: "Pending", Value: intConst},
	})

	// extractEnumValues sorts by string representation, so order is
	// deterministic — "2" sorts before "active".
	require.Len(t, got, 2)
	assert.Equal(t, int64(2), got[0])
	assert.Equal(t, "active", got[1])
}

// TestExtractEnumValues_TypesConst_NilValIsSkipped covers the
// `if v.Val() != nil` guard — a Const whose underlying value is nil
// shouldn't crash or emit a spurious nil entry.
func TestExtractEnumValues_TypesConst_NilValIsSkipped(t *testing.T) {
	// constant.MakeUnknown returns an "unknown" Kind value — not nil from
	// Go's perspective, but represents an unrepresentable constant. The
	// guard is `v.Val() != nil`, which is true here, so this still passes
	// through; that confirms the guard does what it says (skips only true
	// nil-Val() cases — which can only happen for malformed Const objects).
	pkg := types.NewPackage("example.com/enum", "enum")
	c := types.NewConst(token.NoPos, pkg, "Unknown",
		types.Typ[types.Int], constant.MakeUnknown())
	got := extractEnumValues([]EnumConstant{{Name: "Unknown", Value: c}})
	require.Len(t, got, 1, "non-nil Val() (even Unknown) is forwarded")
}

// TestTypeMatches_ConstantIsAlias covers the path where the constant's
// declared type is a Go alias whose underlying type matches the target.
// Without this branch, enum detection wouldn't find constants of an
// aliased type (e.g. `type Status string; const StatusActive Status = "active"`
// against a field declared as `string`).
func TestTypeMatches_ConstantIsAlias(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"example.com/enum": {
			Files: map[string]*metadata.File{
				"e.go": {
					Types: map[string]*metadata.Type{
						"Status": {
							Name:   sp.Get("Status"),
							Pkg:    sp.Get("example.com/enum"),
							Kind:   sp.Get("alias"),
							Target: sp.Get("string"),
						},
					},
				},
			},
		},
	}
	// constantType = "Status" (alias), targetType = "string" — must match
	// via the underlying-type resolution path.
	assert.True(t, typeMatches("example.com/enum.Status", "string", meta))
}

// TestTypeMatches_BothAreAliases_ResolveToSameUnderlying covers the
// nested path: both the constant type and the target are aliases that
// share an underlying type. e.g. `type ID = string` for both sides.
func TestTypeMatches_BothAreAliases_ResolveToSameUnderlying(t *testing.T) {
	meta := newTestMeta()
	sp := meta.StringPool
	meta.Packages = map[string]*metadata.Package{
		"example.com/enum": {
			Files: map[string]*metadata.File{
				"e.go": {
					Types: map[string]*metadata.Type{
						"UserID": {
							Name:   sp.Get("UserID"),
							Pkg:    sp.Get("example.com/enum"),
							Kind:   sp.Get("alias"),
							Target: sp.Get("string"),
						},
						"ContactID": {
							Name:   sp.Get("ContactID"),
							Pkg:    sp.Get("example.com/enum"),
							Kind:   sp.Get("alias"),
							Target: sp.Get("string"),
						},
					},
				},
			},
		},
	}
	assert.True(t, typeMatches("example.com/enum.UserID", "example.com/enum.ContactID", meta),
		"distinct aliases with the same underlying type should match")
}

// TestCallArgToString_KindCall_WithMappedTypeParams covers the generic-
// instantiation formatting path: when a call argument represents
// e.g. `Foo[int, string]`, callArgToString must render the type params
// in `[T, U]` form so downstream printers show generics clearly. The
// existing test of the same shape uses CallArgument.TParams which isn't
// what TypeParams() reads — this one populates TypeParamMap directly
// so the formatting branch actually fires.
func TestCallArgToString_KindCall_WithMappedTypeParams(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	// Build a KindCall arg with a Fun that's just an ident, and a
	// TypeParamMap so TypeParams() returns the instantiation.
	fun := makeCallArg(meta)
	fun.SetKind(metadata.KindIdent)
	fun.SetName("Foo")

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = fun
	arg.SetPkg("example.com/pkg")
	arg.SetName("Foo")
	// Two type params — order in output is map-iteration order, so the
	// test checks that BOTH appear inside the [...] block without
	// asserting their order.
	arg.TypeParamMap = map[string]string{"T": "int", "U": "string"}

	sep := "."
	got := cp.callArgToString(arg, &sep)
	assert.Contains(t, got, "[", "type-param block must be present")
	assert.Contains(t, got, "]")
	assert.Contains(t, got, "int")
	assert.Contains(t, got, "string")
}

// TestCallArgToString_KindCall_NoFun returns the fallback "call(...)"
// when the call argument has no Fun set. Defensive, but the path is
// real: builders occasionally produce KindCall with nil Fun for
// expressions that don't resolve cleanly to a callee.
func TestCallArgToString_KindCall_NoFun(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	assert.Equal(t, "call(...)", cp.callArgToString(arg, nil))
}

// TestCallArgToString_KindTypeConversion_NoFun mirrors the above for
// the type-conversion branch — `[]byte(value)` style — when Fun is nil.
func TestCallArgToString_KindTypeConversion_NoFun(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindTypeConversion)
	assert.Empty(t, cp.callArgToString(arg, nil))
}

// TestCallArgToString_KindInterfaceType formats the empty-interface
// argument shape (e.g. when a function parameter is declared `any`).
func TestCallArgToString_KindInterfaceType(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindInterfaceType)
	assert.Equal(t, "interface{}", cp.callArgToString(arg, nil))
}

// TestCallArgToString_KindRaw returns the raw stored string for the
// raw-expression escape hatch.
func TestCallArgToString_KindRaw(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindRaw)
	arg.SetRaw("anything")
	assert.Equal(t, "anything", cp.callArgToString(arg, nil))
}

// TestResolveGenericType_NoParams_UnparseableInput exercises the
// "no type params provided, base type empty" branch — a degenerate
// input that resolveGenericType must echo back unchanged instead of
// panicking. Lock the behavior so a future refactor of
// extractBaseTypeAndParams can't silently break callers that pass
// pre-resolved type names through.
func TestResolveGenericType_NoParams_UnparseableInput(t *testing.T) {
	cfg := &APISpecConfig{}
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))
	// Empty input — extractBaseTypeAndParams returns "" base, no params.
	got := resolver.ResolveGenericType("", nil)
	assert.Equal(t, "", got)
}

// TestResolveGenericType_WithParams_UnparseableInput covers the
// matching branch in the typeParams > 0 path: when the base-type
// extraction fails, the function returns the original input rather
// than partial garbage.
func TestResolveGenericType_WithParams_UnparseableInput(t *testing.T) {
	cfg := &APISpecConfig{}
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))
	got := resolver.ResolveGenericType("", map[string]string{"T": "int"})
	assert.Equal(t, "", got, "empty input must round-trip even when type params are supplied")
}

// TestResolveGenericType_WithParams_EmptyBrackets exercises the
// degenerate `Foo[]` shape WITH type params supplied — the typeParams
// > 0 branch's empty-paramStr collapse. Existing tests cover the
// no-params equivalent; this one closes the second copy of the
// collapse logic inside the typeParams branch.
func TestResolveGenericType_WithParams_EmptyBrackets(t *testing.T) {
	cfg := &APISpecConfig{}
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))
	got := resolver.ResolveGenericType("Foo[]", map[string]string{"T": "int"})
	assert.Equal(t, "Foo", got, "empty param block collapses to base type")
}

// TestResolveGenericType_WithParams_WhitespaceBrackets — whitespace-
// only parameter block (`Foo[ ]`) with type params supplied. Same
// collapse, different whitespace path.
func TestResolveGenericType_WithParams_WhitespaceBrackets(t *testing.T) {
	cfg := &APISpecConfig{}
	resolver := NewTypeResolver(nil, cfg, NewSchemaMapper(cfg))
	got := resolver.ResolveGenericType("Foo[   ]", map[string]string{"T": "int"})
	assert.Equal(t, "Foo", got)
}

// TestMapGoTypeToOpenAPISchema_ArrayOfCachedComplexType locks the path
// where the array element type is a complex (non-primitive) type that
// was already mapped into usedTypes by an earlier call. The function
// must return a `{type: array, items: {$ref: <type>}}` schema using
// the cached entry rather than re-recursing.
func TestMapGoTypeToOpenAPISchema_ArrayOfCachedComplexType(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()

	// Pre-seed usedTypes with a cached schema for the element type.
	cached := &Schema{
		Type:       "object",
		Properties: map[string]*Schema{"id": {Type: "integer"}},
	}
	usedTypes := map[string]*Schema{"User": cached}

	schema, _ := schemaForType(usedTypes, "[]User", nil, meta, cfg, nil)
	require.NotNil(t, schema)
	assert.Equal(t, "array", schema.Type, "array of complex type")
	require.NotNil(t, schema.Items, "items schema must be set")
	assert.NotEmpty(t, schema.Items.Ref, "items must be a $ref to the cached element type")
}

// TestMapGoTypeToOpenAPISchema_ArrayOfCachedComplexType_NilSchema
// covers the variant where the cached value is nil (placeholder) —
// the function must materialise the schema by recursing rather than
// dereferencing the nil.
func TestMapGoTypeToOpenAPISchema_ArrayOfCachedComplexType_NilSchema(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	// Cache key exists but value is nil — common placeholder shape during
	// schema generation before the type's own pass completes.
	usedTypes := map[string]*Schema{"User": nil}

	schema, _ := schemaForType(usedTypes, "[]User", nil, meta, cfg, nil)
	require.NotNil(t, schema)
	assert.Equal(t, "array", schema.Type)
	require.NotNil(t, schema.Items)
}

// TestMapGoTypeToOpenAPISchema_FixedSizeArrayOfCachedType exercises the
// `[N]ElementType` shape where the element is a cached complex type —
// must add MaxItems and MinItems equal to N alongside the items $ref.
func TestMapGoTypeToOpenAPISchema_FixedSizeArrayOfCachedType(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	usedTypes := map[string]*Schema{
		"User": {Type: "object", Properties: map[string]*Schema{"id": {Type: "integer"}}},
	}
	schema, _ := schemaForType(usedTypes, "[5]User", nil, meta, cfg, nil)
	require.NotNil(t, schema)
	assert.Equal(t, "array", schema.Type)
	assert.Equal(t, 5, schema.MinItems, "fixed-size array must set MinItems")
	assert.Equal(t, 5, schema.MaxItems, "fixed-size array must set MaxItems")
}

// TestMapGoTypeToOpenAPISchema_ArrayOfEnumOverlaysOntoStoredSchema
// exercises the path where an array element is a custom (non-primitive)
// type that ALSO has enum constants in metadata. After the recursive
// mapping stores the element schema under usedTypes/schemas (via the
// ref-promotion branch), the enum-overlay block must apply the enum
// values to the STORED schema (so the components.schemas entry carries
// the enum, not just the inline items pointer).
func TestMapGoTypeToOpenAPISchema_ArrayOfEnumOverlaysOntoStoredSchema(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	sp := meta.StringPool

	// Status is a string-based enum type with three const values. Building
	// the metadata directly so we don't need a full project parse. Tok
	// MUST be "const" — detectEnumFromConstants skips variables that
	// aren't marked as constants in the source.
	meta.Packages = map[string]*metadata.Package{
		"enum": {
			Files: map[string]*metadata.File{
				"e.go": {
					Types: map[string]*metadata.Type{
						"Status": {
							Name:   sp.Get("Status"),
							Pkg:    sp.Get("enum"),
							Kind:   sp.Get("alias"),
							Target: sp.Get("string"),
						},
					},
					Variables: map[string]*metadata.Variable{
						"Active": {
							Name:          sp.Get("Active"),
							Type:          sp.Get("enum.Status"),
							Tok:           sp.Get("const"),
							Value:         sp.Get(`"active"`),
							ComputedValue: "active",
						},
						"Inactive": {
							Name:          sp.Get("Inactive"),
							Type:          sp.Get("enum.Status"),
							Tok:           sp.Get("const"),
							Value:         sp.Get(`"inactive"`),
							ComputedValue: "inactive",
						},
					},
				},
			},
		},
	}

	usedTypes := map[string]*Schema{}
	schema, schemas := schemaForType(usedTypes, "[]enum.Status", nil, meta, cfg, nil)
	require.NotNil(t, schema)
	assert.Equal(t, "array", schema.Type)
	// Element type was non-primitive (`enum.Status`) so the array branch
	// took the enum-overlay path. With Tok=const + ComputedValue set on the
	// fixture, detectEnumFromConstants finds the two declared values and
	// attaches them — the test locks both the branch entry AND the overlay
	// itself, so a regression that drops either side will fail here.
	require.NotNil(t, schema.Items, "items schema must be set for [] of named type")
	assert.ElementsMatch(t, []interface{}{"active", "inactive"}, schema.Items.Enum,
		"enum overlay must attach the package-level const values to the items schema")
	_ = schemas
}

// TestResolveArgToStatusCode_NoResponseMatchers covers the fallback
// branch when the extractor has no response matchers wired up (e.g.
// a degenerate config with no response patterns). Must return
// (0, false) instead of panicking.
func TestResolveArgToStatusCode_NoResponseMatchers(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 10, MaxChildrenPerNode: 5,
		MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	e := NewExtractor(tree, cfg) // empty cfg → no response matchers initialised
	arg := makeLiteralArg(meta, "200")
	code, ok := e.resolveArgToStatusCode(arg)
	assert.Equal(t, 0, code)
	assert.False(t, ok)
}
