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
	"go/types"
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/antst/go-apispec/internal/metadata"
)

// Regex cache for performance optimization
var (
	mapperRegexCache = make(map[string]*regexp.Regexp)
	mapperRegexMutex sync.RWMutex
)

// getCachedMapperRegex returns a cached compiled regex or compiles and caches a new one
func getCachedMapperRegex(pattern string) *regexp.Regexp {
	mapperRegexMutex.RLock()
	if re, exists := mapperRegexCache[pattern]; exists {
		mapperRegexMutex.RUnlock()
		return re
	}
	mapperRegexMutex.RUnlock()

	mapperRegexMutex.Lock()
	defer mapperRegexMutex.Unlock()

	// Double-check after acquiring write lock
	if re, exists := mapperRegexCache[pattern]; exists {
		return re
	}

	re := regexp.MustCompile(pattern)
	mapperRegexCache[pattern] = re
	return re
}

const (
	refComponentsSchemasPrefix = "#/components/schemas/"
)

// schemaComponentNameReplacer sanitizes a Go type string into a valid OpenAPI
// component name (must match ^[a-zA-Z0-9._-]+$). The bare "," case keeps
// multi-type-parameter generic instantiations (e.g. Pair[User,Order]) valid —
// the type string joins arguments with "," (no space), which the earlier ", "
// rule does not catch; it is listed last so ", " still wins at comma-space
// positions (strings.Replacer resolves overlaps in argument order).
var schemaComponentNameReplacer = strings.NewReplacer("/", "_", "-->", ".", " ", "-", "[", "_", "]", "", ", ", "-", ",", "-")

// GeneratorConfig holds generation configuration
type GeneratorConfig struct {
	OpenAPIVersion string `yaml:"openapiVersion"`
	Title          string `yaml:"title"`
	APIVersion     string `yaml:"apiVersion"`
}

// LoadAPISpecConfig loads a APISpecConfig from a YAML file
func LoadAPISpecConfig(path string) (*APISpecConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a user-provided config file path
	if err != nil {
		return nil, err
	}

	var config APISpecConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultAPISpecConfig returns a default configuration
func DefaultAPISpecConfig() *APISpecConfig {
	return &APISpecConfig{}
}

// shortenOperationID strips the Go module path and intermediate type chain
// from an operationId, keeping only the final Type.Method portion.
// e.g. "github.com/org/project/internal/http.Deps.DocumentHandler.GetContent"
//
//	→ "DocumentHandler.GetContent"
//
// If only one dot-segment exists after stripping the path (a bare function),
// the last package segment is preserved for context:
// e.g. "github.com/org/project/users.ListUsers" → "users.ListUsers"
func shortenOperationID(fullID string) string {
	// First strip module path
	short := fullID
	if idx := strings.LastIndex(short, "/"); idx >= 0 {
		short = short[idx+1:]
	}
	// Now short is e.g. "http.Deps.DocumentHandler.GetContent" or "users.ListUsers"
	parts := strings.Split(short, ".")
	if len(parts) <= 2 {
		// "package.Function" or just "Function" — keep as-is
		return short
	}
	// More than 2 segments: keep only the last two (Type.Method)
	return strings.Join(parts[len(parts)-2:], ".")
}

// shortenTypeName strips the Go module path from a type name,
// keeping only the last package segment + type name.
// e.g. "github.com/org/project/internal/http-->CreateDocumentResponse" → "http-->CreateDocumentResponse"
func shortenTypeName(fullName string) string {
	// For generic types like "APIResponse[github.com/.../User]",
	// shorten the base type and each generic parameter separately.
	if bracketIdx := strings.Index(fullName, "["); bracketIdx > 0 && !strings.HasPrefix(fullName, "[]") && !strings.HasPrefix(fullName, "map[") {
		baseName := fullName[:bracketIdx]
		params := fullName[bracketIdx+1:]

		params = strings.TrimSuffix(params, "]")
		shortBase := shortenTypeName(baseName)
		// Shorten each comma-separated param
		shortParams := make([]string, 0, len(strings.Split(params, ",")))
		for _, p := range strings.Split(params, ",") {
			shortParams = append(shortParams, shortenTypeName(strings.TrimSpace(p)))
		}
		return shortBase + "[" + strings.Join(shortParams, ",") + "]"
	}
	if idx := strings.LastIndex(fullName, "/"); idx >= 0 {
		return fullName[idx+1:]
	}
	return fullName
}

// disambiguateOperationIDs scans all operations in paths for duplicate
// operationIds and progressively adds parent package segments until unique.
func disambiguateOperationIDs(paths map[string]PathItem, routes []*RouteInfo) {
	// Build a map of operationId → list of (path, method, route) for collision detection.
	type opRef struct {
		path   string
		method string
		op     *Operation
		route  *RouteInfo
	}

	idRefs := make(map[string][]opRef)
	// Iterate paths and methods in deterministic order — without this, the
	// disambiguation result for two operations that share an operationId
	// (e.g., GET and POST split off the same handler by switch r.Method)
	// flips between runs depending on Go's randomized map iteration, which
	// then trips the determinism test.
	sortedPaths := slices.Sorted(maps.Keys(paths))
	for _, pathStr := range sortedPaths {
		pathItem := paths[pathStr]
		methodOps := []struct {
			method string
			op     *Operation
		}{
			{"GET", pathItem.Get},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
			{"DELETE", pathItem.Delete},
			{"PATCH", pathItem.Patch},
			{"OPTIONS", pathItem.Options},
			{"HEAD", pathItem.Head},
		}
		for _, mo := range methodOps {
			method, op := mo.method, mo.op
			if op != nil && op.OperationID != "" {
				// Find the matching route for full package info.
				var matchedRoute *RouteInfo
				for _, r := range routes {
					convertedPath := convertPathToOpenAPI(joinPaths(r.MountPath, r.Path))
					if convertedPath == pathStr && strings.EqualFold(r.Method, method) {
						matchedRoute = r
						break
					}
				}
				idRefs[op.OperationID] = append(idRefs[op.OperationID], opRef{
					path: pathStr, method: method, op: op, route: matchedRoute,
				})
			}
		}
	}

	// For each duplicate set, add parent package segments to disambiguate.
	for _, refs := range idRefs {
		if len(refs) <= 1 {
			continue
		}
		// Use the route's full package to progressively add segments.
		for i := range refs {
			if refs[i].route == nil {
				continue
			}
			pkg := refs[i].route.Package
			parts := strings.Split(pkg, "/")
			// Start from the second-to-last segment and add until unique.
			for depth := 2; depth <= len(parts); depth++ {
				candidate := strings.Join(parts[len(parts)-depth:], ".") + "." +
					strings.Replace(strings.Replace(refs[i].route.Function, TypeSep, ".", 1), pkg+".", "", 1)
				// Check uniqueness against all others.
				unique := true
				for j := range refs {
					if i != j && refs[j].op.OperationID == candidate {
						unique = false
						break
					}
				}
				if unique {
					refs[i].op.OperationID = candidate
					break
				}
			}
		}
	}
}

// MapMetadataToOpenAPI maps metadata to OpenAPI specification
func MapMetadataToOpenAPI(tree TrackerTreeInterface, cfg *APISpecConfig, genCfg GeneratorConfig) (*OpenAPISpec, error) {
	// Create extractor
	extractor := NewExtractor(tree, cfg)

	// Extract routes
	routes := extractor.ExtractRoutes()

	// Build paths and disambiguate operationIds if using short names
	paths := buildPathsFromRoutes(routes, cfg)
	if cfg != nil && cfg.UseShortNames() {
		disambiguateOperationIDs(paths, routes)
	}

	// Generate component schemas
	components := generateComponentSchemas(tree.GetMetadata(), cfg, routes)

	// Use Info from config if present, else fallback to GeneratorConfig
	var info Info
	if cfg != nil && (cfg.Info.Title != "" || cfg.Info.Description != "" || cfg.Info.Version != "") {
		info = cfg.Info
		if info.Title == "" {
			info.Title = genCfg.Title
		}
		if info.Version == "" {
			info.Version = genCfg.APIVersion
		}
	} else {
		info = Info{Title: genCfg.Title, Version: genCfg.APIVersion}
	}

	// Build OpenAPI spec
	spec := &OpenAPISpec{
		OpenAPI:      genCfg.OpenAPIVersion,
		Info:         info,
		Paths:        paths,
		Components:   &components,
		Servers:      cfg.Servers,
		Security:     cfg.Security,
		Tags:         cfg.Tags,
		ExternalDocs: cfg.ExternalDocs,
	}

	// Fill securitySchemes in components. Two sources, merged in this order
	// (config wins on conflict — explicit user declaration overrides
	// inference):
	//   1. Schemes detected by walking each route's call graph for
	//      Authorization-header reads (see detectSecuritySchemeForRoute).
	//   2. Schemes declared in apispec.yaml under `securitySchemes:`.
	if detected := collectDetectedSecuritySchemes(routes); len(detected) > 0 || len(cfg.SecuritySchemes) > 0 {
		if spec.Components == nil {
			spec.Components = &Components{}
		}
		merged := make(map[string]SecurityScheme, len(detected)+len(cfg.SecuritySchemes))
		for name, sch := range detected {
			merged[name] = sch
		}
		for name, sch := range cfg.SecuritySchemes {
			merged[name] = sch
		}
		spec.Components.SecuritySchemes = merged
	}

	// Post-process: shorten all $ref values to match shortened schema names.
	if cfg != nil && cfg.UseShortNames() && spec.Components != nil {
		shortenAllRefs(spec)
	}

	return spec, nil
}

// shortenAllRefs rewrites all $ref values in the spec to use shortened schema names
// that match the keys in components.Schemas.
//
//nolint:gocyclo // OpenAPI ref shortening across all spec components
func shortenAllRefs(spec *OpenAPISpec) {
	if spec.Components == nil {
		return
	}
	// Build a mapping from old (long) ref names to new (short) schema keys.
	// The old ref name is produced by schemaComponentNameReplacer (without
	// shortening), the new key is what generateSchemas stored (with shortening).
	// We build this by collecting all existing short keys and mapping any
	// ref that ends with the same type suffix to the short key.
	existingKeys := make(map[string]bool)
	for key := range spec.Components.Schemas {
		existingKeys[key] = true
	}

	// Walk the entire spec and shorten any $ref that points to #/components/schemas/...
	var shortenSchemaRef func(s *Schema)
	shortenSchemaRef = func(s *Schema) {
		if s == nil {
			return
		}
		if strings.HasPrefix(s.Ref, refComponentsSchemasPrefix) {
			oldName := strings.TrimPrefix(s.Ref, refComponentsSchemasPrefix)
			if !existingKeys[oldName] {
				// The ref target doesn't match a schema key — it's a long name.
				// Shorten it: extract the portion after the last "_" that starts
				// a package segment (same logic as shortenTypeName but on the
				// already-replaced name where "/" became "_").
				// Find the short key that the long name ends with.
				for shortKey := range existingKeys {
					if strings.HasSuffix(oldName, shortKey) {
						s.Ref = refComponentsSchemasPrefix + shortKey
						break
					}
				}
			}
		}
		shortenSchemaRef(s.Items)
		shortenSchemaRef(s.AdditionalProperties)
		for _, v := range s.Properties {
			shortenSchemaRef(v)
		}
		for _, v := range s.AllOf {
			shortenSchemaRef(v)
		}
		for _, v := range s.OneOf {
			shortenSchemaRef(v)
		}
		for _, v := range s.AnyOf {
			shortenSchemaRef(v)
		}
	}

	// Walk component schemas
	for _, schema := range spec.Components.Schemas {
		shortenSchemaRef(schema)
	}

	// Walk paths
	for _, pathItem := range spec.Paths {
		for _, op := range []*Operation{pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete, pathItem.Patch, pathItem.Options, pathItem.Head} {
			if op == nil {
				continue
			}
			if op.RequestBody != nil {
				for _, mt := range op.RequestBody.Content {
					shortenSchemaRef(mt.Schema)
				}
			}
			for _, resp := range op.Responses {
				for _, mt := range resp.Content {
					shortenSchemaRef(mt.Schema)
				}
			}
			for i := range op.Parameters {
				shortenSchemaRef(op.Parameters[i].Schema)
			}
		}
	}
}

// buildPathsFromRoutes builds OpenAPI paths from extracted routes
func buildPathsFromRoutes(routes []*RouteInfo, cfg *APISpecConfig) map[string]PathItem {
	paths := make(map[string]PathItem)

	for _, route := range routes {
		// Convert path to OpenAPI format
		openAPIPath := convertPathToOpenAPI(joinPaths(route.MountPath, route.Path))

		// Get or create path item
		pathItem, exists := paths[openAPIPath]
		if !exists {
			pathItem = PathItem{}
		}

		var pkg string

		if route.Package != "" {
			pkg = route.Package + "."
		}

		// Create operation
		operationID := pkg + strings.Replace(strings.ReplaceAll(route.Function, TypeSep, "."), pkg, "", 1)
		if cfg != nil && cfg.UseShortNames() {
			operationID = shortenOperationID(operationID)
		}

		// Extract summary and description from handler function's doc comment.
		summary, description := extractDocComment(route)
		if route.Summary != "" {
			summary = route.Summary // Override/config takes precedence
		}
		if route.Description != "" {
			description = route.Description // Override/config takes precedence
		}

		operation := &Operation{
			OperationID: operationID,
			Summary:     summary,
			Description: description,
			Tags:        route.Tags,
		}

		// Add request body if present
		if route.Request != nil {
			operation.RequestBody = &RequestBody{
				Content: map[string]MediaType{
					route.Request.ContentType: {
						Schema: route.Request.Schema,
					},
				},
				Required: route.Request.Required,
			}
		}

		// Add parameters (deduplicated and ensure all path params)
		if len(route.Params) > 0 {
			operation.Parameters = deduplicateParameters(route.Params)
		} else {
			operation.Parameters = nil
		}
		operation.Parameters = dropPhantomPathParams(openAPIPath, operation.Parameters)
		operation.Parameters = ensureAllPathParams(openAPIPath, operation.Parameters)

		// Attach security scheme + drop the now-redundant Authorization
		// header parameter. The scheme is detected during route extraction
		// (see detectSecuritySchemeForRoute); a single Components.securitySchemes
		// entry is shared across every operation referencing it (done outside
		// this loop — see attachSecuritySchemesToComponents).
		if route.SecurityScheme != nil {
			operation.Security = []SecurityRequirement{
				{route.SecurityScheme.Name: []string{}},
			}
			operation.Parameters = dropAuthorizationHeaderParam(operation.Parameters)
		}

		// Add responses
		operation.Responses = buildResponses(route.Response)

		// Set operation on path item
		setOperationOnPathItem(&pathItem, route.Method, operation)
		paths[openAPIPath] = pathItem
	}

	return paths
}

// collectDetectedSecuritySchemes deduplicates the SecurityScheme attached
// to each route into a single map keyed by scheme name. Routes that share
// the same scheme name share the same scheme definition in the output —
// SDK generators and humans get one canonical entry per kind of auth.
func collectDetectedSecuritySchemes(routes []*RouteInfo) map[string]SecurityScheme {
	out := map[string]SecurityScheme{}
	for _, r := range routes {
		if r.SecurityScheme == nil || r.SecurityScheme.Name == "" {
			continue
		}
		// First-write wins: detection is deterministic per (prefix → scheme)
		// so distinct routes that contribute the same name always carry the
		// same shape. Skipping later writes just avoids redundant work.
		if _, ok := out[r.SecurityScheme.Name]; ok {
			continue
		}
		out[r.SecurityScheme.Name] = r.SecurityScheme.Scheme
	}
	return out
}

// dropAuthorizationHeaderParam removes the canonical Authorization header
// parameter from a parameter list. Called when a security scheme is
// attached to the operation — the header is already implicit in the
// scheme reference, so leaving it as a parameter would double-document
// the same auth surface and confuse SDK generators.
func dropAuthorizationHeaderParam(params []Parameter) []Parameter {
	out := params[:0]
	for _, p := range params {
		if p.In == "header" && strings.EqualFold(p.Name, "Authorization") {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// pathTemplatePlaceholders returns the set of {name} placeholder names declared
// in an OpenAPI path template (e.g. "/users/{id}/posts/{postId}" yields id and
// postId). Mirrors the placeholder grammar ensureAllPathParams emits against.
func pathTemplatePlaceholders(openAPIPath string) map[string]struct{} {
	re := getCachedMapperRegex(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	out := make(map[string]struct{})
	for _, m := range re.FindAllStringSubmatch(openAPIPath, -1) {
		out[m[1]] = struct{}{}
	}
	return out
}

// dropPhantomPathParams removes in:path parameters whose name has no matching
// {placeholder} in the path template. An OpenAPI path parameter must appear in
// the URL template, so one that doesn't is invalid — and is almost always a
// static-analysis false positive: a string-literal index on a plain map
// (`fields["displayName"]`) misread as a router var on a route with no path
// placeholders (issue #35). Query, header, cookie, and form parameters pass
// through untouched, as do path params that do match a placeholder.
func dropPhantomPathParams(openAPIPath string, params []Parameter) []Parameter {
	if len(params) == 0 {
		return params
	}
	declared := pathTemplatePlaceholders(openAPIPath)
	out := params[:0]
	for _, p := range params {
		if p.In == "path" {
			if _, ok := declared[p.Name]; !ok {
				continue
			}
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ensureAllPathParams ensures all path parameters in the path are present in the parameters slice
func ensureAllPathParams(openAPIPath string, params []Parameter) []Parameter {
	paramMap := make(map[string]bool)
	for _, p := range params {
		if p.In == "path" {
			paramMap[p.Name] = true
		}
	}
	// Find all {param} in the path
	re := getCachedMapperRegex(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(openAPIPath, -1)
	for _, match := range matches {
		name := match[1]
		if !paramMap[name] {
			// Add path parameter inferred from the path template.
			// Many frameworks extract params at runtime (mux.Vars, chi.URLParam, etc.)
			// so not finding them in static analysis is expected.
			params = append(params, Parameter{
				Name:     name,
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
			})
		}
	}
	return params
}

// deduplicateParameters removes duplicate parameters by (name, in)
func deduplicateParameters(params []Parameter) []Parameter {
	seen := make(map[string]struct{})
	result := make([]Parameter, 0, len(params))
	for _, p := range params {
		key := p.Name + ":" + p.In
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}

// buildResponses builds OpenAPI responses from response info
//
//nolint:gocyclo // response building with status code and content type logic
func buildResponses(respInfo map[string]*ResponseInfo) map[string]Response {
	responses := make(map[string]Response)

	// Handle nil case - return default response indicating no response was found
	if len(respInfo) == 0 {
		responses["default"] = Response{
			Description: "Default response (no response found)",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Type: "object"},
				},
			},
		}
		return responses
	}

	// Pre-process: merge status-only responses (have status code but no body)
	// with body-only responses (have body but no explicit status code).
	// This handles the common pattern: w.WriteHeader(201); json.Encode(user)
	// where WriteHeader and Encode are captured as separate response entries.
	//
	// Issue #33 (per CodeRabbit review): exclude bodyless status entries
	// (1xx/204/304) from merge candidacy. RFC 9110 forbids a body on these
	// statuses, and `collectUsedTypesFromRoutes` (mapper.go below) walks
	// `res.BodyType` to populate components.schemas — so if a bodyless entry
	// were merged with a body-only schema here, the type would leak into
	// `components` even though the emitted response has no `content`.
	var bodyOnlyKeys []string
	var statusOnlyKeys []string
	for key, resp := range respInfo {
		if resp.StatusCode > 0 && !isBodylessStatusCode(resp.StatusCode) && resp.BodyType == "" && resp.Schema == nil {
			statusOnlyKeys = append(statusOnlyKeys, key)
		} else if resp.StatusCode < 0 && resp.BodyType != "" {
			bodyOnlyKeys = append(bodyOnlyKeys, key)
		}
	}
	if len(bodyOnlyKeys) > 0 && len(statusOnlyKeys) > 0 {
		// Assign the body schema from body-only responses to status-only responses.
		for _, sKey := range statusOnlyKeys {
			statusResp := respInfo[sKey]
			for _, bKey := range bodyOnlyKeys {
				bodyResp := respInfo[bKey]
				if bodyResp.Schema != nil {
					if statusResp.Schema == nil {
						statusResp.BodyType = bodyResp.BodyType
						statusResp.Schema = bodyResp.Schema
					} else if !schemasEqual(statusResp.Schema, bodyResp.Schema) {
						statusResp.AlternativeSchemas = append(statusResp.AlternativeSchemas, bodyResp.Schema)
					}
				}
			}
		}
		// Remove body-only entries that were merged
		for _, bKey := range bodyOnlyKeys {
			delete(respInfo, bKey)
		}
	}

	// Add responses (sorted for deterministic output)
	for _, statusCode := range slices.Sorted(maps.Keys(respInfo)) {
		resp := respInfo[statusCode]
		// Compute effective status as a local value so we don't mutate the
		// shared *ResponseInfo — multiple route entries can hold pointers to
		// the same ResponseInfo (e.g., when splitByConditionalMethods fans
		// out a switch-case handler into per-method routes), and mutating
		// the underlying StatusCode would corrupt subsequent rebuilds.
		effectiveStatus := resp.StatusCode
		if effectiveStatus < 0 && resp.BodyType != "" {
			effectiveStatus = http.StatusOK
			statusCode = "200"
		} else if effectiveStatus < 0 {
			statusCode = "default"
		}

		description := http.StatusText(effectiveStatus)
		if effectiveStatus < 0 || description == "" {
			description = "Status code could not be determined"
		}

		// Issue #33: bodyless status codes (1xx, 204, 304) must not carry a
		// Content map per RFC 9110. Emit description-only and skip body/schema
		// computation. The Content field's `omitempty` tag (openapi.go) ensures
		// the field is absent from the serialized output rather than present-
		// but-empty.
		if isBodylessStatusCode(effectiveStatus) {
			responses[statusCode] = Response{Description: description}
			continue
		}

		// If multiple schemas exist for this status code, wrap in oneOf.
		schema := resp.Schema
		if len(resp.AlternativeSchemas) > 0 && schema != nil {
			allSchemas := make([]*Schema, 0, 1+len(resp.AlternativeSchemas))
			allSchemas = append(allSchemas, schema)
			allSchemas = append(allSchemas, resp.AlternativeSchemas...)
			schema = &Schema{OneOf: allSchemas}
		}

		responses[statusCode] = Response{
			Description: description,
			Content: map[string]MediaType{
				resp.ContentType: {
					Schema: schema,
				},
			},
		}
	}

	return responses
}

// setOperationOnPathItem sets an operation on a path item based on HTTP method
func setOperationOnPathItem(item *PathItem, method string, op *Operation) {
	switch strings.ToUpper(method) {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "DELETE":
		item.Delete = op
	case "PATCH":
		item.Patch = op
	case "OPTIONS":
		item.Options = op
	case "HEAD":
		item.Head = op
	}
}

// qualifyElementType attaches a package prefix (e.g. "pkg.", already carrying
// its trailing dot) to a composite type's element — a map value or a struct
// field — applying it to the base element rather than any []/* wrapper, and
// leaving primitives or already-qualified types untouched. Nested wrappers are
// peeled fully, so [][]int stays [][]int (base int is primitive) and [][]Money
// becomes [][]pkg.Money.
//
//	"Money", "pkg."     → "pkg.Money"
//	"[]Money", "pkg."   → "[]pkg.Money"
//	"[][]Money", "pkg." → "[][]pkg.Money"
//	"*Money", "pkg."    → "*pkg.Money"
//	"[][]int", "pkg."   → "[][]int"        (primitive base)
//	"string", "pkg."    → "string"         (primitive)
//	"o.T", "pkg."       → "o.T"            (already qualified)
func qualifyElementType(valueType, pkgWithDot string) string {
	wrapper := ""
	rest := valueType
	for {
		switch {
		case strings.HasPrefix(rest, "[]"):
			wrapper += "[]"
			rest = rest[2:]
		case strings.HasPrefix(rest, "*"):
			wrapper += "*"
			rest = rest[1:]
		default:
			if rest == "" || metadata.IsPrimitiveType(rest) || strings.Contains(rest, ".") {
				return valueType
			}
			return wrapper + pkgWithDot + rest
		}
	}
}

// convertPathToOpenAPI converts a router-specific path template to the OpenAPI
// `{name}` form, per path segment so the different frameworks' syntaxes don't
// interfere:
//
//   - gin/echo colon params:            :id          → {id}
//   - gorilla/mux regex constraints:    {id:[0-9]+}  → {id}  (the pattern may
//     itself contain braces, e.g. {n:[0-9]{3}}; everything after the first ':'
//     is dropped, so nested quantifier braces don't matter)
//   - Go 1.22 ServeMux trailing wildcard {path...}   → {path}
//   - Go 1.22 ServeMux end-of-path anchor {$}        → (removed; carries no
//     path segment)
func convertPathToOpenAPI(path string) string {
	if path == "" {
		return path
	}
	segments := strings.Split(path, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		switch {
		case seg == "{$}":
			// End-of-path anchor — contributes no path segment.
			continue
		case strings.HasPrefix(seg, ":") && len(seg) > 1:
			out = append(out, "{"+seg[1:]+"}")
		case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"):
			inner := seg[1 : len(seg)-1]
			if i := strings.IndexByte(inner, ':'); i >= 0 {
				inner = inner[:i] // drop a gorilla/mux regex constraint
			}
			inner = strings.TrimSuffix(inner, "...") // drop a ServeMux wildcard marker
			out = append(out, "{"+inner+"}")
		default:
			out = append(out, seg)
		}
	}
	result := strings.Join(out, "/")
	if result == "" {
		return "/"
	}
	return result
}

// generateComponentSchemas generates component schemas from metadata
func generateComponentSchemas(meta *metadata.Metadata, cfg *APISpecConfig, routes []*RouteInfo) Components {
	components := Components{
		Schemas: make(map[string]*Schema),
	}

	// Collect all types used in routes
	usedTypes := collectUsedTypesFromRoutes(routes)

	// Generate schemas for used types
	generateSchemas(usedTypes, cfg, components, meta)

	return components
}

// fieldTypeString returns the type string the schema pipeline works from,
// preferring the lossless field.TypeRef (which preserves fixed-array lengths,
// full package paths, multi-parameter generics, and interface/struct shapes)
// over getTypeName's lossy flattened field.Type. Falls back to field.Type when
// no TypeRef was built at analysis time. This is the field-path step of retiring
// getTypeName from schema derivation (T011).
func fieldTypeString(field metadata.Field, meta *metadata.Metadata) string {
	if field.TypeRef != nil {
		return field.TypeRef.String()
	}
	return getStringFromPool(meta, field.Type)
}

// majorVersionInPath matches a Go module major-version path segment ("/v2.",
// "/v3.", …) sitting between an import path and the type name.
var majorVersionInPath = regexp.MustCompile(`/v[0-9]+\.`)

// stripMajorVersion removes the Go module major-version segment from an
// import-path-qualified type string so a versioned external type names the same
// as its unversioned form: "github.com/gofiber/fiber/v2.Map" ->
// "github.com/gofiber/fiber.Map". This matches the historical naming (which
// dropped the segment) and the way external-type config Names spell it. A no-op
// for strings without a "/vN." segment.
func stripMajorVersion(typeName string) string {
	if !strings.Contains(typeName, "/v") {
		return typeName
	}
	return majorVersionInPath.ReplaceAllString(typeName, ".")
}

// schemaName applies the standard name replacer and optionally shortens the type name.
func schemaName(typeName string, cfg *APISpecConfig) string {
	typeName = stripMajorVersion(typeName)
	if cfg != nil && cfg.UseShortNames() {
		return schemaComponentNameReplacer.Replace(shortenTypeName(typeName))
	}
	return schemaComponentNameReplacer.Replace(typeName)
}

func generateSchemas(usedTypes map[string]*Schema, cfg *APISpecConfig, components Components, meta *metadata.Metadata) {
	for _, typeName := range slices.Sorted(maps.Keys(usedTypes)) {
		// Check external types
		if cfg != nil {
			for _, externalType := range cfg.ExternalTypes {
				if externalType.Name == strings.ReplaceAll(typeName, TypeSep, ".") {
					components.Schemas[schemaName(typeName, cfg)] = externalType.OpenAPIType
					continue
				}
			}
		}

		// Find the type in metadata
		typs := findTypesInMetadata(meta, typeName)
		if len(typs) == 0 || typs[typeName] == nil {
			continue
		}

		// Generate schema based on type kind
		for key, typ := range typs {
			if typ == nil {
				continue
			}
			schema, schemas := generateSchemaFromType(usedTypes, key, typ, meta, cfg, nil)
			if schema != nil {
				components.Schemas[schemaName(key, cfg)] = schema
			}
			for schemaKey, newSchema := range schemas {
				components.Schemas[schemaName(schemaKey, cfg)] = newSchema
			}
		}
	}
}

// collectUsedTypesFromRoutes collects all types used in routes
func collectUsedTypesFromRoutes(routes []*RouteInfo) map[string]*Schema {
	usedTypes := make(map[string]*Schema)

	for _, route := range routes {
		// Add request body types
		if route.Request != nil && route.Request.BodyType != "" {
			// addTypeAndDependenciesWithMetadata(route.Request.BodyType, usedTypes, meta, cfg)
			markUsedType(usedTypes, route.Request.BodyType, nil)
		}

		// Add response types (sorted for determinism)
		for _, key := range slices.Sorted(maps.Keys(route.Response)) {
			res := route.Response[key]
			if res.BodyType != "" {
				markUsedType(usedTypes, res.BodyType, nil)
			}
		}

		// Add parameter types
		for _, param := range route.Params {
			if param.Schema != nil && param.Schema.Ref != "" {
				// Extract type name from ref like "#/components/schemas/TypeName"
				refParts := strings.Split(param.Schema.Ref, "/")
				if len(refParts) > 0 {
					typeName := refParts[len(refParts)-1]
					// addTypeAndDependenciesWithMetadata(typeName, usedTypes, meta, cfg)
					markUsedType(usedTypes, typeName, nil)
				}
			}
		}

		for key, usedType := range route.UsedTypes {
			markUsedType(usedTypes, key, usedType)
		}
	}

	return usedTypes
}

// findTypesInMetadata finds a type in metadata
func findTypesInMetadata(meta *metadata.Metadata, typeName string) map[string]*metadata.Type {
	metaTypes := map[string]*metadata.Type{}

	// Skip primitive types - they don't need to be looked up in metadata
	if metadata.IsPrimitiveType(typeName) {
		return nil
	}

	// Guard against nil metadata
	if meta == nil {
		return nil
	}

	typeParts := TypeParts(typeName)

	// The base type only. The type arguments of a generic instantiation
	// (Pair[string,int]) are NOT registered here: each parameter-typed field is
	// resolved by name in generateStructSchema (US2), so the arguments surface as
	// real component references where they are actually used. The former generic
	// branch registered them positionally (T/U/V) — emitting orphan "T-string"
	// components for primitive arguments and nothing for struct arguments
	// (typeByName of the positional placeholder was always nil). It served no live
	// consumer and is removed.
	if typeName != "" {
		metaTypes[typeName] = typeByName(typeParts, meta)
	}

	return metaTypes
}

func typeByName(typeParts Parts, meta *metadata.Metadata) *metadata.Type {
	if meta == nil {
		return nil
	}

	if typeParts.PkgName != "" && typeParts.TypeName != "" {
		pkgName := typeParts.PkgName

		if pkg, exists := meta.Packages[pkgName]; exists {
			for _, file := range pkg.Files {
				if typ, exists := file.Types[typeParts.TypeName]; exists {
					return typ
				}
			}
		}
	}

	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			if typ, exists := file.Types[typeParts.TypeName]; exists {
				return typ
			}
		}
	}
	return nil
}

// bodyTypeFromMetadataRef returns the canonical type string for a body/param
// whose TypeRef leaf (under any pointer/slice/array/map wrappers) names a concrete
// type — so it references the same component a field of that type would:
//
//   - a type in metadata → ref.String() (the fully-qualified form the field path
//     uses);
//   - a configured external type (cfg.ExternalTypes) → ref.String() with the Go
//     module major-version segment stripped, matching how the config Name and the
//     existing string path spell it (e.g. github.com/gofiber/fiber/v2.Map ->
//     github.com/gofiber/fiber.Map).
//
// It returns "" for unresolved leaves and generic instantiations, which the string
// path still handles. This is the alignment seam for migrating the body/param type
// off the string reconstruction (T009/T011).
func bodyTypeFromMetadataRef(ref *metadata.TypeRef, meta *metadata.Metadata, cfg *APISpecConfig) string {
	leaf := ref
	for leaf != nil && (leaf.Kind == metadata.RefPointer || leaf.Kind == metadata.RefSlice ||
		leaf.Kind == metadata.RefArray || leaf.Kind == metadata.RefMap) {
		leaf = leaf.Elem
	}
	if leaf == nil || leaf.Kind != metadata.RefNamed {
		return ""
	}
	// In-metadata named leaf — including a generic instantiation, whose base type
	// resolves by Pkg/Name and whose args ride along in ref.String() for the
	// string path's substitution.
	if typeByRef(leaf, meta) != nil {
		return ref.String()
	}
	// A configured external type; generics are not external-configured.
	if cfg != nil && len(leaf.Args) == 0 {
		leafName := stripMajorVersion(leaf.String())
		for _, et := range cfg.ExternalTypes {
			if et.Name == leafName {
				return stripMajorVersion(ref.String())
			}
		}
	}
	return ""
}

// typeByRef resolves a named TypeRef to its metadata.Type using the ref's own
// Pkg/Name fields — the tree counterpart to typeByName(TypeParts(string)), with
// no string re-parsing. It keeps typeByName's two-step strategy: try the
// qualified package first, then fall back to a name-only search across all
// packages (covering types whose ref.Pkg does not match a metadata package key,
// e.g. import-path vs module-relative differences). Returns nil for non-named
// refs and unresolved names.
func typeByRef(ref *metadata.TypeRef, meta *metadata.Metadata) *metadata.Type {
	if meta == nil || ref == nil || ref.Kind != metadata.RefNamed || ref.Name == "" {
		return nil
	}
	if ref.Pkg != "" {
		if pkg, exists := meta.Packages[ref.Pkg]; exists {
			for _, file := range pkg.Files {
				if typ, exists := file.Types[ref.Name]; exists {
					return typ
				}
			}
		}
	}
	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			if typ, exists := file.Types[ref.Name]; exists {
				return typ
			}
		}
	}
	return nil
}

type Parts struct {
	PkgName      string
	TypeName     string
	GenericTypes []string
}

func TypeParts(typeName string) Parts {
	parts := Parts{}

	// Handle bracket-style generic types first (e.g., "APIResponse[pkg.User]")
	// Extract base type and generic params before any separator splitting.
	if bracketIdx := strings.Index(typeName, "["); bracketIdx > 0 && !strings.Contains(typeName[:bracketIdx], TypeSep) {
		baseName := typeName[:bracketIdx]
		genericParam := typeName[bracketIdx+1:]

		genericParam = strings.TrimSuffix(genericParam, "]")

		// Split base name into pkg.Type
		lastDot := strings.LastIndex(baseName, defaultSep)
		if lastDot > 0 {
			parts.PkgName = baseName[:lastDot]
			parts.TypeName = baseName[lastDot+1:]
		} else {
			parts.TypeName = baseName
		}
		// Format: "ParamName ConcreteType" — use single-letter type params
		// (T, U, V) for positional parameters since we don't have the param
		// names from the type string. generateStructSchema matches field types
		// against these to substitute.
		params := strings.Split(genericParam, ",")
		paramNames := []string{"T", "U", "V", "W", "X", "Y", "Z"}
		for i, param := range params {
			param = strings.TrimSpace(param)
			name := paramNames[0]
			if i < len(paramNames) {
				name = paramNames[i]
			}
			parts.GenericTypes = append(parts.GenericTypes, name+" "+param)
		}
		return parts
	}

	typeParts := strings.Split(typeName, TypeSep)

	if len(typeParts) == 1 {
		lastSep := strings.LastIndex(typeName, defaultSep)
		if lastSep > 0 {
			parts.PkgName = typeName[:lastSep]
			parts.TypeName = typeName[lastSep+1:]
		} else {
			parts.TypeName = typeName
		}
	} else if len(typeParts) > 1 {
		parts.PkgName = typeParts[0]
		parts.TypeName = typeParts[1]
		parts.GenericTypes = typeParts[2:]
	}

	if len(typeParts) == 2 && strings.Contains(typeParts[1], "[") {
		genericParts := strings.Split(typeParts[1], "[")
		if len(genericParts) > 1 {
			parts.TypeName = genericParts[0]
			parts.GenericTypes = []string{genericParts[1][:len(genericParts[1])-1]}
		}
	}

	parts.PkgName = strings.TrimPrefix(parts.PkgName, "*")
	parts.PkgName = strings.TrimPrefix(parts.PkgName, "[]")

	return parts
}

const generateSchemaFromTypeKey = "generateSchemaFromType"

// generateSchemaFromType generates an OpenAPI schema from a metadata type
func generateSchemaFromType(usedTypes map[string]*Schema, key string, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	if visitedTypes == nil {
		visitedTypes = map[string]bool{}
	}

	derivedKey := strings.TrimPrefix(key, "*")
	if visitedTypes[key+generateSchemaFromTypeKey] && canAddRefSchemaForType(derivedKey) {
		return addRefSchemaForType(key), schemas
	}
	visitedTypes[key+generateSchemaFromTypeKey] = true

	if usedTypes[derivedKey] != nil && canAddRefSchemaForType(derivedKey) {
		schemas[derivedKey] = usedTypes[derivedKey]
		return addRefSchemaForType(derivedKey), schemas
	}

	// Check external types
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == strings.ReplaceAll(derivedKey, TypeSep, ".") {
				markUsedType(usedTypes, derivedKey, externalType.OpenAPIType)
				return externalType.OpenAPIType, schemas
			}
		}
	}

	// Get type kind from string pool
	kind := getStringFromPool(meta, typ.Kind)

	var schema *Schema
	var newSchemas map[string]*Schema

	switch kind {
	case "struct":
		schema, newSchemas = generateStructSchema(usedTypes, key, typ, meta, cfg, visitedTypes)
	case "interface":
		schema = generateInterfaceSchema()
	case "alias":
		schema, newSchemas = generateAliasSchema(usedTypes, typ, meta, cfg, visitedTypes)
	default:
		schema = &Schema{Type: "object"}
	}

	markUsedType(usedTypes, key, schema)

	maps.Copy(schemas, newSchemas)

	return schema, schemas
}

// generateStructSchema generates a schema for a struct type
//
//nolint:gocyclo // struct schema generation with field tags and validation
func generateStructSchema(usedTypes map[string]*Schema, key string, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}

	keyParts := TypeParts(key)
	genericTypes := map[string]string{}

	if len(keyParts.GenericTypes) > 0 {
		for i, part := range keyParts.GenericTypes {
			genericType := strings.Split(part, " ")
			// Bind the concrete argument to the DECLARED type-parameter name
			// (typ.TypeParams), not the positional T/U/V placeholder TypeParts
			// assigns from the instantiation string — so Pair[K,V] substitutes K and
			// V, not T and U (decision D3). On a length mismatch (fewer declared
			// names than args) fall back to the placeholder so the common prefix
			// still binds (FR-009, T019).
			name := genericType[0]
			if i < len(typ.TypeParams) {
				name = typ.TypeParams[i]
			}
			if len(genericType) >= 2 {
				// Map type parameter name to concrete type (e.g., "T" → "pkg.User")
				genericTypes[name] = genericType[1]
			} else {
				genericTypes[name] = strings.ReplaceAll(part, " ", "-")
			}
		}
	}

	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
		Required:   []string{},
	}

	pkgName := getStringFromPool(meta, typ.Pkg)

	// structLevelTag accumulates the struct-scope overrides (minProperties,
	// anyOf) declared on the blank `_` marker field. We can't apply it
	// inline because the marker is iterated alongside real fields and the
	// override must land on the parent schema, not on a property.
	var structLevelTag *apispecTag

	for _, field := range typ.Fields {
		fieldName := getStringFromPool(meta, field.Name)
		fieldType := fieldTypeString(field, meta)

		// `_ struct{} `apispec:"..."`` is the convention for struct-level
		// hints — it never serializes (zero-sized + unexported) but its tag
		// gives us a place to attach minProperties/anyOf without inventing a
		// new annotation language. Skip it from Properties/Required and read
		// the tag for later application.
		//
		// Multiple `_` markers merge rather than overwrite: scalar keys take
		// last-write-wins (matching how Go itself resolves duplicate field
		// declarations) and list keys (AnyOf) accumulate. A user splitting
		// hints across markers shouldn't silently lose earlier ones.
		if fieldName == "_" {
			structLevelTag = mergeStructLevelTag(structLevelTag, parseAPISpecTag(getStringFromPool(meta, field.Tag)))
			continue
		}

		if genericType, ok := genericTypes[fieldType]; ok {
			fieldType = genericType
		} else {
			// Substitute type params inside container types: []T, *T, map[K]V
			// Only match when param is a complete token (not substring of another type).
			for param, concrete := range genericTypes {
				// Check common container patterns
				if strings.HasPrefix(fieldType, "[]"+param) {
					fieldType = "[]" + concrete + fieldType[2+len(param):]
					break
				}
				if strings.HasPrefix(fieldType, "*"+param) {
					fieldType = "*" + concrete + fieldType[1+len(param):]
					break
				}
				if strings.HasPrefix(fieldType, "map["+param+"]") {
					fieldType = "map[" + concrete + "]" + fieldType[4+len(param):]
					break
				}
				// map value: map[X]T
				if strings.Contains(fieldType, "]"+param) && strings.HasPrefix(fieldType, "map[") {
					fieldType = strings.Replace(fieldType, "]"+param, "]"+concrete, 1)
					break
				}
			}
		}

		// Check if fieldType is an alias/enum and resolve to underlying type
		// But don't resolve array or map types as we need the original type for enum detection
		if !strings.HasPrefix(fieldType, "[]") && !strings.Contains(fieldType, "map[") {
			if resolvedType := resolveUnderlyingType(fieldType, meta); resolvedType != "" {
				fieldType = resolvedType
			}
		}

		// Extract JSON tag if present
		jsonName := extractJSONName(getStringFromPool(meta, field.Tag))
		if jsonName != "" {
			fieldName = jsonName
		}

		// Extract validation constraints from struct tag
		validationConstraints := extractValidationConstraints(getStringFromPool(meta, field.Tag))

		// Generate schema for field type
		var fieldSchema *Schema
		var newSchemas map[string]*Schema

		if field.NestedType != nil {
			// Handle nested inline struct type
			fieldOriginalType := getStringFromPool(meta, field.NestedType.Name)

			fieldSchema, newSchemas = generateSchemaFromType(usedTypes, fieldOriginalType, field.NestedType, meta, cfg, visitedTypes)
			if fieldSchema == nil {
				fieldSchema = newSchemas[fieldOriginalType]
			}

			maps.Copy(schemas, newSchemas)

			// When the inline struct is the element of a slice/array field
			// ([]struct{...}, [N]struct{...}), NestedType holds the element type;
			// wrap the object back into an array so the items describe its
			// properties. A fixed array also carries its length (decision D5/D7).
			if ref := field.TypeRef; ref != nil && fieldSchema != nil &&
				(ref.Kind == metadata.RefSlice || ref.Kind == metadata.RefArray) {
				fieldSchema = &Schema{Type: "array", Items: fieldSchema}
				setFixedArrayLen(fieldSchema, ref)
			}
		} else {
			isPrimitive := metadata.IsPrimitiveType(fieldType)

			if !isPrimitive && !strings.Contains(fieldType, ".") {
				// Qualify the base element with the package, peeling every
				// []/* wrapper so nested slices like [][]Cell become
				// [][]pkg.Cell rather than []pkg.[]Cell (then pkg._Cell), and
				// primitive-based ones like [][]int are left untouched.
				fieldType = qualifyElementType(fieldType, pkgName+".")
			}

			derivedFieldType := strings.TrimPrefix(fieldType, "*")
			// Check if this field type already exists in usedTypes
			if bodySchema, ok := usedTypes[derivedFieldType]; !isPrimitive && ok {
				// Create a reference to the existing schema
				fieldSchema = addRefSchemaForType(derivedFieldType)

				if bodySchema == nil {
					var newBodySchemas map[string]*Schema

					bodySchema, newBodySchemas = schemaForType(usedTypes, fieldType, nil, meta, cfg, visitedTypes)
					maps.Copy(schemas, newBodySchemas)
				}
				schemas[derivedFieldType] = bodySchema
				markUsedType(usedTypes, derivedFieldType, bodySchema)
			} else {
				fieldSchema, newSchemas = schemaForType(usedTypes, derivedFieldType, field.TypeRef, meta, cfg, visitedTypes)
				if canAddRefSchemaForType(derivedFieldType) {
					schemas[derivedFieldType] = fieldSchema
					fieldSchema = addRefSchemaForType(derivedFieldType)
				}

				maps.Copy(schemas, newSchemas)
			}
		}

		// Required inference composes (issue #48): a field is required by Go
		// default when its json tag has no omitempty (the zero value is always
		// serialized), and a `validate:"required"` constraint forces it. A
		// validate tag *without* required (e.g. `oneof=...`, `min=0`) must add
		// its constraints without suppressing the omitempty-based inference —
		// the previous either/else silently dropped such fields from required.
		fieldTag := getStringFromPool(meta, field.Tag)
		required := !hasOmitempty(fieldTag)
		if validationConstraints != nil {
			applyValidationConstraints(fieldSchema, validationConstraints)
			if validationConstraints.Required {
				required = true
			}
		}
		if required {
			schema.Required = append(schema.Required, fieldName)
		}

		// `apispec:"..."` is the user's explicit override — applied last so it
		// wins over both validator constraints and Go-type-derived defaults.
		applyAPISpecTag(fieldSchema, parseAPISpecTag(getStringFromPool(meta, field.Tag)))

		// Detect and apply enum values from constants if no enum was specified in tags
		// Only apply enum detection for custom types (not built-in types)
		if fieldSchema != nil && len(fieldSchema.Enum) == 0 {
			// Use the original field type before resolution for enum detection
			originalFieldType := fieldTypeString(field, meta)

			// Only detect enums for custom types, not built-in types like string, int, etc.
			if !metadata.IsPrimitiveType(originalFieldType) {
				if enumValues := detectEnumFromConstants(originalFieldType, pkgName, meta); len(enumValues) > 0 {
					switch fieldSchema.Type {
					case "array":
						fieldSchema.Items.Enum = enumValues
					case "object":
						if fieldSchema.AdditionalProperties != nil {
							fieldSchema.AdditionalProperties.Enum = enumValues
						}
					default:
						fieldSchema.Enum = enumValues
					}
				}
			}
		}

		schema.Properties[fieldName] = fieldSchema
	}

	// Promote anonymously embedded structs' fields (Go field promotion; JSON
	// marshals them flat). Runs after the own-field loop so own fields shadow
	// promoted ones, matching Go's selector resolution.
	promoteEmbeddedFields(schema, typ, meta, cfg, visitedTypes)

	// Struct-level overrides land last so they sit on the fully-built schema
	// — minProperties is independent of the per-field passes above, and the
	// anyOf array references property names that are now finalized.
	applyStructLevelAPISpecTag(schema, structLevelTag)

	return schema, schemas
}

// promoteEmbeddedFields flattens the fields of a struct's anonymously embedded
// types into schema, mirroring Go's field promotion and JSON's flat marshaling.
// Fields already present on schema (own fields, or an earlier embed) win, so
// shadowing matches Go's selector rules; transitive embedding resolves because
// the embedded type's own schema is itself promoted.
//
// The embedded type's schema is generated in isolation (fresh maps) and only
// its properties/required are copied out — the embed must NOT be registered as
// a component schema (Go embedding has no separate JSON object), and isolation
// keeps the result independent of map-iteration order.
func promoteEmbeddedFields(schema *Schema, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) {
	for i, embedIdx := range typ.Embeds {
		embedType := strings.TrimPrefix(getStringFromPool(meta, embedIdx), "*")
		// Prefer the lossless EmbedRef over the getTypeName string, mirroring the
		// field path (T009/T011).
		if i < len(typ.EmbedRefs) {
			if ref := typ.EmbedRefs[i]; ref != nil {
				embedType = strings.TrimPrefix(ref.String(), "*")
			}
		}
		et := typeByName(TypeParts(embedType), meta)
		if et == nil || getStringFromPool(meta, et.Kind) != "struct" {
			continue
		}
		es, _ := generateStructSchema(map[string]*Schema{}, embedType, et, meta, cfg, visitedTypes)
		if es == nil {
			continue
		}
		for name, prop := range es.Properties {
			if _, exists := schema.Properties[name]; !exists {
				schema.Properties[name] = prop
			}
		}
		for _, req := range es.Required {
			if schema.Properties[req] != nil && !slices.Contains(schema.Required, req) {
				schema.Required = append(schema.Required, req)
			}
		}
	}
}

// generateInterfaceSchema generates a schema for an interface type
func generateInterfaceSchema() *Schema {
	// For interfaces, we'll create a generic object schema
	// In a more sophisticated implementation, you might analyze interface methods
	return &Schema{
		Type: "object",
	}
}

// generateAliasSchema generates a schema for an alias type
func generateAliasSchema(usedTypes map[string]*Schema, typ *metadata.Type, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	underlyingType := getStringFromPool(meta, typ.Target)

	// Get the original type name for enum detection
	originalTypeName := getStringFromPool(meta, typ.Name)

	// Generate the base schema from underlying type
	schema, schemas := schemaForType(usedTypes, underlyingType, nil, meta, cfg, visitedTypes)

	// If the underlying type is a primitive (like string), try to detect enum values
	if schema != nil && metadata.IsPrimitiveType(underlyingType) {
		// Extract package name for enum detection
		pkgName := ""
		if typeParts := TypeParts(originalTypeName); typeParts.PkgName != "" {
			pkgName = typeParts.PkgName
		}

		// Detect enum values for this alias type using the original type name
		if enumValues := detectEnumFromConstants(originalTypeName, pkgName, meta); len(enumValues) > 0 {
			// Apply enum values to the schema
			schema.Enum = enumValues
		}
	}

	return schema, schemas
}

// resolveUnderlyingType resolves the underlying type for alias/enum types
func resolveUnderlyingType(typeName string, meta *metadata.Metadata) string {
	if meta == nil {
		return ""
	}

	var hasArrayPrefix, hasMapPrefix, hasSlicePrefix, hasStarPrefix bool

	if after, ok := strings.CutPrefix(typeName, "[]"); ok {
		typeName = after
		hasArrayPrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "map["); ok {
		typeName = after
		hasMapPrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "[]"); ok {
		typeName = after
		hasSlicePrefix = true
	}
	if after, ok := strings.CutPrefix(typeName, "*"); ok {
		typeName = after
		hasStarPrefix = true
	}

	// Find the type in metadata
	typs := findTypesInMetadata(meta, typeName)
	if len(typs) == 0 {
		return ""
	}

	for _, typ := range typs {
		if typ == nil {
			continue
		}

		kind := getStringFromPool(meta, typ.Kind)
		if kind == "alias" {
			// Return the underlying type for alias types (like enums)
			underlyingType := getStringFromPool(meta, typ.Target)
			if hasArrayPrefix {
				return "[]" + underlyingType
			}
			if hasMapPrefix {
				return "map[" + underlyingType + "]" + underlyingType
			}
			if hasSlicePrefix {
				return "[]" + underlyingType
			}
			if hasStarPrefix {
				return "*" + underlyingType
			}
			return underlyingType
		}
	}

	return ""
}

func markUsedType(usedTypes map[string]*Schema, typeName string, markValue *Schema) bool {
	if usedTypes[typeName] != nil {
		return true
	}

	usedTypes[typeName] = markValue

	// Handle pointer types by dereferencing them
	if strings.HasPrefix(typeName, "*") {
		dereferencedType := strings.TrimSpace(typeName[1:])
		// Also add the dereferenced type to used types
		if usedTypes[dereferencedType] == nil {
			usedTypes[dereferencedType] = markValue
		}
	}
	return false
}

// getStringFromPool gets a string from the string pool
func getStringFromPool(meta *metadata.Metadata, idx int) string {
	if meta.StringPool == nil {
		return ""
	}
	return meta.StringPool.GetString(idx)
}

// hasOmitempty checks if the json tag contains the omitempty option.
func hasOmitempty(tag string) bool {
	if !strings.Contains(tag, "json:") {
		return false
	}
	parts := strings.Split(tag, "json:")
	if len(parts) < 2 {
		return false
	}
	jsonPart := strings.Split(parts[1], " ")[0]
	jsonPart = strings.Trim(jsonPart, "\"")
	return strings.Contains(jsonPart, "omitempty")
}

// extractJSONName extracts JSON name from a struct tag
func extractJSONName(tag string) string {
	if tag == "" {
		return ""
	}

	// Simple JSON tag extraction
	// In a more sophisticated implementation, you would use reflection or a proper parser
	if strings.Contains(tag, "json:") {
		parts := strings.Split(tag, "json:")
		if len(parts) > 1 {
			jsonPart := strings.Split(parts[1], " ")[0]
			jsonName := strings.Trim(jsonPart, "\"")
			// Remove ,omitempty and other options
			if idx := strings.Index(jsonName, ","); idx != -1 {
				jsonName = jsonName[:idx]
			}
			if jsonName != "" && jsonName != "-" {
				return jsonName
			}
		}
	}

	return ""
}

// ValidationConstraints represents validation constraints extracted from struct tags
type ValidationConstraints struct {
	MinLength *int
	MaxLength *int
	Min       *float64
	Max       *float64
	Format    string
	Pattern   string
	Required  bool
	Dive      bool // When true, constraints apply to array items, not the array itself
	Enum      []interface{}
}

// validationFormatRules maps validation rule names to OpenAPI format strings
var validationFormatRules = map[string]string{
	"email": "email",
	"url":   "uri",
	"uuid":  "uuid",
}

// validationPatternRules maps validation rule names to regex patterns
var validationPatternRules = map[string]string{ //nolint:gosec // not credentials, these are validation regex patterns
	"alpha": `^[a-zA-Z]+$`, "alphanum": `^[a-zA-Z0-9]+$`, "numeric": `^[0-9]+$`,
	"alphaunicode": `^\p{L}+$`, "alphanumunicode": `^[\p{L}\p{N}]+$`,
	"hexadecimal": `^[0-9a-fA-F]+$`, "hexcolor": `^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`,
	"rgb":  `^rgb\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*\)$`,
	"rgba": `^rgba\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]*(?:\.[0-9]+)?)\s*\)$`,
	"hsl":  `^hsl\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]{1,3})%\s*\)$`,
	"hsla": `^hsla\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]{1,3})%\s*,\s*([0-9]*(?:\.[0-9]+)?)\s*\)$`,
	"json": `^[\s\S]*$`, "base64": `^[A-Za-z0-9+/]*={0,2}$`, "base64url": `^[A-Za-z0-9_-]*$`,
	"datetime": `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`,
	"date":     `^\d{4}-\d{2}-\d{2}$`, "time": `^\d{2}:\d{2}:\d{2}$`,
	"ip":          `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`,
	"ipv4":        `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`,
	"ipv6":        `^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`,
	"cidr":        `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(?:[0-9]|[1-2][0-9]|3[0-2])$`,
	"cidrv4":      `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(?:[0-9]|[1-2][0-9]|3[0-2])$`,
	"cidrv6":      `^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\/(?:[0-9]|[1-9][0-9]|1[0-2][0-8])$`,
	"tcp_addr":    `^[a-zA-Z0-9.-]+:\d+$`,
	"tcp4_addr":   `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$`,
	"tcp6_addr":   `^\[(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\]:\d+$`,
	"udp_addr":    `^[a-zA-Z0-9.-]+:\d+$`,
	"udp4_addr":   `^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$`,
	"udp6_addr":   `^\[(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\]:\d+$`,
	"unix_addr":   `^[a-zA-Z0-9._/-]+$`,
	"mac":         `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`,
	"hostname":    `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`,
	"fqdn":        `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\.$`,
	"isbn":        `^(?:ISBN(?:-1[03])?:? )?(?=[0-9X]{10}$|(?=(?:[0-9]+[- ]){3})[- 0-9X]{13}$|97[89][0-9]{10}$|(?=(?:[0-9]+[- ]){4})[- 0-9]{17}$)(?:97[89][- ]?)?[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9X]$`,
	"isbn10":      `^(?:ISBN(?:-10)?:? )?(?=[0-9X]{10}$|(?=(?:[0-9]+[- ]){3})[- 0-9X]{13}$)[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9X]$`,
	"isbn13":      `^(?:ISBN(?:-13)?:? )?(?=[0-9]{13}$|(?=(?:[0-9]+[- ]){4})[- 0-9]{17}$)97[89][- ]?[0-9]{1,5}[- ]?[0-9]+[- ]?[0-9]+[- ]?[0-9]$`,
	"issn":        `^[0-9]{4}-[0-9]{3}[0-9X]$`,
	"uuid3":       `^[0-9a-f]{8}-[0-9a-f]{4}-3[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`,
	"uuid4":       `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	"uuid5":       `^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	"ulid":        `^[0-9A-HJKMNP-TV-Z]{26}$`,
	"ascii":       `^[\x00-\x7F]*$`,
	"printascii":  `^[\x20-\x7E]*$`,
	"multibyte":   `^[\x00-\x7F]*$`,
	"datauri":     `^data:([a-z]+\/[a-z0-9\-\+]+(;[a-z0-9\-\+]+\=[a-z0-9\-\+]+)?)?(;base64)?,([a-z0-9\!\$\&\'\(\)\*\+\,\;\=\-\.\_\~\:\@\/\?\%\s]*)$`,
	"latitude":    `^[-+]?([1-8]?\d(\.\d+)?|90(\.0+)?)$`,
	"longitude":   `^[-+]?(180(\.0+)?|((1[0-7]\d)|([1-9]?\d))(\.\d+)?)$`,
	"ssn":         `^\d{3}-?\d{2}-?\d{4}$`,
	"credit_card": `^(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})$`,
	"mongodb":     `^[0-9a-fA-F]{24}$`,
	"cron":        `^(\*|([0-5]?\d)) (\*|([01]?\d|2[0-3])) (\*|([012]?\d|3[01])) (\*|([0]?\d|1[0-2])) (\*|([0-6]))$`,
}

// applyBoundRule handles the value-bound validator rules min/max/gte/lte,
// reporting whether rule matched one. The bound is stored numerically; on a
// string/slice field applyValidationConstraints reinterprets it as a length.
func applyBoundRule(rule string, constraints *ValidationConstraints) bool {
	var dst **float64
	switch {
	case strings.HasPrefix(rule, "min="), strings.HasPrefix(rule, "gte="):
		dst = &constraints.Min
	case strings.HasPrefix(rule, "max="), strings.HasPrefix(rule, "lte="):
		dst = &constraints.Max
	default:
		return false
	}
	if val, err := strconv.ParseFloat(rule[strings.IndexByte(rule, '=')+1:], 64); err == nil {
		*dst = &val
	}
	return true
}

// applyValidationRule applies a single validation rule to constraints
func applyValidationRule(rule string, constraints *ValidationConstraints) {
	switch {
	case rule == "dive":
		constraints.Dive = true
	case rule == "required":
		constraints.Required = true
	case applyBoundRule(rule, constraints):
		// handled: min/max/gte/lte value bounds
	case strings.HasPrefix(rule, "len="):
		if val, err := strconv.Atoi(strings.TrimPrefix(rule, "len=")); err == nil {
			constraints.MinLength = &val
			constraints.MaxLength = &val
		}
	case strings.HasPrefix(rule, "minlen="):
		if val, err := strconv.Atoi(strings.TrimPrefix(rule, "minlen=")); err == nil {
			constraints.MinLength = &val
		}
	case strings.HasPrefix(rule, "maxlen="):
		if val, err := strconv.Atoi(strings.TrimPrefix(rule, "maxlen=")); err == nil {
			constraints.MaxLength = &val
		}
	case strings.HasPrefix(rule, "regexp="):
		constraints.Pattern = strings.TrimPrefix(rule, "regexp=")
	case strings.HasPrefix(rule, "oneof="):
		enumPart := strings.TrimPrefix(rule, "oneof=")
		for _, val := range strings.Split(enumPart, " ") {
			constraints.Enum = append(constraints.Enum, strings.TrimSpace(val))
		}
	default:
		if format, ok := validationFormatRules[rule]; ok {
			constraints.Format = format
		} else if pattern, ok := validationPatternRules[rule]; ok {
			constraints.Pattern = pattern
		}
	}
}

//nolint:gocyclo // validation constraint extraction from struct tags
func extractValidationConstraints(tag string) *ValidationConstraints {
	if tag == "" {
		return nil
	}

	constraints := &ValidationConstraints{}

	// Parse binding tag (Gin's validation tag — uses same syntax as go-playground/validator)
	if strings.Contains(tag, "binding:") {
		parts := strings.Split(tag, "binding:")
		if len(parts) > 1 {
			bindingTag := strings.Trim(parts[1], "\"")
			// Trim at next space-delimited tag boundary
			if idx := strings.Index(bindingTag, "\" "); idx >= 0 {
				bindingTag = bindingTag[:idx]
			}
			if strings.Contains(bindingTag, "required") {
				constraints.Required = true
			}
		}
	}

	// Parse validate tag (common validation libraries like go-playground/validator)
	if strings.Contains(tag, "validate:") {
		parts := strings.Split(tag, "validate:")
		if len(parts) > 1 {
			validateTag := strings.Trim(parts[1], "\"")

			// Parse common validation rules - improved regex to handle various formats
			// Matches: required, email, min=5, max=10, len=8, regexp=^[a-z]{2,3}$, oneof=val1 val2, etc.
			// This regex captures validation rules more accurately:
			// - Simple rules: required, email, url, etc.
			// - Rules with values: min=5, max=10, len=8
			// - Rules with complex values: regexp=^[a-z]{2,3}$, oneof=val1 val2 val3
			rules := getCachedMapperRegex(`([a-zA-Z_][a-zA-Z0-9_]*(?:=(?:[^,{}]|{[^}]*})*)?)`).FindAllStringSubmatch(validateTag, -1)
			for _, ruleSet := range rules {
				rule := strings.TrimSpace(ruleSet[1])
				applyValidationRule(rule, constraints)
			}
		}
	}
	// Parse custom validation tags
	if strings.Contains(tag, "min:") {
		parts := strings.Split(tag, "min:")
		if len(parts) > 1 {
			minPart := strings.Split(parts[1], " ")[0]
			if val, err := strconv.ParseFloat(strings.Trim(minPart, "\""), 64); err == nil {
				constraints.Min = &val
			}
		}
	}

	if strings.Contains(tag, "max:") {
		parts := strings.Split(tag, "max:")
		if len(parts) > 1 {
			maxPart := strings.Split(parts[1], " ")[0]
			if val, err := strconv.ParseFloat(strings.Trim(maxPart, "\""), 64); err == nil {
				constraints.Max = &val
			}
		}
	}

	if strings.Contains(tag, "regexp:") {
		parts := strings.Split(tag, "regexp:")
		if len(parts) > 1 {
			patternPart := strings.Split(parts[1], " ")[0]
			constraints.Pattern = strings.Trim(patternPart, "\"")
		}
	}

	if strings.Contains(tag, "enum:") {
		parts := strings.Split(tag, "enum:")
		if len(parts) > 1 {
			enumPart := strings.Split(parts[1], " ")[0]
			enumValues := strings.Split(strings.Trim(enumPart, "\""), ",")
			for _, val := range enumValues {
				constraints.Enum = append(constraints.Enum, strings.TrimSpace(val))
			}
		}
	}

	// Check if any constraints were found
	if constraints.MinLength == nil && constraints.MaxLength == nil &&
		constraints.Min == nil && constraints.Max == nil &&
		constraints.Pattern == "" && !constraints.Required && len(constraints.Enum) == 0 {
		return nil
	}

	return constraints
}

// applyValidationConstraints applies validation constraints to an OpenAPI schema
//
//nolint:gocyclo // validation constraint mapping has inherent branching for type-specific rules
func applyValidationConstraints(schema *Schema, constraints *ValidationConstraints) {
	if schema == nil || constraints == nil {
		return
	}

	// Handle dive: apply constraints to array items instead of the array itself
	if constraints.Dive && schema.Type == "array" && schema.Items != nil {
		itemConstraints := *constraints
		itemConstraints.Dive = false
		applyValidationConstraints(schema.Items, &itemConstraints)
		return
	}

	// Apply string length constraints (only for string types). On a string
	// field the value bounds min/max/gte/lte denote *length*, so fall back to
	// Min/Max when the length-specific minlen/maxlen/len aren't present.
	if schema.Type == "string" {
		switch {
		case constraints.MinLength != nil:
			schema.MinLength = *constraints.MinLength
		case constraints.Min != nil:
			schema.MinLength = int(*constraints.Min)
		}
		switch {
		case constraints.MaxLength != nil:
			schema.MaxLength = *constraints.MaxLength
		case constraints.Max != nil:
			schema.MaxLength = int(*constraints.Max)
		}
	}

	// Apply numeric constraints (for integer and number types)
	if schema.Type == "integer" || schema.Type == "number" {
		if constraints.Min != nil {
			schema.Minimum = constraints.Min
		}
		if constraints.Max != nil {
			schema.Maximum = constraints.Max
		}
		// Also check min/max from validate tags for numeric types
		if constraints.MinLength != nil && schema.Type == "integer" {
			schema.Minimum = floatPtr(float64(*constraints.MinLength))
		}
		if constraints.MaxLength != nil && schema.Type == "integer" {
			schema.Maximum = floatPtr(float64(*constraints.MaxLength))
		}
	}

	// Apply pattern constraint
	if constraints.Pattern != "" {
		schema.Pattern = constraints.Pattern
	}

	// Apply format constraint
	if constraints.Format != "" {
		schema.Format = constraints.Format
	}

	// Apply enum constraint
	if len(constraints.Enum) > 0 {
		switch schema.Type {
		case "array":
			schema.Items.Enum = constraints.Enum
		case "object":
			if schema.AdditionalProperties != nil {
				schema.AdditionalProperties.Enum = constraints.Enum
			}
		default:
			schema.Enum = constraints.Enum
		}
	}
}

// detectEnumFromConstants detects if a type has associated constants that form an enum
// This is a generic implementation using enhanced metadata with types.Info
func detectEnumFromConstants(goType string, pkgName string, meta *metadata.Metadata) []interface{} {
	if meta == nil {
		return nil
	}

	var goTypePkgName string

	goTypeParts := TypeParts(goType)
	if goTypeParts.PkgName != "" {
		goTypePkgName = goTypeParts.PkgName
		goTypePkgName = strings.TrimPrefix(goTypePkgName, "*")
		goTypePkgName = strings.TrimPrefix(goTypePkgName, "[]")

		goType = goTypeParts.TypeName
	}

	// Group constants by their resolved type and group index
	constantGroups := make(map[string]map[int][]EnumConstant)

	targetPkgName := pkgName
	if goTypePkgName != "" {
		targetPkgName = goTypePkgName
	}

	// Collect all constants and group them
	if pkg, exist := meta.Packages[targetPkgName]; exist {
		for _, file := range pkg.Files {
			for _, variable := range file.Variables {
				if getStringFromPool(meta, variable.Tok) == "const" {
					varType := getStringFromPool(meta, variable.Type)
					resolvedType := getStringFromPool(meta, variable.ResolvedType)
					varName := getStringFromPool(meta, variable.Name)

					// For enum detection, we want to match against the declared type, not the underlying type
					// Use the declared type if available, otherwise fall back to resolved type
					targetType := varType
					if targetType == "" {
						targetType = resolvedType
					}

					// Check if this constant's type matches our target enum type
					// For iota constants, we also need to check if they're in the same group as a typed constant
					if typeMatches(targetType, goType, meta) ||
						(varType == "" && isInSameGroupAsTypedConstant(variable.GroupIndex, goType, file.Variables, meta)) {
						groupIndex := variable.GroupIndex

						if constantGroups[targetType] == nil {
							constantGroups[targetType] = make(map[int][]EnumConstant)
						}

						enumConst := EnumConstant{
							Name:     varName,
							Type:     varType,
							Resolved: resolvedType,
							Value:    variable.ComputedValue,
							Group:    groupIndex,
						}

						constantGroups[targetType][groupIndex] = append(
							constantGroups[targetType][groupIndex],
							enumConst,
						)
					}
				}
			}
		}
	}

	// Find the best enum group for this type
	var bestEnumValues []interface{}
	var maxGroupSize int

	for _, groups := range constantGroups {
		for _, group := range groups {
			if len(group) > maxGroupSize {
				maxGroupSize = len(group)
				bestEnumValues = extractEnumValues(group)
			}
		}
	}

	return bestEnumValues
}

// EnumConstant represents a constant that might be part of an enum
type EnumConstant struct {
	Name     string
	Type     string
	Resolved string
	Value    interface{}
	Group    int
}

// extractEnumValues extracts the actual values from enum constants
func extractEnumValues(constants []EnumConstant) []interface{} {
	var values []interface{}

	for _, constant := range constants {
		if constant.Value != nil {
			// Use the computed value from types.Info
			switch v := constant.Value.(type) {
			case *types.Const:
				// Handle types.Const values
				if v.Val() != nil {
					extracted := extractConstantValue(v.Val())
					values = append(values, extracted)
				}
			default:
				// The values are already in their proper form (string, int, etc.)
				// Just extract them using our helper function
				extracted := extractConstantValue(v)
				values = append(values, extracted)
			}
		}
	}

	// Sort the values to ensure consistent order
	sort.Slice(values, func(i, j int) bool {
		// Convert to strings for comparison
		valI := fmt.Sprintf("%v", values[i])
		valJ := fmt.Sprintf("%v", values[j])
		return valI < valJ
	})

	return values
}

// extractConstantValue extracts the actual value from a constant.Value
func extractConstantValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}

	// Try to use the String() method if available to extract the value
	if stringer, ok := val.(interface{ String() string }); ok {
		str := stringer.String()

		// For string constants, remove quotes if they exist
		if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
			return str[1 : len(str)-1] // Remove surrounding quotes
		}

		// For numeric constants, try to parse
		if i, err := strconv.ParseInt(str, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			return f
		}
		if b, err := strconv.ParseBool(str); err == nil {
			return b
		}

		// Return the string representation as fallback
		return str
	}

	// If it's not a stringer, return as-is
	return val
}

// typeMatches checks if a constant type matches the target enum type
func typeMatches(constantType, targetType string, meta *metadata.Metadata) bool {
	// Direct match
	if constantType == targetType {
		return true
	}

	// Handle pointer types
	if strings.HasPrefix(constantType, "*") && constantType[1:] == targetType {
		return true
	}
	if strings.HasPrefix(targetType, "*") && targetType[1:] == constantType {
		return true
	}

	// Check if constantType is an alias of targetType
	if resolvedConstType := resolveUnderlyingType(constantType, meta); resolvedConstType != "" {
		if resolvedConstType == targetType {
			return true
		}
		// Also check if the resolved type matches the target's underlying type
		if resolvedTargetType := resolveUnderlyingType(targetType, meta); resolvedTargetType != "" {
			if resolvedConstType == resolvedTargetType {
				return true
			}
		}
	}

	// Handle package-qualified types - extract just the type name
	constTypeParts := strings.Split(constantType, ".")
	targetTypeParts := strings.Split(targetType, ".")

	switch {
	case len(constTypeParts) > 1 && len(targetTypeParts) > 1:
		// Both are package-qualified, compare the type names
		constTypeName := constTypeParts[len(constTypeParts)-1]
		targetTypeName := targetTypeParts[len(targetTypeParts)-1]
		return constTypeName == targetTypeName
	case len(constTypeParts) > 1:
		// Constant is package-qualified, target is not
		constTypeName := constTypeParts[len(constTypeParts)-1]
		return constTypeName == targetType
	case len(targetTypeParts) > 1:
		// Target is package-qualified, constant is not
		targetTypeName := targetTypeParts[len(targetTypeParts)-1]
		return constantType == targetTypeName
	default:
		return false
	}
}

const mapGoTypeToOpenAPISchemaKey = "mapGoTypeToOpenAPISchema"

// mapGoTypeToOpenAPISchema maps Go types to OpenAPI schemas
//
//nolint:gocyclo // Go type to OpenAPI schema mapping with all Go types
func mapGoTypeToOpenAPISchema(usedTypes map[string]*Schema, goType string, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	schemas := map[string]*Schema{}
	var schema *Schema

	if visitedTypes == nil {
		visitedTypes = map[string]bool{}
	}

	isPrimitive := metadata.IsPrimitiveType(goType)

	derivedGoType := strings.TrimPrefix(goType, "*")

	// Check for cycles using both the original type and the derived type
	if (visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] || visitedTypes[derivedGoType+mapGoTypeToOpenAPISchemaKey]) && canAddRefSchemaForType(derivedGoType) {
		return addRefSchemaForType(goType), schemas
	}
	visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] = true

	// Add recursion guard - if we're already processing this type, return a reference
	if schema, exists := usedTypes[derivedGoType]; exists && schema != nil && canAddRefSchemaForType(derivedGoType) {
		return addRefSchemaForType(derivedGoType), schemas
	}

	// Check type mappings first
	for _, mapping := range cfg.TypeMapping {
		if mapping.GoType == goType {
			schema = mapping.OpenAPIType
			markUsedType(usedTypes, goType, schema)

			return schema, schemas
		}
	}

	// Check external types
	if cfg != nil {
		for _, externalType := range cfg.ExternalTypes {
			if externalType.Name == goType {
				schemas[goType] = externalType.OpenAPIType
			}
		}
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		underlyingType := strings.TrimSpace(goType[1:])
		// For pointer types, we generate the same schema as the underlying type
		// but we could add nullable: true if needed for OpenAPI 3.0+
		schema, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, underlyingType, meta, cfg, visitedTypes)
		maps.Copy(schemas, newSchemas)
		return schema, schemas
	}

	// Handle array types (e.g., [16]byte, [N]string)
	if strings.HasPrefix(goType, "[") {
		// Find the closing bracket
		endIdx := strings.Index(goType, "]")
		if endIdx > 1 {
			elementType := strings.TrimSpace(goType[endIdx+1:])
			arraySize := strings.TrimSpace(goType[1:endIdx])

			var resolvedType string
			if resolvedType = resolveUnderlyingType(elementType, meta); resolvedType == "" {
				resolvedType = elementType
			}
			isPrimitiveElement := metadata.IsPrimitiveType(resolvedType)

			// Special handling for byte arrays - convert to string with maxLength
			if elementType == "byte" || resolvedType == "byte" {
				schema = &Schema{
					Type:   "string",
					Format: "byte",
				}
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxLength = *size
				}
				return schema, schemas
			}

			// For other primitive types, create array schema
			if isPrimitiveElement {
				items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newSchemas)

				schema = &Schema{
					Type:  "array",
					Items: items,
				}
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxItems = *size
					schema.MinItems = *size // Fixed size array
				}
				return schema, schemas
			}

			// For complex types, check if already exists in usedTypes
			if bodySchema, ok := usedTypes[elementType]; ok {
				if bodySchema == nil {
					var newBodySchemas map[string]*Schema
					bodySchema, newBodySchemas = mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
					maps.Copy(schemas, newBodySchemas)
				}
				markUsedType(usedTypes, resolvedType, bodySchema)

				// Create a reference to the existing schema
				schema = &Schema{
					Type:  "array",
					Items: addRefSchemaForType(resolvedType),
				}
				if size := parseArraySize(arraySize); size != nil {
					schema.MaxItems = *size
					schema.MinItems = *size // Fixed size array
				}
				return schema, schemas
			}

			items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
			maps.Copy(schemas, newSchemas)

			// Use reference for complex element types in arrays
			if canAddRefSchemaForType(resolvedType) && items != nil {
				schemas[resolvedType] = items
				items = addRefSchemaForType(resolvedType)
			}

			// Apply enum detection for array elements if the element type is not primitive
			if !metadata.IsPrimitiveType(elementType) && items != nil && len(items.Enum) == 0 {
				// Extract package name for enum detection
				pkgName := ""
				if typeParts := TypeParts(elementType); typeParts.PkgName != "" {
					pkgName = typeParts.PkgName
				}

				// Detect enum values for this element type
				if enumValues := detectEnumFromConstants(elementType, pkgName, meta); len(enumValues) > 0 {
					// Apply enum values to the stored schema if it exists
					if storedSchema, exists := schemas[resolvedType]; exists {
						storedSchema.Enum = enumValues
					} else {
						items.Enum = enumValues
					}
				}
			}

			schema = &Schema{
				Type:  "array",
				Items: items,
			}
			if size := parseArraySize(arraySize); size != nil {
				schema.MaxItems = *size
				schema.MinItems = *size // Fixed size array
			}
			return schema, schemas
		}
	}

	// Handle map types
	if strings.Contains(goType, "map[") {
		startIdx := strings.Index(goType, "map[")
		endIdx := strings.Index(goType, "]")
		if endIdx > startIdx+4 {
			keyType := goType[startIdx+4 : endIdx]
			valueType := strings.TrimSpace(goType[endIdx+1:])

			// Qualify the value's element type with the map's package prefix
			// (everything before "map["). The prefix already carries its
			// trailing dot, so it must not be doubled, and any []/* wrapper
			// stays outside the qualifier so map[string][]Money becomes
			// []pkg.Money rather than pkg..[]Money (then pkg.._Money).
			if startIdx > 0 {
				valueType = qualifyElementType(valueType, goType[:startIdx])
			}

			if keyType == "string" {
				var resolvedType string
				if resolvedType = resolveUnderlyingType(valueType, meta); resolvedType == "" {
					resolvedType = valueType
				}

				additionalProperties, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newSchemas)

				// Use reference for complex value types in maps
				if !metadata.IsPrimitiveType(resolvedType) && canAddRefSchemaForType(resolvedType) {
					schemas[resolvedType] = additionalProperties
					additionalProperties = addRefSchemaForType(resolvedType)
				}

				// Apply enum detection for map values if the value type is not primitive
				if !metadata.IsPrimitiveType(valueType) && additionalProperties != nil && len(additionalProperties.Enum) == 0 {
					// Extract package name for enum detection
					pkgName := ""
					if typeParts := TypeParts(valueType); typeParts.PkgName != "" {
						pkgName = typeParts.PkgName
					}

					// Detect enum values for this value type
					if enumValues := detectEnumFromConstants(valueType, pkgName, meta); len(enumValues) > 0 {
						// Apply enum values to the stored schema if it exists
						if storedSchema, exists := usedTypes[resolvedType]; exists && storedSchema != nil {
							storedSchema.Enum = enumValues
						} else if storedSchema, exists := schemas[resolvedType]; exists {
							storedSchema.Enum = enumValues
						} else {
							additionalProperties.Enum = enumValues
						}
					}
				}

				schema = &Schema{
					Type:                 "object",
					AdditionalProperties: additionalProperties,
				}

				return schema, schemas
			}
			// Non-string keys are not supported in OpenAPI, fallback to generic object
			schema = &Schema{Type: "object"}

			return schema, schemas
		}
	}

	// Handle generic struct instantiation (e.g., "APIResponse[pkg.User]").
	// Find the base struct, substitute type parameters, generate inline schema,
	// and store it as a named component.
	if bracketIdx := strings.Index(goType, "["); bracketIdx > 0 && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[") {
		parts := TypeParts(goType)
		typ := typeByName(parts, meta)
		if typ != nil && getStringFromPool(meta, typ.Kind) == "struct" {
			structSchema, newSchemas := generateStructSchema(usedTypes, goType, typ, meta, cfg, visitedTypes)
			if structSchema != nil {
				// Use schemaName for the component key to ensure $ref matches
				componentKey := schemaName(goType, cfg)
				schemas[goType] = structSchema
				maps.Copy(schemas, newSchemas)
				markUsedType(usedTypes, goType, structSchema)
				return &Schema{Ref: refComponentsSchemasPrefix + componentKey}, schemas
			}
		}
	}

	// Handle []byte explicitly before generic slice handling.
	// []byte maps to string/byte (base64) for struct fields.
	// Response-writer context overrides to string/binary in ExtractResponse.
	if goType == "[]byte" {
		return &Schema{Type: "string", Format: "byte"}, schemas
	}
	// Also catch type aliases that resolve to []byte.
	if strings.HasPrefix(goType, "[]") {
		resolved := resolveUnderlyingType(goType, meta)
		if resolved == "[]byte" {
			return &Schema{Type: "string", Format: "byte"}, schemas
		}
	}

	// Handle slice types
	if strings.HasPrefix(goType, "[]") {
		elementType := strings.TrimSpace(goType[2:])

		var resolvedType string
		if resolvedType = resolveUnderlyingType(elementType, meta); resolvedType == "" {
			resolvedType = elementType
		}
		isPrimitiveElement := metadata.IsPrimitiveType(resolvedType)

		// Check if the element type already exists in usedTypes
		if bodySchema, ok := usedTypes[elementType]; !isPrimitiveElement && ok {
			if bodySchema == nil {
				var newBodySchemas map[string]*Schema

				bodySchema, newBodySchemas = mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
				maps.Copy(schemas, newBodySchemas)
			}
			markUsedType(usedTypes, resolvedType, bodySchema)

			// Create a reference to the existing schema
			schema = &Schema{
				Type:  "array",
				Items: addRefSchemaForType(resolvedType),
			}

			return schema, schemas
		}

		items, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, resolvedType, meta, cfg, visitedTypes)
		maps.Copy(schemas, newSchemas)

		// Use reference for complex element types in arrays
		if !isPrimitiveElement && canAddRefSchemaForType(resolvedType) && items != nil {
			schemas[resolvedType] = items
			items = addRefSchemaForType(resolvedType)
		}

		// Apply enum detection for array elements if the element type is not primitive
		if !metadata.IsPrimitiveType(elementType) && items != nil && len(items.Enum) == 0 {
			// Extract package name for enum detection
			pkgName := ""
			if typeParts := TypeParts(elementType); typeParts.PkgName != "" {
				pkgName = typeParts.PkgName
			}

			// Detect enum values for this element type
			if enumValues := detectEnumFromConstants(elementType, pkgName, meta); len(enumValues) > 0 {
				// Apply enum values to the stored schema if it exists
				if storedSchema, exists := usedTypes[resolvedType]; exists && storedSchema != nil {
					storedSchema.Enum = enumValues
				} else if storedSchema, exists := schemas[resolvedType]; exists {
					storedSchema.Enum = enumValues
				} else {
					items.Enum = enumValues
				}
			}
		}

		schema = &Schema{
			Type:  "array",
			Items: items,
		}

		return schema, schemas
	}

	// Default mappings
	switch goType {
	case "string":
		return &Schema{Type: "string"}, schemas
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}, schemas
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return &Schema{Type: "integer", Minimum: floatPtr(0)}, schemas
	case "float32", "float64":
		return &Schema{Type: "number"}, schemas
	case "bool":
		return &Schema{Type: "boolean"}, schemas
	case "time.Time":
		return &Schema{
			Type:   "string",
			Format: "date-time",
		}, schemas
	case "[]string":
		return &Schema{Type: "array", Items: &Schema{Type: "string"}}, schemas
	case "[]time.Time":
		return &Schema{Type: "array", Items: &Schema{Type: "string", Format: "date-time"}}, schemas
	case "[]int":
		return &Schema{Type: "array", Items: &Schema{Type: "integer"}}, schemas
	case "interface{}", "struct{}", "any":
		return &Schema{Type: "object"}, schemas
	default:
		// For custom types, check if it's a struct in metadata
		if meta != nil {
			// Try to find the type in metadata
			typs := findTypesInMetadata(meta, goType)
			for key, typ := range typs {
				if typ != nil {
					// Generate inline schema for the type
					schema, newSchemas := generateSchemaFromType(usedTypes, key, typ, meta, cfg, visitedTypes)
					if schema != nil {
						if canAddRefSchemaForType(key) {
							schemas[key] = schema
							schema = addRefSchemaForType(key)
						}

						maps.Copy(schemas, newSchemas)
						markUsedType(usedTypes, goType, schema)

						return schema, schemas
					}
				}
			}
		}

		if !isPrimitive && goType != "" {
			return addRefSchemaForType(goType), schemas
		}

		return schema, schemas
	}
}

// schemaFromTypeRef produces the OpenAPI schema for a structured TypeRef — the
// tree-based counterpart to the leaf/structural cases of
// mapGoTypeToOpenAPISchema. It returns nil for forms that still need metadata
// lookup, component naming, or generic substitution (named/struct/generic), so
// the string path continues to handle those during the migration.
//
// T007: built and shadow-tested against mapGoTypeToOpenAPISchema for
// equivalence; wired into the live schema path in a follow-up so each golden can
// be verified. Fixed-array length (US1) and generic substitution (US2) are added
// later — this mirrors current output.
func schemaFromTypeRef(ref *metadata.TypeRef) *Schema {
	if ref == nil {
		return nil
	}
	switch ref.Kind {
	case metadata.RefPointer:
		return schemaFromTypeRef(ref.Elem) // a pointer is transparent to the schema
	case metadata.RefSlice:
		if isByteRef(ref.Elem) {
			return &Schema{Type: "string", Format: "byte"} // []byte -> base64 string
		}
		items := schemaFromTypeRef(ref.Elem)
		if items == nil {
			return nil // unknown element — defer to the string path
		}
		return &Schema{Type: "array", Items: items}
	case metadata.RefArray:
		// A fixed-length array mirrors the string path's [N]T handling: byte
		// arrays become a base64 string with maxLength; others an array with
		// minItems == maxItems. Len -1 (inferred/unresolved) carries no constraint.
		if isByteRef(ref.Elem) {
			s := &Schema{Type: "string", Format: "byte"}
			if ref.Len >= 0 {
				s.MaxLength = ref.Len
			}
			return s
		}
		items := schemaFromTypeRef(ref.Elem)
		if items == nil {
			return nil
		}
		s := &Schema{Type: "array", Items: items}
		setFixedArrayLen(s, ref)
		return s
	case metadata.RefMap:
		if !isStringRef(ref.Key) {
			return nil // only string-keyed maps map cleanly to an object
		}
		val := schemaFromTypeRef(ref.Elem)
		if val == nil {
			return nil
		}
		return &Schema{Type: "object", AdditionalProperties: val}
	case metadata.RefInterface:
		return &Schema{Type: "object"}
	case metadata.RefBasic:
		return basicRefSchema(ref.Name)
	default:
		return nil // named / generic / struct / func / chan — string path for now
	}
}

func isByteRef(r *metadata.TypeRef) bool {
	return r != nil && r.Kind == metadata.RefBasic && r.Name == "byte"
}

func isStringRef(r *metadata.TypeRef) bool {
	return r != nil && r.Kind == metadata.RefBasic && r.Name == "string"
}

// basicRefSchema maps a primitive type name to its schema, mirroring the default
// switch of mapGoTypeToOpenAPISchema. Returns nil for an unrecognized name.
func basicRefSchema(name string) *Schema {
	switch name {
	case "string":
		return &Schema{Type: "string"}
	case "int", "int8", "int16", "int32", "int64":
		return &Schema{Type: "integer"}
	case "uint", "uint8", "uint16", "uint32", "uint64", "byte":
		return &Schema{Type: "integer", Minimum: floatPtr(0)}
	case "float32", "float64":
		return &Schema{Type: "number"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "time.Time":
		return &Schema{Type: "string", Format: "date-time"}
	case "interface{}", "struct{}", "any":
		return &Schema{Type: "object"}
	default:
		return nil
	}
}

// schemaForType returns the schema for a type, trying the structured TypeRef
// first (when one is available) and falling back to the string-based
// mapGoTypeToOpenAPISchema for forms the tree generator does not yet handle.
// This is the migration seam: tree-first, string-fallback, output-preserving
// (schemaFromTypeRef is shadow-tested to match the string path).
func schemaForType(usedTypes map[string]*Schema, goType string, ref *metadata.TypeRef, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema) {
	// Shared with schemaForNamedRef and mapGoTypeToOpenAPISchema below so the
	// cycle-tracking map is one instance (and never nil — schemaForNamedRef writes
	// to it before the string path's own nil-guard would run).
	if visitedTypes == nil {
		visitedTypes = map[string]bool{}
	}
	// Config TypeMapping takes precedence over any structural/named resolution,
	// exactly as mapGoTypeToOpenAPISchema applies it before everything else — so
	// neither the tree nor the parse retry below can shadow a configured override.
	if cfg != nil && goType != "" {
		for _, mapping := range cfg.TypeMapping {
			if mapping.GoType == goType {
				markUsedType(usedTypes, goType, mapping.OpenAPIType)
				return mapping.OpenAPIType, map[string]*Schema{}
			}
		}
	}
	if ref != nil {
		if s := schemaFromTypeRef(ref); s != nil {
			return s, map[string]*Schema{}
		}
		// Walk the tree for a named struct leaf, optionally wrapped in
		// pointers/slices/arrays — looking metadata types up by the ref's own
		// Pkg/Name (no string parsing) and reusing the shared component machinery.
		// schemaForRefTree defers (string path) for generics, maps, and alias/enum
		// leaves, which the string path resolves with behaviour this does not
		// reproduce yet.
		if s, ns, ok := schemaForRefTree(usedTypes, ref, meta, cfg, visitedTypes); ok {
			return s, ns
		}
		// The flattened string can be empty where getTypeName has a gap — most
		// notably a multi-type-parameter generic (Pair[K, V]), whose IndexListExpr
		// it never handled. The tree carries the real type, so fall back to its
		// canonical string and let the existing machinery resolve it.
		if goType == "" {
			goType = ref.String()
		}
	} else if goType != "" && !strings.Contains(goType, TypeSep) {
		// String-only caller (no TypeRef): recover one by parsing the canonical
		// string so the structural cases resolve on the tree instead of the string
		// generator. Strings still carrying the metadata "-->" separator are left to
		// mapGoTypeToOpenAPISchema, whose usedTypes keys other consumers (field-format
		// inference) look up by that exact spelling — the convention is unified in a
		// follow-up before those flow through the tree. Alias/generic/config leaves
		// fall through regardless.
		if pref := metadata.ParseTypeRef(goType); pref != nil {
			if s := schemaFromTypeRef(pref); s != nil {
				return s, map[string]*Schema{}
			}
			if s, ns, ok := schemaForRefTree(usedTypes, pref, meta, cfg, visitedTypes); ok {
				return s, ns
			}
		}
	}
	s, newSchemas := mapGoTypeToOpenAPISchema(usedTypes, goType, meta, cfg, visitedTypes)
	// A fixed-length array whose element defers to the string path (a named
	// struct/generic element, e.g. [4]Point) keeps its length only on the
	// TypeRef — getTypeName flattened the field string to "[]Point", dropping the
	// count. Re-apply the length onto the array the string path produced.
	setFixedArrayLen(s, ref)
	return s, newSchemas
}

// schemaForRefTree walks a TypeRef whose leaf is a named struct — directly or
// wrapped in pointers (transparent to the schema), slices, or fixed arrays — and
// builds the schema from the tree, reusing schemaForNamedRef for the leaf. It is
// the recursive completion of schemaFromTypeRef for the cases that need metadata
// lookup and component generation (which the pure schemaFromTypeRef cannot do).
//
// It returns ok=false (deferring to the string path) for anything it does not yet
// reproduce byte-for-byte: generic instantiations, maps, and alias/enum leaves
// (the string path applies inline alias resolution and element enum detection).
// schemaFromTypeRef already handled every primitive-leaf container before this is
// reached, so the slice/array cases here only ever wrap a named leaf.
func schemaForRefTree(usedTypes map[string]*Schema, ref *metadata.TypeRef, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema, bool) {
	if ref == nil {
		return nil, nil, false
	}
	switch ref.Kind {
	case metadata.RefPointer:
		return schemaForRefTree(usedTypes, ref.Elem, meta, cfg, visitedTypes) // transparent
	case metadata.RefNamed:
		if len(ref.Args) != 0 {
			return nil, nil, false // generic instantiation → string path
		}
		return schemaForNamedRef(usedTypes, ref, meta, cfg, visitedTypes)
	case metadata.RefSlice, metadata.RefArray:
		items, schemas, ok := schemaForRefTree(usedTypes, ref.Elem, meta, cfg, visitedTypes)
		if !ok || items == nil {
			return nil, nil, false
		}
		s := &Schema{Type: "array", Items: items}
		setFixedArrayLen(s, ref)
		return s, schemas, true
	case metadata.RefMap:
		// Only string-keyed maps map cleanly to an OpenAPI object (matching
		// schemaFromTypeRef and the string path); others defer.
		if !isStringRef(ref.Key) {
			return nil, nil, false
		}
		val, schemas, ok := schemaForRefTree(usedTypes, ref.Elem, meta, cfg, visitedTypes)
		if !ok || val == nil {
			return nil, nil, false
		}
		return &Schema{Type: "object", AdditionalProperties: val}, schemas, true
	default:
		return nil, nil, false // func / chan / basic → string path
	}
}

// schemaForNamedRef resolves a direct, non-generic named TypeRef to its schema by
// looking the metadata type up via the ref's Pkg/Name (typeByRef — no string
// parsing) and reusing generateSchemaFromType plus the shared component
// machinery. It is a faithful tree-sourced mirror of mapGoTypeToOpenAPISchema's
// named (default) branch, including the same cycle/usedTypes guards keyed on the
// canonical string, so output is identical.
//
// Returns ok=false (defer to the string path) when the type is governed by a
// config mapping, is not found in metadata (external/dangling refs the string
// path names by their short alias), or yields no schema — preserving those exact
// behaviours.
func schemaForNamedRef(usedTypes map[string]*Schema, ref *metadata.TypeRef, meta *metadata.Metadata, cfg *APISpecConfig, visitedTypes map[string]bool) (*Schema, map[string]*Schema, bool) {
	goType := ref.String()

	// Decide whether to handle this leaf BEFORE any side effect on visitedTypes:
	// a premature cycle-key write would make a string-path fallback for the same
	// goType mistake it for a back-reference and emit a bare $ref without ever
	// generating the component.
	//
	// Config mappings take precedence in the string path (checked before any
	// structural handling); leave them to it.
	for _, m := range cfg.TypeMapping {
		if m.GoType == goType {
			return nil, nil, false
		}
	}
	for _, et := range cfg.ExternalTypes {
		if et.Name == goType {
			return nil, nil, false
		}
	}
	typ := typeByRef(ref, meta)
	if typ == nil {
		return nil, nil, false // external/unfound: string path emits the short-named $ref
	}
	// Only structs are resolved on the tree here. An alias/enum field is resolved
	// to its underlying type INLINE by the caller (resolveUnderlyingType in
	// generateStructSchema) before this point — so goType arriving here is already
	// the underlying primitive; deferring keeps that inline-with-enum result
	// instead of fabricating a component $ref. Interface and other kinds likewise
	// defer to the string path's handling (internal-alias tree resolution is T009).
	if getStringFromPool(meta, typ.Kind) != "struct" {
		return nil, nil, false
	}

	// Committed to handling: now apply the same cycle/recursion guards the string
	// path applies on entry, keyed on the same string, so a back-reference resolves
	// to a $ref at the identical point.
	schemas := map[string]*Schema{}
	if visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] && canAddRefSchemaForType(goType) {
		return addRefSchemaForType(goType), schemas, true
	}
	visitedTypes[goType+mapGoTypeToOpenAPISchemaKey] = true
	if s, exists := usedTypes[goType]; exists && s != nil && canAddRefSchemaForType(goType) {
		return addRefSchemaForType(goType), schemas, true
	}

	schema, newSchemas := generateSchemaFromType(usedTypes, goType, typ, meta, cfg, visitedTypes)
	if schema == nil {
		return nil, nil, false
	}
	if canAddRefSchemaForType(goType) {
		schemas[goType] = schema
		schema = addRefSchemaForType(goType)
	}
	maps.Copy(schemas, newSchemas)
	markUsedType(usedTypes, goType, schema)
	return schema, schemas, true
}

// setFixedArrayLen stamps minItems == maxItems == N on an array schema when ref
// is a fixed-length array with a known length (decision D5). It is the single
// source of truth for the array-length rule, shared by every site that builds an
// array schema from a TypeRef (schemaFromTypeRef, schemaForType, and the inline
// nested-struct element path). A no-op for slices, inferred-length ([...]T)
// arrays, byte arrays (which use maxLength), and non-array schemas.
func setFixedArrayLen(s *Schema, ref *metadata.TypeRef) {
	if s != nil && s.Type == "array" && ref != nil && ref.Kind == metadata.RefArray && ref.Len >= 0 {
		s.MinItems = ref.Len
		s.MaxItems = ref.Len
	}
}

func canAddRefSchemaForType(key string) bool {
	// A leading "[" marks an array or slice ("[]T" or a fixed-length "[N]T") —
	// never a nameable component (generic instantiations start with the type
	// name, e.g. "Pair[...]", so they are unaffected). Matching only "[]" missed
	// fixed arrays, which getTypeName used to hide by flattening "[N]T" to "[]T".
	if metadata.IsPrimitiveType(key) || strings.HasPrefix(key, "[") || strings.Contains(key, "map[") {
		return false
	}

	// Exclude _nested types from reference schema generation
	if strings.HasSuffix(key, "_nested") {
		return false
	}

	// Allow reference schemas for custom types
	return true
}

func addRefSchemaForType(goType string) *Schema {
	// For custom types not found in metadata, create a reference.
	// Note: short name shortening for $ref values is applied in a
	// post-processing pass (disambiguateSchemaNames) to ensure
	// consistency with component schema keys.
	goType = strings.TrimPrefix(goType, "*")
	return &Schema{Ref: refComponentsSchemasPrefix + schemaComponentNameReplacer.Replace(goType)}
}

// isInSameGroupAsTypedConstant checks if a constant is in the same group as a typed constant
func isInSameGroupAsTypedConstant(groupIndex int, targetType string, variables map[string]*metadata.Variable, meta *metadata.Metadata) bool {
	for _, variable := range variables {
		if getStringFromPool(meta, variable.Tok) == "const" &&
			variable.GroupIndex == groupIndex {
			varType := getStringFromPool(meta, variable.Type)
			if typeMatches(varType, targetType, meta) {
				return true
			}
		}
	}
	return false
}

// parseArraySize parses the array size from Go array syntax
// Returns the size as an integer, or nil if parsing fails or no size constraint
func parseArraySize(sizeStr string) *int {
	if sizeStr == "" {
		return nil
	}

	// Handle "..." (variable length array)
	if sizeStr == "..." {
		return nil
	}

	// Try to parse as integer
	if size, err := strconv.Atoi(sizeStr); err == nil {
		return &size
	}

	// If it's not a number, return nil (no size constraint)
	return nil
}

// extractDocComment looks up the handler function's doc comment from metadata
// and splits it into summary (first sentence) and description (full text).
// Returns empty strings if no comment is found.
// lookupFuncComment searches a package for a function's doc comment.
func lookupFuncComment(pkg *metadata.Package, funcName, recvType string, sp *metadata.StringPool) (string, string) {
	for _, file := range pkg.Files {
		// Free functions are recorded in file.Functions.
		if fn, exists := file.Functions[funcName]; exists {
			if comment := sp.GetString(fn.Comments); comment != "" {
				return splitDocComment(comment)
			}
		}
		// Method handlers live on their receiver type, not in file.Functions —
		// resolve the comment via the type's method records. recvType (the
		// handler's receiver) is matched case-insensitively because the function
		// path renders it lower-cased ("handler"), while the type is "Handler";
		// an empty recvType (free-function route) matches no type, so the scan
		// is a no-op there.
		if recvType == "" {
			continue
		}
		for _, typ := range file.Types {
			if !strings.EqualFold(sp.GetString(typ.Name), recvType) {
				continue
			}
			for i := range typ.Methods {
				if sp.GetString(typ.Methods[i].Name) != funcName {
					continue
				}
				if comment := sp.GetString(typ.Methods[i].Comments); comment != "" {
					return splitDocComment(comment)
				}
			}
		}
	}
	return "", ""
}

// parseFuncNameAndPackage extracts the bare function name and package prefix
// from a route function path like "myapp.UserHandler.GetUser".
func parseFuncNameAndPackage(function string) (funcName, pkgPrefix string) {
	funcName = function
	if idx := strings.LastIndex(funcName, "."); idx >= 0 {
		pkgPrefix = funcName[:idx]
		funcName = funcName[idx+1:]
	}
	// Strip position suffix if present (e.g., "FuncLit:file.go:10")
	if idx := strings.Index(funcName, ":"); idx >= 0 {
		funcName = funcName[:idx]
	}
	return funcName, pkgPrefix
}

func extractDocComment(route *RouteInfo) (summary, description string) {
	if route == nil || route.Metadata == nil || route.Metadata.StringPool == nil {
		return "", ""
	}

	funcName, pkgPrefix := parseFuncNameAndPackage(route.Function)
	sp := route.Metadata.StringPool

	// For a method handler the receiver type follows the TypeSep in the
	// function path (e.g. ".../echo-->handler.GetUsers" -> "handler"); used to
	// resolve the method's doc comment off its type. The package path itself
	// contains dots, so split on TypeSep first, then take the final dot-segment
	// (handles "...-->main.deps.DocumentHandler").
	recvType := pkgPrefix
	if i := strings.LastIndex(recvType, TypeSep); i >= 0 {
		recvType = recvType[i+len(TypeSep):]
	}
	if i := strings.LastIndex(recvType, "."); i >= 0 {
		recvType = recvType[i+1:]
	}
	recvType = strings.TrimPrefix(recvType, "*")

	// First pass: use route.Package if available (most precise).
	if route.Package != "" {
		if pkg, ok := route.Metadata.Packages[route.Package]; ok {
			if s, d := lookupFuncComment(pkg, funcName, recvType, sp); s != "" || d != "" {
				return s, d
			}
		}
	}

	// Second pass: match by pkgPrefix suffix against metadata package keys.
	// Handles cases like pkgPrefix="users.UserHandler" matching package "users".
	if pkgPrefix != "" {
		for pkgName, pkg := range route.Metadata.Packages {
			if !strings.HasSuffix(pkgPrefix, pkgName) && !strings.HasSuffix(pkgName, pkgPrefix) {
				continue
			}
			if s, d := lookupFuncComment(pkg, funcName, recvType, sp); s != "" || d != "" {
				return s, d
			}
		}
	}

	// Fallback: search all packages (for cases where package prefix doesn't match)
	for _, pkg := range route.Metadata.Packages {
		if s, d := lookupFuncComment(pkg, funcName, recvType, sp); s != "" || d != "" {
			return s, d
		}
	}
	return "", ""
}

// splitDocComment splits a Go doc comment into summary and description.
// Summary is the first sentence (up to the first ". " or ".\n" or the whole
// text if no sentence boundary). Description is the full comment text.
// If the comment is a single sentence, description is left empty to avoid
// duplication in the OpenAPI output.
func splitDocComment(comment string) (summary, description string) {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return "", ""
	}

	// Find the first sentence boundary
	summary = comment
	for i, r := range comment {
		if r == '.' && i+1 < len(comment) {
			next := comment[i+1]
			if next == ' ' || next == '\n' || next == '\r' {
				summary = comment[:i+1]
				break
			}
		}
	}

	// If the summary IS the full comment (single sentence), don't duplicate
	if strings.TrimSpace(strings.TrimSuffix(summary, ".")) == strings.TrimSpace(strings.TrimSuffix(comment, ".")) {
		return summary, ""
	}

	return summary, comment
}
