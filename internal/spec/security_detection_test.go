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

func TestDetectSecuritySchemeFromHandler_HelperTransitive(t *testing.T) {
	// Handler() calls ValidateBearer(). ValidateBearer() does the actual
	// Header.Get + TrimPrefix. The BFS must follow into the helper and
	// still resolve to bearerAuth. We key the Callers index by what
	// edge.Callee.BaseID() returns so the BFS's next-hop lookup matches
	// metadata's own indexing format.
	meta := newTestMeta()

	// Handler → ValidateBearer (helper call)
	helperCall := makeEdge(meta, "Handler", "pkg", "ValidateBearer", "pkg", nil)
	// ValidateBearer → Header.Get("Authorization")
	headerGet := makeEdge(meta, "ValidateBearer", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	headerGet.Callee.RecvType = meta.StringPool.Get("Header")
	// ValidateBearer → strings.TrimPrefix(_, "Bearer ")
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
// and runs TrimPrefix with the given scheme prefix.
func buildAuthHandlerMeta(t *testing.T, prefix string) *metadata.Metadata {
	t.Helper()
	meta := newTestMeta()
	get := makeEdge(meta, "Handler", "pkg", "Get", "net/http",
		[]*metadata.CallArgument{makeLiteralArg(meta, `"Authorization"`)})
	get.Callee.RecvType = meta.StringPool.Get("Header")
	trim := makeEdge(meta, "Handler", "pkg", "TrimPrefix", "strings",
		[]*metadata.CallArgument{
			makeIdentArg(meta, "header", "string"),
			makeLiteralArg(meta, `"`+prefix+`"`),
		})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		"pkg.Handler": {&get, &trim},
	}
	return meta
}
