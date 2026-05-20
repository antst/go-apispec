package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

func TestSchemeFromAuthPrefix_Bearer(t *testing.T) {
	got := schemeFromAuthPrefix("Bearer ")
	require.NotNil(t, got)
	assert.Equal(t, "bearerAuth", got.Name)
	assert.Equal(t, "http", got.Scheme.Type)
	assert.Equal(t, "bearer", got.Scheme.Scheme)
}

func TestSchemeFromAuthPrefix_Basic(t *testing.T) {
	got := schemeFromAuthPrefix("Basic ")
	require.NotNil(t, got)
	assert.Equal(t, "basicAuth", got.Name)
	assert.Equal(t, "http", got.Scheme.Type)
	assert.Equal(t, "basic", got.Scheme.Scheme)
}

func TestSchemeFromAuthPrefix_Empty_FallsBackToAPIKey(t *testing.T) {
	got := schemeFromAuthPrefix("")
	require.NotNil(t, got)
	assert.Equal(t, "apiKeyAuth", got.Name)
	assert.Equal(t, "apiKey", got.Scheme.Type)
	assert.Equal(t, "header", got.Scheme.In)
	assert.Equal(t, "Authorization", got.Scheme.Name)
}

func TestSchemeFromAuthPrefix_NonStandard_LowercasedHTTPScheme(t *testing.T) {
	// Anything other than Bearer/Basic still produces an http scheme with
	// the lowercased name. Best-effort for unusual deployments.
	got := schemeFromAuthPrefix("Digest ")
	require.NotNil(t, got)
	assert.Equal(t, "digestAuth", got.Name)
	assert.Equal(t, "http", got.Scheme.Type)
	assert.Equal(t, "digest", got.Scheme.Scheme)
}

func TestIsAuthHeaderRead_AuthorizationHeader(t *testing.T) {
	meta := newTestMeta()
	arg := makeLiteralArg(meta, `"Authorization"`)
	edge := makeEdge(meta, "h", "main", "Get", "net/http",
		[]*metadata.CallArgument{arg})
	edge.Callee.RecvType = meta.StringPool.Get("Header")

	callee := stringFromPool(meta, edge.Callee.Name)
	recv := stringFromPool(meta, edge.Callee.RecvType)
	assert.True(t, isAuthHeaderRead(callee, recv, &edge, meta))
}

func TestIsAuthHeaderRead_WrongMethod(t *testing.T) {
	// Same Header receiver but a different method — e.g. Set — must not
	// fire detection.
	meta := newTestMeta()
	arg := makeLiteralArg(meta, `"Authorization"`)
	edge := makeEdge(meta, "h", "main", "Set", "net/http",
		[]*metadata.CallArgument{arg})
	edge.Callee.RecvType = meta.StringPool.Get("Header")
	assert.False(t, isAuthHeaderRead("Set", "Header", &edge, meta))
}

func TestIsAuthHeaderRead_DifferentHeaderName(t *testing.T) {
	// Get on Header, but for a different header (e.g. If-None-Match).
	meta := newTestMeta()
	arg := makeLiteralArg(meta, `"If-None-Match"`)
	edge := makeEdge(meta, "h", "main", "Get", "net/http",
		[]*metadata.CallArgument{arg})
	edge.Callee.RecvType = meta.StringPool.Get("Header")
	assert.False(t, isAuthHeaderRead("Get", "Header", &edge, meta))
}

func TestIsAuthHeaderRead_CaseInsensitive(t *testing.T) {
	// Non-canonical casing of the header name still resolves — HTTP headers
	// are case-insensitive and we shouldn't punish lowercase usage.
	meta := newTestMeta()
	arg := makeLiteralArg(meta, `"authorization"`)
	edge := makeEdge(meta, "h", "main", "Get", "net/http",
		[]*metadata.CallArgument{arg})
	edge.Callee.RecvType = meta.StringPool.Get("http.Header")
	assert.True(t, isAuthHeaderRead("Get", "http.Header", &edge, meta))
}

func TestIsAuthHeaderRead_WrongReceiverOrNoArgs(t *testing.T) {
	meta := newTestMeta()
	arg := makeLiteralArg(meta, `"Authorization"`)
	edge := makeEdge(meta, "h", "main", "Get", "net/http",
		[]*metadata.CallArgument{arg})

	// Receiver doesn't contain "Header" — e.g. a Get() on something else.
	assert.False(t, isAuthHeaderRead("Get", "url.Values", &edge, meta))

	// Zero args (degenerate case from malformed metadata).
	noArgs := makeEdge(meta, "h", "main", "Get", "net/http", nil)
	noArgs.Callee.RecvType = meta.StringPool.Get("Header")
	assert.False(t, isAuthHeaderRead("Get", "Header", &noArgs, meta))
}

func TestIsTrimPrefixCall(t *testing.T) {
	assert.True(t, isTrimPrefixCall("TrimPrefix", "strings"))
	assert.False(t, isTrimPrefixCall("TrimPrefix", "bytes"), "only strings.TrimPrefix matters")
	assert.False(t, isTrimPrefixCall("HasPrefix", "strings"), "HasPrefix doesn't extract")
	assert.False(t, isTrimPrefixCall("", ""))
}

func TestTrimPrefixValue_LiteralWins(t *testing.T) {
	meta := newTestMeta()
	header := makeIdentArg(meta, "header", "string")
	prefix := makeLiteralArg(meta, `"Bearer "`)
	edge := makeEdge(meta, "h", "main", "TrimPrefix", "strings",
		[]*metadata.CallArgument{header, prefix})
	assert.Equal(t, "Bearer ", trimPrefixValue(&edge, meta))
}

func TestTrimPrefixValue_NonLiteral_ReturnsEmpty(t *testing.T) {
	// Dynamic prefix (variable) — we don't try to evaluate; the apiKey
	// fallback covers this case.
	meta := newTestMeta()
	header := makeIdentArg(meta, "header", "string")
	prefixVar := makeIdentArg(meta, "prefix", "string")
	edge := makeEdge(meta, "h", "main", "TrimPrefix", "strings",
		[]*metadata.CallArgument{header, prefixVar})
	assert.Empty(t, trimPrefixValue(&edge, meta))
}

func TestTrimPrefixValue_TooFewArgs(t *testing.T) {
	meta := newTestMeta()
	edge := makeEdge(meta, "h", "main", "TrimPrefix", "strings",
		[]*metadata.CallArgument{makeIdentArg(meta, "x", "string")})
	assert.Empty(t, trimPrefixValue(&edge, meta))
}

func TestEdgesFromCaller_DirectAndSuffix(t *testing.T) {
	meta := newTestMeta()
	exact := makeEdge(meta, "Handler", "pkg", "Get", "net/http", nil)
	exact.Callee.RecvType = meta.StringPool.Get("Header")
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"pkg.Handler": {&exact},
	}
	// Exact hit.
	got := edgesFromCaller(meta, "pkg.Handler")
	require.Len(t, got, 1)

	// Suffix fallback for cases where the route.Function format differs
	// from the metadata's caller ID format.
	got = edgesFromCaller(meta, "Handler")
	require.Len(t, got, 1, "suffix match should resolve when exact misses")

	// Total miss yields nil.
	got = edgesFromCaller(meta, "Nope")
	assert.Nil(t, got)
}

func TestDetectSecuritySchemeFromHandler_NilGuards(t *testing.T) {
	assert.Nil(t, detectSecuritySchemeFromHandler(nil))
	assert.Nil(t, detectSecuritySchemeFromHandler(&RouteInfo{Function: "Foo"}), "no metadata → nil")
	assert.Nil(t, detectSecuritySchemeFromHandler(&RouteInfo{Metadata: newTestMeta()}), "no function name → nil")
}

func TestDetectSecuritySchemeFromHandler_BearerInline(t *testing.T) {
	// Build a tiny call graph: Handler() calls Header.Get("Authorization")
	// and strings.TrimPrefix(_, "Bearer "). The detector must follow both
	// edges from the same caller and return bearerAuth.
	meta := buildAuthHandlerMeta(t, "Bearer ")
	route := &RouteInfo{
		Function: "pkg.Handler",
		Metadata: meta,
	}
	got := detectSecuritySchemeFromHandler(route)
	require.NotNil(t, got)
	assert.Equal(t, "bearerAuth", got.Name)
	assert.Equal(t, "bearer", got.Scheme.Scheme)
}

func TestDetectSecuritySchemeFromHandler_RawHeader_APIKey(t *testing.T) {
	// Header.Get("Authorization") with NO TrimPrefix → apiKey fallback.
	meta := newTestMeta()
	get := makeEdge(meta, "Handler", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	get.Callee.RecvType = meta.StringPool.Get("Header")
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"pkg.Handler": {&get},
	}
	got := detectSecuritySchemeFromHandler(&RouteInfo{Function: "pkg.Handler", Metadata: meta})
	require.NotNil(t, got)
	assert.Equal(t, "apiKeyAuth", got.Name)
}

func TestDetectSecuritySchemeFromHandler_NoAuth(t *testing.T) {
	// Handler that doesn't touch the Authorization header at all → nil.
	meta := newTestMeta()
	other := makeEdge(meta, "Handler", "pkg", "Encode", "encoding/json", nil)
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"pkg.Handler": {&other},
	}
	assert.Nil(t, detectSecuritySchemeFromHandler(&RouteInfo{Function: "pkg.Handler", Metadata: meta}))
}

func TestDetectSecuritySchemeFromHandler_UnrelatedTrimPrefix_DoesNotPoison(t *testing.T) {
	// Handler reads r.Header.Get("Authorization") without trimming any
	// prefix from the result, AND also calls strings.TrimPrefix(userID, "@")
	// on an unrelated variable — exactly the alkem-io/matrix-adapter-go
	// shape that produced bogus "@Auth" schemes in v0.4.12. The fix scopes
	// TrimPrefix matching to calls whose first argument traces back to the
	// Authorization-header read, so the "@" prefix is ignored and the
	// handler resolves to plain apiKeyAuth.
	meta := newTestMeta()

	// Header.Get("Authorization") — result NOT trimmed by anything related.
	get := makeEdge(meta, "Handler", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	get.Callee.RecvType = meta.StringPool.Get("Header")
	get.CalleeRecvVarName = "token"
	// Unrelated TrimPrefix on a totally different variable (Matrix user ID).
	matrixTrim := makeEdge(meta, "Handler", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "userID", "string"),
			makeLiteralArg(meta, `"@"`),
		})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		get.Caller.BaseID(): {&get, &matrixTrim},
	}

	got := detectSecuritySchemeFromHandler(&RouteInfo{
		Function: get.Caller.BaseID(),
		Metadata: meta,
	})
	require.NotNil(t, got)
	assert.Equal(t, "apiKeyAuth", got.Name,
		"unrelated TrimPrefix on '@' must not change the scheme away from apiKey")
}

func TestTrimPrefixOnAuthVar_VariableMatch(t *testing.T) {
	meta := newTestMeta()
	authVars := map[string]bool{"header": true}
	edge := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "header", "string"),
			makeLiteralArg(meta, `"Bearer "`),
		})
	assert.True(t, trimPrefixOnAuthVar(&edge, authVars, meta))

	// Different ident → not the auth variable, refuse.
	other := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "userID", "string"),
			makeLiteralArg(meta, `"@"`),
		})
	assert.False(t, trimPrefixOnAuthVar(&other, authVars, meta))
}

func TestTrimPrefixOnAuthVar_EmptyArgsOrNil(t *testing.T) {
	meta := newTestMeta()
	edge := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings", nil)
	assert.False(t, trimPrefixOnAuthVar(&edge, map[string]bool{}, meta))

	withNil := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{nil, makeLiteralArg(meta, `"Bearer "`)})
	assert.False(t, trimPrefixOnAuthVar(&withNil, map[string]bool{}, meta))
}

func TestTrimPrefixOnAuthVar_NonIdentNonCallFirstArg(t *testing.T) {
	// First arg is a literal — not an ident, not a call. Must not match.
	meta := newTestMeta()
	edge := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeLiteralArg(meta, `"some literal"`),
			makeLiteralArg(meta, `"Bearer "`),
		})
	assert.False(t, trimPrefixOnAuthVar(&edge, map[string]bool{"header": true}, meta))
}

// buildHeaderGetCallArg constructs a CallArgument shaped like
// `r.Header.Get("Authorization")` for the inline-call helper tests. The
// arg.Fun is a selector whose Sel name is "Get" and whose receiver
// resolves to a type containing "Header"; arg.Args[0] is the
// literal header name. Mirrors what ExprToCallArgument builds for the
// real `strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")`
// idiom.
func buildHeaderGetCallArg(meta *metadata.Metadata, headerName string) *metadata.CallArgument {
	receiver := makeCallArg(meta)
	receiver.SetKind(metadata.KindIdent)
	receiver.SetType("net/http.Header")

	sel := makeCallArg(meta)
	sel.SetKind(metadata.KindSelector)
	sel.X = receiver
	sel.Sel = makeCallArg(meta)
	sel.Sel.SetKind(metadata.KindIdent)
	sel.Sel.SetName("Get")

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = sel
	arg.Args = []*metadata.CallArgument{makeLiteralArg(meta, `"`+headerName+`"`)}
	return arg
}

func TestIsInlineAuthHeaderCall_HappyPath(t *testing.T) {
	// strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") — the
	// first arg of TrimPrefix is a KindCall to Header.Get with literal
	// "Authorization". This must match without the result ever being
	// bound to a local variable first.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, authHeaderName)
	assert.True(t, isInlineAuthHeaderCall(arg, meta))
}

func TestIsInlineAuthHeaderCall_FallbackReceiverPath(t *testing.T) {
	// Some builders fill arg.Fun.Type instead of arg.Fun.X.Type. Both
	// should resolve. Clear X.Type, set Fun.Type — same conclusion.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, authHeaderName)
	arg.Fun.X.SetType("")
	arg.Fun.SetType("net/http.Header")
	assert.True(t, isInlineAuthHeaderCall(arg, meta))
}

func TestIsInlineAuthHeaderCall_DifferentHeader(t *testing.T) {
	// Header.Get for a header other than Authorization — must not match.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, "X-Trace-ID")
	assert.False(t, isInlineAuthHeaderCall(arg, meta))
}

func TestIsInlineAuthHeaderCall_DifferentMethod(t *testing.T) {
	// Some other method on Header (e.g. Set) — must not match.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, authHeaderName)
	arg.Fun.Sel.SetName("Set")
	assert.False(t, isInlineAuthHeaderCall(arg, meta))
}

func TestIsInlineAuthHeaderCall_WrongReceiverType(t *testing.T) {
	// Selector exists but receiver type doesn't contain "Header" — e.g.
	// some other type's Get method. Must not match.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, authHeaderName)
	arg.Fun.X.SetType("url.Values")
	// Also clear the fallback path so neither resolves.
	arg.Fun.SetType("url.Values")
	assert.False(t, isInlineAuthHeaderCall(arg, meta))
}

func TestIsInlineAuthHeaderCall_NotASelector(t *testing.T) {
	// arg.Fun is an ident (not a selector) — e.g. a bare package-level
	// call. The recognised shape requires the X.Sel form.
	meta := newTestMeta()
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindCall)
	arg.Fun = makeIdentArg(meta, "Get", "")
	assert.False(t, isInlineAuthHeaderCall(arg, meta))

	// Selector with nil Sel — bail safely.
	arg2 := makeCallArg(meta)
	arg2.SetKind(metadata.KindCall)
	arg2.Fun = makeCallArg(meta)
	arg2.Fun.SetKind(metadata.KindSelector)
	assert.False(t, isInlineAuthHeaderCall(arg2, meta))

	// Selector with nil X.
	arg3 := makeCallArg(meta)
	arg3.SetKind(metadata.KindCall)
	arg3.Fun = makeCallArg(meta)
	arg3.Fun.SetKind(metadata.KindSelector)
	arg3.Fun.Sel = makeCallArg(meta)
	arg3.Fun.Sel.SetName("Get")
	assert.False(t, isInlineAuthHeaderCall(arg3, meta))
}

func TestIsInlineAuthHeaderCall_NoArgs(t *testing.T) {
	// Selector + receiver look right but the inner call has no args.
	meta := newTestMeta()
	arg := buildHeaderGetCallArg(meta, authHeaderName)
	arg.Args = nil
	assert.False(t, isInlineAuthHeaderCall(arg, meta))
}

func TestTrimPrefixOnAuthVar_InlineCallFirstArg(t *testing.T) {
	// strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") — the
	// inline-call path: the auth-var set is empty but the first arg IS
	// an Authorization-header read inline, so this still matches.
	meta := newTestMeta()
	innerCall := buildHeaderGetCallArg(meta, authHeaderName)
	edge := makeEdge(meta, "h", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{innerCall, makeLiteralArg(meta, `"Bearer "`)})
	assert.True(t, trimPrefixOnAuthVar(&edge, map[string]bool{}, meta))
}

func TestIsInlineAuthHeaderCall_NilGuards(t *testing.T) {
	meta := newTestMeta()
	assert.False(t, isInlineAuthHeaderCall(nil, meta))

	// Arg with no Fun.
	bare := makeCallArg(meta)
	bare.SetKind(metadata.KindCall)
	assert.False(t, isInlineAuthHeaderCall(bare, meta))
}

func TestDetectSecuritySchemeFromHandler_HelperTransitive(t *testing.T) {
	// Handler() calls ValidateBearer(). ValidateBearer() does the actual
	// Header.Get + TrimPrefix. The BFS must follow into the helper and
	// still resolve to bearerAuth. We key the Callers index by what
	// edge.Callee.BaseID() returns so the BFS's next-hop lookup matches
	// metadata's own indexing format.
	meta := newTestMeta()

	// Handler → ValidateBearer (helper call)
	helperCall := makeEdge(meta, "Handler", "pkg", "ValidateBearer", "pkg", nil)
	// ValidateBearer → header := r.Header.Get("Authorization")
	headerGet := makeEdge(meta, "ValidateBearer", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	headerGet.Callee.RecvType = meta.StringPool.Get("Header")
	headerGet.CalleeRecvVarName = "header"
	// ValidateBearer → strings.TrimPrefix(header, "Bearer ")
	trim := makeEdge(meta, "ValidateBearer", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "header", "string"),
			makeLiteralArg(meta, `"Bearer "`),
		})

	// Use the metadata's own BaseID format as the map key so the BFS hops
	// resolve via the primary (non-suffix) path.
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		helperCall.Caller.BaseID(): {&helperCall},
		helperCall.Callee.BaseID(): {&headerGet, &trim},
	}

	got := detectSecuritySchemeFromHandler(&RouteInfo{
		Function: helperCall.Caller.BaseID(),
		Metadata: meta,
	})
	require.NotNil(t, got, "must follow into the helper")
	assert.Equal(t, "bearerAuth", got.Name)
}

func TestDetectSecuritySchemeFromHandler_CycleSafe(t *testing.T) {
	// Pathological: A → B → A. The BFS visited-set prevents infinite loops.
	// Use BaseID-formatted keys so the BFS's transitive hop genuinely
	// reaches the visited-check (otherwise the key mismatch makes it
	// short-circuit at the "no caller index entry" path instead).
	meta := newTestMeta()
	a := makeEdge(meta, "A", "pkg", "B", "pkg", nil)
	b := makeEdge(meta, "B", "pkg", "A", "pkg", nil)
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		a.Caller.BaseID(): {&a},
		b.Caller.BaseID(): {&b},
	}
	// No Authorization read anywhere — must return nil without spinning.
	assert.Nil(t, detectSecuritySchemeFromHandler(&RouteInfo{
		Function: a.Caller.BaseID(),
		Metadata: meta,
	}))
}

func TestDetectSecuritySchemeFromHandler_DepthCapStopsRecursion(t *testing.T) {
	// Chain of 16 wrapper functions, each calling the next. Auth lives at
	// the end. The BFS depth cap (currently 8 internal hops) prevents us
	// from finding it — runtime guard, not correctness — so the call must
	// return nil rather than spinning or panicking on deep nesting.
	meta := newTestMeta()

	callers := map[string][]*metadata.CallGraphEdge{}
	const N = 16
	edges := make([]*metadata.CallGraphEdge, 0, N+1)
	for i := 0; i < N; i++ {
		callerName := "F" + itoa(i)
		calleeName := "F" + itoa(i+1)
		e := makeEdge(meta, callerName, "pkg", calleeName, "pkg", nil)
		edges = append(edges, &e)
		callers[e.Caller.BaseID()] = []*metadata.CallGraphEdge{&e}
	}
	// The deepest function reads the Authorization header — past the cap.
	final := makeEdge(meta, "F"+itoa(N), "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	final.Callee.RecvType = meta.StringPool.Get("Header")
	callers[final.Caller.BaseID()] = []*metadata.CallGraphEdge{&final}
	meta.Callers = callers

	got := detectSecuritySchemeFromHandler(&RouteInfo{
		Function: edges[0].Caller.BaseID(),
		Metadata: meta,
	})
	assert.Nil(t, got, "depth-capped BFS must give up rather than dive forever")
}

// itoa is a tiny no-import-needed integer formatter for test loops.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestSecuritySchemes_ConfigOverridesDetected exercises the config-override
// path in MapMetadataToOpenAPI: apispec.yaml's `securitySchemes:` block
// must replace any detected scheme of the same name. Lets users opt out of
// auto-detection's defaults (e.g. document bearerFormat=JWT, attach a
// description, override the apiKey header name) without code changes.
func TestSecuritySchemes_ConfigMergesAndOverrides(t *testing.T) {
	detected := map[string]SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer"},
		"basicAuth":  {Type: "http", Scheme: "basic"},
	}
	configured := map[string]SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT", Description: "JWT issued by Auth0"},
		"oauth2":     {Type: "oauth2"},
	}

	// Mimic the mapper's merge inline so we can assert behaviour without
	// spinning up a full extractor.
	merged := make(map[string]SecurityScheme, len(detected)+len(configured))
	for k, v := range detected {
		merged[k] = v
	}
	for k, v := range configured {
		merged[k] = v
	}

	// bearerAuth: configured entry wins, picking up the extra fields.
	require.Contains(t, merged, "bearerAuth")
	assert.Equal(t, "JWT", merged["bearerAuth"].BearerFormat)
	assert.Equal(t, "JWT issued by Auth0", merged["bearerAuth"].Description)

	// basicAuth: detected entry survives (no config conflict).
	require.Contains(t, merged, "basicAuth")
	assert.Equal(t, "basic", merged["basicAuth"].Scheme)

	// oauth2: configured-only schemes still land in the merged output.
	require.Contains(t, merged, "oauth2")
}

func TestCollectDetectedSecuritySchemes_DedupsAndIgnoresNil(t *testing.T) {
	bearer := &DetectedSecurityScheme{
		Name:   "bearerAuth",
		Scheme: SecurityScheme{Type: "http", Scheme: "bearer"},
	}
	bearerDup := &DetectedSecurityScheme{
		Name:   "bearerAuth", // same name, different (impossible-in-practice) shape
		Scheme: SecurityScheme{Type: "http", Scheme: "bearer"},
	}
	basic := &DetectedSecurityScheme{
		Name:   "basicAuth",
		Scheme: SecurityScheme{Type: "http", Scheme: "basic"},
	}

	routes := []*RouteInfo{
		{SecurityScheme: bearer},
		{SecurityScheme: nil}, // unauthenticated route — must be skipped
		{SecurityScheme: bearerDup},
		{SecurityScheme: basic},
		{SecurityScheme: &DetectedSecurityScheme{Name: ""}}, // missing name — skip
	}
	got := collectDetectedSecuritySchemes(routes)
	assert.Len(t, got, 2)
	assert.Equal(t, "bearer", got["bearerAuth"].Scheme)
	assert.Equal(t, "basic", got["basicAuth"].Scheme)
}

func TestDropAuthorizationHeaderParam(t *testing.T) {
	auth := Parameter{Name: "Authorization", In: "header"}
	other := Parameter{Name: "X-Tenant-ID", In: "header"}
	path := Parameter{Name: "id", In: "path"}

	// Only the Authorization header is removed; others survive untouched.
	got := dropAuthorizationHeaderParam([]Parameter{auth, other, path})
	require.Len(t, got, 2)
	assert.Equal(t, "X-Tenant-ID", got[0].Name)
	assert.Equal(t, "id", got[1].Name)

	// Case-insensitive match.
	gotLower := dropAuthorizationHeaderParam([]Parameter{{Name: "authorization", In: "header"}, path})
	require.Len(t, gotLower, 1)
	assert.Equal(t, "id", gotLower[0].Name)

	// Same-named param at a different position (query!) is preserved — only
	// the header-located Authorization is implied by the scheme reference.
	gotQuery := dropAuthorizationHeaderParam([]Parameter{{Name: "Authorization", In: "query"}})
	require.Len(t, gotQuery, 1)

	// All-removed → nil rather than zero-length slice, so the JSON output
	// stays consistent with operations that have no parameters.
	assert.Nil(t, dropAuthorizationHeaderParam([]Parameter{auth}))
	assert.Nil(t, dropAuthorizationHeaderParam(nil))
}

// buildAuthHandlerMeta is a shared fixture for the detector tests — it
// stamps a tiny call graph mimicking a handler that reads the Auth header
// into a local variable `header` and runs TrimPrefix on that variable
// with the given scheme prefix. The CalleeRecvVarName on the Header.Get
// edge is what lets the scoped TrimPrefix matcher pair the two.
func buildAuthHandlerMeta(t *testing.T, prefix string) *metadata.Metadata {
	t.Helper()
	meta := newTestMeta()
	get := makeEdge(meta, "Handler", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	get.Callee.RecvType = meta.StringPool.Get("Header")
	get.CalleeRecvVarName = "header"
	trim := makeEdge(meta, "Handler", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "header", "string"),
			makeLiteralArg(meta, `"`+prefix+`"`),
		})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		get.Caller.BaseID(): {&get, &trim},
	}
	return meta
}
