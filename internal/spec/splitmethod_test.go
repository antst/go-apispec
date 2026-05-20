package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitMethodPrefixedPath_AllMethods(t *testing.T) {
	// Every RFC 7231 / 5789 / 7540 method must be recognised.
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "TRACE", "CONNECT"} {
		method, path, ok := splitMethodPrefixedPath(m + " /resource")
		if assert.True(t, ok, "method=%s", m) {
			assert.Equal(t, m, method, "method=%s", m)
			assert.Equal(t, "/resource", path, "method=%s", m)
		}
	}
}

func TestSplitMethodPrefixedPath_CaseNormalised(t *testing.T) {
	// Go 1.22+ accepts lower-case method tokens too — the returned method is
	// always upper-cased so downstream code (setOperationOnPathItem) can do
	// a straight match.
	method, path, ok := splitMethodPrefixedPath("get /health")
	assert.True(t, ok)
	assert.Equal(t, "GET", method)
	assert.Equal(t, "/health", path)
}

func TestSplitMethodPrefixedPath_TrimsExtraSpaces(t *testing.T) {
	// Multiple spaces between method and path are tolerated — Go 1.22's
	// pattern parser does the same.
	method, path, ok := splitMethodPrefixedPath("POST    /api/users")
	assert.True(t, ok)
	assert.Equal(t, "POST", method)
	assert.Equal(t, "/api/users", path)
}

func TestSplitMethodPrefixedPath_NoMethod(t *testing.T) {
	// Plain pattern — left alone.
	method, path, ok := splitMethodPrefixedPath("/health/live")
	assert.False(t, ok)
	assert.Empty(t, method)
	assert.Empty(t, path)
}

func TestSplitMethodPrefixedPath_UnknownMethod(t *testing.T) {
	// "FOO /path" looks structurally like the prefix syntax but FOO isn't a
	// valid HTTP method, so the splitter declines to split. Caller keeps the
	// original string as the path.
	_, _, ok := splitMethodPrefixedPath("FOO /path")
	assert.False(t, ok)
}

func TestSplitMethodPrefixedPath_NoPathLeadingSlash(t *testing.T) {
	// "GET something" without a leading slash on the second token isn't the
	// Go 1.22+ syntax — splitter declines, preserving the raw value.
	_, _, ok := splitMethodPrefixedPath("GET something")
	assert.False(t, ok)
}

func TestSplitMethodPrefixedPath_EmptyAndEdge(t *testing.T) {
	cases := []string{
		"",
		" ",
		"GET",       // no space
		" /path",    // empty method
		"GET /",     // valid — single slash
		"GET  /",    // multiple spaces, still valid
		"GET\t/x",   // tab isn't the recognised separator — only spaces
		"POST /a b", // path containing a space — keep as-is
	}
	results := map[string]bool{
		"":          false,
		" ":         false,
		"GET":       false,
		" /path":    false,
		"GET /":     true,
		"GET  /":    true,
		"GET\t/x":   false,
		"POST /a b": true,
	}
	for _, s := range cases {
		_, _, ok := splitMethodPrefixedPath(s)
		assert.Equal(t, results[s], ok, "input=%q", s)
	}
}

func TestIsHTTPMethod(t *testing.T) {
	for _, m := range []string{"GET", "Post", "put", "DELETE", "patch", "Options", "head", "Trace", "Connect"} {
		assert.True(t, isHTTPMethod(m), "method=%s", m)
	}
	for _, m := range []string{"", "FOO", "GETx", "POSTING"} {
		assert.False(t, isHTTPMethod(m), "non-method=%s", m)
	}
}
