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

func TestBuildResponses_Deterministic(t *testing.T) {
	respInfo := map[string]*ResponseInfo{
		"200": {StatusCode: 200, ContentType: "application/json", BodyType: "User", Schema: &Schema{Type: "object"}},
		"304": {StatusCode: 304, ContentType: "", BodyType: ""},
		"404": {StatusCode: 404, ContentType: "application/json", BodyType: "Error", Schema: &Schema{Type: "object"}},
		"500": {StatusCode: 500, ContentType: "application/json", BodyType: "Error", Schema: &Schema{Type: "object"}},
	}

	var firstResult map[string]Response
	for i := 0; i < 20; i++ {
		result := buildResponses(respInfo)
		if i == 0 {
			firstResult = result
		} else {
			assert.Equal(t, firstResult, result, "buildResponses produced different output on iteration %d", i)
		}
	}
}

func TestIsBodylessStatusCode(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		{100, true},  // Continue
		{101, true},  // Switching Protocols
		{199, true},  // upper bound of 1xx
		{200, false}, // OK
		{201, false}, // Created
		{204, true},  // No Content
		{301, false}, // Moved Permanently
		{304, true},  // Not Modified
		{400, false}, // Bad Request
		{500, false}, // Internal Server Error
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.expected, isBodylessStatusCode(tt.code), "code %d", tt.code)
		})
	}
}

func TestShortenOperationID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Full module path with receiver.Method
		{
			"github.com/org/project/internal/http.UserHandler.Get",
			"UserHandler.Get",
		},
		// Full module path with DI container chain
		{
			"github.com/alkem-io/file-service-go/internal/adapter/inbound/http.Deps.DocumentHandler.GetContent",
			"DocumentHandler.GetContent",
		},
		// Package + bare function (no receiver) — keeps package
		{
			"github.com/org/project/users.ListUsers",
			"users.ListUsers",
		},
		// Already short: package.Function
		{
			"main.GetUsers",
			"main.GetUsers",
		},
		// Already short: Type.Method (no package)
		{
			"UserHandler.Get",
			"UserHandler.Get",
		},
		// Bare function name
		{
			"GetUsers",
			"GetUsers",
		},
		// Deep chain: package.A.B.C.Method
		{
			"http.Container.SubContainer.Handler.Create",
			"Handler.Create",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shortenOperationID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortenTypeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"github.com/org/project/internal/http-->CreateDocumentResponse",
			"http-->CreateDocumentResponse",
		},
		{
			"github.com/org/project/users-->User",
			"users-->User",
		},
		// No slashes — already short
		{
			"main-->User",
			"main-->User",
		},
		{
			"User",
			"User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shortenTypeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSchemaName(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		typeName string
		cfg      *APISpecConfig
		expected string
	}{
		{
			"short names enabled (default nil)",
			"github.com/org/project/users-->User",
			&APISpecConfig{},
			"users.User",
		},
		{
			"short names enabled (explicit true)",
			"github.com/org/project/users-->User",
			&APISpecConfig{ShortNames: &trueVal},
			"users.User",
		},
		{
			"short names disabled",
			"github.com/org/project/users-->User",
			&APISpecConfig{ShortNames: &falseVal},
			"github.com_org_project_users.User",
		},
		{
			"nil config",
			"github.com/org/project/users-->User",
			nil,
			"github.com_org_project_users.User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := schemaName(tt.typeName, tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUseShortNames(t *testing.T) {
	trueVal := true
	falseVal := false

	assert.True(t, (&APISpecConfig{}).UseShortNames(), "nil ShortNames → true")
	assert.True(t, (&APISpecConfig{ShortNames: &trueVal}).UseShortNames(), "true → true")
	assert.False(t, (&APISpecConfig{ShortNames: &falseVal}).UseShortNames(), "false → false")
}

func TestBuildPathsFromRoutes_ShortNames(t *testing.T) {
	trueVal := true
	falseVal := false

	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "github.com/org/project/users",
		},
	}

	t.Run("short names enabled", func(t *testing.T) {
		cfg := &APISpecConfig{ShortNames: &trueVal}
		paths := buildPathsFromRoutes(routes, cfg)
		require.NotNil(t, paths["/users"].Get)
		assert.Equal(t, "users.GetUsers", paths["/users"].Get.OperationID)
	})

	t.Run("short names disabled", func(t *testing.T) {
		cfg := &APISpecConfig{ShortNames: &falseVal}
		paths := buildPathsFromRoutes(routes, cfg)
		require.NotNil(t, paths["/users"].Get)
		assert.Equal(t, "github.com/org/project/users.GetUsers", paths["/users"].Get.OperationID)
	})

	t.Run("nil cfg — no shortening", func(t *testing.T) {
		paths := buildPathsFromRoutes(routes, nil)
		require.NotNil(t, paths["/users"].Get)
		assert.Equal(t, "github.com/org/project/users.GetUsers", paths["/users"].Get.OperationID)
	})
}

func TestBuildPathsFromRoutes_ShortNames_DepsChain(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/files/{id}",
			Method:   "GET",
			Function: "Deps-->DocumentHandler-->GetContent",
			Package:  "github.com/alkem-io/file-service-go/internal/adapter/inbound/http",
		},
	}

	cfg := &APISpecConfig{}
	paths := buildPathsFromRoutes(routes, cfg)
	require.NotNil(t, paths["/files/{id}"].Get)
	assert.Equal(t, "DocumentHandler.GetContent", paths["/files/{id}"].Get.OperationID)
}

func TestMapGoTypeToOpenAPISchema_ByteSlice(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	t.Run("[]byte returns string/byte", func(t *testing.T) {
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "[]byte", nil, cfg, nil)
		require.NotNil(t, schema)
		assert.Equal(t, "string", schema.Type)
		assert.Equal(t, "byte", schema.Format)
	})

	t.Run("byte returns integer", func(t *testing.T) {
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "byte", nil, cfg, nil)
		require.NotNil(t, schema)
		assert.Equal(t, "integer", schema.Type)
	})
}
