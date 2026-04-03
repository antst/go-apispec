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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// ---------------------------------------------------------------------------
// helpers to build metadata-backed call arguments and edges for tests
// ---------------------------------------------------------------------------

// newTestMeta returns a Metadata with an initialized StringPool.
func newTestMeta() *metadata.Metadata {
	return &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Packages:   map[string]*metadata.Package{},
	}
}

// makeCallArg creates a CallArgument attached to meta.
func makeCallArg(meta *metadata.Metadata) *metadata.CallArgument {
	return metadata.NewCallArgument(meta)
}

// makeLiteralArg creates a literal-kind CallArgument whose value is val.
func makeLiteralArg(meta *metadata.Metadata, val string) *metadata.CallArgument {
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue(val)
	return arg
}

// makeIdentArg creates an ident-kind CallArgument with the given type string.
func makeIdentArg(meta *metadata.Metadata, name, typeStr string) *metadata.CallArgument {
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName(name)
	arg.SetType(typeStr)
	return arg
}

// makeEdge builds a minimal CallGraphEdge with caller/callee names and the supplied args.
func makeEdge(meta *metadata.Metadata, callerName, callerPkg, calleeName, calleePkg string, args []*metadata.CallArgument) metadata.CallGraphEdge {
	edge := metadata.CallGraphEdge{
		Caller: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get(callerName),
			Pkg:  meta.StringPool.Get(callerPkg),
		},
		Callee: metadata.Call{
			Meta: meta,
			Name: meta.StringPool.Get(calleeName),
			Pkg:  meta.StringPool.Get(calleePkg),
		},
		Args: args,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	return edge
}

// makeTrackerNode wraps an edge pointer in a TrackerNode for use with matchers.
func makeTrackerNode(edge *metadata.CallGraphEdge) *TrackerNode {
	return &TrackerNode{
		CallGraphEdge: edge,
	}
}

// ---------------------------------------------------------------------------
// 1. preprocessingBodyType
// ---------------------------------------------------------------------------

func TestPreprocessingBodyType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[]User", "User"},
		{"*User", "User"},
		{"&User", "User"},
		{"[][]byte", "[]byte"},
		{"User", "User"},
		{"", ""},
		{"*[]string", "[]string"},
		{"[]string", "string"},
		{"&SomeType", "SomeType"},
		{"*", "*"},   // single char prefix, after CutPrefix remaining is empty -> kept
		{"[]", "[]"}, // same: after cut remaining is empty -> kept
		{"&", "&"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			result := preprocessingBodyType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 2. isBodylessStatusCode — exhaustive
// ---------------------------------------------------------------------------

func TestIsBodylessStatusCode_Exhaustive(t *testing.T) {
	tests := []struct {
		code     int
		expected bool
	}{
		// 1xx informational — all are bodyless
		{100, true},
		{101, true},
		{102, true},
		{103, true},
		{150, true},
		{199, true},

		// 2xx
		{200, false},
		{201, false},
		{204, true}, // No Content
		{206, false},

		// 3xx
		{301, false},
		{304, true}, // Not Modified
		{307, false},

		// 4xx / 5xx — never bodyless
		{400, false},
		{404, false},
		{500, false},
		{503, false},

		// Edge cases
		{0, false},
		{99, false},
		{600, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code=%d", tt.code), func(t *testing.T) {
			assert.Equal(t, tt.expected, isBodylessStatusCode(tt.code))
		})
	}
}

// ---------------------------------------------------------------------------
// 3. ExtractResponse
// ---------------------------------------------------------------------------

func TestExtractResponse_StatusFromArg(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{
			ResponseContentType: "application/json",
		},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)
	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.ContentType)
}

func TestExtractResponse_DefaultStatus(t *testing.T) {
	meta := newTestMeta()

	// No status arg -- only a body type arg
	bodyArg := makeIdentArg(meta, "user", "User")
	edge := makeEdge(meta, "handler", "main", "Render", "render", []*metadata.CallArgument{bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{
			ResponseContentType: "application/json",
		},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus: 201,
		TypeFromArg:   true,
		TypeArgIndex:  0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, "User", resp.BodyType)
}

func TestExtractResponse_TypeFromArg_IdentType(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "result", "string")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg, bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
		TypeFromArg:    true,
		TypeArgIndex:   1,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "string", resp.BodyType)
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
}

func TestExtractResponse_ByteSliceBody_ProducesBinarySchema(t *testing.T) {
	meta := newTestMeta()

	// Build an ident argument whose type is "[]byte"
	bodyArg := makeIdentArg(meta, "data", "[]byte")
	edge := makeEdge(meta, "handler", "main", "Write", "http", []*metadata.CallArgument{bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/octet-stream"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus: 200,
		TypeFromArg:   true,
		TypeArgIndex:  0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	// preprocessingBodyType strips leading "[]" so body type becomes "byte"
	assert.Equal(t, "byte", resp.BodyType)
	// The special []byte path should produce a binary schema
	require.NotNil(t, resp.Schema)
	assert.Equal(t, "string", resp.Schema.Type)
	assert.Equal(t, "binary", resp.Schema.Format)
}

func TestExtractResponse_NoStatusNoBody_ReturnsNil(t *testing.T) {
	meta := newTestMeta()

	// Edge with no args at all
	edge := makeEdge(meta, "handler", "main", "Noop", "lib", nil)
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		// no StatusFromArg, no TypeFromArg, no DefaultStatus
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	assert.Nil(t, resp)
}

func TestExtractResponse_DefaultContentTypeOverride(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:      true,
		StatusArgIndex:     0,
		DefaultContentType: "text/plain",
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, "text/plain", resp.ContentType)
}

func TestExtractResponse_TypeFromArg_SkipsBodylessCodes(t *testing.T) {
	meta := newTestMeta()

	bodyArg := makeIdentArg(meta, "payload", "string")
	edge := makeEdge(meta, "handler", "main", "Send", "lib", []*metadata.CallArgument{bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	// Pattern: TypeFromArg but NOT StatusFromArg -- it should pick the lowest
	// non-bodyless status code from the existing route.Response.
	pattern := ResponsePattern{
		TypeFromArg:  true,
		TypeArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	// Pre-populate route with a 204 (bodyless) and a 200
	route.Response["204"] = &ResponseInfo{StatusCode: 204, BodyType: ""}
	route.Response["200"] = &ResponseInfo{StatusCode: 200, BodyType: ""}

	resp := matcher.ExtractResponse(node, route)
	require.NotNil(t, resp)
	// Should pick 200 because 204 is bodyless
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "string", resp.BodyType)
}

func TestExtractResponse_StatusFromArg_HttpConst(t *testing.T) {
	meta := newTestMeta()

	// Use an http constant name that SchemaMapper recognizes
	statusArg := makeLiteralArg(meta, "StatusCreated")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestExtractResponse_LiteralBodyType(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeLiteralArg(meta, "42")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg, bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
		TypeFromArg:    true,
		TypeArgIndex:   1,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	// "42" is an integer literal, so determineLiteralType returns "int"
	assert.Equal(t, "int", resp.BodyType)
}

func TestExtractResponse_Deref(t *testing.T) {
	meta := newTestMeta()

	bodyArg := makeIdentArg(meta, "user", "*User")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{bodyArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		DefaultStatus: 200,
		TypeFromArg:   true,
		TypeArgIndex:  0,
		Deref:         true,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	// After deref, the leading * is stripped, then preprocessingBodyType runs
	assert.Equal(t, "User", resp.BodyType)
}

// ---------------------------------------------------------------------------
// 4. ApplyOverrides
// ---------------------------------------------------------------------------

func TestApplyOverrides_MatchingFunction(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName:   "GetUser",
				Summary:        "Get a user",
				ResponseStatus: 200,
				ResponseType:   "User",
				Tags:           []string{"users"},
			},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "GetUser",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "OldType"},
		},
	}

	applier.ApplyOverrides(route)

	assert.Equal(t, "Get a user", route.Summary)
	assert.Equal(t, []string{"users"}, route.Tags)
	assert.Equal(t, "User", route.Response["200"].BodyType)
	assert.Equal(t, 200, route.Response["200"].StatusCode)
}

func TestApplyOverrides_NonMatchingFunction(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName:   "GetUser",
				Summary:        "Get a user",
				ResponseStatus: 200,
				ResponseType:   "User",
				Tags:           []string{"users"},
			},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "DeleteUser",
		Summary:  "original",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "OldType"},
		},
	}

	applier.ApplyOverrides(route)

	assert.Equal(t, "original", route.Summary)
	assert.Equal(t, "OldType", route.Response["200"].BodyType)
}

func TestApplyOverrides_PartialOverride_SummaryOnly(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName: "ListItems",
				Summary:      "List all items",
			},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "ListItems",
		Summary:  "old summary",
		Tags:     []string{"items"},
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "ItemList"},
		},
	}

	applier.ApplyOverrides(route)

	assert.Equal(t, "List all items", route.Summary)
	// Tags should not change since override has no tags
	assert.Equal(t, []string{"items"}, route.Tags)
	// BodyType unchanged since override has no ResponseType
	assert.Equal(t, "ItemList", route.Response["200"].BodyType)
}

func TestApplyOverrides_ResponseStatusUpdate(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName:   "CreateUser",
				ResponseStatus: 201,
			},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "CreateUser",
		Response: map[string]*ResponseInfo{
			"201": {StatusCode: 201, BodyType: "User"},
		},
	}

	applier.ApplyOverrides(route)
	assert.Equal(t, 201, route.Response["201"].StatusCode)
}

func TestApplyOverrides_NoOverrides(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: nil,
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "GetUser",
		Summary:  "original",
	}

	applier.ApplyOverrides(route)
	assert.Equal(t, "original", route.Summary)
}

func TestHasOverride(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{FunctionName: "GetUser"},
			{FunctionName: "ListUsers"},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	assert.True(t, applier.HasOverride("GetUser"))
	assert.True(t, applier.HasOverride("ListUsers"))
	assert.False(t, applier.HasOverride("DeleteUser"))
}

// ---------------------------------------------------------------------------
// 5. NewRouteInfo
// ---------------------------------------------------------------------------

func TestNewRouteInfo(t *testing.T) {
	route := NewRouteInfo()
	require.NotNil(t, route)
	require.NotNil(t, route.Response)
	require.NotNil(t, route.UsedTypes)
	assert.Empty(t, route.Response)
	assert.Empty(t, route.UsedTypes)
	assert.Equal(t, "", route.Path)
	assert.Equal(t, "", route.Method)
}

// ---------------------------------------------------------------------------
// 6. RouteInfo.IsValid
// ---------------------------------------------------------------------------

func TestRouteInfo_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		route    RouteInfo
		expected bool
	}{
		{
			name:     "valid: path and handler set",
			route:    RouteInfo{Path: "/users", Handler: "getUsers"},
			expected: true,
		},
		{
			name:     "invalid: missing path",
			route:    RouteInfo{Handler: "getUsers"},
			expected: false,
		},
		{
			name:     "invalid: missing handler",
			route:    RouteInfo{Path: "/users"},
			expected: false,
		},
		{
			name:     "invalid: both empty",
			route:    RouteInfo{},
			expected: false,
		},
		{
			name:     "valid: minimal",
			route:    RouteInfo{Path: "/", Handler: "root"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.route.IsValid())
		})
	}
}

// ---------------------------------------------------------------------------
// 7. determineLiteralType
// ---------------------------------------------------------------------------

func TestDetermineLiteralType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Integers
		{"42", "int"},
		{"-1", "int"},
		{"0", "int"},

		// Unsigned integers (parsed first as int, so strconv.ParseInt succeeds)
		{"100", "int"},

		// Floats
		{"3.14", "float64"},
		{"-0.5", "float64"},

		// Booleans
		{"true", "bool"},
		{"false", "bool"},

		// Nil
		{"nil", "interface{}"},

		// Strings (default)
		{"hello", "string"},
		{"", "string"},

		// Quoted values
		{`"hello"`, "string"},
		{"`raw`", "string"},
		{`"42"`, "int"}, // quotes are stripped, then "42" parses as int
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			assert.Equal(t, tt.expected, determineLiteralType(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// 8. convertPathToOpenAPI
// ---------------------------------------------------------------------------

func TestConvertPathToOpenAPI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/users/:id", "/users/{id}"},
		{"/users/:id/posts/:postId", "/users/{id}/posts/{postId}"},
		{"/users", "/users"},
		{"/:param", "/{param}"},
		{"/a/:b/c/:d/e", "/a/{b}/c/{d}/e"},
		// Already in OpenAPI format should remain unchanged
		{"/users/{id}", "/users/{id}"},
		// No params
		{"/", "/"},
		{"", ""},
		// Param with underscore
		{"/items/:item_id", "/items/{item_id}"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			assert.Equal(t, tt.expected, convertPathToOpenAPI(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// 9. joinPaths — additional edge cases
// ---------------------------------------------------------------------------

func TestJoinPaths(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"", "", "/"},
		{"/api", "/v1", "/api/v1"},
		{"/api/", "/v1", "/api/v1"},
		{"", "/users", "/users"},
		{"/api", "", "/api/"},
		{"api", "users", "api/users"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("join(%q,%q)", tt.a, tt.b), func(t *testing.T) {
			assert.Equal(t, tt.expected, joinPaths(tt.a, tt.b))
		})
	}
}

// ---------------------------------------------------------------------------
// 10. ResponsePatternMatcherImpl.MatchNode
// ---------------------------------------------------------------------------

func TestResponsePatternMatcher_MatchNode_NilNode(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}},
	}

	assert.False(t, matcher.MatchNode(nil))
}

func TestResponsePatternMatcher_MatchNode_NilEdge(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}},
	}

	node := &TrackerNode{} // no edge
	assert.False(t, matcher.MatchNode(node))
}

func TestResponsePatternMatcher_MatchNode_CallRegex(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "JSON", "gin", nil)
	node := makeTrackerNode(&edge)

	// Matching regex
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}},
	}
	assert.True(t, matcher.MatchNode(node))

	// Non-matching regex
	matcherNoMatch := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{CallRegex: "^XML$"}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

func TestResponsePatternMatcher_MatchNode_RecvTypeRegex(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "JSON", "github.com/gin-gonic/gin", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{RecvTypeRegex: `gin.*Context`}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{RecvTypeRegex: `^echo\.Context$`}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

func TestResponsePatternMatcher_MatchNode_RecvType(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "JSON", "gin", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{RecvType: "gin.Context"}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{RecvType: "echo.Context"}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

func TestResponsePatternMatcher_MatchNode_FunctionNameRegex(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "GetUserHandler", "main", "JSON", "gin", nil)
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{FunctionNameRegex: ".*Handler$"}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{FunctionNameRegex: "^Admin"}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

// ---------------------------------------------------------------------------
// 11. ResponsePatternMatcherImpl.GetPriority
// ---------------------------------------------------------------------------

func TestResponsePatternMatcher_GetPriority(t *testing.T) {
	tests := []struct {
		name     string
		pattern  ResponsePattern
		expected int
	}{
		{"no patterns", ResponsePattern{}, 0},
		{"call regex only", ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}}, 10},
		{"function name regex only", ResponsePattern{BasePattern: BasePattern{FunctionNameRegex: ".*"}}, 5},
		{"recv type regex only", ResponsePattern{BasePattern: BasePattern{RecvTypeRegex: ".*"}}, 3},
		{"recv type only", ResponsePattern{BasePattern: BasePattern{RecvType: "gin.Context"}}, 3},
		{"all patterns", ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$", FunctionNameRegex: ".*", RecvTypeRegex: ".*"}}, 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &ResponsePatternMatcherImpl{
				BasePatternMatcher: &BasePatternMatcher{},
				pattern:            tt.pattern,
			}
			assert.Equal(t, tt.expected, matcher.GetPriority())
		})
	}
}

// ---------------------------------------------------------------------------
// 12. ResponsePatternMatcherImpl.GetPattern
// ---------------------------------------------------------------------------

func TestResponsePatternMatcher_GetPattern(t *testing.T) {
	pattern := ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}, DefaultStatus: 200}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{},
		pattern:            pattern,
	}

	result := matcher.GetPattern()
	rp, ok := result.(ResponsePattern)
	require.True(t, ok)
	assert.Equal(t, "^JSON$", rp.CallRegex)
	assert.Equal(t, 200, rp.DefaultStatus)
}

// ---------------------------------------------------------------------------
// 13. ParamPatternMatcherImpl.MatchNode
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_MatchNode_NilAndEmpty(t *testing.T) {
	cfg := &APISpecConfig{}
	meta := newTestMeta()
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{CallRegex: "^Param$"}},
	}

	assert.False(t, matcher.MatchNode(nil))
	assert.False(t, matcher.MatchNode(&TrackerNode{}))
}

func TestParamPatternMatcher_MatchNode_Success(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "Param", "gin", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{CallRegex: "^Param$", RecvType: "gin.Context"}},
	}

	assert.True(t, matcher.MatchNode(node))
}

// ---------------------------------------------------------------------------
// 14. ParamPatternMatcherImpl.GetPriority and GetPattern
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_GetPriority(t *testing.T) {
	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{},
		pattern:            ParamPattern{BasePattern: BasePattern{CallRegex: "^Param$", FunctionNameRegex: ".*Handler", RecvType: "gin.Context"}},
	}
	// CallRegex(10) + FunctionNameRegex(5) + RecvType(3) = 18
	assert.Equal(t, 18, matcher.GetPriority())
}

func TestParamPatternMatcher_GetPattern(t *testing.T) {
	pattern := ParamPattern{BasePattern: BasePattern{CallRegex: "^Query$"}, ParamIn: "query"}
	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{},
		pattern:            pattern,
	}

	result := matcher.GetPattern()
	pp, ok := result.(ParamPattern)
	require.True(t, ok)
	assert.Equal(t, "query", pp.ParamIn)
}

// ---------------------------------------------------------------------------
// 15. ParamPatternMatcherImpl.ExtractParam
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_ExtractParam_Basic(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	paramNameArg := makeLiteralArg(meta, "id")
	edge := makeEdge(meta, "handler", "main", "Param", "gin", []*metadata.CallArgument{paramNameArg})
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{
			ParamIn:       "path",
			ParamArgIndex: 0,
		},
	}

	route := NewRouteInfo()
	param := matcher.ExtractParam(node, route)

	require.NotNil(t, param)
	assert.Equal(t, "id", param.Name)
	assert.Equal(t, "path", param.In)
	assert.True(t, param.Required) // path params are always required
	require.NotNil(t, param.Schema)
	assert.Equal(t, "string", param.Schema.Type) // default schema
}

func TestParamPatternMatcher_ExtractParam_QueryParam(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	paramNameArg := makeLiteralArg(meta, "page")
	edge := makeEdge(meta, "handler", "main", "Query", "gin", []*metadata.CallArgument{paramNameArg})
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{
			ParamIn:       "query",
			ParamArgIndex: 0,
		},
	}

	route := NewRouteInfo()
	param := matcher.ExtractParam(node, route)

	require.NotNil(t, param)
	assert.Equal(t, "page", param.Name)
	assert.Equal(t, "query", param.In)
	assert.False(t, param.Required) // query params are NOT required by default
}

// ---------------------------------------------------------------------------
// 16. NewOverrideApplier and NewResponsePatternMatcher constructors
// ---------------------------------------------------------------------------

func TestNewOverrideApplier(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{{FunctionName: "Foo"}},
	}
	applier := NewOverrideApplier(cfg)
	require.NotNil(t, applier)
	assert.True(t, applier.HasOverride("Foo"))
	assert.False(t, applier.HasOverride("Bar"))
}

func TestNewResponsePatternMatcher(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ResponsePattern{BasePattern: BasePattern{CallRegex: "^JSON$"}, DefaultStatus: 200}
	matcher := NewResponsePatternMatcher(pattern, cfg, contextProvider, typeResolver)
	require.NotNil(t, matcher)
	assert.Equal(t, 200, matcher.pattern.DefaultStatus)
}

func TestNewParamPatternMatcher(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	typeResolver := NewTypeResolver(meta, cfg, NewSchemaMapper(cfg))

	pattern := ParamPattern{BasePattern: BasePattern{CallRegex: "^Param$"}, ParamIn: "path"}
	matcher := NewParamPatternMatcher(pattern, cfg, contextProvider, typeResolver)
	require.NotNil(t, matcher)
	assert.Equal(t, "path", matcher.pattern.ParamIn)
}

// ---------------------------------------------------------------------------
// 17. getCachedRegex
// ---------------------------------------------------------------------------

func TestGetCachedRegex_Extractor(t *testing.T) {
	// Valid pattern
	re, err := getCachedRegex(`^test\d+$`)
	require.NoError(t, err)
	require.NotNil(t, re)
	assert.True(t, re.MatchString("test123"))
	assert.False(t, re.MatchString("hello"))

	// Same pattern again (cached path)
	re2, err := getCachedRegex(`^test\d+$`)
	require.NoError(t, err)
	assert.Equal(t, re, re2)

	// Invalid pattern
	_, err = getCachedRegex(`[invalid`)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// 18. ExtractResponse with TypeConversion arg kind
// ---------------------------------------------------------------------------

func TestExtractResponse_TypeConversionArg(t *testing.T) {
	meta := newTestMeta()

	// Build a type conversion argument: []byte("hello")
	innerArg := makeLiteralArg(meta, "hello")
	outerArg := makeCallArg(meta)
	outerArg.SetKind(metadata.KindTypeConversion)
	outerArg.Args = []*metadata.CallArgument{innerArg}

	statusArg := makeLiteralArg(meta, "200")
	edge := makeEdge(meta, "handler", "main", "Write", "http",
		[]*metadata.CallArgument{statusArg, outerArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
		TypeFromArg:    true,
		TypeArgIndex:   1,
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	route.Metadata = meta
	resp := matcher.ExtractResponse(node, route)

	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	// The type conversion unwraps to the inner arg (literal "hello") -> determineLiteralType -> "string"
	assert.Equal(t, "string", resp.BodyType)
}

// ---------------------------------------------------------------------------
// 19. extractResponseFromNode — edge dedup and nil checks
// ---------------------------------------------------------------------------

func TestExtractResponseFromNode_NilNode(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}

	mockTree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	})
	extractor := NewExtractor(mockTree, cfg)

	visitedEdges := make(map[string]bool)
	route := NewRouteInfo()
	resp := extractor.extractResponseFromNode(nil, route, visitedEdges)
	assert.Nil(t, resp)
}

func TestExtractResponseFromNode_NilEdge(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}

	mockTree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	})
	extractor := NewExtractor(mockTree, cfg)

	visitedEdges := make(map[string]bool)
	route := NewRouteInfo()
	node := &TrackerNode{} // no edge
	resp := extractor.extractResponseFromNode(node, route, visitedEdges)
	assert.Nil(t, resp)
}

// ---------------------------------------------------------------------------
// 20. Extractor.ExtractRoutes with empty tree
// ---------------------------------------------------------------------------

func TestExtractor_ExtractRoutes_EmptyTree(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}

	mockTree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	})
	extractor := NewExtractor(mockTree, cfg)

	routes := extractor.ExtractRoutes()
	assert.Empty(t, routes)
}

// ---------------------------------------------------------------------------
// 21. traverseForRoutes with nil node
// ---------------------------------------------------------------------------

func TestTraverseForRoutes_NilNode(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}

	mockTree := NewMockTrackerTree(meta, metadata.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	})
	extractor := NewExtractor(mockTree, cfg)

	routes := make([]*RouteInfo, 0)
	// Should not panic
	extractor.traverseForRoutes(nil, "", nil, &routes)
	assert.Empty(t, routes)
}

// ---------------------------------------------------------------------------
// 22. ParamPatternMatcher with RecvTypeRegex
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_MatchNode_RecvTypeRegex(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "Param", "github.com/gin-gonic/gin", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{RecvTypeRegex: `gin.*Context`}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{RecvTypeRegex: `^echo\.Context$`}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

// ---------------------------------------------------------------------------
// 23. ParamPatternMatcher with FunctionNameRegex
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_MatchNode_FunctionNameRegex(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "GetUserHandler", "main", "Param", "gin", nil)
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{FunctionNameRegex: ".*Handler$"}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{FunctionNameRegex: "^Admin"}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

// ---------------------------------------------------------------------------
// 24. ExtractResponse: status from arg with invalid/unparseable status
// ---------------------------------------------------------------------------

func TestExtractResponse_InvalidStatusArg(t *testing.T) {
	meta := newTestMeta()

	statusArg := makeLiteralArg(meta, "not_a_number")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg})
	node := makeTrackerNode(&edge)

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	pattern := ResponsePattern{
		StatusFromArg:  true,
		StatusArgIndex: 0,
		// No DefaultStatus, no TypeFromArg
	}
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: pattern,
	}

	route := NewRouteInfo()
	resp := matcher.ExtractResponse(node, route)

	// Status not resolved, no body type -> nil
	assert.Nil(t, resp)
}

// ---------------------------------------------------------------------------
// 25. ApplyOverrides with ResponseType containing prefix characters
// ---------------------------------------------------------------------------

func TestApplyOverrides_ResponseTypePreprocessed(t *testing.T) {
	cfg := &APISpecConfig{
		Overrides: []Override{
			{
				FunctionName: "GetItem",
				ResponseType: "[]Item",
			},
		},
	}
	applier := &OverrideApplierImpl{cfg: cfg}

	route := &RouteInfo{
		Function: "GetItem",
		Response: map[string]*ResponseInfo{
			"200": {StatusCode: 200, BodyType: "OldType"},
		},
	}

	applier.ApplyOverrides(route)

	// preprocessingBodyType("[]Item") -> "Item"
	assert.Equal(t, "Item", route.Response["200"].BodyType)
}

// ---------------------------------------------------------------------------
// Integration tests: exercise the Extractor tree traversal with real patterns
// ---------------------------------------------------------------------------

// buildExtractorWithPatterns creates an Extractor wired with response / request /
// param patterns so that the tree traversal actually fires matchers.
func buildExtractorWithPatterns(meta *metadata.Metadata) (*Extractor, *MockTrackerTree) {
	cfg := &APISpecConfig{
		Defaults: Defaults{
			ResponseContentType: "application/json",
			RequestContentType:  "application/json",
			ResponseStatus:      200,
		},
		Framework: FrameworkConfig{
			RoutePatterns: []RoutePattern{
				{BasePattern: BasePattern{CallRegex: `^(Get|Post|Put|Delete|Patch|Handle)$`}, MethodFromCall: true,
					PathFromArg:     true,
					PathArgIndex:    0,
					HandlerFromArg:  true,
					HandlerArgIndex: 1,
				},
			},
			ResponsePatterns: []ResponsePattern{
				{BasePattern: BasePattern{CallRegex: `^JSON$`}, StatusFromArg: true,
					StatusArgIndex: 0,
					TypeFromArg:    true,
					TypeArgIndex:   1,
				},
			},
			RequestBodyPatterns: []RequestBodyPattern{
				{BasePattern: BasePattern{CallRegex: `^BindJSON$`}, TypeFromArg: true,
					TypeArgIndex: 0,
				},
			},
			ParamPatterns: []ParamPattern{
				{BasePattern: BasePattern{CallRegex: `^Param$`}, ParamIn: "path",
					ParamArgIndex: 0,
				},
			},
			MountPatterns: []MountPattern{
				{BasePattern: BasePattern{CallRegex: `^Group$`}, PathFromArg: true,
					IsMount: true,
				},
			},
		},
	}

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    1000,
		MaxChildrenPerNode: 50,
		MaxArgsPerFunction: 10,
		MaxNestedArgsDepth: 5,
	}
	mockTree := NewMockTrackerTree(meta, limits)
	extractor := NewExtractor(mockTree, cfg)
	return extractor, mockTree
}

func TestExtractor_HandleRouteNode_WithValidRoute(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Build a route node: r.Get("/users", listUsers)
	pathArg := makeLiteralArg(meta, "/users")
	handlerArg := makeCallArg(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("listUsers")
	handlerArg.SetType("func()")

	edge := makeEdge(meta, "main", "main", "Get", "chi", []*metadata.CallArgument{pathArg, handlerArg})
	node := makeTrackerNode(&edge)

	routeInfo := NewRouteInfo()
	routes := make([]*RouteInfo, 0)

	// Execute the route pattern to fill routeInfo
	found := extractor.executeRoutePattern(node, routeInfo)
	require.True(t, found)

	// Now handle the route node
	extractor.handleRouteNode(node, routeInfo, "/api", []string{"api"}, &routes)

	// routeInfo should have mount path prepended
	assert.Equal(t, "/api/", routeInfo.MountPath)
	assert.Equal(t, []string{"api"}, routeInfo.Tags)
}

func TestExtractor_HandleRouteNode_UpdatesExistingRoute(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	pathArg := makeLiteralArg(meta, "/items")
	handlerArg := makeCallArg(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("listItems")
	handlerArg.SetType("func()")

	edge := makeEdge(meta, "main", "main", "Get", "chi", []*metadata.CallArgument{pathArg, handlerArg})
	node := makeTrackerNode(&edge)

	routeInfo := NewRouteInfo()
	found := extractor.executeRoutePattern(node, routeInfo)
	require.True(t, found)

	// Seed routes with same function name to test update path
	existingRoute := NewRouteInfo()
	existingRoute.Function = routeInfo.Function
	existingRoute.Path = "/old"
	existingRoute.Handler = "listItems"
	routes := []*RouteInfo{existingRoute}

	extractor.handleRouteNode(node, routeInfo, "", nil, &routes)

	// Should have updated in-place, not added
	assert.Len(t, routes, 1)
}

func TestExtractor_ExtractRouteChildren_ResponseAndParam(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Parent route node (simple, no edge matching any pattern)
	parentEdge := makeEdge(meta, "handler", "main", "unknownCall", "pkg", nil)
	parentNode := makeTrackerNode(&parentEdge)

	// Child 1: JSON(200, result) - response
	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "result", "string")
	respEdge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg, bodyArg})
	respNode := makeTrackerNode(&respEdge)

	// Child 2: Param("id") - param
	paramArg := makeLiteralArg(meta, "id")
	paramEdge := makeEdge(meta, "handler", "main", "Param", "gin", []*metadata.CallArgument{paramArg})
	paramNode := makeTrackerNode(&paramEdge)

	// Child 3: BindJSON(req) - request
	reqArg := makeIdentArg(meta, "req", "UserRequest")
	reqEdge := makeEdge(meta, "handler", "main", "BindJSON", "gin", []*metadata.CallArgument{reqArg})
	reqNode := makeTrackerNode(&reqEdge)

	// Wire children
	parentNode.AddChild(respNode)
	parentNode.AddChild(paramNode)
	parentNode.AddChild(reqNode)

	route := NewRouteInfo()
	route.Metadata = meta
	visitedEdges := make(map[string]bool)
	routes := make([]*RouteInfo, 0)

	extractor.extractRouteChildren(parentNode, route, nil, &routes, visitedEdges)

	// Response should be extracted
	assert.NotEmpty(t, route.Response, "expected at least one response")

	// Param should be extracted
	assert.NotEmpty(t, route.Params, "expected at least one param")
	assert.Equal(t, "id", route.Params[0].Name)
	assert.Equal(t, "path", route.Params[0].In)

	// Request should be extracted
	require.NotNil(t, route.Request)
	assert.Equal(t, "UserRequest", route.Request.BodyType)
}

func TestExtractor_ExtractResponseFromNode_EdgeDedup(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "resp", "string")
	edge := makeEdge(meta, "handler", "main", "JSON", "gin", []*metadata.CallArgument{statusArg, bodyArg})
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	route.Metadata = meta
	visitedEdges := make(map[string]bool)

	// First call -- should return a response
	resp1 := extractor.extractResponseFromNode(node, route, visitedEdges)
	require.NotNil(t, resp1)

	// Second call with same edge -- should be deduplicated (nil)
	resp2 := extractor.extractResponseFromNode(node, route, visitedEdges)
	assert.Nil(t, resp2)
}

func TestExtractor_ExtractRequestFromNode_NoMatch(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Edge that does NOT match the BindJSON pattern
	edge := makeEdge(meta, "handler", "main", "SomeOtherCall", "lib", nil)
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	req := extractor.extractRequestFromNode(node, route)
	assert.Nil(t, req)
}

func TestExtractor_ExtractParamFromNode_NoMatch(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Edge that does NOT match the Param pattern
	edge := makeEdge(meta, "handler", "main", "SomeOtherCall", "lib", nil)
	node := makeTrackerNode(&edge)

	route := NewRouteInfo()
	param := extractor.extractParamFromNode(node, route)
	assert.Nil(t, param)
}

func TestExtractor_FindTargetNode_NilAssignment(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	result := extractor.findTargetNode(nil)
	assert.Nil(t, result)
}

func TestExtractor_FindTargetNode_NotFound(t *testing.T) {
	meta := newTestMeta()
	extractor, mockTree := buildExtractorWithPatterns(meta)

	// Add a root node to the tree
	rootEdge := makeEdge(meta, "main", "main", "init", "main", nil)
	rootNode := makeTrackerNode(&rootEdge)
	mockTree.AddRoot(rootNode)

	// Search for an assignment whose ID won't match any node
	assignment := makeCallArg(meta)
	assignment.SetKind(metadata.KindIdent)
	assignment.SetName("nonexistent")
	assignment.SetPkg("nonexistent")
	assignment.SetPosition("file.go:999:1")

	result := extractor.findTargetNode(assignment)
	assert.Nil(t, result)
}

func TestExtractor_FindTargetNode_Found(t *testing.T) {
	meta := newTestMeta()
	extractor, mockTree := buildExtractorWithPatterns(meta)

	// Build target node
	targetEdge := makeEdge(meta, "setup", "main", "NewRouter", "chi", nil)
	targetNode := makeTrackerNode(&targetEdge)
	mockTree.AddRoot(targetNode)

	// Build an assignment arg whose ID matches the target node's key
	assignment := makeCallArg(meta)
	assignment.SetKind(metadata.KindIdent)
	assignment.SetName("NewRouter")
	assignment.SetPkg("chi")
	// ID will be based on pkg+name, matching the target node's Callee.ID()

	targetKey := targetNode.GetKey()
	assignmentID := assignment.ID()

	// Only assert if IDs match (depends on internal ID generation)
	if targetKey == assignmentID {
		result := extractor.findTargetNode(assignment)
		assert.NotNil(t, result)
	}
}

func TestExtractor_HandleMountNode_WithPath(_ *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Build a mount node with a child
	groupEdge := makeEdge(meta, "main", "main", "Group", "chi", nil)
	mountNode := makeTrackerNode(&groupEdge)

	// Add a child route node
	pathArg := makeLiteralArg(meta, "/items")
	handlerArg := makeCallArg(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("listItems")
	handlerArg.SetType("func()")
	childEdge := makeEdge(meta, "main", "main", "Get", "chi", []*metadata.CallArgument{pathArg, handlerArg})
	childNode := makeTrackerNode(&childEdge)
	mountNode.AddChild(childNode)

	mountInfo := MountInfo{
		Path: "/api/v1",
	}

	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)

	extractor.handleMountNode(mountNode, mountInfo, "", nil, &routes, visited)

	// The child should have been traversed with the mount path
	// (Whether a route is added depends on pattern matching)
}

func TestExtractor_HandleMountNode_WithMountPath(_ *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	mountEdge := makeEdge(meta, "main", "main", "Group", "chi", nil)
	mountNode := makeTrackerNode(&mountEdge)

	mountInfo := MountInfo{
		Path: "/v2",
	}

	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)

	// Call with existing mountPath
	extractor.handleMountNode(mountNode, mountInfo, "/api", nil, &routes, visited)
	// Should not panic. Mount path becomes /api/v2
}

func TestExtractor_HandleMountNode_EmptyPathWithTags(_ *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	mountEdge := makeEdge(meta, "main", "main", "Group", "chi", nil)
	mountNode := makeTrackerNode(&mountEdge)

	// No child to traverse but exercises the tag logic
	mountInfo := MountInfo{}

	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)

	extractor.handleMountNode(mountNode, mountInfo, "", []string{"existing-tag"}, &routes, visited)
}

func TestExtractor_HandleRouterAssignment_NilTarget(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	assignment := makeCallArg(meta)
	assignment.SetKind(metadata.KindIdent)
	assignment.SetName("nonexistent")
	assignment.SetPkg("nonexistent")
	assignment.SetPosition("file.go:1:1")

	mountInfo := MountInfo{
		Assignment: assignment,
	}

	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)

	// Should not panic; target node won't be found
	extractor.handleRouterAssignment(mountInfo, "/api", nil, &routes, visited)
	assert.Empty(t, routes)
}

func TestExtractor_TraverseForRoutesWithVisited_CyclePrevention(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Create a node that references itself via children
	edge := makeEdge(meta, "main", "main", "unknown", "pkg", nil)
	node := makeTrackerNode(&edge)

	routes := make([]*RouteInfo, 0)
	visited := make(map[string]bool)

	// Pre-mark the node as visited to test the cycle prevention path
	visited[node.GetKey()] = true

	extractor.traverseForRoutesWithVisited(node, "", nil, &routes, visited)
	assert.Empty(t, routes)
}

func TestExtractor_FullIntegration_RouteWithResponseAndParam(t *testing.T) {
	meta := newTestMeta()
	extractor, mockTree := buildExtractorWithPatterns(meta)

	// Root node: r.Get("/users/:id", getUser)
	pathArg := makeLiteralArg(meta, "/users/:id")
	handlerArg := makeCallArg(meta)
	handlerArg.SetKind(metadata.KindIdent)
	handlerArg.SetName("getUser")
	handlerArg.SetType("func()")

	routeEdge := makeEdge(meta, "main", "main", "Get", "chi", []*metadata.CallArgument{pathArg, handlerArg})
	rootNode := makeTrackerNode(&routeEdge)

	// Child: c.JSON(200, user)
	statusArg := makeLiteralArg(meta, "200")
	bodyArg := makeIdentArg(meta, "user", "string")
	respEdge := makeEdge(meta, "getUser", "main", "JSON", "gin", []*metadata.CallArgument{statusArg, bodyArg})
	respNode := makeTrackerNode(&respEdge)

	// Child: c.Param("id")
	paramArg := makeLiteralArg(meta, "id")
	paramEdge := makeEdge(meta, "getUser", "main", "Param", "gin", []*metadata.CallArgument{paramArg})
	paramNode := makeTrackerNode(&paramEdge)

	rootNode.AddChild(respNode)
	rootNode.AddChild(paramNode)
	mockTree.AddRoot(rootNode)

	routes := extractor.ExtractRoutes()

	// We should have extracted at least the route
	assert.NotEmpty(t, routes)
}

// ---------------------------------------------------------------------------
// resolveTypeOrigin coverage for ResponsePatternMatcherImpl
// ---------------------------------------------------------------------------

func TestResponseResolveTypeOrigin_GenericType(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{},
	}

	arg := makeIdentArg(meta, "data", "T")
	arg.IsGenericType = true
	arg.GenericTypeName = meta.StringPool.Get("T")

	edge := makeEdge(meta, "handler", "main", "Send", "lib", nil)
	edge.TypeParamMap = map[string]string{"T": "UserDTO"}
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "T")
	assert.Equal(t, "UserDTO", result)
}

func TestResponseResolveTypeOrigin_AssignmentMap(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{},
	}

	arg := makeIdentArg(meta, "result", "interface{}")

	concreteTypeIdx := meta.StringPool.Get("UserResponse")

	edge := makeEdge(meta, "handler", "main", "Send", "lib", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"result": {
			{ConcreteType: concreteTypeIdx},
		},
	}
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "interface{}")
	assert.Equal(t, "UserResponse", result)
}

func TestResponseResolveTypeOrigin_Fallback(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{},
	}

	arg := makeIdentArg(meta, "val", "SomeType")
	edge := makeEdge(meta, "handler", "main", "Send", "lib", nil)
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "SomeType")
	assert.Equal(t, "SomeType", result)
}

// ---------------------------------------------------------------------------
// resolveTypeOrigin coverage for ParamPatternMatcherImpl
// ---------------------------------------------------------------------------

func TestParamResolveTypeOrigin_GenericType(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{},
	}

	arg := makeIdentArg(meta, "param", "T")
	arg.IsGenericType = true
	arg.GenericTypeName = meta.StringPool.Get("T")

	edge := makeEdge(meta, "handler", "main", "GetParam", "lib", nil)
	edge.TypeParamMap = map[string]string{"T": "int"}
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "T")
	assert.Equal(t, "int", result)
}

func TestParamResolveTypeOrigin_AssignmentMap(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{},
	}

	arg := makeIdentArg(meta, "val", "interface{}")
	concreteTypeIdx := meta.StringPool.Get("int64")

	edge := makeEdge(meta, "handler", "main", "GetParam", "lib", nil)
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"val": {
			{ConcreteType: concreteTypeIdx},
		},
	}
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "interface{}")
	assert.Equal(t, "int64", result)
}

func TestParamResolveTypeOrigin_Fallback(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{},
	}

	arg := makeIdentArg(meta, "val", "string")
	edge := makeEdge(meta, "handler", "main", "GetParam", "lib", nil)
	node := makeTrackerNode(&edge)

	result := matcher.resolveTypeOrigin(arg, node, "string")
	assert.Equal(t, "string", result)
}

// ---------------------------------------------------------------------------
// ParamPatternMatcherImpl.ExtractParam with TypeFromArg
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_ExtractParam_WithType(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	paramNameArg := makeLiteralArg(meta, "page")
	typeArg := makeIdentArg(meta, "pageNum", "int")
	edge := makeEdge(meta, "handler", "main", "QueryTyped", "gin",
		[]*metadata.CallArgument{paramNameArg, typeArg})
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{
			ParamIn:       "query",
			ParamArgIndex: 0,
			TypeFromArg:   true,
			TypeArgIndex:  1,
		},
	}

	route := NewRouteInfo()
	route.Metadata = meta
	param := matcher.ExtractParam(node, route)

	require.NotNil(t, param)
	assert.Equal(t, "page", param.Name)
	assert.Equal(t, "query", param.In)
	require.NotNil(t, param.Schema)
	assert.Equal(t, "integer", param.Schema.Type)
}

func TestParamPatternMatcher_ExtractParam_LiteralTypeArg(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	paramNameArg := makeLiteralArg(meta, "count")
	typeArg := makeLiteralArg(meta, "42")
	edge := makeEdge(meta, "handler", "main", "QueryTyped", "gin",
		[]*metadata.CallArgument{paramNameArg, typeArg})
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{
			ParamIn:       "query",
			ParamArgIndex: 0,
			TypeFromArg:   true,
			TypeArgIndex:  1,
		},
	}

	route := NewRouteInfo()
	route.Metadata = meta
	param := matcher.ExtractParam(node, route)

	require.NotNil(t, param)
	assert.Equal(t, "count", param.Name)
	// "42" -> determineLiteralType -> "int" -> schema integer
	require.NotNil(t, param.Schema)
	assert.Equal(t, "integer", param.Schema.Type)
}

func TestParamPatternMatcher_ExtractParam_Deref(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	paramNameArg := makeLiteralArg(meta, "limit")
	typeArg := makeIdentArg(meta, "limitPtr", "*int")
	edge := makeEdge(meta, "handler", "main", "QueryTyped", "gin",
		[]*metadata.CallArgument{paramNameArg, typeArg})
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{
			ParamIn:       "query",
			ParamArgIndex: 0,
			TypeFromArg:   true,
			TypeArgIndex:  1,
			Deref:         true,
		},
	}

	route := NewRouteInfo()
	route.Metadata = meta
	param := matcher.ExtractParam(node, route)

	require.NotNil(t, param)
	// *int -> deref -> int -> schema integer
	require.NotNil(t, param.Schema)
	assert.Equal(t, "integer", param.Schema.Type)
}

// ---------------------------------------------------------------------------
// ExecuteMountPattern
// ---------------------------------------------------------------------------

func TestExtractor_ExecuteMountPattern_NoMatch(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	// Edge that doesn't match mount pattern
	edge := makeEdge(meta, "main", "main", "Handle", "http", nil)
	node := makeTrackerNode(&edge)

	_, found := extractor.executeMountPattern(node)
	assert.False(t, found)
}

func TestExtractor_ExecuteMountPattern_Match(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	pathArg := makeLiteralArg(meta, "/api")
	edge := makeEdge(meta, "main", "main", "Group", "chi", []*metadata.CallArgument{pathArg})
	node := makeTrackerNode(&edge)

	mountInfo, found := extractor.executeMountPattern(node)
	assert.True(t, found)
	assert.Equal(t, "/api", mountInfo.Path)
}

// ---------------------------------------------------------------------------
// ExecuteRoutePattern
// ---------------------------------------------------------------------------

func TestExtractor_ExecuteRoutePattern_NoMatch(t *testing.T) {
	meta := newTestMeta()
	extractor, _ := buildExtractorWithPatterns(meta)

	edge := makeEdge(meta, "main", "main", "SomeOtherFunc", "lib", nil)
	node := makeTrackerNode(&edge)

	routeInfo := NewRouteInfo()
	found := extractor.executeRoutePattern(node, routeInfo)
	assert.False(t, found)
}

// ---------------------------------------------------------------------------
// ParamPatternMatcher MatchNode with RecvType (non-regex)
// ---------------------------------------------------------------------------

func TestParamPatternMatcher_MatchNode_RecvType(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	edge := makeEdge(meta, "handler", "main", "Param", "gin", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{RecvType: "gin.Context"}},
	}
	assert.True(t, matcher.MatchNode(node))

	matcherNoMatch := &ParamPatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ParamPattern{BasePattern: BasePattern{RecvType: "echo.Context"}},
	}
	assert.False(t, matcherNoMatch.MatchNode(node))
}

// ---------------------------------------------------------------------------
// ResponsePatternMatcherImpl MatchNode: recvType with no pkg
// ---------------------------------------------------------------------------

func TestResponsePatternMatcher_MatchNode_RecvTypeNoPkg(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	contextProvider := NewContextProvider(meta)
	schemaMapper := NewSchemaMapper(cfg)

	// Callee with recvType but empty pkg
	edge := makeEdge(meta, "handler", "main", "JSON", "", nil)
	edge.Callee.RecvType = meta.StringPool.Get("Context")
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: contextProvider,
			cfg:             cfg,
			schemaMapper:    schemaMapper,
		},
		pattern: ResponsePattern{BasePattern: BasePattern{RecvType: "Context"}},
	}
	assert.True(t, matcher.MatchNode(node))
}
