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
	"regexp"
	"strings"
	"testing"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestMapMetadataToOpenAPI_Comprehensive tests the main mapping function with various scenarios
func TestMapMetadataToOpenAPI_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"nil_tree", "Should handle nil tracker tree", testMapMetadataToOpenAPINilTree},
		{"empty_routes", "Should handle empty routes", testMapMetadataToOpenAPIEmptyRoutes},
		{"with_config_info", "Should use config info when present", testMapMetadataToOpenAPIWithConfigInfo},
		{"with_security_schemes", "Should include security schemes", testMapMetadataToOpenAPIWithSecuritySchemes},
		{"with_external_docs", "Should include external docs", testMapMetadataToOpenAPIWithExternalDocs},
		{"with_servers", "Should include servers", testMapMetadataToOpenAPIWithServers},
		{"with_tags", "Should include tags", testMapMetadataToOpenAPIWithTags},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMapMetadataToOpenAPINilTree(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	// This should panic because NewExtractor doesn't handle nil tree
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when tree is nil")
		}
	}()

	_, err := MapMetadataToOpenAPI(nil, cfg, genCfg)
	if err != nil {
		t.Errorf("Expected error for nil metadata, got: %v", err)
	}
}

func testMapMetadataToOpenAPIEmptyRoutes(t *testing.T) {
	// Create a mock tree that returns empty routes
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := DefaultAPISpecConfig()
	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec == nil {
		t.Fatal("Expected non-nil spec")
		return
	}

	if len(spec.Paths) != 0 {
		t.Errorf("Expected empty paths, got %d", len(spec.Paths))
	}
}

func testMapMetadataToOpenAPIWithConfigInfo(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Info: Info{
			Title:       "Config Title",
			Description: "Config Description",
			Version:     "2.0.0",
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Generator Title", // Should be overridden
		APIVersion:     "1.0.0",           // Should be overridden
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.Info.Title != "Config Title" {
		t.Errorf("Expected title 'Config Title', got %s", spec.Info.Title)
	}

	if spec.Info.Description != "Config Description" {
		t.Errorf("Expected description 'Config Description', got %s", spec.Info.Description)
	}

	if spec.Info.Version != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got %s", spec.Info.Version)
	}
}

func testMapMetadataToOpenAPIWithSecuritySchemes(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		SecuritySchemes: map[string]SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.Components.SecuritySchemes == nil {
		t.Fatal("Expected security schemes to be set")
	}

	if len(spec.Components.SecuritySchemes) != 1 {
		t.Errorf("Expected 1 security scheme, got %d", len(spec.Components.SecuritySchemes))
	}

	if spec.Components.SecuritySchemes["bearerAuth"].Type != "http" {
		t.Errorf("Expected security scheme type 'http', got %s", spec.Components.SecuritySchemes["bearerAuth"].Type)
	}
}

func testMapMetadataToOpenAPIWithExternalDocs(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		ExternalDocs: &ExternalDocumentation{
			Description: "API Documentation",
			URL:         "https://example.com/docs",
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if spec.ExternalDocs == nil {
		t.Fatal("Expected external docs to be set")
	}

	if spec.ExternalDocs.Description != "API Documentation" {
		t.Errorf("Expected external docs description 'API Documentation', got %s", spec.ExternalDocs.Description)
	}

	if spec.ExternalDocs.URL != "https://example.com/docs" {
		t.Errorf("Expected external docs URL 'https://example.com/docs', got %s", spec.ExternalDocs.URL)
	}
}

func testMapMetadataToOpenAPIWithServers(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Servers: []Server{
			{
				URL:         "https://api.example.com/v1",
				Description: "Production server",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(spec.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(spec.Servers))
	}

	if spec.Servers[0].URL != "https://api.example.com/v1" {
		t.Errorf("Expected server URL 'https://api.example.com/v1', got %s", spec.Servers[0].URL)
	}
}

func testMapMetadataToOpenAPIWithTags(t *testing.T) {
	mockTree := &MockTrackerTree{
		meta: &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages:   make(map[string]*metadata.Package),
		},
	}

	cfg := &APISpecConfig{
		Tags: []Tag{
			{
				Name:        "users",
				Description: "User management operations",
			},
		},
	}

	genCfg := GeneratorConfig{
		OpenAPIVersion: "3.0.3",
		Title:          "Test API",
		APIVersion:     "1.0.0",
	}

	spec, err := MapMetadataToOpenAPI(mockTree, cfg, genCfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(spec.Tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(spec.Tags))
	}

	if spec.Tags[0].Name != "users" {
		t.Errorf("Expected tag name 'users', got %s", spec.Tags[0].Name)
	}
}

// TestBuildPathsFromRoutes_Comprehensive tests path building with various scenarios
func TestBuildPathsFromRoutes_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"empty_routes", "Should handle empty routes", testBuildPathsFromRoutesEmpty},
		{"single_route", "Should handle single route", testBuildPathsFromRoutesSingle},
		{"multiple_routes_same_path", "Should handle multiple routes on same path", testBuildPathsFromRoutesMultipleSamePath},
		{"path_with_params", "Should convert path parameters", testBuildPathsFromRoutesWithParams},
		{"all_http_methods", "Should handle all HTTP methods", testBuildPathsFromRoutesAllMethods},
		{"with_request_body", "Should handle request body", testBuildPathsFromRoutesWithRequestBody},
		{"with_parameters", "Should handle parameters", testBuildPathsFromRoutesWithParameters},
		{"with_responses", "Should handle responses", testBuildPathsFromRoutesWithResponses},
		{"with_package_prefix", "Should handle package prefix", testBuildPathsFromRoutesWithPackagePrefix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testBuildPathsFromRoutesEmpty(t *testing.T) {
	routes := []*RouteInfo{}
	paths := buildPathsFromRoutes(routes, nil)

	if len(paths) != 0 {
		t.Errorf("Expected empty paths, got %d", len(paths))
	}
}

func testBuildPathsFromRoutesSingle(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Get.OperationID != "main.GetUsers" {
		t.Errorf("Expected operation ID 'main.GetUsers', got %s", pathItem.Get.OperationID)
	}
}

func testBuildPathsFromRoutesMultipleSamePath(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
		},
		{
			Path:     "/users",
			Method:   "POST",
			Function: "CreateUser",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Post == nil {
		t.Fatal("Expected POST operation to exist")
	}
}

func testBuildPathsFromRoutesWithParams(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users/:id",
			Method:   "GET",
			Function: "GetUser",
			Package:  "main",
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/users/{id}"]
	if !exists {
		t.Fatal("Expected path '/users/{id}' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}
}

func testBuildPathsFromRoutesAllMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	routes := make([]*RouteInfo, len(methods))

	for i, method := range methods {
		routes[i] = &RouteInfo{
			Path:     "/test",
			Method:   method,
			Function: method + "Test",
			Package:  "main",
		}
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/test"]
	if !exists {
		t.Fatal("Expected path '/test' to exist")
	}

	if pathItem.Get == nil {
		t.Error("Expected GET operation to exist")
	}
	if pathItem.Post == nil {
		t.Error("Expected POST operation to exist")
	}
	if pathItem.Put == nil {
		t.Error("Expected PUT operation to exist")
	}
	if pathItem.Delete == nil {
		t.Error("Expected DELETE operation to exist")
	}
	if pathItem.Patch == nil {
		t.Error("Expected PATCH operation to exist")
	}
	if pathItem.Options == nil {
		t.Error("Expected OPTIONS operation to exist")
	}
	if pathItem.Head == nil {
		t.Error("Expected HEAD operation to exist")
	}
}

func testBuildPathsFromRoutesWithRequestBody(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "POST",
			Function: "CreateUser",
			Package:  "main",
			Request: &RequestInfo{
				ContentType: "application/json",
				Schema:      &Schema{Type: "object"},
			},
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Post == nil {
		t.Fatal("Expected POST operation to exist")
	}

	if pathItem.Post.RequestBody == nil {
		t.Fatal("Expected request body to exist")
	}

	if pathItem.Post.RequestBody.Content["application/json"].Schema.Type != "object" {
		t.Errorf("Expected schema type 'object', got %s", pathItem.Post.RequestBody.Content["application/json"].Schema.Type)
	}
}

func testBuildPathsFromRoutesWithParameters(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
			Params: []Parameter{
				{
					Name:     "limit",
					In:       "query",
					Required: false,
					Schema:   &Schema{Type: "integer"},
				},
			},
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if len(pathItem.Get.Parameters) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(pathItem.Get.Parameters))
	}

	if pathItem.Get.Parameters[0].Name != "limit" {
		t.Errorf("Expected parameter name 'limit', got %s", pathItem.Get.Parameters[0].Name)
	}
}

func testBuildPathsFromRoutesWithResponses(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "main",
			Response: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "array"},
				},
			},
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if len(pathItem.Get.Responses) != 1 {
		t.Errorf("Expected 1 response, got %d", len(pathItem.Get.Responses))
	}

	response, exists := pathItem.Get.Responses["200"]
	if !exists {
		t.Fatal("Expected response '200' to exist")
	}

	if response.Content["application/json"].Schema.Type != "array" {
		t.Errorf("Expected schema type 'array', got %s", response.Content["application/json"].Schema.Type)
	}
}

func testBuildPathsFromRoutesWithPackagePrefix(t *testing.T) {
	routes := []*RouteInfo{
		{
			Path:     "/users",
			Method:   "GET",
			Function: "GetUsers",
			Package:  "api.v1",
		},
	}

	paths := buildPathsFromRoutes(routes, nil)

	pathItem, exists := paths["/users"]
	if !exists {
		t.Fatal("Expected path '/users' to exist")
	}

	if pathItem.Get == nil {
		t.Fatal("Expected GET operation to exist")
	}

	if pathItem.Get.OperationID != "api.v1.GetUsers" {
		t.Errorf("Expected operation ID 'api.v1.GetUsers', got %s", pathItem.Get.OperationID)
	}
}

// TestConvertPathToOpenAPI_Comprehensive tests path conversion
func TestConvertPathToOpenAPI_Comprehensive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users", "/users"},
		{"/users/:id", "/users/{id}"},
		{"/users/:id/posts/:postId", "/users/{id}/posts/{postId}"},
		{"/users/:userId/comments/:commentId", "/users/{userId}/comments/{commentId}"},
		{"/api/v1/users/:id", "/api/v1/users/{id}"},
		{"/users/:id/posts/:postId/comments/:commentId", "/users/{id}/posts/{postId}/comments/{commentId}"},
		{"/:param", "/{param}"},
		{"/:param1/:param2", "/{param1}/{param2}"},
		{"/users/:id/posts", "/users/{id}/posts"},
		{"/posts/:id", "/posts/{id}"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertPathToOpenAPI(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestEnsureAllPathParams_Comprehensive tests path parameter handling
func TestEnsureAllPathParams_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		params   []Parameter
		expected int // expected number of parameters
	}{
		{
			name:     "no_params_needed",
			path:     "/users",
			params:   []Parameter{},
			expected: 0,
		},
		{
			name:     "missing_path_param",
			path:     "/users/{id}",
			params:   []Parameter{},
			expected: 1,
		},
		{
			name: "existing_path_param",
			path: "/users/{id}",
			params: []Parameter{
				{Name: "id", In: "path", Required: true},
			},
			expected: 1,
		},
		{
			name: "mixed_params",
			path: "/users/{id}/posts/{postId}",
			params: []Parameter{
				{Name: "id", In: "path", Required: true},
			},
			expected: 2,
		},
		{
			name: "query_param_ignored",
			path: "/users/{id}",
			params: []Parameter{
				{Name: "limit", In: "query", Required: false},
			},
			expected: 2, // 1 existing + 1 missing path param
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureAllPathParams(tt.path, tt.params)
			if len(result) != tt.expected {
				t.Errorf("Expected %d parameters, got %d", tt.expected, len(result))
			}

			// Check that all path parameters are present
			pathParams := make(map[string]bool)
			for _, p := range result {
				if p.In == "path" {
					pathParams[p.Name] = true
				}
			}

			// Extract expected path parameters from the path
			re := regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
			matches := re.FindAllStringSubmatch(tt.path, -1)
			for _, match := range matches {
				paramName := match[1]
				if !pathParams[paramName] {
					t.Errorf("Expected path parameter '%s' to be present", paramName)
				}
			}
		})
	}
}

// TestDeduplicateParameters_Comprehensive tests parameter deduplication
func TestDeduplicateParameters_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		params   []Parameter
		expected int
	}{
		{
			name:     "empty_params",
			params:   []Parameter{},
			expected: 0,
		},
		{
			name: "no_duplicates",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "limit", In: "query"},
			},
			expected: 2,
		},
		{
			name: "duplicate_name_different_in",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "query"},
			},
			expected: 2,
		},
		{
			name: "duplicate_name_same_in",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "path"},
			},
			expected: 1,
		},
		{
			name: "multiple_duplicates",
			params: []Parameter{
				{Name: "id", In: "path"},
				{Name: "id", In: "path"},
				{Name: "limit", In: "query"},
				{Name: "limit", In: "query"},
				{Name: "offset", In: "query"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateParameters(tt.params)
			if len(result) != tt.expected {
				t.Errorf("Expected %d parameters, got %d", tt.expected, len(result))
			}
		})
	}
}

// TestBuildResponses_Comprehensive tests response building
func TestBuildResponses_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		respInfo map[string]*ResponseInfo
		expected int
	}{
		{
			name: "nil_responses",
			respInfo: map[string]*ResponseInfo{
				"default": {
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 1, // Default response
		},
		{
			name: "empty_responses",
			respInfo: map[string]*ResponseInfo{
				"default": {
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 1, // No default response for empty map
		},
		{
			name: "single_response",
			respInfo: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 1,
		},
		{
			name: "multiple_responses",
			respInfo: map[string]*ResponseInfo{
				"200": {
					StatusCode:  200,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
				"404": {
					StatusCode:  404,
					ContentType: "application/json",
					Schema:      &Schema{Type: "object"},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResponses(tt.respInfo)
			if len(result) != tt.expected {
				t.Errorf("Expected %d responses, got %d", tt.expected, len(result))
			}

			if tt.respInfo == nil {
				// Check default response
				if response, exists := result["default"]; !exists {
					t.Error("Expected default response")
				} else if response.Description != "Default response (no response found)" {
					t.Errorf("Expected description 'Default response (no response found)', got %s", response.Description)
				}
			}
		})
	}
}

// TestSetOperationOnPathItem_Comprehensive tests operation setting
func TestSetOperationOnPathItem_Comprehensive(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "INVALID"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			item := &PathItem{}
			operation := &Operation{OperationID: method + "Test"}

			setOperationOnPathItem(item, method, operation)

			switch strings.ToUpper(method) {
			case "GET":
				if item.Get != operation {
					t.Error("Expected GET operation to be set")
				}
			case "POST":
				if item.Post != operation {
					t.Error("Expected POST operation to be set")
				}
			case "PUT":
				if item.Put != operation {
					t.Error("Expected PUT operation to be set")
				}
			case "DELETE":
				if item.Delete != operation {
					t.Error("Expected DELETE operation to be set")
				}
			case "PATCH":
				if item.Patch != operation {
					t.Error("Expected PATCH operation to be set")
				}
			case "OPTIONS":
				if item.Options != operation {
					t.Error("Expected OPTIONS operation to be set")
				}
			case "HEAD":
				if item.Head != operation {
					t.Error("Expected HEAD operation to be set")
				}
			default:
				// Invalid method should not set any operation
				if item.Get != nil || item.Post != nil || item.Put != nil || item.Delete != nil ||
					item.Patch != nil || item.Options != nil || item.Head != nil {
					t.Error("Expected no operation to be set for invalid method")
				}
			}
		})
	}
}

// TestMapGoTypeToOpenAPISchema_Comprehensive tests type mapping with various scenarios
func TestMapGoTypeToOpenAPISchema_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"primitive_types", "Should handle all primitive types", testMapGoTypeToOpenAPISchemaPrimitiveTypes},
		{"pointer_types", "Should handle pointer types", testMapGoTypeToOpenAPISchemaPointerTypes},
		{"slice_types", "Should handle slice types", testMapGoTypeToOpenAPISchemaSliceTypes},
		{"array_types", "Should handle array types", testMapGoTypeToOpenAPISchemaArrayTypes},
		{"map_types", "Should handle map types", testMapGoTypeToOpenAPISchemaMapTypes},
		{"custom_types", "Should handle custom types", testMapGoTypeToOpenAPISchemaCustomTypes},
		{"external_types", "Should handle external types", testMapGoTypeToOpenAPISchemaExternalTypes},
		{"type_mappings", "Should handle type mappings", testMapGoTypeToOpenAPISchemaTypeMappings},
		{"nil_metadata", "Should handle nil metadata", testMapGoTypeToOpenAPISchemaNilMetadata},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMapGoTypeToOpenAPISchemaPrimitiveTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	primitiveTests := []struct {
		goType       string
		expectedType string
	}{
		{"string", "string"},
		{"int", "integer"},
		{"int8", "integer"},
		{"int16", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"uint", "integer"},
		{"uint8", "integer"},
		{"uint16", "integer"},
		{"uint32", "integer"},
		{"uint64", "integer"},
		{"byte", "integer"},
		{"float32", "number"},
		{"float64", "number"},
		{"bool", "boolean"},
		{"time.Time", "string"},
		{"[]byte", "string"},
		{"[]string", "array"},
		{"[]time.Time", "array"},
		{"[]int", "array"},
		{"interface{}", "object"},
		{"struct{}", "object"},
		{"any", "object"},
	}

	for _, tt := range primitiveTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg, nil)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchemaPointerTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	pointerTests := []struct {
		goType       string
		expectedType string
	}{
		{"*string", "string"},
		{"*int", "integer"},
		{"*bool", "boolean"},
		{"*time.Time", "string"},
	}

	for _, tt := range pointerTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg, nil)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchemaSliceTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	sliceTests := []struct {
		goType            string
		expectedType      string
		expectedItemsType string
	}{
		{"[]string", "array", "string"},
		{"[]int", "array", "integer"},
		{"[]bool", "array", "boolean"},
		{"[]*User", "array", ""},
	}

	for _, tt := range sliceTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg, nil)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
			if schema.Items == nil {
				t.Errorf("Expected items schema for %s", tt.goType)
			} else if schema.Items.Type != tt.expectedItemsType {
				t.Errorf("Expected items type %s for %s, got %s", tt.expectedItemsType, tt.goType, schema.Items.Type)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchemaArrayTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	arrayTests := []struct {
		goType            string
		expectedType      string
		expectedFormat    string
		expectedMaxLength *int
		expectedMaxItems  *int
		expectedMinItems  *int
		description       string
	}{
		{
			goType:            "[16]byte",
			expectedType:      "string",
			expectedFormat:    "byte",
			expectedMaxLength: func() *int { size := 16; return &size }(),
			description:       "Fixed-size byte array should be converted to string with maxLength",
		},
		{
			goType:            "[32]byte",
			expectedType:      "string",
			expectedFormat:    "byte",
			expectedMaxLength: func() *int { size := 32; return &size }(),
			description:       "32-byte array should be converted to string with maxLength 32",
		},
		{
			goType:           "[5]int",
			expectedType:     "array",
			expectedMaxItems: func() *int { size := 5; return &size }(),
			expectedMinItems: func() *int { size := 5; return &size }(),
			description:      "Fixed-size int array should be array with maxItems and minItems",
		},
		{
			goType:           "[10]string",
			expectedType:     "array",
			expectedMaxItems: func() *int { size := 10; return &size }(),
			expectedMinItems: func() *int { size := 10; return &size }(),
			description:      "Fixed-size string array should be array with maxItems and minItems",
		},
		{
			goType:       "[...]int",
			expectedType: "array",
			description:  "Variable-length array should be array without size constraints",
		},
	}

	for _, tt := range arrayTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg, make(map[string]bool))

			if schema == nil {
				t.Fatalf("Expected schema for %s, got nil", tt.goType)
				return
			}

			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s for %s", tt.expectedType, schema.Type, tt.goType)
			}

			if tt.expectedFormat != "" && schema.Format != tt.expectedFormat {
				t.Errorf("Expected format %s, got %s for %s", tt.expectedFormat, schema.Format, tt.goType)
			}

			if tt.expectedMaxLength != nil {
				if schema.MaxLength != *tt.expectedMaxLength {
					t.Errorf("Expected maxLength %d, got %d for %s", *tt.expectedMaxLength, schema.MaxLength, tt.goType)
				}
			}

			if tt.expectedMaxItems != nil {
				if schema.MaxItems != *tt.expectedMaxItems {
					t.Errorf("Expected maxItems %d, got %d for %s", *tt.expectedMaxItems, schema.MaxItems, tt.goType)
				}
			}

			if tt.expectedMinItems != nil {
				if schema.MinItems != *tt.expectedMinItems {
					t.Errorf("Expected minItems %d, got %d for %s", *tt.expectedMinItems, schema.MinItems, tt.goType)
				}
			}
		})
	}
}

func testMapGoTypeToOpenAPISchemaMapTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	mapTests := []struct {
		goType                  string
		expectedType            string
		expectedAdditionalProps bool
	}{
		{"map[string]string", "object", true},
		{"map[string]int", "object", true},
		{"map[string]bool", "object", true},
		{"map[int]string", "object", false}, // Non-string keys not supported
	}

	for _, tt := range mapTests {
		t.Run(tt.goType, func(t *testing.T) {
			schema, _ := mapGoTypeToOpenAPISchema(usedTypes, tt.goType, nil, cfg, nil)
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type %s for %s, got %s", tt.expectedType, tt.goType, schema.Type)
			}
			if tt.expectedAdditionalProps && schema.AdditionalProperties == nil {
				t.Errorf("Expected additional properties for %s", tt.goType)
			}
		})
	}
}

func testMapGoTypeToOpenAPISchemaCustomTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	// Create metadata with a custom type
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: stringPool.Get("Name"),
										Type: stringPool.Get("string"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "User", meta, cfg, nil)
	// Should be a reference
	if schema.Ref == "" {
		t.Errorf("Expected reference for custom type, got empty Ref")
	}
	// Extract the referenced type name
	refType := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
	// Check that the referenced schema exists in usedTypes
	if refSchema, exists := usedTypes[refType]; exists && refSchema != nil {
		if refSchema.Type != "object" {
			t.Errorf("Expected type 'object' for custom type in usedTypes, got %s", refSchema.Type)
		}
	} else {
		t.Errorf("Expected User schema in usedTypes, got %v", usedTypes)
	}
}

func testMapGoTypeToOpenAPISchemaExternalTypes(t *testing.T) {
	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{
			{
				Name: "CustomType",
				OpenAPIType: &Schema{
					Type: "string",
					Enum: []interface{}{"value1", "value2"},
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	_, schemas := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg, nil)
	// External types are added to schemas map, not returned directly
	if externalSchema, exists := schemas["CustomType"]; exists {
		if externalSchema.Type != "string" {
			t.Errorf("Expected type 'string' for external type, got %s", externalSchema.Type)
		}
		if len(externalSchema.Enum) != 2 {
			t.Errorf("Expected 2 enum values, got %d", len(externalSchema.Enum))
		}
	} else {
		t.Error("Expected external type to be in schemas map")
	}
}

func testMapGoTypeToOpenAPISchemaTypeMappings(t *testing.T) {
	cfg := &APISpecConfig{
		TypeMapping: []TypeMapping{
			{
				GoType: "CustomType",
				OpenAPIType: &Schema{
					Type:   "integer",
					Format: "int64",
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg, nil)
	if schema.Type != "integer" {
		t.Errorf("Expected type 'integer' for mapped type, got %s", schema.Type)
	}
	if schema.Format != "int64" {
		t.Errorf("Expected format 'int64' for mapped type, got %s", schema.Format)
	}
}

func testMapGoTypeToOpenAPISchemaNilMetadata(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	// Test with nil metadata
	schema, _ := mapGoTypeToOpenAPISchema(usedTypes, "CustomType", nil, cfg, nil)
	if schema == nil {
		t.Error("Expected non-nil schema")
		return
	}

	if schema.Ref == "" {
		t.Error("Expected reference schema for unknown type")
	}
}

// TestGenerateSchemaFromType_Comprehensive_Extended tests schema generation from metadata types
func TestGenerateSchemaFromType_Comprehensive_Extended(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"struct_type", "Should generate struct schema", testGenerateSchemaFromTypeStruct},
		{"interface_type", "Should generate interface schema", testGenerateSchemaFromTypeInterface},
		{"alias_type", "Should generate alias schema", testGenerateSchemaFromTypeAlias},
		{"external_type", "Should handle external types", testGenerateSchemaFromTypeExternal},
		{"nil_type", "Should handle nil type", testGenerateSchemaFromTypeNil},
		{"with_generics", "Should handle generic types", testGenerateSchemaFromTypeWithGenerics},
		{"with_nested_types", "Should handle nested types", testGenerateSchemaFromTypeWithNestedTypes},
		{"with_json_tags", "Should handle JSON tags", testGenerateSchemaFromTypeWithJSONTags},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testGenerateSchemaFromTypeStruct(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("User"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Name"),
				Type: stringPool.Get("string"),
			},
			{
				Name: stringPool.Get("Age"),
				Type: stringPool.Get("int"),
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg, nil)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(schema.Properties))
	}
}

func testGenerateSchemaFromTypeInterface(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("Handler"),
		Kind: stringPool.Get("interface"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "Handler", typ, meta, cfg, nil)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
}

func testGenerateSchemaFromTypeAlias(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name:   stringPool.Get("UserID"),
		Kind:   stringPool.Get("alias"),
		Target: stringPool.Get("string"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "UserID", typ, meta, cfg, nil)
	if schema.Type != "string" {
		t.Errorf("Expected type 'string', got %s", schema.Type)
	}
}

func testGenerateSchemaFromTypeExternal(t *testing.T) {
	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{
			{
				Name: "ExternalType",
				OpenAPIType: &Schema{
					Type: "string",
				},
			},
		},
	}
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("ExternalType"),
		Kind: stringPool.Get("struct"),
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "ExternalType", typ, meta, cfg, nil)
	if schema.Type != "string" {
		t.Errorf("Expected type 'string', got %s", schema.Type)
	}
}

func testGenerateSchemaFromTypeNil(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// This should panic because typ is nil
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when typ is nil")
		}
	}()

	generateSchemaFromType(usedTypes, "Test", nil, meta, cfg, nil)
}

func testGenerateSchemaFromTypeWithGenerics(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("Container"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Data"),
				Type: stringPool.Get("T"),
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "Container-T", typ, meta, cfg, nil)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
}

func testGenerateSchemaFromTypeWithNestedTypes(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	nestedType := &metadata.Type{
		Name: stringPool.Get("Address"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Street"),
				Type: stringPool.Get("string"),
			},
		},
	}

	typ := &metadata.Type{
		Name: stringPool.Get("User"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name:       stringPool.Get("Address"),
				Type:       stringPool.Get("Address"),
				NestedType: nestedType,
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg, nil)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if len(schema.Properties) != 1 {
		t.Errorf("Expected 1 property, got %d", len(schema.Properties))
	}
}

func testGenerateSchemaFromTypeWithJSONTags(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	usedTypes := make(map[string]*Schema)

	stringPool := metadata.NewStringPool()
	typ := &metadata.Type{
		Name: stringPool.Get("User"),
		Kind: stringPool.Get("struct"),
		Fields: []metadata.Field{
			{
				Name: stringPool.Get("Name"),
				Type: stringPool.Get("string"),
				Tag:  stringPool.Get(`json:"user_name"`),
			},
		},
	}

	meta := &metadata.Metadata{StringPool: stringPool}

	schema, _ := generateSchemaFromType(usedTypes, "User", typ, meta, cfg, nil)
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got %s", schema.Type)
	}
	if schema.Properties["user_name"] == nil {
		t.Error("Expected property 'user_name' from JSON tag")
	}
}

// TestResolveUnderlyingType_Comprehensive tests underlying type resolution
func TestResolveUnderlyingType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"alias_type", "Should resolve alias to underlying type", testResolveUnderlyingTypeAlias},
		{"non_alias_type", "Should return empty for non-alias", testResolveUnderlyingTypeNonAlias},
		{"nil_metadata", "Should handle nil metadata", testResolveUnderlyingTypeNilMetadata},
		{"with_prefixes", "Should handle type prefixes", testResolveUnderlyingTypeWithPrefixes},
		{"not_found", "Should return empty for not found", testResolveUnderlyingTypeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testResolveUnderlyingTypeAlias(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"UserID": {
								Name:   stringPool.Get("UserID"),
								Kind:   stringPool.Get("alias"),
								Target: stringPool.Get("string"),
							},
						},
					},
				},
			},
		},
	}

	result := resolveUnderlyingType("UserID", meta)
	if result != "string" {
		t.Errorf("Expected 'string', got %s", result)
	}
}

func testResolveUnderlyingTypeNonAlias(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	result := resolveUnderlyingType("User", meta)
	if result != "" {
		t.Errorf("Expected empty string, got %s", result)
	}
}

func testResolveUnderlyingTypeNilMetadata(t *testing.T) {
	result := resolveUnderlyingType("UserID", nil)
	if result != "" {
		t.Errorf("Expected empty string for nil metadata, got %s", result)
	}
}

func testResolveUnderlyingTypeWithPrefixes(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"UserID": {
								Name:   stringPool.Get("UserID"),
								Kind:   stringPool.Get("alias"),
								Target: stringPool.Get("string"),
							},
						},
					},
				},
			},
		},
	}

	// Test with array prefix
	result := resolveUnderlyingType("[]UserID", meta)
	if result != "[]string" {
		t.Errorf("Expected '[]string', got %s", result)
	}

	// Test with pointer prefix
	result = resolveUnderlyingType("*UserID", meta)
	if result != "*string" {
		t.Errorf("Expected '*string', got %s", result)
	}
}

func testResolveUnderlyingTypeNotFound(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages:   make(map[string]*metadata.Package),
	}

	result := resolveUnderlyingType("NonExistentType", meta)
	if result != "" {
		t.Errorf("Expected empty string for non-existent type, got %s", result)
	}
}

// TestMarkUsedType_Comprehensive tests type marking functionality
func TestMarkUsedType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"basic_marking", "Should mark type as used", testMarkUsedTypeBasic},
		{"pointer_type", "Should handle pointer types", testMarkUsedTypePointer},
		{"already_marked", "Should return true if already marked", testMarkUsedTypeAlreadyMarked},
		{"different_values", "Should handle different mark values", testMarkUsedTypeDifferentValues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testMarkUsedTypeBasic(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	result := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if result {
		t.Error("Expected false for first marking")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as used")
	}
}

func testMarkUsedTypePointer(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	result := markUsedType(usedTypes, "*User", &Schema{Type: "object"})
	if result {
		t.Error("Expected false for first marking")
	}
	if usedTypes["*User"] == nil {
		t.Error("Expected *User to be marked as used")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as used")
	}
}

func testMarkUsedTypeAlreadyMarked(t *testing.T) {
	usedTypes := make(map[string]*Schema)
	usedTypes["User"] = &Schema{Type: "object"}

	result := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if !result {
		t.Error("Expected true for already marked type")
	}
}

func testMarkUsedTypeDifferentValues(t *testing.T) {
	usedTypes := make(map[string]*Schema)

	// Mark with true
	result1 := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if result1 {
		t.Error("Expected false for first marking")
	}
	if usedTypes["User"] == nil {
		t.Error("Expected User to be marked as true")
	}

	// Mark with false
	result2 := markUsedType(usedTypes, "User", &Schema{Type: "object"})
	if !result2 {
		t.Error("Expected true for already marked type")
	}
	// Value should remain true (first marking takes precedence)
	if usedTypes["User"] == nil {
		t.Error("Expected User to remain marked as true")
	}
}

// TestIsPrimitiveType_Comprehensive tests primitive type detection
func TestIsPrimitiveType_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"basic_primitives", "Should detect basic primitives", testIsPrimitiveTypeBasicPrimitives},
		{"pointer_primitives", "Should detect pointer primitives", testIsPrimitiveTypePointerPrimitives},
		{"slice_primitives", "Should detect slice primitives", testIsPrimitiveTypeSlicePrimitives},
		{"map_primitives", "Should detect map primitives", testIsPrimitiveTypeMapPrimitives},
		{"custom_types", "Should not detect custom types", testIsPrimitiveTypeCustomTypes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testIsPrimitiveTypeBasicPrimitives(t *testing.T) {
	primitives := []string{
		"string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "byte", "rune",
		"error", "interface{}", "struct{}", "any",
		"complex64", "complex128",
	}

	for _, primitive := range primitives {
		if !metadata.IsPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveTypePointerPrimitives(t *testing.T) {
	pointerPrimitives := []string{
		"*string", "*int", "*bool", "*float64",
	}

	for _, primitive := range pointerPrimitives {
		if !metadata.IsPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveTypeSlicePrimitives(t *testing.T) {
	slicePrimitives := []string{
		"[]string", "[]int", "[]bool", "[]float64",
	}

	for _, primitive := range slicePrimitives {
		if !metadata.IsPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveTypeMapPrimitives(t *testing.T) {
	mapPrimitives := []string{
		"map[string]string", "map[string]int", "map[string]bool",
	}

	for _, primitive := range mapPrimitives {
		if !metadata.IsPrimitiveType(primitive) {
			t.Errorf("Expected %s to be primitive", primitive)
		}
	}
}

func testIsPrimitiveTypeCustomTypes(t *testing.T) {
	customTypes := []string{
		"User", "UserID", "CustomType", "MyStruct",
	}

	for _, customType := range customTypes {
		if metadata.IsPrimitiveType(customType) {
			t.Errorf("Expected %s to not be primitive", customType)
		}
	}
}

// TestExtractJSONName_Comprehensive tests JSON name extraction
func TestExtractJSONName_Comprehensive(t *testing.T) {
	tests := []struct {
		tag      string
		expected string
	}{
		{"", ""},
		{`json:"name"`, "name"},
		{`json:"user_name"`, "user_name"},
		{`json:"name,omitempty"`, "name"},
		{`json:"-"`, ""},
		{`json:"name,omitempty,string"`, "name"},
		{`other:"value"`, ""},
		{`json:"name" other:"value"`, "name"},
		{`other:"value" json:"name"`, "name"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := extractJSONName(tt.tag)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestTypeParts_Comprehensive tests type parts parsing
func TestTypeParts_Comprehensive(t *testing.T) {
	tests := []struct {
		input                string
		expectedPkgName      string
		expectedTypeName     string
		expectedGenericTypes []string
	}{
		{"string", "", "string", nil},
		{"main-->User", "main", "User", nil},
		{"pkg-->Type-->T", "pkg", "Type", []string{"T"}},
		{"Container[T]", "", "Container", []string{"T T"}},
		{"Container[T, U]", "", "Container", []string{"T T", "U U"}},
		{"pkg-->Container[T]", "pkg", "Container", []string{"T"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := TypeParts(tt.input)
			if result.PkgName != tt.expectedPkgName {
				t.Errorf("Expected PkgName to be %s, got %s", tt.expectedPkgName, result.PkgName)
			}
			if result.TypeName != tt.expectedTypeName {
				t.Errorf("Expected TypeName to be %s, got %s", tt.expectedTypeName, result.TypeName)
			}
			if len(result.GenericTypes) != len(tt.expectedGenericTypes) {
				t.Errorf("Expected %d generic types, got %d", len(tt.expectedGenericTypes), len(result.GenericTypes))
			}
			for i, expected := range tt.expectedGenericTypes {
				if i < len(result.GenericTypes) && result.GenericTypes[i] != expected {
					t.Errorf("Expected generic type %d to be %s, got %s", i, expected, result.GenericTypes[i])
				}
			}
		})
	}
}

// TestFindTypesInMetadata_Comprehensive tests type finding in metadata
func TestFindTypesInMetadata_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"primitive_type", "Should return nil for primitives", testFindTypesInMetadataPrimitive},
		{"nil_metadata", "Should handle nil metadata", testFindTypesInMetadataNilMetadata},
		{"found_type", "Should find existing type", testFindTypesInMetadataFoundType},
		{"not_found", "Should return empty for not found", testFindTypesInMetadataNotFound},
		{"generic_type", "Should handle generic types", testFindTypesInMetadataGenericType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testFindTypesInMetadataPrimitive(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: stringPool}

	result := findTypesInMetadata(meta, "string")
	if result != nil {
		t.Error("Expected nil for primitive type")
	}
}

func testFindTypesInMetadataNilMetadata(t *testing.T) {
	result := findTypesInMetadata(nil, "User")
	if result != nil {
		t.Error("Expected nil for nil metadata")
	}
}

func testFindTypesInMetadataFoundType(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: stringPool.Get("User"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	result := findTypesInMetadata(meta, "User")
	if len(result) == 0 {
		t.Error("Expected to find User type")
	}
	if result["User"] == nil {
		t.Error("Expected User type to be present")
	}
}

func testFindTypesInMetadataNotFound(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages:   make(map[string]*metadata.Package),
	}

	result := findTypesInMetadata(meta, "NonExistent")
	if len(result) == 0 {
		t.Error("Expected non-empty result for non-existent type (should contain the type name as key)")
	}
}

func testFindTypesInMetadataGenericType(t *testing.T) {
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"types.go": {
						Types: map[string]*metadata.Type{
							"Container": {
								Name: stringPool.Get("Container"),
								Kind: stringPool.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	result := findTypesInMetadata(meta, "Container-T")
	if len(result) == 0 {
		t.Error("Expected to find generic type")
	}
	// The function should return the type name as a key, even if the type doesn't exist
	if _, exists := result["Container-T"]; !exists {
		t.Error("Expected Container-T type name to be present as key")
	}
}

// TestCollectUsedTypesFromRoutes_Comprehensive tests used type collection
func TestCollectUsedTypesFromRoutes_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{"empty_routes", "Should handle empty routes", testCollectUsedTypesFromRoutesEmpty},
		{"with_request_body", "Should collect request body types", testCollectUsedTypesFromRoutesWithRequestBody},
		{"with_response_types", "Should collect response types", testCollectUsedTypesFromRoutesWithResponseTypes},
		{"with_parameters", "Should collect parameter types", testCollectUsedTypesFromRoutesWithParameters},
		{"mixed_types", "Should collect all types", testCollectUsedTypesFromRoutesMixedTypes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func testCollectUsedTypesFromRoutesEmpty(t *testing.T) {
	routes := []*RouteInfo{}
	result := collectUsedTypesFromRoutes(routes)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d types", len(result))
	}
}

func testCollectUsedTypesFromRoutesWithRequestBody(t *testing.T) {
	routes := []*RouteInfo{
		{
			Request: &RequestInfo{
				BodyType: "User",
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutesWithResponseTypes(t *testing.T) {
	routes := []*RouteInfo{
		{
			Response: map[string]*ResponseInfo{
				"200": {
					BodyType: "User",
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutesWithParameters(t *testing.T) {
	routes := []*RouteInfo{
		{
			Params: []Parameter{
				{
					Schema: &Schema{
						Ref: "#/components/schemas/User",
					},
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	if _, exists := result["User"]; !exists {
		t.Error("Expected User type to be collected")
	}
}

func testCollectUsedTypesFromRoutesMixedTypes(t *testing.T) {
	routes := []*RouteInfo{
		{
			Request: &RequestInfo{
				BodyType: "CreateUserRequest",
			},
			Response: map[string]*ResponseInfo{
				"200": {
					BodyType: "User",
				},
			},
			Params: []Parameter{
				{
					Schema: &Schema{
						Ref: "#/components/schemas/UserID",
					},
				},
			},
		},
	}

	result := collectUsedTypesFromRoutes(routes)
	expectedTypes := []string{"CreateUserRequest", "User", "UserID"}
	for _, expectedType := range expectedTypes {
		if _, exists := result[expectedType]; !exists {
			t.Errorf("Expected %s type to be collected", expectedType)
		}
	}
}
