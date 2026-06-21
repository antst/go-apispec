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

	"github.com/antst/go-apispec/internal/metadata"
)

// --- builders -------------------------------------------------------------

func wsIdent(meta *metadata.Metadata, name, pkg, typ string) *metadata.CallArgument {
	a := metadata.NewCallArgument(meta)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	if pkg != "" {
		a.SetPkg(pkg)
	}
	if typ != "" {
		a.SetType(typ)
	}
	return a
}

// wsCompositeReturn builds `&Struct{Field: paramIdent, ...}` as a KindUnary
// wrapping a KindCompositeLit of KindKeyValue elements. `bindings` maps field
// name → param ident name (insertion order irrelevant for the test).
func wsCompositeReturn(meta *metadata.Metadata, bindings [][2]string) *metadata.CallArgument {
	composite := metadata.NewCallArgument(meta)
	composite.SetKind(metadata.KindCompositeLit)
	elts := make([]*metadata.CallArgument, 0, len(bindings))
	for _, b := range bindings {
		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue)
		kv.X = wsIdent(meta, b[0], "", "")   // key = field name
		kv.Fun = wsIdent(meta, b[1], "", "") // value = param ident
		elts = append(elts, kv)
	}
	composite.Args = elts
	unary := metadata.NewCallArgument(meta)
	unary.SetKind(metadata.KindUnary)
	unary.X = composite
	return unary
}

// wsEnvelopeType builds a common.Envelope-style type with a generic Data field.
func wsEnvelopeType(meta *metadata.Metadata) *metadata.Type {
	sp := meta.StringPool
	return &metadata.Type{
		Name: sp.Get("Envelope"),
		Pkg:  sp.Get("common"),
		Fields: []metadata.Field{
			{Name: sp.Get("Message"), Type: sp.Get("string"), Tag: sp.Get(`json:"message,omitempty"`)},
			{Name: sp.Get("Data"), Type: sp.Get("interface{}"), Tag: sp.Get(`json:"data,omitempty"`)},
			{Name: sp.Get("Code"), Type: sp.Get("int"), Tag: sp.Get(`json:"code,omitempty"`)},
		},
	}
}

func wsMeta() *metadata.Metadata {
	m := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Packages:   map[string]*metadata.Package{},
		Callers:    map[string][]*metadata.CallGraphEdge{},
		Callees:    map[string][]*metadata.CallGraphEdge{},
	}
	return m
}

// wsMatcher builds a ResponsePatternMatcherImpl bound to meta.
func wsMatcher(meta *metadata.Metadata) *ResponsePatternMatcherImpl {
	cp := NewContextProvider(meta)
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(pmcTestCfg(), cp, nil),
		pattern:            ResponsePattern{},
	}
}

// wsGraph wires the full envelope scenario and returns the matcher, the Encode
// node (with a route-parent edge carrying the concrete payload type), and the
// `response` ident arg.
func wsGraph(t *testing.T, payloadType string) (*ResponsePatternMatcherImpl, *TrackerNode, *metadata.CallArgument) {
	t.Helper()
	meta := wsMeta()
	sp := meta.StringPool

	// Constructor edge: RespondWithSuccess -> NewEnvelope, ParamArgMap carries
	// the constructor's parameter names mapped to helper-local idents.
	ctorEdge := buildCallGraphEdge(meta, "RespondWithSuccess", "common", "NewEnvelope", "common", nil)
	ctorEdge.ParamArgMap = map[string]metadata.CallArgument{
		"message": *wsIdent(meta, "message", "common", ""), // unresolved -> dropped
		"data":    *wsIdent(meta, "data", "common", ""),    // resolved via parent edge
		"code":    *wsIdent(meta, "code", "common", ""),    // unresolved -> dropped
	}
	helperBaseID := ctorEdge.Caller.BaseID()
	meta.Callers[helperBaseID] = []*metadata.CallGraphEdge{ctorEdge}

	// Functions: helper with the `response := NewEnvelope(...)` assignment, and
	// the constructor returning &Envelope{...}.
	helper := &metadata.Function{
		Name: sp.Get("RespondWithSuccess"),
		Pkg:  sp.Get("common"),
		AssignmentMap: map[string][]metadata.Assignment{
			"response": {{CalleeFunc: "NewEnvelope", CalleePkg: "common", ReturnIndex: 0}},
		},
	}
	ctor := &metadata.Function{
		Name: sp.Get("NewEnvelope"),
		Pkg:  sp.Get("common"),
		ReturnVars: []metadata.CallArgument{
			*wsCompositeReturn(meta, [][2]string{{"Message", "message"}, {"Data", "data"}, {"Code", "code"}}),
		},
	}
	meta.Packages["common"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"common.go": {
				Functions: map[string]*metadata.Function{
					"RespondWithSuccess": helper,
					"NewEnvelope":        ctor,
				},
				Types: map[string]*metadata.Type{"Envelope": wsEnvelopeType(meta)},
			},
		},
	}

	// The matched Encode node lives inside the helper; its body arg is the
	// local `response` whose declared type is the wrapper struct.
	responseArg := wsIdent(meta, "response", "common", "common.Envelope")
	encodeEdge := buildCallGraphEdge(meta, "RespondWithSuccess", "common", "Encode", "encoding/json",
		[]*metadata.CallArgument{responseArg})
	node := buildTrackerNode(encodeEdge)

	// Route parent edge: listOrders -> RespondWithSuccess, carrying the
	// concrete payload type for `data`.
	parentEdge := buildCallGraphEdge(meta, "listOrders", "main", "RespondWithSuccess", "common", nil)
	parentEdge.ParamArgMap = map[string]metadata.CallArgument{
		"data": *wsIdent(meta, "resp", "", payloadType),
	}
	node.Parent = buildTrackerNode(parentEdge)

	return wsMatcher(meta), node, responseArg
}

// --- pure helpers ---------------------------------------------------------

func TestCleanOverrideType(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"interface{}":   "",
		"any":           "",
		"*any":          "",
		"&any":          "",
		"untyped int":   "",
		"mapToGeneric":  "", // bare function-like name
		"string":        "string",
		"int":           "int",
		"orders.Order":  "orders.Order",
		"*orders.Order": "orders.Order",
		"[]byte":        "[]byte",
		"pkg/sub.T":     "pkg/sub.T",
	}
	for in, want := range cases {
		assert.Equal(t, want, cleanOverrideType(in), "cleanOverrideType(%q)", in)
	}
}

func TestFieldParamBindingsFromReturnVar(t *testing.T) {
	meta := wsMeta()
	params := map[string]bool{"message": true, "data": true, "code": true}

	// nil arg.
	assert.Nil(t, fieldParamBindingsFromReturnVar(nil, params))

	// Non-composite (plain ident) -> nil.
	assert.Nil(t, fieldParamBindingsFromReturnVar(wsIdent(meta, "x", "", ""), params))

	// Happy path through &T{...}.
	ret := wsCompositeReturn(meta, [][2]string{{"Message", "message"}, {"Data", "data"}})
	got := fieldParamBindingsFromReturnVar(ret, params)
	assert.Equal(t, map[string]string{"Message": "message", "Data": "data"}, got)

	// Value not a known param -> dropped, yielding nil when nothing remains.
	retUnknown := wsCompositeReturn(meta, [][2]string{{"Data", "notAParam"}})
	assert.Nil(t, fieldParamBindingsFromReturnVar(retUnknown, params))

	// Element that isn't a key-value is ignored.
	composite := metadata.NewCallArgument(meta)
	composite.SetKind(metadata.KindCompositeLit)
	composite.Args = []*metadata.CallArgument{wsIdent(meta, "loose", "", "")}
	assert.Nil(t, fieldParamBindingsFromReturnVar(composite, params))

	// Paren-wrapped composite is unwrapped like the address-of case.
	inner := wsCompositeReturn(meta, [][2]string{{"Data", "data"}}) // &T{...}
	paren := metadata.NewCallArgument(meta)
	paren.SetKind(metadata.KindParen)
	paren.X = inner.X // the composite lit
	assert.Equal(t, map[string]string{"Data": "data"}, fieldParamBindingsFromReturnVar(paren, params))

	// Key-value elements with nil/non-ident/empty parts are skipped.
	noisy := metadata.NewCallArgument(meta)
	noisy.SetKind(metadata.KindCompositeLit)
	kvNilVal := metadata.NewCallArgument(meta)
	kvNilVal.SetKind(metadata.KindKeyValue)
	kvNilVal.X = wsIdent(meta, "Data", "", "")
	kvNilVal.Fun = nil
	kvLitKey := metadata.NewCallArgument(meta)
	kvLitKey.SetKind(metadata.KindKeyValue)
	kvLitKey.X = buildLiteralArg(meta, "lit")
	kvLitKey.Fun = wsIdent(meta, "data", "", "")
	kvEmpty := metadata.NewCallArgument(meta)
	kvEmpty.SetKind(metadata.KindKeyValue)
	kvEmpty.X = wsIdent(meta, "", "", "")
	kvEmpty.Fun = wsIdent(meta, "data", "", "")
	noisy.Args = []*metadata.CallArgument{kvNilVal, kvLitKey, kvEmpty}
	assert.Nil(t, fieldParamBindingsFromReturnVar(noisy, params))
}

func TestParamNameSetFromEdge(t *testing.T) {
	meta := wsMeta()
	edge := buildCallGraphEdge(meta, "h", "p", "c", "p", nil)
	edge.ParamArgMap = map[string]metadata.CallArgument{
		"a": *wsIdent(meta, "a", "", ""),
		"":  *wsIdent(meta, "blank", "", ""),
	}
	got := paramNameSetFromEdge(edge)
	assert.True(t, got["a"])
	assert.False(t, got[""], "blank param names are excluded")
}

func TestWrapperFieldIsGeneric(t *testing.T) {
	meta := wsMeta()
	typ := wsEnvelopeType(meta)
	assert.True(t, wrapperFieldIsGeneric(meta, typ, "Data"))
	assert.False(t, wrapperFieldIsGeneric(meta, typ, "Message"))
	assert.False(t, wrapperFieldIsGeneric(meta, typ, "Missing"))
	assert.False(t, wrapperFieldIsGeneric(meta, nil, "Data"))

	// Pointer-to-generic field is still generic.
	sp := meta.StringPool
	ptrType := &metadata.Type{Fields: []metadata.Field{{Name: sp.Get("P"), Type: sp.Get("*any")}}}
	assert.True(t, wrapperFieldIsGeneric(meta, ptrType, "P"))
}

func TestJsonNameForField(t *testing.T) {
	meta := wsMeta()
	typ := wsEnvelopeType(meta)
	assert.Equal(t, "data", jsonNameForField(meta, typ, "Data"))
	assert.Equal(t, "", jsonNameForField(meta, typ, "Missing"))
	assert.Equal(t, "", jsonNameForField(meta, nil, "Data"))

	// No tag -> falls back to the Go field name.
	sp := meta.StringPool
	noTag := &metadata.Type{Fields: []metadata.Field{{Name: sp.Get("Raw")}}}
	assert.Equal(t, "Raw", jsonNameForField(meta, noTag, "Raw"))
}

func TestLookupWrapperType(t *testing.T) {
	meta := wsMeta()
	meta.Packages["common"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"common.go": {Types: map[string]*metadata.Type{"Envelope": wsEnvelopeType(meta)}},
		},
	}
	assert.Nil(t, lookupWrapperType(nil, "common.Envelope"))
	assert.Nil(t, lookupWrapperType(meta, ""))
	assert.NotNil(t, lookupWrapperType(meta, "common.Envelope"))
	assert.NotNil(t, lookupWrapperType(meta, "*common.Envelope"))
	assert.Nil(t, lookupWrapperType(meta, "common.Nope"))
	assert.Nil(t, lookupWrapperType(meta, "[]"), "type string with no type name")
}

func TestFindFunction(t *testing.T) {
	meta := wsMeta()
	fn := &metadata.Function{Name: meta.StringPool.Get("F")}
	meta.Packages["p"] = &metadata.Package{
		Files: map[string]*metadata.File{"p.go": {Functions: map[string]*metadata.Function{"F": fn}}},
	}
	assert.Nil(t, findFunction(nil, "p", "F"))
	assert.Nil(t, findFunction(meta, "missing", "F"))
	assert.Nil(t, findFunction(meta, "p", "Nope"))
	assert.Same(t, fn, findFunction(meta, "p", "F"))
}

func TestFindConstructorEdge(t *testing.T) {
	meta := wsMeta()
	edge := buildCallGraphEdge(meta, "H", "p", "Ctor", "p", nil)
	meta.Callers["base"] = []*metadata.CallGraphEdge{edge}
	assert.Same(t, edge, findConstructorEdge(meta, "base", "p", "Ctor"))
	assert.Nil(t, findConstructorEdge(meta, "base", "p", "Other"))
	assert.Nil(t, findConstructorEdge(meta, "missing", "p", "Ctor"))
}

func TestMetadataFromContextProvider(t *testing.T) {
	meta := wsMeta()
	assert.Same(t, meta, metadataFromContextProvider(NewContextProvider(meta)))
	assert.Nil(t, metadataFromContextProvider(nil))
}

// --- specialiseWrapperSchema ---------------------------------------------

func TestSpecialiseWrapperSchema(t *testing.T) {
	meta := wsMeta()
	sp := meta.StringPool
	meta.Packages["common"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"common.go": {Types: map[string]*metadata.Type{"Envelope": wsEnvelopeType(meta)}},
		},
	}
	// Register the payload type so schemaForType discovers a
	// component for it (exercising the discovered-registration loop).
	meta.Packages["orders"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"orders.go": {Types: map[string]*metadata.Type{"Order": {
				Name:   sp.Get("Order"),
				Pkg:    sp.Get("orders"),
				Kind:   sp.Get("struct"),
				Fields: []metadata.Field{{Name: sp.Get("ID"), Type: sp.Get("string"), Tag: sp.Get(`json:"id"`)}},
			}}},
		},
	}
	cfg := pmcTestCfg()
	base := &Schema{Ref: "#/components/schemas/common.Envelope"}
	ovr := []wrapperFieldOverride{{StructFieldName: "Data", GoType: "orders.Order"}}

	// Guard cases return the (passed-in) schema unchanged.
	assert.Nil(t, specialiseWrapperSchema(nil, ovr, "common.Envelope", nil, meta, cfg))
	assert.Equal(t, base, specialiseWrapperSchema(base, nil, "common.Envelope", nil, meta, cfg))
	inlined := &Schema{Type: "object"}
	assert.Same(t, inlined, specialiseWrapperSchema(inlined, ovr, "common.Envelope", nil, meta, cfg))
	assert.Equal(t, base, specialiseWrapperSchema(base, ovr, "common.Envelope", nil, nil, cfg))
	assert.Equal(t, base, specialiseWrapperSchema(base, ovr, "common.Nope", nil, meta, cfg))

	// Non-generic field is skipped -> base unchanged.
	usedA := map[string]*Schema{}
	concreteOnly := []wrapperFieldOverride{{StructFieldName: "Message", GoType: "string"}}
	assert.Equal(t, base, specialiseWrapperSchema(base, concreteOnly, "common.Envelope", usedA, meta, cfg))

	// Happy path: allOf with overridden data property + component registered.
	used := map[string]*Schema{}
	got := specialiseWrapperSchema(base, ovr, "common.Envelope", used, meta, cfg)
	require.Len(t, got.AllOf, 2)
	assert.Equal(t, base, got.AllOf[0])
	require.NotNil(t, got.AllOf[1].Properties["data"])
	assert.Contains(t, used, "orders.Order", "discovered component registered")
}

// --- collectWrapperOverrides + resolveOverrideGoType -----------------------

func TestCollectWrapperOverrides_HappyPath(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	got := matcher.collectWrapperOverrides(arg, node)
	require.Len(t, got, 1, "only the generic data field resolves to a concrete type")
	assert.Equal(t, "Data", got[0].StructFieldName)
	assert.Equal(t, "orders.Order", got[0].GoType)
}

func TestCollectWrapperOverrides_Guards(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)

	// nil / non-ident arg.
	assert.Nil(t, matcher.collectWrapperOverrides(nil, node))
	lit := buildLiteralArg(meta, "x")
	assert.Nil(t, matcher.collectWrapperOverrides(lit, node))

	// nil node.
	assert.Nil(t, matcher.collectWrapperOverrides(arg, nil))

	// node with nil edge.
	assert.Nil(t, matcher.collectWrapperOverrides(arg, buildTrackerNode(nil)))

	// Unknown variable name (no assignment).
	assert.Nil(t, matcher.collectWrapperOverrides(wsIdent(meta, "unknown", "common", ""), node))
}

func TestCollectWrapperOverrides_NoCtorEdge(t *testing.T) {
	// Drop the constructor edge so findConstructorEdge returns nil.
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	meta.Callers = map[string][]*metadata.CallGraphEdge{}
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

func TestCollectWrapperOverrides_AssignWithoutCallee(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	helper := findFunction(meta, "common", "RespondWithSuccess")
	helper.AssignmentMap["response"] = []metadata.Assignment{{}} // no CalleeFunc
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

func TestCollectWrapperOverrides_ReturnIndexOutOfRange(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	helper := findFunction(meta, "common", "RespondWithSuccess")
	helper.AssignmentMap["response"] = []metadata.Assignment{
		{CalleeFunc: "NewEnvelope", CalleePkg: "common", ReturnIndex: 7},
	}
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

func TestCollectWrapperOverrides_HelperNotFound(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	// Remove the enclosing helper function from the package.
	delete(meta.Packages["common"].Files["common.go"].Functions, "RespondWithSuccess")
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

func TestCollectWrapperOverrides_CtorNotFound(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	helper := findFunction(meta, "common", "RespondWithSuccess")
	helper.AssignmentMap["response"] = []metadata.Assignment{
		{CalleeFunc: "Ghost", CalleePkg: "common", ReturnIndex: 0},
	}
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

func TestCollectWrapperOverrides_NoBindings(t *testing.T) {
	matcher, node, arg := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	ctor := findFunction(meta, "common", "NewEnvelope")
	// A constructor whose return is a bare ident has no field→param bindings.
	ctor.ReturnVars = []metadata.CallArgument{*wsIdent(meta, "x", "", "")}
	assert.Nil(t, matcher.collectWrapperOverrides(arg, node))
}

// TestExtractResponse_WrapperEnvelope drives the full ExtractResponse path so
// the envelope-specialisation hook in extractor.go is exercised: a body that
// resolves to the wrapper struct is emitted as allOf[base $ref, {data}].
func TestExtractResponse_WrapperEnvelope(t *testing.T) {
	matcher, node, _ := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)
	// Register the payload type so its component is discoverable.
	sp := meta.StringPool
	meta.Packages["orders"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"orders.go": {Types: map[string]*metadata.Type{"Order": {
				Name:   sp.Get("Order"),
				Pkg:    sp.Get("orders"),
				Kind:   sp.Get("struct"),
				Fields: []metadata.Field{{Name: sp.Get("ID"), Type: sp.Get("string"), Tag: sp.Get(`json:"id"`)}},
			}}},
		},
	}

	matcher.pattern = ResponsePattern{
		TypeFromArg:   true,
		TypeArgIndex:  0,
		DefaultStatus: 200,
	}
	route := &RouteInfo{
		Response:  map[string]*ResponseInfo{},
		UsedTypes: map[string]*Schema{},
		Metadata:  meta,
	}

	infos := matcher.ExtractResponse(node, route)
	require.Len(t, infos, 1)
	info := infos[0]
	require.NotNil(t, info.Schema)
	require.Len(t, info.Schema.AllOf, 2, "envelope response specialised to allOf")
	assert.Equal(t, "#/components/schemas/common.Envelope", info.Schema.AllOf[0].Ref)
	require.NotNil(t, info.Schema.AllOf[1].Properties["data"])
	assert.Contains(t, info.Schema.AllOf[1].Properties["data"].Ref, "orders.Order")
}

func TestResolveOverrideGoType(t *testing.T) {
	matcher, node, _ := wsGraph(t, "orders.Order")
	meta := metadataFromContextProvider(matcher.contextProvider)

	// nil arg.
	assert.Equal(t, "", matcher.resolveOverrideGoType(nil, node))

	// ident "data" resolves through the parent edge to the concrete type.
	assert.Equal(t, "orders.Order", matcher.resolveOverrideGoType(wsIdent(meta, "data", "common", ""), node))

	// ident with no parent mapping but a concrete declared type falls back.
	assert.Equal(t, "models.Thing", matcher.resolveOverrideGoType(wsIdent(meta, "other", "common", "models.Thing"), node))

	// non-ident uses the declared type directly.
	lit := buildLiteralArg(meta, "v")
	lit.SetType("models.Lit")
	assert.Equal(t, "models.Lit", matcher.resolveOverrideGoType(lit, node))
}
