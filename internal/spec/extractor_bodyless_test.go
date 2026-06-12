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
