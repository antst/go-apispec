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
	got, _ := matcher.resolveTypeOrigin(arg, node, "T")
	assert.Equal(t, "pkg.ConcreteType", got)
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

	got, _ := sharedResolveTypeOrigin(arg, node, "original", cp, false)
	assert.Equal(t, "pkg.Resolved", got)
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
