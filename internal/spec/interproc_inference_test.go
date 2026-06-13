// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// reqStructMeta returns metadata holding a single struct `pkg.Req` with the
// given Go-field → json-tag pairs, ready for field-inference tests.
func reqStructMeta(t *testing.T, fields map[string]string) *metadata.Metadata {
	t.Helper()
	meta := newTestMeta()
	fs := make([]metadata.Field, 0, len(fields))
	for goName, tag := range fields {
		fs = append(fs, metadata.Field{
			Name: meta.StringPool.Get(goName),
			Tag:  meta.StringPool.Get(tag),
		})
	}
	meta.Packages = map[string]*metadata.Package{
		"pkg": {Files: map[string]*metadata.File{
			"f.go": {Types: map[string]*metadata.Type{
				"Req": {Name: meta.StringPool.Get("Req"), Fields: fs},
			}},
		}},
	}
	return meta
}

func reqRoute(meta *metadata.Metadata, props map[string]*Schema) *RouteInfo {
	return &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {Type: "object", Properties: props},
		},
	}
}

// setParamArg records a callee parameter → caller argument mapping on an edge,
// mirroring what metadata's call-graph builder produces.
func setParamArg(edge *metadata.CallGraphEdge, param string, arg *metadata.CallArgument) {
	if edge.ParamArgMap == nil {
		edge.ParamArgMap = map[string]metadata.CallArgument{}
	}
	edge.ParamArgMap[param] = *arg
}

// --- unwrapArgRefs -----------------------------------------------------------

func TestUnwrapArgRefs(t *testing.T) {
	meta := newTestMeta()
	assert.Nil(t, unwrapArgRefs(nil))

	ident := makeIdentArg(meta, "body", "pkg.Req")
	assert.Same(t, ident, unwrapArgRefs(ident), "plain ident is returned unchanged")

	addr := wrapUnary(meta, ident, "&")
	assert.Same(t, ident, unwrapArgRefs(addr), "&body unwraps to body")

	star := makeCallArg(meta)
	star.SetKind(metadata.KindStar)
	star.X = ident
	assert.Same(t, ident, unwrapArgRefs(star), "*body unwraps to body")

	// Nested *(&body) unwraps both levels.
	nested := wrapUnary(meta, star, "*")
	assert.Same(t, ident, unwrapArgRefs(nested))
}

// --- fieldNameForArg ---------------------------------------------------------

func TestFieldNameForArg(t *testing.T) {
	meta := newTestMeta()
	binding := fieldBinding{
		structVars: map[string]struct{}{"body": {}},
		fieldVars:  map[string]string{"raw": "SourceID"},
	}

	// Selector on a struct var.
	assert.Equal(t, "DestID", fieldNameForArg(mkSelArgPtr(meta, "body", "DestID"), binding))
	// *body.Opt — pointer-dereferenced selector.
	assert.Equal(t, "Opt", fieldNameForArg(wrapUnary(meta, mkSelArgPtr(meta, "body", "Opt"), "*"), binding))
	// Bare ident previously bound to a field's value.
	assert.Equal(t, "SourceID", fieldNameForArg(makeIdentArg(meta, "raw", "string"), binding))

	// Non-matches.
	assert.Empty(t, fieldNameForArg(nil, binding))
	assert.Empty(t, fieldNameForArg(mkSelArgPtr(meta, "other", "X"), binding), "selector on unknown var")
	assert.Empty(t, fieldNameForArg(makeIdentArg(meta, "unbound", "string"), binding), "ident not in fieldVars")
	assert.Empty(t, fieldNameForArg(makeLiteralArg(meta, "x"), binding))

	bareSel := makeCallArg(meta)
	bareSel.SetKind(metadata.KindSelector) // no X/Sel
	assert.Empty(t, fieldNameForArg(bareSel, binding))
}

// --- childBinding ------------------------------------------------------------

func TestChildBinding(t *testing.T) {
	meta := newTestMeta()
	parent := fieldBinding{
		structVars: map[string]struct{}{"body": {}},
		fieldVars:  map[string]string{"src": "SourceID"},
	}

	edge := makeEdge(meta, "Copy", "pkg", "helper", "pkg", nil)
	// param "whole" receives the whole struct (&body) → struct var in child.
	setParamArg(&edge, "whole", wrapUnary(meta, makeIdentArg(meta, "body", "pkg.Req"), "&"))
	// param "field" receives body.AuthID (a selector) → field var in child.
	setParamArg(&edge, "field", mkSelArgPtr(meta, "body", "AuthID"))
	// param "passthru" receives src (already a field var) → field var in child.
	setParamArg(&edge, "passthru", makeIdentArg(meta, "src", "string"))
	// param "unrelated" receives an unknown var → ignored.
	setParamArg(&edge, "unrelated", makeIdentArg(meta, "w", "http.ResponseWriter"))

	child := childBinding(&edge, parent)

	_, isStruct := child.structVars["whole"]
	assert.True(t, isStruct, "whole-struct arg makes the param a struct var")
	assert.Equal(t, "AuthID", child.fieldVars["field"], "selector arg binds param to that field")
	assert.Equal(t, "SourceID", child.fieldVars["passthru"], "field-var arg carries its field through")
	_, unrelatedStruct := child.structVars["unrelated"]
	_, unrelatedField := child.fieldVars["unrelated"]
	assert.False(t, unrelatedStruct || unrelatedField, "unrelated arg is dropped")
}

// --- propagateFieldFormats (interprocedural) ---------------------------------

// TestApplyJSONFieldConverterFormats_FieldPassedHelper covers issue #36 Repro 1
// (field-passed): the handler passes body.SourceID into a helper that runs
// uuid.Parse on the parameter. format=uuid must propagate back to sourceId.
func TestApplyJSONFieldConverterFormats_FieldPassedHelper(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{"SourceID": `json:"sourceId"`})

	// Copy → validateUUID(body.SourceID)
	callHelper := makeEdge(meta, "Copy", "pkg", "validateUUID", "pkg",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "SourceID")})
	setParamArg(&callHelper, "raw", mkSelArgPtr(meta, "body", "SourceID"))
	// validateUUID → uuid.Parse(raw)
	callParse := makeEdge(meta, "validateUUID", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{makeIdentArg(meta, "raw", "string")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		callHelper.Caller.BaseID(): {&callHelper},
		callParse.Caller.BaseID():  {&callParse},
	}

	route := reqRoute(meta, map[string]*Schema{"sourceId": {Type: "string"}})
	applyJSONFieldConverterFormats("body", "pkg.Req", callHelper.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["sourceId"].Format,
		"format must follow body.SourceID into the helper that parses it")
}

// TestApplyJSONFieldConverterFormats_StructPassedHelper covers issue #36
// Repro 1 (struct-passed): the handler passes the whole body into a helper that
// selects fields and runs uuid.Parse on them.
func TestApplyJSONFieldConverterFormats_StructPassedHelper(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{
		"DestID":   `json:"destId"`,
		"TagsetID": `json:"tagsetId,omitempty"`,
	})

	// Copy → resolveIDs(body)
	callHelper := makeEdge(meta, "Copy", "pkg", "resolveIDs", "pkg",
		[]*metadata.CallArgument{makeIdentArg(meta, "body", "pkg.Req")})
	setParamArg(&callHelper, "in", makeIdentArg(meta, "body", "pkg.Req"))
	// resolveIDs → uuid.Parse(in.DestID) and uuid.Parse(*in.TagsetID)
	callParseDest := makeEdge(meta, "resolveIDs", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "in", "DestID")})
	callParseTagset := makeEdge(meta, "resolveIDs", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{wrapUnary(meta, mkSelArgPtr(meta, "in", "TagsetID"), "*")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		callHelper.Caller.BaseID():    {&callHelper},
		callParseDest.Caller.BaseID(): {&callParseDest, &callParseTagset},
	}

	route := reqRoute(meta, map[string]*Schema{
		"destId":   {Type: "string"},
		"tagsetId": {Type: "string"},
	})
	applyJSONFieldConverterFormats("body", "pkg.Req", callHelper.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["destId"].Format)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["tagsetId"].Format,
		"pointer-dereferenced field selected inside the helper still resolves")
}

// TestApplyJSONFieldConverterFormats_SameHelperTwice verifies sibling calls into
// the same helper with different field bindings are both followed (the on-stack
// guard is unwound between siblings).
func TestApplyJSONFieldConverterFormats_SameHelperTwice(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{
		"A": `json:"a"`,
		"B": `json:"b"`,
	})

	callA := makeEdge(meta, "Copy", "pkg", "check", "pkg",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "A")})
	setParamArg(&callA, "v", mkSelArgPtr(meta, "body", "A"))
	callB := makeEdge(meta, "Copy", "pkg", "check", "pkg",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "B")})
	setParamArg(&callB, "v", mkSelArgPtr(meta, "body", "B"))
	parseInCheck := makeEdge(meta, "check", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{makeIdentArg(meta, "v", "string")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		callA.Caller.BaseID():        {&callA, &callB},
		parseInCheck.Caller.BaseID(): {&parseInCheck},
	}

	route := reqRoute(meta, map[string]*Schema{
		"a": {Type: "string"},
		"b": {Type: "string"},
	})
	applyJSONFieldConverterFormats("body", "pkg.Req", callA.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["a"].Format)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["b"].Format,
		"second call into the same helper must also be followed")
}

// TestApplyJSONFieldConverterFormats_RecursiveHelperTerminates ensures a helper
// that (transitively) calls itself can't drive infinite recursion.
func TestApplyJSONFieldConverterFormats_RecursiveHelperTerminates(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{"ID": `json:"id"`})

	// Copy → recur(body); recur → recur(in); recur → uuid.Parse(in.ID)
	callHelper := makeEdge(meta, "Copy", "pkg", "recur", "pkg",
		[]*metadata.CallArgument{makeIdentArg(meta, "body", "pkg.Req")})
	setParamArg(&callHelper, "in", makeIdentArg(meta, "body", "pkg.Req"))
	selfCall := makeEdge(meta, "recur", "pkg", "recur", "pkg",
		[]*metadata.CallArgument{makeIdentArg(meta, "in", "pkg.Req")})
	setParamArg(&selfCall, "in", makeIdentArg(meta, "in", "pkg.Req"))
	parse := makeEdge(meta, "recur", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "in", "ID")})

	meta.Callers = map[string][]*metadata.CallGraphEdge{
		callHelper.Caller.BaseID(): {&callHelper},
		selfCall.Caller.BaseID():   {&selfCall, &parse},
	}

	route := reqRoute(meta, map[string]*Schema{"id": {Type: "string"}})
	// Must terminate (and still pick up the format on the first visit).
	applyJSONFieldConverterFormats("body", "pkg.Req", callHelper.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["id"].Format)
}

// TestPropagateFieldFormats_DepthGuards exercises the empty-binding and
// depth-limit early returns directly.
func TestPropagateFieldFormats_DepthGuards(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{"ID": `json:"id"`})
	parse := makeEdge(meta, "Copy", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "ID")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{parse.Caller.BaseID(): {&parse}}
	schema := &Schema{Type: "object", Properties: map[string]*Schema{"id": {Type: "string"}}}
	fields := map[string]string{"ID": "id"}
	full := fieldBinding{structVars: map[string]struct{}{"body": {}}, fieldVars: map[string]string{}}

	// Empty binding → no-op.
	propagateFieldFormats(parse.Caller.BaseID(), fieldBinding{}, fields, schema, meta, map[string]struct{}{}, 0)
	assert.Empty(t, schema.Properties["id"].Format)

	// Over the depth limit → no-op.
	propagateFieldFormats(parse.Caller.BaseID(), full, fields, schema, meta, map[string]struct{}{}, maxFieldInferenceDepth+1)
	assert.Empty(t, schema.Properties["id"].Format)

	// In-bounds → applies.
	propagateFieldFormats(parse.Caller.BaseID(), full, fields, schema, meta, map[string]struct{}{}, 0)
	assert.Equal(t, "uuid", schema.Properties["id"].Format)
}

// TestPropagateFieldFormats_DepthCapStopsDescent verifies that at the maximum
// depth a converter is still recorded but helper calls are no longer followed.
func TestPropagateFieldFormats_DepthCapStopsDescent(t *testing.T) {
	meta := reqStructMeta(t, map[string]string{"ID": `json:"id"`})
	// Copy → helper(body); helper → uuid.Parse(in.ID)
	callHelper := makeEdge(meta, "Copy", "pkg", "helper", "pkg",
		[]*metadata.CallArgument{makeIdentArg(meta, "body", "pkg.Req")})
	setParamArg(&callHelper, "in", makeIdentArg(meta, "body", "pkg.Req"))
	parse := makeEdge(meta, "helper", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "in", "ID")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		callHelper.Caller.BaseID(): {&callHelper},
		parse.Caller.BaseID():      {&parse},
	}
	schema := &Schema{Type: "object", Properties: map[string]*Schema{"id": {Type: "string"}}}
	binding := fieldBinding{structVars: map[string]struct{}{"body": {}}, fieldVars: map[string]string{}}

	// At the cap, the Copy→helper edge must not be descended into.
	propagateFieldFormats(callHelper.Caller.BaseID(), binding, map[string]string{"ID": "id"},
		schema, meta, map[string]struct{}{}, maxFieldInferenceDepth)
	assert.Empty(t, schema.Properties["id"].Format, "helper descent is capped at maxFieldInferenceDepth")
}

func TestChildBinding_NilUnwrapSkipped(t *testing.T) {
	meta := newTestMeta()
	edge := makeEdge(meta, "Copy", "pkg", "helper", "pkg", nil)
	bareUnary := makeCallArg(meta)
	bareUnary.SetKind(metadata.KindUnary) // no X → unwrap yields nil
	setParamArg(&edge, "p", bareUnary)
	child := childBinding(&edge, fieldBinding{structVars: map[string]struct{}{"body": {}}})
	assert.True(t, child.empty(), "a param whose arg unwraps to nil is skipped")
}

// TestResolveBodyTypeThroughCallSite_WalksPastNonMatchingAncestors covers the
// loop's continue branches: an edge-less node and a non-matching call between
// the decode node and the real call site.
func TestResolveBodyTypeThroughCallSite_WalksPastNonMatchingAncestors(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	decodeEdge := makeEdge(meta, "decodeStrictJSON", "pkg", "Decode", "encoding/json",
		[]*metadata.CallArgument{makeIdentArg(meta, "dst", "any")})
	// Immediate parent: a node with no edge (skipped).
	edgeless := &TrackerNode{}
	// Next: a non-matching call (NewDecoder, not decodeStrictJSON) — skipped.
	other := makeEdge(meta, "decodeStrictJSON", "pkg", "NewDecoder", "encoding/json", nil)
	// Finally: the real call site Copy → decodeStrictJSON(&body).
	callSite := makeEdge(meta, "Copy", "pkg", "decodeStrictJSON", "pkg", nil)
	setParamArg(&callSite, "dst", wrapUnary(meta, makeIdentArg(meta, "body", "pkg.Req"), "&"))

	decodeNode := makeTrackerNode(&decodeEdge)
	otherNode := makeTrackerNode(&other)
	otherNode.Parent = makeTrackerNode(&callSite)
	edgeless.Parent = otherNode
	decodeNode.Parent = edgeless

	bodyType, varName, baseID := resolveBodyTypeThroughCallSite(decodeNode, "dst", cp)
	assert.Equal(t, "pkg.Req", bodyType)
	assert.Equal(t, "body", varName)
	assert.Equal(t, callSite.Caller.BaseID(), baseID)
}

// --- isFreeFormBodyType ------------------------------------------------------

func TestIsFreeFormBodyType(t *testing.T) {
	for _, s := range []string{"", "any", "interface{}", "interface {}", "object", "  any  "} {
		assert.Truef(t, isFreeFormBodyType(s), "%q is free-form", s)
	}
	for _, s := range []string{"pkg.Req", "string", "[]byte", "map[string]any"} {
		assert.Falsef(t, isFreeFormBodyType(s), "%q is concrete", s)
	}
}

// --- resolveBodyTypeThroughCallSite -----------------------------------------

// TestResolveBodyTypeThroughCallSite covers issue #36 Repro 2: the decode lives
// in decodeStrictJSON(dst any); walking up to the call site recovers the
// concrete &body argument and its type.
func TestResolveBodyTypeThroughCallSite(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	// Tree: Copy → decodeStrictJSON(&body) → json.Decode(dst)
	callHelper := makeEdge(meta, "Copy", "pkg", "decodeStrictJSON", "pkg",
		[]*metadata.CallArgument{wrapUnary(meta, makeIdentArg(meta, "body", "pkg.Req"), "&")})
	setParamArg(&callHelper, "dst", wrapUnary(meta, makeIdentArg(meta, "body", "pkg.Req"), "&"))
	decodeEdge := makeEdge(meta, "decodeStrictJSON", "pkg", "Decode", "encoding/json",
		[]*metadata.CallArgument{makeIdentArg(meta, "dst", "any")})

	parent := makeTrackerNode(&callHelper)
	decodeNode := makeTrackerNode(&decodeEdge)
	decodeNode.Parent = parent

	bodyType, varName, baseID := resolveBodyTypeThroughCallSite(decodeNode, "dst", cp)
	assert.Equal(t, "pkg.Req", bodyType, "concrete type recovered from the call site")
	assert.Equal(t, "body", varName, "re-anchored onto the caller's variable")
	assert.Equal(t, callHelper.Caller.BaseID(), baseID, "field inference runs in the caller frame")
}

func TestResolveBodyTypeThroughCallSite_Guards(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)

	// Nil node / empty param.
	bt, _, _ := resolveBodyTypeThroughCallSite(nil, "dst", cp)
	assert.Empty(t, bt)
	decodeEdge := makeEdge(meta, "helper", "pkg", "Decode", "encoding/json", nil)
	node := makeTrackerNode(&decodeEdge)
	bt, _, _ = resolveBodyTypeThroughCallSite(node, "", cp)
	assert.Empty(t, bt)

	// No ancestor edge invokes the enclosing helper → unresolved.
	bt, _, _ = resolveBodyTypeThroughCallSite(node, "dst", cp)
	assert.Empty(t, bt)

	// Ancestor calls the helper but maps the param to a non-ident (call expr) →
	// unresolved rather than wrong.
	callHelper := makeEdge(meta, "Copy", "pkg", "helper", "pkg", nil)
	complexArg := makeCallArg(meta)
	complexArg.SetKind(metadata.KindCall)
	setParamArg(&callHelper, "dst", complexArg)
	node.Parent = makeTrackerNode(&callHelper)
	bt, _, _ = resolveBodyTypeThroughCallSite(node, "dst", cp)
	assert.Empty(t, bt, "non-ident mapped argument is not resolved")

	// Ancestor calls the helper but has no mapping for the param → unresolved.
	callHelper2 := makeEdge(meta, "Copy", "pkg", "helper", "pkg", nil)
	node2 := makeTrackerNode(&decodeEdge)
	node2.Parent = makeTrackerNode(&callHelper2)
	bt, _, _ = resolveBodyTypeThroughCallSite(node2, "dst", cp)
	assert.Empty(t, bt, "missing ParamArgMap entry → unresolved")
}

// TestRefineBodyTypeThroughHelper exercises the matcher-level wrapper that
// drives the interprocedural body-type fallback during request extraction.
func TestRefineBodyTypeThroughHelper(t *testing.T) {
	meta := newTestMeta()
	cfg := DefaultChiConfig()
	matcher := NewRequestPatternMatcher(RequestBodyPattern{}, cfg, NewContextProvider(meta),
		NewTypeResolver(meta, cfg, NewSchemaMapper(cfg)))

	// Already concrete → returned unchanged, reqInfo untouched.
	concrete := &RequestInfo{BodyType: "pkg.Req", DecodeTargetVar: "body"}
	bt, frame := matcher.refineBodyTypeThroughHelper(nil, concrete, "pkg.Req", "frameA")
	assert.Equal(t, "pkg.Req", bt)
	assert.Equal(t, "frameA", frame)
	assert.Equal(t, "body", concrete.DecodeTargetVar)

	// Free-form but unresolvable (nil node) → returned unchanged.
	unresolved := &RequestInfo{BodyType: "any", DecodeTargetVar: "dst"}
	bt, frame = matcher.refineBodyTypeThroughHelper(nil, unresolved, "any", "frameB")
	assert.Equal(t, "any", bt)
	assert.Equal(t, "frameB", frame)
	assert.Equal(t, "any", unresolved.BodyType)

	// Free-form and resolvable through the call site → upgraded in place.
	decodeEdge := makeEdge(meta, "decodeStrictJSON", "pkg", "Decode", "encoding/json",
		[]*metadata.CallArgument{makeIdentArg(meta, "dst", "any")})
	callSite := makeEdge(meta, "Copy", "pkg", "decodeStrictJSON", "pkg", nil)
	setParamArg(&callSite, "dst", wrapUnary(meta, makeIdentArg(meta, "body", "pkg.Req"), "&"))
	decodeNode := makeTrackerNode(&decodeEdge)
	decodeNode.Parent = makeTrackerNode(&callSite)

	upgraded := &RequestInfo{BodyType: "any", DecodeTargetVar: "dst"}
	bt, frame = matcher.refineBodyTypeThroughHelper(decodeNode, upgraded, "any", "frameC")
	assert.Equal(t, "pkg.Req", bt)
	assert.Equal(t, callSite.Caller.BaseID(), frame)
	assert.Equal(t, "pkg.Req", upgraded.BodyType)
	assert.Equal(t, "body", upgraded.DecodeTargetVar, "decode target re-anchored to the caller var")
}

func TestResolveBodyTypeThroughCallSite_NoEdgeNode(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	// A node with no edge returns empty.
	bare := &TrackerNode{}
	bt, _, _ := resolveBodyTypeThroughCallSite(bare, "dst", cp)
	require.Empty(t, bt)
}
