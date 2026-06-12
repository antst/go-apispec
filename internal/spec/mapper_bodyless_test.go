// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildResponses_Bodyless covers FR-001 from spec 006 (the mapper-side
// bodyless guard for issue #33). buildResponses must NOT emit a Content map
// for 1xx/204/304 status entries, regardless of whether Schema or
// ContentType are set on the underlying ResponseInfo.
//
// Reverting the `if isBodylessStatusCode(effectiveStatus)` branch in
// buildResponses MUST make TestBuildResponses_304_NoContent fail — this is
// SC-005's "load-bearing" check on the mapper fix.

func TestBuildResponses_BodylessStatuses_NoContent(t *testing.T) {
	// Parameterized coverage of all three bodyless ranges (1xx via 102, 204,
	// 304). Every case must drop the Content map and emit description-only.
	// Reverting the `if isBodylessStatusCode(effectiveStatus)` branch in
	// buildResponses MUST make ALL three subtests fail — SC-005 load-bearing.
	cases := []struct {
		name        string
		statusCode  int
		schema      *Schema
		contentType string
		wantDesc    string
	}{
		{"304_NotModified", 304, &Schema{Type: "object"}, "application/json", "Not Modified"},
		{"204_NoContent", 204, &Schema{Type: "object"}, "application/json", "No Content"},
		{"102_Processing", 102, &Schema{Type: "string"}, "text/plain", "Processing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := strconv.Itoa(tc.statusCode)
			respInfo := map[string]*ResponseInfo{
				key: {StatusCode: tc.statusCode, Schema: tc.schema, ContentType: tc.contentType},
			}
			out := buildResponses(respInfo)
			got := out[key]
			assert.Nil(t, got.Content, "%d must not carry a Content map (RFC 9110)", tc.statusCode)
			assert.Equal(t, tc.wantDesc, got.Description)
		})
	}
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

func TestBuildResponses_BodylessNotMergedWithBodyOnly(t *testing.T) {
	// Per CodeRabbit review on PR #34: the pre-merge step in buildResponses
	// (status-only ⨯ body-only fold) must EXCLUDE bodyless statuses from the
	// status-only candidate set. Otherwise a handler that does
	//     w.WriteHeader(204)               // 204 entry, status-only
	//     json.Encode(user)                // body-only entry with sentinel StatusCode
	// would have the User schema folded into the 204 entry's BodyType/Schema.
	// The emitted Response correctly omits Content (bodyless mapper guard),
	// but collectUsedTypesFromRoutes (mapper.go) walks res.BodyType to
	// populate components.schemas — a phantom User schema would leak into
	// components even though no response references it.
	//
	// Reverting the `!isBodylessStatusCode(resp.StatusCode)` clause in the
	// pre-merge candidate detection MUST make this test fail.
	respInfo := map[string]*ResponseInfo{
		"204": {StatusCode: 204, BodyType: "", Schema: nil, ContentType: "application/json"},
		"-1":  {StatusCode: -1, BodyType: "User", Schema: &Schema{Type: "object"}, ContentType: "application/json"},
	}
	_ = buildResponses(respInfo)

	// After the call, respInfo["204"] must NOT have been mutated to carry the
	// body-only entry's BodyType/Schema. This is the load-bearing assertion.
	assert.Empty(t, respInfo["204"].BodyType, "204's BodyType must not be folded from the body-only entry")
	assert.Nil(t, respInfo["204"].Schema, "204's Schema must not be folded from the body-only entry")
}

func TestBuildResponses_BodyOnlyStillMergesIntoBodyBearingStatusOnly(t *testing.T) {
	// Control case: the merge SHOULD still fire for non-bodyless status-only
	// entries (the original WriteHeader(201)+Encode(user) pattern motivating
	// the pre-merge logic). Proves the bodyless exclusion is targeted, not
	// a blanket disable of the merge.
	respInfo := map[string]*ResponseInfo{
		"201": {StatusCode: 201, BodyType: "", Schema: nil, ContentType: "application/json"},
		"-1":  {StatusCode: -1, BodyType: "User", Schema: &Schema{Type: "object"}, ContentType: "application/json"},
	}
	out := buildResponses(respInfo)

	got := out["201"]
	require.NotNil(t, got.Content, "201 must carry Content (body-bearing)")
	assert.Equal(t, "User", respInfo["201"].BodyType, "201's BodyType must be folded from body-only entry")
	assert.NotNil(t, respInfo["201"].Schema, "201's Schema must be folded from body-only entry")
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
