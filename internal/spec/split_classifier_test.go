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

// scExtractor builds an Extractor whose mock tree exposes meta.
func scExtractor(meta *metadata.Metadata) *Extractor {
	tree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3,
	})
	return NewExtractor(tree, &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}})
}

// scBranch builds a BranchContext at the given block whose ParentStmtPos (pos)
// resolves to a function's CFG (so splitRouteFnKey can find it).
func scBranch(meta *metadata.Metadata, block int32, pos string, methods ...string) *metadata.BranchContext {
	return &metadata.BranchContext{
		BlockKind:     "switch-case",
		BlockIndex:    block,
		CaseValues:    methods,
		ParentStmtPos: meta.StringPool.Get(pos),
	}
}

// scResp is a tiny ResponseInfo constructor for these tests.
func scResp(code int, body string, br *metadata.BranchContext) *ResponseInfo {
	return &ResponseInfo{StatusCode: code, ContentType: "application/json", BodyType: body, Branch: br}
}

// scCollect returns method → sorted status codes present on each split route.
func scCollect(routes []*RouteInfo) map[string][]int {
	out := map[string][]int{}
	for _, r := range routes {
		codes := make([]int, 0, len(r.Response))
		for _, resp := range r.Response {
			codes = append(codes, resp.StatusCode)
		}
		// insertion-sort keeps the test assertion order-stable
		for i := 1; i < len(codes); i++ {
			for j := i; j > 0 && codes[j-1] > codes[j]; j-- {
				codes[j-1], codes[j] = codes[j], codes[j-1]
			}
		}
		out[r.Method] = codes
	}
	return out
}

// switchCFG models go/cfg's lowering of `switch r.Method { case GET; case POST;
// default }`: a test chain (B0→B4→B6) with case bodies hanging off it. B2=GET body,
// B3=POST body, B5=default body, B1=SwitchDone merge. Verified against go/cfg.
func switchCFG() [][]int32 {
	return [][]int32{
		{2, 4}, // B0 tag
		{},     // B1 SwitchDone (merge)
		{1},    // B2 GET body
		{1},    // B3 POST body
		{3, 6}, // B4 SwitchNextCase (tests POST)
		{1},    // B5 default body
		{5},    // B6 SwitchNextCase (→ default)
	}
}

// TestSplit_SwitchDefaultExcluded: the `default:` 405 descends from the dispatch
// root (B0) and is mutually exclusive with both arms → excluded from GET and POST.
func TestSplit_SwitchDefaultExcluded(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 2}, "post:1": {Block: 3}, "def:1": {Block: 5},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 2, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 3, "post:1", "POST")),
			"405": scResp(405, "", scBranch(meta, 5, "def:1")),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200}, got["GET"], "GET must not carry the 405 default")
	assert.Equal(t, []int{201}, got["POST"], "POST must not carry the 405 default")
}

// TestSplit_AfterMergeIndependentShared: a conditional after the switch merge (B7,
// reachable from the arms) is reachable together with every method → shared.
func TestSplit_AfterMergeIndependentShared(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	succs := switchCFG()
	succs[1] = []int32{7}      // SwitchDone → after-block
	succs = append(succs, nil) // B7 (after-merge independent), terminal
	meta.InstallFunctionCFGForTest(fn, succs, map[string]metadata.BlockLoc{
		"get:1": {Block: 2}, "post:1": {Block: 3}, "ind:1": {Block: 7},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 2, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 3, "post:1", "POST")),
			// An if-form independent after the merge (a switch `default:` here would be
			// the dispatch fallback; this is an orthogonal `if logErr { 500 }`).
			"500": scResp(500, "Err", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 7, ParentStmtPos: meta.StringPool.Get("ind:1")}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200, 500}, got["GET"], "GET must carry the shared 500")
	assert.Equal(t, []int{201, 500}, got["POST"], "POST must carry the shared 500")
}

// ifFormCFG models `if bad {…return}; if GET {…} else if POST {…}`: the dispatch
// root is B2 (the first `if r.Method`), NOT the function entry, so the pre-dispatch
// 500 (B1) is not dominated by the root.
func ifFormCFG() [][]int32 {
	return [][]int32{
		{1, 2}, // B0 `if bad` cond
		{},     // B1 bad-then (500, returns)
		{3, 4}, // B2 `if GET` cond  (dispatch root)
		{6},    // B3 GET body
		{5, 6}, // B4 `else if POST` cond
		{6},    // B5 POST body
		{},     // B6 merge
	}
}

// TestSplit_PreDispatchIndependentShared: a pre-dispatch 500 sits outside the
// dispatch (root does not dominate it) → shared onto every method (issue F1).
func TestSplit_PreDispatchIndependentShared(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, ifFormCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 3}, "post:1": {Block: 5}, "bad:1": {Block: 1},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 3, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 5, "post:1", "POST")),
			"500": scResp(500, "Err", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 1, ParentStmtPos: meta.StringPool.Get("bad:1")}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200, 500}, got["GET"])
	assert.Equal(t, []int{201, 500}, got["POST"])
}

// nestedCFG: GET arm (B1) contains a nested conditional (B3, idom=B1); POST is B2.
func nestedCFG() [][]int32 {
	return [][]int32{
		{1, 2}, // B0 `if GET` cond
		{3, 4}, // B1 GET arm, nested if
		{5},    // B2 POST arm
		{5},    // B3 nested 404 (idom B1)
		{5},    // B4 GET 200 continuation (idom B1)
		{},     // B5 merge
	}
}

// TestSplit_NestedConditionalAttributedToMethod: a conditional dominated by the GET
// arm belongs to GET only, never POST.
func TestSplit_NestedConditionalAttributedToMethod(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, nestedCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 1}, "post:1": {Block: 2}, "nf:1": {Block: 3},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 1, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 2, "post:1", "POST")),
			"404": scResp(404, "NF", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 3, ParentStmtPos: meta.StringPool.Get("nf:1")}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200, 404}, got["GET"], "nested 404 belongs to GET")
	assert.Equal(t, []int{201}, got["POST"], "POST must not carry GET's nested 404")
}

// TestSplit_NoCFGModelDegrades: with no CFG model, a non-method conditional is
// conservatively excluded (never leaked onto a method as a phantom).
func TestSplit_NoCFGModelDegrades(t *testing.T) {
	meta := newTestMeta()
	ext := scExtractor(meta) // no InstallFunctionCFGForTest → fnKey == ""
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", &metadata.BranchContext{BlockKind: "switch-case", BlockIndex: 2, CaseValues: []string{"GET"}}),
			"201": scResp(201, "Made", &metadata.BranchContext{BlockKind: "switch-case", BlockIndex: 3, CaseValues: []string{"POST"}}),
			"500": scResp(500, "Err", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 5}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200}, got["GET"])
	assert.Equal(t, []int{201}, got["POST"])
}

// TestSplit_UnconditionalShared: a Branch==nil response is shared onto all methods.
func TestSplit_UnconditionalShared(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 2}, "post:1": {Block: 3},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 2, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 3, "post:1", "POST")),
			"400": scResp(400, "Err", nil), // unconditional
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200, 400}, got["GET"])
	assert.Equal(t, []int{201, 400}, got["POST"])
}

// combinedCFG: a combined `case "GET","HEAD"` arm (B1) holds a nested conditional
// (B3, idom=B1); POST is B2. The arm block B1 serves both GET and HEAD.
func combinedCFG() [][]int32 {
	return [][]int32{
		{1, 2}, // B0 dispatch
		{3, 4}, // B1 GET/HEAD arm, nested if
		{5},    // B2 POST arm
		{5},    // B3 nested 404 (idom B1)
		{5},    // B4 GET/HEAD 200 continuation
		{},     // B5 merge
	}
}

// TestSplit_CombinedMethodCase: a conditional nested in a combined `case "GET",
// "HEAD"` arm is attributed to BOTH methods, deterministically (not whichever the
// map happened to map the shared arm block to).
func TestSplit_CombinedMethodCase(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, combinedCFG(), map[string]metadata.BlockLoc{
		"gh:1": {Block: 1}, "post:1": {Block: 2}, "nf:1": {Block: 3},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 1, "gh:1", "GET", "HEAD")),
			"201": scResp(201, "Made", scBranch(meta, 2, "post:1", "POST")),
			"404": scResp(404, "NF", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 3, ParentStmtPos: meta.StringPool.Get("nf:1")}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200, 404}, got["GET"], "nested 404 attributed to GET")
	assert.Equal(t, []int{200, 404}, got["HEAD"], "AND to HEAD (combined case)")
	assert.Equal(t, []int{201}, got["POST"], "POST unaffected")
}

// fallthroughCFG: the POST arm (B3) FALLS THROUGH into the default body (B5), so
// the default is reachable from an arm (mutual exclusivity fails) — it must still
// be excluded, recognised structurally as the switch `default:`.
func fallthroughCFG() [][]int32 {
	return [][]int32{
		{2, 4}, // B0 tag
		{},     // B1 SwitchDone
		{1},    // B2 GET body → merge
		{5},    // B3 POST body → default (fallthrough!)
		{3, 6}, // B4 SwitchNextCase
		{1},    // B5 default body (405) → merge
		{5},    // B6 SwitchNextCase → default
	}
}

// TestSplit_FallthroughDefaultExcluded: a `fallthrough` into the default must not
// leak the 405 onto the handled methods (regression guard for the inner-loop find).
func TestSplit_FallthroughDefaultExcluded(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, fallthroughCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 2}, "post:1": {Block: 3}, "def:1": {Block: 5},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 2, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 3, "post:1", "POST")),
			"405": scResp(405, "", scBranch(meta, 5, "def:1")), // switch-case, empty case values
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200}, got["GET"], "405 must not leak onto GET via fallthrough")
	assert.Equal(t, []int{201}, got["POST"], "405 must not leak onto POST via fallthrough")
}

// TestSplit_ForeignBranchExcluded: a non-method response whose branch belongs to a
// DIFFERENT function (an interprocedural helper) must be excluded, never reasoned
// about against the handler's CFG — else its helper-local block index aliases a
// handler block and leaks the response onto a method it does not belong to.
func TestSplit_ForeignBranchExcluded(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{
		"get:1": {Block: 2}, "post:1": {Block: 3},
	})
	// A separate helper function whose branch position resolves to "helper", not fn.
	meta.InstallFunctionCFGForTest("helper", [][]int32{{1}, {}}, map[string]metadata.BlockLoc{
		"helper:1": {Block: 1},
	})
	ext := scExtractor(meta)
	route := &RouteInfo{
		Path: "/r", Function: "h", UsedTypes: map[string]*Schema{},
		Response: map[string]*ResponseInfo{
			"200": scResp(200, "List", scBranch(meta, 2, "get:1", "GET")),
			"201": scResp(201, "Made", scBranch(meta, 3, "post:1", "POST")),
			"599": scResp(599, "Trace", &metadata.BranchContext{BlockKind: "if-then", BlockIndex: 1, ParentStmtPos: meta.StringPool.Get("helper:1")}),
		},
	}
	got := scCollect(ext.splitByConditionalMethods(route))
	assert.Equal(t, []int{200}, got["GET"], "foreign 599 must not appear on GET")
	assert.Equal(t, []int{201}, got["POST"], "foreign 599 must not leak onto POST")
}

// --- direct helper unit tests ---

func TestCommonDominator(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{"x": {Block: 2}})

	root, ok := commonDominator(meta, fn, []int32{2, 3})
	require.True(t, ok)
	assert.Equal(t, int32(0), root, "LCA of the GET/POST arms is the switch tag B0")

	single, ok := commonDominator(meta, fn, []int32{3})
	require.True(t, ok)
	assert.Equal(t, int32(3), single)

	_, ok = commonDominator(meta, "", []int32{2, 3})
	assert.False(t, ok, "no fnKey → no root")
	_, ok = commonDominator(meta, fn, nil)
	assert.False(t, ok, "no blocks → no root")

	// An unreachable arm block (idom = -1) forces the ancestor walk to break
	// without meeting a common ancestor; the call still returns, never panics.
	um := newTestMeta()
	um.InstallFunctionCFGForTest("u", [][]int32{{1}, {}, {}}, map[string]metadata.BlockLoc{"x": {Block: 0}})
	r2, ok := commonDominator(um, "u", []int32{1, 2})
	assert.True(t, ok)
	assert.Equal(t, int32(2), r2)
}

func TestMutuallyExclusiveWithArms(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{"x": {Block: 2}})

	assert.True(t, mutuallyExclusiveWithArms(meta, fn, 5, []int32{2, 3}), "default arm shares no path with the arms")
	assert.False(t, mutuallyExclusiveWithArms(meta, fn, 0, []int32{2, 3}), "the tag reaches both arms")
	assert.False(t, mutuallyExclusiveWithArms(meta, fn, 2, []int32{2, 3}), "an arm block is not exclusive with itself")
}

func TestDominatingMethods(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, nestedCFG(), map[string]metadata.BlockLoc{"x": {Block: 1}})

	armBlocks := []int32{1, 2}
	byBlock := map[int32][]string{1: {"GET"}, 2: {"POST"}}
	m, ok := dominatingMethods(meta, fn, 3, armBlocks, byBlock)
	require.True(t, ok)
	assert.Equal(t, []string{"GET"}, m, "B3 is dominated by the GET arm B1")

	_, ok = dominatingMethods(meta, fn, 2, armBlocks, byBlock)
	assert.False(t, ok, "POST arm dominates no other method's region")
}

func TestBranchNamesMethod(t *testing.T) {
	assert.True(t, branchNamesMethod([]string{"GET"}))
	assert.True(t, branchNamesMethod([]string{"post"}), "case-insensitive")
	assert.True(t, branchNamesMethod([]string{"active", "DELETE"}))
	assert.False(t, branchNamesMethod([]string{"active"}))
	assert.False(t, branchNamesMethod(nil))
}

func TestDispatchArms(t *testing.T) {
	mr := map[string]map[string]*ResponseInfo{
		// GET has two statuses in the SAME arm block (exercises the per-method dedup).
		"GET":  {"200": scResp(200, "L", &metadata.BranchContext{BlockIndex: 2}), "204": scResp(204, "", &metadata.BranchContext{BlockIndex: 2})},
		"HEAD": {"200": scResp(200, "L", &metadata.BranchContext{BlockIndex: 2})}, // combined arm: same block
		"POST": {"201": scResp(201, "M", &metadata.BranchContext{BlockIndex: 3})},
		"NIL":  {"500": scResp(500, "", nil)}, // a branchless entry is skipped (defensive)
	}
	blocks, byBlock := dispatchArms(mr)
	assert.Equal(t, []int32{2, 3}, blocks, "blocks sorted")
	assert.Equal(t, []string{"GET", "HEAD"}, byBlock[2], "a combined case serves both methods, sorted (GET deduped)")
	assert.Equal(t, []string{"POST"}, byBlock[3])
}

func TestSplitRouteFnKey(t *testing.T) {
	meta := newTestMeta()
	const fn = "fn"
	meta.InstallFunctionCFGForTest(fn, switchCFG(), map[string]metadata.BlockLoc{"get:1": {Block: 2}})

	resolved := map[string]map[string]*ResponseInfo{
		"GET": {"200": scResp(200, "L", scBranch(meta, 2, "get:1", "GET"))},
	}
	assert.Equal(t, fn, splitRouteFnKey(meta, resolved))

	unresolved := map[string]map[string]*ResponseInfo{
		"GET": {"200": scResp(200, "L", &metadata.BranchContext{BlockIndex: 2, ParentStmtPos: meta.StringPool.Get("nowhere:9")})},
	}
	assert.Equal(t, "", splitRouteFnKey(meta, unresolved))

	nilBranch := map[string]map[string]*ResponseInfo{"GET": {"200": scResp(200, "L", nil)}}
	assert.Equal(t, "", splitRouteFnKey(meta, nilBranch))
}
