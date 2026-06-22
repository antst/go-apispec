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

	"github.com/antst/go-apispec/internal/metadata"
)

// rsSelector builds `<x>.Body` as a KindSelector.
func rsBodySelector(meta *metadata.Metadata, recv string) *metadata.CallArgument {
	s := metadata.NewCallArgument(meta)
	s.SetKind(metadata.KindSelector)
	s.X = wsIdent(meta, recv, "", "")
	s.Sel = wsIdent(meta, "Body", "", "")
	return s
}

func TestArgReferencesRequest(t *testing.T) {
	meta := pmcTestMeta()

	assert.False(t, argReferencesRequest(nil), "nil")

	// A value typed *http.Request.
	req := wsIdent(meta, "r", "", "*http.Request")
	assert.True(t, argReferencesRequest(req))

	// r.Body selector.
	assert.True(t, argReferencesRequest(rsBodySelector(meta, "r")))

	// io.ReadAll(r.Body) — recurse through the call's args.
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = wsIdent(meta, "ReadAll", "io", "")
	call.Args = []*metadata.CallArgument{rsBodySelector(meta, "r")}
	assert.True(t, argReferencesRequest(call))

	// A bare []byte ident with no request reference.
	assert.False(t, argReferencesRequest(wsIdent(meta, "raw", "", "[]byte")))

	// A selector that isn't .Body.
	other := metadata.NewCallArgument(meta)
	other.SetKind(metadata.KindSelector)
	other.X = wsIdent(meta, "row", "", "")
	other.Sel = wsIdent(meta, "ContentMetadata", "", "")
	assert.False(t, argReferencesRequest(other))

	// The request reference sits in the call's Fun position, e.g. r.Body.Close().
	funCall := metadata.NewCallArgument(meta)
	funCall.SetKind(metadata.KindCall)
	funCall.Fun = rsBodySelector(meta, "r")
	assert.True(t, argReferencesRequest(funCall))
}

func TestDecodeReadsRequestBody(t *testing.T) {
	meta := pmcTestMeta()
	matcher := &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(pmcTestCfg(), NewContextProvider(meta)),
	}

	// nil edge → nothing to filter.
	assert.True(t, matcher.decodeReadsRequestBody(buildTrackerNode(nil), nil))

	// Chained json.NewDecoder(r.Body).Decode — decoder source is the body → accept.
	bodyEdge := buildCallGraphEdge(meta, "h", "app", "Decode", "json", nil)
	bodyEdge.ChainParent = buildCallGraphEdge(meta, "h", "app", "NewDecoder", "json",
		[]*metadata.CallArgument{rsBodySelector(meta, "r")})
	assert.True(t, matcher.decodeReadsRequestBody(buildTrackerNode(bodyEdge), bodyEdge))

	// Chained json.NewDecoder(buf).Decode — decoder source isn't the body → reject.
	bufEdge := buildCallGraphEdge(meta, "h", "app", "Decode", "json", nil)
	bufEdge.ChainParent = buildCallGraphEdge(meta, "h", "app", "NewDecoder", "json",
		[]*metadata.CallArgument{wsIdent(meta, "buf", "", "*bytes.Buffer")})
	assert.False(t, matcher.decodeReadsRequestBody(buildTrackerNode(bufEdge), bufEdge))

	// Arg-sourced json.Unmarshal(r.Body, &v) → accept; Unmarshal(<literal>, &v) → reject (#52).
	matcher.pattern = RequestBodyPattern{TypeFromArg: true, TypeArgIndex: 1}
	okEdge := buildCallGraphEdge(meta, "h", "app", "Unmarshal", "json",
		[]*metadata.CallArgument{rsBodySelector(meta, "r"), wsIdent(meta, "v", "", "*T")})
	assert.True(t, matcher.decodeReadsRequestBody(buildTrackerNode(okEdge), okEdge))

	badEdge := buildCallGraphEdge(meta, "h", "app", "Unmarshal", "json",
		[]*metadata.CallArgument{buildLiteralArg(meta, "x"), wsIdent(meta, "v", "", "*T")})
	assert.False(t, matcher.decodeReadsRequestBody(buildTrackerNode(badEdge), badEdge))
}

func TestDecodeSourceTracesToRequest(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool
	matcher := &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(pmcTestCfg(), NewContextProvider(meta)),
	}

	// A handler function whose `body` local is assigned io.ReadAll(r.Body),
	// while `raw` is assigned from a non-request source.
	readAll := metadata.NewCallArgument(meta)
	readAll.SetKind(metadata.KindCall)
	readAll.Fun = wsIdent(meta, "ReadAll", "io", "")
	readAll.Args = []*metadata.CallArgument{rsBodySelector(meta, "r")}

	dbCall := metadata.NewCallArgument(meta)
	dbCall.SetKind(metadata.KindCall)
	dbCall.Fun = wsIdent(meta, "readMetadataColumn", "app", "")

	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"h.go": {Functions: map[string]*metadata.Function{
				"handler": {
					Name: sp.Get("handler"),
					Pkg:  sp.Get("app"),
					AssignmentMap: map[string][]metadata.Assignment{
						"body": {{Value: *readAll}},
						"raw":  {{Value: *dbCall}},
					},
				},
			}},
		},
	}
	node := buildTrackerNode(buildCallGraphEdge(meta, "handler", "app", "Unmarshal", "encoding/json", nil))

	// nil source → nothing to validate.
	assert.True(t, matcher.decodeSourceTracesToRequest(node, nil))

	// Direct request reference.
	assert.True(t, matcher.decodeSourceTracesToRequest(node, rsBodySelector(meta, "r")))
	assert.True(t, matcher.decodeSourceTracesToRequest(node, wsIdent(meta, "r", "", "*http.Request")))

	// Ident assigned from io.ReadAll(r.Body).
	assert.True(t, matcher.decodeSourceTracesToRequest(node, wsIdent(meta, "body", "", "[]byte")))

	// Ident assigned from a non-request (DB) source → rejected (issue #52).
	assert.False(t, matcher.decodeSourceTracesToRequest(node, wsIdent(meta, "raw", "", "[]byte")))

	// Ident with no assignment at all → rejected.
	assert.False(t, matcher.decodeSourceTracesToRequest(node, wsIdent(meta, "unknown", "", "[]byte")))

	// Node without an edge → cannot trace assignments → rejected.
	assert.False(t, matcher.decodeSourceTracesToRequest(buildTrackerNode(nil), wsIdent(meta, "body", "", "[]byte")))

	// Caller function cannot be resolved in the metadata → rejected.
	ghost := buildTrackerNode(buildCallGraphEdge(meta, "ghost", "ghostpkg", "Unmarshal", "encoding/json", nil))
	assert.False(t, matcher.decodeSourceTracesToRequest(ghost, wsIdent(meta, "body", "", "[]byte")))

	// Non-ident, non-request literal → rejected.
	assert.False(t, matcher.decodeSourceTracesToRequest(node, buildLiteralArg(meta, "x")))
}
