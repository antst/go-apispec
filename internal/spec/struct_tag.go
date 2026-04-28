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
	"strings"
)

// apispecTagKey is the struct-tag key apispec recognises for OpenAPI schema
// hints that flow analysis can't infer (e.g. UUID-typed string fields that
// are never passed to uuid.Parse, or fields that hold an RFC3339 timestamp
// without a runtime parser call).
const apispecTagKey = "apispec"

// apispecTag captures the OpenAPI schema overrides declared in an
// `apispec:"..."` struct tag. Empty fields mean "no override" rather than
// "blank value" — applyAPISpecTag only writes non-empty entries onto the
// schema so it never erases existing data.
type apispecTag struct {
	Type   string
	Format string
}

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
		}
	}
	if t.Type == "" && t.Format == "" {
		return nil
	}
	return t
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
