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
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/antst/go-apispec/internal/metadata"
)

// Regex cache for performance optimization
var (
	regexCache = make(map[string]*regexp.Regexp)
	regexMutex sync.RWMutex
)

// getCachedRegex returns a cached compiled regex or compiles and caches a new one
func getCachedRegex(pattern string) (*regexp.Regexp, error) {
	regexMutex.RLock()
	if re, exists := regexCache[pattern]; exists {
		regexMutex.RUnlock()
		return re, nil
	}
	regexMutex.RUnlock()

	regexMutex.Lock()
	defer regexMutex.Unlock()

	// Double-check after acquiring write lock
	if re, exists := regexCache[pattern]; exists {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexCache[pattern] = re
	return re, nil
}

const (
	TypeSep    = "-->"
	defaultSep = "."
)

// findVacantStatusForBody picks the lowest status code in route.Response
// that hasn't yet been claimed by a body-bearing pattern. An entry is
// "vacant" only when BOTH BodyType and Schema are empty — entries with a
// schema set by expandHelperFunctionResponses (Schema != nil, BodyType == "")
// must NOT be picked, otherwise Render-method schemas mis-attribute to
// lower-numbered helper statuses (issue #30 — RejectedContentResponse
// leaking to 400 in alkem-io/file-service). Bodyless status codes
// (1xx/204/304) are also excluded per RFC 7231.
//
// Returns the status code and true on a successful pick; returns 0/false
// when no vacant slot exists.
func findVacantStatusForBody(route *RouteInfo) (int, bool) {
	for _, key := range slices.Sorted(maps.Keys(route.Response)) {
		resp := route.Response[key]
		if resp.BodyType == "" && resp.Schema == nil &&
			resp.StatusCode >= 100 && resp.StatusCode < 600 &&
			!isBodylessStatusCode(resp.StatusCode) {
			return resp.StatusCode, true
		}
	}
	return 0, false
}

// applyDetectedContentType propagates a handler-detected Content-Type onto
// route responses whose ContentType is still the default. Returns early when
// the detected type already equals the default (no-op assignment would skip
// the loop body anyway). Issue #33: skip bodyless status entries (1xx/204/304)
// — they will never carry a body in the emitted spec (per the mapper-side
// guard in buildResponses), so mutating their ContentType is wasted state
// that confuses any downstream inspector of RouteInfo. The override only
// matches entries whose ContentType is the default — preserving entries
// already pinned by pattern-specific DefaultContentType values (e.g.,
// http.Error → text/plain).
func applyDetectedContentType(route *RouteInfo, defaultCT string) {
	if route.detectedContentType == defaultCT {
		return
	}
	for _, resp := range route.Response {
		if isBodylessStatusCode(resp.StatusCode) {
			continue
		}
		if resp.ContentType == defaultCT {
			resp.ContentType = route.detectedContentType
		}
	}
}

// isBodylessStatusCode returns true for HTTP status codes that must not
// include a message body per RFC 7231: 1xx informational, 204 No Content,
// and 304 Not Modified.
func isBodylessStatusCode(code int) bool {
	return (code >= 100 && code < 200) || code == 204 || code == 304
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// schemasEqual checks if two schemas are structurally equivalent.
func schemasEqual(a, b *Schema) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Type != b.Type || a.Format != b.Format || a.Ref != b.Ref {
		return false
	}
	if !floatPtrEqual(a.Minimum, b.Minimum) || !floatPtrEqual(a.Maximum, b.Maximum) {
		return false
	}
	if a.Items != nil && b.Items != nil {
		return schemasEqual(a.Items, b.Items)
	}
	if a.Items != nil || b.Items != nil {
		return false
	}
	if a.AdditionalProperties != nil && b.AdditionalProperties != nil {
		return schemasEqual(a.AdditionalProperties, b.AdditionalProperties)
	}
	if a.AdditionalProperties != nil || b.AdditionalProperties != nil {
		return false
	}
	return true
}

// RouteInfo represents extracted route information
type RouteInfo struct {
	Path        string
	MountPath   string
	Method      string
	Handler     string
	Package     string
	File        string
	Function    string
	Summary     string
	Description string
	Tags        []string
	Request     *RequestInfo
	Response    map[string]*ResponseInfo
	Params      []Parameter

	UsedTypes map[string]*Schema
	Metadata  *metadata.Metadata

	// Resolved router group prefix (if any)
	GroupPrefix string

	// Content-Type detected from Header().Set("Content-Type", value) calls
	detectedContentType string

	// SecurityScheme, when set, names the security scheme this route uses
	// (e.g. "bearerAuth"). The scheme definition itself is held centrally
	// on the generator config — multiple routes that use the same auth
	// pattern share a single Components.securitySchemes entry. nil means
	// no auth was detected for this route.
	SecurityScheme *DetectedSecurityScheme
}

// DetectedSecurityScheme captures the result of scanning a handler's call
// graph for an Authorization-header check. Name is the chosen schema key
// (under components.securitySchemes); Scheme is the inferred shape.
type DetectedSecurityScheme struct {
	Name   string
	Scheme SecurityScheme
}

func NewRouteInfo() *RouteInfo {
	return &RouteInfo{
		Response:  make(map[string]*ResponseInfo),
		UsedTypes: make(map[string]*Schema),
	}
}

// IsValid checks if the route info is valid
func (r *RouteInfo) IsValid() bool {
	return r.Path != "" && r.Handler != ""
}

// RequestInfo represents request information
type RequestInfo struct {
	ContentType string
	BodyType    string
	// BodyTypeRef is the structured resolved body type (Phase 3) consumed by
	// schema generation; BodyType (the string) stays for component naming.
	BodyTypeRef *metadata.TypeRef
	Schema      *Schema

	// Required indicates whether the OpenAPI requestBody.required flag should be
	// set. When a request-body pattern matches (json.Decode, c.Bind, etc.) the
	// body must arrive populated — handlers that decode it are deterministic
	// 400s on empty input — so the matcher sets this true.
	Required bool

	// DecodeTargetVar is the local variable name the request body decodes
	// into (e.g., `body` in `json.NewDecoder(r.Body).Decode(&body)`). Used
	// to drive field-level converter inference: any later access to
	// `<DecodeTargetVar>.<FieldName>` consumed by a known converter back-
	// propagates schema type/format onto that struct field.
	DecodeTargetVar string
}

// ResponseInfo represents response information
type ResponseInfo struct {
	StatusCode  int
	ContentType string
	BodyType    string
	// BodyTypeRef is the structured resolved body type (Phase 3) consumed by
	// schema generation; BodyType (the string) stays for component naming.
	BodyTypeRef *metadata.TypeRef
	Schema      *Schema
	// AlternativeSchemas holds additional schemas when multiple response
	// types share the same status code (e.g., ErrorResponse and map[string]string
	// both returned on 400). These get wrapped in oneOf during serialization.
	AlternativeSchemas []*Schema
	// Branch context from CFG analysis (nil = unconditional)
	Branch *metadata.BranchContext
}

// Extractor provides a cleaner, more modular approach to extraction
type Extractor struct {
	tree            TrackerTreeInterface
	cfg             *APISpecConfig
	contextProvider ContextProvider
	schemaMapper    SchemaMapper
	overrideApplier OverrideApplier

	// Pattern matchers
	routeMatchers    []RoutePatternMatcher
	mountMatchers    []MountPatternMatcher
	requestMatchers  []RequestPatternMatcher
	responseMatchers []ResponsePatternMatcher
	paramMatchers    []ParamPatternMatcher

	warnings *WarningSink // non-fatal analysis warnings → stderr (spec 009, FR-008/FR-012)
}

// warn records a non-fatal analysis warning (lazily creating a stderr sink). Used by
// the helper-binding degrade path (FR-012).
func (e *Extractor) warn(pos, msg string) {
	if e == nil {
		return
	}
	if e.warnings == nil {
		e.warnings = NewWarningSink()
	}
	e.warnings.Warn(pos, msg)
}

// isLikelyMediaType checks if a string looks like a valid MIME type (type/subtype).
// Returns false for Go variable/field paths like "model.Document.MimeType" or
// fully-qualified Go paths like "github.com/org/pkg/model.Document.MimeType".
func isLikelyMediaType(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	// Strip parameters (e.g., "text/plain; charset=utf-8" → "text/plain")
	if idx := strings.IndexByte(v, ';'); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	// A valid MIME type has exactly one "/" separating type and subtype.
	// Go import paths have multiple slashes (github.com/org/pkg/...).
	if strings.Count(v, "/") != 1 {
		return false
	}
	parts := strings.SplitN(v, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	// MIME type parts don't contain dots (e.g., "application" not "model.Document").
	// Go field paths always have dots. Exception: vendor MIME subtypes like
	// "vnd.api+json" are valid, so only check the type (left of /).
	if strings.Contains(parts[0], ".") {
		return false
	}
	return true
}

// checkContentTypePattern checks if a node matches a Content-Type header set pattern
// (e.g., w.Header().Set("Content-Type", "image/png")) and stores the detected
// content type on all route responses.
func (e *Extractor) checkContentTypePattern(node TrackerNodeInterface, route *RouteInfo) {
	edge := node.GetEdge()
	if edge == nil {
		return
	}
	callName := e.contextProvider.GetString(edge.Callee.Name)
	recvType := e.contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := e.contextProvider.GetString(edge.Callee.Pkg)

	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

	for _, pattern := range e.cfg.Framework.ContentTypePatterns {
		if pattern.CallRegex != "" {
			re, err := getCachedRegex(pattern.CallRegex)
			if err != nil || !re.MatchString(callName) {
				continue
			}
		}
		if pattern.RecvTypeRegex != "" {
			re, err := getCachedRegex(pattern.RecvTypeRegex)
			if err != nil || !re.MatchString(fqRecvType) {
				continue
			}
		}
		if len(edge.Args) > pattern.HeaderNameArgIndex {
			headerName := e.contextProvider.GetArgumentInfo(edge.Args[pattern.HeaderNameArgIndex])
			headerName = strings.Trim(headerName, "\"")
			if strings.EqualFold(headerName, "Content-Type") && len(edge.Args) > pattern.HeaderValueArgIndex {
				val := e.contextProvider.GetArgumentInfo(edge.Args[pattern.HeaderValueArgIndex])
				val = strings.Trim(val, "\"")
				// Validate the value looks like a MIME type (type/subtype).
				// Variable or field paths (e.g., doc.MimeType) don't contain "/"
				// and should fall back to application/octet-stream.
				if val != "" && !isLikelyMediaType(val) {
					val = "application/octet-stream"
				}
				if val != "" {
					// Store the detected value, then propagate to existing
					// responses via the shared helper — which (a) skips bodyless
					// status entries per issue #33 and (b) preserves entries
					// already pinned by pattern-specific DefaultContentType
					// values (e.g., http.Error → text/plain).
					route.detectedContentType = val
					applyDetectedContentType(route, e.cfg.Defaults.ResponseContentType)
				}
			}
		}
	}
}

// NewExtractor creates a new refactored extractor
func NewExtractor(tree TrackerTreeInterface, cfg *APISpecConfig) *Extractor {
	contextProvider := NewContextProvider(tree.GetMetadata())
	schemaMapper := NewSchemaMapper(cfg)
	overrideApplier := NewOverrideApplier(cfg)

	extractor := &Extractor{
		tree:            tree,
		cfg:             cfg,
		contextProvider: contextProvider,
		schemaMapper:    schemaMapper,
		overrideApplier: overrideApplier,
	}

	// Initialize pattern matchers
	extractor.initializePatternMatchers()

	return extractor
}

// initializePatternMatchers initializes all pattern matchers
func (e *Extractor) initializePatternMatchers() {
	// Initialize route matchers
	for _, pattern := range e.cfg.Framework.RoutePatterns {
		matcher := NewRoutePatternMatcher(pattern, e.cfg, e.contextProvider)
		e.routeMatchers = append(e.routeMatchers, matcher)
	}

	// Initialize mount matchers
	for _, pattern := range e.cfg.Framework.MountPatterns {
		matcher := NewMountPatternMatcher(pattern, e.cfg, e.contextProvider)
		e.mountMatchers = append(e.mountMatchers, matcher)
	}

	// Initialize request matchers
	for _, pattern := range e.cfg.Framework.RequestBodyPatterns {
		matcher := NewRequestPatternMatcher(pattern, e.cfg, e.contextProvider)
		e.requestMatchers = append(e.requestMatchers, matcher)
	}

	// Initialize response matchers
	for _, pattern := range e.cfg.Framework.ResponsePatterns {
		matcher := NewResponsePatternMatcher(pattern, e.cfg, e.contextProvider)
		e.responseMatchers = append(e.responseMatchers, matcher)
	}

	// Initialize param matchers
	for _, pattern := range e.cfg.Framework.ParamPatterns {
		matcher := NewParamPatternMatcher(pattern, e.cfg, e.contextProvider)
		e.paramMatchers = append(e.paramMatchers, matcher)
	}
}

// ExtractRoutes extracts all routes from the tracker tree
func (e *Extractor) ExtractRoutes() []*RouteInfo {
	routes := make([]*RouteInfo, 0)
	for _, root := range e.tree.GetRoots() {
		e.traverseForRoutes(root, "", nil, &routes)
	}
	return routes
}

// traverseForRoutes traverses the tree to find routes
func (e *Extractor) traverseForRoutes(node TrackerNodeInterface, mountPath string, mountTags []string, routes *[]*RouteInfo) {
	e.traverseForRoutesWithVisited(node, mountPath, mountTags, routes, make(map[string]bool))
}

// traverseForRoutesWithVisited traverses with visited tracking to prevent cycles
func (e *Extractor) traverseForRoutesWithVisited(node TrackerNodeInterface, mountPath string, mountTags []string, routes *[]*RouteInfo, visited map[string]bool) {
	if node == nil {
		return
	}

	// Prevent infinite recursion
	nodeKey := node.GetKey()
	if visited[nodeKey] {
		return
	}
	visited[nodeKey] = true

	routeInfo := NewRouteInfo()

	// Check for mount patterns first
	if mountInfo, isMount := e.executeMountPattern(node); isMount {
		e.handleMountNode(node, mountInfo, mountPath, mountTags, routes, visited)
	} else if isRoute := e.executeRoutePattern(node, routeInfo); isRoute {
		// Check for route patterns
		e.handleRouteNode(node, routeInfo, mountPath, mountTags, routes)
	} else {
		// Continue traversing children
		for _, child := range node.GetChildren() {
			e.traverseForRoutesWithVisited(child, mountPath, mountTags, routes, visited)
		}
	}
}

// executeMountPattern executes mount pattern matching
func (e *Extractor) executeMountPattern(node TrackerNodeInterface) (MountInfo, bool) {
	var bestMatch MountInfo
	var bestPriority int
	var found bool

	for _, matcher := range e.mountMatchers {
		if matcher.MatchNode(node) {
			priority := matcher.GetPriority()
			if !found || priority > bestPriority {
				mountInfo := matcher.ExtractMount(node)
				bestMatch = mountInfo
				bestPriority = priority
				found = true
			}
		}
	}

	return bestMatch, found
}

// executeRoutePattern executes route pattern matching
func (e *Extractor) executeRoutePattern(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	var bestPriority int
	var found bool

	for _, matcher := range e.routeMatchers {
		if matcher.MatchNode(node) {
			priority := matcher.GetPriority()
			if !found || priority > bestPriority {
				found = matcher.ExtractRoute(node, routeInfo)
				if found {
					bestPriority = priority
				}
			}
		}
	}

	return found
}

// handleMountNode handles a mount node
func (e *Extractor) handleMountNode(node TrackerNodeInterface, mountInfo MountInfo, mountPath string, mountTags []string, routes *[]*RouteInfo, visited map[string]bool) {
	// Update mount path if needed
	if mountInfo.Path != "" {
		if mountPath == "" || !strings.HasSuffix(mountPath, mountInfo.Path) {
			mountPath = joinPaths(mountPath, mountInfo.Path)
		}
	}

	// Handle router assignment if present
	if mountInfo.Assignment != nil {
		e.handleRouterAssignment(mountInfo, mountPath, mountTags, routes, visited)
	} else if mountInfo.RouterArg != nil {
		// Variable-based mount: the router arg is a variable (e.g., apiMux in
		// rootMux.Handle("/api/", apiMux)). Search the call graph for edges
		// where this variable is the receiver, and traverse them with the mount path.
		e.handleVariableMount(mountInfo.RouterArg, mountPath, mountTags, routes)
	}

	// Continue traversing children.
	// Special handling for StripPrefix: when a mount wraps the child mux in
	// http.StripPrefix(prefix, childMux), the StripPrefix node's children
	// include the actual mux variable. Extract it and resolve via handleVariableMount.
	for _, child := range node.GetChildren() {
		childKey := child.GetKey()
		if strings.Contains(childKey, "net/http.StripPrefix") {
			for _, spChild := range child.GetChildren() {
				if arg := spChild.GetArgument(); arg != nil && arg.GetName() != "" {
					e.handleVariableMount(arg, mountPath, mountTags, routes)
				}
			}
			continue
		}
		var newTags []string
		if mountPath != "" {
			newTags = []string{mountPath}
		} else {
			newTags = mountTags
		}
		e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, visited)
	}
}

// handleRouteNode handles a route node
func (e *Extractor) handleRouteNode(node TrackerNodeInterface, routeInfo *RouteInfo, mountPath string, mountTags []string, routes *[]*RouteInfo) {
	// Prepend mount path if present
	if mountPath != "" {
		routeInfo.MountPath = joinPaths(mountPath, routeInfo.MountPath)
	}

	// Set tags from mountTags if present
	if len(mountTags) > 0 {
		routeInfo.Tags = mountTags
	}

	// Extract route/request/response/params from children with visited edges tracking
	visitedEdges := make(map[string]bool)
	e.extractRouteChildren(node, routeInfo, mountTags, routes, visitedEdges)

	// If no responses were found and the handler is an interface method,
	// try to resolve to the concrete implementation and re-extract.
	// Only trigger for types confirmed as interfaces in the metadata.
	if len(routeInfo.Response) == 0 && routeInfo.Function != "" {
		if e.isInterfaceHandler(routeInfo) {
			e.resolveInterfaceHandler(node, routeInfo, mountTags, routes, visitedEdges)
		}
	}

	// Override response Content-Type if handler sets it via Header().Set().
	// Only apply to responses using the default content type — don't override
	// responses that already have a pattern-specific type (e.g., http.Error
	// sets "text/plain; charset=utf-8" via DefaultContentType on the pattern).
	if routeInfo.detectedContentType != "" {
		applyDetectedContentType(routeInfo, e.cfg.Defaults.ResponseContentType)
	}

	// Detect conditional HTTP methods from CFG branch context.
	// If responses have switch-case branch contexts with HTTP method case values,
	// split into separate RouteInfo entries per method.
	if methodRoutes := e.splitByConditionalMethods(routeInfo); len(methodRoutes) > 0 {
		for _, mr := range methodRoutes {
			e.overrideApplier.ApplyOverrides(mr)
			if mr.IsValid() && routes != nil {
				*routes = append(*routes, mr)
			}
		}
		return
	}

	// Apply overrides
	e.overrideApplier.ApplyOverrides(routeInfo)

	if routeInfo.IsValid() && routes != nil {
		// Update existing route or add new one
		var found bool
		for i := range *routes {
			if (*routes)[i].Function == routeInfo.Function {
				(*routes)[i] = routeInfo
				found = true
				break
			}
		}
		if !found {
			*routes = append(*routes, routeInfo)
		}
	}
}

// handleRouterAssignment handles router assignment for mounts
func (e *Extractor) handleRouterAssignment(mountInfo MountInfo, mountPath string, mountTags []string, routes *[]*RouteInfo, visited map[string]bool) {
	// Find the target node for the assignment
	targetNode := e.findTargetNode(mountInfo.Assignment)
	if targetNode != nil {
		for _, child := range targetNode.GetChildren() {
			var newTags []string
			if mountPath != "" {
				newTags = []string{mountPath}
			} else {
				newTags = mountTags
			}
			e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, visited)
		}
	}
}

// handleVariableMount handles the case where a mount's router argument is a variable
// (e.g., rootMux.Handle("/api/", apiMux)). It searches the call graph for edges
// where this variable is the receiver, finds the corresponding tracker tree nodes,
// and traverses them with the accumulated mount path.
func (e *Extractor) handleVariableMount(routerArg *metadata.CallArgument, mountPath string, mountTags []string, routes *[]*RouteInfo) {
	if routerArg == nil {
		return
	}
	varName := routerArg.GetName()
	if varName == "" {
		return
	}

	// Find the NewServeMux creation node for this variable in the tree.
	// The creation node (e.g., net/http.NewServeMux@main.go:38:12) has the
	// route registrations (HandleFunc, Handle) as its children in the tracker tree.
	// Use a fresh visited map so nodes already visited without mount context
	// can be re-traversed with the mount path prefix.
	freshVisited := make(map[string]bool)
	e.tree.TraverseTree(func(treeNode TrackerNodeInterface) bool {
		edge := treeNode.GetEdge()
		if edge == nil {
			return true
		}
		if edge.CalleeRecvVarName == varName {
			// Found the creation node for this variable — traverse its children
			// with the mount path
			for _, child := range treeNode.GetChildren() {
				var newTags []string
				if mountPath != "" {
					newTags = []string{mountPath}
				} else {
					newTags = mountTags
				}
				e.traverseForRoutesWithVisited(child, mountPath, newTags, routes, freshVisited)
			}
			return false // found it, stop searching
		}
		return true
	})
}

// findTargetNode finds the target node for an assignment
func (e *Extractor) findTargetNode(assignment *metadata.CallArgument) TrackerNodeInterface {
	if assignment == nil {
		return nil
	}

	// Use breadth-first search to find the target node
	queue := e.tree.GetRoots()
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:] // dequeue

		if node.GetKey() == assignment.ID() {
			return node
		}

		queue = append(queue, node.GetChildren()...)
	}

	return nil
}

// resolveCallReturnValue looks up a function's constant return value
// from the metadata. Returns the constant string or empty if not found.
func (r *ResponsePatternMatcherImpl) resolveCallReturnValue(arg *metadata.CallArgument) string {
	var funcName string
	if arg.Fun != nil {
		funcName = r.contextProvider.GetArgumentInfo(arg.Fun)
	}
	if funcName == "" {
		return ""
	}
	// Strip package prefix
	if idx := strings.LastIndex(funcName, "."); idx >= 0 {
		funcName = funcName[idx+1:]
	}
	// The ContextProvider wraps metadata — use the GetFunctionConstantReturn
	// method if available, or search through metadata directly.
	if cp, ok := r.contextProvider.(*ContextProviderImpl); ok && cp.meta != nil {
		for _, pkg := range cp.meta.Packages {
			for _, file := range pkg.Files {
				if fn, exists := file.Functions[funcName]; exists && fn.ConstantReturnValue != "" {
					return fn.ConstantReturnValue
				}
			}
		}
	}
	return ""
}

// resolveParamArgStatus walks up the tracker tree parent chain to find a
// ParamArgMap that maps a function parameter name to the actual argument
// passed by the caller. This handles error helper patterns like:
//
//	writeJSONError(w, http.StatusBadRequest, "msg")
//	func writeJSONError(w http.ResponseWriter, code int, msg string) {
//	    w.WriteHeader(code)  // ← code is a parameter, not a local variable
//	}
func (r *ResponsePatternMatcherImpl) resolveParamArgStatus(node TrackerNodeInterface, paramName string) (int, bool) {
	// Walk up parent chain to find the call edge that invoked this function
	for parent := node.GetParent(); parent != nil; parent = parent.GetParent() {
		parentEdge := parent.GetEdge()
		if parentEdge == nil {
			continue
		}
		if arg, exists := parentEdge.ParamArgMap[paramName]; exists {
			// Found the parameter mapping — resolve the argument value
			argStr := r.contextProvider.GetArgumentInfo(&arg)
			if status, ok := r.schemaMapper.MapStatusCode(argStr); ok {
				return status, true
			}
		}
	}
	return 0, false
}

// resolveParamArgType walks up the tracker tree parent chain to find a
// ParamArgMap entry for a function parameter and returns the concrete type
// of the argument passed by the caller. This handles patterns like:
//
//	respondJSON(w, 201, user)
//	func respondJSON(w http.ResponseWriter, code int, data interface{}) {
//	    json.Encode(data)  // ← data is interface{}, but user is User
//	}
func (r *ResponsePatternMatcherImpl) resolveParamArgType(node TrackerNodeInterface, paramName string) (string, *metadata.TypeRef) {
	for parent := node.GetParent(); parent != nil; parent = parent.GetParent() {
		parentEdge := parent.GetEdge()
		if parentEdge == nil {
			continue
		}
		if arg, exists := parentEdge.ParamArgMap[paramName]; exists {
			// Byte-identical to main: GetArgumentInfo's fully-qualified string IS the
			// param's type, and we thread ParseTypeRef of that exact string — the PR's
			// "ParseTypeRef of the string the call sites pass" design — so schemaForType
			// consumes the ref without its own re-parse (Phase 3).
			//
			// We deliberately do NOT source the string natively from arg.TypeRef.String()
			// or a concrete arg.GetResolvedType() here (the reverted Phase-4 D6 fast-paths).
			// Both render the canonical "."-separated form, but the resolution layer — and
			// main — carry the "-->" TypeSep form. resolveParamArgType feeds the
			// SEPARATOR-SENSITIVE cleanOverrideType (via resolveOverrideGoType,
			// wrapper_specialisation.go), whose `ContainsAny(t, "./[")` test ACCEPTS
			// "pkg.Order" but REJECTS "pkg-->Order"; a native "." string there flips the
			// wrapper allOf override on for a payload where main emitted a bare $ref — a
			// byte-identical break confirmed by the PR #62 full review (golden-invisible:
			// the same-package envelope shape isn't in the corpus). GetArgumentInfo keeps
			// every resolution-layer consumer aligned with main; the threaded ParseTypeRef
			// still spares schemaForType its re-parse. (The body consumer normalizes
			// "."/"-->" identically, which is why this hid behind a clean golden.)
			if info := r.contextProvider.GetArgumentInfo(&arg); info != "" && info != "interface{}" && info != "any" {
				return info, metadata.ParseTypeRef(info)
			}
			// Fallback to the raw arg type, likewise threaded.
			if argType := arg.GetType(); argType != "" && argType != "interface{}" && argType != "any" {
				return argType, metadata.ParseTypeRef(argType)
			}
		}
	}
	return "", nil
}

// writeDestTracesToResponseWriter reports whether a free-function write's
// destination argument is, or derives from, an http.ResponseWriter — directly
// (its type names http.ResponseWriter), through a local assignment in the
// write's own function (dst := …), or through the parameter the caller bound it
// to. It rejects writes to files, buffers and other non-response writers that
// merely happen to be reachable from a route handler — e.g. file-service's
// storage adapter io.Copy(dst, src) copying an upload to an *os.File (issue
// #52). The bias is toward precision: an unresolved destination is rejected, so
// a phantom binary response is never emitted (and can never flap on map order).
func (r *ResponsePatternMatcherImpl) writeDestTracesToResponseWriter(node TrackerNodeInterface, dst *metadata.CallArgument) bool {
	if dst == nil {
		return false
	}
	if argReferencesResponseWriter(dst) {
		return true
	}
	if dst.GetKind() != metadata.KindIdent {
		return false
	}
	// Local assignment within the writing function (dst := <expr>).
	if edge := node.GetEdge(); edge != nil {
		if assigns, ok := edge.AssignmentMap[dst.GetName()]; ok {
			for i := range assigns {
				if argReferencesResponseWriter(&assigns[i].Value) {
					return true
				}
			}
		}
	}
	// Parameter the caller bound to a response writer (helper(w io.Writer)
	// invoked with the handler's http.ResponseWriter).
	if t, _ := r.resolveParamArgType(node, dst.GetName()); strings.Contains(t, "http.ResponseWriter") {
		return true
	}
	return false
}

// argReferencesResponseWriter walks a CallArgument expression tree looking for a
// value typed http.ResponseWriter (or *http.ResponseWriter). Recurses through
// selector/call expressions so c.Response().Writer and similar are recognised.
func argReferencesResponseWriter(arg *metadata.CallArgument) bool {
	if arg == nil {
		return false
	}
	if strings.Contains(arg.GetType(), "http.ResponseWriter") {
		return true
	}
	if argReferencesResponseWriter(arg.X) || argReferencesResponseWriter(arg.Fun) || argReferencesResponseWriter(arg.Sel) {
		return true
	}
	for _, a := range arg.Args {
		if argReferencesResponseWriter(a) {
			return true
		}
	}
	return false
}

// isInterfaceHandler checks if the route handler's receiver type is an interface.
func (e *Extractor) isInterfaceHandler(route *RouteInfo) bool {
	meta := e.tree.GetMetadata()
	if meta == nil {
		return false
	}
	sp := meta.StringPool

	// Extract the type name from the function (e.g., "pkg-->ContentServer.Serve" → "ContentServer")
	funcName := route.Function
	parts := strings.Split(funcName, TypeSep)
	if len(parts) < 2 {
		return false
	}
	methodPart := parts[len(parts)-1]
	dotIdx := strings.LastIndex(methodPart, ".")
	if dotIdx <= 0 {
		return false
	}
	typeName := methodPart[:dotIdx]

	// Check if this type is an interface in the metadata
	for _, pkg := range meta.Packages {
		for name, typ := range pkg.Types {
			if name == typeName && sp.GetString(typ.Kind) == "interface" {
				return true
			}
		}
	}
	return false
}

// resolveInterfaceHandler checks if the route handler is an interface method.
// If so, searches the call graph for concrete implementations with the same
// method name and extracts responses from their call edges directly.
func (e *Extractor) resolveInterfaceHandler(_ TrackerNodeInterface, route *RouteInfo, _ []string, _ *[]*RouteInfo, _ map[string]bool) {
	funcName := route.Function
	if idx := strings.LastIndex(funcName, "."); idx >= 0 {
		funcName = funcName[idx+1:]
	}
	if funcName == "" {
		return
	}

	meta := e.tree.GetMetadata()
	if meta == nil {
		return
	}

	// Search the call graph for edges where the caller is a concrete method
	// with the same name AND in the same package as the route handler.
	routePkg := route.Package
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		callerName := e.contextProvider.GetString(edge.Caller.Name)
		callerRecvType := e.contextProvider.GetString(edge.Caller.RecvType)
		callerPkg := e.contextProvider.GetString(edge.Caller.Pkg)

		if callerName != funcName || callerRecvType == "" {
			continue
		}
		// Only match concrete methods in the same package as the route
		if routePkg != "" && !strings.Contains(callerPkg, routePkg) && !strings.Contains(routePkg, callerPkg) {
			continue
		}

		// Found a call from the concrete implementation — check if it's
		// a response-writing call (Encode, Write, JSON, etc.)
		calleeName := e.contextProvider.GetString(edge.Callee.Name)
		for _, matcher := range e.responseMatchers {
			// Create a minimal node wrapper for this edge
			mockNode := &callGraphEdgeNode{edge: edge}
			if matcher.MatchNode(mockNode) {
				for _, resp := range matcher.ExtractResponse(mockNode, route) {
					if resp != nil && (resp.BodyType != "" || resp.StatusCode != 0) {
						route.Response[fmt.Sprintf("%d", resp.StatusCode)] = resp
					}
				}
			}
			_ = calleeName
		}
	}
}

// callGraphEdgeNode wraps a CallGraphEdge to implement TrackerNodeInterface
// for use in pattern matching against raw call graph edges.
type callGraphEdgeNode struct {
	edge *metadata.CallGraphEdge
}

func (n *callGraphEdgeNode) GetEdge() *metadata.CallGraphEdge                       { return n.edge }
func (n *callGraphEdgeNode) GetKey() string                                         { return "" }
func (n *callGraphEdgeNode) GetChildren() []TrackerNodeInterface                    { return nil }
func (n *callGraphEdgeNode) GetParent() TrackerNodeInterface                        { return nil }
func (n *callGraphEdgeNode) GetArgument() *metadata.CallArgument                    { return nil }
func (n *callGraphEdgeNode) GetTypeParamMap() map[string]string                     { return nil }
func (n *callGraphEdgeNode) GetArgType() metadata.ArgumentType                      { return 0 }
func (n *callGraphEdgeNode) GetArgIndex() int                                       { return -1 }
func (n *callGraphEdgeNode) GetArgContext() string                                  { return "" }
func (n *callGraphEdgeNode) GetRootAssignmentMap() map[string][]metadata.Assignment { return nil }

// splitByConditionalMethods checks if a route's responses have CFG branch
// context with HTTP method case values — a `switch r.Method { case "GET": … }`
// OR an `if r.Method == http.MethodPost { … }` guard, both of which record the
// method in CaseValues (spec 009, US2). If so, returns separate RouteInfo entries
// per method. Returns nil if no conditional methods are detected.
//
// Every other response is attributed to the per-method operations using the
// per-function CFG (spec 009): an unconditional response, or an *independent*
// conditional (an `if cond { … }` orthogonal to the method dispatch), is reachable
// whatever the method ran and so is carried onto EVERY operation; a conditional
// *nested inside* a method arm (e.g. a 404 in the `case GET:` body) is carried onto
// that one method; and the dispatch FALLBACK arm — a `switch r.Method` default or a
// bare `else` of an `if r.Method ==`, whose branch point is shared with the method
// arms — is excluded from the handled methods (a 405 there is not a response of GET
// or POST). When the handler has no CFG model, non-method conditionals are
// conservatively excluded (the pre-009 behavior).
//
// Known limitation: the dispatch fallback arm is not emitted as its OWN operation
// (it has no single HTTP method to attach to). Chain explicit arms
// (`else if r.Method == http.MethodGet`) to get an operation for each method.
func (e *Extractor) splitByConditionalMethods(route *RouteInfo) []*RouteInfo {
	// Collect HTTP methods from response branch contexts
	methodResponses := make(map[string]map[string]*ResponseInfo) // method → statusCode → response

	for statusCode, resp := range route.Response {
		if resp.Branch == nil || len(resp.Branch.CaseValues) == 0 {
			continue
		}
		for _, val := range resp.Branch.CaseValues {
			method := strings.ToUpper(val)
			if !isValidHTTPMethodStr(method) {
				continue
			}
			if methodResponses[method] == nil {
				methodResponses[method] = make(map[string]*ResponseInfo)
			}
			methodResponses[method][statusCode] = resp
		}
	}

	if len(methodResponses) < 2 {
		return nil // Not enough methods to split
	}

	// Classify every response NOT already attributed to a method, using the
	// per-function CFG (spec 009). See the doc comment for the cases. fnKey == ""
	// (no model) degrades to excluding non-method conditionals, never leaking a
	// fallback onto a method.
	meta := e.tree.GetMetadata()
	fnKey := splitRouteFnKey(meta, methodResponses)
	armBlocks, armBlockToMethods := dispatchArms(methodResponses)
	dispatchRoot, haveRoot := commonDominator(meta, fnKey, armBlocks)

	shared := make(map[string]*ResponseInfo)
	for statusCode, resp := range route.Response {
		if resp.Branch == nil {
			shared[statusCode] = resp // unconditional → on every method
			continue
		}
		if branchNamesMethod(resp.Branch.CaseValues) {
			continue // already attributed to its method(s) above
		}
		if fnKey == "" {
			continue // no CFG model: conservatively exclude (pre-009 behavior)
		}
		rb := resp.Branch.BlockIndex
		if methods, ok := dominatingMethods(meta, fnKey, rb, armBlocks, armBlockToMethods); ok {
			for _, m := range methods { // a combined `case "GET", "HEAD"` arm owns several
				if methodResponses[m] != nil {
					methodResponses[m][statusCode] = resp // nested inside that arm
				}
			}
			continue
		}
		if isDispatchFallback(meta, fnKey, resp.Branch, dispatchRoot, haveRoot, armBlocks) {
			continue // a `switch` default / `if r.Method ==` bare else → "any other method"
		}
		shared[statusCode] = resp // independent conditional → on every method
	}

	var result []*RouteInfo
	for method, responses := range methodResponses {
		merged := make(map[string]*ResponseInfo, len(shared)+len(responses))
		for s, r := range shared {
			merged[s] = r
		}
		for s, r := range responses { // method-specific wins on any status overlap
			merged[s] = r
		}
		mr := &RouteInfo{
			Path:                route.Path,
			MountPath:           route.MountPath,
			Method:              method,
			Handler:             route.Handler,
			Package:             route.Package,
			File:                route.File,
			Function:            route.Function,
			Summary:             route.Summary,
			Description:         route.Description,
			Tags:                route.Tags,
			Request:             route.Request,
			Response:            merged,
			Params:              route.Params,
			UsedTypes:           route.UsedTypes,
			Metadata:            route.Metadata,
			GroupPrefix:         route.GroupPrefix,
			detectedContentType: route.detectedContentType,
		}
		result = append(result, mr)
	}
	return result
}

// splitRouteFnKey resolves the FunctionCFGs key for the handler being split, from
// any method branch's parent-statement position (registered in cfgPosToFn during
// CFG annotation). Returns "" when no model is available — the caller then degrades
// to the pre-009 behavior (non-method conditionals excluded).
func splitRouteFnKey(meta *metadata.Metadata, methodResponses map[string]map[string]*ResponseInfo) string {
	for _, resps := range methodResponses {
		for _, r := range resps {
			if r.Branch == nil {
				continue
			}
			if k := meta.FnKeyForPos(meta.StringPool.GetString(r.Branch.ParentStmtPos)); k != "" {
				return k
			}
		}
	}
	return ""
}

// dispatchArms returns the sorted method-arm branch blocks and each block → the
// HTTP method(s) it serves. A combined `case "GET", "HEAD":` lowers to ONE block
// that serves both methods, so the value is a slice (mapping it to a single method
// would be last-writer-wins over map order — nondeterministic). Both the block list
// and each method list are sorted so downstream attribution is deterministic.
func dispatchArms(methodResponses map[string]map[string]*ResponseInfo) ([]int32, map[int32][]string) {
	armBlockToMethods := make(map[int32][]string)
	for method, resps := range methodResponses {
		for _, r := range resps {
			if r.Branch == nil {
				continue
			}
			b := r.Branch.BlockIndex
			if !slices.Contains(armBlockToMethods[b], method) {
				armBlockToMethods[b] = append(armBlockToMethods[b], method)
			}
		}
	}
	blocks := make([]int32, 0, len(armBlockToMethods))
	for b := range armBlockToMethods {
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	for _, ms := range armBlockToMethods {
		sort.Strings(ms)
	}
	return blocks, armBlockToMethods
}

// commonDominator returns the lowest common dominator of the given blocks — the
// "dispatch root" that every method arm descends from (the `switch r.Method` tag or
// the first `if r.Method ==` condition). ok=false when there is no CFG model. go/cfg
// lowers a switch into a chain of test blocks, so the arms' immediate dominators
// differ; their common dominator is the stable root shared by every arm INCLUDING
// the default. Walks immediate-dominator chains (Cooper-Harvey-Kennedy idom tree).
func commonDominator(meta *metadata.Metadata, fnKey string, blocks []int32) (int32, bool) {
	if fnKey == "" || len(blocks) == 0 {
		return -1, false
	}
	cur := blocks[0]
	for _, b := range blocks[1:] {
		anc := make(map[int32]bool)
		for x := cur; ; {
			anc[x] = true
			p, ok := meta.IDom(fnKey, x)
			if !ok {
				break // reached the entry (it dominates everything)
			}
			x = p
		}
		y := b
		for !anc[y] {
			p, ok := meta.IDom(fnKey, y)
			if !ok {
				break
			}
			y = p
		}
		cur = y
	}
	return cur, true
}

// isDispatchFallback reports whether a non-method conditional response (branch br)
// must be EXCLUDED from the handled methods: either a `switch` `default:` arm
// (recognised structurally by its empty case values, so a stray `fallthrough` into
// it cannot leak its 405 onto a method), or a fallback that descends from the
// dispatch root yet shares no control-flow path with any arm (the bare `else` of an
// `if r.Method ==` chain). A conditional outside the dispatch (root does not
// dominate it, e.g. a pre-dispatch `if bad { … return }`) or one reachable together
// with the arms is NOT a fallback — it is independent and shared.
//
// Limitation: the structural `default:` rule also matches the default arm of an
// UNRELATED switch dominated by the dispatch root (e.g. an orthogonal
// `switch mode { default: … }` after the dispatch); such a response is excluded
// rather than shared. This is conservative, not a regression — the pre-CFG split
// dropped every non-unconditional response here regardless.
func isDispatchFallback(meta *metadata.Metadata, fnKey string, br *metadata.BranchContext, dispatchRoot int32, haveRoot bool, armBlocks []int32) bool {
	if !haveRoot || !meta.Dominates(fnKey, dispatchRoot, br.BlockIndex) {
		return false
	}
	isSwitchDefault := br.BlockKind == "switch-case" && len(br.CaseValues) == 0
	return isSwitchDefault || mutuallyExclusiveWithArms(meta, fnKey, br.BlockIndex, armBlocks)
}

// mutuallyExclusiveWithArms reports whether block rb shares NO control-flow path
// with any method arm (neither reaches the other) — true for a dispatch fallback
// arm (a sibling of the method arms), false for a conditional that falls through
// to or from the arms (an independent before/after the dispatch).
func mutuallyExclusiveWithArms(meta *metadata.Metadata, fnKey string, rb int32, armBlocks []int32) bool {
	for _, a := range armBlocks {
		if a == rb {
			return false
		}
		if meta.Reaches(fnKey, metadata.BlockLoc{Block: a}, metadata.BlockLoc{Block: rb}) ||
			meta.Reaches(fnKey, metadata.BlockLoc{Block: rb}, metadata.BlockLoc{Block: a}) {
			return false
		}
	}
	return true
}

// dominatingMethods returns the HTTP method(s) whose dispatch arm dominates block
// rb (rb lies inside that arm's region, e.g. a conditional nested in `case GET:`),
// or ok=false if no method arm dominates it. armBlocks is sorted, so when several
// arms dominate rb (unusual nested/fallthrough shapes) the choice is deterministic.
func dominatingMethods(meta *metadata.Metadata, fnKey string, rb int32, armBlocks []int32, armBlockToMethods map[int32][]string) ([]string, bool) {
	for _, armBlock := range armBlocks {
		if armBlock != rb && meta.Dominates(fnKey, armBlock, rb) {
			return armBlockToMethods[armBlock], true
		}
	}
	return nil, false
}

// branchNamesMethod reports whether any of a branch's case values name an HTTP
// method (so its responses are already attributed per method).
func branchNamesMethod(caseValues []string) bool {
	for _, v := range caseValues {
		if isValidHTTPMethodStr(strings.ToUpper(v)) {
			return true
		}
	}
	return false
}

func isValidHTTPMethodStr(s string) bool {
	switch s {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
		return true
	}
	return false
}

// ExtractionCallback is called for each node during unified tree visitor traversal.
type ExtractionCallback func(node TrackerNodeInterface, route *RouteInfo)

// visitChildren recursively traverses a node's children, calling each callback
// for every visited node. Handles recursion and visited tracking in one place.
func (e *Extractor) visitChildren(node TrackerNodeInterface, route *RouteInfo, callbacks []ExtractionCallback) {
	for _, child := range node.GetChildren() {
		for _, cb := range callbacks {
			cb(child, route)
		}
		e.visitChildren(child, route, callbacks)
	}
}

// addResponse adds a response to the route, merging schemas for duplicate status codes.
//
// Issue #33 (producer-side invariant): for bodyless status codes (1xx/204/304),
// strip any body-bearing fields on insert. Per RFC 9110 these statuses cannot
// carry a message body, and emitting one is incorrect output. Enforcing the
// invariant here — rather than relying solely on the mapper-side guard in
// buildResponses — keeps RouteInfo internally consistent for any future
// consumer that reads the structure directly (debug tooling, alternative
// emitters, metrics exporters). The mapper-side guard remains as the
// secondary defense and as the authority for the predicate.
func (e *Extractor) addResponse(route *RouteInfo, resp *ResponseInfo) {
	if isBodylessStatusCode(resp.StatusCode) {
		resp.Schema = nil
		resp.AlternativeSchemas = nil
		resp.BodyType = ""
		resp.BodyTypeRef = nil
	}
	key := fmt.Sprintf("%d", resp.StatusCode)
	if existing, ok := route.Response[key]; ok && resp.Schema != nil {
		if existing.Schema == nil {
			existing.BodyType = resp.BodyType
			existing.BodyTypeRef = resp.BodyTypeRef
			existing.Schema = resp.Schema
		} else {
			isDuplicate := schemasEqual(existing.Schema, resp.Schema)
			if !isDuplicate {
				for _, alt := range existing.AlternativeSchemas {
					if schemasEqual(alt, resp.Schema) {
						isDuplicate = true
						break
					}
				}
			}
			if !isDuplicate {
				existing.AlternativeSchemas = append(existing.AlternativeSchemas, resp.Schema)
			}
		}
	} else {
		route.Response[key] = resp
	}
}

// helperCall represents a call to a helper function with its ParamArgMap.
type helperCall struct {
	node TrackerNodeInterface
	edge *metadata.CallGraphEdge
}

// resolveArgToStatusCode attempts to map a CallArgument to an HTTP status code.
func (e *Extractor) resolveArgToStatusCode(arg *metadata.CallArgument) (int, bool) {
	argStr := e.contextProvider.GetArgumentInfo(arg)
	for _, matcher := range e.responseMatchers {
		if rm, ok := matcher.(*ResponsePatternMatcherImpl); ok {
			return rm.schemaMapper.MapStatusCode(argStr)
		}
	}
	return 0, false
}

// expandHelperFunctionResponses scans the route node's descendants for helper
// functions called multiple times with different status codes. For each group,
// it creates additional responses for status codes not captured by the primary
// response extraction (which deduplicates by Callee.ID).
//
//nolint:gocyclo // helper expansion fans out: filter → seed → param-infer → schema fill
func (e *Extractor) expandHelperFunctionResponses(routeNode TrackerNodeInterface, route *RouteInfo, fallbackEdges map[string]bool) {
	groups := e.collectHelperCallGroups(routeNode)

	for _, calls := range groups {
		if len(calls) < 2 {
			continue
		}
		// Strip fallback edges (issue #27): edges flagged by
		// helperFallbackEdges represent helper-internal error branches whose
		// statuses must not propagate to the caller. Only swap in `filtered`
		// when the scanner actually removed something — that way groups the
		// scanner never touched keep their original shape (and we don't
		// accidentally drop legitimate multi-branch handler patterns like
		// three `c.JSON(400, ...)` returns in if/else if/else, which never
		// get classified by helperFallbackEdges in the first place because
		// the route node and response primitives are skipped).
		//
		// After filtering, fall back to the standard `len(calls) < 2` skip:
		// a group whose survivors are all fallbacks (filtered=0) or down to
		// one straggler has no quorum to drive expansion.
		if len(fallbackEdges) > 0 {
			filtered := calls[:0:0]
			for _, c := range calls {
				if !fallbackEdges[c.edge.Callee.ID()] {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) < len(calls) {
				calls = filtered
				if len(calls) < 2 {
					continue
				}
			}
		}

		// Only expand helpers that actually contain response-writing calls
		// (WriteHeader, Encode, etc.) to avoid fabricating responses for
		// unrelated helpers that happen to be called multiple times.
		if !e.helperContainsResponsePattern(calls[0].node) {
			continue
		}

		statusParam, baseSchema, contentType := e.findStatusParamAndSchema(calls, route)
		if statusParam == "" {
			// No existing seed — fall back to inferring the status parameter
			// directly from the call signatures so we can still populate
			// schemas for helpers whose primary-pass extraction was skipped
			// (e.g., WriteJSON-with-builtin-body — see issue #27).
			statusParam = e.inferStatusParamFromCalls(calls)
			if statusParam == "" {
				continue
			}
		}
		if contentType == "" {
			contentType = e.cfg.Defaults.ResponseContentType
		}

		// Find the "body" parameter name — the one whose argument resolved to the
		// base schema's type. This is used to resolve per-call body types below.
		bodyParam := e.findBodyParamName(calls, route, statusParam, baseSchema)

		for _, call := range calls {
			arg, exists := call.edge.ParamArgMap[statusParam]
			if !exists {
				continue
			}
			if status, ok := e.resolveArgToStatusCode(&arg); ok {
				key := fmt.Sprintf("%d", status)
				// Fill in: either the status code has no entry yet, or the
				// entry was registered by a WriteHeader pass without a body
				// (e.g., a helper writes `WriteHeader(status); Write(derived)`
				// and the derived body bottoms out at an unresolvable builtin
				// — see issue #27). For schema-less existing entries we still
				// need to populate the schema from the caller's body arg.
				existing, exists := route.Response[key]
				if exists && existing.Schema != nil {
					continue
				}
				// Resolve this call's body type from its own ParamArgMap.
				// If the body parameter carries a different type per call
				// (e.g., respondJSON(w, 200, user) vs respondJSON(w, 400, err)),
				// each sibling gets its own schema.
				schema := baseSchema
				if bodyParam != "" {
					if bodyArg, ok := call.edge.ParamArgMap[bodyParam]; ok {
						bodyType := e.contextProvider.GetArgumentInfo(&bodyArg)
						if bodyType != "" && bodyType != "interface{}" && bodyType != "any" {
							if s, _ := schemaForType(route.UsedTypes, bodyType, metadata.ParseTypeRef(bodyType), route.Metadata, e.cfg, nil); s != nil {
								schema = s
							}
						}
					}
				}
				e.addResponse(route, &ResponseInfo{
					StatusCode:  status,
					ContentType: contentType,
					Schema:      schema,
				})
			}
		}
	}
}

// collectHelperCallGroups recursively collects calls with ParamArgMap, grouped
// by callee BaseID.
func (e *Extractor) collectHelperCallGroups(routeNode TrackerNodeInterface) map[string][]helperCall {
	groups := make(map[string][]helperCall)
	var collect func(node TrackerNodeInterface)
	collect = func(node TrackerNodeInterface) {
		for _, child := range node.GetChildren() {
			edge := child.GetEdge()
			if edge != nil && len(edge.ParamArgMap) > 0 {
				baseID := edge.Callee.BaseID()
				groups[baseID] = append(groups[baseID], helperCall{node: child, edge: edge})
			}
			collect(child)
		}
	}
	collect(routeNode)
	return groups
}

// inferStatusParamFromCalls picks the helper parameter whose arguments resolve
// to HTTP status codes across the call group, without relying on an existing
// route.Response entry. Used as a fallback when the primary response pass did
// not seed a schema for any of the helper's status codes (see issue #27 — a
// WriteJSON helper whose body argument is a builtin call like `append`).
//
// Parameter names are visited in sorted order so that helpers with multiple
// status-typed parameters yield the same pick across runs — same reasoning
// as the disambiguateOperationIDs determinism fix below.
func (e *Extractor) inferStatusParamFromCalls(calls []helperCall) string {
	for _, call := range calls {
		for _, pName := range slices.Sorted(maps.Keys(call.edge.ParamArgMap)) {
			arg := call.edge.ParamArgMap[pName]
			if _, ok := e.resolveArgToStatusCode(&arg); ok {
				return pName
			}
		}
	}
	return ""
}

// findStatusParamAndSchema finds which parameter in a group of helper calls
// maps to a status code that already has a response with a schema.
func (e *Extractor) findStatusParamAndSchema(calls []helperCall, route *RouteInfo) (paramName string, schema *Schema, contentType string) {
	for _, call := range calls {
		for pName, arg := range call.edge.ParamArgMap {
			if status, ok := e.resolveArgToStatusCode(&arg); ok {
				key := fmt.Sprintf("%d", status)
				if resp, exists := route.Response[key]; exists && resp.Schema != nil {
					return pName, resp.Schema, resp.ContentType
				}
			}
		}
	}
	return "", nil, ""
}

// findBodyParamName finds which ParamArgMap parameter carries the response body.
// It identifies the parameter by exclusion: not the status code param, not a
// ResponseWriter, and not a string (message). The remaining param is the body.
func (e *Extractor) findBodyParamName(calls []helperCall, _ *RouteInfo, statusParam string, _ *Schema) string {
	if len(calls) == 0 {
		return ""
	}

	// Use the first call to identify parameter roles
	for pName, arg := range calls[0].edge.ParamArgMap {
		if pName == statusParam || pName == "w" || pName == "writer" || pName == "rw" || pName == "response" {
			continue
		}
		// Skip if this resolves to a status code
		if _, ok := e.resolveArgToStatusCode(&arg); ok {
			continue
		}
		// Skip string literal parameters (message strings)
		if arg.GetKind() == metadata.KindLiteral {
			continue
		}
		// This is likely the body parameter
		return pName
	}
	return ""
}

// helperContainsResponsePattern checks if a helper function node has any children
// that match a response pattern (e.g., WriteHeader, Encode). This prevents
// expandHelperFunctionResponses from fabricating responses for non-response helpers.
func (e *Extractor) helperContainsResponsePattern(helperNode TrackerNodeInterface) bool {
	for _, child := range helperNode.GetChildren() {
		for _, matcher := range e.responseMatchers {
			if matcher.MatchNode(child) {
				return true
			}
		}
	}
	return false
}

// helperFallbackEdges identifies response-writing edges that live inside an
// error-fallback branch of a helper function whose primary write path is
// unconditional. The classic shape is:
//
//	func WriteJSON(w, status, v) {
//	    data, err := json.Marshal(v)
//	    if err != nil {
//	        w.WriteHeader(500)                                 // ← Branch=if-then
//	        w.Write([]byte(`{"error":"..."}`))                 // ← Branch=if-then
//	        return
//	    }
//	    w.WriteHeader(status)                                  // ← Branch=nil
//	    w.Write(append(data, '\n'))                            // ← Branch=nil
//	}
//
// The if-then writes are defensive — they fire only if json.Marshal fails,
// which is unreachable for the struct/map payloads the caller actually passes.
// Without this filter the caller's response set gets contaminated with the
// fallback's hardcoded 500 and literal []byte body (issue #27).
//
// The rule is intentionally narrow: only filter when the SAME helper exposes
// at least one unconditional response-writing edge. A helper made entirely of
// branched writes (e.g., if/else returning different statuses) keeps all of
// its writes — none of them is a "primary" path to compare against.
//
// Returned set is keyed by edge ID (the same ID used for visitedEdges).
func (e *Extractor) helperFallbackEdges(routeNode TrackerNodeInterface) map[string]bool {
	fallback := make(map[string]bool)
	if routeNode == nil {
		return fallback
	}

	var visit func(node TrackerNodeInterface, isRoot bool)
	visit = func(node TrackerNodeInterface, isRoot bool) {
		if e.isHelperInvocation(node, isRoot) {
			hw := e.classifyHelperWrites(node)
			// #27: a defensive branch only contaminates the caller when the SAME
			// helper also has an unconditional (primary) write to compare against.
			if hw.hasUnconditional() {
				for _, child := range hw.conditional {
					fallback[child.GetEdge().Callee.ID()] = true
				}
			}
		}
		for _, child := range node.GetChildren() {
			visit(child, false)
		}
	}
	visit(routeNode, true)
	return fallback
}

// helperWrites partitions a helper invocation's response-writing child edges by
// whether each is reachable on the helper's unconditional (primary) path or only
// under an internal branch. It is the reusable core shared by the #27 fallback
// filter (helperFallbackEdges) and the US1 helper-internal type-switch binding /
// degrade (FR-011/FR-012).
type helperWrites struct {
	unconditional []TrackerNodeInterface // Branch == nil — the helper's primary path
	conditional   []TrackerNodeInterface // Branch != nil — guarded by an internal branch
}

func (h helperWrites) hasUnconditional() bool { return len(h.unconditional) > 0 }

// isHelperInvocation reports whether node represents a USER-DEFINED helper call
// (as opposed to the route node or a response primitive). A node is a helper
// invocation when:
//  1. it is not the route node itself (whose children are the handler's body —
//     branches there are legitimate control flow, not internal fallback logic),
//  2. its edge carries a ParamArgMap (the call passed bound arguments through to
//     the callee's parameters), and
//  3. the call itself is not a response-pattern primitive (Status, JSON,
//     WriteHeader, …) — for chained calls like `c.Status(400).JSON(map)` the
//     Status node may parent the JSON node, and treating Status as a helper would
//     mis-classify legitimate handler branches.
func (e *Extractor) isHelperInvocation(node TrackerNodeInterface, isRoot bool) bool {
	if isRoot {
		return false
	}
	edge := node.GetEdge()
	return edge != nil && len(edge.ParamArgMap) > 0 && !e.matchesAnyResponse(node)
}

// classifyHelperWrites returns the response-writing DIRECT children of a helper-
// invocation node, partitioned by whether each is reachable unconditionally or only
// under an internal if-branch. Non-response children are ignored.
//
// It scans direct children only — deliberately NOT the whole subtree (CodeRabbit
// suggested recursing to catch nested writes; that was evaluated and rejected — see
// below). The #27 defensive-fallback shape it serves is
// `WriteHeader(status); if err { http.Error }`, where both the unconditional primary
// write and the if-then fallback are DIRECT children. A SUB-handler reached through a
// wrapper, by contrast, writes its success body via a nested
// `json.NewEncoder(w).Encode(v)` (a grandchild under NewEncoder) and its real error
// via a direct `if err { http.Error(4xx) }`. Recursing would treat that nested
// success write as the helper's "unconditional primary" and then wrongly filter the
// sub-handler's genuine 4xx as a defensive fallback (regression observed on the
// enum_validation fixture: POST drops its 400). Direct-children scanning is correct
// for both shapes.
func (e *Extractor) classifyHelperWrites(node TrackerNodeInterface) helperWrites {
	var hw helperWrites
	for _, child := range node.GetChildren() {
		if child.GetEdge() == nil || !e.matchesAnyResponse(child) {
			continue
		}
		br := child.GetEdge().Branch
		switch {
		case br == nil:
			hw.unconditional = append(hw.unconditional, child)
		case br.BlockKind == "if-then" || br.BlockKind == "if-else":
			// Only an if-guarded write is a #27-style defensive fallback. A
			// switch-case/select-case write is a type-switch (or method) arm owned by
			// the type-switch-binding pass, so it must NOT be filtered here as a
			// fallback — doing so would drop a precisely-bound arm.
			hw.conditional = append(hw.conditional, child)
		}
	}
	return hw
}

// matchesAnyResponse reports whether node matches any registered response pattern.
func (e *Extractor) matchesAnyResponse(node TrackerNodeInterface) bool {
	for _, m := range e.responseMatchers {
		if m.MatchNode(node) {
			return true
		}
	}
	return false
}

// helperTypeSwitchEdges resolves helper-internal type-switches against the
// call-site argument (FR-011/FR-012, US1 #5/#6) and returns the set of case-write
// edge IDs to FILTER OUT. For each helper invocation whose internal branches are
// type-switch arms, it binds the switched parameter to the call-site argument (via
// the case's SwitchOperand + the edge's ParamArgMap) and resolves its concrete
// type:
//   - precise concrete binding → keep only the matched arm (or the default arm
//     when the concrete type matches no case); filter the rest, no warning;
//   - imprecise binding (interface/error/any, or the operand cannot be resolved) →
//     keep only the default/unconditional path, filter all typed arms, and warn —
//     never fanning out every arm.
//
// Returned set is keyed by edge ID (the same ID used for visitedEdges), so it
// composes with helperFallbackEdges.
//
// Arm-write edge IDs are keyed by the helper's INTERNAL call position, which is
// identical across multiple call sites of the same helper in one route. A handler
// that invokes the same type-switching helper twice with different concrete
// arguments must therefore INTERSECT, not union, the per-site decisions: an arm
// kept by ANY site is reachable and must survive. We collect both the filtered and
// the kept sets and return filtered MINUS kept.
func (e *Extractor) helperTypeSwitchEdges(routeNode TrackerNodeInterface) map[string]bool {
	ae := armEdges{filtered: map[string]bool{}, kept: map[string]bool{}}
	if routeNode == nil {
		return ae.filtered
	}
	var visit func(node TrackerNodeInterface, isRoot bool)
	visit = func(node TrackerNodeInterface, isRoot bool) {
		if e.isHelperInvocation(node, isRoot) {
			e.bindHelperTypeSwitch(node, ae)
		}
		for _, child := range node.GetChildren() {
			visit(child, false)
		}
	}
	visit(routeNode, true)
	for id := range ae.kept {
		delete(ae.filtered, id)
	}
	return ae.filtered
}

// armEdges accumulates, across every type-switch binding in one route, the arm-write
// edge IDs to drop (filtered) and the ones an actual binding keeps (kept). The final
// drop set is filtered − kept, so a write reachable from any call site survives.
type armEdges struct {
	filtered map[string]bool
	kept     map[string]bool
}

// switchArm holds the response-writing edges of one type-switch's arms, gathered
// from a helper's subtree: typed arms (with case types) and the default arm.
type switchArm struct {
	typed []TrackerNodeInterface // arms with non-empty CaseTypeRefs
	dflt  []TrackerNodeInterface // the default arm (no case types)
}

// bindHelperTypeSwitch performs the per-invocation binding for helperTypeSwitchEdges,
// adding the filtered-out arm-write edge IDs to filtered. A single helper may host
// more than one type-switch (on different parameters), so arms are grouped by the
// switched operand and each group whose operand is one of this node's callee
// parameters is bound independently.
func (e *Extractor) bindHelperTypeSwitch(node TrackerNodeInterface, ae armEdges) {
	edge := node.GetEdge()
	if edge == nil || len(edge.ParamArgMap) == 0 {
		return
	}
	arms, escaped := e.collectSwitchArms(node)
	if escaped {
		// The helper has a conditional (if/else) response write that is not a clean
		// type-switch arm — typically a `case` body with nested control flow, whose
		// writes go/cfg annotates with the inner branch's context rather than the
		// arm's, but also any other if-guarded write in the helper. In either case
		// the arms are not fully captured, so we keep ALL arms (the pre-binding
		// over-approximation) rather than risk dropping or leaking a status
		// (FR-012: never mis-bind; over-approximate when uncertain).
		return
	}
	for _, operand := range slices.Sorted(maps.Keys(arms)) {
		arm := arms[operand]
		if len(arm.typed) == 0 {
			continue
		}
		arg, isHost := edge.ParamArgMap[operand]
		if !isHost {
			// The operand is not a parameter of this node's callee — this node only
			// parents the arm writes in the tracker tree (e.g. an inner
			// json.NewEncoder call), so it is not the switch's binding site.
			continue
		}
		argRef, precise := e.boundArgRef(&arg, node)
		e.applyTypeSwitchBinding(arm, argRef, precise, edge, ae)
	}
}

// collectSwitchArms gathers, from node's subtree, every response-writing edge whose
// BranchContext is a type-switch arm DECLARED IN THE HOST'S OWN FUNCTION, grouped by
// the switched operand. It recurses through the whole subtree because an arm's
// writes (WriteHeader, Encode, …) may be nested under intermediate calls (e.g.
// json.NewEncoder(w).Encode) rather than direct children of the helper invocation;
// the `sameFunc` scope check confines collection to the host callee's body so a
// NESTED helper's type-switch (whose operand may shadow an outer parameter name) is
// left to its own binding pass instead of being mis-bound against this call site.
// escaped is true when the host function has any response write under an
// if-then/if-else branch. The common cause is a type-switch arm with nested
// control flow (its writes are attributed to the inner branch, not the arm), but
// any if-guarded response write in the helper trips it. The caller then declines
// to filter (over-approximates safely) because the arms are not fully captured.
func (e *Extractor) collectSwitchArms(node TrackerNodeInterface) (arms map[string]*switchArm, escaped bool) {
	arms = make(map[string]*switchArm)
	host := node.GetEdge()
	if host == nil {
		return arms, false
	}
	var walk func(n TrackerNodeInterface)
	walk = func(n TrackerNodeInterface) {
		for _, child := range n.GetChildren() {
			ce := child.GetEdge()
			if ce != nil && ce.Branch != nil && sameFunc(ce.Caller, host.Callee) && e.matchesAnyResponse(child) {
				// A "clean" type-switch arm write is a switch-case block carrying the
				// switched operand. ANY other conditional response write in the helper
				// — an if/else, a select case, a nested or unrelated value-switch
				// (switch-case with no operand) — means an arm contains control flow
				// go/cfg attributes elsewhere, so the arms are not cleanly captured.
				if ce.Branch.BlockKind == "switch-case" && ce.Branch.SwitchOperand != "" {
					arm := arms[ce.Branch.SwitchOperand]
					if arm == nil {
						arm = &switchArm{}
						arms[ce.Branch.SwitchOperand] = arm
					}
					if len(ce.Branch.CaseTypeRefs) > 0 {
						arm.typed = append(arm.typed, child)
					} else {
						arm.dflt = append(arm.dflt, child)
					}
				} else {
					escaped = true
				}
			}
			walk(child)
		}
	}
	walk(node)
	return arms, escaped
}

// sameFunc reports whether two Call references identify the same function (by name,
// package, and receiver type — all string-pool indices in one Metadata).
func sameFunc(a, b metadata.Call) bool {
	return a.Name == b.Name && a.Pkg == b.Pkg && a.RecvType == b.RecvType
}

// applyTypeSwitchBinding decides which arm writes to filter for one type-switch,
// given the bound call-site argument leaf:
//   - precise concrete match → keep the matched arms, filter the other typed arms
//     AND the default (the concrete type selects a specific arm; default does not run);
//   - precise but no match (a concrete type that hits the default) → keep the default,
//     filter the typed arms, no warning;
//   - imprecise binding → keep the default, filter the typed arms, and warn (FR-012).
func (e *Extractor) applyTypeSwitchBinding(arm *switchArm, argRef *metadata.TypeRef, precise bool, edge *metadata.CallGraphEdge, ae armEdges) {
	var selected []TrackerNodeInterface
	if precise && argRef != nil {
		for _, c := range arm.typed {
			if refMatchesAnyCase(c.GetEdge().Branch.CaseTypeRefs, argRef) {
				selected = append(selected, c)
			}
		}
	}

	if len(selected) > 0 {
		// Precise match: KEEP the matched arms; filter the other typed arms AND the
		// default (the concrete type selects a specific arm; default does not run).
		keep := make(map[string]bool, len(selected))
		for _, c := range selected {
			id := c.GetEdge().Callee.ID()
			keep[id] = true
			ae.kept[id] = true
		}
		mark := func(nodes []TrackerNodeInterface) {
			for _, c := range nodes {
				if id := c.GetEdge().Callee.ID(); !keep[id] {
					ae.filtered[id] = true
				}
			}
		}
		mark(arm.typed)
		mark(arm.dflt)
		return
	}

	// No typed arm selected. When the binding is IMPRECISE and there is no default
	// arm to fall back to, the runtime value will still hit one of the typed arms —
	// we cannot tell which — so KEEP them all (over-approximate) and warn, rather
	// than drop every response. (A precise concrete type that matches no case and
	// has no default genuinely produces nothing, which is Go-correct.)
	if !precise && len(arm.dflt) == 0 {
		for _, c := range arm.typed {
			ae.kept[c.GetEdge().Callee.ID()] = true
		}
		e.warn(e.contextProvider.GetString(edge.Position),
			"helper type-switch: call-site argument type not statically known; emitting all arms")
		return
	}

	// Otherwise KEEP the default arm, filter every typed arm.
	for _, c := range arm.dflt {
		ae.kept[c.GetEdge().Callee.ID()] = true
	}
	for _, c := range arm.typed {
		ae.filtered[c.GetEdge().Callee.ID()] = true
	}
	if !precise {
		e.warn(e.contextProvider.GetString(edge.Position),
			"helper type-switch: call-site argument type not statically known; emitting unconditional result only")
	}
}

// boundArgRef resolves a call-site argument to its STATIC type ref. precise is
// false when that type is an interface/error/any that does not pin a concrete type
// at the call site (FR-012 degrade), in which case the ref is returned as nil.
//
// The argument's structured TypeRef is the static type of the argument expression —
// exactly what a Go type-switch discriminates, and conservative per FR-012: an
// `any`/`error`-typed argument stays a RefInterface (imprecise) even if a concrete
// value flowed into it, so we never over-resolve across an interface boundary. The
// string-based origin is the fallback only when the structured ref is absent (a
// deserialized/hand-built argument).
func (e *Extractor) boundArgRef(arg *metadata.CallArgument, node TrackerNodeInterface) (ref *metadata.TypeRef, precise bool) {
	ref = arg.TypeRef
	if ref == nil {
		_, ref = sharedResolveTypeOrigin(arg, node, e.contextProvider.GetArgumentInfo(arg), e.contextProvider, false)
	}
	if isImpreciseLeaf(ref.NamedLeaf()) {
		return nil, false
	}
	return ref, true
}

// isImpreciseLeaf reports whether a resolved named leaf fails to pin a concrete
// type — a nil/interface leaf, or the built-in dynamic names error/any/interface{}
// (FR-012).
//
// Known limitation: a NAMED interface (e.g. a domain `Animal`, or io.Reader) whose
// concrete value is unknown is carried as RefNamed, not RefInterface, so it is not
// recognised as imprecise here. If such a value is the type-switch operand AND the
// switch has a same-named interface case, the binding will pick that interface arm
// instead of degrading. Recognising named interfaces needs interface-aware type
// metadata at TypeRef-construction time (a non-trivial cross-cutting change); the
// pattern — a value of named-interface static type matched against its own
// interface case — is uncommon, so it is left as a documented gap.
func isImpreciseLeaf(leaf *metadata.TypeRef) bool {
	if leaf == nil || leaf.Kind == metadata.RefInterface {
		return true
	}
	switch leaf.Name {
	case "", "error", "any", "interface{}":
		return true
	}
	return false
}

// typeRefShapeEqual reports whether two type refs denote the SAME Go type,
// structurally: same Kind, and for named types same package+name, recursing
// through pointer/slice/array/map elements, map keys, array length, and generic
// instantiation arguments. This is the exact-type identity a Go type-switch tests
// — it distinguishes T from *T, []T, and Box[A] from Box[B] — so the binding never
// over- or under-matches across the value/pointer/slice/generic distinctions
// (FR-012). It is immune to package-qualifier string-format differences (it
// compares the structured fields, not String()).
func typeRefShapeEqual(a, b *metadata.TypeRef) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind || a.Name != b.Name || a.Pkg != b.Pkg || a.Len != b.Len {
		return false
	}
	if !typeRefShapeEqual(a.Elem, b.Elem) || !typeRefShapeEqual(a.Key, b.Key) {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !typeRefShapeEqual(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

// refMatchesAnyCase reports whether the argument's static type is exactly one of a
// case clause's types (a `case A, B:` clause carries several). The match is the
// structural type identity a Go type-switch performs, so a `*T`/`[]T`/`Box[User]`
// argument binds only the case of that same shape — never a sibling arm.
func refMatchesAnyCase(caseRefs []*metadata.TypeRef, argRef *metadata.TypeRef) bool {
	for _, cr := range caseRefs {
		if typeRefShapeEqual(cr, argRef) {
			return true
		}
	}
	return false
}

// extractRouteChildren extracts request, response, and params from children nodes
// using the unified visitor with registered callbacks.
func (e *Extractor) extractRouteChildren(routeNode TrackerNodeInterface, route *RouteInfo, mountTags []string, routes *[]*RouteInfo, visitedEdges map[string]bool) {
	// Identify response-writing edges inside defensive error-fallback branches
	// of helpers that also expose an unconditional write path. These edges
	// must not contribute to the caller's response schema — see issue #27 and
	// helperFallbackEdges for the exact rule.
	fallbackEdges := e.helperFallbackEdges(routeNode)
	// Also filter the type-switch arms of a response helper that the call-site
	// argument does not bind to (spec 009, FR-011/FR-012). Both sets gate the same
	// visitedEdges check, so merge them.
	for id := range e.helperTypeSwitchEdges(routeNode) {
		fallbackEdges[id] = true
	}

	callbacks := []ExtractionCallback{
		// Route-in-route detection
		func(node TrackerNodeInterface, route *RouteInfo) {
			if isRoute := e.executeRoutePattern(node, route); isRoute {
				e.handleRouteNode(node, route, "", mountTags, routes)
			}
		},
		// Request extraction — first match wins. The handler-level decode of
		// r.Body is hit by depth-first traversal before any nested
		// json.Decode / json.Unmarshal inside helper or service functions
		// (which deserialize unrelated payloads like RabbitMQ messages and
		// must not override the real request body type).
		func(node TrackerNodeInterface, route *RouteInfo) {
			if route.Request != nil {
				return
			}
			if req := e.extractRequestFromNode(node, route); req != nil {
				route.Request = req
			}
		},
		// Response extraction with schema merging
		func(node TrackerNodeInterface, route *RouteInfo) {
			if edge := node.GetEdge(); edge != nil && fallbackEdges[edge.Callee.ID()] {
				return
			}
			for _, resp := range e.extractResponseFromNode(node, route, visitedEdges) {
				if resp == nil || (resp.BodyType == "" && resp.StatusCode == 0) {
					continue
				}
				e.addResponse(route, resp)
			}
		},
		// Parameter extraction
		func(node TrackerNodeInterface, route *RouteInfo) {
			if param := e.extractParamFromNode(node, route); param != nil {
				route.Params = append(route.Params, *param)
			}
		},
		// Content-Type detection
		e.checkContentTypePattern,
		// Map index parameter extraction (mux.Vars)
		e.extractParamsFromAssignmentMaps,
	}

	e.visitChildren(routeNode, route, callbacks)

	// After all children are visited and responses have schemas, expand
	// helper function responses. When the same error helper is called multiple
	// times with different status codes (e.g., writeJSONError(w, 400, ...) and
	// writeJSONError(w, 404, ...)), the dedup in extractResponseFromNode only
	// processes one call's WriteHeader/Encode. This post-pass creates responses
	// for the other calls using the schema from the processed one. Helper
	// fallback edges (issue #27) are excluded so that a hardcoded
	// `WriteHeader(500)` inside `if err != nil { ... }` does not get expanded
	// into a phantom 500 response on every caller.
	e.expandHelperFunctionResponses(routeNode, route, fallbackEdges)

	// Extract parameters from the route node itself
	if param := e.extractParamFromNode(routeNode, route); param != nil {
		route.Params = append(route.Params, *param)
	}

	// Query parameters declared on a gorilla/mux builder chain
	// (.HandleFunc(...).Queries("q", "{q}")) sit on sibling nodes, not in the
	// handler body, so extract them from the route registration itself.
	e.extractMuxQueriesParams(routeNode, route)

	// Look for Authorization-header reads transitively from the handler
	// function and infer the matching OpenAPI security scheme. Uses the
	// call-graph caller index directly because the tracker tree's
	// route-shaped subtree doesn't always surface handler-body edges as
	// children. Done after parameter extraction so the mapper can prune
	// the redundant Authorization header parameter from operations that
	// now use a securityScheme.
	if scheme := detectSecuritySchemeFromHandler(route); scheme != nil {
		route.SecurityScheme = scheme
	}
}

// baseTypeName reduces a (possibly pointer-qualified, package-qualified) type
// string to its bare type name: "github.com/gorilla/mux.*Route" → "Route",
// "*Router" → "Router". Used for exact receiver-type matching so a substring
// like "Route" doesn't also match "Router"/"RouterGroup".
func baseTypeName(typeStr string) string {
	typeStr = strings.ReplaceAll(typeStr, "*", "")
	if i := strings.LastIndexAny(typeStr, "./"); i >= 0 {
		typeStr = typeStr[i+1:]
	}
	return typeStr
}

// extractMuxQueriesParams adds query parameters declared on a gorilla/mux route
// builder chain — r.HandleFunc(path, h).Queries("q", "{q}", "page", "{page}")
// registers query params q and page. The .Queries call is a sibling of the
// route registration node within the same chain (so sibling routes don't leak);
// its even-indexed args are the parameter keys (odd args are "{var}" value
// templates). Gated on a *Route receiver so an unrelated method named Queries
// can't inject phantom params.
func (e *Extractor) extractMuxQueriesParams(routeNode TrackerNodeInterface, route *RouteInfo) {
	if routeNode == nil {
		return
	}
	parent := routeNode.GetParent()
	if parent == nil {
		return
	}
	existing := map[string]bool{}
	for _, p := range route.Params {
		if p.In == "query" {
			existing[p.Name] = true
		}
	}
	for _, sib := range parent.GetChildren() {
		edge := sib.GetEdge()
		if edge == nil || e.contextProvider.GetString(edge.Callee.Name) != "Queries" {
			continue
		}
		if baseTypeName(e.contextProvider.GetString(edge.Callee.RecvType)) != "Route" {
			continue
		}
		for i := 0; i < len(edge.Args); i += 2 {
			name := strings.Trim(e.contextProvider.GetArgumentInfo(edge.Args[i]), `"'{} `)
			if name == "" || existing[name] {
				continue
			}
			existing[name] = true
			route.Params = append(route.Params, Parameter{
				Name: name,
				In:   "query",
				// A gorilla/mux route with .Queries only matches when the key is
				// present, so the parameter is required.
				Required: true,
				Schema:   &Schema{Type: "string"},
			})
		}
	}
}

// extractParamsFromAssignmentMaps scans a node's assignment map for map index
// expressions with string literal keys. When a variable is assigned from a
// map index (e.g., id := vars["id"]), the key is extracted as a path parameter.
// This handles patterns like mux.Vars(r)["id"] where the parameter name comes
// from the map access, not from a function call argument.
func (e *Extractor) extractParamsFromAssignmentMaps(node TrackerNodeInterface, route *RouteInfo) {
	edge := node.GetEdge()
	if edge == nil || edge.AssignmentMap == nil {
		return
	}

	existingParams := make(map[string]bool)
	for _, p := range route.Params {
		existingParams[p.Name] = true
	}

	for _, assignments := range edge.AssignmentMap {
		for _, assignment := range assignments {
			val := &assignment.Value
			if val.GetKind() != metadata.KindIndex {
				continue
			}
			// The index key is in val.Fun
			if val.Fun == nil || val.Fun.GetKind() != metadata.KindLiteral {
				continue
			}
			key := e.contextProvider.GetArgumentInfo(val.Fun)
			key = strings.Trim(key, "\"")
			if key == "" || existingParams[key] {
				continue
			}
			// Add as a path parameter
			route.Params = append(route.Params, Parameter{
				Name:     key,
				In:       "path",
				Required: true,
				Schema:   &Schema{Type: "string"},
			})
			existingParams[key] = true
		}
	}
}

// extractRequestFromNode extracts request information from a node
func (e *Extractor) extractRequestFromNode(node TrackerNodeInterface, route *RouteInfo) *RequestInfo {
	for _, matcher := range e.requestMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractRequest(node, route)
		}
	}
	return nil
}

// extractResponseFromNode extracts response information from a node.
// Returns a slice because a single call site can yield multiple responses when
// conditional status codes apply (see ExtractResponse / issue #39).
func (e *Extractor) extractResponseFromNode(node TrackerNodeInterface, route *RouteInfo, visitedEdges map[string]bool) []*ResponseInfo {
	// Ensure that each edge is visited only once
	if node == nil || node.GetEdge() == nil {
		return nil
	}

	edge := node.GetEdge()
	edgeID := edge.Callee.ID()
	if visitedEdges[edgeID] {
		return nil // Edge already processed
	}

	// Mark edge as visited before processing to ensure MatchNode is only called once per edge
	visitedEdges[edgeID] = true

	for _, matcher := range e.responseMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractResponse(node, route)
		}
	}
	// // If no response matcher matches, return the default response info
	// return &ResponseInfo{
	// 	StatusCode:  e.cfg.Defaults.ResponseStatus,
	// 	ContentType: e.cfg.Defaults.ResponseContentType,
	// }
	return nil
}

// extractParamFromNode extracts parameter information from a node
func (e *Extractor) extractParamFromNode(node TrackerNodeInterface, route *RouteInfo) *Parameter {
	for _, matcher := range e.paramMatchers {
		if matcher.MatchNode(node) {
			return matcher.ExtractParam(node, route)
		}
	}
	return nil
}

// joinPaths joins two URL paths cleanly
func joinPaths(a, b string) string {
	a = strings.TrimRight(a, "/")
	b = strings.TrimLeft(b, "/")
	if a == "" {
		return "/" + b
	}
	// Avoid double-mounting: if b already starts with a's last segment,
	// strip the overlap. e.g. joinPaths("/payment", "payment/process") → "/payment/process"
	aBase := a
	if idx := strings.LastIndex(a, "/"); idx >= 0 {
		aBase = a[idx+1:]
	}
	if aBase != "" && strings.HasPrefix(b, aBase+"/") {
		return a + "/" + b[len(aBase)+1:]
	}
	return a + "/" + b
}

// determineLiteralType determines the appropriate Go type for a literal value
func determineLiteralType(literalValue string) string {
	// Remove quotes if present
	cleanValue := strings.Trim(literalValue, "\"`")

	// Check for numeric literals
	if _, err := strconv.ParseInt(cleanValue, 10, 64); err == nil {
		return "int"
	}
	if _, err := strconv.ParseUint(cleanValue, 10, 64); err == nil {
		return "uint"
	}
	if _, err := strconv.ParseFloat(cleanValue, 64); err == nil {
		return "float64"
	}

	// Check for boolean literals
	if cleanValue == "true" || cleanValue == "false" {
		return "bool"
	}

	// Check for nil
	if cleanValue == "nil" {
		return "interface{}"
	}

	// Default to string for everything else
	return "string"
}

func preprocessingBodyType(bodyType string) string {
	if after, ok := strings.CutPrefix(bodyType, "[]"); ok && after != "" {
		bodyType = after
	}
	if after, ok := strings.CutPrefix(bodyType, "*"); ok && after != "" {
		bodyType = after
	}
	if after, ok := strings.CutPrefix(bodyType, "&"); ok && after != "" {
		bodyType = after
	}
	return bodyType
}

// ResponsePatternMatcherImpl implements ResponsePatternMatcher
type ResponsePatternMatcherImpl struct {
	*BasePatternMatcher
	pattern ResponsePattern
}

// NewResponsePatternMatcher creates a new response pattern matcher
func NewResponsePatternMatcher(pattern ResponsePattern, cfg *APISpecConfig, contextProvider ContextProvider) *ResponsePatternMatcherImpl {
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the response pattern
func (r *ResponsePatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	return baseMatchNode(node, r.pattern.BasePattern, r.contextProvider)
}

// GetPattern returns the response pattern
func (r *ResponsePatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *ResponsePatternMatcherImpl) GetPriority() int {
	return basePriority(r.pattern.BasePattern)
}

// ExtractResponse extracts response information from a matched node.
//
// Returns a slice to support conditional status codes (issue #39): when the
// status arg is a local variable reassigned across branches with different
// status codes, we emit one ResponseInfo per distinct status (all sharing the
// same body/schema). For the typical "one status per call" case the slice has
// exactly one element — byte-identical to the previous single-response output.
//
//nolint:gocyclo // response extraction with multiple pattern types
func (r *ResponsePatternMatcherImpl) ExtractResponse(node TrackerNodeInterface, route *RouteInfo) []*ResponseInfo {
	var (
		statusResolved bool
	)

	// Get least status code from response map (sorted for determinism)
	leastStatusCode := 0
	for _, key := range slices.Sorted(maps.Keys(route.Response)) {
		resp := route.Response[key]
		if resp.StatusCode < leastStatusCode {
			leastStatusCode = resp.StatusCode
		}
	}

	contentType := r.cfg.Defaults.ResponseContentType
	if r.pattern.DefaultContentType != "" {
		contentType = r.pattern.DefaultContentType
	}

	respInfo := &ResponseInfo{
		StatusCode:  leastStatusCode - 1,
		ContentType: contentType,
	}

	edge := node.GetEdge()

	// For free-function writes whose destination is the first argument
	// (io.Copy(dst, src), io.WriteString(dst, s), fmt.Fprintf(dst, …)), the
	// destination must be the HTTP response. Otherwise an io.Copy to a file or
	// buffer reachable deep in the handler's call graph (e.g. file-service's
	// storage adapter copying an upload to disk) is misinferred as a binary
	// response body, and — depending on call-graph map ordering — flaps run to
	// run (issue #52, response-side counterpart of decodeReadsRequestBody).
	if r.pattern.ValidateWriterDest && edge != nil {
		if len(edge.Args) == 0 || !r.writeDestTracesToResponseWriter(node, edge.Args[0]) {
			return nil
		}
	}

	if r.pattern.StatusFromArg && len(edge.Args) > r.pattern.StatusArgIndex {
		arg := edge.Args[r.pattern.StatusArgIndex]
		statusStr := r.contextProvider.GetArgumentInfo(arg)
		if status, ok := r.schemaMapper.MapStatusCode(statusStr); ok {
			statusResolved = true
			respInfo.StatusCode = status
		} else if arg.GetKind() == metadata.KindIdent {
			// Status code stored in a variable — resolve via assignment map
			if assignments, exists := edge.AssignmentMap[arg.GetName()]; exists && len(assignments) > 0 {
				assignedValue := r.contextProvider.GetArgumentInfo(&assignments[0].Value)
				if status, ok := r.schemaMapper.MapStatusCode(assignedValue); ok {
					statusResolved = true
					respInfo.StatusCode = status
				}
			}
			// If not found in AssignmentMap, check if this is a function parameter
			// passed from a caller (e.g., writeJSONError(w, http.StatusBadRequest, "msg")
			// where statusCode is a parameter, not a local variable).
			// Walk up the parent chain to find a ParamArgMap that maps this parameter name.
			if !statusResolved {
				if status, ok := r.resolveParamArgStatus(node, arg.GetName()); ok {
					statusResolved = true
					respInfo.StatusCode = status
				}
			}
		} else if arg.GetKind() == metadata.KindCall {
			// Status code from function call — check if the callee has a
			// constant return value (e.g., func getStatus() int { return 200 })
			if resolved := r.resolveCallReturnValue(arg); resolved != "" {
				if status, ok := r.schemaMapper.MapStatusCode(resolved); ok {
					statusResolved = true
					respInfo.StatusCode = status
				}
			}
		}
	}

	if !statusResolved && r.pattern.DefaultStatus > 0 {
		respInfo.StatusCode = r.pattern.DefaultStatus
		statusResolved = true
	}

	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {
		// If status code is not from argument, find the lowest valid status code
		// with no body type assigned yet. Skip bodyless codes (1xx, 204, 304).
		if !r.pattern.StatusFromArg {
			if status, ok := findVacantStatusForBody(route); ok {
				respInfo.StatusCode = status
			}
		}

		arg := edge.Args[r.pattern.TypeArgIndex]

		// If the argument is a type conversion (e.g., []byte("text")),
		// use the conversion target type, not the inner literal.
		var conversionTargetType string
		if arg.GetKind() == metadata.KindTypeConversion {
			conversionTargetType = r.contextProvider.GetArgumentInfo(arg)
			if len(arg.Args) > 0 {
				arg = arg.Args[0]
			}
		}

		bodyType := r.contextProvider.GetArgumentInfo(arg)
		var bodyRef *metadata.TypeRef // resolution-emitted structured type (Phase 3)
		// For a composite literal whose named leaf is a type we have in metadata,
		// prefer the lossless TypeRef string: its canonical fully-qualified form is
		// what the field path uses, so the body references the same component. A
		// literal needs no origin tracing, so taking it here is safe; external types
		// (not in metadata) keep their existing short-alias form (T009/T011).
		if arg.GetKind() == metadata.KindCompositeLit && arg.TypeRef != nil && route.Metadata != nil {
			if s := bodyTypeFromMetadataRef(arg.TypeRef, route.Metadata, r.cfg); s != "" {
				bodyType = s
			}
		}
		// Prefer the conversion target type if available
		if conversionTargetType != "" {
			bodyType = conversionTargetType
		}

		// Preserve generic type from the argument's raw type info. When the arg
		// type is a generic instantiation (e.g. "APIResponse[pkg.User]") it
		// carries the wrapper the resolved type may have lost. But for a generic
		// composite literal (APIResponse[User]{…}), GetArgumentInfo already
		// reconstructed the *bound* form while arg.Type is still the unbound
		// declaration "APIResponse[T any]" — so only fall back to rawArgType when
		// bodyType isn't already a bound (concrete-arg) generic instantiation.
		rawArgType := r.contextProvider.GetString(arg.Type)
		// Prefer the lossless TypeRef, which carries the BOUND generic
		// instantiation (Pair[User,Order]) where arg.Type may hold only the unbound
		// declaration (Pair[K,V]) — the wrapper this branch exists to preserve
		// (T009/T011).
		if arg.TypeRef != nil {
			rawArgType = arg.TypeRef.String()
			// Phase 4: baseline the ref from the arg's own structured type — native,
			// no parse. resolveTypeOrigin / literal / deref below refine it in
			// lockstep with bodyType where they change the string.
			bodyRef = arg.TypeRef
		}
		if strings.Contains(rawArgType, "[") && !strings.HasPrefix(rawArgType, "[]") && !strings.HasPrefix(rawArgType, "map[") {
			if !genericArgsAreConcrete(bodyType) {
				bodyType = rawArgType
				bodyRef = arg.TypeRef // rawArgType == arg.TypeRef.String()
			}
		}

		// A type conversion overrode bodyType with the conversion TARGET above, but the
		// bodyRef baseline came from the INNER (converted) arg, so they can diverge —
		// e.g. List[int](xs) gives bodyType "List[T any]" while bodyRef is []int. Re-
		// derive bodyRef from the target string so it stays in lockstep with bodyType
		// (the Phase-4 byte-identical invariant); the literal/origin branches below do
		// not re-resolve a generic target. A non-generic target is re-traced by
		// resolveTypeOrigin next, which overwrites this in lockstep.
		if conversionTargetType != "" && bodyType == conversionTargetType {
			bodyRef = metadata.ParseTypeRef(bodyType)
		}

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			bodyType = determineLiteralType(bodyType)
			bodyRef = metadata.ParseTypeRef(bodyType) // literal-type boundary (Phase 4)
		} else if !strings.Contains(bodyType, "[") || strings.HasPrefix(bodyType, "[]") || strings.HasPrefix(bodyType, "map[") {
			// Trace type origin for non-literal, non-generic arguments.
			// Skip type resolution for generic types (e.g., "APIResponse[User]")
			// to preserve the wrapper type and enable generic struct instantiation.
			bodyType, bodyRef = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed — unwrap the ref's pointer in lockstep.
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
				bodyRef = derefPointerRef(bodyRef)
			}
		}

		// If the body type is interface{} or unresolved and the argument is a
		// function parameter, resolve the concrete type from the caller's argument
		// via ParamArgMap (e.g., respondJSON(w, 201, user) where data is interface{}).
		if (bodyType == "interface{}" || bodyType == "" || bodyType == "any") && arg.GetKind() == metadata.KindIdent {
			if concreteType, concreteRef := r.resolveParamArgType(node, arg.GetName()); concreteType != "" {
				bodyType = concreteType
				bodyRef = concreteRef
			}
		}

		// go/types stores `Typ[Invalid].String() == "invalid type"` for things
		// it cannot resolve to a concrete type — most often a builtin like
		// `append`, `len`, `copy`, `make`, or an untyped constant. A body
		// expression that bottoms out at this sentinel cannot produce a useful
		// schema and must not fabricate a `$ref` to a non-existent schema name.
		// Clear bodyType so expandHelperFunctionResponses can fill the schema
		// in from the caller's ParamArgMap arg instead (issue #27).
		if strings.Contains(bodyType, "invalid type") {
			bodyType = ""
			bodyRef = nil
		}

		respInfo.BodyType = preprocessingBodyType(bodyType)
		// Phase 4: bodyRef is kept in lockstep with bodyType through every transform
		// above (arg.TypeRef baseline, resolveTypeOrigin, resolveParamArgType, literal
		// boundary, deref unwrap, invalid-type clear). The generic-wrapper branch
		// (bodyType contains "[") skips origin tracing, so a body whose arg carried no
		// TypeRef can reach here with bodyRef still nil; refForResolved backfills the
		// canonical ParseTypeRef(bodyType) there. Backfill bodyRef IN PLACE so both the
		// carrier and the schemaForType call below consume the same threaded ref — no
		// schema-layer re-parse (the Phase-3/4 goal; passing the raw nil here would send
		// schemaForType back through its own ParseTypeRef). refForResolved never
		// overwrites a non-nil ref, so canonical/stale handling is unchanged; and
		// ParseTypeRef("") is nil, so a cleared body stays exempt and the Phase-3 carrier
		// contract (TestEveryResolvedBodyTypeReachesSchemaWithRef) holds for the skip
		// path too.
		bodyRef = refForResolved(bodyRef, bodyType)
		respInfo.BodyTypeRef = bodyRef

		// In response-writer context, []byte means raw binary content.
		if bodyType == "[]byte" {
			respInfo.Schema = &Schema{Type: "string", Format: "binary"}
		} else {
			schema, _ := schemaForType(route.UsedTypes, bodyType, bodyRef, route.Metadata, r.cfg, nil)
			respInfo.Schema = schema

			// Wrapper/envelope specialisation: when the body flows through a
			// helper that wraps the payload in a struct with a generic
			// (interface{}/any) field, recover the concrete per-route payload
			// type from the call site and emit allOf[base $ref, {field override}].
			if overrides := r.collectWrapperOverrides(arg, node); len(overrides) > 0 {
				meta := metadataFromContextProvider(r.contextProvider)
				respInfo.Schema = specialiseWrapperSchema(respInfo.Schema, overrides, bodyType, route.UsedTypes, meta, r.cfg)
			}
		}
	}

	// If no type was extracted from args but the pattern specifies a default
	// body type (e.g., fmt.Fprintf → "string", io.Copy → "[]byte"), use it.
	if respInfo.BodyType == "" && r.pattern.DefaultBodyType != "" {
		bodyType := r.pattern.DefaultBodyType
		respInfo.BodyType = preprocessingBodyType(bodyType)
		respInfo.BodyTypeRef = metadata.ParseTypeRef(bodyType) // Phase 3 carrier
		if bodyType == "[]byte" {
			// For io.Copy, try to trace the reader source to distinguish
			// binary (os.Open) from text (strings.NewReader).
			isBinary := true
			if len(edge.Args) > 1 {
				readerArg := edge.Args[1]
				readerInfo := r.contextProvider.GetArgumentInfo(readerArg)
				if strings.Contains(readerInfo, "strings") || strings.Contains(readerInfo, "NewReader") {
					isBinary = false
				}
				// Also check assignment map for variable readers
				if readerArg.GetKind() == metadata.KindIdent {
					if assignments, exists := edge.AssignmentMap[readerArg.GetName()]; exists && len(assignments) > 0 {
						assignedInfo := r.contextProvider.GetArgumentInfo(&assignments[0].Value)
						if strings.Contains(assignedInfo, "strings") || strings.Contains(assignedInfo, "NewReader") {
							isBinary = false
						}
					}
				}
			}
			if isBinary {
				respInfo.Schema = &Schema{Type: "string", Format: "binary"}
			} else {
				respInfo.Schema = &Schema{Type: "string"}
			}
		} else {
			schema, _ := schemaForType(route.UsedTypes, bodyType, respInfo.BodyTypeRef, route.Metadata, r.cfg, nil)
			respInfo.Schema = schema
		}
	}

	// Propagate branch context from the call graph edge.
	if node.GetEdge() != nil {
		respInfo.Branch = node.GetEdge().Branch
	}

	// Conditional status fan-out (issue #39): if the status arg is a local
	// variable with branched assignments mapping to distinct status codes, emit
	// one response per status, all sharing the body/schema. Runs before the
	// no-status-no-body guard so patterns whose status arg is an opaque ident
	// (e.g. RespondWithError(w, err)) still produce responses when the branches
	// encode the codes. The set is filtered to assignments that reach this call
	// site (issue #50), so a single reachable status yields a single response
	// rather than falling through to the unresolvable latest-wins path.
	if r.pattern.StatusFromArg && len(edge.Args) > r.pattern.StatusArgIndex {
		if statuses := r.expandStatusesFromIdent(edge.Args[r.pattern.StatusArgIndex], edge); len(statuses) >= 1 {
			out := make([]*ResponseInfo, 0, len(statuses))
			for _, st := range statuses {
				out = append(out, &ResponseInfo{
					StatusCode:  st,
					ContentType: respInfo.ContentType,
					BodyType:    respInfo.BodyType,
					BodyTypeRef: respInfo.BodyTypeRef,
					Schema:      respInfo.Schema,
					Branch:      respInfo.Branch,
				})
			}
			return out
		}
	}

	if !statusResolved && respInfo.BodyType == "" {
		return nil
	}

	return []*ResponseInfo{respInfo}
}

// expandStatusesFromIdent walks the caller function's AssignmentMap for the
// given ident and returns the distinct status codes implied by the RHS calls
// of each assignment. For each assignment whose value is a call, the first
// argument that parses as a known HTTP status (via MapStatusCode) is taken as
// that branch's status. This captures the common pattern:
//
//	if errors.As(err, &a) { err = NewError(msg, http.StatusUnauthorized) }
//	else                  { err = NewError(msg, http.StatusNotFound) }
//	RespondWithError(w, err)
//
// Returns nil (leaving latest-wins behaviour intact) when the arg is not an
// ident, the caller function or its AssignmentMap can't be located, or fewer
// than two assignments exist.
func (r *ResponsePatternMatcherImpl) expandStatusesFromIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) []int {
	if arg == nil || arg.GetKind() != metadata.KindIdent || edge == nil {
		return nil
	}
	meta := metadataFromContextProvider(r.contextProvider)
	if meta == nil {
		return nil
	}
	fn := findFunction(meta, meta.StringPool.GetString(edge.Caller.Pkg), meta.StringPool.GetString(edge.Caller.Name))
	if fn == nil {
		return nil
	}
	assigns, ok := fn.AssignmentMap[arg.GetName()]
	if !ok || len(assigns) < 2 {
		return nil
	}
	// Reachability filter (spec 009, FR-002/FR-006): keep the assignments whose
	// value can reach this response call along some control-flow path AND that are
	// not overwritten on every path to the call by a later, call-dominating
	// assignment. Computed structurally from the CFG reachability model — replacing
	// the former source-position + last-unconditional-index heuristic. Mutually
	// exclusive sibling branches never reach each other, so they do not shadow one
	// another (the if/else fan-out is preserved); an unconditional overwrite that
	// dominates the call kills earlier assignments.
	callPos := meta.StringPool.GetString(edge.Position)
	fnKey := meta.FnKeyForPos(callPos)
	callLoc, callOK := meta.BlockFor(fnKey, callPos)
	cands := r.collectStatusCands(meta, fnKey, assigns)

	// FR-008: when the CFG cannot place this call (an unmodelled construct), degrade
	// to the unconditionally-reachable statuses + warn, rather than guessing.
	if !callOK || fnKey == "" {
		return r.degradeToUnconditional(callPos, cands)
	}
	return contributingStatuses(meta, fnKey, callLoc, cands)
}

// statusCand is a status code paired with its control-flow location, used by the
// reachability filter in expandStatusesFromIdent.
type statusCand struct {
	status int
	loc    metadata.BlockLoc
	hasLoc bool // the assignment was placed in a CFG block
	uncond bool // the assignment is unconditional (Branch == nil)
}

// collectStatusCands turns each call-valued assignment into a status candidate
// located in fnKey's CFG. Non-call assignments, and calls with no status-code
// argument, are skipped.
func (r *ResponsePatternMatcherImpl) collectStatusCands(meta *metadata.Metadata, fnKey string, assigns []metadata.Assignment) []statusCand {
	cands := make([]statusCand, 0, len(assigns))
	for i := range assigns {
		if assigns[i].Value.GetKind() != metadata.KindCall {
			continue
		}
		status, okStatus := statusFromCallArgs(r, &assigns[i].Value)
		if !okStatus {
			continue
		}
		loc, hasLoc := meta.BlockFor(fnKey, meta.StringPool.GetString(assigns[i].Position))
		cands = append(cands, statusCand{status: status, loc: loc, hasLoc: hasLoc, uncond: assigns[i].Branch == nil})
	}
	return cands
}

// degradeToUnconditional returns the unconditionally-reachable statuses, warning
// when that drops any conditional candidate (FR-008). Used when the CFG cannot
// place the response call.
func (r *ResponsePatternMatcherImpl) degradeToUnconditional(callPos string, cands []statusCand) []int {
	uncond := make([]int, 0, len(cands))
	for i := range cands {
		if cands[i].uncond {
			uncond = append(uncond, cands[i].status)
		}
	}
	if len(uncond) < len(cands) {
		r.warn(callPos, "conditional status fan-out: control flow not modelled; using unconditional statuses")
	}
	return dedupInts(uncond)
}

// contributingStatuses applies the reachability + kill predicate (FR-002/FR-006):
// a candidate contributes iff it reaches the call and is not overwritten on every
// path by a later assignment whose block dominates the call.
//
// The kill test is a single-dominator approximation of "overwritten on every
// path": it does not recognise that a set of mutually-exclusive sibling
// reassignments can jointly cover every path (e.g. an unconditional default that
// BOTH arms of an if/else overwrite), so such a dead default still contributes a
// phantom status. This is a long-standing accuracy limitation shared with the
// pre-CFG heuristic — output is unchanged for it, so it is not introduced here; a
// full reaching-definitions pass would be the principled fix.
func contributingStatuses(meta *metadata.Metadata, fnKey string, callLoc metadata.BlockLoc, cands []statusCand) []int {
	out := make([]int, 0, len(cands))
	for i := range cands {
		if !cands[i].hasLoc || !meta.Reaches(fnKey, cands[i].loc, callLoc) {
			continue // cannot reach the call
		}
		if !killedByDominator(meta, fnKey, callLoc, cands, i) {
			out = append(out, cands[i].status)
		}
	}
	return dedupInts(out)
}

// killedByDominator reports whether cands[i] is overwritten on every path to the
// call: some cands[j] between cands[i] and the call whose block dominates the call
// block. cands[j] must lie ON a path from cands[i] to the call — it must both be
// reachable from cands[i] AND itself reach the call. The j-reaches-call clause is
// essential: a later reassignment that sits AFTER the call in the SAME straight-line
// block satisfies Reaches(i,j) and the reflexive block self-domination, but executes
// after the response write and must NOT shadow the value the call actually read.
func killedByDominator(meta *metadata.Metadata, fnKey string, callLoc metadata.BlockLoc, cands []statusCand, i int) bool {
	for j := range cands {
		if i == j || !cands[j].hasLoc || cands[i].loc == cands[j].loc {
			continue
		}
		if meta.Reaches(fnKey, cands[i].loc, cands[j].loc) &&
			meta.Reaches(fnKey, cands[j].loc, callLoc) &&
			meta.Dominates(fnKey, cands[j].loc.Block, callLoc.Block) {
			return true
		}
	}
	return false
}

// dedupInts returns the input with duplicate values removed, preserving first-seen order.
func dedupInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, dup := seen[v]; !dup {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// statusFromCallArgs returns the first argument of a call that parses as a known
// HTTP status code (e.g. the http.StatusXxx in NewError(msg, http.StatusXxx)).
func statusFromCallArgs(r *ResponsePatternMatcherImpl, call *metadata.CallArgument) (int, bool) {
	for _, callArg := range call.Args {
		if callArg == nil {
			continue
		}
		if status, ok := r.schemaMapper.MapStatusCode(r.contextProvider.GetArgumentInfo(callArg)); ok {
			return status, true
		}
	}
	return 0, false
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (r *ResponsePatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) (string, *metadata.TypeRef) {
	// Honour explicit resolved-type info on the argument first — set when an
	// earlier analysis pass already pinned the concrete type.
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType, refForResolved(arg.ResolvedTypeRef, resolvedType)
	}

	// Substitute generic type parameters using the call site's TypeParamMap.
	// Without this, a response written through a helper like
	// `WriteJSON[T any](w, status, v T)` would emit the bare type parameter
	// (e.g. `pkg.T`) instead of the concrete instantiation at the call site
	// (e.g. `dto.CheckRoomHTTPResponse`). Mirrors the request-side logic.
	if genericType, ref := traceGenericOrigin(node, originalType); genericType != "" {
		return genericType, ref
	}

	return sharedResolveTypeOrigin(arg, node, originalType, r.contextProvider, false)
}

// ParamPatternMatcherImpl implements ParamPatternMatcher
type ParamPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern ParamPattern
}

// NewParamPatternMatcher creates a new param pattern matcher
func NewParamPatternMatcher(pattern ParamPattern, cfg *APISpecConfig, contextProvider ContextProvider) *ParamPatternMatcherImpl {
	return &ParamPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the param pattern
func (p *ParamPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	return baseMatchNode(node, p.pattern.BasePattern, p.contextProvider)
}

// GetPattern returns the param pattern
func (p *ParamPatternMatcherImpl) GetPattern() interface{} {
	return p.pattern
}

// GetPriority returns the priority of this pattern
func (p *ParamPatternMatcherImpl) GetPriority() int {
	return basePriority(p.pattern.BasePattern)
}

// ExtractParam extracts parameter information from a matched node
func (p *ParamPatternMatcherImpl) ExtractParam(node TrackerNodeInterface, route *RouteInfo) *Parameter {
	param := &Parameter{
		In: p.pattern.ParamIn,
	}

	edge := node.GetEdge()
	if len(edge.Args) > p.pattern.ParamArgIndex {
		param.Name = p.contextProvider.GetArgumentInfo(edge.Args[p.pattern.ParamArgIndex])
	}

	if p.pattern.TypeFromArg && len(edge.Args) > p.pattern.TypeArgIndex {
		arg := edge.Args[p.pattern.TypeArgIndex]
		paramType := p.contextProvider.GetArgumentInfo(arg)
		var paramRef *metadata.TypeRef // resolution-emitted structured type (Phase 3; param-path local, no carrier field)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			paramType = determineLiteralType(paramType)
			paramRef = metadata.ParseTypeRef(paramType) // literal-type boundary (Phase 4)
		} else {
			// Trace type origin for non-literal arguments (sets paramRef in lockstep)
			paramType, paramRef = p.resolveTypeOrigin(arg, node, paramType)

			// Apply dereferencing if needed — unwrap the ref's pointer in lockstep.
			if p.pattern.Deref && strings.HasPrefix(paramType, "*") {
				paramType = strings.TrimPrefix(paramType, "*")
				paramRef = derefPointerRef(paramRef)
			}
		}

		// Phase 4: paramRef is kept in lockstep with paramType (resolveTypeOrigin,
		// literal boundary, deref unwrap), so the schema generator consumes it
		// directly — no reconcile re-parse.
		schema, _ := schemaForType(route.UsedTypes, paramType, paramRef, route.Metadata, p.cfg, nil)
		param.Schema = schema
	}

	// When the pattern doesn't pin a schema (no TypeFromArg, no DefaultType),
	// try to infer one from the converter applied to the parameter value
	// (e.g. strconv.Atoi → integer, strconv.ParseBool → boolean). DefaultType
	// takes precedence so callers like FormFile keep their fixed schema.
	if param.Schema == nil && p.pattern.DefaultType == "" {
		if inferred := inferParamConverterSchema(node, route); inferred != nil {
			param.Schema = inferred
		}
	}

	// Ensure all parameters have a schema - default to string if none specified
	if param.Schema == nil {
		schemaType := "string"
		if p.pattern.DefaultType != "" {
			schemaType = p.pattern.DefaultType
		}
		param.Schema = &Schema{Type: schemaType, Format: p.pattern.DefaultFormat}
	}

	// Ensure path parameters are always required
	if p.pattern.ParamIn == "path" {
		param.Required = true
	}

	return param
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (p *ParamPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) (string, *metadata.TypeRef) {
	return sharedResolveTypeOrigin(arg, node, originalType, p.contextProvider, false)
}

// OverrideApplierImpl implements OverrideApplier
type OverrideApplierImpl struct {
	cfg *APISpecConfig
}

// NewOverrideApplier creates a new override applier
func NewOverrideApplier(cfg *APISpecConfig) *OverrideApplierImpl {
	return &OverrideApplierImpl{
		cfg: cfg,
	}
}

// ApplyOverrides applies manual overrides to route info
func (o *OverrideApplierImpl) ApplyOverrides(routeInfo *RouteInfo) {
	for _, override := range o.cfg.Overrides {
		if override.FunctionName == routeInfo.Function {
			if override.Summary != "" {
				routeInfo.Summary = override.Summary
			}
			// Description was parsed from config but never applied (issue #46);
			// a config override wins over the doc-comment-derived description.
			if override.Description != "" {
				routeInfo.Description = override.Description
			}
			if res, exists := routeInfo.Response[fmt.Sprintf("%d", override.ResponseStatus)]; exists && override.ResponseStatus != 0 && routeInfo.Response != nil {
				res.StatusCode = override.ResponseStatus
			}
			if override.ResponseType != "" && routeInfo.Response != nil {
				for _, key := range slices.Sorted(maps.Keys(routeInfo.Response)) {
					routeInfo.Response[key].BodyType = preprocessingBodyType(override.ResponseType)
					routeInfo.Response[key].BodyTypeRef = metadata.ParseTypeRef(override.ResponseType)
				}
			}
			if len(override.Tags) > 0 {
				routeInfo.Tags = override.Tags
			}
		}
	}
}

// HasOverride checks if there's an override for a function
func (o *OverrideApplierImpl) HasOverride(functionName string) bool {
	for _, override := range o.cfg.Overrides {
		if override.FunctionName == functionName {
			return true
		}
	}
	return false
}
