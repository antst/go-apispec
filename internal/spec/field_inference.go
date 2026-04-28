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

// applyJSONFieldConverterFormats back-propagates converter-derived OpenAPI
// schema formats onto a request-body struct's fields.
//
// For every edge in the same handler that consumes `<targetVar>.<fieldName>`
// (directly or behind a unary/star), we look up the callee in
// paramConverters and write its type/format onto the matching property of
// the struct schema. The struct's JSON-tag derived property names are
// resolved from the metadata so the inference doesn't depend on field-name
// casing conventions.
//
// Tag-driven overrides (`apispec:"format=..."`) take precedence over flow
// inference: applyAPISpecTag runs after this and overwrites whatever flow
// analysis wrote.
func applyJSONFieldConverterFormats(targetVar, bodyType string, callerBaseID string, route *RouteInfo) {
	if targetVar == "" || bodyType == "" || route == nil || route.Metadata == nil {
		return
	}
	structSchema := route.UsedTypes[bodyType]
	if structSchema == nil || structSchema.Properties == nil {
		return
	}

	// Map from Go field name to (JSON name, schema-property pointer). When the
	// JSON tag is missing, fall back to the Go field name — the schema
	// generator does the same.
	fields := lookupStructFields(bodyType, route.Metadata)
	if len(fields) == 0 {
		return
	}

	siblings := route.Metadata.Callers[callerBaseID]
	for _, sib := range siblings {
		for _, arg := range sib.Args {
			fieldGoName := selectorFieldOnTarget(arg, targetVar)
			if fieldGoName == "" {
				continue
			}
			jsonName, ok := fields[fieldGoName]
			if !ok {
				continue
			}
			prop, ok := structSchema.Properties[jsonName]
			if !ok || prop == nil {
				continue
			}
			c := lookupParamConverter(
				stringFromPool(route.Metadata, sib.Callee.Pkg),
				stringFromPool(route.Metadata, sib.Callee.Name),
			)
			if c == nil {
				continue
			}
			// Don't clobber an explicit format — flow inference is best-effort
			// and a previous edge in this loop, the validator tag pass, or a
			// nested-type pass might have set something more specific.
			if prop.Format == "" {
				prop.Format = c.Format
			}
			if c.Type != "" && (prop.Type == "" || prop.Type == "string") {
				// Converters that change the wire type (Atoi → integer) only
				// apply when the JSON field is currently typed as string —
				// otherwise the explicit struct-field type wins.
				prop.Type = c.Type
			}
		}
	}
}

// selectorFieldOnTarget returns the field name of `<targetVar>.<field>`
// when the argument is exactly that selector (or one wrapped in a single
// unary/star, which covers `*body.field` for pointer fields). Returns ""
// for any other shape so callers fall through.
func selectorFieldOnTarget(arg *metadata.CallArgument, targetVar string) string {
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindUnary, metadata.KindStar:
		if arg.X == nil {
			return ""
		}
		return selectorFieldOnTarget(arg.X, targetVar)
	case metadata.KindSelector:
		if arg.X == nil || arg.Sel == nil {
			return ""
		}
		if arg.X.GetKind() != metadata.KindIdent || arg.X.GetName() != targetVar {
			return ""
		}
		return arg.Sel.GetName()
	}
	return ""
}

// lookupStructFields returns a Go-field-name → JSON-name map for a struct
// referenced by its fully-qualified type name (e.g.,
// "json_dto.CopyDocumentRequest"). Returns nil if the type can't be found.
func lookupStructFields(bodyType string, meta *metadata.Metadata) map[string]string {
	if meta == nil {
		return nil
	}
	parts := TypeParts(strings.TrimPrefix(bodyType, "*"))
	if parts.PkgName == "" || parts.TypeName == "" {
		return nil
	}
	pkg, ok := meta.Packages[parts.PkgName]
	if !ok {
		return nil
	}
	for _, file := range pkg.Files {
		typ, ok := file.Types[parts.TypeName]
		if !ok {
			continue
		}
		out := make(map[string]string, len(typ.Fields))
		for _, field := range typ.Fields {
			goName := stringFromPool(meta, field.Name)
			if goName == "" {
				continue
			}
			tag := stringFromPool(meta, field.Tag)
			jsonName := extractJSONName(tag)
			if jsonName == "" {
				jsonName = goName
			}
			out[goName] = jsonName
		}
		return out
	}
	return nil
}
