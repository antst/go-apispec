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
		assigns = append(assigns, metadata.Assignment{Value: csNewErrorCall(meta, name)})
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
