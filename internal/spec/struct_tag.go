// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package spec

import (
	"reflect"
	"strconv"
	"strings"
)

// apispecTagKey is the struct-tag key apispec recognises for OpenAPI schema
// hints that flow analysis can't infer (e.g. UUID-typed string fields that
// are never passed to uuid.Parse, or fields that hold an RFC3339 timestamp
// without a runtime parser call).
const apispecTagKey = "apispec"

// apispecTag captures the OpenAPI schema overrides declared in an
// `apispec:"..."` struct tag. Empty / nil fields mean "no override" rather
// than "blank value" — appliers only write set entries onto the schema so
// they never erase existing data.
//
// Field-level keys (Type, Format) apply to the property a tag sits on.
// Struct-level keys (MinProperties, AnyOf) apply to the parent schema and
// are read off a blank marker field — `_ struct{} `apispec:"..."“ — since
// Go struct tags are inherently field-scoped.
type apispecTag struct {
	Type   string
	Format string

	// MinProperties mirrors the OpenAPI keyword: when non-nil, the parent
	// schema must have at least this many properties set. Pointer so we can
	// distinguish "not specified" from "explicitly 0" (the latter is a
	// no-op but still valid spec).
	MinProperties *int

	// AnyOf lists the JSON property names whose presence individually
	// satisfies the schema. Emitted as a slice of `{required: [name]}`
	// schemas under the parent's `anyOf`. Empty when not specified.
	AnyOf []string
}

// anyOfFieldSeparator splits the value of `anyOf=...` into individual JSON
// property names. We use `|` rather than `,` because `,` is already the
// key=value pair separator inside the apispec tag.
const anyOfFieldSeparator = "|"

// parseAPISpecTag returns the apispec tag declared on a struct field, or nil
// when the tag is absent or has no recognised keys.
//
// The wire format is the standard Go struct-tag convention used by
// encoding/json, validate, etc.: `apispec:"key=value,key=value"`. Recognised
// keys: `format`, `type`. Unknown keys are silently ignored so users can
// embed third-party hints in the same tag without breaking apispec.
func parseAPISpecTag(rawTag string) *apispecTag {
	if rawTag == "" {
		return nil
	}
	value := reflect.StructTag(rawTag).Get(apispecTagKey)
	if value == "" {
		return nil
	}

	t := &apispecTag{}
	for _, part := range strings.Split(value, ",") {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		switch key {
		case "format":
			t.Format = val
		case "type":
			t.Type = val
		case "minProperties":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				continue
			}
			t.MinProperties = &n
		case "anyOf":
			t.AnyOf = splitAnyOfFields(val)
		}
	}
	if t.Type == "" && t.Format == "" && t.MinProperties == nil && len(t.AnyOf) == 0 {
		return nil
	}
	return t
}

// splitAnyOfFields parses the `anyOf=...` value into trimmed, non-empty
// field names. Empty fragments (e.g. trailing `|`) are dropped.
func splitAnyOfFields(value string) []string {
	parts := strings.Split(value, anyOfFieldSeparator)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyAPISpecTag overwrites the schema's type/format from the tag. Tag
// declarations are user-explicit and therefore the strongest signal — they
// win over flow inference, validation tags, and Go-type-derived defaults.
func applyAPISpecTag(schema *Schema, t *apispecTag) {
	if schema == nil || t == nil {
		return
	}
	if t.Type != "" {
		schema.Type = t.Type
	}
	if t.Format != "" {
		schema.Format = t.Format
	}
}

// applyStructLevelAPISpecTag writes struct-scope overrides (MinProperties,
// AnyOf) onto the parent schema. Called once per struct after every field
// has been processed, with the tag read from a blank marker field.
//
// Each entry in t.AnyOf becomes its own `{required: [name]}` schema so
// generated clients see "at least one of these named fields must be
// present" rather than the looser MinProperties-only contract.
func applyStructLevelAPISpecTag(schema *Schema, t *apispecTag) {
	if schema == nil || t == nil {
		return
	}
	if t.MinProperties != nil {
		schema.MinProperties = *t.MinProperties
	}
	if len(t.AnyOf) > 0 {
		anyOf := make([]*Schema, 0, len(t.AnyOf))
		for _, name := range t.AnyOf {
			anyOf = append(anyOf, &Schema{Required: []string{name}})
		}
		schema.AnyOf = anyOf
	}
}
