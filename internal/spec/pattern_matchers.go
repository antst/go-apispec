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
	warnings        *WarningSink // non-fatal analysis warnings → stderr (spec 009, FR-008)
}

// warn records a non-fatal analysis warning (lazily creating a stderr sink). Used by
// the conditional-analysis degrade paths (FR-008/FR-012).
func (b *BasePatternMatcher) warn(pos, msg string) {
	if b == nil {
		return
	}
	if b.warnings == nil {
		b.warnings = NewWarningSink()
	}
	b.warnings.Warn(pos, msg)
}

// NewBasePatternMatcher creates a new base pattern matcher
func NewBasePatternMatcher(cfg *APISpecConfig, contextProvider ContextProvider) *BasePatternMatcher {
	return &BasePatternMatcher{
		contextProvider: contextProvider,
		cfg:             cfg,
		schemaMapper:    NewSchemaMapper(cfg),
	}
}

// RoutePatternMatcherImpl implements RoutePatternMatcher
type RoutePatternMatcherImpl struct {
	*BasePatternMatcher
	pattern RoutePattern
}

// NewRoutePatternMatcher creates a new route pattern matcher
func NewRoutePatternMatcher(pattern RoutePattern, cfg *APISpecConfig, contextProvider ContextProvider) *RoutePatternMatcherImpl {
	return &RoutePatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
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
		// Go 1.22+ net/http ServeMux accepts an optional method prefix in
		// the pattern: `HandleFunc("GET /path", h)`. The space separator
		// makes the syntax unambiguous — URL paths can't contain spaces —
		// so this heuristic is safe across frameworks, not just stdlib.
		if method, path, ok := splitMethodPrefixedPath(routeInfo.Path); ok {
			// Path-prefix syntax is explicit user intent — override the
			// initial-default method as well as an empty one. (The route
			// info is initialized with Method=POST by default; without this
			// override, "GET /health/live" would still emit a POST operation.)
			routeInfo.Path = path
			routeInfo.Method = method
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
	return isHTTPMethod(method)
}

// isHTTPMethod is the package-level version used by helpers that don't have a
// matcher in scope. Method names are compared case-insensitively against the
// fixed RFC 7231 / 5789 / 7540 set.
func isHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE", "CONNECT":
		return true
	}
	return false
}

// splitMethodPrefixedPath recognises the Go 1.22+ net/http ServeMux pattern
// syntax `"[METHOD ][HOST]/[PATH]"` and returns the method plus the path
// component. Both bare paths (`"GET /health"`) and host-qualified patterns
// (`"GET example.com/api"` → method=GET, path=/api) are accepted; the host
// is dropped because OpenAPI carries it in `servers:`, not the path key.
// Returns ok=false for non-pattern strings so unrelated values pass through
// unchanged.
func splitMethodPrefixedPath(s string) (method, path string, ok bool) {
	space := strings.IndexByte(s, ' ')
	if space <= 0 {
		return "", "", false
	}
	candidate := s[:space]
	rest := strings.TrimLeft(s[space+1:], " ")
	if !isHTTPMethod(candidate) {
		return "", "", false
	}
	// Either `/path` (no host) or `host.example.com/path` (host-qualified).
	// In the latter case strip the host so the OpenAPI key stays `/path`.
	if strings.HasPrefix(rest, "/") {
		return strings.ToUpper(candidate), rest, true
	}
	if slash := strings.IndexByte(rest, '/'); slash > 0 {
		return strings.ToUpper(candidate), rest[slash:], true
	}
	return "", "", false
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
func NewMountPatternMatcher(pattern MountPattern, cfg *APISpecConfig, contextProvider ContextProvider) *MountPatternMatcherImpl {
	return &MountPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
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
func NewRequestPatternMatcher(pattern RequestBodyPattern, cfg *APISpecConfig, contextProvider ContextProvider) *RequestPatternMatcherImpl {
	return &RequestPatternMatcherImpl{
		BasePatternMatcher: NewBasePatternMatcher(cfg, contextProvider),
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
	// Reject decode calls that don't actually read the HTTP request body
	// (wrong decoder source, or an unrelated arg-sourced json.Unmarshal).
	edge := node.GetEdge()
	if !r.decodeReadsRequestBody(node, edge) {
		return nil
	}

	reqInfo := &RequestInfo{
		ContentType: r.cfg.Defaults.RequestContentType,
	}

	// Frame whose call graph drives field-format inference. Defaults to the
	// function holding the decode call, but is re-anchored to the caller when
	// the decode is resolved through an extracted helper (issue #36).
	fieldFrameBaseID := edge.Caller.BaseID()

	if r.pattern.TypeFromArg && len(edge.Args) > r.pattern.TypeArgIndex {
		arg := edge.Args[r.pattern.TypeArgIndex]
		bodyType := r.contextProvider.GetArgumentInfo(arg)
		var bodyRef *metadata.TypeRef // resolution-emitted structured type (Phase 3)

		// Check if this is a literal value - if so, determine appropriate type
		if arg.GetKind() == metadata.KindLiteral {
			bodyType = determineLiteralType(bodyType)
			// The literal type is a freshly-determined primitive string — parse it
			// once here, at the boundary where it originates (Phase 4: keep bodyRef
			// in lockstep so the schema path never re-derives it).
			bodyRef = metadata.ParseTypeRef(bodyType)
		} else if s := bodyTypeFromMetadataRef(arg.TypeRef, route.Metadata, r.cfg); s != "" {
			// The decode target (&dto) carries a TypeRef whose named leaf is a type
			// in metadata — its declared type IS the body type, no origin tracing
			// needed. Use the canonical fully-qualified form (deref'd to match the
			// field path) so the request body references the same component as a
			// field of that type. Generic/helper decode targets (leaf RefParam) get
			// "" here and fall through to the resolution chain below (T009/T011).
			bodyType = strings.TrimPrefix(s, "*")
			bodyRef = metadata.ParseTypeRef(bodyType)
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

			// Trace type origin (sets bodyRef in lockstep with bodyType)
			bodyType, bodyRef = r.resolveTypeOrigin(arg, node, bodyType)

			// Apply dereferencing if needed — unwrap the ref's pointer layer in
			// lockstep with the string strip (Phase 4).
			if r.pattern.Deref && strings.HasPrefix(bodyType, "*") {
				bodyType = strings.TrimPrefix(bodyType, "*")
				bodyRef = derefPointerRef(bodyRef)
			}
		}

		reqInfo.BodyType = preprocessingBodyType(bodyType)
		reqInfo.DecodeTargetVar = decodeTargetVarName(arg)

		// Interprocedural fallback (issues #36, #39): when the decode lives in an
		// extracted (possibly shared or generic) helper, recover the concrete
		// type from the route handler's call site and re-anchor the decode
		// target + field-inference frame onto the handler.
		bodyType, fieldFrameBaseID, bodyRef = r.refineBodyTypeThroughHelper(node, reqInfo, bodyType, fieldFrameBaseID, bodyRef, route)

		// Phase 4: bodyRef has been kept in lockstep with bodyType through every
		// transform above (resolution, deref, literal/metadata-ref boundary, helper
		// refinement), so the schema generator consumes the structure directly with
		// no re-parse. The corpus guard TestEveryResolvedBodyTypeReachesSchemaWithRef
		// (internal/engine) asserts the invariant it actually enforces: every resolved
		// (non-empty) request/response BodyType reaches schema generation with a
		// non-nil BodyTypeRef. Exact String()==bodyType equality is not asserted —
		// resolution/qualification can canonicalise the rendering — but byte-identical
		// schema output is held by the SC-003 golden.
		reqInfo.BodyTypeRef = bodyRef
		schema, _ := schemaForType(route.UsedTypes, bodyType, bodyRef, route.Metadata, r.cfg, nil)
		reqInfo.Schema = schema
	}

	if reqInfo.BodyType == "" {
		return nil
	}

	// A handler reading the body via Decode/Bind/etc. is committed to receiving
	// it — empty input fails decoding or returns a zero-valued struct that the
	// handler then 400s on. Marking required:true reflects that contract.
	reqInfo.Required = true

	// Walk the handler (and one or more levels of extracted helpers) for
	// `<targetVar>.<field>` accesses fed into known converters and back-
	// propagate their schema formats onto the struct fields. Cheap (we already
	// have the call graph indexed by caller) and strictly additive — never
	// erases existing information.
	if reqInfo.DecodeTargetVar != "" && route.Metadata != nil {
		applyJSONFieldConverterFormats(
			reqInfo.DecodeTargetVar,
			reqInfo.BodyType,
			fieldFrameBaseID,
			route,
		)
	}

	return reqInfo
}

// decodeReadsRequestBody reports whether a matched decode call actually reads
// the HTTP request body, filtering out two false positives:
//
//   - a chained json.NewDecoder(x).Decode(&v) whose decoder source x isn't
//     r.Body (the parent call's first arg); and
//   - an arg-sourced decode — json.Unmarshal(data, &v), render.DecodeJSON(r, &v)
//     — whose data argument doesn't trace to the request. Otherwise an unrelated
//     json.Unmarshal reachable deep in the handler's call graph (e.g. a DB-column
//     parse) is misattributed as the request body and, depending on call-graph
//     map ordering, flaps run-to-run (issue #52).
func (r *RequestPatternMatcherImpl) decodeReadsRequestBody(node TrackerNodeInterface, edge *metadata.CallGraphEdge) bool {
	if edge == nil {
		return true
	}
	if edge.ChainParent != nil {
		parentName := r.contextProvider.GetString(edge.ChainParent.Callee.Name)
		if parentName == "NewDecoder" && len(edge.ChainParent.Args) > 0 {
			parentArg := r.contextProvider.GetArgumentInfo(edge.ChainParent.Args[0])
			if !strings.Contains(parentArg, "Body") {
				return false
			}
		}
	}
	if r.pattern.TypeFromArg && r.pattern.TypeArgIndex >= 1 && len(edge.Args) > r.pattern.TypeArgIndex {
		if !r.decodeSourceTracesToRequest(node, edge.Args[r.pattern.TypeArgIndex-1]) {
			return false
		}
	}
	return true
}

// decodeSourceTracesToRequest reports whether a decode call's data-source
// argument is, or derives from, the HTTP request — directly (r.Body, a
// *http.Request value), or via a local assignment within the decode's own
// function (e.g. b := io.ReadAll(r.Body); json.Unmarshal(b, &v)). It rejects
// decodes of unrelated data — e.g. a DB-column json.Unmarshal reachable deep in
// the handler's call graph — that would otherwise be misinferred as the request
// body (issue #52).
func (r *RequestPatternMatcherImpl) decodeSourceTracesToRequest(node TrackerNodeInterface, src *metadata.CallArgument) bool {
	if src == nil {
		return true // nothing to validate
	}
	if argReferencesRequest(src) {
		return true
	}
	// The source is a local/param ident: accept when it's assigned from the
	// request body within the decode's own function.
	if src.GetKind() != metadata.KindIdent || node == nil {
		return false
	}
	edge := node.GetEdge()
	if edge == nil {
		return false
	}
	meta := metadataFromContextProvider(r.contextProvider)
	if meta == nil {
		return false
	}
	fn := findFunction(meta, meta.StringPool.GetString(edge.Caller.Pkg), meta.StringPool.GetString(edge.Caller.Name))
	if fn == nil {
		return false
	}
	assigns := fn.AssignmentMap[src.GetName()]
	for i := range assigns {
		if argReferencesRequest(&assigns[i].Value) {
			return true
		}
	}
	return false
}

// argReferencesRequest walks a CallArgument expression tree looking for a
// reference to the HTTP request or its body: a `.Body` selector (r.Body,
// request.Body) or a value typed `*http.Request`/`http.Request`. Recurses
// through call expressions so io.ReadAll(r.Body) and similar are recognised.
func argReferencesRequest(arg *metadata.CallArgument) bool {
	if arg == nil {
		return false
	}
	if t := arg.GetType(); strings.Contains(t, "http.Request") {
		return true
	}
	if arg.GetKind() == metadata.KindSelector && arg.Sel != nil && arg.Sel.GetName() == "Body" {
		return true
	}
	if argReferencesRequest(arg.X) || argReferencesRequest(arg.Fun) || argReferencesRequest(arg.Sel) {
		return true
	}
	for _, a := range arg.Args {
		if argReferencesRequest(a) {
			return true
		}
	}
	return false
}

// refineBodyTypeThroughHelper resolves a decode that lives in an extracted
// helper through the route handler's call site (issues #36, #39).
//
// When the decode call sits directly in the route handler there is nothing
// interprocedural to do. Otherwise it resolves the decode target parameter to
// the concrete argument the handler passed (`&body`) and:
//
//   - re-anchors the field-inference frame and decode-target variable onto the
//     handler — needed even when the body type already resolved concretely via
//     a generic type parameter (issue #39 Variant B), because the converter
//     calls (uuid.Parse(body.X)) live in the handler, not the helper; and
//   - overrides the body type only when the current one is free-form (an `any`
//     parameter — issue #39 Variant A); a concrete generic instantiation wins.
//
// reqInfo is mutated in place; the returned (bodyType, fieldFrameBaseID) replace
// the caller's locals for schema generation and field inference.
// refineBodyTypeThroughHelper returns the (possibly interprocedurally refined)
// body type, its field-inference frame, and a *TypeRef kept in lockstep with the
// returned string (Phase 4): when the call-site recovery replaces the body type,
// the ref is re-derived from the new concrete string at that boundary; otherwise
// the caller's existing bodyRef passes through unchanged.
func (r *RequestPatternMatcherImpl) refineBodyTypeThroughHelper(node TrackerNodeInterface, reqInfo *RequestInfo, bodyType, fieldFrameBaseID string, bodyRef *metadata.TypeRef, route *RouteInfo) (string, string, *metadata.TypeRef) {
	if node == nil || route == nil || route.Metadata == nil {
		return bodyType, fieldFrameBaseID, bodyRef
	}
	edge := node.GetEdge()
	if edge == nil {
		return bodyType, fieldFrameBaseID, bodyRef
	}
	// Inline decode: the decode call sits directly in the route handler, so the
	// frame is already correct — nothing to resolve across a call boundary.
	if edgeCallerIsRouteHandler(edge, route) {
		return bodyType, fieldFrameBaseID, bodyRef
	}

	resolved, varName, frameBaseID := resolveBodyTypeThroughCallSite(node, reqInfo.DecodeTargetVar, route, r.contextProvider)
	if resolved == "" {
		return bodyType, fieldFrameBaseID, bodyRef
	}

	reqInfo.DecodeTargetVar = varName
	fieldFrameBaseID = frameBaseID

	if isFreeFormBodyType(reqInfo.BodyType) && !isFreeFormBodyType(resolved) {
		reqInfo.BodyType = preprocessingBodyType(resolved)
		bodyType = resolved
		bodyRef = metadata.ParseTypeRef(bodyType)
	}
	return bodyType, fieldFrameBaseID, bodyRef
}

// maxCallSiteDepth bounds how far up the tracker tree resolveBodyTypeThroughCallSite
// walks looking for the call site that invoked the decode helper.
const maxCallSiteDepth = 16

// isFreeFormBodyType reports whether a resolved body type carries no concrete
// schema — the empty string or a bare interface. These are the only cases the
// interprocedural fallback should try to refine; a genuine `any` body that
// can't be resolved is left free-form.
func isFreeFormBodyType(t string) bool {
	switch strings.TrimSpace(t) {
	case "", "any", "interface{}", "interface {}", "object":
		return true
	}
	return false
}

// resolveBodyTypeThroughCallSite resolves a decode that lives in an extracted
// helper to the concrete body type passed at the call site, returning that type
// plus the caller-frame variable name and base ID (so downstream field
// inference runs in the right frame).
//
// It first disambiguates by the route's own handler (issue #39): a strict-decode
// helper shared by several handlers is deduplicated to a single tracker-tree
// node whose parent points at just one caller, so the parent walk alone resolves
// every route to that one caller's type. Resolving through the call edge whose
// caller IS this route's handler picks the right one. The tracker-tree walk
// remains as a fallback for single-caller helpers reached through deeper chains
// (issue #36).
func resolveBodyTypeThroughCallSite(node TrackerNodeInterface, paramName string, route *RouteInfo, cp ContextProvider) (bodyType, varName, callerBaseID string) {
	if node == nil || paramName == "" {
		return "", "", ""
	}
	edge := node.GetEdge()
	if edge == nil {
		return "", "", ""
	}

	// (1) Route-handler disambiguation: among every edge that calls the
	// enclosing helper, pick the one whose caller is this route's handler.
	if route != nil && route.Metadata != nil {
		candidates := route.Metadata.Callees[edge.Caller.BaseID()]
		// Pass 1 — exact handler identity (free functions and methods registered
		// directly, e.g. r.Post("/x", h.Copy)).
		for _, e := range candidates {
			if !callerMatchesHandlerExact(e, route) {
				continue
			}
			if bt, vn := concreteTypeFromParamArg(e, paramName, cp); bt != "" {
				return bt, vn, e.Caller.BaseID()
			}
		}
		// Pass 2 — selector-chain registration (issue #41 / file-service):
		// handlers wired as `deps.DocumentHandler.Copy` inside an r.Route(...)
		// closure leave route.Function in registration-site form
		// ("main-->main.deps.DocumentHandler.Copy"), which never equals the
		// method's own BaseID ("pkg/api.DocumentHandler.Copy"). route.Package
		// still names the handler's package, so fall back to a (package, method
		// name) match — accepted only when it resolves to a single candidate so
		// same-named methods can't mis-bind.
		if e := uniquePkgNameHandler(candidates, route); e != nil {
			if bt, vn := concreteTypeFromParamArg(e, paramName, cp); bt != "" {
				return bt, vn, e.Caller.BaseID()
			}
		}
	}

	// (2) Fallback: walk the tracker tree to the nearest ancestor edge that
	// invoked the enclosing helper.
	enclosingName := edge.Caller.Name
	enclosingPkg := edge.Caller.Pkg
	for depth, cur := 0, node.GetParent(); cur != nil && depth < maxCallSiteDepth; cur, depth = cur.GetParent(), depth+1 {
		anc := cur.GetEdge()
		if anc == nil {
			continue
		}
		if anc.Callee.Name != enclosingName || anc.Callee.Pkg != enclosingPkg {
			continue
		}
		bt, vn := concreteTypeFromParamArg(anc, paramName, cp)
		if bt == "" {
			return "", "", ""
		}
		return bt, vn, anc.Caller.BaseID()
	}
	return "", "", ""
}

// edgeCallerIsRouteHandler reports whether the edge's caller is the route's
// handler, by exact identity or a (package, method-name) match. Used to detect
// an inline decode (the decode call sitting directly in the handler).
func edgeCallerIsRouteHandler(e *metadata.CallGraphEdge, route *RouteInfo) bool {
	return callerMatchesHandlerExact(e, route) || callerMatchesHandlerByPkgName(e, route)
}

// callerMatchesHandlerExact matches the edge's caller against the route's
// handler by exact identity. RouteInfo.Function carries the handler in several
// forms:
//
//	free function: "pkg.Func"                       (== caller BaseID)
//	method:        "pkg-->pkg.RecvType.Method"      (TypeSep-prefixed BaseID)
//	bare:          "Func"                           (with Package set separately)
//
// The first two both end in the caller's BaseID, so the primary check compares
// against e.Caller.BaseID() after stripping any TypeSep prefix; the bare form
// is handled as a fallback.
func callerMatchesHandlerExact(e *metadata.CallGraphEdge, route *RouteInfo) bool {
	if route == nil || route.Metadata == nil || route.Function == "" {
		return false
	}
	want := route.Function
	if idx := strings.Index(want, TypeSep); idx >= 0 {
		want = want[idx+len(TypeSep):]
	}
	if e.Caller.BaseID() == want {
		return true
	}
	name := stringFromPool(route.Metadata, e.Caller.Name)
	if name == "" {
		return false
	}
	return route.Function == name &&
		(route.Package == "" || route.Package == stringFromPool(route.Metadata, e.Caller.Pkg))
}

// callerMatchesHandlerByPkgName matches the edge's caller against the route's
// handler by (package, function/method name). This recovers the handler when it
// is registered through a selector chain (e.g. `deps.DocumentHandler.Copy`
// inside an r.Route(...) closure), where RouteInfo.Function is the registration
// expression rather than the method's own identity but RouteInfo.Package still
// names the handler's defining package. Deliberately ignores the receiver type,
// which the registration form does not carry reliably (it uses the field name).
func callerMatchesHandlerByPkgName(e *metadata.CallGraphEdge, route *RouteInfo) bool {
	if route == nil || route.Metadata == nil || route.Function == "" || route.Package == "" {
		return false
	}
	name := stringFromPool(route.Metadata, e.Caller.Name)
	if name == "" || route.Package != stringFromPool(route.Metadata, e.Caller.Pkg) {
		return false
	}
	return lastDotSegment(route.Function) == name
}

// uniquePkgNameHandler returns the single candidate whose caller matches the
// route handler by (package, name), or nil when zero or more than one match —
// the uniqueness guard prevents two same-named methods that share a decode
// helper from binding to the wrong endpoint.
func uniquePkgNameHandler(candidates []*metadata.CallGraphEdge, route *RouteInfo) *metadata.CallGraphEdge {
	var match *metadata.CallGraphEdge
	for _, e := range candidates {
		if !callerMatchesHandlerByPkgName(e, route) {
			continue
		}
		if match != nil {
			return nil // ambiguous
		}
		match = e
	}
	return match
}

// lastDotSegment returns the substring after the final '.', or s when there is
// none — i.e. the function/method name from a dotted identifier.
func lastDotSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// concreteTypeFromParamArg resolves the concrete body type and caller-frame
// variable name from a call edge's mapping of paramName to the argument the
// caller passed (e.g. dst -> &body). Returns "","" when that argument isn't a
// plain identifier, optionally wrapped in a single &/* (covering `&body`).
func concreteTypeFromParamArg(edge *metadata.CallGraphEdge, paramName string, cp ContextProvider) (bodyType, varName string) {
	callArg, ok := edge.ParamArgMap[paramName]
	if !ok {
		return "", ""
	}
	base := unwrapArgRefs(&callArg)
	if base == nil || base.GetKind() != metadata.KindIdent {
		return "", ""
	}
	return strings.TrimPrefix(cp.GetArgumentInfo(base), "*"), base.GetName()
}

// decodeTargetVarName returns the local variable name that the decode call's
// type-arg refers to (e.g., "body" in `Decode(&body)` or `Decode(body)`).
// Empty when the argument isn't a simple variable reference (literal, call
// expression, complex expression).
func decodeTargetVarName(arg *metadata.CallArgument) string {
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		return arg.GetName()
	case metadata.KindUnary, metadata.KindStar:
		// `&body` / `*body` — unwrap one level.
		if arg.X != nil && arg.X.GetKind() == metadata.KindIdent {
			return arg.X.GetName()
		}
	}
	return ""
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
func (r *RequestPatternMatcherImpl) resolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) (string, *metadata.TypeRef) {
	// Request-specific: trace generic origin through the type-param tree before shared logic
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType, refForResolved(arg.ResolvedTypeRef, resolvedType)
	}

	if genericType, ref := traceGenericOrigin(node, originalType); genericType != "" {
		return genericType, ref
	}

	// Delegate to shared logic (checkFuncLit=true to match original Request behavior)
	return sharedResolveTypeOrigin(arg, node, originalType, r.contextProvider, true)
}

func traceGenericOrigin(node TrackerNodeInterface, originalType string) (string, *metadata.TypeRef) {
	typeParams := node.GetTypeParamMap()

	// The bare type name (a type parameter like T resolves through the call site's
	// type-param map), obtained from the tree. Unwrap any
	// pointer/slice/array so a "*pkg.T" still keys on "T".
	leaf := metadata.ParseTypeRef(originalType).NamedLeaf()
	if len(typeParams) > 0 && leaf != nil && leaf.Name != "" {
		searchType := leaf.Name
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
			return searchType, metadata.ParseTypeRef(searchType)
		}
	}
	return "", nil
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
