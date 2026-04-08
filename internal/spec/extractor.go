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
	Path      string
	MountPath string
	Method    string
	Handler   string
	Package   string
	File      string
	Function  string
	Summary   string
	Tags      []string
	Request   *RequestInfo
	Response  map[string]*ResponseInfo
	Params    []Parameter

	UsedTypes map[string]*Schema
	Metadata  *metadata.Metadata

	// Resolved router group prefix (if any)
	GroupPrefix string

	// Content-Type detected from Header().Set("Content-Type", value) calls
	detectedContentType string
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
	Schema      *Schema
}

// ResponseInfo represents response information
type ResponseInfo struct {
	StatusCode  int
	ContentType string
	BodyType    string
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
	typeResolver    TypeResolver
	overrideApplier OverrideApplier

	// Pattern matchers
	routeMatchers    []RoutePatternMatcher
	mountMatchers    []MountPatternMatcher
	requestMatchers  []RequestPatternMatcher
	responseMatchers []ResponsePatternMatcher
	paramMatchers    []ParamPatternMatcher
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
				// If the value doesn't look like a valid MIME type (must contain "/"),
				// it's a variable or field path (e.g., doc.MimeType). Fall back to
				// application/octet-stream for dynamic content types.
				if val != "" && !strings.Contains(val, "/") {
					val = "application/octet-stream"
				}
				if val != "" {
					// Override content type on existing responses that use the
					// default. Don't override responses with pattern-specific
					// content types (e.g., http.Error → text/plain).
					defaultCT := e.cfg.Defaults.ResponseContentType
					for _, resp := range route.Response {
						if resp.ContentType == defaultCT {
							resp.ContentType = val
						}
					}
					// Store for future responses that haven't been added yet
					route.detectedContentType = val
				}
			}
		}
	}
}

// NewExtractor creates a new refactored extractor
func NewExtractor(tree TrackerTreeInterface, cfg *APISpecConfig) *Extractor {
	contextProvider := NewContextProvider(tree.GetMetadata())
	schemaMapper := NewSchemaMapper(cfg)
	typeResolver := NewTypeResolver(tree.GetMetadata(), cfg, schemaMapper)
	overrideApplier := NewOverrideApplier(cfg)

	extractor := &Extractor{
		tree:            tree,
		cfg:             cfg,
		contextProvider: contextProvider,
		schemaMapper:    schemaMapper,
		typeResolver:    typeResolver,
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
		matcher := NewRoutePatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.routeMatchers = append(e.routeMatchers, matcher)
	}

	// Initialize mount matchers
	for _, pattern := range e.cfg.Framework.MountPatterns {
		matcher := NewMountPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.mountMatchers = append(e.mountMatchers, matcher)
	}

	// Initialize request matchers
	for _, pattern := range e.cfg.Framework.RequestBodyPatterns {
		matcher := NewRequestPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.requestMatchers = append(e.requestMatchers, matcher)
	}

	// Initialize response matchers
	for _, pattern := range e.cfg.Framework.ResponsePatterns {
		matcher := NewResponsePatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
		e.responseMatchers = append(e.responseMatchers, matcher)
	}

	// Initialize param matchers
	for _, pattern := range e.cfg.Framework.ParamPatterns {
		matcher := NewParamPatternMatcher(pattern, e.cfg, e.contextProvider, e.typeResolver)
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
		defaultCT := e.cfg.Defaults.ResponseContentType
		for _, resp := range routeInfo.Response {
			if resp.ContentType == defaultCT {
				resp.ContentType = routeInfo.detectedContentType
			}
		}
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
func (r *ResponsePatternMatcherImpl) resolveParamArgType(node TrackerNodeInterface, paramName string) string {
	for parent := node.GetParent(); parent != nil; parent = parent.GetParent() {
		parentEdge := parent.GetEdge()
		if parentEdge == nil {
			continue
		}
		if arg, exists := parentEdge.ParamArgMap[paramName]; exists {
			// Use GetArgumentInfo first — it produces the fully-qualified type
			// (e.g., "error_helpers.User" instead of just "User")
			if info := r.contextProvider.GetArgumentInfo(&arg); info != "" && info != "interface{}" && info != "any" {
				return info
			}
			// Fallback to raw type
			if argType := arg.GetType(); argType != "" && argType != "interface{}" && argType != "any" {
				return argType
			}
		}
	}
	return ""
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
				resp := matcher.ExtractResponse(mockNode, route)
				if resp != nil && (resp.BodyType != "" || resp.StatusCode != 0) {
					key := fmt.Sprintf("%d", resp.StatusCode)
					route.Response[key] = resp
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
// context with HTTP method case values (e.g., switch r.Method case "GET").
// If so, returns separate RouteInfo entries per method. Returns nil if no
// conditional methods are detected.
func (e *Extractor) splitByConditionalMethods(route *RouteInfo) []*RouteInfo {
	// Collect HTTP methods from response branch contexts
	methodResponses := make(map[string]map[string]*ResponseInfo) // method → statusCode → response

	for statusCode, resp := range route.Response {
		if resp.Branch == nil || resp.Branch.BlockKind != "switch-case" || len(resp.Branch.CaseValues) == 0 {
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

	var result []*RouteInfo
	for method, responses := range methodResponses {
		mr := &RouteInfo{
			Path:                route.Path,
			MountPath:           route.MountPath,
			Method:              method,
			Handler:             route.Handler,
			Package:             route.Package,
			File:                route.File,
			Function:            route.Function,
			Summary:             route.Summary,
			Tags:                route.Tags,
			Request:             route.Request,
			Response:            responses,
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
func (e *Extractor) addResponse(route *RouteInfo, resp *ResponseInfo) {
	key := fmt.Sprintf("%d", resp.StatusCode)
	if existing, ok := route.Response[key]; ok && resp.Schema != nil {
		if existing.Schema == nil {
			existing.BodyType = resp.BodyType
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
func (e *Extractor) expandHelperFunctionResponses(routeNode TrackerNodeInterface, route *RouteInfo) {
	groups := e.collectHelperCallGroups(routeNode)

	for _, calls := range groups {
		if len(calls) < 2 {
			continue
		}

		// Only expand helpers that actually contain response-writing calls
		// (WriteHeader, Encode, etc.) to avoid fabricating responses for
		// unrelated helpers that happen to be called multiple times.
		if !e.helperContainsResponsePattern(calls[0].node) {
			continue
		}

		statusParam, baseSchema, contentType := e.findStatusParamAndSchema(calls, route)
		if statusParam == "" || baseSchema == nil {
			continue
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
				if _, exists := route.Response[key]; !exists {
					// Resolve this call's body type from its own ParamArgMap.
					// If the body parameter carries a different type per call
					// (e.g., respondJSON(w, 200, user) vs respondJSON(w, 400, err)),
					// each sibling gets its own schema.
					schema := baseSchema
					if bodyParam != "" {
						if bodyArg, ok := call.edge.ParamArgMap[bodyParam]; ok {
							bodyType := e.contextProvider.GetArgumentInfo(&bodyArg)
							if bodyType != "" && bodyType != "interface{}" && bodyType != "any" {
								if s, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, e.cfg, nil); s != nil {
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

// extractRouteChildren extracts request, response, and params from children nodes
// using the unified visitor with registered callbacks.
func (e *Extractor) extractRouteChildren(routeNode TrackerNodeInterface, route *RouteInfo, mountTags []string, routes *[]*RouteInfo, visitedEdges map[string]bool) {
	callbacks := []ExtractionCallback{
		// Route-in-route detection
		func(node TrackerNodeInterface, route *RouteInfo) {
			if isRoute := e.executeRoutePattern(node, route); isRoute {
				e.handleRouteNode(node, route, "", mountTags, routes)
			}
		},
		// Request extraction
		func(node TrackerNodeInterface, route *RouteInfo) {
			if req := e.extractRequestFromNode(node, route); req != nil {
				route.Request = req
			}
		},
		// Response extraction with schema merging
		func(node TrackerNodeInterface, route *RouteInfo) {
			resp := e.extractResponseFromNode(node, route, visitedEdges)
			if resp == nil || (resp.BodyType == "" && resp.StatusCode == 0) {
				return
			}
			e.addResponse(route, resp)
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
	// for the other calls using the schema from the processed one.
	e.expandHelperFunctionResponses(routeNode, route)

	// Extract parameters from the route node itself
	if param := e.extractParamFromNode(routeNode, route); param != nil {
		route.Params = append(route.Params, *param)
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

// extractResponseFromNode extracts response information from a node
func (e *Extractor) extractResponseFromNode(node TrackerNodeInterface, route *RouteInfo, visitedEdges map[string]bool) *ResponseInfo {
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
func NewResponsePatternMatcher(pattern ResponsePattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *ResponsePatternMatcherImpl {
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
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

// ExtractResponse extracts response information from a matched node
//
//nolint:gocyclo // response extraction with multiple pattern types
func (r *ResponsePatternMatcherImpl) ExtractResponse(node TrackerNodeInterface, route *RouteInfo) *ResponseInfo {
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
			for _, key := range slices.Sorted(maps.Keys(route.Response)) {
				resp := route.Response[key]
				if resp.BodyType == "" && resp.StatusCode >= 100 && resp.StatusCode < 600 && !isBodylessStatusCode(resp.StatusCode) {
					respInfo.StatusCode = resp.StatusCode
					break
				}
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
		// Prefer the conversion target type if available
		if conversionTargetType != "" {
			bodyType = conversionTargetType
		}

		// Preserve generic type from the argument's raw type info.
		// When the arg type is a generic instantiation (e.g., "APIResponse[pkg.User]"),
		// use it instead of the resolved type which may lose the wrapper.
		rawArgType := r.contextProvider.GetString(arg.Type)
		if strings.Contains(rawArgType, "[") && !strings.HasPrefix(rawArgType, "[]") && !strings.HasPrefix(rawArgType, "map[") {
			bodyType = rawArgType
		}

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			bodyType = determineLiteralType(bodyType)
		} else if !strings.Contains(bodyType, "[") || strings.HasPrefix(bodyType, "[]") || strings.HasPrefix(bodyType, "map[") {
			// Trace type origin for non-literal, non-generic arguments.
			// Skip type resolution for generic types (e.g., "APIResponse[User]")
			// to preserve the wrapper type and enable generic struct instantiation.
			bodyType = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		// If the body type is interface{} or unresolved and the argument is a
		// function parameter, resolve the concrete type from the caller's argument
		// via ParamArgMap (e.g., respondJSON(w, 201, user) where data is interface{}).
		if (bodyType == "interface{}" || bodyType == "" || bodyType == "any") && arg.GetKind() == metadata.KindIdent {
			if concreteType := r.resolveParamArgType(node, arg.GetName()); concreteType != "" {
				bodyType = concreteType
			}
		}

		respInfo.BodyType = preprocessingBodyType(bodyType)

		// In response-writer context, []byte means raw binary content.
		if bodyType == "[]byte" {
			respInfo.Schema = &Schema{Type: "string", Format: "binary"}
		} else {
			schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)
			respInfo.Schema = schema
		}
	}

	// If no type was extracted from args but the pattern specifies a default
	// body type (e.g., fmt.Fprintf → "string", io.Copy → "[]byte"), use it.
	if respInfo.BodyType == "" && r.pattern.DefaultBodyType != "" {
		bodyType := r.pattern.DefaultBodyType
		respInfo.BodyType = preprocessingBodyType(bodyType)
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
			schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)
			respInfo.Schema = schema
		}
	}

	if !statusResolved && respInfo.BodyType == "" {
		return nil
	}

	// Propagate branch context from the call graph edge
	if node.GetEdge() != nil {
		respInfo.Branch = node.GetEdge().Branch
	}

	return respInfo
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (r *ResponsePatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	return sharedResolveTypeOrigin(arg, node, originalType, r.contextProvider, false)
}

// ParamPatternMatcherImpl implements ParamPatternMatcher
type ParamPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern ParamPattern
}

// NewParamPatternMatcher creates a new param pattern matcher
func NewParamPatternMatcher(pattern ParamPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *ParamPatternMatcherImpl {
	return &ParamPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
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

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			// For literal values, determine the appropriate type based on the value
			paramType = determineLiteralType(paramType)
		} else {
			// Trace type origin for non-literal arguments
			paramType = p.resolveTypeOrigin(arg, node, paramType)

			// Apply dereferencing if needed
			if p.pattern.Deref && strings.HasPrefix(paramType, "*") {
				paramType = strings.TrimPrefix(paramType, "*")
			}
		}

		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, paramType, route.Metadata, p.cfg, nil)
		param.Schema = schema
	}

	// Ensure all parameters have a schema - default to string if none specified
	if param.Schema == nil {
		param.Schema = &Schema{Type: "string"}
	}

	// Ensure path parameters are always required
	if p.pattern.ParamIn == "path" {
		param.Required = true
	}

	return param
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (p *ParamPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
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
			if res, exists := routeInfo.Response[fmt.Sprintf("%d", override.ResponseStatus)]; exists && override.ResponseStatus != 0 && routeInfo.Response != nil {
				res.StatusCode = override.ResponseStatus
			}
			if override.ResponseType != "" && routeInfo.Response != nil {
				for _, key := range slices.Sorted(maps.Keys(routeInfo.Response)) {
					routeInfo.Response[key].BodyType = preprocessingBodyType(override.ResponseType)
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
