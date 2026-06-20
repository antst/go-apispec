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

import "testing"

// TestEveryStructFieldHasTypeRef is the regression guard for the getTypeName
// retirement: across the whole fixture corpus, every struct field must carry a
// structured TypeRef. The schema pipeline derives field types from TypeRef
// (fieldTypeString); a nil TypeRef would silently fall back to the lossy
// getTypeName string, re-opening the gap this migration closed. If this ever
// fails, a producer stopped attaching a TypeRef — fix the producer, do not widen
// the fallback.
func TestEveryStructFieldHasTypeRef(t *testing.T) {
	total, missing := 0, 0
	for _, tc := range allFrameworks(t) {
		cfg := DefaultEngineConfig()
		cfg.InputDir = tc.inputDir
		meta, err := NewEngine(cfg).GenerateMetadataOnly()
		if err != nil || meta == nil {
			continue
		}
		for _, pkg := range meta.Packages {
			for _, file := range pkg.Files {
				for typeName, typ := range file.Types {
					for _, f := range typ.Fields {
						total++
						if f.TypeRef == nil {
							missing++
							t.Errorf("%s: field %s.%s has nil TypeRef (would fall back to getTypeName)",
								tc.name, typeName, meta.StringPool.GetString(f.Name))
						}
					}
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no struct fields scanned — corpus did not load")
	}
	t.Logf("scanned %d struct fields across the corpus, %d missing a TypeRef", total, missing)
}
