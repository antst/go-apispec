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

// TestApplyDetectedContentType covers FR-002 from spec 006 (the extractor-side
// bodyless guard for issue #33). applyDetectedContentType must NOT mutate the
// ContentType of bodyless status entries (1xx/204/304), even when their
// ContentType matches the default — they will never carry a body in the
// emitted spec, so the override is wasted and misleading.
//
// Reverting the `if isBodylessStatusCode(resp.StatusCode) { continue }` guard
// MUST make TestApplyDetectedContentType_SkipsBodyless fail — this is
// SC-005's "load-bearing" check on the extractor fix.

func TestApplyDetectedContentType_SkipsBodyless(t *testing.T) {
	// Mixed bodyless + body-bearing entries, all carrying default ContentType.
	// After the call, only body-bearing entries' ContentType updates.
	route := &RouteInfo{
		detectedContentType: "application/octet-stream",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json"},
			"304": {StatusCode: 304, ContentType: "application/json"},
			"204": {StatusCode: 204, ContentType: "application/json"},
			"102": {StatusCode: 102, ContentType: "application/json"},
		},
	}
	applyDetectedContentType(route, "application/json")

	assert.Equal(t, "application/octet-stream", route.Response["200"].ContentType,
		"body-bearing 200 must adopt the detected content-type")
	assert.Equal(t, "application/json", route.Response["304"].ContentType,
		"bodyless 304 must keep the default content-type (will be dropped at output anyway)")
	assert.Equal(t, "application/json", route.Response["204"].ContentType,
		"bodyless 204 must keep the default content-type")
	assert.Equal(t, "application/json", route.Response["102"].ContentType,
		"bodyless 102 must keep the default content-type")
}

func TestApplyDetectedContentType_AllBodyless_NoChange(t *testing.T) {
	// When every entry is bodyless, no mutation occurs anywhere.
	route := &RouteInfo{
		detectedContentType: "application/octet-stream",
		Response: map[string]*ResponseInfo{
			"204": {StatusCode: 204, ContentType: "application/json"},
			"304": {StatusCode: 304, ContentType: "application/json"},
		},
	}
	applyDetectedContentType(route, "application/json")

	assert.Equal(t, "application/json", route.Response["204"].ContentType)
	assert.Equal(t, "application/json", route.Response["304"].ContentType)
}

func TestApplyDetectedContentType_NonDefaultPreserved(t *testing.T) {
	// Body-bearing entries whose ContentType is ALREADY non-default (e.g., set
	// by a pattern's DefaultContentType like http.Error → text/plain) must NOT
	// be overridden. Preserves the existing condition unchanged.
	route := &RouteInfo{
		detectedContentType: "application/octet-stream",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "text/plain"},
		},
	}
	applyDetectedContentType(route, "application/json")

	assert.Equal(t, "text/plain", route.Response["200"].ContentType,
		"non-default ContentType must be preserved")
}

func TestApplyDetectedContentType_EmptyResponseMap(t *testing.T) {
	// Empty route — must not panic; nothing to override.
	route := &RouteInfo{
		detectedContentType: "application/octet-stream",
		Response:            map[string]*ResponseInfo{},
	}
	applyDetectedContentType(route, "application/json")

	assert.Empty(t, route.Response)
}

// TestAddResponse_StripsBodyFieldsOnBodyless covers the producer-side invariant
// from issue #33 follow-up (F2): addResponse must strip Schema, AlternativeSchemas,
// and BodyType when the status code is bodyless (1xx/204/304). RFC 9110 forbids a
// body on these statuses, and enforcing the invariant at insert time keeps RouteInfo
// internally consistent for any consumer that reads the structure directly.
//
// Reverting the bodyless block at the top of addResponse MUST make this test fail.
func TestAddResponse_StripsBodyFieldsOnBodyless(t *testing.T) {
	e := &Extractor{}
	route := &RouteInfo{Response: map[string]*ResponseInfo{}}

	bodylessCases := []struct {
		name       string
		statusCode int
	}{
		{"304_NotModified", 304},
		{"204_NoContent", 204},
		{"102_Processing", 102},
	}
	for _, tc := range bodylessCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &ResponseInfo{
				StatusCode:         tc.statusCode,
				Schema:             &Schema{Type: "object"},
				AlternativeSchemas: []*Schema{{Type: "string"}},
				BodyType:           "github.com/example/pkg.Body",
				ContentType:        "application/json",
			}
			e.addResponse(route, resp)
			key := strconv.Itoa(tc.statusCode)
			stored := route.Response[key]
			require.NotNil(t, stored, "entry must be inserted")
			assert.Nil(t, stored.Schema, "Schema must be stripped for bodyless status")
			assert.Nil(t, stored.AlternativeSchemas, "AlternativeSchemas must be stripped for bodyless status")
			assert.Empty(t, stored.BodyType, "BodyType must be stripped for bodyless status")
			// ContentType is intentionally retained (see addResponse comment) — it's still
			// meaningful for negotiation-style headers; the mapper drops the entire Content
			// map regardless.
			assert.Equal(t, "application/json", stored.ContentType, "ContentType is intentionally retained")
		})
	}
}

func TestAddResponse_PreservesBodyFieldsOnBodyBearing(t *testing.T) {
	// Control case: addResponse must NOT strip Schema/BodyType/AlternativeSchemas
	// for body-bearing statuses. Proves the guard is bodyless-specific.
	e := &Extractor{}
	route := &RouteInfo{Response: map[string]*ResponseInfo{}}
	schema := &Schema{Type: "object"}
	alt := []*Schema{{Type: "string"}}

	e.addResponse(route, &ResponseInfo{
		StatusCode:         200,
		Schema:             schema,
		AlternativeSchemas: alt,
		BodyType:           "User",
		ContentType:        "application/json",
	})
	stored := route.Response["200"]
	require.NotNil(t, stored)
	assert.Equal(t, schema, stored.Schema, "200 body-bearing: Schema preserved")
	assert.Equal(t, alt, stored.AlternativeSchemas, "200 body-bearing: AlternativeSchemas preserved")
	assert.Equal(t, "User", stored.BodyType, "200 body-bearing: BodyType preserved")
}

func TestApplyDetectedContentType_DetectedEqualsDefault_NoOp(t *testing.T) {
	// Issue #33 F3: when the detected type equals the default, the function
	// must return early — no iteration, no mutation, no documentation/code
	// drift. The body-bearing entry's ContentType stays at the default value
	// it already had (which equals the detected type, so the result is the
	// same in either case; the early-return is about cycles + clarity).
	route := &RouteInfo{
		detectedContentType: "application/json",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json"},
			"404": {StatusCode: 404, ContentType: "text/plain"},
		},
	}
	applyDetectedContentType(route, "application/json")

	assert.Equal(t, "application/json", route.Response["200"].ContentType,
		"200's matching ContentType is unchanged (would have been a no-op even without the guard)")
	assert.Equal(t, "text/plain", route.Response["404"].ContentType,
		"404's non-default ContentType is preserved (would have been preserved either way)")
}
