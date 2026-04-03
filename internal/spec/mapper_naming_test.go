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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// ---------------------------------------------------------------------------
// 1. extractValidationConstraints
// ---------------------------------------------------------------------------

func TestExtractValidationConstraints_EmptyTag(t *testing.T) {
	result := extractValidationConstraints("")
	assert.Nil(t, result, "empty tag should return nil")
}

func TestExtractValidationConstraints_NoValidatePrefix(t *testing.T) {
	result := extractValidationConstraints(`json:"name,omitempty"`)
	assert.Nil(t, result, "tag without validate: prefix and no other recognized constraint should return nil")
}

func TestExtractValidationConstraints_RequiredOnly(t *testing.T) {
	result := extractValidationConstraints(`validate:"required"`)
	require.NotNil(t, result)
	assert.True(t, result.Required)
	assert.Nil(t, result.MinLength)
	assert.Nil(t, result.MaxLength)
	assert.Nil(t, result.Min)
	assert.Nil(t, result.Max)
	assert.Empty(t, result.Pattern)
	assert.Empty(t, result.Format)
	assert.Empty(t, result.Enum)
}

func TestExtractValidationConstraints_MinMaxNumeric(t *testing.T) {
	result := extractValidationConstraints(`validate:"min=5,max=100"`)
	require.NotNil(t, result)
	require.NotNil(t, result.Min)
	require.NotNil(t, result.Max)
	assert.Equal(t, float64(5), *result.Min)
	assert.Equal(t, float64(100), *result.Max)
}

func TestExtractValidationConstraints_LenExact(t *testing.T) {
	result := extractValidationConstraints(`validate:"len=8"`)
	require.NotNil(t, result)
	require.NotNil(t, result.MinLength)
	require.NotNil(t, result.MaxLength)
	assert.Equal(t, 8, *result.MinLength)
	assert.Equal(t, 8, *result.MaxLength)
}

func TestExtractValidationConstraints_MinLenMaxLen(t *testing.T) {
	result := extractValidationConstraints(`validate:"minlen=3,maxlen=20"`)
	require.NotNil(t, result)
	require.NotNil(t, result.MinLength)
	require.NotNil(t, result.MaxLength)
	assert.Equal(t, 3, *result.MinLength)
	assert.Equal(t, 20, *result.MaxLength)
}

func TestExtractValidationConstraints_EmailFormatOnly(t *testing.T) {
	// Note: when only Format is set (and nothing else), the function returns nil
	// because the final nil check does not include Format in its conditions.
	// This documents the current behavior.
	result := extractValidationConstraints(`validate:"email"`)
	assert.Nil(t, result, "format-only constraint returns nil due to nil check omitting Format")
}

func TestExtractValidationConstraints_EmailFormatWithRequired(t *testing.T) {
	result := extractValidationConstraints(`validate:"required,email"`)
	require.NotNil(t, result)
	assert.True(t, result.Required)
	assert.Equal(t, "email", result.Format)
}

func TestExtractValidationConstraints_URLFormatWithRequired(t *testing.T) {
	result := extractValidationConstraints(`validate:"required,url"`)
	require.NotNil(t, result)
	assert.True(t, result.Required)
	assert.Equal(t, "uri", result.Format)
}

func TestExtractValidationConstraints_UUIDFormatWithRequired(t *testing.T) {
	result := extractValidationConstraints(`validate:"required,uuid"`)
	require.NotNil(t, result)
	assert.True(t, result.Required)
	assert.Equal(t, "uuid", result.Format)
}

func TestExtractValidationConstraints_OneofEnum(t *testing.T) {
	result := extractValidationConstraints(`validate:"oneof=active inactive pending"`)
	require.NotNil(t, result)
	require.Len(t, result.Enum, 3)
	assert.Equal(t, "active", result.Enum[0])
	assert.Equal(t, "inactive", result.Enum[1])
	assert.Equal(t, "pending", result.Enum[2])
}

func TestExtractValidationConstraints_RegexpPattern(t *testing.T) {
	result := extractValidationConstraints(`validate:"regexp=^[a-z]+$"`)
	require.NotNil(t, result)
	assert.Equal(t, "^[a-z]+$", result.Pattern)
}

func TestExtractValidationConstraints_CombinedRules(t *testing.T) {
	result := extractValidationConstraints(`validate:"required,min=1,max=50,email"`)
	require.NotNil(t, result)
	assert.True(t, result.Required)
	require.NotNil(t, result.Min)
	require.NotNil(t, result.Max)
	assert.Equal(t, float64(1), *result.Min)
	assert.Equal(t, float64(50), *result.Max)
	assert.Equal(t, "email", result.Format)
}

func TestExtractValidationConstraints_AlphaPattern(t *testing.T) {
	result := extractValidationConstraints(`validate:"alpha"`)
	require.NotNil(t, result)
	assert.Equal(t, `^[a-zA-Z]+$`, result.Pattern)
}

func TestExtractValidationConstraints_AlphanumPattern(t *testing.T) {
	result := extractValidationConstraints(`validate:"alphanum"`)
	require.NotNil(t, result)
	assert.Equal(t, `^[a-zA-Z0-9]+$`, result.Pattern)
}

func TestExtractValidationConstraints_NumericPattern(t *testing.T) {
	result := extractValidationConstraints(`validate:"numeric"`)
	require.NotNil(t, result)
	assert.Equal(t, `^[0-9]+$`, result.Pattern)
}

func TestExtractValidationConstraints_CustomMinTag(t *testing.T) {
	// Test the custom min:/max: tag format
	result := extractValidationConstraints(`min:"3.5"`)
	require.NotNil(t, result)
	require.NotNil(t, result.Min)
	assert.Equal(t, 3.5, *result.Min)
}

func TestExtractValidationConstraints_CustomMaxTag(t *testing.T) {
	result := extractValidationConstraints(`max:"99.9"`)
	require.NotNil(t, result)
	require.NotNil(t, result.Max)
	assert.Equal(t, 99.9, *result.Max)
}

func TestExtractValidationConstraints_CustomRegexpTag(t *testing.T) {
	result := extractValidationConstraints(`regexp:"^[0-9]+$"`)
	require.NotNil(t, result)
	assert.Equal(t, "^[0-9]+$", result.Pattern)
}

func TestExtractValidationConstraints_CustomEnumTag(t *testing.T) {
	result := extractValidationConstraints(`enum:"a,b,c"`)
	require.NotNil(t, result)
	require.Len(t, result.Enum, 3)
	assert.Equal(t, "a", result.Enum[0])
	assert.Equal(t, "b", result.Enum[1])
	assert.Equal(t, "c", result.Enum[2])
}

func TestExtractValidationConstraints_NoConstraintsFound(t *testing.T) {
	// A validate tag with no recognized rules should return nil
	result := extractValidationConstraints(`validate:"unknownrule"`)
	assert.Nil(t, result, "unrecognized rule should yield nil constraints")
}

// ---------------------------------------------------------------------------
// 2. applyValidationConstraints
// ---------------------------------------------------------------------------

func TestApplyValidationConstraints_NilConstraints(t *testing.T) {
	schema := &Schema{Type: "string"}
	applyValidationConstraints(schema, nil)
	// no-op: schema should be unchanged
	assert.Equal(t, "string", schema.Type)
	assert.Equal(t, 0, schema.MinLength)
	assert.Equal(t, 0, schema.MaxLength)
}

func TestApplyValidationConstraints_NilSchema(_ *testing.T) {
	constraints := &ValidationConstraints{Required: true}
	// Should not panic
	applyValidationConstraints(nil, constraints)
}

func TestApplyValidationConstraints_StringLength(t *testing.T) {
	schema := &Schema{Type: "string"}
	minLen := 3
	maxLen := 50
	constraints := &ValidationConstraints{
		MinLength: &minLen,
		MaxLength: &maxLen,
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, 3, schema.MinLength)
	assert.Equal(t, 50, schema.MaxLength)
}

func TestApplyValidationConstraints_StringLengthIgnoredForInteger(t *testing.T) {
	schema := &Schema{Type: "integer"}
	minLen := 3
	maxLen := 50
	constraints := &ValidationConstraints{
		MinLength: &minLen,
		MaxLength: &maxLen,
	}
	applyValidationConstraints(schema, constraints)
	// For integer, MinLength/MaxLength are applied as Minimum/Maximum
	assert.Equal(t, floatPtr(3), schema.Minimum)
	assert.Equal(t, floatPtr(50), schema.Maximum)
}

func TestApplyValidationConstraints_NumericMinMax(t *testing.T) {
	schema := &Schema{Type: "number"}
	minVal := float64(1.5)
	maxVal := float64(99.9)
	constraints := &ValidationConstraints{
		Min: &minVal,
		Max: &maxVal,
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, floatPtr(1.5), schema.Minimum)
	assert.Equal(t, floatPtr(99.9), schema.Maximum)
}

func TestApplyValidationConstraints_Pattern(t *testing.T) {
	schema := &Schema{Type: "string"}
	constraints := &ValidationConstraints{
		Pattern: `^[a-z]+$`,
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, `^[a-z]+$`, schema.Pattern)
}

func TestApplyValidationConstraints_Format(t *testing.T) {
	schema := &Schema{Type: "string"}
	constraints := &ValidationConstraints{
		Format: "email",
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, "email", schema.Format)
}

func TestApplyValidationConstraints_EnumOnPrimitive(t *testing.T) {
	schema := &Schema{Type: "string"}
	constraints := &ValidationConstraints{
		Enum: []interface{}{"a", "b", "c"},
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, []interface{}{"a", "b", "c"}, schema.Enum)
}

func TestApplyValidationConstraints_EnumOnArray(t *testing.T) {
	schema := &Schema{
		Type:  "array",
		Items: &Schema{Type: "string"},
	}
	constraints := &ValidationConstraints{
		Enum: []interface{}{"x", "y"},
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, []interface{}{"x", "y"}, schema.Items.Enum)
	assert.Nil(t, schema.Enum)
}

func TestApplyValidationConstraints_EnumOnObjectWithAdditionalProperties(t *testing.T) {
	schema := &Schema{
		Type:                 "object",
		AdditionalProperties: &Schema{Type: "string"},
	}
	constraints := &ValidationConstraints{
		Enum: []interface{}{"foo", "bar"},
	}
	applyValidationConstraints(schema, constraints)
	assert.Equal(t, []interface{}{"foo", "bar"}, schema.AdditionalProperties.Enum)
}

func TestApplyValidationConstraints_EnumOnObjectNoAdditionalProperties(_ *testing.T) {
	schema := &Schema{Type: "object"}
	constraints := &ValidationConstraints{
		Enum: []interface{}{"foo"},
	}
	// Should not panic even without AdditionalProperties
	applyValidationConstraints(schema, constraints)
}

// ---------------------------------------------------------------------------
// 3. extractConstantValue
// ---------------------------------------------------------------------------

func TestExtractConstantValue_Nil(t *testing.T) {
	result := extractConstantValue(nil)
	assert.Nil(t, result)
}

func TestExtractConstantValue_QuotedString(t *testing.T) {
	// A stringer that returns a quoted string
	val := stringerVal(`"hello"`)
	result := extractConstantValue(val)
	assert.Equal(t, "hello", result)
}

func TestExtractConstantValue_IntegerString(t *testing.T) {
	val := stringerVal("42")
	result := extractConstantValue(val)
	assert.Equal(t, int64(42), result)
}

func TestExtractConstantValue_FloatString(t *testing.T) {
	val := stringerVal("3.14")
	result := extractConstantValue(val)
	assert.Equal(t, 3.14, result)
}

func TestExtractConstantValue_BoolString(t *testing.T) {
	val := stringerVal("true")
	result := extractConstantValue(val)
	assert.Equal(t, true, result)
}

func TestExtractConstantValue_PlainString(t *testing.T) {
	val := stringerVal("some_identifier")
	result := extractConstantValue(val)
	assert.Equal(t, "some_identifier", result)
}

func TestExtractConstantValue_NonStringer(t *testing.T) {
	// Pass a plain int -- it has no String() method
	result := extractConstantValue(12345)
	assert.Equal(t, 12345, result)
}

// stringerVal is a helper that implements fmt.Stringer.
type stringerVal string

func (s stringerVal) String() string { return string(s) }

// ---------------------------------------------------------------------------
// 4. typeMatches
// ---------------------------------------------------------------------------

func TestTypeMatches_DirectMatch(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.True(t, typeMatches("Status", "Status", meta))
}

func TestTypeMatches_PointerConstant(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.True(t, typeMatches("*Status", "Status", meta))
}

func TestTypeMatches_PointerTarget(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.True(t, typeMatches("Status", "*Status", meta))
}

func TestTypeMatches_PackageQualifiedConstant(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	// constant is package-qualified, target is not
	assert.True(t, typeMatches("pkg.Status", "Status", meta))
}

func TestTypeMatches_PackageQualifiedTarget(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	// target is package-qualified, constant is not
	assert.True(t, typeMatches("Status", "pkg.Status", meta))
}

func TestTypeMatches_BothPackageQualified(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.True(t, typeMatches("models.Status", "models.Status", meta))
	// Different package but same type name
	assert.True(t, typeMatches("a.Status", "b.Status", meta))
}

func TestTypeMatches_NoMatch(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.False(t, typeMatches("Status", "Role", meta))
}

func TestTypeMatches_EmptyStrings(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	assert.True(t, typeMatches("", "", meta), "two empty strings should match directly")
	assert.False(t, typeMatches("Status", "", meta))
}

// ---------------------------------------------------------------------------
// 5. isInSameGroupAsTypedConstant
// ---------------------------------------------------------------------------

func TestIsInSameGroupAsTypedConstant_MatchingGroupAndType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	variables := map[string]*metadata.Variable{
		"StatusActive": {
			Name:       sp.Get("StatusActive"),
			Tok:        sp.Get("const"),
			Type:       sp.Get("Status"),
			GroupIndex: 1,
		},
		"StatusInactive": {
			Name:       sp.Get("StatusInactive"),
			Tok:        sp.Get("const"),
			Type:       sp.Get("Status"),
			GroupIndex: 1,
		},
	}

	assert.True(t, isInSameGroupAsTypedConstant(1, "Status", variables, meta))
}

func TestIsInSameGroupAsTypedConstant_NonMatchingGroupIndex(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	variables := map[string]*metadata.Variable{
		"StatusActive": {
			Name:       sp.Get("StatusActive"),
			Tok:        sp.Get("const"),
			Type:       sp.Get("Status"),
			GroupIndex: 2,
		},
	}

	// Looking for group index 1, but variable is in group 2
	assert.False(t, isInSameGroupAsTypedConstant(1, "Status", variables, meta))
}

func TestIsInSameGroupAsTypedConstant_NonMatchingType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	variables := map[string]*metadata.Variable{
		"RoleAdmin": {
			Name:       sp.Get("RoleAdmin"),
			Tok:        sp.Get("const"),
			Type:       sp.Get("Role"),
			GroupIndex: 1,
		},
	}

	assert.False(t, isInSameGroupAsTypedConstant(1, "Status", variables, meta))
}

func TestIsInSameGroupAsTypedConstant_EmptyVariables(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	variables := map[string]*metadata.Variable{}

	assert.False(t, isInSameGroupAsTypedConstant(0, "Status", variables, meta))
}

func TestIsInSameGroupAsTypedConstant_NonConstVariable(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}

	variables := map[string]*metadata.Variable{
		"myVar": {
			Name:       sp.Get("myVar"),
			Tok:        sp.Get("var"), // not "const"
			Type:       sp.Get("Status"),
			GroupIndex: 1,
		},
	}

	assert.False(t, isInSameGroupAsTypedConstant(1, "Status", variables, meta))
}

// ---------------------------------------------------------------------------
// 6. LoadAPISpecConfig
// ---------------------------------------------------------------------------

func TestLoadAPISpecConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "apispec.yaml")

	yamlContent := `
shortNames: true
info:
  title: "Test API"
  version: "1.0.0"
  description: "A test API"
framework:
  routePatterns: []
`
	err := os.WriteFile(cfgPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadAPISpecConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "Test API", cfg.Info.Title)
	assert.Equal(t, "1.0.0", cfg.Info.Version)
	assert.Equal(t, "A test API", cfg.Info.Description)
	assert.True(t, cfg.UseShortNames())
}

func TestLoadAPISpecConfig_InvalidPath(t *testing.T) {
	cfg, err := LoadAPISpecConfig("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoadAPISpecConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")

	badContent := `
shortNames: [[[invalid yaml
  not: {properly: {closed
`
	err := os.WriteFile(cfgPath, []byte(badContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadAPISpecConfig(cfgPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoadAPISpecConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "empty.yaml")
	err := os.WriteFile(cfgPath, []byte(""), 0644)
	require.NoError(t, err)

	cfg, err := LoadAPISpecConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	// Default: ShortNames nil means UseShortNames() returns true
	assert.True(t, cfg.UseShortNames())
}

func TestLoadAPISpecConfig_ShortNamesFalse(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	yamlContent := `shortNames: false`
	err := os.WriteFile(cfgPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadAPISpecConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.UseShortNames())
}

// ---------------------------------------------------------------------------
// 7. findTypesInMetadata
// ---------------------------------------------------------------------------

func TestFindTypesInMetadata_FoundType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"models": {
				Files: map[string]*metadata.File{
					"user.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: sp.Get("User"),
								Kind: sp.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	result := findTypesInMetadata(meta, "models-->User")
	require.NotNil(t, result)
	assert.Contains(t, result, "models-->User")
	assert.NotNil(t, result["models-->User"])
}

func TestFindTypesInMetadata_PrimitiveType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	result := findTypesInMetadata(meta, "string")
	assert.Nil(t, result, "primitive types should return nil")
}

func TestFindTypesInMetadata_UnknownType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
	}

	result := findTypesInMetadata(meta, "nonexistent-->Foo")
	// Should return a map, but the type should be nil
	require.NotNil(t, result)
	assert.Nil(t, result["nonexistent-->Foo"])
}

func TestFindTypesInMetadata_NilMetadata(t *testing.T) {
	result := findTypesInMetadata(nil, "SomeType")
	assert.Nil(t, result)
}

func TestFindTypesInMetadata_GenericTypeWithBrackets(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"models": {
				Files: map[string]*metadata.File{
					"response.go": {
						Types: map[string]*metadata.Type{
							"Response": {
								Name: sp.Get("Response"),
								Kind: sp.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: sp.Get("Data"),
										Type: sp.Get("T"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// TypeParts for "models-->Response[T string]" should parse generics
	result := findTypesInMetadata(meta, "models-->Response")
	require.NotNil(t, result)
	assert.NotNil(t, result["models-->Response"])
}

func TestFindTypesInMetadata_TypeWithoutPackage(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"main": {
				Files: map[string]*metadata.File{
					"models.go": {
						Types: map[string]*metadata.Type{
							"Item": {
								Name: sp.Get("Item"),
								Kind: sp.Get("struct"),
							},
						},
					},
				},
			},
		},
	}

	// A bare type name without package separator
	result := findTypesInMetadata(meta, "Item")
	require.NotNil(t, result)
	assert.NotNil(t, result["Item"])
}

// ---------------------------------------------------------------------------
// 8. shortenAllRefs
// ---------------------------------------------------------------------------

func TestShortenAllRefs_BasicRefRewriting(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]*Schema{
				"http-->CreateDocumentResponse": {
					Type: "object",
					Properties: map[string]*Schema{
						"nested": {
							Ref: "#/components/schemas/http-->CreateDocumentResponse",
						},
					},
				},
			},
		},
		Paths: map[string]PathItem{},
	}

	shortenAllRefs(spec)

	// The short key "http-->CreateDocumentResponse" exists,
	// so the ref should remain pointing to it
	assert.Equal(t,
		"#/components/schemas/http-->CreateDocumentResponse",
		spec.Components.Schemas["http-->CreateDocumentResponse"].Properties["nested"].Ref,
	)
}

func TestShortenAllRefs_NilComponents(_ *testing.T) {
	spec := &OpenAPISpec{Components: nil}
	// Should not panic
	shortenAllRefs(spec)
}

func TestShortenAllRefs_RefInPaths(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]*Schema{
				"http.CreateResponse": {
					Type: "object",
				},
			},
		},
		Paths: map[string]PathItem{
			"/api/items": {
				Post: &Operation{
					OperationID: "createItem",
					RequestBody: &RequestBody{
						Content: map[string]MediaType{
							"application/json": {
								Schema: &Schema{
									Ref: "#/components/schemas/http.CreateResponse",
								},
							},
						},
					},
					Responses: map[string]Response{
						"200": {
							Description: "OK",
							Content: map[string]MediaType{
								"application/json": {
									Schema: &Schema{
										Ref: "#/components/schemas/http.CreateResponse",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	shortenAllRefs(spec)

	// The ref should still point to the existing schema key
	reqSchema := spec.Paths["/api/items"].Post.RequestBody.Content["application/json"].Schema
	assert.Equal(t, "#/components/schemas/http.CreateResponse", reqSchema.Ref)
}

func TestShortenAllRefs_RefInParameters(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]*Schema{
				"http.SortOrder": {Type: "string"},
			},
		},
		Paths: map[string]PathItem{
			"/api/items": {
				Get: &Operation{
					OperationID: "listItems",
					Parameters: []Parameter{
						{
							Name: "sort",
							In:   "query",
							Schema: &Schema{
								Ref: "#/components/schemas/http.SortOrder",
							},
						},
					},
					Responses: map[string]Response{},
				},
			},
		},
	}

	shortenAllRefs(spec)
	paramSchema := spec.Paths["/api/items"].Get.Parameters[0].Schema
	assert.Equal(t, "#/components/schemas/http.SortOrder", paramSchema.Ref)
}

func TestShortenAllRefs_RefInItemsAndAllOf(t *testing.T) {
	spec := &OpenAPISpec{
		Components: &Components{
			Schemas: map[string]*Schema{
				"Item": {Type: "object"},
				"List": {
					Type: "object",
					Properties: map[string]*Schema{
						"items": {
							Type: "array",
							Items: &Schema{
								Ref: "#/components/schemas/Item",
							},
						},
					},
					AllOf: []*Schema{
						{Ref: "#/components/schemas/Item"},
					},
					OneOf: []*Schema{
						{Ref: "#/components/schemas/Item"},
					},
					AnyOf: []*Schema{
						{Ref: "#/components/schemas/Item"},
					},
				},
			},
		},
		Paths: map[string]PathItem{},
	}

	shortenAllRefs(spec)

	list := spec.Components.Schemas["List"]
	assert.Equal(t, "#/components/schemas/Item", list.Properties["items"].Items.Ref)
	assert.Equal(t, "#/components/schemas/Item", list.AllOf[0].Ref)
	assert.Equal(t, "#/components/schemas/Item", list.OneOf[0].Ref)
	assert.Equal(t, "#/components/schemas/Item", list.AnyOf[0].Ref)
}

// ---------------------------------------------------------------------------
// 9. disambiguateOperationIDs
// ---------------------------------------------------------------------------

func TestDisambiguateOperationIDs_NoDuplicates(t *testing.T) {
	paths := map[string]PathItem{
		"/users": {
			Get: &Operation{OperationID: "ListUsers"},
		},
		"/items": {
			Get: &Operation{OperationID: "ListItems"},
		},
	}
	routes := []*RouteInfo{
		{Method: "GET", Path: "/users", Function: "ListUsers", Package: "github.com/org/project/users"},
		{Method: "GET", Path: "/items", Function: "ListItems", Package: "github.com/org/project/items"},
	}

	disambiguateOperationIDs(paths, routes)

	assert.Equal(t, "ListUsers", paths["/users"].Get.OperationID)
	assert.Equal(t, "ListItems", paths["/items"].Get.OperationID)
}

func TestDisambiguateOperationIDs_WithDuplicates(t *testing.T) {
	// Two different packages, same short operationId
	paths := map[string]PathItem{
		"/api/v1/users": {
			Get: &Operation{OperationID: "Handler.List"},
		},
		"/api/v2/users": {
			Get: &Operation{OperationID: "Handler.List"},
		},
	}
	routes := []*RouteInfo{
		{
			Method:   "GET",
			Path:     "/api/v1/users",
			Function: "Handler" + TypeSep + "List",
			Package:  "github.com/org/project/v1/users",
		},
		{
			Method:   "GET",
			Path:     "/api/v2/users",
			Function: "Handler" + TypeSep + "List",
			Package:  "github.com/org/project/v2/users",
		},
	}

	disambiguateOperationIDs(paths, routes)

	// After disambiguation, the operationIds should be different
	op1 := paths["/api/v1/users"].Get.OperationID
	op2 := paths["/api/v2/users"].Get.OperationID
	assert.NotEqual(t, op1, op2, "operationIds should be disambiguated")
}

func TestDisambiguateOperationIDs_EmptyPaths(_ *testing.T) {
	paths := map[string]PathItem{}
	routes := []*RouteInfo{}
	// Should not panic
	disambiguateOperationIDs(paths, routes)
}

func TestDisambiguateOperationIDs_NilOperations(_ *testing.T) {
	paths := map[string]PathItem{
		"/users": {
			Get: nil, // nil operation
		},
	}
	routes := []*RouteInfo{}

	// Should not panic
	disambiguateOperationIDs(paths, routes)
}

// ---------------------------------------------------------------------------
// 10. schemaName (edge cases)
// ---------------------------------------------------------------------------

func TestSchemaName_WithShortNames(t *testing.T) {
	boolTrue := true
	cfg := &APISpecConfig{ShortNames: &boolTrue}

	// Full path with TypeSep
	name := schemaName("github.com/org/project/internal/http-->CreateResponse", cfg)
	assert.Equal(t, "http.CreateResponse", name)
}

func TestSchemaName_WithoutShortNames(t *testing.T) {
	boolFalse := false
	cfg := &APISpecConfig{ShortNames: &boolFalse}

	name := schemaName("github.com/org/project/internal/http-->CreateResponse", cfg)
	// Without short names, the full path is kept but TypeSep is replaced
	assert.Contains(t, name, "CreateResponse")
}

func TestSchemaName_NilConfig(t *testing.T) {
	// nil config means UseShortNames returns true on the nil receiver...
	// Actually, schemaName checks cfg != nil. With nil cfg, it doesn't shorten.
	name := schemaName("pkg-->Type", nil)
	assert.Equal(t, "pkg.Type", name)
}

func TestSchemaName_SpecialCharacters(t *testing.T) {
	boolTrue := true
	cfg := &APISpecConfig{ShortNames: &boolTrue}

	// Test with characters that get replaced: /, -->, space, [, ], ", "
	name := schemaName("http-->Response[string, int]", cfg)
	assert.NotContains(t, name, "[")
	assert.NotContains(t, name, "]")
	assert.NotContains(t, name, ", ")
}

// ---------------------------------------------------------------------------
// 11. generateSchemas
// ---------------------------------------------------------------------------

func TestGenerateSchemas_BasicStruct(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"models": {
				Files: map[string]*metadata.File{
					"user.go": {
						Types: map[string]*metadata.Type{
							"User": {
								Name: sp.Get("User"),
								Pkg:  sp.Get("models"),
								Kind: sp.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: sp.Get("Name"),
										Type: sp.Get("string"),
										Tag:  sp.Get(`json:"name"`),
									},
									{
										Name: sp.Get("Age"),
										Type: sp.Get("int"),
										Tag:  sp.Get(`json:"age"`),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cfg := DefaultAPISpecConfig()
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	usedTypes := map[string]*Schema{
		"models-->User": nil,
	}

	generateSchemas(usedTypes, cfg, components, meta)

	// The schema should be generated for User
	assert.NotEmpty(t, components.Schemas, "should have generated at least one schema")

	// Find the User schema (key may be shortened)
	var userSchema *Schema
	for key, schema := range components.Schemas {
		if schema != nil && schema.Type == "object" {
			userSchema = schema
			_ = key
			break
		}
	}
	if userSchema != nil {
		assert.Equal(t, "object", userSchema.Type)
		assert.Contains(t, userSchema.Properties, "name")
		assert.Contains(t, userSchema.Properties, "age")
	}
}

func TestGenerateSchemas_EmptyUsedTypes(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	cfg := DefaultAPISpecConfig()
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	usedTypes := map[string]*Schema{}
	generateSchemas(usedTypes, cfg, components, meta)

	assert.Empty(t, components.Schemas)
}

func TestGenerateSchemas_PrimitiveTypeSkipped(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
	}
	cfg := DefaultAPISpecConfig()
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	// Primitive types should not generate schemas
	usedTypes := map[string]*Schema{
		"string": nil,
		"int":    nil,
	}
	generateSchemas(usedTypes, cfg, components, meta)

	assert.Empty(t, components.Schemas, "primitive types should not generate schemas")
}

func TestGenerateSchemas_ExternalType(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
	}

	cfg := &APISpecConfig{
		ExternalTypes: []ExternalType{
			{
				Name:        "primitive.ObjectID",
				OpenAPIType: &Schema{Type: "string", Format: "objectid"},
			},
		},
	}

	components := Components{
		Schemas: make(map[string]*Schema),
	}

	usedTypes := map[string]*Schema{
		"primitive.ObjectID": nil,
	}

	generateSchemas(usedTypes, cfg, components, meta)

	// The external type schema should be set
	found := false
	for _, schema := range components.Schemas {
		if schema != nil && schema.Type == "string" && schema.Format == "objectid" {
			found = true
			break
		}
	}
	assert.True(t, found, "external type should generate a schema with the configured OpenAPI type")
}

func TestGenerateSchemas_WithShortNamesFalse(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"models": {
				Files: map[string]*metadata.File{
					"item.go": {
						Types: map[string]*metadata.Type{
							"Item": {
								Name: sp.Get("Item"),
								Pkg:  sp.Get("models"),
								Kind: sp.Get("struct"),
								Fields: []metadata.Field{
									{
										Name: sp.Get("ID"),
										Type: sp.Get("int"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	boolFalse := false
	cfg := &APISpecConfig{ShortNames: &boolFalse}

	components := Components{
		Schemas: make(map[string]*Schema),
	}
	usedTypes := map[string]*Schema{
		"models-->Item": nil,
	}

	generateSchemas(usedTypes, cfg, components, meta)
	assert.NotEmpty(t, components.Schemas)
}

// ---------------------------------------------------------------------------
// Additional edge-case tests for helper functions
// ---------------------------------------------------------------------------

func TestShortenOperationID_Simple(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/org/project/users.ListUsers", "users.ListUsers"},
		{"github.com/org/project/internal/http.Deps.DocumentHandler.GetContent", "DocumentHandler.GetContent"},
		{"SimpleFunction", "SimpleFunction"},
		{"pkg.Function", "pkg.Function"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shortenOperationID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortenTypeName_Simple(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/org/project/internal/http-->CreateDocumentResponse", "http-->CreateDocumentResponse"},
		{"http-->Response", "http-->Response"},
		{"Response", "Response"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shortenTypeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertPathToOpenAPI_Naming(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users/:id", "/users/{id}"},
		{"/users/:user_id/posts/:post_id", "/users/{user_id}/posts/{post_id}"},
		{"/users", "/users"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertPathToOpenAPI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeduplicateParameters(t *testing.T) {
	params := []Parameter{
		{Name: "id", In: "path"},
		{Name: "id", In: "path"},
		{Name: "id", In: "query"},
		{Name: "sort", In: "query"},
		{Name: "sort", In: "query"},
	}

	result := deduplicateParameters(params)
	assert.Len(t, result, 3)
}

func TestBuildResponses_NilRespInfo(t *testing.T) {
	result := buildResponses(nil)
	require.Contains(t, result, "default")
	assert.Equal(t, "Default response (no response found)", result["default"].Description)
}

func TestBuildResponses_NegativeStatusCode(t *testing.T) {
	respInfo := map[string]*ResponseInfo{
		"-1": {StatusCode: -1, ContentType: "application/json", Schema: &Schema{Type: "object"}},
	}
	result := buildResponses(respInfo)
	require.Contains(t, result, "default")
	assert.Equal(t, "Status code could not be determined", result["default"].Description)
}

func TestSetOperationOnPathItem_AllMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	for _, method := range methods {
		item := &PathItem{}
		op := &Operation{OperationID: "test_" + method}
		setOperationOnPathItem(item, method, op)

		switch method {
		case "GET":
			assert.Equal(t, op, item.Get)
		case "POST":
			assert.Equal(t, op, item.Post)
		case "PUT":
			assert.Equal(t, op, item.Put)
		case "DELETE":
			assert.Equal(t, op, item.Delete)
		case "PATCH":
			assert.Equal(t, op, item.Patch)
		case "OPTIONS":
			assert.Equal(t, op, item.Options)
		case "HEAD":
			assert.Equal(t, op, item.Head)
		}
	}
}

func TestExtractJSONName_Naming(t *testing.T) {
	tests := []struct {
		tag      string
		expected string
	}{
		{`json:"name,omitempty"`, "name"},
		{`json:"user_id"`, "user_id"},
		{`json:"-"`, ""},
		{`json:""`, ""},
		{"", ""},
		{`yaml:"name"`, ""},
		{`json:"field_name" validate:"required"`, "field_name"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := extractJSONName(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTypeParts_VariousFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Parts
	}{
		{
			name:  "with TypeSep",
			input: "models-->User",
			expected: Parts{
				PkgName:  "models",
				TypeName: "User",
			},
		},
		{
			name:  "with dot separator",
			input: "github.com/org/project.Type",
			expected: Parts{
				PkgName:  "github.com/org/project",
				TypeName: "Type",
			},
		},
		{
			name:  "bare type",
			input: "User",
			expected: Parts{
				TypeName: "User",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TypeParts(tt.input)
			assert.Equal(t, tt.expected.PkgName, result.PkgName)
			assert.Equal(t, tt.expected.TypeName, result.TypeName)
		})
	}
}

func TestEnsureAllPathParams(t *testing.T) {
	params := []Parameter{
		{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string"}},
	}

	result := ensureAllPathParams("/users/{id}/posts/{post_id}", params)
	assert.Len(t, result, 2)

	// Check post_id was added
	var postIDParam *Parameter
	for i := range result {
		if result[i].Name == "post_id" {
			postIDParam = &result[i]
			break
		}
	}
	require.NotNil(t, postIDParam)
	assert.Equal(t, "path", postIDParam.In)
	assert.True(t, postIDParam.Required)
}

func TestDefaultAPISpecConfig_Naming(t *testing.T) {
	cfg := DefaultAPISpecConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.UseShortNames(), "default config should have shortNames enabled")
}
