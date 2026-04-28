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

func TestApplyAPISpecTag_NilGuards(t *testing.T) {
	applyAPISpecTag(nil, &apispecTag{Format: "uuid"}) // shouldn't panic
	applyAPISpecTag(&Schema{}, nil)                   // shouldn't panic
	s := &Schema{Type: "string"}
	applyAPISpecTag(s, nil)
	assert.Equal(t, "string", s.Type, "nil tag → no change")
}
