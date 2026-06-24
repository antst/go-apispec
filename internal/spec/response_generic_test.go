package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestResponseResolveTypeOrigin_ResolvedTypeWins covers the early-return
// branch added so the response matcher honours an already-pinned concrete
// type rather than re-tracing from scratch.
func TestResponseResolveTypeOrigin_ResolvedTypeWins(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	cp := NewContextProvider(meta)

	arg := makeIdentArg(meta, "v", "T")
	arg.SetResolvedType("pkg.ConcreteType")

	edge := makeEdge(meta, "WriteJSON", "pkg", "Encode", "encoding/json",
		[]*metadata.CallArgument{arg})
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: cp,
			cfg:             cfg,
		},
	}
	got, ref := matcher.resolveTypeOrigin(arg, node, "T")
	assert.Equal(t, "pkg.ConcreteType", got)
	// FIX 1: the resolved-type read site carries the lockstep ref (set by
	// SetResolvedType), not a bare string.
	if assert.NotNil(t, ref) {
		assert.Equal(t, metadata.ParseTypeRef("pkg.ConcreteType").String(), ref.String())
	}
}

// TestSharedResolveTypeOrigin_ResolvedTypeShortCircuit covers the
// fast-path in sharedResolveTypeOrigin: when the argument already carries
// a resolved-type annotation (set by an earlier pass), we return it
// directly without touching the call graph.
func TestSharedResolveTypeOrigin_ResolvedTypeShortCircuit(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeIdentArg(meta, "x", "")
	arg.SetResolvedType("pkg.Resolved")
	node := makeTrackerNode(nil)

	got, ref := sharedResolveTypeOrigin(arg, node, "original", cp, false)
	assert.Equal(t, "pkg.Resolved", got)
	// FIX 1: lockstep ref threaded out of the resolved-type branch.
	if assert.NotNil(t, ref) {
		assert.Equal(t, metadata.ParseTypeRef("pkg.Resolved").String(), ref.String())
	}
}

// TestSharedResolveTypeOrigin_ResolvedTypeNilRefBackfilled drives the FIX 1
// boundary fallback at a read site: a deserialized arg can carry a non-empty
// ResolvedType (the exported field) with a nil ResolvedTypeRef. The read site
// must backfill a non-nil ref (refForResolved → ParseTypeRef) — the same parse
// schemaForType would otherwise perform — rather than thread the nil it found.
func TestSharedResolveTypeOrigin_ResolvedTypeNilRefBackfilled(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	arg := makeIdentArg(meta, "x", "")
	// Simulate deserialization: pooled ResolvedType present, structured ref absent.
	arg.ResolvedType = meta.StringPool.Get("pkg.Resolved")
	arg.ResolvedTypeRef = nil
	node := makeTrackerNode(nil)

	got, ref := sharedResolveTypeOrigin(arg, node, "original", cp, false)
	assert.Equal(t, "pkg.Resolved", got)
	if assert.NotNil(t, ref, "a deserialized ResolvedType without its ref must be backfilled") {
		assert.Equal(t, metadata.ParseTypeRef("pkg.Resolved").String(), ref.String())
	}
}

// TestResponseResolveTypeOrigin_GenericTypeParamSubstituted exercises the
// fallback into traceGenericOrigin when the arg has no explicit resolved
// type but the node carries a TypeParamMap pointing T → ConcreteType (as a
// `WriteJSON[ConcreteType]` instantiation would).
func TestResponseResolveTypeOrigin_GenericTypeParamSubstituted(t *testing.T) {
	meta := newTestMeta()
	cfg := &APISpecConfig{}
	cp := NewContextProvider(meta)

	arg := makeIdentArg(meta, "v", "T")

	edge := makeEdge(meta, "WriteJSON", "pkg", "Encode", "encoding/json",
		[]*metadata.CallArgument{arg})
	edge.TypeParamMap = map[string]string{"T": "pkg.ConcreteType"}
	node := makeTrackerNode(&edge)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: cp,
			cfg:             cfg,
		},
	}
	got, _ := matcher.resolveTypeOrigin(arg, node, "T")
	assert.Equal(t, "pkg.ConcreteType", got)
}
