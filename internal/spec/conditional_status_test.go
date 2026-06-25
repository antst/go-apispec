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
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// csNewErrorCall builds the RHS of `err = NewError(msg, http.<statusName>)`:
// a KindCall whose args are an opaque message ident and an http status selector.
func csNewErrorCall(meta *metadata.Metadata, statusName string) metadata.CallArgument {
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = buildIdentArg(meta, "NewError", "app")

	status := metadata.NewCallArgument(meta)
	status.SetKind(metadata.KindSelector)
	status.X = buildIdentArg(meta, "http", "")
	status.Sel = buildIdentArg(meta, statusName, "")

	call.Args = []*metadata.CallArgument{buildIdentArg(meta, "msg", "app"), status}
	return *call
}

// csSpec describes one `err = NewError(msg, http.<status>)` assignment placed in a
// CFG block.
type csSpec struct {
	status string
	block  int32
	uncond bool // Branch == nil (else a conditional if/else arm)
}

// csScenario builds a handler whose `err` variable is reassigned per the specs and
// then passed to RespondWithError(w, err) at callBlock, installs a FunctionCFG from
// the successor adjacency, and returns the matcher + the call edge. If installCFG is
// false the model is NOT installed (exercising the FR-008 degrade path).
func csScenario(t *testing.T, specs []csSpec, callBlock int32, succs [][]int32, installCFG bool) (*ResponsePatternMatcherImpl, *metadata.CallGraphEdge) {
	t.Helper()
	meta := pmcTestMeta()
	sp := meta.StringPool

	posBlocks := map[string]metadata.BlockLoc{}
	assigns := make([]metadata.Assignment, 0, len(specs))
	for i, s := range specs {
		pos := fmt.Sprintf("h.go:%d:3", 10+i)
		var br *metadata.BranchContext
		if !s.uncond {
			br = &metadata.BranchContext{BlockKind: "if-else"}
		}
		assigns = append(assigns, metadata.Assignment{
			Value:    csNewErrorCall(meta, s.status),
			Branch:   br,
			Position: sp.Get(pos),
		})
		posBlocks[pos] = metadata.BlockLoc{Block: s.block}
	}
	callPos := "h.go:50:3"
	posBlocks[callPos] = metadata.BlockLoc{Block: callBlock}
	if installCFG {
		meta.InstallFunctionCFGForTest("handler", succs, posBlocks)
	}

	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"h.go": {Functions: map[string]*metadata.Function{
				"handler": {
					Name:          sp.Get("handler"),
					Pkg:           sp.Get("app"),
					AssignmentMap: map[string][]metadata.Assignment{"err": assigns},
				},
			}},
		},
	}
	edge := buildCallGraphEdge(meta, "handler", "app", "RespondWithError", "app",
		[]*metadata.CallArgument{buildIdentArg(meta, "w", "app"), buildIdentArg(meta, "err", "app")})
	edge.Position = sp.Get(callPos)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: NewContextProvider(meta), schemaMapper: NewSchemaMapper(pmcTestCfg())},
		pattern:            ResponsePattern{StatusFromArg: true, StatusArgIndex: 1},
	}
	return matcher, edge
}

// siblingSuccs builds an n-way branch CFG: entry(0) → blocks 1..n → merge(n+1).
func siblingSuccs(n int) (succs [][]int32, merge int32) {
	succs = make([][]int32, n+2)
	entry := make([]int32, 0, n)
	for i := 1; i <= n; i++ {
		entry = append(entry, int32(i))  //nolint:gosec // small
		succs[i] = []int32{int32(n + 1)} //nolint:gosec // small
	}
	succs[0] = entry
	return succs, int32(n + 1) //nolint:gosec // small
}

func sortedStatuses(in []int) []int { s := append([]int(nil), in...); sort.Ints(s); return s }

func TestExpandStatusesFromIdent_SiblingFanOut(t *testing.T) {
	// Three mutually-exclusive sibling branches each reach the call → fan out all.
	succs, merge := siblingSuccs(3)
	matcher, edge := csScenario(t, []csSpec{
		{"StatusUnauthorized", 1, false}, {"StatusNotFound", 2, false}, {"StatusInternalServerError", 3, false},
	}, merge, succs, true)
	assert.Equal(t, []int{401, 404, 500}, sortedStatuses(matcher.expandStatusesFromIdent(edge.Args[1], edge)))
}

func TestExpandStatusesFromIdent_Guards(t *testing.T) {
	succs, merge := siblingSuccs(2)
	matcher, edge := csScenario(t, []csSpec{{"StatusNotFound", 1, false}, {"StatusForbidden", 2, false}}, merge, succs, true)
	assert.Nil(t, matcher.expandStatusesFromIdent(nil, edge), "nil arg")
	meta := matcherMeta(matcher)
	assert.Nil(t, matcher.expandStatusesFromIdent(buildLiteralArg(meta, "x"), edge), "non-ident arg")
	assert.Nil(t, matcher.expandStatusesFromIdent(edge.Args[1], nil), "nil edge")
	assert.Nil(t, matcher.expandStatusesFromIdent(buildIdentArg(meta, "ghost", "app"), edge), "unknown variable")
}

func TestExpandStatusesFromIdent_SingleAssignmentUntouched(t *testing.T) {
	succs, merge := siblingSuccs(1)
	matcher, edge := csScenario(t, []csSpec{{"StatusNotFound", 1, false}}, merge, succs, true)
	assert.Nil(t, matcher.expandStatusesFromIdent(edge.Args[1], edge), "single assignment must not fan out")
}

func TestExpandStatusesFromIdent_DedupAndNonCall(t *testing.T) {
	// Two sibling assignments of the SAME status dedup to one.
	succs, merge := siblingSuccs(2)
	matcher, edge := csScenario(t, []csSpec{{"StatusNotFound", 1, false}, {"StatusNotFound", 2, false}}, merge, succs, true)
	assert.Equal(t, []int{404}, matcher.expandStatusesFromIdent(edge.Args[1], edge))

	// A non-call assignment is skipped (still ≥2 entries, only one is a call).
	meta := matcherMeta(matcher)
	fn := findFunction(meta, "app", "handler")
	keep := fn.AssignmentMap["err"][0] // the 404 call at block 1
	fn.AssignmentMap["err"] = []metadata.Assignment{keep, {Value: *buildIdentArg(meta, "plain", "app")}}
	assert.Equal(t, []int{404}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_CallWithoutStatusSkipped(t *testing.T) {
	// A call-valued assignment whose arguments carry no HTTP status contributes
	// nothing; only the status-bearing sibling survives.
	succs, merge := siblingSuccs(2)
	matcher, edge := csScenario(t, []csSpec{{"StatusNotFound", 1, false}, {"StatusBadRequest", 2, false}}, merge, succs, true)
	meta := matcherMeta(matcher)
	fn := findFunction(meta, "app", "handler")
	noStatus := metadata.NewCallArgument(meta)
	noStatus.SetKind(metadata.KindCall)
	noStatus.Fun = buildIdentArg(meta, "Wrap", "app")
	noStatus.Args = []*metadata.CallArgument{buildIdentArg(meta, "msg", "app")}
	fn.AssignmentMap["err"][1].Value = *noStatus
	assert.Equal(t, []int{404}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_AfterCallReassignSameBlockKept(t *testing.T) {
	// One straight-line block where the call sits BETWEEN two assignments:
	// err=400 (node 0) → RespondWithError (node 1) → err=500 (node 2). The 500 store
	// executes AFTER the response write, in the same block, so it must NOT shadow the
	// 400 the call actually reads. Expect [400] (a regression guard for the kill
	// predicate's "killer must reach the call" clause).
	meta := pmcTestMeta()
	sp := meta.StringPool
	pos400, posCall, pos500 := "h.go:10:3", "h.go:11:3", "h.go:12:3"
	assigns := []metadata.Assignment{
		{Value: csNewErrorCall(meta, "StatusBadRequest"), Position: sp.Get(pos400)},
		{Value: csNewErrorCall(meta, "StatusInternalServerError"), Position: sp.Get(pos500)},
	}
	meta.InstallFunctionCFGForTest("handler", [][]int32{{}}, map[string]metadata.BlockLoc{
		pos400:  {Block: 0, Node: 0},
		posCall: {Block: 0, Node: 1},
		pos500:  {Block: 0, Node: 2},
	})
	meta.Packages["app"] = &metadata.Package{Files: map[string]*metadata.File{
		"h.go": {Functions: map[string]*metadata.Function{
			"handler": {Name: sp.Get("handler"), Pkg: sp.Get("app"), AssignmentMap: map[string][]metadata.Assignment{"err": assigns}},
		}},
	}}
	edge := buildCallGraphEdge(meta, "handler", "app", "RespondWithError", "app",
		[]*metadata.CallArgument{buildIdentArg(meta, "w", "app"), buildIdentArg(meta, "err", "app")})
	edge.Position = sp.Get(posCall)
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: NewContextProvider(meta), schemaMapper: NewSchemaMapper(pmcTestCfg())},
		pattern:            ResponsePattern{StatusFromArg: true, StatusArgIndex: 1},
	}
	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_UnconditionalShadow(t *testing.T) {
	// Straight line: 500 (block1) → 400 (block2) → call (block3). 400 dominates the
	// call and 500 reaches it, so 500 is overwritten — only 400 survives.
	succs := [][]int32{{1}, {2}, {3}, {}}
	matcher, edge := csScenario(t, []csSpec{{"StatusInternalServerError", 1, true}, {"StatusBadRequest", 2, true}}, 3, succs, true)
	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_AfterCallExcluded(t *testing.T) {
	// 400 (block1) → call (block2) → 500 (block3). 500 is after the call and cannot
	// reach it, so only 400 contributes.
	succs := [][]int32{{1}, {2}, {3}, {}}
	matcher, edge := csScenario(t, []csSpec{{"StatusBadRequest", 1, false}, {"StatusInternalServerError", 3, false}}, 2, succs, true)
	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_DegradeAndWarn(t *testing.T) {
	// No CFG installed → the call cannot be placed → degrade to the unconditionally-
	// reachable statuses (the Branch==nil one) and warn.
	matcher, edge := csScenario(t, []csSpec{{"StatusInternalServerError", 0, true}, {"StatusBadRequest", 1, false}}, 0, nil, false)
	var buf bytes.Buffer
	matcher.warnings = &WarningSink{out: &buf}
	assert.Equal(t, []int{500}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
	assert.Contains(t, buf.String(), "warning:")
}

func TestExpandStatusesFromIdent_NoMetadata(t *testing.T) {
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "handler", "app", "RespondWithError", "app",
		[]*metadata.CallArgument{buildIdentArg(meta, "err", "app")})
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: &mockContextProvider{}, schemaMapper: NewSchemaMapper(pmcTestCfg())},
		pattern:            ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
	}
	assert.Nil(t, matcher.expandStatusesFromIdent(edge.Args[0], edge))
}

// TestExtractResponse_ConditionalStatusFanOut drives the full ExtractResponse path.
func TestExtractResponse_ConditionalStatusFanOut(t *testing.T) {
	succs, merge := siblingSuccs(3)
	matcher, edge := csScenario(t, []csSpec{
		{"StatusUnauthorized", 1, false}, {"StatusNotFound", 2, false}, {"StatusInternalServerError", 3, false},
	}, merge, succs, true)
	route := &RouteInfo{Response: map[string]*ResponseInfo{}, UsedTypes: map[string]*Schema{}}
	infos := matcher.ExtractResponse(buildTrackerNode(edge), route)
	require.Len(t, infos, 3, "one response per distinct branch status")
	assert.Equal(t, []int{401, 404, 500}, sortedStatuses([]int{infos[0].StatusCode, infos[1].StatusCode, infos[2].StatusCode}))
}

func matcherMeta(m *ResponsePatternMatcherImpl) *metadata.Metadata {
	return metadataFromContextProvider(m.contextProvider)
}

func TestDedupInts(t *testing.T) {
	assert.Equal(t, []int{1, 2, 3}, dedupInts([]int{1, 2, 2, 3, 1, 3}))
	assert.Empty(t, dedupInts(nil))
}

func TestStatusFromCallArgs_NoStatus(t *testing.T) {
	meta := pmcTestMeta()
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: NewContextProvider(meta), schemaMapper: NewSchemaMapper(pmcTestCfg())},
	}
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Args = []*metadata.CallArgument{nil, buildIdentArg(meta, "notAStatus", "app")}
	_, ok := statusFromCallArgs(matcher, call)
	assert.False(t, ok)
}
