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

package engine

import (
	"testing"

	"github.com/antst/go-apispec/internal/metadata"
	"github.com/antst/go-apispec/internal/spec"
)

// TestEveryResolvedBodyTypeReachesSchemaWithRef is the Phase-3 regression guard
// for SC-002: across the whole fixture corpus, every request/response body
// position whose resolution produced a type (non-empty BodyType) MUST carry a
// non-nil BodyTypeRef — the structured type the schema generator consumes without
// re-parsing. A nil ref means a producer set BodyType without threading the
// resolved TypeRef, re-opening the string→tree boundary this phase closed.
//
// Parameter positions retain no carrier (the resolved type is a local consumed
// into Parameter.Schema), so they are intentionally NOT guarded here — they are
// covered by the parse-free gate (SC-001) and the byte-identical golden (SC-003).
func TestEveryResolvedBodyTypeReachesSchemaWithRef(t *testing.T) {
	total, missing := 0, 0
	for _, tc := range allFrameworks(t) {
		cfg := DefaultEngineConfig()
		cfg.InputDir = tc.inputDir
		meta, err := NewEngine(cfg).GenerateMetadataOnly()
		if err != nil {
			t.Fatalf("%s: metadata generation failed: %v", tc.name, err)
		}
		if meta == nil {
			t.Fatalf("%s: metadata generation returned nil metadata", tc.name)
		}
		limits := metadata.TrackerLimits{
			MaxNodesPerTree:    cfg.MaxNodesPerTree,
			MaxChildrenPerNode: cfg.MaxChildrenPerNode,
			MaxArgsPerFunction: cfg.MaxArgsPerFunction,
			MaxNestedArgsDepth: cfg.MaxNestedArgsDepth,
			MaxRecursionDepth:  cfg.MaxRecursionDepth,
		}
		tree := spec.NewTrackerTree(meta, limits)
		routes := spec.NewExtractor(tree, tc.configFn()).ExtractRoutes()

		check := func(pos, bodyType string, ref *metadata.TypeRef) {
			if bodyType == "" {
				return // resolved to nothing → degraded (FR-005), exempt
			}
			total++
			if ref == nil {
				missing++
				t.Errorf("%s: %s body %q reached schema generation without a BodyTypeRef", tc.name, pos, bodyType)
			}
		}
		for _, route := range routes {
			if route.Request != nil {
				check("request", route.Request.BodyType, route.Request.BodyTypeRef)
			}
			for status, resp := range route.Response {
				if resp != nil {
					check("response["+status+"]", resp.BodyType, resp.BodyTypeRef)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no resolved body positions scanned — corpus did not load")
	}
	t.Logf("scanned %d request/response body positions, %d missing a BodyTypeRef", total, missing)
}
