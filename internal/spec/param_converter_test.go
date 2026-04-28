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

func TestVarBoundConverter_SkipsCallsWithoutBoundVar(t *testing.T) {
	meta := newTestMeta()
	fv := makeEdge(meta, "Upload", "main", "FormValue", "net/http", nil)
	// no CalleeRecvVarName — caller used the result inline, not bound to a var
	assert.Nil(t, varBoundConverter(&fv, meta))
}

func TestInferParamConverterSchema_NilGuards(t *testing.T) {
	// Various nil guards keep the helper from panicking when called from the
	// matcher with a partly-initialised route.
	assert.Nil(t, inferParamConverterSchema(nil, &RouteInfo{}))
	assert.Nil(t, inferParamConverterSchema(&TrackerNode{}, nil))
	assert.Nil(t, inferParamConverterSchema(&TrackerNode{}, &RouteInfo{}))
}

// formatLine builds a "file:line:col" string suitable for positionLine.
func formatLine(line int) string {
	// Using a fixed file/col keeps the tests focused on line ordering.
	return "main.go:" + strconv.Itoa(line) + ":1"
}
