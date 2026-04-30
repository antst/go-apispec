package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAPISpecTag_FormatOnly(t *testing.T) {
	tag := parseAPISpecTag(`json:"sourceId" apispec:"format=uuid"`)
	require.NotNil(t, tag)
	assert.Equal(t, "uuid", tag.Format)
	assert.Empty(t, tag.Type)
}

func TestParseAPISpecTag_TypeOnly(t *testing.T) {
	tag := parseAPISpecTag(`apispec:"type=integer"`)
	require.NotNil(t, tag)
	assert.Equal(t, "integer", tag.Type)
	assert.Empty(t, tag.Format)
}

func TestParseAPISpecTag_TypeAndFormat(t *testing.T) {
	tag := parseAPISpecTag(`apispec:"type=integer,format=int64"`)
	require.NotNil(t, tag)
	assert.Equal(t, "integer", tag.Type)
	assert.Equal(t, "int64", tag.Format)
}

func TestParseAPISpecTag_UnknownKeysIgnored(t *testing.T) {
	// Unknown keys mixed in with recognised ones don't break parsing — the
	// known keys still apply. Lets users embed third-party hints alongside.
	tag := parseAPISpecTag(`apispec:"format=uuid,unknown=foo,deprecated=true"`)
	require.NotNil(t, tag)
	assert.Equal(t, "uuid", tag.Format)
}

func TestParseAPISpecTag_AllUnknown_NilResult(t *testing.T) {
	// When every key is unknown there's nothing to apply — return nil so
	// callers can short-circuit without checking each field.
	tag := parseAPISpecTag(`apispec:"unknown=value"`)
	assert.Nil(t, tag)
}

func TestParseAPISpecTag_EmptyValue_Skipped(t *testing.T) {
	tag := parseAPISpecTag(`apispec:"format="`)
	assert.Nil(t, tag, "empty value should not be set")
}

func TestParseAPISpecTag_MalformedPart_Skipped(t *testing.T) {
	// Parts without an `=` are silently skipped, not treated as bool flags.
	tag := parseAPISpecTag(`apispec:"format=uuid,bareflag"`)
	require.NotNil(t, tag)
	assert.Equal(t, "uuid", tag.Format)
}

func TestParseAPISpecTag_NoTag(t *testing.T) {
	assert.Nil(t, parseAPISpecTag(""))
	assert.Nil(t, parseAPISpecTag(`json:"only"`))
}

func TestParseAPISpecTag_WhitespaceTolerated(t *testing.T) {
	tag := parseAPISpecTag(`apispec:" format = uuid , type = string "`)
	require.NotNil(t, tag)
	assert.Equal(t, "uuid", tag.Format)
	assert.Equal(t, "string", tag.Type)
}

func TestApplyAPISpecTag_OverwritesTypeAndFormat(t *testing.T) {
	s := &Schema{Type: "string"}
	applyAPISpecTag(s, &apispecTag{Type: "integer", Format: "int64"})
	assert.Equal(t, "integer", s.Type)
	assert.Equal(t, "int64", s.Format)
}

func TestApplyAPISpecTag_OnlyFormat_TypePreserved(t *testing.T) {
	// Tags that set only `format` must not erase the existing type.
	s := &Schema{Type: "string", Format: "byte"}
	applyAPISpecTag(s, &apispecTag{Format: "uuid"})
	assert.Equal(t, "string", s.Type)
	assert.Equal(t, "uuid", s.Format)
}

func TestParseAPISpecTag_MinPropertiesAndAnyOf(t *testing.T) {
	tag := parseAPISpecTag(`apispec:"minProperties=1,anyOf=displayName|storageBucketId|temporaryLocation"`)
	require.NotNil(t, tag)
	require.NotNil(t, tag.MinProperties)
	assert.Equal(t, 1, *tag.MinProperties)
	assert.Equal(t, []string{"displayName", "storageBucketId", "temporaryLocation"}, tag.AnyOf)
}

func TestParseAPISpecTag_MinProperties_Invalid(t *testing.T) {
	// Non-integer values are silently ignored — no opinion is better than
	// guessing for a malformed tag.
	tag := parseAPISpecTag(`apispec:"minProperties=not-a-number"`)
	assert.Nil(t, tag, "invalid minProperties + nothing else → nil tag")

	// Mixed with something valid — only the valid bit survives.
	tag = parseAPISpecTag(`apispec:"format=uuid,minProperties=abc"`)
	require.NotNil(t, tag)
	assert.Nil(t, tag.MinProperties)
	assert.Equal(t, "uuid", tag.Format)
}

func TestParseAPISpecTag_MinProperties_Negative(t *testing.T) {
	// Negative values aren't valid OpenAPI — silently rejected.
	tag := parseAPISpecTag(`apispec:"minProperties=-1"`)
	assert.Nil(t, tag)
}

func TestParseAPISpecTag_MinProperties_Zero(t *testing.T) {
	// Zero is technically valid (no constraint, but explicit). Honour it
	// rather than silently dropping it — matches OpenAPI semantics.
	tag := parseAPISpecTag(`apispec:"minProperties=0"`)
	require.NotNil(t, tag)
	require.NotNil(t, tag.MinProperties)
	assert.Equal(t, 0, *tag.MinProperties)
}

func TestParseAPISpecTag_AnyOf_TrimsAndDropsEmpty(t *testing.T) {
	// Whitespace and trailing/duplicated separators are normalized.
	tag := parseAPISpecTag(`apispec:"anyOf= a | b ||c|"`)
	require.NotNil(t, tag)
	assert.Equal(t, []string{"a", "b", "c"}, tag.AnyOf)
}

func TestParseAPISpecTag_AnyOf_AllEmpty(t *testing.T) {
	// `anyOf=|` — every fragment is empty after trimming, so the field
	// stays nil and parseAPISpecTag returns nil overall.
	tag := parseAPISpecTag(`apispec:"anyOf=||"`)
	assert.Nil(t, tag)
}

func TestApplyStructLevelAPISpecTag_BothFields(t *testing.T) {
	s := &Schema{Type: "object", Properties: map[string]*Schema{}}
	one := 1
	applyStructLevelAPISpecTag(s, &apispecTag{
		MinProperties: &one,
		AnyOf:         []string{"a", "b"},
	})
	assert.Equal(t, 1, s.MinProperties)
	require.Len(t, s.AnyOf, 2)
	assert.Equal(t, []string{"a"}, s.AnyOf[0].Required)
	assert.Equal(t, []string{"b"}, s.AnyOf[1].Required)
}

func TestApplyStructLevelAPISpecTag_OnlyMinProperties(t *testing.T) {
	s := &Schema{Type: "object"}
	two := 2
	applyStructLevelAPISpecTag(s, &apispecTag{MinProperties: &two})
	assert.Equal(t, 2, s.MinProperties)
	assert.Empty(t, s.AnyOf, "AnyOf left untouched when not specified")
}

func TestApplyStructLevelAPISpecTag_OnlyAnyOf(t *testing.T) {
	s := &Schema{Type: "object"}
	applyStructLevelAPISpecTag(s, &apispecTag{AnyOf: []string{"x"}})
	assert.Equal(t, 0, s.MinProperties, "MinProperties left untouched")
	require.Len(t, s.AnyOf, 1)
	assert.Equal(t, []string{"x"}, s.AnyOf[0].Required)
}

func TestApplyStructLevelAPISpecTag_NilGuards(t *testing.T) {
	applyStructLevelAPISpecTag(nil, &apispecTag{}) // shouldn't panic
	s := &Schema{}
	applyStructLevelAPISpecTag(s, nil)
	assert.Equal(t, 0, s.MinProperties)
	assert.Nil(t, s.AnyOf)
}

func TestApplyAPISpecTag_NilGuards(t *testing.T) {
	applyAPISpecTag(nil, &apispecTag{Format: "uuid"}) // shouldn't panic
	applyAPISpecTag(&Schema{}, nil)                   // shouldn't panic
	s := &Schema{Type: "string"}
	applyAPISpecTag(s, nil)
	assert.Equal(t, "string", s.Type, "nil tag → no change")
}
