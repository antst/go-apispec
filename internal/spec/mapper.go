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

var schemaComponentNameReplacer = strings.NewReplacer("/", "_", "-->", ".", " ", "-", "[", "_", "]", "", ", ", "-")

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
	for pathStr, pathItem := range paths {
		for method, op := range map[string]*Operation{
			"GET": pathItem.Get, "POST": pathItem.Post, "PUT": pathItem.Put,
			"DELETE": pathItem.Delete, "PATCH": pathItem.Patch, "OPTIONS": pathItem.Options,
			"HEAD": pathItem.Head,
		} {
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

	// Fill securitySchemes in components if present in config
	if len(cfg.SecuritySchemes) > 0 {
		if spec.Components == nil {
			spec.Components = &Components{}
		}
		spec.Components.SecuritySchemes = cfg.SecuritySchemes
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
			}
		}

		// Add parameters (deduplicated and ensure all path params)
		if len(route.Params) > 0 {
			operation.Parameters = deduplicateParameters(route.Params)
		} else {
			operation.Parameters = nil
		}
		operation.Parameters = ensureAllPathParams(openAPIPath, operation.Parameters)

		// Add responses
		operation.Responses = buildResponses(route.Response)

		// Set operation on path item
		setOperationOnPathItem(&pathItem, route.Method, operation)
		paths[openAPIPath] = pathItem
	}

	return paths
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
	var bodyOnlyKeys []string
	var statusOnlyKeys []string
	for key, resp := range respInfo {
		if resp.StatusCode > 0 && resp.BodyType == "" && resp.Schema == nil {
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
		// If status code is unknown but the response has a body, infer 200 (OK).
		// This handles the common case where handlers write a response without
		// calling WriteHeader explicitly.
		if resp.StatusCode < 0 && resp.BodyType != "" {
			resp.StatusCode = http.StatusOK
			statusCode = "200"
		} else if resp.StatusCode < 0 {
			// No body and no status — use "default"
			statusCode = "default"
		}

		description := http.StatusText(resp.StatusCode)
		if resp.StatusCode < 0 || description == "" {
			description = "Status code could not be determined"
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

// convertPathToOpenAPI converts a Go path to OpenAPI format
func convertPathToOpenAPI(path string) string {
	// Regular expression to match :param format
	// This matches a colon followed by one or more word characters (letters, digits, underscore)
	re := getCachedMapperRegex(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

	// Replace all matches with {param} format
	result := re.ReplaceAllString(path, "{$1}")

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

// schemaName applies the standard name replacer and optionally shortens the type name.
func schemaName(typeName string, cfg *APISpecConfig) string {
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
			var schema *Schema
			var schemas map[string]*Schema

			if typ == nil {
				keyParts := strings.Split(key, "-")
				if len(keyParts) > 1 {
					schema, schemas = mapGoTypeToOpenAPISchema(usedTypes, keyParts[1], meta, cfg, nil)
				}
			} else {
				schema, schemas = generateSchemaFromType(usedTypes, key, typ, meta, cfg, nil)
			}
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
	var pkgName string

	if !metadata.IsPrimitiveType(typeParts.PkgName) && typeParts.PkgName != "" {
		pkgName = typeParts.PkgName + "."
	}

	// Generics
	if len(typeParts.GenericTypes) > 0 {
		for _, part := range typeParts.GenericTypes {
			genericType := strings.Split(part, " ")
			if metadata.IsPrimitiveType(genericType[1]) {
				metaTypes[pkgName+genericType[0]+"-"+genericType[1]] = nil
			} else {
				genericTypeParts := TypeParts(genericType[0])

				if t := typeByName(genericTypeParts, meta); t != nil {
					metaTypes[pkgName+genericType[0]+"_"+genericType[1]] = t
				}
			}
		}
	}

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
		for _, part := range keyParts.GenericTypes {
			genericType := strings.Split(part, " ")
			if len(genericType) >= 2 {
				// Map type parameter name to concrete type (e.g., "T" → "pkg.User")
				genericTypes[genericType[0]] = genericType[1]
			} else {
				genericTypes[genericType[0]] = strings.ReplaceAll(part, " ", "-")
			}
		}
	}

	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
		Required:   []string{},
	}

	pkgName := getStringFromPool(meta, typ.Pkg)

	for _, field := range typ.Fields {
		fieldName := getStringFromPool(meta, field.Name)
		fieldType := getStringFromPool(meta, field.Type)

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
			// Handle nested struct type
			fieldOriginalType := getStringFromPool(meta, field.NestedType.Name)

			fieldSchema, newSchemas = generateSchemaFromType(usedTypes, fieldOriginalType, field.NestedType, meta, cfg, visitedTypes)
			if fieldSchema == nil {
				fieldSchema = newSchemas[fieldOriginalType]
			}

			maps.Copy(schemas, newSchemas)
		} else {
			isPrimitive := metadata.IsPrimitiveType(fieldType)

			if !isPrimitive && !strings.Contains(fieldType, ".") {
				re := getCachedMapperRegex(`((\[\])?\*?)(.+)$`)
				matches := re.FindStringSubmatch(fieldType)
				if len(matches) >= 4 {
					fieldType = matches[1] + pkgName + "." + matches[3]
				}
			}

			derivedFieldType := strings.TrimPrefix(fieldType, "*")
			// Check if this field type already exists in usedTypes
			if bodySchema, ok := usedTypes[derivedFieldType]; !isPrimitive && ok {
				// Create a reference to the existing schema
				fieldSchema = addRefSchemaForType(derivedFieldType)

				if bodySchema == nil {
					var newBodySchemas map[string]*Schema

					bodySchema, newBodySchemas = mapGoTypeToOpenAPISchema(usedTypes, fieldType, meta, cfg, visitedTypes)
					maps.Copy(schemas, newBodySchemas)
				}
				schemas[derivedFieldType] = bodySchema
				markUsedType(usedTypes, derivedFieldType, bodySchema)
			} else {
				fieldSchema, newSchemas = mapGoTypeToOpenAPISchema(usedTypes, derivedFieldType, meta, cfg, visitedTypes)
				if canAddRefSchemaForType(derivedFieldType) {
					schemas[derivedFieldType] = fieldSchema
					fieldSchema = addRefSchemaForType(derivedFieldType)
				}

				maps.Copy(schemas, newSchemas)
			}
		}

		// Apply validation constraints to the schema
		if validationConstraints != nil {
			applyValidationConstraints(fieldSchema, validationConstraints)

			// Add to required fields if marked as required
			if validationConstraints.Required {
				schema.Required = append(schema.Required, fieldName)
			}
		} else {
			// Fields without omitempty and without a "-" json tag are required
			// by default in Go (zero-value is always serialized).
			fieldTag := getStringFromPool(meta, field.Tag)
			if !hasOmitempty(fieldTag) {
				schema.Required = append(schema.Required, fieldName)
			}
		}

		// Detect and apply enum values from constants if no enum was specified in tags
		// Only apply enum detection for custom types (not built-in types)
		if fieldSchema != nil && len(fieldSchema.Enum) == 0 {
			// Use the original field type before resolution for enum detection
			originalFieldType := getStringFromPool(meta, field.Type)

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

	return schema, schemas
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
	schema, schemas := mapGoTypeToOpenAPISchema(usedTypes, underlyingType, meta, cfg, visitedTypes)

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

// applyValidationRule applies a single validation rule to constraints
func applyValidationRule(rule string, constraints *ValidationConstraints) {
	switch {
	case rule == "dive":
		constraints.Dive = true
	case rule == "required":
		constraints.Required = true
	case strings.HasPrefix(rule, "min="):
		if val, err := strconv.Atoi(strings.TrimPrefix(rule, "min=")); err == nil {
			constraints.Min = &[]float64{float64(val)}[0]
		}
	case strings.HasPrefix(rule, "max="):
		if val, err := strconv.Atoi(strings.TrimPrefix(rule, "max=")); err == nil {
			constraints.Max = &[]float64{float64(val)}[0]
		}
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

	// Apply string length constraints (only for string types)
	if schema.Type == "string" {
		if constraints.MinLength != nil {
			schema.MinLength = *constraints.MinLength
		}
		if constraints.MaxLength != nil {
			schema.MaxLength = *constraints.MaxLength
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

			// add package name to value type
			if startIdx > 0 {
				valueType = goType[:startIdx] + "." + valueType
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

func canAddRefSchemaForType(key string) bool {
	if metadata.IsPrimitiveType(key) || strings.HasPrefix(key, "[]") || strings.Contains(key, "map[") {
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
func lookupFuncComment(pkg *metadata.Package, funcName string, sp *metadata.StringPool) (string, string) {
	for _, file := range pkg.Files {
		if fn, exists := file.Functions[funcName]; exists {
			if comment := sp.GetString(fn.Comments); comment != "" {
				return splitDocComment(comment)
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

	// First pass: use route.Package if available (most precise).
	if route.Package != "" {
		if pkg, ok := route.Metadata.Packages[route.Package]; ok {
			if s, d := lookupFuncComment(pkg, funcName, sp); s != "" || d != "" {
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
			if s, d := lookupFuncComment(pkg, funcName, sp); s != "" || d != "" {
				return s, d
			}
		}
	}

	// Fallback: search all packages (for cases where package prefix doesn't match)
	for _, pkg := range route.Metadata.Packages {
		if s, d := lookupFuncComment(pkg, funcName, sp); s != "" || d != "" {
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
