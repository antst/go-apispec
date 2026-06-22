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

// TestResolveParamArgType_RefBranches exercises every return path of
// resolveParamArgType and locks the Phase-3 invariant: the returned *TypeRef is
// non-nil exactly when the string is non-empty, and always equals ParseTypeRef
// of that string (so threading it into schemaForType is byte-identical to
// re-parsing — research D1). Covers the GetArgumentInfo branch, the raw-type
// fallback (the branch a KindKeyValue arg forces, since GetArgumentInfo returns
// "" for it), the nil-parent-edge continue, and the terminal no-match.
func TestResolveParamArgType_RefBranches(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	m := NewResponsePatternMatcher(ResponsePattern{}, cfg, NewContextProvider(meta))

	childEdge := makeEdge(meta, "helper", "app", "Decode", "json", nil)

	// Branch 1: GetArgumentInfo yields the fully-qualified type of the bound arg.
	t.Run("argument-info branch", func(t *testing.T) {
		parent := makeEdge(meta, "Handler", "app", "helper", "app", nil)
		parent.ParamArgMap = map[string]metadata.CallArgument{
			"p": *makeIdentArg(meta, "u", "error_helpers.User"),
		}
		child := makeTrackerNode(&childEdge)
		child.Parent = makeTrackerNode(&parent)

		got, ref := m.resolveParamArgType(child, "p")
		assert.NotEmpty(t, got, "a typed bound arg must resolve to a non-empty type")
		if assert.NotNil(t, ref, "non-empty resolved type must carry a ref") {
			assert.Equal(t, metadata.ParseTypeRef(got).String(), ref.String(),
				"ref must equal ParseTypeRef of the returned string")
		}
	})

	// Branch D6 (Phase 4): when the bound arg carries its own structured TypeRef,
	// resolveParamArgType sources it directly — the returned ref is that very
	// pointer (no re-parse), and the string is its rendering.
	t.Run("typeref branch sources the ref natively", func(t *testing.T) {
		tr := metadata.ParseTypeRef("svc.Account")
		arg := makeIdentArg(meta, "u", "shadowed.ByTypeRef")
		arg.TypeRef = tr
		parent := makeEdge(meta, "Handler", "app", "helper", "app", nil)
		parent.ParamArgMap = map[string]metadata.CallArgument{"p": *arg}
		child := makeTrackerNode(&childEdge)
		child.Parent = makeTrackerNode(&parent)

		got, ref := m.resolveParamArgType(child, "p")
		assert.Equal(t, "svc.Account", got, "string comes from arg.TypeRef, not GetArgumentInfo")
		assert.Same(t, tr, ref, "ref is the arg's own TypeRef pointer — native, not re-parsed")
	})

	// Branch 2 (Phase-3 fallback ref): GetArgumentInfo returns "" for a
	// KindKeyValue arg, so resolution falls through to the raw GetType() and now
	// also materializes a ref from it.
	t.Run("raw-type fallback branch", func(t *testing.T) {
		kv := metadata.NewCallArgument(meta)
		kv.SetKind(metadata.KindKeyValue) // GetArgumentInfo => ""
		kv.SetType("svc.Payload")         // ...but GetType() is concrete
		parent := makeEdge(meta, "Handler", "app", "helper", "app", nil)
		parent.ParamArgMap = map[string]metadata.CallArgument{"q": *kv}
		child := makeTrackerNode(&childEdge)
		child.Parent = makeTrackerNode(&parent)

		got, ref := m.resolveParamArgType(child, "q")
		assert.Equal(t, "svc.Payload", got, "must fall back to the raw arg type")
		if assert.NotNil(t, ref, "the fallback path must also thread a ref") {
			assert.Equal(t, metadata.ParseTypeRef("svc.Payload").String(), ref.String())
		}
	})

	// Branch 3: a parent with a nil edge is skipped (continue); the match is
	// found one level further up.
	t.Run("nil parent edge is skipped", func(t *testing.T) {
		parent := makeEdge(meta, "Handler", "app", "helper", "app", nil)
		parent.ParamArgMap = map[string]metadata.CallArgument{
			"p": *makeIdentArg(meta, "u", "error_helpers.User"),
		}
		grandparent := makeTrackerNode(&parent)
		nilEdgeParent := makeTrackerNode(nil) // GetEdge() == nil → continue
		nilEdgeParent.Parent = grandparent
		child := makeTrackerNode(&childEdge)
		child.Parent = nilEdgeParent

		got, ref := m.resolveParamArgType(child, "p")
		assert.NotEmpty(t, got)
		assert.NotNil(t, ref)
	})

	// Terminal: no parent binds the parameter → ("", nil), and a nil string must
	// carry a nil ref.
	t.Run("no match yields empty string and nil ref", func(t *testing.T) {
		lone := makeTrackerNode(&childEdge) // no Parent → GetParent() nil
		got, ref := m.resolveParamArgType(lone, "absent")
		assert.Equal(t, "", got)
		assert.Nil(t, ref)
	})
}
