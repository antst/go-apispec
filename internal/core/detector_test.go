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

package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFrameworkDetector(t *testing.T) {
	detector := NewFrameworkDetector()
	if detector == nil {
		t.Error("NewFrameworkDetector returned nil")
	}
}

func TestDetect_NoGoFiles(t *testing.T) {
	// Create a temporary directory without Go files
	tempDir, err := os.MkdirTemp("", "apispec_test_no_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	detector := NewFrameworkDetector()
	frameworks, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should return ["net/http"] as default when no Go files are found
	if len(frameworks) != 1 || frameworks[0] != "net/http" {
		t.Errorf("Expected [net/http], got %v", frameworks)
	}
}

func TestDetect_WithGoFiles(t *testing.T) {
	// Create a temporary directory with Go files
	tempDir, err := os.MkdirTemp("", "apispec_test_with_go")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a Go file
	goFile := filepath.Join(tempDir, "main.go")
	goContent := `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello"))
	})
}`

	err = os.WriteFile(goFile, []byte(goContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	detector := NewFrameworkDetector()
	frameworks, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should detect net/http framework
	if len(frameworks) != 1 || frameworks[0] != "net/http" {
		t.Errorf("Expected [net/http], got %v", frameworks)
	}
}

func TestDetect_AllFrameworks(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{"gin", "github.com/gin-gonic/gin", "gin"},
		{"chi", "github.com/go-chi/chi/v5", "chi"},
		{"echo", "github.com/labstack/echo/v4", "echo"},
		{"fiber", "github.com/gofiber/fiber/v2", "fiber"},
		{"mux", "github.com/gorilla/mux", "mux"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			goFile := filepath.Join(tempDir, "main.go")
			content := `package main
import "` + tt.importPath + `"
var _ = ` + tt.name + `.New
`
			if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			detector := NewFrameworkDetector()
			fw, err := detector.Detect(tempDir)
			if err != nil {
				t.Fatalf("Detect failed: %v", err)
			}
			if fw != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, fw)
			}
		})
	}
}

func TestDetect_SkipsUnparsableFiles(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "bad.go"), []byte("not valid go"), 0644); err != nil {
		t.Fatal(err)
	}
	detector := NewFrameworkDetector()
	fw, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if fw != "net/http" {
		t.Errorf("Expected net/http fallback, got %s", fw)
	}
}

func TestCollectGoFiles(t *testing.T) {
	// Create a temporary directory with mixed file types
	tempDir, err := os.MkdirTemp("", "apispec_test_collect")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create Go files
	goFiles := []string{"main.go", "handler.go", "utils.go"}
	for _, filename := range goFiles {
		goFile := filepath.Join(tempDir, filename)
		goContent := `package main

func main() {}`

		err = os.WriteFile(goFile, []byte(goContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", filename, err)
		}
	}

	// Create non-Go files
	nonGoFiles := []string{"readme.txt", "config.yaml", "data.json"}
	for _, filename := range nonGoFiles {
		nonGoFile := filepath.Join(tempDir, filename)
		err = os.WriteFile(nonGoFile, []byte("content"), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", filename, err)
		}
	}

	goFilesFound, err := CollectGoFiles(tempDir)
	if err != nil {
		t.Fatalf("CollectGoFiles failed: %v", err)
	}

	// Should find exactly 3 Go files
	if len(goFilesFound) != 3 {
		t.Errorf("Expected 3 Go files, found %d", len(goFilesFound))
	}

	// Check that all expected Go files are found
	for _, expectedFile := range goFiles {
		found := false
		for _, foundFile := range goFilesFound {
			if filepath.Base(foundFile) == expectedFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected Go file %s not found", expectedFile)
		}
	}
}

func TestDetect_MultipleFrameworks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "apispec_test_multi")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a file importing Chi
	chiFile := filepath.Join(tempDir, "chi_routes.go")
	err = os.WriteFile(chiFile, []byte(`package main
import "github.com/go-chi/chi/v5"
func chiRoutes(r chi.Router) {}
`), 0644)
	if err != nil {
		t.Fatalf("Failed to write chi file: %v", err)
	}

	// Create a file importing Gin
	ginFile := filepath.Join(tempDir, "gin_routes.go")
	err = os.WriteFile(ginFile, []byte(`package main
import "github.com/gin-gonic/gin"
func ginRoutes(r *gin.Engine) {}
`), 0644)
	if err != nil {
		t.Fatalf("Failed to write gin file: %v", err)
	}

	detector := NewFrameworkDetector()
	frameworks, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if len(frameworks) != 2 {
		t.Fatalf("Expected 2 frameworks, got %d: %v", len(frameworks), frameworks)
	}

	// Both chi and gin should be detected (order depends on file walk order)
	seen := make(map[string]bool)
	for _, fw := range frameworks {
		seen[fw] = true
	}
	if !seen["chi"] {
		t.Error("Expected chi to be detected")
	}
	if !seen["gin"] {
		t.Error("Expected gin to be detected")
	}
}

func TestDetect_InvalidDirectory(t *testing.T) {
	detector := NewFrameworkDetector()

	// Test with non-existent directory
	_, err := detector.Detect("/non/existent/directory")
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}
