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
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// FrameworkDetector detects the web framework used in a project
type FrameworkDetector struct{}

// NewFrameworkDetector creates a new framework detector
func NewFrameworkDetector() *FrameworkDetector {
	return &FrameworkDetector{}
}

// Detect determines which web frameworks are used in the given directory.
// Returns all detected frameworks. Falls back to ["net/http"] if none found.
func (d *FrameworkDetector) Detect(dir string) ([]string, error) {
	// Collect Go files
	goFiles, err := CollectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	// Track which frameworks we've seen (preserves detection order)
	seen := make(map[string]bool)
	var frameworks []string

	// Parse files to check for framework imports
	fset := token.NewFileSet()
	for _, filePath := range goFiles {
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			continue // Skip files that can't be parsed
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			var fw string
			switch {
			case strings.Contains(importPath, "gin-gonic/gin"):
				fw = "gin"
			case strings.Contains(importPath, "go-chi/chi"):
				fw = "chi"
			case strings.Contains(importPath, "labstack/echo"):
				fw = "echo"
			case strings.Contains(importPath, "gofiber/fiber"):
				fw = "fiber"
			case strings.Contains(importPath, "gorilla/mux"):
				fw = "mux"
			}
			if fw != "" && !seen[fw] {
				seen[fw] = true
				frameworks = append(frameworks, fw)
			}
		}
	}

	if len(frameworks) == 0 {
		return []string{"net/http"}, nil
	}
	return frameworks, nil
}

// CollectGoFiles recursively collects all .go files from a directory
func CollectGoFiles(dir string) ([]string, error) {
	var goFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	return goFiles, err
}
