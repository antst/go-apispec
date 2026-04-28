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
	"regexp"

	"github.com/antst/go-apispec/pkg/patterns"
)

const (
	defaultRequestContentType  = "application/json"
	defaultResponseContentType = "application/json"
	defaultResponseStatus      = 200
)

// FrameworkConfig defines framework-specific extraction patterns
type FrameworkConfig struct {
	// Route extraction patterns
	RoutePatterns []RoutePattern `yaml:"routePatterns"`

	// Request body extraction patterns
	RequestBodyPatterns []RequestBodyPattern `yaml:"requestBodyPatterns"`

	// Response extraction patterns
	ResponsePatterns []ResponsePattern `yaml:"responsePatterns"`

	// Parameter extraction patterns
	ParamPatterns []ParamPattern `yaml:"paramPatterns"`

	// Mount/subrouter patterns
	MountPatterns []MountPattern `yaml:"mountPatterns"`

	// Content-Type extraction patterns (matches Header().Set("Content-Type", value))
	ContentTypePatterns []ContentTypePattern `yaml:"contentTypePatterns,omitempty"`
}

// BasePattern contains the shared matching fields used by all pattern types.
type BasePattern struct {
	CallRegex              string   `yaml:"callRegex,omitempty"`
	FunctionNameRegex      string   `yaml:"functionNameRegex,omitempty"`
	RecvType               string   `yaml:"recvType,omitempty"`
	RecvTypeRegex          string   `yaml:"recvTypeRegex,omitempty"`
	CallerPkgPatterns      []string `yaml:"callerPkgPatterns,omitempty"`
	CallerRecvTypePatterns []string `yaml:"callerRecvTypePatterns,omitempty"`
	CalleePkgPatterns      []string `yaml:"calleePkgPatterns,omitempty"`
	CalleeRecvTypePatterns []string `yaml:"calleeRecvTypePatterns,omitempty"`
}

// ContentTypePattern defines how to extract Content-Type from header calls.
type ContentTypePattern struct {
	BasePattern         `yaml:",inline"`
	HeaderNameArgIndex  int `yaml:"headerNameArgIndex,omitempty"`
	HeaderValueArgIndex int `yaml:"headerValueArgIndex,omitempty"`
}

// RoutePattern defines how to extract route information
type RoutePattern struct {
	BasePattern `yaml:",inline"`

	// Argument extraction hints
	MethodArgIndex  int `yaml:"methodArgIndex,omitempty"`  // Which arg contains HTTP method
	PathArgIndex    int `yaml:"pathArgIndex,omitempty"`    // Which arg contains path
	HandlerArgIndex int `yaml:"handlerArgIndex,omitempty"` // Which arg contains handler

	// Extraction hints
	MethodFromCall    bool `yaml:"methodFromCall,omitempty"`    // Extract method from function name
	MethodFromHandler bool `yaml:"methodFromHandler,omitempty"` // Extract method from handler function name
	PathFromArg       bool `yaml:"pathFromArg,omitempty"`       // Extract path from argument
	HandlerFromArg    bool `yaml:"handlerFromArg,omitempty"`    // Extract handler from argument

	// Method extraction configuration
	MethodExtraction *MethodExtractionConfig `yaml:"methodExtraction,omitempty"`
}

// RequestBodyPattern defines how to extract request body information
type RequestBodyPattern struct {
	BasePattern `yaml:",inline"`

	// Argument extraction hints
	TypeArgIndex int `yaml:"typeArgIndex,omitempty"` // Which arg contains type info

	// Extraction hints
	TypeFromArg    bool `yaml:"typeFromArg,omitempty"`    // Extract type from argument
	TypeFromReturn bool `yaml:"typeFromReturn,omitempty"` // Extract type from return value
	Deref          bool `yaml:"deref,omitempty"`          // Dereference pointer types

	// Context-aware validation
	AllowForGetMethods bool `yaml:"allowForGetMethods,omitempty"` // Allow this pattern for GET/HEAD methods
}

// ResponsePattern defines how to extract response information
type ResponsePattern struct {
	BasePattern `yaml:",inline"`

	// Argument extraction hints
	StatusArgIndex int `yaml:"statusArgIndex,omitempty"` // Which arg contains status code
	TypeArgIndex   int `yaml:"typeArgIndex,omitempty"`   // Which arg contains type info

	// Extraction hints
	StatusFromArg bool `yaml:"statusFromArg,omitempty"` // Extract status from argument
	TypeFromArg   bool `yaml:"typeFromArg,omitempty"`   // Extract type from argument
	Deref         bool `yaml:"deref,omitempty"`         // Dereference pointer types
	// DefaultStatus specifies a fallback status code when it can't be extracted from args
	DefaultStatus int `yaml:"defaultStatus,omitempty"`
	// DefaultContentType overrides the config default content type when set
	DefaultContentType string `yaml:"defaultContentType,omitempty"`
	// DefaultBodyType specifies a fixed body type when the function writes a
	// response but has no type argument to extract (e.g., fmt.Fprintf → "string",
	// io.Copy → "[]byte"). Used only when TypeFromArg is false.
	DefaultBodyType string `yaml:"defaultBodyType,omitempty"`
}

// ParamPattern defines how to extract parameter information
type ParamPattern struct {
	BasePattern `yaml:",inline"`

	// Parameter location and extraction
	ParamIn       string `yaml:"paramIn,omitempty"`       // path, query, header, cookie
	ParamArgIndex int    `yaml:"paramArgIndex,omitempty"` // Which arg contains parameter
	TypeArgIndex  int    `yaml:"typeArgIndex,omitempty"`  // Which arg contains type info

	// Extraction hints
	TypeFromArg bool `yaml:"typeFromArg,omitempty"` // Extract type from argument
	Deref       bool `yaml:"deref,omitempty"`       // Dereference pointer types

	// Default schema hints used when TypeFromArg is false. Useful for calls
	// like r.FormFile(name) where the schema is fixed (string/binary) regardless
	// of how the result is consumed.
	DefaultType   string `yaml:"defaultType,omitempty"`
	DefaultFormat string `yaml:"defaultFormat,omitempty"`
}

// MountPattern defines how to extract mount/subrouter information
type MountPattern struct {
	BasePattern `yaml:",inline"`

	// Argument extraction hints
	PathArgIndex   int `yaml:"pathArgIndex,omitempty"`   // Which arg contains mount path
	RouterArgIndex int `yaml:"routerArgIndex,omitempty"` // Which arg contains router

	// Extraction hints
	PathFromArg   bool `yaml:"pathFromArg,omitempty"`   // Extract path from argument
	RouterFromArg bool `yaml:"routerFromArg,omitempty"` // Extract router from argument
	IsMount       bool `yaml:"isMount,omitempty"`       // This is a mount operation
}

// defaultContentTypePatterns returns the standard Content-Type extraction
// patterns used by all framework configs.
func defaultContentTypePatterns() []ContentTypePattern {
	return []ContentTypePattern{
		{BasePattern: BasePattern{CallRegex: `^Set$`,
			RecvTypeRegex: `^net/http\.Header$`}, HeaderNameArgIndex: 0,
			HeaderValueArgIndex: 1,
		},
	}
}

// MethodMapping defines how to extract HTTP methods from function names
type MethodMapping struct {
	Patterns []string `yaml:"patterns,omitempty"` // Function name patterns (e.g., ["get", "list", "show"])
	Method   string   `yaml:"method,omitempty"`   // HTTP method (e.g., "GET")
	Priority int      `yaml:"priority,omitempty"` // Higher priority = checked first
}

// MethodExtractionConfig defines how to extract HTTP methods
type MethodExtractionConfig struct {
	// Method mappings from function names
	MethodMappings []MethodMapping `yaml:"methodMappings,omitempty"`

	// Extraction strategy
	UsePrefix     bool `yaml:"usePrefix,omitempty"`     // Check for prefix matches (getUser -> GET)
	UseContains   bool `yaml:"useContains,omitempty"`   // Check for contains matches (userGet -> GET)
	CaseSensitive bool `yaml:"caseSensitive,omitempty"` // Case sensitive matching

	// Fallback behavior
	DefaultMethod    string `yaml:"defaultMethod,omitempty"`    // Default method when none found
	InferFromContext bool   `yaml:"inferFromContext,omitempty"` // Try to infer from call context
}

// TypeMapping maps Go types to OpenAPI schemas
type TypeMapping struct {
	GoType      string  `yaml:"goType"`
	OpenAPIType *Schema `yaml:"openapiType"`
}

// Override provides manual overrides for specific functions
type Override struct {
	FunctionName   string   `yaml:"functionName"`
	Summary        string   `yaml:"summary,omitempty"`
	Description    string   `yaml:"description,omitempty"`
	ResponseStatus int      `yaml:"responseStatus,omitempty"`
	ResponseType   string   `yaml:"responseType,omitempty"`
	Tags           []string `yaml:"tags,omitempty"`
}

// IncludeExclude defines what to include/exclude
type IncludeExclude struct {
	Files     []string `yaml:"files"`
	Packages  []string `yaml:"packages"`
	Functions []string `yaml:"functions"`
	Types     []string `yaml:"types"`
}

// matchesPattern checks if a path matches a gitignore-style pattern
func matchesPattern(pattern, path string) bool {
	return patterns.Match(pattern, path)
}

// ShouldIncludeFile checks if a file should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeFile(filePath string) bool {
	// If no patterns specified, include everything
	if len(ie.Files) == 0 {
		return true
	}

	// Check if file matches any include pattern
	for _, pattern := range ie.Files {
		if matchesPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

// ShouldIncludePackage checks if a package should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludePackage(pkgPath string) bool {
	// If no patterns specified, include everything
	if len(ie.Packages) == 0 {
		return true
	}

	// Check if package matches any include pattern
	for _, pattern := range ie.Packages {
		if matchesPattern(pattern, pkgPath) {
			return true
		}
	}
	return false
}

// ShouldIncludeFunction checks if a function should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeFunction(funcName string) bool {
	// If no patterns specified, include everything
	if len(ie.Functions) == 0 {
		return true
	}

	// Check if function matches any include pattern
	for _, pattern := range ie.Functions {
		if matchesPattern(pattern, funcName) {
			return true
		}
	}
	return false
}

// ShouldIncludeType checks if a type should be included based on include/exclude patterns
func (ie *IncludeExclude) ShouldIncludeType(typeName string) bool {
	// If no patterns specified, include everything
	if len(ie.Types) == 0 {
		return true
	}

	// Check if type matches any include pattern
	for _, pattern := range ie.Types {
		if matchesPattern(pattern, typeName) {
			return true
		}
	}
	return false
}

// ShouldExcludeFile checks if a file should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeFile(filePath string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Files) == 0 {
		return false
	}

	// Check if file matches any exclude pattern
	for _, pattern := range ie.Files {
		if matchesPattern(pattern, filePath) {
			return true
		}
	}
	return false
}

// ShouldExcludePackage checks if a package should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludePackage(pkgPath string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Packages) == 0 {
		return false
	}

	// Check if package matches any exclude pattern
	for _, pattern := range ie.Packages {
		if matchesPattern(pattern, pkgPath) {
			return true
		}
	}
	return false
}

// ShouldExcludeFunction checks if a function should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeFunction(funcName string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Functions) == 0 {
		return false
	}

	// Check if function matches any exclude pattern
	for _, pattern := range ie.Functions {
		if matchesPattern(pattern, funcName) {
			return true
		}
	}
	return false
}

// ShouldExcludeType checks if a type should be excluded based on exclude patterns
func (ie *IncludeExclude) ShouldExcludeType(typeName string) bool {
	// If no patterns specified, exclude nothing
	if len(ie.Types) == 0 {
		return false
	}

	// Check if type matches any exclude pattern
	for _, pattern := range ie.Types {
		if matchesPattern(pattern, typeName) {
			return true
		}
	}
	return false
}

// Defaults provides default values
type Defaults struct {
	RequestContentType  string `yaml:"requestContentType,omitempty"`
	ResponseContentType string `yaml:"responseContentType,omitempty"`
	ResponseStatus      int    `yaml:"responseStatus,omitempty"`
}

// ExternalType defines an external type that should be treated as known
type ExternalType struct {
	Name        string  `yaml:"name"`        // Full type name (e.g., "primitive.ObjectID")
	OpenAPIType *Schema `yaml:"openapiType"` // OpenAPI schema for this type
	Description string  `yaml:"description,omitempty"`
}

// APISpecConfig is the main configuration struct
type APISpecConfig struct {
	// Use short names for operationIds and schema names (strip module path).
	// nil = true (default). Set to false to retain fully-qualified names.
	ShortNames *bool `yaml:"shortNames,omitempty"`

	// Framework-specific patterns
	Framework FrameworkConfig `yaml:"framework"`

	// Type mappings
	TypeMapping []TypeMapping `yaml:"typeMapping"`

	// External types that should be treated as known
	ExternalTypes []ExternalType `yaml:"externalTypes"`

	// Manual overrides
	Overrides []Override `yaml:"overrides"`

	// Include/exclude filters
	Include IncludeExclude `yaml:"include"`
	Exclude IncludeExclude `yaml:"exclude"`

	// Defaults
	Defaults Defaults `yaml:"defaults"`

	// OpenAPI metadata
	Info            Info                      `yaml:"info"`
	Servers         []Server                  `yaml:"servers"`
	Security        []SecurityRequirement     `yaml:"security"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes"`
	Tags            []Tag                     `yaml:"tags"`
	ExternalDocs    *ExternalDocumentation    `yaml:"externalDocs"`
}

// UseShortNames returns true if short names should be used (default when nil).
func (c *APISpecConfig) UseShortNames() bool {
	return c.ShortNames == nil || *c.ShortNames
}

// ShouldIncludeFile checks if a file should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeFile(filePath string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeFile(filePath) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeFile(filePath)
}

// ShouldIncludePackage checks if a package should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludePackage(pkgPath string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludePackage(pkgPath) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludePackage(pkgPath)
}

// ShouldIncludeFunction checks if a function should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeFunction(funcName string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeFunction(funcName) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeFunction(funcName)
}

// ShouldIncludeType checks if a type should be included based on include/exclude filters
func (c *APISpecConfig) ShouldIncludeType(typeName string) bool {
	// First check exclude patterns (exclude takes precedence)
	if c.Exclude.ShouldExcludeType(typeName) {
		return false
	}

	// Then check include patterns
	return c.Include.ShouldIncludeType(typeName)
}

// MatchPattern checks if a pattern matches a value
func (p *RoutePattern) MatchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

// MatchFunctionName checks if the function name regex matches
func (p *RoutePattern) MatchFunctionName(functionName string) bool {
	return p.MatchPattern(p.FunctionNameRegex, functionName)
}

// DefaultChiConfig returns a default configuration for Chi router
func DefaultChiConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,

					RecvTypeRegex: "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$"}, MethodFromCall: true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
				// HandleFunc registers a handler for all methods — method detection
				// deferred to conditional method analysis via CFG.
				{BasePattern: BasePattern{CallRegex: `^HandleFunc$`,
					RecvTypeRegex: "^github.com/go-chi/chi(/v\\d)?\\.\\*?(Router|Mux)$"},
					PathFromArg:    true,
					HandlerFromArg: true,
					PathArgIndex:   0, HandlerArgIndex: 1,
					MethodExtraction: DefaultMethodExtractionConfig(),
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^DecodeJSON$`,

					RecvTypeRegex: "^github\\.com/go-chi/render$"}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Decode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*Decoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`,

					RecvTypeRegex: "json"}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^JSON$`,

					RecvTypeRegex: "^github\\.com/go-chi/render$"}, TypeArgIndex: 2,
					TypeFromArg:   true,
					StatusFromArg: false,
					Deref:         true,
				},
				{BasePattern: BasePattern{CallRegex: `^Status$`,

					RecvTypeRegex: "^github\\.com/go-chi/render$"}, StatusArgIndex: 1,
					StatusFromArg: true,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: "^URLParam$",

					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$"}, ParamIn: "path",
					ParamArgIndex: 1,
				},
				{BasePattern: BasePattern{CallRegex: "^URLParam$",

					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?\\.\\*?Context$"}, ParamIn: "path",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^URLParamFromCtx$",

					RecvTypeRegex: "^github\\.com/go-chi/chi(/v\\d)?$"}, ParamIn: "path",
					ParamArgIndex: 1,
				},
				{BasePattern: BasePattern{CallRegex: "^FormValue$"}, ParamIn: "form",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormFile$"}, ParamIn: "form",
					ParamArgIndex: 0,
					DefaultType:   "string",
					DefaultFormat: "binary",
				},
				{BasePattern: BasePattern{CallRegex: "^Get$",

					RecvType: "net/url.Values"}, ParamIn: "query",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^PathValue$",

					RecvType: "net/http.*Request"}, ParamIn: "path",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^Mount$`}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{BasePattern: BasePattern{CallRegex: `^Route$`}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      defaultResponseStatus,
		},
	}
}

// DefaultEchoConfig returns a default configuration for Echo framework
func DefaultEchoConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,

					RecvTypeRegex: "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$"}, MethodFromCall: true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^(?i)(Bind)$`,

					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Decode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*Decoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`,

					RecvTypeRegex: "json"}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`,

					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context"}, StatusArgIndex: 0,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					StatusFromArg: true,
					Deref:         true,
				},
				{BasePattern: BasePattern{CallRegex: `^(?i)(NoContent)$`,

					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context"}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: "^Param$"}, ParamIn: "path",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^QueryParam$",

					RecvTypeRegex: "github\\.com/labstack/echo/v\\d\\.Context"}, ParamIn: "query",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormValue$"}, ParamIn: "form",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormFile$"}, ParamIn: "form",
					ParamArgIndex: 0,
					DefaultType:   "string",
					DefaultFormat: "binary",
				},
				{BasePattern: BasePattern{CallRegex: "^Cookie$"}, ParamIn: "cookie",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^Group$`,

					RecvTypeRegex: "^github\\.com/labstack/echo(/v\\d)?\\.\\*(Echo|Group)$"}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}

// DefaultFiberConfig returns a default configuration for Fiber framework
func DefaultFiberConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`}, MethodFromCall: true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^BodyParser$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?Ctx$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Decode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Decoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`,

					RecvTypeRegex: "json"}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^JSON$`,
					// Fiber's c.JSON does not take status, only data

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, StatusArgIndex: -1,
					TypeArgIndex: 0,
					TypeFromArg:  true,
					Deref:        true,
				},
				{BasePattern: BasePattern{CallRegex: `^Status$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^SendString$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, StatusArgIndex: -1,
					TypeArgIndex: 0,
					TypeFromArg:  true,
				},
				{BasePattern: BasePattern{CallRegex: `^SendStatus$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, StatusArgIndex: 0,
					TypeArgIndex: -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: "^Params$",

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, ParamIn: "path",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^Query$",

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, ParamIn: "query",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormValue$",

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, ParamIn: "form",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormFile$",

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, ParamIn: "form",
					ParamArgIndex: 0,
					DefaultType:   "string",
					DefaultFormat: "binary",
				},
				{BasePattern: BasePattern{CallRegex: "^Cookies$",

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*Ctx$`}, ParamIn: "cookie",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^Mount$`}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{BasePattern: BasePattern{CallRegex: `^Group$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
				{BasePattern: BasePattern{CallRegex: `^Use$`,

					RecvTypeRegex: `^github\.com/gofiber/fiber(/v\d)?\.\*?(App|Router|Group)$`}, PathFromArg: false,
					RouterFromArg: false,
					IsMount:       false,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gofiber/fiber.Map",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}

// DefaultGinConfig returns a default configuration for Gin framework
func DefaultGinConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^(?i)(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)$`,

					RecvTypeRegex: "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$"}, MethodFromCall: true,
					PathFromArg:     true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^(?i)(BindJSON|ShouldBindJSON|BindXML|BindYAML|BindForm|ShouldBind)$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Decode$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`}, StatusArgIndex: 0,
					TypeArgIndex:  1,
					TypeFromArg:   true,
					StatusFromArg: true,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: "^Param$"}, ParamIn: "path",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^Query$"}, ParamIn: "query",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^DefaultQuery$"}, ParamIn: "query",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^GetHeader$"}, ParamIn: "header",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^Group$`,

					RecvTypeRegex: "^github\\.com/gin-gonic/gin\\.\\*(Engine|RouterGroup)$"}, PathFromArg: true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
		ExternalTypes: []ExternalType{
			{
				Name: "github.com/gin-gonic/gin.H",
				OpenAPIType: &Schema{
					Type: "object",
				},
			},
		},
	}
}

// DefaultMethodExtractionConfig returns a default method extraction configuration
func DefaultMethodExtractionConfig() *MethodExtractionConfig {
	return &MethodExtractionConfig{
		MethodMappings: []MethodMapping{
			{Patterns: []string{"get", "list", "show", "find", "fetch", "retrieve"}, Method: "GET", Priority: 10},
			{Patterns: []string{"post", "create", "add", "new", "insert"}, Method: "POST", Priority: 10},
			{Patterns: []string{"put", "update", "edit", "modify", "replace"}, Method: "PUT", Priority: 10},
			{Patterns: []string{"delete", "remove", "destroy"}, Method: "DELETE", Priority: 10},
			{Patterns: []string{"patch", "partial"}, Method: "PATCH", Priority: 10},
			{Patterns: []string{"options"}, Method: "OPTIONS", Priority: 10},
			{Patterns: []string{"head"}, Method: "HEAD", Priority: 10},
		},
		UsePrefix:        true,
		UseContains:      true,
		CaseSensitive:    false,
		DefaultMethod:    "GET",
		InferFromContext: true,
	}
}

// DefaultMuxConfig returns a default configuration for Gorilla Mux framework
func DefaultMuxConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^HandleFunc$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`}, PathFromArg: true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{BasePattern: BasePattern{CallRegex: `^Handle$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`}, PathFromArg: true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					HandlerArgIndex: 1,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{BasePattern: BasePattern{CallRegex: `^HandlerFunc$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Route$`}, HandlerFromArg: true,
					HandlerArgIndex: 0,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{BasePattern: BasePattern{CallRegex: `^Handler$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Route$`}, HandlerFromArg: true,
					HandlerArgIndex: 0,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{BasePattern: BasePattern{CallRegex: `^Path$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?(Router|Route)$`}, PathFromArg: true,
					PathArgIndex: 0,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
				{BasePattern: BasePattern{CallRegex: `^Methods$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?(Router|Route)$`}, MethodFromHandler: true,
					MethodArgIndex: 0,

					MethodExtraction: DefaultMethodExtractionConfig(),
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^Decode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Decoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`,

					RecvTypeRegex: "json"}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`,

					RecvTypeRegex: ".*json(iter)?\\.\\*?Encoder"}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				// Note: mux.Vars(r) returns map[string]string — parameter names
				// are extracted from path templates by ensureAllPathParams instead.
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^PathPrefix$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Router$`}, PathFromArg: true,
					PathArgIndex: 0,
					IsMount:      true,
				},
				{BasePattern: BasePattern{CallRegex: `^Subrouter$`,

					RecvTypeRegex: `^github\.com/gorilla/mux\.\*?Route$`}, IsMount: true,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}

// DefaultHTTPConfig returns a default configuration for net/http
func DefaultHTTPConfig() *APISpecConfig {
	return &APISpecConfig{
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^HandleFunc$`,

					RecvTypeRegex: "^net/http(\\.\\*ServeMux)?$"}, PathFromArg: true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
				},
				{BasePattern: BasePattern{CallRegex: `^Handle$`}, PathFromArg: true,
					HandlerFromArg:  true,
					PathArgIndex:    0,
					MethodArgIndex:  -1,
					HandlerArgIndex: 1,
				},
			},
			MountPatterns: []MountPattern{
				// net/http ServeMux nesting: mux.Handle("/prefix/", childMux)
				{BasePattern: BasePattern{
					CallRegex:     `^Handle$`,
					RecvTypeRegex: `^net/http(\.\*ServeMux)?$`,
				},
					PathFromArg:    true,
					RouterFromArg:  true,
					PathArgIndex:   0,
					RouterArgIndex: 1,
					IsMount:        true,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^Decode$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Unmarshal$`}, TypeArgIndex: 1,
					TypeFromArg: true,
					Deref:       true,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^WriteHeader$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, StatusArgIndex: 0,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^Write$`,

					RecvTypeRegex: `^net/http\.ResponseWriter$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Error$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 2,
					StatusFromArg: true,
					TypeFromArg:   true,
					TypeArgIndex:  1,

					DefaultContentType: "text/plain; charset=utf-8",
				},
				{BasePattern: BasePattern{CallRegex: `^NotFound$`,
					// Always 404

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: -1,
					StatusFromArg: false,
					TypeArgIndex:  -1,

					DefaultStatus: http.StatusNotFound,
				},
				{BasePattern: BasePattern{CallRegex: `^Redirect$`,

					RecvTypeRegex: `^net/http$`}, StatusArgIndex: 3,
					StatusFromArg: true,
					TypeArgIndex:  -1,
				},
				{BasePattern: BasePattern{CallRegex: `^(?i)(JSON|String|XML|YAML|ProtoBuf|Data|File|Redirect)$`}, StatusArgIndex: 0,
					TypeArgIndex: 1,
					TypeFromArg:  true,
					Deref:        true,
				},
				{BasePattern: BasePattern{CallRegex: `^Marshal$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				{BasePattern: BasePattern{CallRegex: `^Encode$`}, TypeArgIndex: 0,
					TypeFromArg: true,
					Deref:       true,
				},
				// stdlib write functions that target io.Writer (often ResponseWriter)
				{BasePattern: BasePattern{CallRegex: `^Fprintf$|^Fprint$|^Fprintln$`,
					RecvTypeRegex: `^fmt$`}, DefaultBodyType: "string",
				},
				{BasePattern: BasePattern{CallRegex: `^Copy$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "[]byte",
				},
				{BasePattern: BasePattern{CallRegex: `^WriteString$`,
					RecvTypeRegex: `^io$`}, DefaultBodyType: "string",
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: "^FormValue$"}, ParamIn: "form",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^FormFile$"}, ParamIn: "form",
					ParamArgIndex: 0,
					DefaultType:   "string",
					DefaultFormat: "binary",
				},
				{BasePattern: BasePattern{CallRegex: "^Get$"}, ParamIn: "header",
					ParamArgIndex: 0,
				},
				{BasePattern: BasePattern{CallRegex: "^Cookie$"}, ParamIn: "cookie",
					ParamArgIndex: 0,
				},
			},
			ContentTypePatterns: defaultContentTypePatterns(),
		},
		Defaults: Defaults{
			RequestContentType:  defaultRequestContentType,
			ResponseContentType: defaultResponseContentType,
			ResponseStatus:      http.StatusOK,
		},
	}
}
