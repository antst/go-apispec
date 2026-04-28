package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

// mkSelArgPtr builds a CallArgument representing `<recvVar>.<field>` for
// use in field-inference tests. It mirrors how metadata.go builds selector
// arguments from ast.SelectorExpr.
func mkSelArgPtr(meta *metadata.Metadata, recvVar, field string) *metadata.CallArgument {
	x := makeCallArg(meta)
	x.SetKind(metadata.KindIdent)
	x.SetName(recvVar)

	sel := makeCallArg(meta)
	sel.SetKind(metadata.KindIdent)
	sel.SetName(field)

	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindSelector)
	arg.X = x
	arg.Sel = sel
	return arg
}

// wrapUnary wraps an inner CallArgument as `*<inner>` (KindUnary "*"), which
// is how the metadata represents pointer dereference like `*body.Field`.
func wrapUnary(meta *metadata.Metadata, inner *metadata.CallArgument, op string) *metadata.CallArgument {
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindUnary)
	arg.X = inner
	arg.SetValue(op)
	return arg
}

func TestSelectorFieldOnTarget_DirectSelector(t *testing.T) {
	meta := newTestMeta()
	arg := mkSelArgPtr(meta, "body", "SourceID")
	assert.Equal(t, "SourceID", selectorFieldOnTarget(arg, "body"))
}

func TestSelectorFieldOnTarget_DereferencedPointer(t *testing.T) {
	meta := newTestMeta()
	inner := mkSelArgPtr(meta, "body", "TagsetID")
	arg := wrapUnary(meta, inner, "*")
	assert.Equal(t, "TagsetID", selectorFieldOnTarget(arg, "body"))
}

func TestSelectorFieldOnTarget_AddressOf(t *testing.T) {
	// `&body.Field` — uncommon as a converter arg but still a valid selector
	// shape. The unary unwrap should handle it.
	meta := newTestMeta()
	inner := mkSelArgPtr(meta, "body", "ID")
	arg := wrapUnary(meta, inner, "&")
	assert.Equal(t, "ID", selectorFieldOnTarget(arg, "body"))
}

func TestSelectorFieldOnTarget_DifferentVar(t *testing.T) {
	meta := newTestMeta()
	arg := mkSelArgPtr(meta, "other", "SourceID")
	assert.Empty(t, selectorFieldOnTarget(arg, "body"))
}

func TestSelectorFieldOnTarget_NotASelector(t *testing.T) {
	meta := newTestMeta()
	ident := makeIdentArg(meta, "body", "")
	assert.Empty(t, selectorFieldOnTarget(ident, "body"))
	literal := makeLiteralArg(meta, "x")
	assert.Empty(t, selectorFieldOnTarget(literal, "body"))
}

func TestSelectorFieldOnTarget_NilGuards(t *testing.T) {
	assert.Empty(t, selectorFieldOnTarget(nil, "body"))
	meta := newTestMeta()
	bare := makeCallArg(meta)
	bare.SetKind(metadata.KindSelector) // no X / Sel
	assert.Empty(t, selectorFieldOnTarget(bare, "body"))
	bareUnary := makeCallArg(meta)
	bareUnary.SetKind(metadata.KindUnary) // no X
	assert.Empty(t, selectorFieldOnTarget(bareUnary, "body"))
	// Selector whose X isn't an ident — e.g. `pkg.Const.Field` — must not match.
	meta2 := newTestMeta()
	innerSel := mkSelArgPtr(meta2, "pkg", "Const")
	wrapped := makeCallArg(meta2)
	wrapped.SetKind(metadata.KindSelector)
	wrapped.X = innerSel
	wrapped.Sel = makeIdentArg(meta2, "Field", "")
	assert.Empty(t, selectorFieldOnTarget(wrapped, "body"))
}

func TestLookupStructFields_NotFound(t *testing.T) {
	meta := newTestMeta()
	assert.Nil(t, lookupStructFields("missing.Type", meta))
	assert.Nil(t, lookupStructFields("just-a-name", meta), "unparseable type → nil")
}

func TestLookupStructFields_NilGuards(t *testing.T) {
	assert.Nil(t, lookupStructFields("pkg.Type", nil))
}

func TestLookupStructFields_TypeInOnlyOneOfMultipleFiles(t *testing.T) {
	// Two files in the same package; the requested type lives in the
	// second. The helper has to keep scanning past the empty file rather
	// than returning early.
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"a.go": {Types: map[string]*metadata.Type{"OtherType": {}}},
				"b.go": {Types: map[string]*metadata.Type{
					"Req": {
						Name:   meta.StringPool.Get("Req"),
						Fields: []metadata.Field{{Name: meta.StringPool.Get("ID")}},
					},
				}},
			},
		},
	}
	got := lookupStructFields("pkg.Req", meta)
	require.Len(t, got, 1)
	assert.Equal(t, "ID", got["ID"])
}

func TestLookupStructFields_TypeMissingInAllFiles(t *testing.T) {
	// Package exists but the type doesn't live in any of its files.
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"a.go": {Types: map[string]*metadata.Type{"OtherType": {}}},
				"b.go": {Types: map[string]*metadata.Type{"YetAnother": {}}},
			},
		},
	}
	assert.Nil(t, lookupStructFields("pkg.Req", meta))
}

func TestLookupStructFields_FieldWithEmptyName_Skipped(t *testing.T) {
	// Defensive: a field whose Name index resolves to "" is skipped rather
	// than producing an empty-string key in the result.
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {Types: map[string]*metadata.Type{
					"Req": {
						Name: meta.StringPool.Get("Req"),
						Fields: []metadata.Field{
							{Name: -1}, // -1 → "" out of the StringPool
							{Name: meta.StringPool.Get("Real")},
						},
					},
				}},
			},
		},
	}
	got := lookupStructFields("pkg.Req", meta)
	require.Len(t, got, 1)
	_, hasEmpty := got[""]
	assert.False(t, hasEmpty, "empty Go field name must not appear in the map")
	assert.Equal(t, "Real", got["Real"])
}

func TestLookupStructFields_FoundUsesJSONNameWhenPresent(t *testing.T) {
	meta := newTestMeta()
	tagSourceID := meta.StringPool.Get(`json:"sourceId"`)
	tagPlain := meta.StringPool.Get("")
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name: meta.StringPool.Get("Req"),
							Fields: []metadata.Field{
								{Name: meta.StringPool.Get("SourceID"), Tag: tagSourceID},
								{Name: meta.StringPool.Get("Untagged"), Tag: tagPlain},
							},
						},
					},
				},
			},
		},
	}
	got := lookupStructFields("pkg.Req", meta)
	require.Len(t, got, 2)
	assert.Equal(t, "sourceId", got["SourceID"], "json tag → used")
	assert.Equal(t, "Untagged", got["Untagged"], "no tag → Go name")
}

func TestApplyJSONFieldConverterFormats_HappyPath(t *testing.T) {
	// Build a mini call graph: Copy(body) → uuid.Parse(body.SourceID) and
	// strings.TrimSpace(body.DisplayName). The converter run should set
	// format=uuid on sourceId and leave displayName alone.
	meta := newTestMeta()
	sourceTag := meta.StringPool.Get(`json:"sourceId"`)
	displayTag := meta.StringPool.Get(`json:"displayName"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name: meta.StringPool.Get("Req"),
							Fields: []metadata.Field{
								{Name: meta.StringPool.Get("SourceID"), Tag: sourceTag},
								{Name: meta.StringPool.Get("DisplayName"), Tag: displayTag},
							},
						},
					},
				},
			},
		},
	}

	converterEdge := makeEdge(meta, "Copy", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "SourceID")})
	noisyEdge := makeEdge(meta, "Copy", "pkg", "TrimSpace", "strings",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "DisplayName")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		converterEdge.Caller.BaseID(): {&converterEdge, &noisyEdge},
	}

	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"sourceId":    {Type: "string"},
					"displayName": {Type: "string"},
				},
			},
		},
	}

	applyJSONFieldConverterFormats("body", "pkg.Req", converterEdge.Caller.BaseID(), route)

	src := route.UsedTypes["pkg.Req"].Properties["sourceId"]
	disp := route.UsedTypes["pkg.Req"].Properties["displayName"]
	assert.Equal(t, "string", src.Type)
	assert.Equal(t, "uuid", src.Format, "uuid.Parse(body.SourceID) → format=uuid")
	assert.Equal(t, "string", disp.Type)
	assert.Empty(t, disp.Format, "non-converter consumer must not set a format")
}

func TestApplyJSONFieldConverterFormats_FieldMapsToMissingProperty(t *testing.T) {
	// Field exists on the Go struct (so lookupStructFields returns it) but the
	// schema's Properties map doesn't have a matching entry — possible when
	// the schema was generated with a stale snapshot or partially trimmed.
	// The helper must skip rather than create a property out of thin air.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"id"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {Types: map[string]*metadata.Type{
					"Req": {
						Name:   meta.StringPool.Get("Req"),
						Fields: []metadata.Field{{Name: meta.StringPool.Get("ID"), Tag: tag}},
					},
				}},
			},
		},
	}
	edge := makeEdge(meta, "Handler", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "ID")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}
	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {Type: "object", Properties: map[string]*Schema{}}, // empty
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Empty(t, route.UsedTypes["pkg.Req"].Properties,
		"missing property must not be created by inference")
}

func TestApplyJSONFieldConverterFormats_NilProperty(t *testing.T) {
	// Properties map has the key but the value is nil (defensive: callers
	// shouldn't normally insert nil, but the helper guards against it).
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"id"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {Types: map[string]*metadata.Type{
					"Req": {
						Name:   meta.StringPool.Get("Req"),
						Fields: []metadata.Field{{Name: meta.StringPool.Get("ID"), Tag: tag}},
					},
				}},
			},
		},
	}
	edge := makeEdge(meta, "Handler", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "ID")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}
	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {Type: "object", Properties: map[string]*Schema{"id": nil}},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Nil(t, route.UsedTypes["pkg.Req"].Properties["id"],
		"nil property must remain nil — never instantiate")
}

func TestApplyJSONFieldConverterFormats_ZeroFieldsStruct(t *testing.T) {
	// Struct exists in metadata but has no fields — early return without
	// scanning any caller edges.
	meta := newTestMeta()
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Empty": {Name: meta.StringPool.Get("Empty")},
					},
				},
			},
		},
	}
	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Empty": {Type: "object", Properties: map[string]*Schema{}},
		},
	}
	// Should not panic and should be a no-op.
	applyJSONFieldConverterFormats("body", "pkg.Empty", "pkg.Caller", route)
	assert.Empty(t, route.UsedTypes["pkg.Empty"].Properties)
}

func TestApplyJSONFieldConverterFormats_SkipsNonSelectorArgs(t *testing.T) {
	// Mix of literal/ident args interleaved with the selector — the literals
	// must be skipped without affecting iteration.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"id"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name:   meta.StringPool.Get("Req"),
							Fields: []metadata.Field{{Name: meta.StringPool.Get("ID"), Tag: tag}},
						},
					},
				},
			},
		},
	}

	edge := makeEdge(meta, "Handler", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{
			makeLiteralArg(meta, "ignored"),
			makeIdentArg(meta, "v", "string"), // ident, not a selector
			mkSelArgPtr(meta, "body", "ID"),
		})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}
	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"id": {Type: "string"},
				},
			},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["id"].Format)
}

func TestApplyJSONFieldConverterFormats_FieldNotInStruct(t *testing.T) {
	// `body.UnknownField` consumed by a converter — but UnknownField isn't
	// declared on the struct (e.g. typo or refactor leftover). The helper
	// must skip it rather than mutating an unrelated property.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"id"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name:   meta.StringPool.Get("Req"),
							Fields: []metadata.Field{{Name: meta.StringPool.Get("ID"), Tag: tag}},
						},
					},
				},
			},
		},
	}
	edge := makeEdge(meta, "Handler", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "Phantom")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}
	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"id": {Type: "string"},
				},
			},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Empty(t, route.UsedTypes["pkg.Req"].Properties["id"].Format,
		"unrelated field reference must not change any property")
}

func TestApplyJSONFieldConverterFormats_NoOpGuards(_ *testing.T) {
	meta := newTestMeta()
	emptyRoute := &RouteInfo{Metadata: meta, UsedTypes: map[string]*Schema{}}

	// Each guard returns silently — the assertion is "no panic".
	applyJSONFieldConverterFormats("", "pkg.Req", "x", emptyRoute)
	applyJSONFieldConverterFormats("body", "", "x", emptyRoute)
	applyJSONFieldConverterFormats("body", "pkg.Req", "x", nil)
	applyJSONFieldConverterFormats("body", "pkg.Req", "x", &RouteInfo{})    // nil meta
	applyJSONFieldConverterFormats("body", "pkg.Req", "x", emptyRoute)      // schema absent
	applyJSONFieldConverterFormats("body", "missing.Type", "x", &RouteInfo{ // schema present, type metadata absent
		Metadata:  meta,
		UsedTypes: map[string]*Schema{"missing.Type": {Type: "object", Properties: map[string]*Schema{}}},
	})
}

func TestApplyJSONFieldConverterFormats_DoesNotClobberExistingFormat(t *testing.T) {
	// If a property already carries a more-specific format (set by a
	// validator tag or a previous pass), flow inference must not overwrite it.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"id"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name:   meta.StringPool.Get("Req"),
							Fields: []metadata.Field{{Name: meta.StringPool.Get("ID"), Tag: tag}},
						},
					},
				},
			},
		},
	}

	edge := makeEdge(meta, "Copy", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "ID")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}

	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"id": {Type: "string", Format: "byte"},
				},
			},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Equal(t, "byte", route.UsedTypes["pkg.Req"].Properties["id"].Format,
		"existing format must not be overwritten by flow inference")
}

func TestApplyJSONFieldConverterFormats_PreservesExistingNonStringType(t *testing.T) {
	// If a property's type is already non-string (e.g., integer set by a
	// previous pass), flow inference must not change it just because the
	// converter would pick a different type.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"count"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name:   meta.StringPool.Get("Req"),
							Fields: []metadata.Field{{Name: meta.StringPool.Get("Count"), Tag: tag}},
						},
					},
				},
			},
		},
	}

	// Pretend someone passes body.Count to ParseBool — converter says boolean,
	// but the property was already typed as integer (which we trust more).
	edge := makeEdge(meta, "Handler", "pkg", "ParseBool", "strconv",
		[]*metadata.CallArgument{mkSelArgPtr(meta, "body", "Count")})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}

	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"count": {Type: "integer"},
				},
			},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Equal(t, "integer", route.UsedTypes["pkg.Req"].Properties["count"].Type,
		"non-string existing type wins over converter inference")
}

func TestApplyJSONFieldConverterFormats_HandlesPointerDeref(t *testing.T) {
	// uuid.Parse(*body.OptionalID) is the common pattern for optional UUID
	// pointer fields. The dereference shouldn't break the selector match.
	meta := newTestMeta()
	tag := meta.StringPool.Get(`json:"optionalId,omitempty"`)
	meta.Packages = map[string]*metadata.Package{
		"pkg": {
			Files: map[string]*metadata.File{
				"f.go": {
					Types: map[string]*metadata.Type{
						"Req": {
							Name:   meta.StringPool.Get("Req"),
							Fields: []metadata.Field{{Name: meta.StringPool.Get("OptionalID"), Tag: tag}},
						},
					},
				},
			},
		},
	}

	deref := wrapUnary(meta, mkSelArgPtr(meta, "body", "OptionalID"), "*")
	edge := makeEdge(meta, "Handler", "pkg", "Parse", "github.com/google/uuid",
		[]*metadata.CallArgument{deref})
	meta.Callers = map[string][]*metadata.CallGraphEdge{
		edge.Caller.BaseID(): {&edge},
	}

	route := &RouteInfo{
		Metadata: meta,
		UsedTypes: map[string]*Schema{
			"pkg.Req": {
				Type: "object",
				Properties: map[string]*Schema{
					"optionalId": {Type: "string"},
				},
			},
		},
	}
	applyJSONFieldConverterFormats("body", "pkg.Req", edge.Caller.BaseID(), route)
	assert.Equal(t, "uuid", route.UsedTypes["pkg.Req"].Properties["optionalId"].Format)
}
