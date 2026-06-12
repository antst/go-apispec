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

// TestFindVacantStatusForBody covers FR-002 from spec 005 (the schema-aware
// picker fix for issue #30). The picker must require BOTH BodyType=="" AND
// Schema==nil; entries populated by expandHelperFunctionResponses carry a
// Schema but leave BodyType empty, and treating those as "vacant" mis-
// attributes Render-method schemas to lower-numbered helper statuses.
//
// Reverting the `&& resp.Schema == nil` clause in findVacantStatusForBody
// MUST make TestFindVacantStatusForBody_PrefersSchemaNil fail — this is
// SC-006's "load-bearing" check on the picker fix.
func TestFindVacantStatusForBody_PrefersSchemaNil(t *testing.T) {
	// 400 has a schema (from expand-pass); 422 is truly vacant. The picker
	// must skip 400 and pick 422 even though 400 sorts lower.
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"400": {StatusCode: 400, BodyType: "", Schema: &Schema{Type: "object"}},
			"422": {StatusCode: 422, BodyType: "", Schema: nil},
		},
	}
	status, ok := findVacantStatusForBody(route)
	assert.True(t, ok, "expected a vacant pick")
	assert.Equal(t, 422, status, "picker must skip the schema-set entry at 400 and pick the schema-nil entry at 422")
}

func TestFindVacantStatusForBody_NoPickWhenAllPopulated(t *testing.T) {
	// All entries carry a schema; the picker must report no vacant slot
	// rather than attaching a body schema to an already-populated status.
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "", Schema: &Schema{Type: "object"}},
			"400": {StatusCode: 400, BodyType: "", Schema: &Schema{Type: "object"}},
			"422": {StatusCode: 422, BodyType: "", Schema: &Schema{Type: "string"}},
		},
	}
	_, ok := findVacantStatusForBody(route)
	assert.False(t, ok, "all entries are populated; the picker must not pick any")
}

func TestFindVacantStatusForBody_PicksLowestVacant(t *testing.T) {
	// Two truly-vacant entries; the picker returns the lower sort-key entry
	// (the historical "lowest empty BodyType" semantics, preserved post-fix).
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"422": {StatusCode: 422, BodyType: "", Schema: nil},
			"500": {StatusCode: 500, BodyType: "", Schema: nil},
		},
	}
	status, ok := findVacantStatusForBody(route)
	assert.True(t, ok)
	assert.Equal(t, 422, status, "lowest sort-key vacant entry wins")
}

func TestFindVacantStatusForBody_SkipsBodylessAnd2xxLike304(t *testing.T) {
	// 204 No Content and 304 Not Modified must never receive a body schema.
	// The picker must skip them even if otherwise "vacant".
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"204": {StatusCode: 204, BodyType: "", Schema: nil},
			"304": {StatusCode: 304, BodyType: "", Schema: nil},
			"422": {StatusCode: 422, BodyType: "", Schema: nil},
		},
	}
	status, ok := findVacantStatusForBody(route)
	assert.True(t, ok)
	assert.Equal(t, 422, status, "must skip bodyless 204/304 and pick the next vacant body-bearing status")
}

func TestFindVacantStatusForBody_SkipsOutOfRange(t *testing.T) {
	// Statuses outside the valid HTTP range (100..599) must never be picked.
	// The sentinel "no real status" value used elsewhere in extractor.go is
	// a negative number (leastStatusCode - 1); these must NOT be claimed.
	route := &RouteInfo{
		Response: map[string]*ResponseInfo{
			"-1":  {StatusCode: -1, BodyType: "", Schema: nil},
			"600": {StatusCode: 600, BodyType: "", Schema: nil},
		},
	}
	_, ok := findVacantStatusForBody(route)
	assert.False(t, ok, "out-of-range statuses must not be picked")
}

func TestFindVacantStatusForBody_EmptyRoute(t *testing.T) {
	route := &RouteInfo{Response: map[string]*ResponseInfo{}}
	_, ok := findVacantStatusForBody(route)
	assert.False(t, ok, "empty route.Response cannot yield a pick")
}
