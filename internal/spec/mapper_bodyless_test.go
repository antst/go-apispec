// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildResponses_Bodyless covers FR-001 from spec 006 (the mapper-side
// bodyless guard for issue #33). buildResponses must NOT emit a Content map
// for 1xx/204/304 status entries, regardless of whether Schema or
// ContentType are set on the underlying ResponseInfo.
//
// Reverting the `if isBodylessStatusCode(effectiveStatus)` branch in
// buildResponses MUST make TestBuildResponses_304_NoContent fail — this is
// SC-005's "load-bearing" check on the mapper fix.

func TestBuildResponses_304_NoContent(t *testing.T) {
	// 304 with schema and a non-default content-type — both must be dropped
	// from the emitted Response.
	respInfo := map[string]*ResponseInfo{
		"304": {StatusCode: 304, Schema: &Schema{Type: "object"}, ContentType: "application/json"},
	}
	out := buildResponses(respInfo)
	got := out["304"]
	assert.Nil(t, got.Content, "304 must not carry a Content map (RFC 9110)")
	assert.Equal(t, "Not Modified", got.Description)
}

func TestBuildResponses_204_NoContent(t *testing.T) {
	respInfo := map[string]*ResponseInfo{
		"204": {StatusCode: 204, Schema: &Schema{Type: "object"}, ContentType: "application/json"},
	}
	out := buildResponses(respInfo)
	got := out["204"]
	assert.Nil(t, got.Content, "204 must not carry a Content map")
	assert.Equal(t, "No Content", got.Description)
}

func TestBuildResponses_102_NoContent(t *testing.T) {
	// 102 Processing is a 1xx — every 1xx is bodyless per isBodylessStatusCode.
	respInfo := map[string]*ResponseInfo{
		"102": {StatusCode: 102, Schema: &Schema{Type: "string"}, ContentType: "text/plain"},
	}
	out := buildResponses(respInfo)
	got := out["102"]
	assert.Nil(t, got.Content, "1xx (102) must not carry a Content map")
	assert.Equal(t, "Processing", got.Description)
}

func TestBuildResponses_200_RetainsContent(t *testing.T) {
	// Control case: 200 is a body-bearing status; Content must be retained.
	// Proves the guard is bodyless-specific, not a broad regression.
	respInfo := map[string]*ResponseInfo{
		"200": {StatusCode: 200, Schema: &Schema{Type: "object"}, ContentType: "application/json"},
	}
	out := buildResponses(respInfo)
	got := out["200"]
	assert.NotNil(t, got.Content, "200 must retain its Content map")
	assert.Contains(t, got.Content, "application/json")
	assert.NotNil(t, got.Content["application/json"].Schema)
	assert.Equal(t, "OK", got.Description)
}

func TestBuildResponses_304_NoSchema_StillNoContent(t *testing.T) {
	// Even without a Schema, the bodyless guard still applies — the bug also
	// manifests when only ContentType is set (e.g., via content-type detection
	// override that pre-fix leaks onto bodyless entries).
	respInfo := map[string]*ResponseInfo{
		"304": {StatusCode: 304, ContentType: "application/octet-stream"},
	}
	out := buildResponses(respInfo)
	got := out["304"]
	assert.Nil(t, got.Content, "304 must not carry Content even when Schema is nil")
	assert.Equal(t, "Not Modified", got.Description)
}

func TestBuildResponses_304_OneOf_StillNoContent(t *testing.T) {
	// Even with multiple alternative schemas accumulated, the bodyless guard
	// short-circuits BEFORE oneOf wrapping — proves the guard placement is
	// correct relative to the schema-merging code path.
	respInfo := map[string]*ResponseInfo{
		"304": {
			StatusCode:         304,
			Schema:             &Schema{Type: "object"},
			AlternativeSchemas: []*Schema{{Type: "string"}, {Type: "integer"}},
			ContentType:        "application/json",
		},
	}
	out := buildResponses(respInfo)
	got := out["304"]
	assert.Nil(t, got.Content, "304 with oneOf still must not carry Content")
	assert.Equal(t, "Not Modified", got.Description)
}
