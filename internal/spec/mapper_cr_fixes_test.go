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

// TestTypeByRef_UnqualifiedFallbackDeterministic locks the fix for the
// non-deterministic package-miss fallback: a bare RefNamed (no Pkg) whose name
// exists in more than one package must resolve to the same type on every run.
// Before the fix the fallback ranged meta.Packages (a map) and returned an
// arbitrary match, flapping the output between runs.
func TestTypeByRef_UnqualifiedFallbackDeterministic(t *testing.T) {
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"zebra": {Files: map[string]*metadata.File{"z.go": {Types: map[string]*metadata.Type{
			"User": {Name: meta.StringPool.Get("zebraUser")},
		}}}},
		"alpha": {Files: map[string]*metadata.File{"a.go": {Types: map[string]*metadata.Type{
			"User": {Name: meta.StringPool.Get("alphaUser")},
		}}}},
	}
	// No Pkg → the unqualified fallback runs; sorted package order picks "alpha".
	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Name: "User"}

	for i := 0; i < 25; i++ {
		got := typeByRef(ref, meta)
		if assert.NotNil(t, got) {
			assert.Equal(t, "alphaUser", meta.StringPool.GetString(got.Name),
				"unqualified collision must resolve to the sorted-first package deterministically")
		}
	}
}

// TestSchemaForNamedRef_NilCfg locks the cfg nil-guard: schemaForType accepts a
// nil *APISpecConfig, so schemaForNamedRef (reachable from it) must not panic
// ranging cfg.TypeMapping / cfg.ExternalTypes when cfg is nil.
func TestSchemaForNamedRef_NilCfg(t *testing.T) {
	meta := newTestMeta()
	ref := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "pkg", Name: "Unknown"}
	usedTypes := map[string]*Schema{}

	assert.NotPanics(t, func() {
		_, _, _ = schemaForNamedRef(usedTypes, ref, "", meta, nil, nil)
	}, "nil cfg must be tolerated (no TypeMapping/ExternalTypes to apply)")
}
