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

// TestExtractDocComment_MethodHandler covers issue #45: a method handler's doc
// comment is resolved via the receiver type's method records (methods aren't in
// file.Functions), and the receiver is matched through the TypeSep'd, lower-cased
// function path ("app-->userhandler.GetUser" against type "UserHandler").
func TestExtractDocComment_MethodHandler(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool(), Packages: map[string]*metadata.Package{}}
	sp := meta.StringPool
	meta.Packages["app"] = &metadata.Package{
		Files: map[string]*metadata.File{
			"h.go": {
				Functions: map[string]*metadata.Function{
					"freeHandler": {Name: sp.Get("freeHandler"), Comments: sp.Get("freeHandler is a free function. With detail.")},
				},
				Types: map[string]*metadata.Type{
					"UserHandler": {
						Name: sp.Get("UserHandler"),
						Methods: []metadata.Method{
							{Name: sp.Get("GetUser"), Comments: sp.Get("GetUser returns a user by ID. It supports caching.")},
							{Name: sp.Get("Other"), Comments: sp.Get("Other does something else.")},
						},
					},
				},
			},
		},
	}

	// Free function: resolved via file.Functions; recvType ("app") matches no
	// type so the method scan is a no-op.
	free := &RouteInfo{Function: "app.freeHandler", Package: "app", Metadata: meta}
	fs, _ := extractDocComment(free)
	assert.Equal(t, "freeHandler is a free function.", fs)

	// Bare function name (empty prefix) exercises the recvType=="" path.
	bare := &RouteInfo{Function: "freeHandler", Package: "app", Metadata: meta}
	bs, _ := extractDocComment(bare)
	assert.Equal(t, "freeHandler is a free function.", bs)

	// Receiver rendered lower-case after the TypeSep, as the real pipeline does.
	route := &RouteInfo{Function: "app-->userhandler.GetUser", Package: "app", Metadata: meta}
	s, d := extractDocComment(route)
	assert.Equal(t, "GetUser returns a user by ID.", s)
	assert.Contains(t, d, "caching")

	// A receiver that matches no type yields nothing (and doesn't pick a
	// same-named method off an unrelated type).
	none := &RouteInfo{Function: "app-->nosuchtype.GetUser", Package: "app", Metadata: meta}
	s2, d2 := extractDocComment(none)
	assert.Empty(t, s2)
	assert.Empty(t, d2)

	// Resolving the second method exercises the method-name-mismatch skip.
	other := &RouteInfo{Function: "app-->userhandler.Other", Package: "app", Metadata: meta}
	os, _ := extractDocComment(other)
	assert.Equal(t, "Other does something else.", os)

	// An unknown bare function (no prefix → empty recvType, not in Functions)
	// exercises the recvType=="" skip and falls through to no comment.
	unknown := &RouteInfo{Function: "unknownBare", Package: "app", Metadata: meta}
	us, _ := extractDocComment(unknown)
	assert.Empty(t, us)
}

// TestApplyOverrides_Description covers issue #46: a config override's
// description is applied to the operation (previously parsed but ignored).
func TestApplyOverrides_Description(t *testing.T) {
	applier := &OverrideApplierImpl{cfg: &APISpecConfig{
		Overrides: []Override{{FunctionName: "GetThing", Description: "Override description."}},
	}}
	route := &RouteInfo{Function: "GetThing", Description: "old"}
	applier.ApplyOverrides(route)
	assert.Equal(t, "Override description.", route.Description)

	// No description in the override leaves the existing one untouched.
	applier2 := &OverrideApplierImpl{cfg: &APISpecConfig{
		Overrides: []Override{{FunctionName: "GetThing", Summary: "S"}},
	}}
	route2 := &RouteInfo{Function: "GetThing", Description: "keep"}
	applier2.ApplyOverrides(route2)
	assert.Equal(t, "keep", route2.Description)
}
