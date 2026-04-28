package spec

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

func TestLookupParamConverter_Known(t *testing.T) {
	cases := []struct {
		pkg, name string
		want      paramConverter
	}{
		{"strconv", "Atoi", paramConverter{Type: "integer"}},
		{"strconv", "ParseInt", paramConverter{Type: "integer"}},
		{"strconv", "ParseUint", paramConverter{Type: "integer"}},
		{"strconv", "ParseBool", paramConverter{Type: "boolean"}},
		{"strconv", "ParseFloat", paramConverter{Type: "number"}},
		{"github.com/google/uuid", "Parse", paramConverter{Type: "string", Format: "uuid"}},
	}
	for _, tc := range cases {
		c := lookupParamConverter(tc.pkg, tc.name)
		require.NotNil(t, c, "expected match for %s.%s", tc.pkg, tc.name)
		assert.Equal(t, tc.want, *c)
	}
}

func TestLookupParamConverter_Unknown(t *testing.T) {
	assert.Nil(t, lookupParamConverter("strings", "TrimSpace"))
	assert.Nil(t, lookupParamConverter("", "Atoi"))
	assert.Nil(t, lookupParamConverter("strconv", ""))
}

func TestPositionLine(t *testing.T) {
	cases := map[string]int{
		"":                      0,
		"main.go":               0,
		"main.go:42:7":          42,
		"/abs/path/foo.go:10:1": 10,
		"sub/handler.go:1:1":    1,
		"weird:notanumber:1":    0,
	}
	for in, want := range cases {
		assert.Equal(t, want, positionLine(in), "input=%q", in)
	}
}

func TestEdgeArgUsesVariable(t *testing.T) {
	meta := newTestMeta()

	identV := makeIdentArg(meta, "v", "string")
	identOther := makeIdentArg(meta, "other", "string")
	literal := makeLiteralArg(meta, "lit")

	edge := makeEdge(meta, "Caller", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{identV, identOther, literal})

	assert.True(t, edgeArgUsesVariable(&edge, "v"))
	assert.True(t, edgeArgUsesVariable(&edge, "other"))
	assert.False(t, edgeArgUsesVariable(&edge, "missing"))
	assert.False(t, edgeArgUsesVariable(&edge, ""))
}

func TestSchemaFromConverter(t *testing.T) {
	s := schemaFromConverter(&paramConverter{Type: "integer"})
	require.NotNil(t, s)
	assert.Equal(t, "integer", s.Type)
	assert.Empty(t, s.Format)

	s = schemaFromConverter(&paramConverter{Type: "string", Format: "uuid"})
	assert.Equal(t, "string", s.Type)
	assert.Equal(t, "uuid", s.Format)
}

func TestInlineConverter_NoParent(t *testing.T) {
	meta := newTestMeta()
	edge := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	node := makeTrackerNode(&edge)
	assert.Nil(t, inlineConverter(node, meta))
}

func TestInlineConverter_KnownParent(t *testing.T) {
	meta := newTestMeta()

	parentEdge := makeEdge(meta, "Upload", "main", "Atoi", "strconv", nil)
	childEdge := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)

	parent := makeTrackerNode(&parentEdge)
	child := makeTrackerNode(&childEdge)
	child.Parent = parent

	c := inlineConverter(child, meta)
	require.NotNil(t, c)
	assert.Equal(t, "integer", c.Type)
}

func TestInlineConverter_UnknownParent(t *testing.T) {
	meta := newTestMeta()

	parentEdge := makeEdge(meta, "Upload", "main", "TrimSpace", "strings", nil)
	childEdge := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)

	parent := makeTrackerNode(&parentEdge)
	child := makeTrackerNode(&childEdge)
	child.Parent = parent

	assert.Nil(t, inlineConverter(child, meta))
}

func TestVarBoundConverter_PicksClosestForwardConsumer(t *testing.T) {
	// Simulate two FormValue calls bound to a shadowed `v`, each followed by
	// a different converter — Atoi (line 53) and ParseBool (line 41) — to make
	// sure the selector picks the converter at-or-after the FormValue's line.
	meta := newTestMeta()
	siblings := []*metadata.CallGraphEdge{}

	makeFormValue := func(line int) *metadata.CallGraphEdge {
		e := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
		e.CalleeRecvVarName = "v"
		e.Position = meta.StringPool.Get(formatLine(line))
		siblings = append(siblings, &e)
		return &e
	}
	makeConsumer := func(callee, pkg string, line int) *metadata.CallGraphEdge {
		e := makeEdge(meta, "Upload", "main", callee, pkg,
			[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
		e.Position = meta.StringPool.Get(formatLine(line))
		siblings = append(siblings, &e)
		return &e
	}

	fvBool := makeFormValue(40)
	makeConsumer("ParseBool", "strconv", 41)
	fvInt := makeFormValue(52)
	makeConsumer("Atoi", "strconv", 53)

	// Wire the caller index by hand — newTestMeta keeps Callers nil.
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fvBool.Caller.BaseID(): siblings,
	}

	c := varBoundConverter(fvBool, meta)
	require.NotNil(t, c)
	assert.Equal(t, "boolean", c.Type, "FormValue at line 40 should pair with ParseBool at line 41")

	c = varBoundConverter(fvInt, meta)
	require.NotNil(t, c)
	assert.Equal(t, "integer", c.Type, "FormValue at line 52 should pair with Atoi at line 53, not ParseBool at 41")
}

func TestVarBoundConverter_NoConsumerOrUnknown(t *testing.T) {
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "displayName"
	fv.Position = meta.StringPool.Get("main.go:60:2")
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv},
	}
	assert.Nil(t, varBoundConverter(&fv, meta), "no consumer → no inference")

	// Consumer exists but isn't a known converter
	noisy := makeEdge(meta, "Upload", "main", "TrimSpace", "strings",
		[]*metadata.CallArgument{makeIdentArg(meta, "displayName", "string")})
	noisy.Position = meta.StringPool.Get("main.go:61:2")
	meta.Callers[fv.Caller.BaseID()] = []*metadata.CallGraphEdge{&fv, &noisy}
	assert.Nil(t, varBoundConverter(&fv, meta), "non-converter consumer → no inference")
}

func TestVarBoundConverter_NonConverterConsumerStopsInference(t *testing.T) {
	// Regression: when the closest forward consumer of the bound variable is
	// NOT a converter (e.g. strings.Split), inference must stop and fall
	// through to the default schema. Skipping past the non-converter would
	// leak a converter from a later, unrelated shadowed-`v` scope into this
	// scope's parameter — the bug reported against the strings.Split idiom.
	meta := newTestMeta()

	// FormValue at line 40, bound to v.
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	fv.Position = meta.StringPool.Get(formatLine(40))

	// Closest forward consumer at line 41: strings.Split (non-converter).
	split := makeEdge(meta, "Upload", "main", "Split", "strings",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	split.Position = meta.StringPool.Get(formatLine(41))

	// Later sibling at line 51 in a different (shadowed) `v` scope: ParseBool.
	// Without the fix this is what the old logic erroneously selected.
	parseBool := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	parseBool.Position = meta.StringPool.Get(formatLine(51))

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv, &split, &parseBool},
	}

	assert.Nil(t, varBoundConverter(&fv, meta),
		"closest forward consumer is non-converter — must not jump past it to a later converter in a different scope")
}

func TestVarBoundConverter_PreferKnownPositionOverUnknown(t *testing.T) {
	// When metadata lacks position info on one consumer (sibLine == 0), a
	// later valid-position consumer must still win — otherwise the first
	// unknown-position sibling would lock in and block real matches.
	meta := newTestMeta()

	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	fv.Position = meta.StringPool.Get(formatLine(40))

	// Iteration order is preserved in a slice, so put the unknown-position
	// sibling first to exercise the fallback path explicitly.
	unknown := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	// no Position set — positionLine returns 0

	known := makeEdge(meta, "Upload", "main", "Atoi", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	known.Position = meta.StringPool.Get(formatLine(41))

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv, &unknown, &known},
	}

	c := varBoundConverter(&fv, meta)
	require.NotNil(t, c)
	assert.Equal(t, "integer", c.Type, "should prefer known-position Atoi over unknown-position ParseBool")
}

func TestVarBoundConverter_MultipleUnknownPositions_PicksFirst(t *testing.T) {
	// When several consumers all lack position metadata, fall back to the
	// first one in iteration order. meta.Callers slices are populated in
	// AST-walk order (source order), so this gives a deterministic answer.
	meta := newTestMeta()

	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	fv.Position = meta.StringPool.Get(formatLine(40))

	// Two consumers, both with no Position set. The first one in slice
	// order wins regardless of which is the "real" pairing.
	first := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	second := makeEdge(meta, "Upload", "main", "Atoi", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv, &first, &second},
	}

	c := varBoundConverter(&fv, meta)
	require.NotNil(t, c)
	assert.Equal(t, "boolean", c.Type, "first unknown-position consumer wins (slice order)")
}

func TestVarBoundConverter_FallsBackToUnknownPosition(t *testing.T) {
	// When EVERY converter sibling has unknown position, return one of them
	// rather than nil — the alternative would be silently dropping the type.
	meta := newTestMeta()

	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	fv.Position = meta.StringPool.Get(formatLine(40))

	noPos := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv, &noPos},
	}

	c := varBoundConverter(&fv, meta)
	require.NotNil(t, c)
	assert.Equal(t, "boolean", c.Type)
}

func TestVarBoundConverter_SkipsCallsWithoutBoundVar(t *testing.T) {
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	// no CalleeRecvVarName — caller used the result inline, not bound to a var
	assert.Nil(t, varBoundConverter(&fv, meta))
}

func TestInferParamConverterSchema_NilGuards(t *testing.T) {
	// Various nil guards keep the helper from panicking when called from the
	// matcher with a partly-initialised route.
	meta := newTestMeta()
	assert.Nil(t, inferParamConverterSchema(nil, &RouteInfo{Metadata: meta}))
	assert.Nil(t, inferParamConverterSchema(&TrackerNode{}, nil))
	assert.Nil(t, inferParamConverterSchema(&TrackerNode{}, &RouteInfo{}), "route.Metadata is nil")
	// Node with no edge — short-circuit before we touch any metadata fields.
	assert.Nil(t, inferParamConverterSchema(&TrackerNode{}, &RouteInfo{Metadata: meta}))
}

func TestInferParamConverterSchema_InlineMatch(t *testing.T) {
	// Dispatcher: an inline parent that's a known converter wins over the
	// var-bound branch and produces the converter's schema.
	meta := newTestMeta()
	parentEdge := makeEdge(meta, "Upload", "main", "Atoi", "strconv", nil)
	childEdge := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	parent := makeTrackerNode(&parentEdge)
	child := makeTrackerNode(&childEdge)
	child.Parent = parent

	got := inferParamConverterSchema(child, &RouteInfo{Metadata: meta})
	require.NotNil(t, got)
	assert.Equal(t, "integer", got.Type)
}

func TestInferParamConverterSchema_VarBoundMatch(t *testing.T) {
	// Dispatcher: no inline match, but a var-bound consumer is a known
	// converter — schema still gets inferred via the second branch.
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	fv.Position = meta.StringPool.Get(formatLine(10))

	consumer := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})
	consumer.Position = meta.StringPool.Get(formatLine(11))

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {&fv, &consumer},
	}

	got := inferParamConverterSchema(makeTrackerNode(&fv), &RouteInfo{Metadata: meta})
	require.NotNil(t, got)
	assert.Equal(t, "boolean", got.Type)
}

func TestInferParamConverterSchema_NoMatch(t *testing.T) {
	// Dispatcher: neither inline nor var-bound matches a known converter —
	// the helper returns nil so the caller falls back to the default schema.
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	assert.Nil(t, inferParamConverterSchema(makeTrackerNode(&fv), &RouteInfo{Metadata: meta}))
}

func TestInlineConverter_ParentWithoutEdge(t *testing.T) {
	// A parent tracker node without a CallGraphEdge should yield nil rather
	// than dereferencing the nil edge.
	meta := newTestMeta()
	childEdge := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	parent := &TrackerNode{} // no edge
	child := makeTrackerNode(&childEdge)
	child.Parent = parent
	assert.Nil(t, inlineConverter(child, meta))
}

func TestVarBoundConverter_EmptySiblings(t *testing.T) {
	// meta.Callers entry exists but is empty — early return without scanning.
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	fv.CalleeRecvVarName = "v"
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		fv.Caller.BaseID(): {},
	}
	assert.Nil(t, varBoundConverter(&fv, meta))
}

func TestEdgeArgUsesVariable_NilArg(t *testing.T) {
	// A nil entry in the slice is skipped instead of panicking.
	meta := newTestMeta()
	edge := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{nil, makeIdentArg(meta, "v", "string")})
	assert.True(t, edgeArgUsesVariable(&edge, "v"))

	onlyNil := makeEdge(meta, "Upload", "main", "ParseBool", "strconv",
		[]*metadata.CallArgument{nil, nil})
	assert.False(t, edgeArgUsesVariable(&onlyNil, "v"))
}

func TestStringFromPool_NilGuards(t *testing.T) {
	// nil meta and nil StringPool both yield "" rather than panicking.
	assert.Equal(t, "", stringFromPool(nil, 0))
	assert.Equal(t, "", stringFromPool(&metadata.Metadata{}, 0))

	// Sanity: with a real pool, the index is resolved.
	meta := newTestMeta()
	idx := meta.StringPool.Get("hello")
	assert.Equal(t, "hello", stringFromPool(meta, idx))
}

// formatLine builds a "file:line:col" string suitable for positionLine.
func formatLine(line int) string {
	// Using a fixed file/col keeps the tests focused on line ordering.
	return "main.go:" + strconv.Itoa(line) + ":1"
}
