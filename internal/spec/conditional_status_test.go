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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// csNewErrorCall builds the RHS of `err = NewError(msg, http.<statusName>)`:
// a KindCall whose args are an opaque message ident and an http status
// selector.
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

// csMatcher builds a response matcher whose status arg is an opaque error
// (RespondWithError(w, err) -> StatusArgIndex 1) and registers a handler whose
// `err` variable is reassigned across branches.
func csMatcher(t *testing.T, branchStatuses []string) (*ResponsePatternMatcherImpl, *TrackerNode) {
	t.Helper()
	meta := pmcTestMeta()
	sp := meta.StringPool

	assigns := make([]metadata.Assignment, 0, len(branchStatuses))
	for _, name := range branchStatuses {
		// These mirror `err = NewError(msg, http.Status…)` in distinct if/else
		// branches, so each carries a (conditional) BranchContext — sibling
		// branches don't shadow one another, so the fan-out keeps every status.
		assigns = append(assigns, metadata.Assignment{
			Value:  csNewErrorCall(meta, name),
			Branch: &metadata.BranchContext{BlockKind: "if-else"},
		})
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

	cp := NewContextProvider(meta)
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: cp, schemaMapper: NewSchemaMapper(pmcTestCfg())},
		pattern:            ResponsePattern{StatusFromArg: true, StatusArgIndex: 1},
	}
	return matcher, buildTrackerNode(edge)
}

func TestExpandStatusesFromIdent(t *testing.T) {
	matcher, node := csMatcher(t, []string{"StatusUnauthorized", "StatusNotFound", "StatusInternalServerError"})
	edge := node.GetEdge()
	errArg := edge.Args[1]

	got := matcher.expandStatusesFromIdent(errArg, edge)
	assert.Equal(t, []int{401, 404, 500}, got)

	// Guards.
	assert.Nil(t, matcher.expandStatusesFromIdent(nil, edge), "nil arg")
	assert.Nil(t, matcher.expandStatusesFromIdent(buildLiteralArg(matcherMeta(matcher), "x"), edge), "non-ident arg")
	assert.Nil(t, matcher.expandStatusesFromIdent(errArg, nil), "nil edge")

	// Unknown variable (no assignments) -> nil.
	meta := matcherMeta(matcher)
	assert.Nil(t, matcher.expandStatusesFromIdent(buildIdentArg(meta, "ghost", "app"), edge))
}

func TestExpandStatusesFromIdent_SingleAssignmentUntouched(t *testing.T) {
	// A single assignment must NOT fan out (latest-wins is preserved upstream).
	matcher, node := csMatcher(t, []string{"StatusNotFound"})
	assert.Nil(t, matcher.expandStatusesFromIdent(node.GetEdge().Args[1], node.GetEdge()))
}

func TestExpandStatusesFromIdent_DedupAndNonCall(t *testing.T) {
	matcher, node := csMatcher(t, []string{"StatusNotFound", "StatusNotFound"})
	// Two assignments, same status -> deduped to one (and so no fan-out).
	assert.Equal(t, []int{404}, matcher.expandStatusesFromIdent(node.GetEdge().Args[1], node.GetEdge()))

	// An assignment whose value is not a call is skipped.
	meta := matcherMeta(matcher)
	fn := findFunction(meta, "app", "handler")
	fn.AssignmentMap["err"] = []metadata.Assignment{
		csAssign(csNewErrorCall(meta, "StatusForbidden")),
		{Value: *buildIdentArg(meta, "plain", "app")}, // not a call -> skipped
	}
	assert.Equal(t, []int{403}, matcher.expandStatusesFromIdent(node.GetEdge().Args[1], node.GetEdge()))
}

func TestExpandStatusesFromIdent_NoMetadata(t *testing.T) {
	// A matcher whose context provider isn't a *ContextProviderImpl yields no
	// metadata, so expansion is skipped.
	meta := pmcTestMeta()
	edge := buildCallGraphEdge(meta, "handler", "app", "RespondWithError", "app",
		[]*metadata.CallArgument{buildIdentArg(meta, "err", "app")})
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{cfg: pmcTestCfg(), contextProvider: &mockContextProvider{}, schemaMapper: NewSchemaMapper(pmcTestCfg())},
		pattern:            ResponsePattern{StatusFromArg: true, StatusArgIndex: 0},
	}
	assert.Nil(t, matcher.expandStatusesFromIdent(edge.Args[0], edge))
}

// TestExtractResponse_ConditionalStatusFanOut drives the full ExtractResponse
// path: a RespondWithError(w, err) whose err is branch-assigned to distinct
// status constructors fans out to one response per status.
func TestExtractResponse_ConditionalStatusFanOut(t *testing.T) {
	matcher, node := csMatcher(t, []string{"StatusUnauthorized", "StatusNotFound", "StatusInternalServerError"})
	route := &RouteInfo{Response: map[string]*ResponseInfo{}, UsedTypes: map[string]*Schema{}}

	infos := matcher.ExtractResponse(node, route)
	require.Len(t, infos, 3, "one response per distinct branch status")
	got := []int{infos[0].StatusCode, infos[1].StatusCode, infos[2].StatusCode}
	sort.Ints(got)
	assert.Equal(t, []int{401, 404, 500}, got)
}

func csAssign(v metadata.CallArgument) metadata.Assignment { return metadata.Assignment{Value: v} }

func matcherMeta(m *ResponsePatternMatcherImpl) *metadata.Metadata {
	return metadataFromContextProvider(m.contextProvider)
}

func TestPositionAfter(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"f.go:20:1", "f.go:10:1", true},  // later line
		{"f.go:10:5", "f.go:10:2", true},  // same line, later column
		{"f.go:10:1", "f.go:20:1", false}, // earlier line
		{"f.go:10:1", "f.go:10:1", false}, // equal
		{"", "f.go:10:1", false},          // missing a
		{"f.go:10:1", "", false},          // missing b
		{"bad", "f.go:10:1", false},       // unparseable a
		{"f.go:x:1", "f.go:10:1", false},  // non-numeric line
	}
	for _, c := range cases {
		assert.Equal(t, c.want, positionAfter(c.a, c.b), "%q vs %q", c.a, c.b)
	}
}

func TestExpandStatusesFromIdent_Reachability(t *testing.T) {
	// Two assignments (400 then 500); the 500 is positioned AFTER the response
	// call site, so only 400 reaches it (issue #50).
	matcher, node := csMatcher(t, []string{"StatusBadRequest", "StatusInternalServerError"})
	meta := matcherMeta(matcher)
	sp := meta.StringPool
	fn := findFunction(meta, "app", "handler")
	fn.AssignmentMap["err"][0].Position = sp.Get("h.go:10:3") // before the call
	fn.AssignmentMap["err"][1].Position = sp.Get("h.go:20:3") // after the call

	edge := node.GetEdge()
	edge.Position = sp.Get("h.go:15:3") // call site between the two assignments

	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_UnconditionalShadow(t *testing.T) {
	// code=500 (unconditional) then code=400 (unconditional) before the call:
	// the later unconditional assignment overwrites the earlier on every path,
	// so only 400 reaches the call (issue #50, the reused-variable case).
	matcher, node := csMatcher(t, []string{"StatusInternalServerError", "StatusBadRequest"})
	meta := matcherMeta(matcher)
	sp := meta.StringPool
	fn := findFunction(meta, "app", "handler")
	fn.AssignmentMap["err"][0].Position = sp.Get("h.go:10:3") // 500, earlier
	fn.AssignmentMap["err"][0].Branch = nil                   // unconditional
	fn.AssignmentMap["err"][1].Position = sp.Get("h.go:12:3") // 400, later
	fn.AssignmentMap["err"][1].Branch = nil                   // unconditional (shadows 500)
	edge := node.GetEdge()
	edge.Position = sp.Get("h.go:15:3")
	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}

func TestExpandStatusesFromIdent_SiblingBranchesNotShadowed(t *testing.T) {
	// Two conditional (if-then / if-else) assignments before the call don't
	// shadow each other — both reach, so the intended fan-out is preserved.
	matcher, node := csMatcher(t, []string{"StatusBadRequest", "StatusNotFound"})
	meta := matcherMeta(matcher)
	sp := meta.StringPool
	fn := findFunction(meta, "app", "handler")
	fn.AssignmentMap["err"][0].Position = sp.Get("h.go:10:3")
	fn.AssignmentMap["err"][0].Branch = &metadata.BranchContext{BlockKind: "if-then"}
	fn.AssignmentMap["err"][1].Position = sp.Get("h.go:12:3")
	fn.AssignmentMap["err"][1].Branch = &metadata.BranchContext{BlockKind: "if-else"}
	edge := node.GetEdge()
	edge.Position = sp.Get("h.go:15:3")
	got := matcher.expandStatusesFromIdent(edge.Args[1], edge)
	sort.Ints(got)
	assert.Equal(t, []int{400, 404}, got)
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

func TestExpandStatusesFromIdent_ShadowRobustToUnparseablePosition(t *testing.T) {
	// Copilot (#57): the LATER unconditional assignment — the real shadow
	// boundary — has an unparseable position. The index-based boundary must
	// still shadow the earlier 500, leaving only 400 (a position-based boundary
	// would have stayed pinned to 500 and leaked it).
	matcher, node := csMatcher(t, []string{"StatusInternalServerError", "StatusBadRequest"})
	meta := matcherMeta(matcher)
	sp := meta.StringPool
	fn := findFunction(meta, "app", "handler")
	fn.AssignmentMap["err"][0].Position = sp.Get("h.go:10:3") // 500, parseable
	fn.AssignmentMap["err"][0].Branch = nil
	fn.AssignmentMap["err"][1].Position = sp.Get("???") // 400, UNPARSEABLE
	fn.AssignmentMap["err"][1].Branch = nil
	edge := node.GetEdge()
	edge.Position = sp.Get("h.go:15:3")
	assert.Equal(t, []int{400}, matcher.expandStatusesFromIdent(edge.Args[1], edge))
}
