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

// maxFieldInferenceDepth bounds how many helper-call levels the field-format
// inference follows below the handler. 0 would be the legacy intraprocedural
// behaviour (handler body only); the call graph is already indexed so each
// extra level is cheap, but the depth is capped so a pathological graph can't
// drive unbounded recursion. One or two levels covers the lint-driven helper
// extraction this guards against (issue #36).
const maxFieldInferenceDepth = 8

// fieldBinding records, within a single function frame, how that frame's local
// variables and parameters relate to the request-body struct whose field
// formats we're inferring:
//
//   - structVars: names that currently hold the whole struct, so a selector
//     `<name>.<Field>` on one of them identifies a struct field.
//   - fieldVars: names that hold a single field's *value* (because the call
//     site passed `body.Field` straight into a parameter), mapped to the Go
//     field name — so a converter applied directly to such a name pins that
//     field's format.
type fieldBinding struct {
	structVars map[string]struct{}
	fieldVars  map[string]string
}

func (b fieldBinding) empty() bool {
	return len(b.structVars) == 0 && len(b.fieldVars) == 0
}

// applyJSONFieldConverterFormats back-propagates converter-derived OpenAPI
// schema formats onto a request-body struct's fields, following helper calls
// through the call graph (issue #36) so that extracting a `uuid.Parse` (or
// other converter) into a helper no longer silently drops the inferred format.
//
// Starting from the handler frame (callerBaseID) with the decode target as the
// sole struct variable, every outgoing edge is examined: a known converter
// applied to a tracked struct field (or to a variable that holds a field's
// value) writes its type/format onto the matching property; a call into an
// analysable project function is followed one level deeper with the struct/
// field bindings remapped onto that function's parameters.
//
// The struct's JSON-tag derived property names are resolved from the metadata
// so the inference doesn't depend on field-name casing conventions.
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

	binding := fieldBinding{
		structVars: map[string]struct{}{targetVar: {}},
		fieldVars:  map[string]string{},
	}
	propagateFieldFormats(callerBaseID, binding, fields, structSchema, route.Metadata, map[string]struct{}{}, 0)
}

// propagateFieldFormats walks the outgoing edges of one function frame,
// applying converter formats to bound fields and recursing into project
// helpers. `onStack` holds the frames currently being expanded so a recursive
// helper can't loop; it's unwound on return so two sibling calls into the same
// helper (with different bindings) are both followed.
func propagateFieldFormats(
	callerBaseID string,
	binding fieldBinding,
	fields map[string]string,
	structSchema *Schema,
	meta *metadata.Metadata,
	onStack map[string]struct{},
	depth int,
) {
	if binding.empty() || depth > maxFieldInferenceDepth {
		return
	}
	if _, busy := onStack[callerBaseID]; busy {
		return
	}
	onStack[callerBaseID] = struct{}{}
	defer delete(onStack, callerBaseID)

	for _, edge := range meta.Callers[callerBaseID] {
		// A known converter is a leaf: record the format and stop — it never
		// also counts as a helper to descend into.
		if c := lookupParamConverter(
			stringFromPool(meta, edge.Callee.Pkg),
			stringFromPool(meta, edge.Callee.Name),
		); c != nil {
			for _, arg := range edge.Args {
				if fieldGoName := fieldNameForArg(arg, binding); fieldGoName != "" {
					applyConverterToField(structSchema, fields, fieldGoName, c)
				}
			}
			continue
		}

		// Otherwise, if this is an analysable project function that the struct
		// (or one of its fields) flows into, follow it one level deeper.
		if depth >= maxFieldInferenceDepth {
			continue
		}
		calleeBase := edge.Callee.BaseID()
		if len(meta.Callers[calleeBase]) == 0 {
			continue // external/stdlib/leaf — no body to analyse
		}
		if child := childBinding(edge, binding); !child.empty() {
			propagateFieldFormats(calleeBase, child, fields, structSchema, meta, onStack, depth+1)
		}
	}
}

// fieldNameForArg returns the Go field name an argument refers to under the
// current binding: a selector `<structVar>.<Field>` (optionally behind a
// unary/star, covering `*body.Field` for pointer fields), or a bare identifier
// previously bound to a field's value. Returns "" for anything else.
func fieldNameForArg(arg *metadata.CallArgument, binding fieldBinding) string {
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindUnary, metadata.KindStar:
		return fieldNameForArg(arg.X, binding)
	case metadata.KindSelector:
		if arg.X == nil || arg.Sel == nil || arg.X.GetKind() != metadata.KindIdent {
			return ""
		}
		if _, ok := binding.structVars[arg.X.GetName()]; ok {
			return arg.Sel.GetName()
		}
	case metadata.KindIdent:
		if f, ok := binding.fieldVars[arg.GetName()]; ok {
			return f
		}
	}
	return ""
}

// childBinding projects the caller-frame binding onto a callee's parameters
// using the edge's parameter→argument map, so the recursion can reason in the
// callee's own variable names. A parameter receiving the whole struct becomes
// a struct variable; one receiving `body.Field` (or a variable already bound
// to a field) becomes a field variable.
func childBinding(edge *metadata.CallGraphEdge, binding fieldBinding) fieldBinding {
	child := fieldBinding{structVars: map[string]struct{}{}, fieldVars: map[string]string{}}
	for param, arg := range edge.ParamArgMap {
		base := unwrapArgRefs(&arg)
		if base == nil {
			continue
		}
		switch base.GetKind() {
		case metadata.KindIdent:
			if _, ok := binding.structVars[base.GetName()]; ok {
				child.structVars[param] = struct{}{}
			} else if f, ok := binding.fieldVars[base.GetName()]; ok {
				child.fieldVars[param] = f
			}
		case metadata.KindSelector:
			if base.X != nil && base.Sel != nil && base.X.GetKind() == metadata.KindIdent {
				if _, ok := binding.structVars[base.X.GetName()]; ok {
					child.fieldVars[param] = base.Sel.GetName()
				}
			}
		}
	}
	return child
}

// applyConverterToField writes a converter's type/format onto the schema
// property of the named Go field, without clobbering an already-set format.
func applyConverterToField(structSchema *Schema, fields map[string]string, fieldGoName string, c *paramConverter) {
	jsonName, ok := fields[fieldGoName]
	if !ok {
		return
	}
	prop, ok := structSchema.Properties[jsonName]
	if !ok || prop == nil {
		return
	}
	// Don't clobber an explicit format — flow inference is best-effort and a
	// previous edge, the validator tag pass, or a nested-type pass might have
	// set something more specific.
	if prop.Format == "" {
		prop.Format = c.Format
	}
	if c.Type != "" && (prop.Type == "" || prop.Type == "string") {
		// Converters that change the wire type (Atoi → integer) only apply when
		// the JSON field is currently typed as string — otherwise the explicit
		// struct-field type wins.
		prop.Type = c.Type
	}
}

// unwrapArgRefs strips leading unary/star wrappers (`&x`, `*x`) to reach the
// underlying identifier or selector.
func unwrapArgRefs(arg *metadata.CallArgument) *metadata.CallArgument {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar) {
		arg = arg.X
	}
	return arg
}

// lookupStructFields returns a Go-field-name → JSON-name map for a struct
// referenced by its fully-qualified type name (e.g.,
// "json_dto.CopyDocumentRequest"). Returns nil if the type can't be found.
func lookupStructFields(bodyType string, meta *metadata.Metadata) map[string]string {
	if meta == nil {
		return nil
	}
	typ := typeByRefGated(metadata.ParseTypeRef(strings.TrimPrefix(bodyType, "*")), meta)
	if typ == nil {
		return nil
	}
	out := make(map[string]string, len(typ.Fields))
	for _, field := range typ.Fields {
		goName := stringFromPool(meta, field.Name)
		if goName == "" {
			continue
		}
		// Mirror generateStructSchema's field visibility: an unexported field and a
		// `json:"-"` field have no property in the schema, so there is nothing to
		// infer a format onto — skip them here too rather than carry a phantom entry.
		if stringFromPool(meta, field.Scope) == metadata.ScopeUnexported {
			continue
		}
		tag := stringFromPool(meta, field.Tag)
		jsonName, skip, _ := jsonFieldName(tag)
		if skip {
			continue
		}
		if jsonName == "" {
			jsonName = goName
		}
		out[goName] = jsonName
	}
	return out
}
