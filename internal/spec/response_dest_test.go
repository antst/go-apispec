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

func rdMatcher(meta *metadata.Metadata) *ResponsePatternMatcherImpl {
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: NewContextProvider(meta),
			cfg:             cfg,
			schemaMapper:    NewSchemaMapper(cfg),
		},
		pattern: ResponsePattern{},
	}
}

func TestArgReferencesResponseWriter(t *testing.T) {
	meta := newTestMeta()

	assert.False(t, argReferencesResponseWriter(nil))

	// A value typed http.ResponseWriter.
	assert.True(t, argReferencesResponseWriter(makeIdentArg(meta, "w", "net/http.ResponseWriter")))

	// A file destination — not a response writer.
	assert.False(t, argReferencesResponseWriter(makeIdentArg(meta, "dst", "*os.File")))

	// io.Copy(w, f) — recurse through the call's args.
	call := makeCallArg(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = makeIdentArg(meta, "Copy", "")
	call.Args = []*metadata.CallArgument{
		makeIdentArg(meta, "w", "net/http.ResponseWriter"),
		makeIdentArg(meta, "f", "*os.File"),
	}
	assert.True(t, argReferencesResponseWriter(call))

	// c.Response().Writer style — the receiver carries the type.
	sel := makeCallArg(meta)
	sel.SetKind(metadata.KindSelector)
	sel.X = makeIdentArg(meta, "rw", "http.ResponseWriter")
	sel.Sel = makeIdentArg(meta, "Writer", "")
	assert.True(t, argReferencesResponseWriter(sel))
}

func TestWriteDestTracesToResponseWriter(t *testing.T) {
	meta := newTestMeta()
	m := rdMatcher(meta)

	base := makeEdge(meta, "h", "app", "Copy", "io", nil)
	baseNode := makeTrackerNode(&base)

	// nil destination → rejected.
	assert.False(t, m.writeDestTracesToResponseWriter(baseNode, nil))

	// Direct http.ResponseWriter destination — io.Copy(w, f).
	e1 := makeEdge(meta, "Download", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "w", "net/http.ResponseWriter"),
		makeIdentArg(meta, "f", "*os.File"),
	})
	assert.True(t, m.writeDestTracesToResponseWriter(makeTrackerNode(&e1), e1.Args[0]))

	// io.Copy(dst, src) where dst is a bare *os.File ident — rejected (issue #52).
	e2 := makeEdge(meta, "store", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "dst", "*os.File"),
		makeIdentArg(meta, "src", "io.Reader"),
	})
	assert.False(t, m.writeDestTracesToResponseWriter(makeTrackerNode(&e2), e2.Args[0]))

	// Destination ident assigned from a response writer (rw := w) — accepted.
	e3 := makeEdge(meta, "h", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "rw", ""),
		makeIdentArg(meta, "src", ""),
	})
	e3.AssignmentMap = map[string][]metadata.Assignment{
		"rw": {{Value: *makeIdentArg(meta, "w", "net/http.ResponseWriter")}},
	}
	assert.True(t, m.writeDestTracesToResponseWriter(makeTrackerNode(&e3), e3.Args[0]))

	// Destination ident assigned from a file — rejected.
	e4 := makeEdge(meta, "h", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "f", ""),
		makeIdentArg(meta, "src", ""),
	})
	e4.AssignmentMap = map[string][]metadata.Assignment{
		"f": {{Value: *makeIdentArg(meta, "tmp", "*os.File")}},
	}
	assert.False(t, m.writeDestTracesToResponseWriter(makeTrackerNode(&e4), e4.Args[0]))

	// Destination is a parameter the caller bound to a response writer:
	// stream(out io.Writer) called as stream(w) where w is http.ResponseWriter.
	parent := makeEdge(meta, "Download", "app", "stream", "app", nil)
	parent.ParamArgMap = map[string]metadata.CallArgument{
		"out": *makeIdentArg(meta, "w", "net/http.ResponseWriter"),
	}
	parentNode := makeTrackerNode(&parent)
	e5 := makeEdge(meta, "stream", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "out", "io.Writer"),
		makeIdentArg(meta, "src", "io.Reader"),
	})
	child := makeTrackerNode(&e5)
	child.Parent = parentNode
	assert.True(t, m.writeDestTracesToResponseWriter(child, e5.Args[0]))

	// A non-ident destination that isn't a response writer — rejected.
	assert.False(t, m.writeDestTracesToResponseWriter(baseNode, makeLiteralArg(meta, "x")))
}

func TestExtractResponse_ValidateWriterDestGate(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: NewContextProvider(meta),
			cfg:             cfg,
			schemaMapper:    NewSchemaMapper(cfg),
		},
		pattern: ResponsePattern{DefaultBodyType: "[]byte", ValidateWriterDest: true},
	}
	route := NewRouteInfo()
	route.Metadata = meta

	// io.Copy(dst, src) where dst is a file → gate rejects → no response.
	fileEdge := makeEdge(meta, "store", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "dst", "*os.File"),
		makeIdentArg(meta, "src", "io.Reader"),
	})
	assert.Empty(t, matcher.ExtractResponse(makeTrackerNode(&fileEdge), route))

	// io.Copy(w, f) where w is the response writer → gate passes → binary response.
	wEdge := makeEdge(meta, "Download", "app", "Copy", "io", []*metadata.CallArgument{
		makeIdentArg(meta, "w", "net/http.ResponseWriter"),
		makeIdentArg(meta, "f", "*os.File"),
	})
	resp := firstResponse(matcher.ExtractResponse(makeTrackerNode(&wEdge), route))
	require.NotNil(t, resp)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "binary", resp.Schema.Format)

	// A pattern that writes to arg0 but the call carries no args → gate rejects.
	noArgs := makeEdge(meta, "h", "app", "Copy", "io", nil)
	assert.Empty(t, matcher.ExtractResponse(makeTrackerNode(&noArgs), route))
}
