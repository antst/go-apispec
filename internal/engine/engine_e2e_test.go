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

package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intspec "github.com/antst/go-apispec/internal/spec"
	"github.com/antst/go-apispec/spec"
)

// frameworkTestCase holds the configuration for testing a single framework.
type frameworkTestCase struct {
	name     string
	inputDir string
	configFn func() *spec.APISpecConfig
}

// allFrameworks returns test cases for every supported framework testdata dir.
// It skips frameworks whose testdata directory does not exist.
func allFrameworks(t *testing.T) []frameworkTestCase {
	t.Helper()
	cases := []frameworkTestCase{
		{name: "chi", inputDir: "../../testdata/chi", configFn: spec.DefaultChiConfig},
		{name: "gin", inputDir: "../../testdata/gin", configFn: spec.DefaultGinConfig},
		{name: "echo", inputDir: "../../testdata/echo", configFn: spec.DefaultEchoConfig},
		{name: "fiber", inputDir: "../../testdata/fiber", configFn: spec.DefaultFiberConfig},
		{name: "mux", inputDir: "../../testdata/mux", configFn: spec.DefaultMuxConfig},
		{name: "response_patterns", inputDir: "../../testdata/response_patterns", configFn: spec.DefaultChiConfig},
	}
	var available []frameworkTestCase
	for _, tc := range cases {
		if _, err := os.Stat(tc.inputDir); err == nil {
			available = append(available, tc)
		}
	}
	require.NotEmpty(t, available, "at least one framework testdata directory must exist")
	return available
}

// newDefaultCfg returns an EngineConfig pre-filled with sensible defaults for e2e tests.
func newDefaultCfg(inputDir string, apiCfg *spec.APISpecConfig) *EngineConfig {
	return &EngineConfig{
		InputDir:           inputDir,
		OutputFile:         "openapi.json",
		Title:              "Test",
		APIVersion:         "1.0.0",
		OpenAPIVersion:     "3.1.1",
		MaxNodesPerTree:    50000,
		MaxChildrenPerNode: 500,
		MaxArgsPerFunction: 100,
		MaxNestedArgsDepth: 100,
		MaxRecursionDepth:  10,
		Verbose:            false,
		APISpecConfig:      apiCfg,
	}
}

// collectOperations returns every non-nil Operation in the spec together with its operationId.
func collectOperations(result *spec.OpenAPISpec) []*operation {
	var ops []*operation
	for path, item := range result.Paths {
		for method, op := range map[string]*intspec.Operation{
			"GET":    item.Get,
			"POST":   item.Post,
			"PUT":    item.Put,
			"DELETE": item.Delete,
			"PATCH":  item.Patch,
		} {
			if op != nil {
				ops = append(ops, &operation{path: path, method: method, op: op})
			}
		}
	}
	return ops
}

type operation struct {
	path   string
	method string
	op     *intspec.Operation
}

// collectAllRefValues recursively collects every $ref string found in response content schemas.
func collectAllRefValues(result *spec.OpenAPISpec) []string {
	var refs []string
	for _, item := range result.Paths {
		for _, op := range []*intspec.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch} {
			if op == nil {
				continue
			}
			for _, resp := range op.Responses {
				for _, media := range resp.Content {
					if media.Schema != nil {
						refs = append(refs, collectSchemaRefs(media.Schema)...)
					}
				}
			}
		}
	}
	return refs
}

func collectSchemaRefs(s *spec.Schema) []string {
	if s == nil {
		return nil
	}
	var refs []string
	if s.Ref != "" {
		refs = append(refs, s.Ref)
	}
	if s.Items != nil {
		refs = append(refs, collectSchemaRefs(s.Items)...)
	}
	for _, prop := range s.Properties {
		refs = append(refs, collectSchemaRefs(prop)...)
	}
	for _, child := range s.AllOf {
		refs = append(refs, collectSchemaRefs(child)...)
	}
	for _, child := range s.OneOf {
		refs = append(refs, collectSchemaRefs(child)...)
	}
	for _, child := range s.AnyOf {
		refs = append(refs, collectSchemaRefs(child)...)
	}
	return refs
}

// ---------------------------------------------------------------------------
// 1. TestE2E_Chi_FullPipeline
// ---------------------------------------------------------------------------

func TestE2E_Chi_FullPipeline(t *testing.T) {
	cfg := newDefaultCfg("../../testdata/chi", spec.DefaultChiConfig())
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	// OpenAPI version
	assert.Equal(t, "3.1.1", result.OpenAPI)

	// Info
	assert.NotEmpty(t, result.Info.Title)
	assert.NotEmpty(t, result.Info.Version)

	// Paths: expect at least some of the chi routes
	require.NotEmpty(t, result.Paths, "expected non-empty Paths")

	foundUsers := false
	foundProducts := false
	foundPayment := false
	for path := range result.Paths {
		if strings.Contains(path, "user") || strings.Contains(path, "User") {
			foundUsers = true
		}
		if strings.Contains(path, "product") || strings.Contains(path, "Product") {
			foundProducts = true
		}
		if strings.Contains(path, "payment") || strings.Contains(path, "Payment") || strings.Contains(path, "stripe") {
			foundPayment = true
		}
	}
	assert.True(t, foundUsers, "expected a path related to users; got paths: %v", pathKeys(result))
	assert.True(t, foundProducts, "expected a path related to products; got paths: %v", pathKeys(result))
	assert.True(t, foundPayment, "expected a path related to payment; got paths: %v", pathKeys(result))

	// Operations have non-empty operationIds
	ops := collectOperations(result)
	require.NotEmpty(t, ops, "expected at least one operation")
	for _, o := range ops {
		assert.NotEmpty(t, o.op.OperationID, "operationId should be non-empty for %s %s", o.method, o.path)
	}

	// Components.Schemas is non-empty
	require.NotNil(t, result.Components, "expected non-nil Components")
	assert.NotEmpty(t, result.Components.Schemas, "expected non-empty schemas")

	// Response status codes are present on at least some operations
	foundResponseCodes := false
	for _, o := range ops {
		if len(o.op.Responses) > 0 {
			foundResponseCodes = true
			for code := range o.op.Responses {
				assert.NotEmpty(t, code, "response status code should not be empty")
			}
		}
	}
	assert.True(t, foundResponseCodes, "expected at least one operation with response status codes")
}

// ---------------------------------------------------------------------------
// 2. TestE2E_Chi_Determinism
// ---------------------------------------------------------------------------

func TestE2E_Chi_Determinism(t *testing.T) {
	cfg1 := newDefaultCfg("../../testdata/chi", spec.DefaultChiConfig())
	eng1 := NewEngine(cfg1)
	result1, err := eng1.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result1)

	cfg2 := newDefaultCfg("../../testdata/chi", spec.DefaultChiConfig())
	eng2 := NewEngine(cfg2)
	result2, err := eng2.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result2)

	json1, err := json.Marshal(result1)
	require.NoError(t, err)
	json2, err := json.Marshal(result2)
	require.NoError(t, err)

	assert.Equal(t, string(json1), string(json2), "two consecutive generations should produce identical JSON output")
}

// ---------------------------------------------------------------------------
// 3. TestE2E_Chi_ShortNames
// ---------------------------------------------------------------------------

func TestE2E_Chi_ShortNames(t *testing.T) {
	chiCfg := spec.DefaultChiConfig()
	// Default config has ShortNames nil (= true)
	cfg := newDefaultCfg("../../testdata/chi", chiCfg)
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	// All operationIds should have no "/" character
	ops := collectOperations(result)
	require.NotEmpty(t, ops)
	for _, o := range ops {
		assert.NotContains(t, o.op.OperationID, "/",
			"operationId %q should not contain '/' with short names", o.op.OperationID)
	}

	// All schema keys should have no "/" character
	if result.Components != nil {
		for key := range result.Components.Schemas {
			assert.NotContains(t, key, "/",
				"schema key %q should not contain '/' with short names", key)
		}
	}

	// All $ref values should not contain full module path
	refs := collectAllRefValues(result)
	for _, ref := range refs {
		assert.NotContains(t, ref, "github.com/antst/go-apispec/testdata/chi",
			"$ref %q should not contain full module path with short names", ref)
	}
}

// ---------------------------------------------------------------------------
// 4. TestE2E_Chi_LegacyNames
// ---------------------------------------------------------------------------

func TestE2E_Chi_LegacyNames(t *testing.T) {
	chiCfg := spec.DefaultChiConfig()
	f := false
	chiCfg.ShortNames = &f

	cfg := newDefaultCfg("../../testdata/chi", chiCfg)
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	// operationIds should contain the full module path
	ops := collectOperations(result)
	require.NotEmpty(t, ops)
	foundLegacyOpID := false
	for _, o := range ops {
		if strings.Contains(o.op.OperationID, "github.com") || strings.Contains(o.op.OperationID, "ehabterra") {
			foundLegacyOpID = true
			break
		}
	}
	assert.True(t, foundLegacyOpID,
		"with ShortNames=false, at least one operationId should contain the full module path")

	// Schema keys should contain underscored module path
	if result.Components != nil && len(result.Components.Schemas) > 0 {
		foundLegacySchema := false
		for key := range result.Components.Schemas {
			if strings.Contains(key, "ehabterra") || strings.Contains(key, "apispec") {
				foundLegacySchema = true
				break
			}
		}
		assert.True(t, foundLegacySchema,
			"with ShortNames=false, at least one schema key should contain module path components")
	}
}

// ---------------------------------------------------------------------------
// 5. TestE2E_Gin_FullPipeline
// ---------------------------------------------------------------------------

func TestE2E_Gin_FullPipeline(t *testing.T) {
	cfg := newDefaultCfg("../../testdata/gin", spec.DefaultGinConfig())
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "3.1.1", result.OpenAPI)
	require.NotEmpty(t, result.Paths, "expected non-empty Paths for gin")

	// Expect paths related to users
	foundUsers := false
	for path := range result.Paths {
		if strings.Contains(strings.ToLower(path), "user") {
			foundUsers = true
			break
		}
	}
	assert.True(t, foundUsers, "expected a path related to users; got paths: %v", pathKeys(result))

	// Operations should have operationIds
	ops := collectOperations(result)
	require.NotEmpty(t, ops, "expected at least one operation for gin")
	for _, o := range ops {
		assert.NotEmpty(t, o.op.OperationID, "operationId should be non-empty for %s %s", o.method, o.path)
	}

	// User schema should exist
	if result.Components != nil && len(result.Components.Schemas) > 0 {
		foundUserSchema := false
		for key := range result.Components.Schemas {
			if strings.Contains(strings.ToLower(key), "user") {
				foundUserSchema = true
				break
			}
		}
		assert.True(t, foundUserSchema, "expected User schema in components; got schemas: %v", schemaKeys(result))
	}
}

// ---------------------------------------------------------------------------
// 6. TestE2E_Echo_FullPipeline
// ---------------------------------------------------------------------------

func TestE2E_Echo_FullPipeline(t *testing.T) {
	cfg := newDefaultCfg("../../testdata/echo", spec.DefaultEchoConfig())
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "3.1.1", result.OpenAPI)
	require.NotEmpty(t, result.Paths, "expected non-empty Paths for echo")

	// Expect various paths
	foundUserPath := false
	foundHealthPath := false
	foundInfoPath := false
	for path := range result.Paths {
		lower := strings.ToLower(path)
		if strings.Contains(lower, "user") {
			foundUserPath = true
		}
		if strings.Contains(lower, "health") {
			foundHealthPath = true
		}
		if strings.Contains(lower, "info") {
			foundInfoPath = true
		}
	}
	assert.True(t, foundUserPath, "expected a user-related path; got paths: %v", pathKeys(result))
	assert.True(t, foundHealthPath, "expected a health-related path; got paths: %v", pathKeys(result))
	assert.True(t, foundInfoPath, "expected an info-related path; got paths: %v", pathKeys(result))

	// Check for multiple response types (User, ErrorResponse, SuccessResponse) in schemas
	if result.Components != nil && len(result.Components.Schemas) > 0 {
		foundUser := false
		foundError := false
		foundSuccess := false
		for key := range result.Components.Schemas {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "user") && !strings.Contains(lower, "request") {
				foundUser = true
			}
			if strings.Contains(lower, "error") {
				foundError = true
			}
			if strings.Contains(lower, "success") {
				foundSuccess = true
			}
		}
		assert.True(t, foundUser, "expected User schema in echo components; got schemas: %v", schemaKeys(result))
		assert.True(t, foundError, "expected ErrorResponse schema in echo components; got schemas: %v", schemaKeys(result))
		assert.True(t, foundSuccess, "expected SuccessResponse schema in echo components; got schemas: %v", schemaKeys(result))
	}
}

// ---------------------------------------------------------------------------
// 7. TestE2E_Fiber_FullPipeline
// ---------------------------------------------------------------------------

func TestE2E_Fiber_FullPipeline(t *testing.T) {
	cfg := newDefaultCfg("../../testdata/fiber", spec.DefaultFiberConfig())
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "3.1.1", result.OpenAPI)
	require.NotEmpty(t, result.Paths, "expected non-empty Paths for fiber")

	// Expect paths related to various endpoints
	foundUsers := false
	foundProducts := false
	foundPayment := false
	foundHealth := false
	for path := range result.Paths {
		lower := strings.ToLower(path)
		if strings.Contains(lower, "user") {
			foundUsers = true
		}
		if strings.Contains(lower, "product") {
			foundProducts = true
		}
		if strings.Contains(lower, "payment") || strings.Contains(lower, "stripe") {
			foundPayment = true
		}
		if strings.Contains(lower, "health") {
			foundHealth = true
		}
	}
	assert.True(t, foundUsers, "expected a user-related path; got paths: %v", pathKeys(result))
	assert.True(t, foundProducts, "expected a product-related path; got paths: %v", pathKeys(result))
	assert.True(t, foundPayment, "expected a payment-related path; got paths: %v", pathKeys(result))
	assert.True(t, foundHealth, "expected a health-related path; got paths: %v", pathKeys(result))

	// User and Product schemas should exist
	if result.Components != nil && len(result.Components.Schemas) > 0 {
		foundUserSchema := false
		foundProductSchema := false
		for key := range result.Components.Schemas {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "user") && !strings.Contains(lower, "request") {
				foundUserSchema = true
			}
			if strings.Contains(lower, "product") && !strings.Contains(lower, "request") {
				foundProductSchema = true
			}
		}
		assert.True(t, foundUserSchema, "expected User schema in fiber components; got schemas: %v", schemaKeys(result))
		assert.True(t, foundProductSchema, "expected Product schema in fiber components; got schemas: %v", schemaKeys(result))
	}
}

// ---------------------------------------------------------------------------
// 8. TestE2E_Mux_FullPipeline
// ---------------------------------------------------------------------------

func TestE2E_Mux_FullPipeline(t *testing.T) {
	cfg := newDefaultCfg("../../testdata/mux", spec.DefaultMuxConfig())
	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "3.1.1", result.OpenAPI)

	// At least some paths should be generated
	require.NotEmpty(t, result.Paths, "expected non-empty Paths for mux")

	// Operations should have operationIds
	ops := collectOperations(result)
	require.NotEmpty(t, ops, "expected at least one operation for mux")
	for _, o := range ops {
		assert.NotEmpty(t, o.op.OperationID, "operationId should be non-empty for %s %s", o.method, o.path)
	}
}

// ---------------------------------------------------------------------------
// 9. TestE2E_AllFrameworks_Determinism
// ---------------------------------------------------------------------------

func TestE2E_AllFrameworks_Determinism(t *testing.T) {
	for _, tc := range allFrameworks(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg1 := newDefaultCfg(tc.inputDir, tc.configFn())
			eng1 := NewEngine(cfg1)
			result1, err := eng1.GenerateOpenAPI()
			require.NoError(t, err)
			require.NotNil(t, result1)

			cfg2 := newDefaultCfg(tc.inputDir, tc.configFn())
			eng2 := NewEngine(cfg2)
			result2, err := eng2.GenerateOpenAPI()
			require.NoError(t, err)
			require.NotNil(t, result2)

			json1, err := json.Marshal(result1)
			require.NoError(t, err)
			json2, err := json.Marshal(result2)
			require.NoError(t, err)

			assert.Equal(t, string(json1), string(json2),
				"framework %s: two consecutive generations should produce byte-identical JSON", tc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// 10. TestE2E_AllFrameworks_NoSlashInOperationIDs
// ---------------------------------------------------------------------------

func TestE2E_AllFrameworks_NoSlashInOperationIDs(t *testing.T) {
	for _, tc := range allFrameworks(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := newDefaultCfg(tc.inputDir, tc.configFn())
			eng := NewEngine(cfg)
			result, err := eng.GenerateOpenAPI()
			require.NoError(t, err)
			require.NotNil(t, result)

			ops := collectOperations(result)
			for _, o := range ops {
				assert.NotContains(t, o.op.OperationID, "/",
					"framework %s: operationId %q at %s %s should not contain '/'",
					tc.name, o.op.OperationID, o.method, o.path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 11. TestE2E_AllFrameworks_NoSlashInSchemaNames
// ---------------------------------------------------------------------------

func TestE2E_AllFrameworks_NoSlashInSchemaNames(t *testing.T) {
	for _, tc := range allFrameworks(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := newDefaultCfg(tc.inputDir, tc.configFn())
			eng := NewEngine(cfg)
			result, err := eng.GenerateOpenAPI()
			require.NoError(t, err)
			require.NotNil(t, result)

			if result.Components == nil {
				return
			}
			for key := range result.Components.Schemas {
				assert.NotContains(t, key, "/",
					"framework %s: schema key %q should not contain '/'", tc.name, key)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func pathKeys(result *spec.OpenAPISpec) []string {
	keys := make([]string, 0, len(result.Paths))
	for k := range result.Paths {
		keys = append(keys, k)
	}
	return keys
}

func schemaKeys(result *spec.OpenAPISpec) []string {
	if result.Components == nil {
		return nil
	}
	keys := make([]string, 0, len(result.Components.Schemas))
	for k := range result.Components.Schemas {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Golden-file regression tests
// ---------------------------------------------------------------------------
// ONE code path for both generation and comparison. The function
// generateGoldenSpec() is the single source of truth.
//
// TestGolden_AllFrameworks compares generateGoldenSpec() output against
// committed golden files. It NEVER overwrites them.
//
// TestUpdateGolden_AllFrameworks writes generateGoldenSpec() output TO
// the golden files. Run it explicitly to update after intentional changes:
//
//	go test ./internal/engine/ -run TestUpdateGolden -v
//
// The update test is skipped by default (requires -run to select it).

// generateGoldenSpec is the SINGLE code path for producing golden file content.
// Both the comparison test and the update test call this function.
func generateGoldenSpec(t *testing.T, tc frameworkTestCase, legacy bool) []byte {
	t.Helper()
	apiCfg := tc.configFn()
	if legacy {
		f := false
		apiCfg.ShortNames = &f
	}
	cfg := DefaultEngineConfig()
	cfg.InputDir = tc.inputDir
	cfg.APISpecConfig = apiCfg

	eng := NewEngine(cfg)
	result, err := eng.GenerateOpenAPI()
	require.NoError(t, err, "generation failed for %s (legacy=%v)", tc.name, legacy)
	require.NotNil(t, result)

	got, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	return got
}

// goldenFilePath returns the path to the golden file for a framework.
func goldenFilePath(tc frameworkTestCase, legacy bool) string {
	name := "expected_openapi.json"
	if legacy {
		name = "expected_openapi_legacy.json"
	}
	return filepath.Join(tc.inputDir, name)
}

// compareGolden compares generated output against the committed golden file.
// On mismatch, saves generated output to temp and fails with a diff.
func compareGolden(t *testing.T, tc frameworkTestCase, legacy bool, got []byte) {
	t.Helper()
	goldenPath := goldenFilePath(tc, legacy)
	label := "short"
	if legacy {
		label = "legacy"
	}

	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file missing: %s\nRun: go test ./internal/engine/ -run TestUpdateGolden -v", goldenPath)
	}
	require.NoError(t, err)

	if string(got) != string(expected) {
		tmpPath := filepath.Join(os.TempDir(), "golden_"+tc.name+"_"+label+"_got.json")
		_ = os.WriteFile(tmpPath, got, 0644) //nolint:gosec

		gotLines := strings.Split(string(got), "\n")
		expLines := strings.Split(string(expected), "\n")
		for i := 0; i < len(gotLines) && i < len(expLines); i++ {
			if gotLines[i] != expLines[i] {
				t.Errorf("%s golden mismatch for %s at line %d:\n  expected: %s\n  got:      %s\n\nGenerated: %s\nTo review: diff %s %s\nTo update: go test ./internal/engine/ -run TestUpdateGolden -v",
					label, tc.name, i+1, expLines[i], gotLines[i], tmpPath, goldenPath, tmpPath)
				return
			}
		}
		if len(gotLines) != len(expLines) {
			t.Errorf("%s golden mismatch for %s: expected %d lines, got %d lines\nGenerated: %s",
				label, tc.name, len(expLines), len(gotLines), tmpPath)
		}
	}
}

func TestGolden_AllFrameworks(t *testing.T) {
	for _, tc := range allFrameworks(t) {
		t.Run(tc.name, func(t *testing.T) {
			got := generateGoldenSpec(t, tc, false)
			compareGolden(t, tc, false, got)
		})
	}
}

func TestGolden_AllFrameworks_Legacy(t *testing.T) {
	for _, tc := range allFrameworks(t) {
		t.Run(tc.name, func(t *testing.T) {
			got := generateGoldenSpec(t, tc, true)
			compareGolden(t, tc, true, got)
		})
	}
}

// TestUpdateGolden_AllFrameworks regenerates ALL golden files using the
// same generateGoldenSpec() function. Run explicitly:
//
//	go test ./internal/engine/ -run TestUpdateGolden -v
func TestUpdateGolden_AllFrameworks(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "TestUpdateGolden") {
		t.Skip("run explicitly: go test ./internal/engine/ -run TestUpdateGolden -v")
	}
	for _, tc := range allFrameworks(t) {
		t.Run(tc.name, func(t *testing.T) {
			for _, legacy := range []bool{false, true} {
				got := generateGoldenSpec(t, tc, legacy)
				path := goldenFilePath(tc, legacy)

				existing, _ := os.ReadFile(path)
				if string(got) == string(existing) {
					t.Logf("unchanged: %s", path)
					continue
				}

				err := os.WriteFile(path, got, 0644) //nolint:gosec
				require.NoError(t, err)
				t.Logf("UPDATED: %s", path)
			}
		})
	}
}
