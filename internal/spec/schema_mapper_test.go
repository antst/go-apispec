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
	"net/http"
	"testing"
)

func TestNewSchemaMapper(t *testing.T) {
	cfg := &APISpecConfig{}
	mapper := NewSchemaMapper(cfg)

	if mapper == nil {
		t.Fatal("NewSchemaMapper returned nil")
		return
	}

	if mapper.cfg != cfg {
		t.Error("SchemaMapper config not set correctly")
	}
}

func TestMapStatusCode(t *testing.T) {
	cfg := &APISpecConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name        string
		statusStr   string
		expected    int
		shouldMatch bool
	}{
		// net/http constants
		{
			name:        "StatusOK",
			statusStr:   "StatusOK",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		{
			name:        "StatusCreated",
			statusStr:   "StatusCreated",
			expected:    http.StatusCreated,
			shouldMatch: true,
		},
		{
			name:        "StatusAccepted",
			statusStr:   "StatusAccepted",
			expected:    http.StatusAccepted,
			shouldMatch: true,
		},
		{
			name:        "StatusNoContent",
			statusStr:   "StatusNoContent",
			expected:    http.StatusNoContent,
			shouldMatch: true,
		},
		{
			name:        "StatusBadRequest",
			statusStr:   "StatusBadRequest",
			expected:    http.StatusBadRequest,
			shouldMatch: true,
		},
		{
			name:        "StatusUnauthorized",
			statusStr:   "StatusUnauthorized",
			expected:    http.StatusUnauthorized,
			shouldMatch: true,
		},
		{
			name:        "StatusForbidden",
			statusStr:   "StatusForbidden",
			expected:    http.StatusForbidden,
			shouldMatch: true,
		},
		{
			name:        "StatusNotFound",
			statusStr:   "StatusNotFound",
			expected:    http.StatusNotFound,
			shouldMatch: true,
		},
		{
			name:        "StatusConflict",
			statusStr:   "StatusConflict",
			expected:    http.StatusConflict,
			shouldMatch: true,
		},
		{
			name:        "StatusInternalServerError",
			statusStr:   "StatusInternalServerError",
			expected:    http.StatusInternalServerError,
			shouldMatch: true,
		},
		{
			name:        "StatusNotImplemented",
			statusStr:   "StatusNotImplemented",
			expected:    http.StatusNotImplemented,
			shouldMatch: true,
		},
		{
			name:        "StatusBadGateway",
			statusStr:   "StatusBadGateway",
			expected:    http.StatusBadGateway,
			shouldMatch: true,
		},
		{
			name:        "StatusServiceUnavailable",
			statusStr:   "StatusServiceUnavailable",
			expected:    http.StatusServiceUnavailable,
			shouldMatch: true,
		},
		// net/http prefixed constants
		{
			name:        "net/http.StatusOK",
			statusStr:   "net/http.StatusOK",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		// quoted strings
		{
			name:        "quoted StatusOK",
			statusStr:   "\"StatusOK\"",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		{
			name:        "quoted net/http.StatusOK",
			statusStr:   "\"net/http.StatusOK\"",
			expected:    http.StatusOK,
			shouldMatch: true,
		},
		// numeric strings
		{
			name:        "numeric 200",
			statusStr:   "200",
			expected:    200,
			shouldMatch: true,
		},
		{
			name:        "numeric 404",
			statusStr:   "404",
			expected:    404,
			shouldMatch: true,
		},
		{
			name:        "numeric 500",
			statusStr:   "500",
			expected:    500,
			shouldMatch: true,
		},
		// invalid cases
		{
			name:        "invalid string",
			statusStr:   "InvalidStatus",
			expected:    0,
			shouldMatch: false,
		},
		{
			name:        "empty string",
			statusStr:   "",
			expected:    0,
			shouldMatch: false,
		},
		{
			name:        "non-numeric string",
			statusStr:   "abc",
			expected:    0,
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, matched := mapper.MapStatusCode(tt.statusStr)
			if matched != tt.shouldMatch {
				t.Errorf("expected match %v, got %v", tt.shouldMatch, matched)
			}
			if matched && result != tt.expected {
				t.Errorf("expected status %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestMapMethodFromFunctionName(t *testing.T) {
	cfg := &APISpecConfig{}
	mapper := NewSchemaMapper(cfg)

	tests := []struct {
		name     string
		funcName string
		expected string
	}{
		{
			name:     "getUsers",
			funcName: "getUsers",
			expected: "GET",
		},
		{
			name:     "createUser",
			funcName: "createUser",
			expected: "",
		},
		{
			name:     "updateUser",
			funcName: "updateUser",
			expected: "",
		},
		{
			name:     "deleteUser",
			funcName: "deleteUser",
			expected: "DELETE",
		},
		{
			name:     "patchUser",
			funcName: "patchUser",
			expected: "PATCH",
		},
		{
			name:     "optionsHandler",
			funcName: "optionsHandler",
			expected: "OPTIONS",
		},
		{
			name:     "headRequest",
			funcName: "headRequest",
			expected: "HEAD",
		},
		{
			name:     "mixed case GetUser",
			funcName: "GetUser",
			expected: "GET",
		},
		{
			name:     "mixed case POSTHandler",
			funcName: "POSTHandler",
			expected: "POST",
		},
		{
			name:     "no method in name",
			funcName: "handler",
			expected: "",
		},
		{
			name:     "empty function name",
			funcName: "",
			expected: "",
		},
		{
			name:     "multiple methods - first wins",
			funcName: "getPostUser",
			expected: "GET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapMethodFromFunctionName(tt.funcName)
			if result != tt.expected {
				t.Errorf("expected method %s, got %s", tt.expected, result)
			}
		})
	}
}

// schemasEqual is defined in extractor.go (same package)
