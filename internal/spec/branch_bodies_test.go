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
	"github.com/stretchr/testify/require"
)

// TestAddResponse_BranchBodiesAttributedPerStatus is the unit-level guard for US3
// (FR-005): bodies written on different branches land under their own status code,
// never merged into one shape.
func TestAddResponse_BranchBodiesAttributedPerStatus(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	route := &RouteInfo{Response: map[string]*ResponseInfo{}}

	ext.addResponse(route, &ResponseInfo{StatusCode: 200, Schema: &Schema{Ref: "#/components/schemas/FullUser"}})
	ext.addResponse(route, &ResponseInfo{StatusCode: 404, Schema: &Schema{Ref: "#/components/schemas/ErrorBody"}})

	require.Len(t, route.Response, 2)
	require.NotNil(t, route.Response["200"].Schema)
	require.NotNil(t, route.Response["404"].Schema)
	assert.Equal(t, "#/components/schemas/FullUser", route.Response["200"].Schema.Ref)
	assert.Equal(t, "#/components/schemas/ErrorBody", route.Response["404"].Schema.Ref)
}

// TestAddResponse_SameStatusDistinctBodies records two distinct bodies under the
// same status as alternatives rather than dropping one.
func TestAddResponse_SameStatusDistinctBodies(t *testing.T) {
	meta := newTestMeta()
	ext, _ := newTestExtractor(meta)
	route := &RouteInfo{Response: map[string]*ResponseInfo{}}

	ext.addResponse(route, &ResponseInfo{StatusCode: 200, Schema: &Schema{Ref: "A"}})
	ext.addResponse(route, &ResponseInfo{StatusCode: 200, Schema: &Schema{Ref: "B"}})
	ext.addResponse(route, &ResponseInfo{StatusCode: 200, Schema: &Schema{Ref: "A"}}) // duplicate ignored

	require.Len(t, route.Response, 1)
	assert.Equal(t, "A", route.Response["200"].Schema.Ref)
	require.Len(t, route.Response["200"].AlternativeSchemas, 1)
	assert.Equal(t, "B", route.Response["200"].AlternativeSchemas[0].Ref)
}
