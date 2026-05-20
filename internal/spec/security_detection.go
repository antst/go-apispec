// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"strings"

	"github.com/antst/go-apispec/internal/metadata"
)

// authHeaderName is the canonical Authorization header label. Comparison is
// case-insensitive so handlers using non-canonical casing (e.g.
// "authorization") still match.
const authHeaderName = "Authorization"

// detectSecuritySchemeFromHandler scans the call graph reachable from the
// route's handler function and returns the OpenAPI security scheme that the
// handler enforces, or nil when no auth check is recognised. Recognised
// idioms:
//
//   - r.Header.Get("Authorization") — direct header read
//   - strings.TrimPrefix(<header value>, "<scheme> ") — extracts the scheme
//     and the bearer/basic/etc. token
//
// Both inline (the patterns appear in the handler body) and helper-wrapped
// (the patterns appear in a function called by the handler — transitively)
// shapes resolve, because we BFS through meta.Callers from the handler's
// own ID. First scheme found wins — handlers rarely mix bearer and basic
// on the same endpoint, and first-match keeps the result deterministic.
//
// Uses the call-graph index rather than the tracker tree because route
// nodes in the tree don't always expose handler-body edges as children
// (the tree is route-registration-shaped); meta.Callers is direct.
func detectSecuritySchemeFromHandler(route *RouteInfo) *DetectedSecurityScheme {
	if route == nil || route.Metadata == nil || route.Function == "" {
		return nil
	}

	prefix, found := walkCallersForAuthPrefix(route.Function, route.Metadata)
	if !found {
		return nil
	}
	return schemeFromAuthPrefix(prefix)
}

// walkCallersForAuthPrefix BFSes from the given function through meta.Callers
// (edges keyed by caller-function base ID). For each edge, it checks the
// callee for an Authorization-header read or a TrimPrefix call. Helper
// functions are followed transitively up to a hard depth cap to keep
// runtime bounded on pathological graphs.
//
// Returns (prefix, true) when at least one Authorization-header read was
// seen. The prefix is "" when no TrimPrefix-style scheme was paired with
// the read (apiKey-style raw token).
func walkCallersForAuthPrefix(handlerID string, meta *metadata.Metadata) (string, bool) {
	const maxDepth = 8

	type queued struct {
		id    string
		depth int
	}
	visited := map[string]bool{handlerID: true}
	queue := []queued{{handlerID, 0}}

	hasHeaderRead := false
	var prefix string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, edge := range edgesFromCaller(meta, cur.id) {
			callee := stringFromPool(meta, edge.Callee.Name)
			pkg := stringFromPool(meta, edge.Callee.Pkg)
			recv := stringFromPool(meta, edge.Callee.RecvType)

			switch {
			case isAuthHeaderRead(callee, recv, edge, meta):
				hasHeaderRead = true
			case isTrimPrefixCall(callee, pkg):
				if p := trimPrefixValue(edge, meta); p != "" && prefix == "" {
					prefix = p
				}
			}

			if cur.depth+1 >= maxDepth {
				continue
			}
			// Descend into the callee — but only when it's a user-defined
			// function we can re-look-up in meta.Callers. Builtins and
			// stdlib calls (json, strings, http stdlib helpers) don't have
			// further edges relevant to auth detection.
			calleeID := edge.Callee.BaseID()
			if visited[calleeID] {
				continue
			}
			if _, ok := meta.Callers[calleeID]; !ok {
				continue
			}
			visited[calleeID] = true
			queue = append(queue, queued{calleeID, cur.depth + 1})
		}
	}

	return prefix, hasHeaderRead
}

// edgesFromCaller returns the edges originating from the named function
// using the per-caller index. Tries the exact ID first, then falls back
// to a name-only suffix match — route.Function carries a user-facing form
// (e.g. "pkg.Handler.Method") that doesn't always equal the metadata's
// base ID format.
func edgesFromCaller(meta *metadata.Metadata, id string) []*metadata.CallGraphEdge {
	if edges, ok := meta.Callers[id]; ok {
		return edges
	}
	// Fallback: route.Function may use "-->" or differ in formatting from
	// the metadata's caller IDs. Try matching by the trailing function-name
	// segment, which is stable across formats.
	short := id
	if idx := strings.LastIndex(short, "."); idx >= 0 {
		short = short[idx+1:]
	}
	for key, edges := range meta.Callers {
		if strings.HasSuffix(key, "."+short) || key == short {
			return edges
		}
	}
	return nil
}

// isAuthHeaderRead matches `r.Header.Get("Authorization")` — the canonical
// stdlib idiom. Recv-type check accepts both `http.Header` and the
// fully-qualified `net/http.Header` so framework configs that rewrite
// receivers still resolve.
func isAuthHeaderRead(callee, recv string, edge *metadata.CallGraphEdge, meta *metadata.Metadata) bool {
	if callee != "Get" {
		return false
	}
	if !strings.Contains(recv, "Header") {
		return false
	}
	if len(edge.Args) == 0 {
		return false
	}
	name := strings.Trim(stringFromPool(meta, edge.Args[0].Value), `"`)
	return strings.EqualFold(name, authHeaderName)
}

// isTrimPrefixCall matches `strings.TrimPrefix(...)` regardless of import
// alias — the package path is what counts.
func isTrimPrefixCall(callee, pkg string) bool {
	return callee == "TrimPrefix" && pkg == "strings"
}

// trimPrefixValue extracts the literal prefix argument (second positional
// argument) from a strings.TrimPrefix call. Returns "" when the prefix is
// a non-literal (e.g. a variable) — we don't try to evaluate dynamic
// expressions; the apiKey fallback covers that case.
func trimPrefixValue(edge *metadata.CallGraphEdge, meta *metadata.Metadata) string {
	if len(edge.Args) < 2 {
		return ""
	}
	arg := edge.Args[1]
	if arg.GetKind() != metadata.KindLiteral {
		return ""
	}
	return strings.Trim(stringFromPool(meta, arg.Value), `"`)
}

// schemeFromAuthPrefix maps a trimmed prefix string to an OpenAPI security
// scheme. Recognised prefixes:
//
//	"Bearer "  → http / bearer
//	"Basic "   → http / basic
//	"<other>"  → http with the lowercased scheme name (best effort for
//	             non-standard auth)
//	""         → apiKey in header (raw token, no scheme prefix)
//
// Single source of truth for prefix-to-scheme mapping; extending it
// covers any new scheme without touching the walker.
func schemeFromAuthPrefix(prefix string) *DetectedSecurityScheme {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return &DetectedSecurityScheme{
			Name: "apiKeyAuth",
			Scheme: SecurityScheme{
				Type: "apiKey",
				In:   "header",
				Name: authHeaderName,
			},
		}
	}
	scheme := strings.ToLower(trimmed)
	name := scheme + "Auth"
	return &DetectedSecurityScheme{
		Name: name,
		Scheme: SecurityScheme{
			Type:   "http",
			Scheme: scheme,
		},
	}
}
