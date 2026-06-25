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
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

func tsNamed(name string) *metadata.TypeRef {
	return &metadata.TypeRef{Kind: metadata.RefNamed, Name: name, Pkg: "app"}
}

func tsPtr(elem *metadata.TypeRef) *metadata.TypeRef {
	return &metadata.TypeRef{Kind: metadata.RefPointer, Elem: elem}
}

// tsArmWrite builds a WriteHeader response edge in a type-switch arm: caseRefs are
// the arm's case types (nil ⇒ the default arm), operand is the switched variable.
func tsArmWrite(meta *metadata.Metadata, pos string, caseRefs []*metadata.TypeRef, operand string) metadata.CallGraphEdge {
	edge := metadata.CallGraphEdge{
		// Caller is the host helper "Respond" (app): the arm write lives in the
		// helper body whose type-switch declares it, which collectSwitchArms scopes by.
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("Respond"), Pkg: meta.StringPool.Get("app")},
		Callee: metadata.Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("WriteHeader"),
			Pkg:      meta.StringPool.Get("net/http"),
			RecvType: meta.StringPool.Get("ResponseWriter"),
			Position: meta.StringPool.Get(pos),
		},
		Position: meta.StringPool.Get(pos),
		Args:     []*metadata.CallArgument{makeIdentArg(meta, "code", "int")},
		Branch:   &metadata.BranchContext{BlockKind: "switch-case", CaseTypeRefs: caseRefs, SwitchOperand: operand},
	}
	edge.Callee.Edge = &edge
	return edge
}

// tsHostTree builds a route → Respond(w, x) host → {404 arm, 200 arm, default arm}
// tree, where x is bound to the given argument TypeRef. Returns the extractor, root,
// and the three arm edges.
func tsHostTree(t *testing.T, argRef *metadata.TypeRef) (*Extractor, *TrackerNode, metadata.CallGraphEdge, metadata.CallGraphEdge, metadata.CallGraphEdge) {
	t.Helper()
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	xarg := metadata.NewCallArgument(meta)
	xarg.SetKind(metadata.KindIdent)
	xarg.SetName("x")
	xarg.TypeRef = argRef
	host := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{"x": *xarg})

	wh404 := tsArmWrite(meta, "h.go:10:3", []*metadata.TypeRef{tsPtr(tsNamed("NotFoundError"))}, "x")
	wh200 := tsArmWrite(meta, "h.go:13:3", []*metadata.TypeRef{tsPtr(tsNamed("SuccessBody"))}, "x")
	whDef := tsArmWrite(meta, "h.go:16:3", nil, "x")

	root := &TrackerNode{key: "route", Children: []*TrackerNode{
		{key: "respond", CallGraphEdge: &host, Children: []*TrackerNode{
			{key: "wh404", CallGraphEdge: &wh404},
			// Nest the 200 + default writes under an inner encoder node to prove the
			// recursive arm collection reaches non-direct children.
			{key: "enc", CallGraphEdge: &wh200, Children: []*TrackerNode{
				{key: "whdef", CallGraphEdge: &whDef},
			}},
		}},
	}}
	return ext, root, wh404, wh200, whDef
}

func TestHelperTypeSwitchEdges_PreciseMatch(t *testing.T) {
	// x is *NotFoundError → only the 404 arm survives; the 200 arm and default arm
	// are filtered, no warning.
	ext, root, wh404, wh200, whDef := tsHostTree(t, tsPtr(tsNamed("NotFoundError")))
	var buf bytes.Buffer
	ext.warnings = &WarningSink{out: &buf}

	filtered := ext.helperTypeSwitchEdges(root)
	assert.False(t, filtered[wh404.Callee.ID()], "matched 404 arm kept")
	assert.True(t, filtered[wh200.Callee.ID()], "unmatched 200 arm filtered")
	assert.True(t, filtered[whDef.Callee.ID()], "default arm filtered on a precise match")
	assert.Empty(t, buf.String(), "no warning on a precise match")
}

func TestHelperTypeSwitchEdges_ImpreciseDegrades(t *testing.T) {
	// x is an interface → degrade: both typed arms filtered, default kept, warn.
	ext, root, wh404, wh200, whDef := tsHostTree(t, &metadata.TypeRef{Kind: metadata.RefInterface})
	var buf bytes.Buffer
	ext.warnings = &WarningSink{out: &buf}

	filtered := ext.helperTypeSwitchEdges(root)
	assert.True(t, filtered[wh404.Callee.ID()], "typed 404 arm filtered")
	assert.True(t, filtered[wh200.Callee.ID()], "typed 200 arm filtered")
	assert.False(t, filtered[whDef.Callee.ID()], "default arm kept")
	assert.Contains(t, buf.String(), "type-switch", "imprecise binding warns")
}

func TestHelperTypeSwitchEdges_ConcreteNoMatchHitsDefault(t *testing.T) {
	// x is a concrete type matching no arm → it hits the default: typed arms
	// filtered, default kept, but NO warning (the binding is precise).
	ext, root, wh404, wh200, whDef := tsHostTree(t, tsPtr(tsNamed("OtherType")))
	var buf bytes.Buffer
	ext.warnings = &WarningSink{out: &buf}

	filtered := ext.helperTypeSwitchEdges(root)
	assert.True(t, filtered[wh404.Callee.ID()])
	assert.True(t, filtered[wh200.Callee.ID()])
	assert.False(t, filtered[whDef.Callee.ID()], "default arm kept")
	assert.Empty(t, buf.String(), "a concrete type that hits the default does not warn")
}

func TestHelperTypeSwitchEdges_NonHostInnerNodeSkipped(t *testing.T) {
	// The arm writes sit under an inner node whose callee does NOT have the switched
	// operand as a parameter — it must not filter anything.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	inner := makeHelperEdge(meta, "NewEncoder", "json", map[string]metadata.CallArgument{
		"w": *makeIdentArg(meta, "w", "http.ResponseWriter"),
	})
	wh404 := tsArmWrite(meta, "h.go:10:3", []*metadata.TypeRef{tsPtr(tsNamed("NotFoundError"))}, "x")
	root := &TrackerNode{key: "route", Children: []*TrackerNode{
		{key: "enc", CallGraphEdge: &inner, Children: []*TrackerNode{
			{key: "wh404", CallGraphEdge: &wh404},
		}},
	}}
	assert.Empty(t, ext.helperTypeSwitchEdges(root), "inner non-host node filters nothing")
}

func TestHelperTypeSwitchEdges_ArmScopedToHostFunction(t *testing.T) {
	// An arm write whose Caller is a DIFFERENT function (a nested helper's switch
	// that happens to share the operand name "x") must NOT be bound against this
	// host's call-site argument — it is left to its own binding pass.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	host := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{
		"x": *makeIdentArg(meta, "x", "any"),
	})
	foreign := tsArmWrite(meta, "h.go:9:3", []*metadata.TypeRef{tsPtr(tsNamed("NotFoundError"))}, "x")
	foreign.Caller = metadata.Call{Meta: meta, Name: meta.StringPool.Get("Nested"), Pkg: meta.StringPool.Get("app")}
	root := &TrackerNode{key: "route", Children: []*TrackerNode{
		{key: "respond", CallGraphEdge: &host, Children: []*TrackerNode{
			{key: "foreign", CallGraphEdge: &foreign},
		}},
	}}
	assert.Empty(t, ext.helperTypeSwitchEdges(root), "a nested helper's arm is not bound by the outer host")
}

func TestHelperTypeSwitchEdges_NestedArmBailsKeepsAll(t *testing.T) {
	// The host helper has a response write under an if-then branch — a type-switch
	// arm with nested control flow whose write go/cfg attributes to the inner branch,
	// not the arm. The binder cannot attribute every write to its arm, so it must
	// keep ALL arms (filter nothing) — a safe over-approximation, never a mis-bind.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	xarg := metadata.NewCallArgument(meta)
	xarg.SetKind(metadata.KindIdent)
	xarg.SetName("x")
	xarg.TypeRef = tsPtr(tsNamed("NotFoundError")) // concrete: would normally bind the 404 arm
	host := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{"x": *xarg})

	wh404 := tsArmWrite(meta, "h.go:10:3", []*metadata.TypeRef{tsPtr(tsNamed("NotFoundError"))}, "x")
	wh200 := tsArmWrite(meta, "h.go:13:3", []*metadata.TypeRef{tsPtr(tsNamed("SuccessBody"))}, "x")
	whNested := tsArmWrite(meta, "h.go:16:3", nil, "")
	whNested.Branch = &metadata.BranchContext{BlockKind: "if-then"} // escaped: nested control flow
	root := &TrackerNode{key: "route", Children: []*TrackerNode{
		{key: "respond", CallGraphEdge: &host, Children: []*TrackerNode{
			{key: "wh404", CallGraphEdge: &wh404},
			{key: "wh200", CallGraphEdge: &wh200},
			{key: "whnested", CallGraphEdge: &whNested},
		}},
	}}
	// Without the bail, the precise *NotFoundError match would filter the 200 arm;
	// with it, nothing is filtered.
	assert.Empty(t, ext.helperTypeSwitchEdges(root), "nested-arm control flow → keep all arms, filter nothing")
}

func TestHelperTypeSwitchEdges_MultiCallSiteKeepsBoth(t *testing.T) {
	// One route invokes the same type-switch helper from two sites with different
	// concrete args. The arm writes share Callee IDs (same helper-internal call
	// position), so the per-site filter decisions must INTERSECT: each site's matched
	// arm survives; only the arm no site keeps (the default) is filtered. Without the
	// kept-set, the union would erase every arm and the operation would lose all
	// responses.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	site := func(argRef *metadata.TypeRef) *TrackerNode {
		xarg := metadata.NewCallArgument(meta)
		xarg.SetKind(metadata.KindIdent)
		xarg.SetName("x")
		xarg.TypeRef = argRef
		host := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{"x": *xarg})
		// SHARED positions across both sites → colliding Callee IDs (one helper body).
		wh404 := tsArmWrite(meta, "h.go:10:3", []*metadata.TypeRef{tsPtr(tsNamed("NotFoundError"))}, "x")
		wh200 := tsArmWrite(meta, "h.go:13:3", []*metadata.TypeRef{tsPtr(tsNamed("SuccessBody"))}, "x")
		whDef := tsArmWrite(meta, "h.go:16:3", nil, "x")
		return &TrackerNode{key: "respond", CallGraphEdge: &host, Children: []*TrackerNode{
			{key: "wh404", CallGraphEdge: &wh404},
			{key: "wh200", CallGraphEdge: &wh200},
			{key: "whdef", CallGraphEdge: &whDef},
		}}
	}
	siteA := site(tsPtr(tsNamed("NotFoundError")))
	siteB := site(tsPtr(tsNamed("SuccessBody")))
	root := &TrackerNode{key: "route", Children: []*TrackerNode{siteA, siteB}}

	filtered := ext.helperTypeSwitchEdges(root)
	wh404ID := siteA.Children[0].GetEdge().Callee.ID()
	wh200ID := siteA.Children[1].GetEdge().Callee.ID()
	whDefID := siteA.Children[2].GetEdge().Callee.ID()
	assert.False(t, filtered[wh404ID], "404 kept by site A → must survive")
	assert.False(t, filtered[wh200ID], "200 kept by site B → must survive")
	assert.True(t, filtered[whDefID], "default kept by no site → filtered")
}

func TestHelperTypeSwitchEdges_NilRoot(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	assert.Empty(t, ext.helperTypeSwitchEdges(nil))
}

func TestRefMatchesAnyCase(t *testing.T) {
	nf := tsNamed("NotFoundError")
	ptrNF := tsPtr(nf)
	// Same shape binds: *T arg ↔ case *T, T arg ↔ case T.
	assert.True(t, refMatchesAnyCase([]*metadata.TypeRef{ptrNF}, ptrNF))
	assert.True(t, refMatchesAnyCase([]*metadata.TypeRef{nf}, nf))
	// Pointer-ness MUST match (Go type-switch semantics): *T arg does not bind
	// case T, and T arg does not bind case *T — no over-approximation (FR-012).
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{nf}, ptrNF), "*T arg must not bind case T")
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{ptrNF}, nf), "T arg must not bind case *T")
	// Among several cases, the shape-matching one binds.
	assert.True(t, refMatchesAnyCase([]*metadata.TypeRef{tsNamed("Other"), ptrNF}, ptrNF))
	// Negatives: wrong name, nil list, same name different package, non-named arg.
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{tsNamed("Other")}, nf))
	assert.False(t, refMatchesAnyCase(nil, nf))
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{{Kind: metadata.RefNamed, Name: "NotFoundError", Pkg: "other"}}, nf))
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{nf}, &metadata.TypeRef{Kind: metadata.RefInterface}))

	// Slice/array/map case types bind exactly (regression for the pointer-only gap):
	// a []T arg matches `case []T` but not `case T` or `case []U`.
	sliceItem := &metadata.TypeRef{Kind: metadata.RefSlice, Elem: tsNamed("Item")}
	assert.True(t, refMatchesAnyCase([]*metadata.TypeRef{sliceItem}, sliceItem))
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{tsNamed("Item")}, sliceItem), "[]T arg must not bind case T")
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{{Kind: metadata.RefSlice, Elem: tsNamed("Other")}}, sliceItem))

	// Generic instantiations are distinguished by their type Args: Box[User] binds
	// case Box[User] but NOT case Box[Order].
	boxUser := &metadata.TypeRef{Kind: metadata.RefNamed, Name: "Box", Pkg: "app", Args: []*metadata.TypeRef{tsNamed("User")}}
	boxOrder := &metadata.TypeRef{Kind: metadata.RefNamed, Name: "Box", Pkg: "app", Args: []*metadata.TypeRef{tsNamed("Order")}}
	assert.True(t, refMatchesAnyCase([]*metadata.TypeRef{boxUser}, boxUser))
	assert.False(t, refMatchesAnyCase([]*metadata.TypeRef{boxOrder}, boxUser), "Box[User] must not bind case Box[Order]")
}

func TestTypeRefShapeEqual(t *testing.T) {
	nf := tsNamed("NotFoundError")
	assert.True(t, typeRefShapeEqual(nil, nil))
	assert.False(t, typeRefShapeEqual(nf, nil))
	assert.False(t, typeRefShapeEqual(nil, nf))
	assert.True(t, typeRefShapeEqual(tsPtr(nf), tsPtr(tsNamed("NotFoundError"))))
	assert.False(t, typeRefShapeEqual(tsPtr(nf), nf)) // *T != T
	// map[string]Item vs map[string]Other differ by value; vs map[int]Item by key.
	mSI := &metadata.TypeRef{Kind: metadata.RefMap, Key: tsNamed("string"), Elem: tsNamed("Item")}
	assert.True(t, typeRefShapeEqual(mSI, &metadata.TypeRef{Kind: metadata.RefMap, Key: tsNamed("string"), Elem: tsNamed("Item")}))
	assert.False(t, typeRefShapeEqual(mSI, &metadata.TypeRef{Kind: metadata.RefMap, Key: tsNamed("string"), Elem: tsNamed("Other")}))
	assert.False(t, typeRefShapeEqual(mSI, &metadata.TypeRef{Kind: metadata.RefMap, Key: tsNamed("int"), Elem: tsNamed("Item")}))
	// arrays differ by length.
	a3 := &metadata.TypeRef{Kind: metadata.RefArray, Len: 3, Elem: tsNamed("Item")}
	assert.False(t, typeRefShapeEqual(a3, &metadata.TypeRef{Kind: metadata.RefArray, Len: 4, Elem: tsNamed("Item")}))
}

func TestIsImpreciseLeaf(t *testing.T) {
	assert.True(t, isImpreciseLeaf(nil))
	assert.True(t, isImpreciseLeaf(&metadata.TypeRef{Kind: metadata.RefInterface}))
	for _, n := range []string{"", "error", "any", "interface{}"} {
		assert.True(t, isImpreciseLeaf(&metadata.TypeRef{Kind: metadata.RefNamed, Name: n}), n)
	}
	assert.False(t, isImpreciseLeaf(tsNamed("NotFoundError")))
}

func TestBindHelperTypeSwitch_Guards(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	ae := armEdges{filtered: map[string]bool{}, kept: map[string]bool{}}

	// No edge → no-op.
	ext.bindHelperTypeSwitch(&TrackerNode{key: "x"}, ae)
	assert.Empty(t, ae.filtered)
	assert.Empty(t, ae.kept)

	// Edge with no ParamArgMap → no-op.
	noParams := makeHelperEdge(meta, "Respond", "app", nil)
	ext.bindHelperTypeSwitch(&TrackerNode{key: "r", CallGraphEdge: &noParams}, ae)
	assert.Empty(t, ae.filtered)
}

func TestBindHelperTypeSwitch_DefaultOnlyArmIgnored(t *testing.T) {
	// A switch with only a default arm (no typed cases) has nothing to bind.
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	host := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{
		"x": *makeIdentArg(meta, "x", "any"),
	})
	whDef := tsArmWrite(meta, "h.go:9:3", nil, "x")
	root := &TrackerNode{key: "route", Children: []*TrackerNode{
		{key: "respond", CallGraphEdge: &host, Children: []*TrackerNode{
			{key: "whdef", CallGraphEdge: &whDef},
		}},
	}}
	assert.Empty(t, ext.helperTypeSwitchEdges(root))
}

func TestClassifyHelperWrites_Partition(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)

	uncond := tsArmWrite(meta, "h.go:1:3", nil, "")
	uncond.Branch = nil // unconditional response write
	// An if-then write is a #27 defensive fallback (conditional). A switch-case arm
	// is NOT a #27 fallback — it is owned by the type-switch binding — so it is
	// excluded from both buckets here.
	ifThen := tsArmWrite(meta, "h.go:2:3", nil, "")
	ifThen.Branch = &metadata.BranchContext{BlockKind: "if-then"}
	switchArm := tsArmWrite(meta, "h.go:3:3", []*metadata.TypeRef{tsPtr(tsNamed("T"))}, "x")
	nonResp := makeHelperEdge(meta, "doWork", "app", nil) // not a response write

	// The host is the Respond helper; tsArmWrite tags each write's Caller as
	// Respond/app, which classifyHelperWrites scopes to via sameFunc.
	hostEdge := makeHelperEdge(meta, "Respond", "app", map[string]metadata.CallArgument{"x": {Meta: meta}})
	host := &TrackerNode{key: "h", CallGraphEdge: &hostEdge, Children: []*TrackerNode{
		{key: "nil"}, // nil edge → skipped
		{key: "uncond", CallGraphEdge: &uncond},
		{key: "ifthen", CallGraphEdge: &ifThen},
		{key: "switcharm", CallGraphEdge: &switchArm},
		{key: "nonresp", CallGraphEdge: &nonResp},
	}}
	hw := ext.classifyHelperWrites(host)
	assert.Len(t, hw.unconditional, 1)
	assert.Len(t, hw.conditional, 1, "only the if-then write is a #27 fallback; the switch-case arm is excluded")
	assert.True(t, hw.hasUnconditional())
}

func TestBoundArgRef(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	node := buildTrackerNode(buildCallGraphEdge(meta, "h", "app", "Respond", "app", nil))

	// Structured TypeRef present → precise, FULL ref returned (pointer preserved).
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.TypeRef = tsPtr(tsNamed("NotFoundError"))
	ref, precise := ext.boundArgRef(arg, node)
	assert.True(t, precise)
	if assert.NotNil(t, ref) {
		assert.Equal(t, metadata.RefPointer, ref.Kind)
		assert.Equal(t, "NotFoundError", ref.NamedLeaf().Name)
	}

	// Interface TypeRef → imprecise (nil ref).
	argI := metadata.NewCallArgument(meta)
	argI.SetKind(metadata.KindIdent)
	argI.TypeRef = &metadata.TypeRef{Kind: metadata.RefInterface}
	ref, precise = ext.boundArgRef(argI, node)
	assert.False(t, precise)
	assert.Nil(t, ref)

	// No structured TypeRef → fall back to the string-based origin (ResolvedType).
	argS := metadata.NewCallArgument(meta)
	argS.SetKind(metadata.KindIdent)
	argS.SetResolvedType("*app.NotFoundError")
	argS.TypeRef = nil
	argS.ResolvedTypeRef = nil
	ref, precise = ext.boundArgRef(argS, node)
	assert.True(t, precise)
	if assert.NotNil(t, ref) {
		assert.Equal(t, "NotFoundError", ref.NamedLeaf().Name)
	}
}
