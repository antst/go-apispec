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
	"strings"
	"sync"

	"github.com/antst/go-apispec/internal/metadata"
)

// Regex cache for pattern matchers
var (
	patternRegexCache = make(map[string]*regexp.Regexp)
	patternRegexMutex sync.RWMutex
)

// getCachedPatternRegex returns a cached compiled regex or compiles and caches a new one
func getCachedPatternRegex(pattern string) (*regexp.Regexp, error) {
	patternRegexMutex.RLock()
	if re, exists := patternRegexCache[pattern]; exists {
		patternRegexMutex.RUnlock()
		return re, nil
	}
	patternRegexMutex.RUnlock()

	patternRegexMutex.Lock()
	defer patternRegexMutex.Unlock()

	// Double-check after acquiring write lock
	if re, exists := patternRegexCache[pattern]; exists {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	patternRegexCache[pattern] = re
	return re, nil
}

// baseMatchNode contains the shared matching logic used by all MatchNode implementations.
// It checks the CallRegex, FunctionNameRegex, RecvTypeRegex, and RecvType fields of a
// BasePattern against the given node.
func baseMatchNode(node TrackerNodeInterface, pattern BasePattern, contextProvider ContextProvider) bool {
	if node == nil || node.GetEdge() == nil {
		return false
	}

	edge := node.GetEdge()
	callName := contextProvider.GetString(edge.Callee.Name)
	recvType := contextProvider.GetString(edge.Callee.RecvType)
	recvPkg := contextProvider.GetString(edge.Callee.Pkg)

	// Build fully qualified receiver type
	fqRecvType := recvPkg
	if fqRecvType != "" && recvType != "" {
		fqRecvType += "." + recvType
	} else if recvType != "" {
		fqRecvType = recvType
	}

	// Check call regex
	if pattern.CallRegex != "" {
		re, err := getCachedPatternRegex(pattern.CallRegex)
		if err != nil || !re.MatchString(callName) {
			return false
		}
	}

	// Check function name regex
	if pattern.FunctionNameRegex != "" {
		funcName := contextProvider.GetString(edge.Caller.Name)
		re, err := getCachedPatternRegex(pattern.FunctionNameRegex)
		if err != nil || !re.MatchString(funcName) {
			return false
		}
	}

	// Check receiver type
	if pattern.RecvTypeRegex != "" {
		re, err := getCachedPatternRegex(pattern.RecvTypeRegex)
		if err != nil || !re.MatchString(fqRecvType) {
			return false
		}
	} else if pattern.RecvType != "" && pattern.RecvType != fqRecvType {
		return false
	}

	return true
}

// basePriority computes the shared priority score for a BasePattern.
// More specific patterns have higher priority.
func basePriority(pattern BasePattern) int {
	priority := 0
	if pattern.CallRegex != "" {
		priority += 10
	}
	if pattern.FunctionNameRegex != "" {
		priority += 5
	}
	if pattern.RecvTypeRegex != "" || pattern.RecvType != "" {
		priority += 3
	}
	return priority
}

// BasePatternMatcher provides common functionality for all pattern matchers
type BasePatternMatcher struct {
	contextProvider ContextProvider
	cfg             *APISpecConfig
	schemaMapper    SchemaMapper
	typeResolver    TypeResolver
}

// NewBasePatternMatcher creates a new base pattern matcher
func NewBasePatternMatcher(cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *BasePatternMatcher {
	return &BasePatternMatcher{
		contextProvider: contextProvider,
		cfg:             cfg,
		schemaMapper:    NewSchemaMapper(cfg),
		typeResolver:    typeResolver,
	}
}

// RoutePatternMatcherImpl implements RoutePatternMatcher
type RoutePatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RoutePattern
}

// NewRoutePatternMatcher creates a new route pattern matcher
func NewRoutePatternMatcher(pattern RoutePattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RoutePatternMatcherImpl {
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the route pattern
func (r *RoutePatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	return baseMatchNode(node, r.pattern.BasePattern, r.contextProvider)
}

// GetPattern returns the route pattern
func (r *RoutePatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *RoutePatternMatcherImpl) GetPriority() int {
	return basePriority(r.pattern.BasePattern)
}

// ExtractRoute extracts route information from a matched node
func (r *RoutePatternMatcherImpl) ExtractRoute(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	found := false

	edge := node.GetEdge()
	if routeInfo == nil || routeInfo.File == "" || routeInfo.Package == "" {
		*routeInfo = RouteInfo{
			Method:    http.MethodPost, // Default method
			Package:   r.contextProvider.GetString(edge.Callee.Pkg),
			File:      r.contextProvider.GetString(edge.Position),
			Response:  make(map[string]*ResponseInfo),
			UsedTypes: make(map[string]*Schema),
		}
	}

	if edge != nil {
		routeInfo.Metadata = edge.Callee.Meta
	} else if node.GetArgument() != nil {
		routeInfo.Metadata = node.GetArgument().Meta
	}

	if routeInfo.File == "" && node.GetArgument() != nil {
		routeInfo.File = node.GetArgument().GetPosition()
	}

	found = r.extractRouteDetails(node, routeInfo)

	// Extract handler information
	if r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		found = true
		handlerArg := edge.Args[r.pattern.HandlerArgIndex]
		if handlerArg.GetKind() == metadata.KindIdent || handlerArg.GetKind() == metadata.KindFuncLit {
			handlerName := handlerArg.GetName()
			// Use variable tracing to resolve handler
			originVar, originPkg, originType, _ := r.traceVariable(
				handlerName,
				r.contextProvider.GetString(edge.Caller.Name),
				r.contextProvider.GetString(edge.Caller.Pkg),
			)
			if originVar != "" {
				routeInfo.Handler = originVar
			}
			if originPkg != "" {
				routeInfo.Package = originPkg
			}

			var originTypeStr string
			if originType != nil {
				originTypeStr = r.contextProvider.GetArgumentInfo(originType)
			}
			if originTypeStr != "" {
				routeInfo.Summary = originTypeStr
			}
		}
	}

	return found
}

// extractRouteDetails extracts route details from a node
//
//nolint:gocyclo // route detail extraction with multiple pattern sources
func (r *RoutePatternMatcherImpl) extractRouteDetails(node TrackerNodeInterface, routeInfo *RouteInfo) bool {
	found := false
	edge := node.GetEdge()

	switch {
	case r.pattern.MethodFromCall:
		funcName := r.contextProvider.GetString(edge.Callee.Name)
		routeInfo.Method = r.extractMethodFromFunctionNameWithConfig(funcName, r.pattern.MethodExtraction)
		found = true
	case r.pattern.MethodFromHandler && r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex:
		// Extract method from handler function name
		handlerArg := edge.Args[r.pattern.HandlerArgIndex]
		handlerName := r.contextProvider.GetArgumentInfo(handlerArg)
		if handlerName != "" {
			routeInfo.Method = r.extractMethodFromFunctionNameWithConfig(handlerName, r.pattern.MethodExtraction)
			found = true
		}
	case r.pattern.MethodArgIndex >= 0 && len(edge.Args) > r.pattern.MethodArgIndex:
		methodArg := edge.Args[r.pattern.MethodArgIndex]
		methodValue := methodArg.GetValue()

		// Handle different method extraction patterns
		if methodValue != "" {
			// Clean up method value - remove quotes and extract HTTP method
			cleanMethod := strings.Trim(methodValue, "\"'")

			// Check if it's a valid HTTP method
			if r.isValidHTTPMethod(cleanMethod) {
				routeInfo.Method = strings.ToUpper(cleanMethod)
				found = true
			} else {
				// If not a valid method, try to extract from argument info
				argInfo := r.contextProvider.GetArgumentInfo(methodArg)
				if argInfo != "" {
					cleanArgInfo := strings.Trim(argInfo, "\"'")
					if r.isValidHTTPMethod(cleanArgInfo) {
						routeInfo.Method = strings.ToUpper(cleanArgInfo)
						found = true
					}
				}
			}
		}

		// If we still don't have a method, try to infer from context (if enabled)
		if routeInfo.Method == "" && r.pattern.MethodExtraction != nil && r.pattern.MethodExtraction.InferFromContext {
			routeInfo.Method = r.inferMethodFromContext(node, edge)
			found = true
		}
	}

	if r.pattern.PathFromArg && len(edge.Args) > r.pattern.PathArgIndex {
		arg := edge.Args[r.pattern.PathArgIndex]
		routeInfo.Path = r.contextProvider.GetArgumentInfo(arg)
		// If path is a variable name, resolve via assignment map
		if arg.GetKind() == metadata.KindIdent && !strings.HasPrefix(routeInfo.Path, "/") {
			if assignments, exists := edge.AssignmentMap[arg.GetName()]; exists && len(assignments) > 0 {
				resolved := r.contextProvider.GetArgumentInfo(&assignments[0].Value)
				resolved = strings.Trim(resolved, "\"")
				if strings.HasPrefix(resolved, "/") {
					routeInfo.Path = resolved
				}
			}
		}
		if routeInfo.Path == "" {
			routeInfo.Path = "/"
		}
		found = true
	}

	if r.pattern.HandlerFromArg && len(edge.Args) > r.pattern.HandlerArgIndex {
		routeInfo.Handler = r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.HandlerArgIndex])
		routeInfo.Function = r.contextProvider.GetArgumentInfo(edge.Args[r.pattern.HandlerArgIndex])

		pkg := edge.Args[r.pattern.HandlerArgIndex].GetPkg()
		if pkg == "" {
			if node != nil && edge != nil && edge.Args[r.pattern.HandlerArgIndex].Fun != nil {
				pkg = edge.Args[r.pattern.HandlerArgIndex].Fun.GetPkg()
			}
		}
		routeInfo.Package = pkg
		found = true
	}

	return found
}

// isValidHTTPMethod checks if a string is a valid HTTP method
func (r *RoutePatternMatcherImpl) isValidHTTPMethod(method string) bool {
	validMethods := []string{
		"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE", "CONNECT",
	}

	upperMethod := strings.ToUpper(method)
	for _, valid := range validMethods {
		if upperMethod == valid {
			return true
		}
	}
	return false
}

// inferMethodFromContext attempts to infer HTTP method from context
func (r *RoutePatternMatcherImpl) inferMethodFromContext(node TrackerNodeInterface, edge *metadata.CallGraphEdge) string {
	// Check if context inference is enabled
	if r.pattern.MethodExtraction == nil || !r.pattern.MethodExtraction.InferFromContext {
		return ""
	}

	// Try to find method from chained calls (like Mux .Methods("GET"))
	if node != nil {
		// Look for parent or sibling nodes that might contain method info
		parent := node.GetParent()
		if parent != nil {
			// Check if parent has method information
			for _, child := range parent.GetChildren() {
				if child != node && child.GetEdge() != nil {
					childEdge := child.GetEdge()
					callName := r.contextProvider.GetString(childEdge.Callee.Name)

					// Look for Methods call
					if callName == "Methods" && len(childEdge.Args) > 0 {
						methodArg := childEdge.Args[0]
						methodValue := strings.Trim(methodArg.GetValue(), "\"'")
						if r.isValidHTTPMethod(methodValue) {
							return strings.ToUpper(methodValue)
						}

						// Try argument info as well
						argInfo := r.contextProvider.GetArgumentInfo(methodArg)
						cleanArgInfo := strings.Trim(argInfo, "\"'")
						if r.isValidHTTPMethod(cleanArgInfo) {
							return strings.ToUpper(cleanArgInfo)
						}
					}
				}
			}
		}
	}

	// Try to infer from handler function name using pattern's method extraction config
	handlerName := r.contextProvider.GetString(edge.Caller.Name)
	if handlerName != "" {
		method := r.extractMethodFromFunctionNameWithConfig(handlerName, r.pattern.MethodExtraction)
		if method != "" && method != "POST" { // Don't use POST as default
			return method
		}
	}

	// Also try the handler from the arguments if available
	if len(edge.Args) > 1 {
		handlerArg := edge.Args[1] // Typically the handler is the second argument
		argInfo := r.contextProvider.GetArgumentInfo(handlerArg)
		if argInfo != "" {
			method := r.extractMethodFromFunctionNameWithConfig(argInfo, r.pattern.MethodExtraction)
			if method != "" && method != "POST" {
				return method
			}
		}
	}

	// Default fallback
	return "GET"
}

// MountPatternMatcherImpl implements MountPatternMatcher
type MountPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern MountPattern
}

// NewMountPatternMatcher creates a new mount pattern matcher
func NewMountPatternMatcher(pattern MountPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *MountPatternMatcherImpl {
	return &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the mount pattern
func (m *MountPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	return baseMatchNode(node, m.pattern.BasePattern, m.contextProvider) && m.pattern.IsMount
}

// GetPattern returns the mount pattern
func (m *MountPatternMatcherImpl) GetPattern() interface{} {
	return m.pattern
}

// GetPriority returns the priority of this pattern
func (m *MountPatternMatcherImpl) GetPriority() int {
	return basePriority(m.pattern.BasePattern)
}

// ExtractMount extracts mount information from a matched node
func (m *MountPatternMatcherImpl) ExtractMount(node TrackerNodeInterface) MountInfo {
	mountInfo := MountInfo{
		Pattern: m.pattern,
	}

	edge := node.GetEdge()
	// Extract path if available
	if m.pattern.PathFromArg && len(edge.Args) > m.pattern.PathArgIndex {
		mountInfo.Path = m.contextProvider.GetArgumentInfo(edge.Args[m.pattern.PathArgIndex])
	}

	// Extract router argument if available
	if m.pattern.RouterArgIndex >= 0 && len(edge.Args) > m.pattern.RouterArgIndex {
		mountInfo.RouterArg = edge.Args[m.pattern.RouterArgIndex]

		// Trace router origin
		m.traceRouterOrigin(mountInfo.RouterArg, node)

		// Find assignment function
		mountInfo.Assignment = m.findAssignmentFunction(mountInfo.RouterArg)
	}

	return mountInfo
}

// RequestPatternMatcherImpl implements RequestPatternMatcher
type RequestPatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RequestBodyPattern
}

// NewRequestPatternMatcher creates a new request pattern matcher
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *APISpecConfig, contextProvider ContextProvider, typeResolver TypeResolver) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider, typeResolver),
		pattern:            pattern,
	}
}

// MatchNode checks if a node matches the request pattern
func (r *RequestPatternMatcherImpl) MatchNode(node TrackerNodeInterface) bool {
	return baseMatchNode(node, r.pattern.BasePattern, r.contextProvider)
}

// GetPattern returns the request pattern
func (r *RequestPatternMatcherImpl) GetPattern() interface{} {
	return r.pattern
}

// GetPriority returns the priority of this pattern
func (r *RequestPatternMatcherImpl) GetPriority() int {
	return basePriority(r.pattern.BasePattern)
}

// ExtractRequest extracts request information from a matched node
func (r *RequestPatternMatcherImpl) ExtractRequest(node TrackerNodeInterface, route *RouteInfo) *RequestInfo {
	// For chained Decode calls (e.g., json.NewDecoder(r.Body).Decode(&user)),
	// check if the parent call's first argument is r.Body. If not, this isn't
	// a request body decode and should be skipped.
	edge := node.GetEdge()
	if edge != nil && edge.ChainParent != nil {
		parentName := r.contextProvider.GetString(edge.ChainParent.Callee.Name)
		if parentName == "NewDecoder" && len(edge.ChainParent.Args) > 0 {
			parentArg := r.contextProvider.GetArgumentInfo(edge.ChainParent.Args[0])
			// Only accept r.Body or request.Body as the decoder source
			if !strings.Contains(parentArg, "Body") {
				return nil
			}
		}
	}

	reqInfo := &RequestInfo{
		ContentType: r.cfg.Defaults.RequestContentType,
	}

	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {
		arg := edge.Args[r.pattern.TypeArgIndex]
		bodyType := r.contextProvider.GetArgumentInfo(arg)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			bodyType = determineLiteralType(bodyType)
		} else {
			// Check for resolved type information in the CallArgument
			if resolvedType := arg.GetResolvedType(); resolvedType != "" {
				bodyType = resolvedType
			} else if arg.IsGenericType && arg.GenericTypeName != -1 {
				// If it's a generic type, try to resolve it from the edge's type parameters
				if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
					bodyType = concreteType
				}
			}

			// Trace type origin
			bodyType = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
			}
		}

		reqInfo.BodyType = preprocessingBodyType(bodyType)
		schema, _ := mapGoTypeToOpenAPISchema(route.UsedTypes, bodyType, route.Metadata, r.cfg, nil)
		reqInfo.Schema = schema
	}

	if reqInfo.BodyType == "" {
		return nil
	}

	return reqInfo
}

// Helper methods for BasePatternMatcher
func (b *BasePatternMatcher) matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	re, err := getCachedPatternRegex(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func (b *BasePatternMatcher) traceVariable(varName, funcName, pkgName string) (originVar, originPkg string, originType *metadata.CallArgument, originFunc string) {
	ctxImpl, ok := b.contextProvider.(*ContextProviderImpl)
	if !ok || ctxImpl.meta == nil {
		return varName, pkgName, nil, originFunc
	}
	originVar, originPkg, originType, originFunc = metadata.TraceVariableOrigin(varName, funcName, pkgName, ctxImpl.meta)
	return originVar, originPkg, originType, originFunc
}

func (b *BasePatternMatcher) traceRouterOrigin(routerArg *metadata.CallArgument, node TrackerNodeInterface) {
	// Trace router origin based on argument kind
	edge := node.GetEdge()
	switch routerArg.GetKind() {
	case metadata.KindIdent:
		b.traceVariable(
			routerArg.GetName(),
			b.contextProvider.GetString(edge.Caller.Name),
			b.contextProvider.GetString(edge.Caller.Pkg),
		)
	case metadata.KindUnary, metadata.KindStar:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	case metadata.KindSelector:
		if routerArg.X != nil {
			b.traceVariable(
				routerArg.X.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	case metadata.KindCall:
		if routerArg.Fun != nil {
			b.traceVariable(
				routerArg.Fun.GetName(),
				b.contextProvider.GetString(edge.Caller.Name),
				b.contextProvider.GetString(edge.Caller.Pkg),
			)
		}
	}
}

func (b *BasePatternMatcher) findAssignmentFunction(arg *metadata.CallArgument) *metadata.CallArgument {
	// Use contextProvider to access metadata
	ctxImpl, ok := b.contextProvider.(*ContextProviderImpl)
	if !ok || ctxImpl.meta == nil {
		return nil
	}
	meta := ctxImpl.meta

	for _, edge := range meta.CallGraph {
		for _, varAssignments := range edge.AssignmentMap {
			for _, assign := range varAssignments {
				varName := b.contextProvider.GetString(assign.VariableName)
				varType := b.contextProvider.GetString(assign.ConcreteType)
				varPkg := b.contextProvider.GetString(assign.Pkg)

				if varName == arg.GetName() && varPkg == arg.GetPkg() && arg.X != nil && arg.X.Type != -1 && varType == arg.X.GetType() {
					// Get the function name directly (it's already a string)
					for _, targetArg := range edge.Args {
						if targetArg.GetKind() == metadata.KindCall && targetArg.Fun != nil {
							return targetArg.Fun
						}
					}
				}
			}
		}
	}
	return nil
}

// resolveTypeOrigin traces the origin of a type through assignments and type parameters
func (r *RequestPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) string {
	// Request-specific: trace generic origin via TypeParts before shared logic
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	typeParts := TypeParts(originalType)
	genericType := traceGenericOrigin(node, typeParts)
	if genericType != "" {
		return genericType
	}

	// Delegate to shared logic (checkFuncLit=true to match original Request behavior)
	return sharedResolveTypeOrigin(arg, node, originalType, r.contextProvider, true)
}

func traceGenericOrigin(node TrackerNodeInterface, typeParts Parts) string {
	typeParams := node.GetTypeParamMap()

	if len(typeParams) > 0 && typeParts.TypeName != "" {
		searchType := typeParts.TypeName
		foundMapping := false

		for {
			concreteType, exists := typeParams[searchType]
			if !exists || concreteType == "" {
				break
			}
			searchType = concreteType
			foundMapping = true
		}
		// Only return the concrete type if we found a mapping
		if foundMapping {
			return searchType
		}
	}
	return ""
}

func (b *BasePatternMatcher) extractMethodFromFunctionNameWithConfig(funcName string, config *MethodExtractionConfig) string {
	if funcName == "" {
		return ""
	}

	// Use default config if none provided
	if config == nil {
		config = DefaultMethodExtractionConfig()
	}

	// Prepare function name based on case sensitivity
	searchName := funcName
	if !config.CaseSensitive {
		searchName = strings.ToLower(funcName)
	}

	// Sort mappings by priority (highest first)
	mappings := make([]MethodMapping, len(config.MethodMappings))
	copy(mappings, config.MethodMappings)

	// Simple bubble sort by priority (descending)
	for i := 0; i < len(mappings)-1; i++ {
		for j := 0; j < len(mappings)-i-1; j++ {
			if mappings[j].Priority < mappings[j+1].Priority {
				mappings[j], mappings[j+1] = mappings[j+1], mappings[j]
			}
		}
	}

	// Check prefix matches first if enabled
	if config.UsePrefix {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				searchPattern := pattern
				if !config.CaseSensitive {
					searchPattern = strings.ToLower(pattern)
				}

				if strings.HasPrefix(searchName, searchPattern) {
					// Make sure it's a word boundary (not part of another word)
					if len(searchName) == len(searchPattern) || !b.isLetter(rune(searchName[len(searchPattern)])) {
						return mapping.Method
					}
				}
			}
		}
	}

	// Check contains matches if enabled
	if config.UseContains {
		for _, mapping := range mappings {
			for _, pattern := range mapping.Patterns {
				searchPattern := pattern
				if !config.CaseSensitive {
					searchPattern = strings.ToLower(pattern)
				}

				if strings.Contains(searchName, searchPattern) {
					return mapping.Method
				}
			}
		}
	}

	return config.DefaultMethod
}

// isLetter checks if a rune is a letter
func (b *BasePatternMatcher) isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
