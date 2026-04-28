package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

func TestDecodeTargetVarName_Ident(t *testing.T) {
	meta := newTestMeta()
	arg := makeIdentArg(meta, "body", "main")
	assert.Equal(t, "body", decodeTargetVarName(arg))
}

func TestDecodeTargetVarName_AddressOf(t *testing.T) {
	// `&body` — the dominant idiom for json.Decode/json.Unmarshal targets.
	meta := newTestMeta()
	inner := makeIdentArg(meta, "body", "main")
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindUnary)
	arg.X = inner
	arg.SetValue("&")
	assert.Equal(t, "body", decodeTargetVarName(arg))
}

func TestDecodeTargetVarName_StarDeref(t *testing.T) {
	// `*body` — uncommon as a Decode target, but the helper handles it
	// symmetrically with KindUnary so var-name extraction stays predictable.
	meta := newTestMeta()
	inner := makeIdentArg(meta, "body", "main")
	arg := makeCallArg(meta)
	arg.SetKind(metadata.KindStar)
	arg.X = inner
	assert.Equal(t, "body", decodeTargetVarName(arg))
}

func TestDecodeTargetVarName_NotAVar(t *testing.T) {
	// Literal, raw, or complex expressions don't yield a target var — flow
	// inference simply doesn't apply, so the helper returns "".
	meta := newTestMeta()
	assert.Empty(t, decodeTargetVarName(makeLiteralArg(meta, "{}")))
	assert.Empty(t, decodeTargetVarName(nil))

	// Unary on something that isn't an ident — e.g., `&someFunc()`.
	bareUnary := makeCallArg(meta)
	bareUnary.SetKind(metadata.KindUnary)
	bareUnary.SetValue("&")
	bareUnary.X = makeLiteralArg(meta, "{}")
	assert.Empty(t, decodeTargetVarName(bareUnary))
}

func TestDecodeTargetVarName_UnaryWithoutInner(t *testing.T) {
	meta := newTestMeta()
	bare := makeCallArg(meta)
	bare.SetKind(metadata.KindUnary)
	assert.Empty(t, decodeTargetVarName(bare))
}
