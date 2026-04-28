// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"strconv"
	"strings"

	"github.com/antst/go-apispec/internal/metadata"
)

// paramConverter describes the OpenAPI schema produced by a known converter
// applied to the result of a parameter-extraction call (e.g. r.FormValue).
type paramConverter struct {
	Type   string
	Format string
}

// paramConverters maps "<callee-pkg>.<callee-name>" to the schema it implies
// when applied to a parameter value. Lookup keys are the same form as
// edge.Callee.Pkg + "." + edge.Callee.Name. Values are intentionally minimal
// so the map stays the single source of truth — extending it covers any new
// converter we want to recognise.
var paramConverters = map[string]paramConverter{
	"strconv.Atoi":                      {Type: "integer"},
	"strconv.ParseInt":                  {Type: "integer"},
	"strconv.ParseUint":                 {Type: "integer"},
	"strconv.ParseBool":                 {Type: "boolean"},
	"strconv.ParseFloat":                {Type: "number"},
	"github.com/google/uuid.Parse":      {Type: "string", Format: "uuid"},
	"github.com/google/uuid.MustParse":  {Type: "string", Format: "uuid"},
	"github.com/google/uuid.FromString": {Type: "string", Format: "uuid"},
}

// lookupParamConverter returns the converter schema for a callee identified
// by its package path and name, or nil when no converter matches.
func lookupParamConverter(pkg, name string) *paramConverter {
	if name == "" {
		return nil
	}
	if pkg != "" {
		if c, ok := paramConverters[pkg+"."+name]; ok {
			return &c
		}
	}
	return nil
}

// inferParamConverterSchema returns a Schema inferred from a known converter
// applied to the parameter value, or nil when no inference applies.
//
// Two consumption patterns are recognised:
//
//  1. Inline — the parameter call is the direct argument of the converter
//     (e.g. strconv.Atoi(r.FormValue("x"))). The tracker tree links the
//     parameter node as an arg-child of the converter node, so the converter
//     edge sits one level up.
//
//  2. Var-bound — the parameter result is bound to a local variable and
//     consumed later (e.g. v := r.FormValue("x"); strconv.ParseBool(v)).
//     The tracker tree gives no direct link, so we scan caller-side edges
//     for the next consumer of that variable in source order.
func inferParamConverterSchema(node TrackerNodeInterface, route *RouteInfo) *Schema {
	if node == nil || route == nil || route.Metadata == nil {
		return nil
	}
	edge := node.GetEdge()
	if edge == nil {
		return nil
	}

	if c := inlineConverter(node, route.Metadata); c != nil {
		return schemaFromConverter(c)
	}
	if c := varBoundConverter(edge, route.Metadata); c != nil {
		return schemaFromConverter(c)
	}
	return nil
}

// inlineConverter detects the "converter(call(...))" idiom by inspecting the
// parameter node's tracker parent.
func inlineConverter(node TrackerNodeInterface, meta *metadata.Metadata) *paramConverter {
	parent := node.GetParent()
	if parent == nil {
		return nil
	}
	parentEdge := parent.GetEdge()
	if parentEdge == nil {
		return nil
	}
	pkg := stringFromPool(meta, parentEdge.Callee.Pkg)
	name := stringFromPool(meta, parentEdge.Callee.Name)
	return lookupParamConverter(pkg, name)
}

// varBoundConverter detects the "v := call(...); converter(v)" idiom by
// scanning sibling caller-side edges for the first consumer of the bound
// variable that matches a known converter.
func varBoundConverter(edge *metadata.CallGraphEdge, meta *metadata.Metadata) *paramConverter {
	if edge.CalleeRecvVarName == "" {
		return nil
	}
	siblings := meta.Callers[edge.Caller.BaseID()]
	if len(siblings) == 0 {
		return nil
	}

	parentLine := positionLine(stringFromPool(meta, edge.Position))
	var (
		best        *paramConverter
		bestLine    int             // 0 == no best yet
		bestUnknown *paramConverter // fallback when every match has unknown position
	)

	for _, sib := range siblings {
		if sib == edge {
			continue
		}
		if !edgeArgUsesVariable(sib, edge.CalleeRecvVarName) {
			continue
		}
		c := lookupParamConverter(stringFromPool(meta, sib.Callee.Pkg), stringFromPool(meta, sib.Callee.Name))
		if c == nil {
			continue
		}
		sibLine := positionLine(stringFromPool(meta, sib.Position))
		// Prefer the closest consumer at or after the parameter call to keep
		// shadowed variables (`v` reused in different if-init scopes) bound to
		// the right converter.
		if parentLine > 0 && sibLine > 0 && sibLine < parentLine {
			continue
		}
		if sibLine <= 0 {
			// No position info — keep as a last-resort fallback so a single
			// unknown-line sibling doesn't block a later sibling with a valid
			// line from being selected.
			if bestUnknown == nil {
				bestUnknown = c
			}
			continue
		}
		if best == nil || sibLine < bestLine {
			best = c
			bestLine = sibLine
		}
	}
	if best != nil {
		return best
	}
	return bestUnknown
}

// edgeArgUsesVariable reports whether any direct argument of the edge is an
// identifier referring to the named variable.
func edgeArgUsesVariable(edge *metadata.CallGraphEdge, varName string) bool {
	for _, arg := range edge.Args {
		if arg == nil {
			continue
		}
		if arg.GetKind() == metadata.KindIdent && arg.GetName() == varName {
			return true
		}
	}
	return false
}

// positionLine extracts the line number from a "file:line:col" position
// string. Returns 0 when the string isn't in that form.
func positionLine(pos string) int {
	if pos == "" {
		return 0
	}
	// Position format: "file:line:col" — split from the right so file paths
	// containing ':' don't trip up the lookup.
	parts := strings.Split(pos, ":")
	if len(parts) < 2 {
		return 0
	}
	line, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0
	}
	return line
}

// stringFromPool resolves a StringPool index, returning "" when meta or the
// pool is nil.
func stringFromPool(m *metadata.Metadata, idx int) string {
	if m == nil || m.StringPool == nil {
		return ""
	}
	return m.StringPool.GetString(idx)
}

func schemaFromConverter(c *paramConverter) *Schema {
	return &Schema{Type: c.Type, Format: c.Format}
}
