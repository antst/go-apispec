// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

func hfLimits() metadata.TrackerLimits {
	return metadata.TrackerLimits{MaxRecursionDepth: 5, MaxNodesPerTree: 1000, MaxChildrenPerNode: 100}
}

// hfInterfaceType registers an interface type with the given ImplementedBy
// entries under meta.Packages[pkg].
func hfInterfaceType(meta *metadata.Metadata, pkg, name string, implementedBy ...string) {
	sp := meta.StringPool
	impl := make([]int, 0, len(implementedBy))
	for _, n := range implementedBy {
		impl = append(impl, sp.Get(n))
	}
	meta.Packages[pkg] = &metadata.Package{
		Files: map[string]*metadata.File{
			pkg + ".go": {Types: map[string]*metadata.Type{
				name: {Name: sp.Get(name), Pkg: sp.Get(pkg), Kind: sp.Get("interface"), ImplementedBy: impl},
			}},
		},
	}
}

func TestInterfaceImplementers(t *testing.T) {
	meta := pmcTestMeta()
	// Use a dotted import path to prove the last-dot split (not strings.Split
	// with len==2, which the older inline resolver gets wrong).
	hfInterfaceType(meta, "api", "Handlers",
		"github.com/x/y/handlers.userHandlers", // dotted path
		"nodothere",                            // no dot -> skipped
		"trailing.",                            // trailing dot -> skipped
	)

	got := interfaceImplementers(meta, "api", "Handlers")
	require.Len(t, got, 1)
	assert.Equal(t, "github.com/x/y/handlers", got[0].pkg)
	assert.Equal(t, "userHandlers", got[0].typ)

	// Guards.
	assert.Nil(t, interfaceImplementers(meta, "missing", "Handlers"))
	assert.Nil(t, interfaceImplementers(meta, "api", "Nope"))

	// Non-interface kind -> nil.
	sp := meta.StringPool
	meta.Packages["s"] = &metadata.Package{Files: map[string]*metadata.File{
		"s.go": {Types: map[string]*metadata.Type{"T": {Name: sp.Get("T"), Kind: sp.Get("struct")}}},
	}}
	assert.Nil(t, interfaceImplementers(meta, "s", "T"))
}

func TestParentFunctionEdges(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	// An edge whose ParentFunction is (handlers, Create, *userHandlers).
	edge := buildCallGraphEdge(meta, "FuncLit:x", "handlers", "Bind", "echo", nil)
	edge.ParentFunction = &metadata.Call{
		Meta: meta, Name: sp.Get("Create"), Pkg: sp.Get("handlers"), RecvType: sp.Get("*userHandlers"),
	}
	// An edge with no ParentFunction must be skipped by the index builder.
	noPF := buildCallGraphEdge(meta, "other", "handlers", "Noise", "echo", nil)
	meta.ParentFunctions["k1"] = []*metadata.CallGraphEdge{edge, noPF}

	tree := NewTrackerTree(meta, hfLimits())

	// recvBare is matched with the leading '*' stripped on both sides.
	got := tree.parentFunctionEdges(meta, "handlers", "Create", "userHandlers")
	require.Len(t, got, 1)
	assert.Same(t, edge, got[0])

	// Second call hits the cached index.
	assert.Len(t, tree.parentFunctionEdges(meta, "handlers", "Create", "*userHandlers"), 1)

	// A non-matching key yields nothing.
	assert.Empty(t, tree.parentFunctionEdges(meta, "handlers", "Get", "userHandlers"))
}

// TestNewTrackerNode_InterfaceMethodDottedPath proves the interface-method
// resolver attaches the concrete implementation's method edges even when the
// implementer's import path contains dots (e.g. github.com/...). The previous
// inline strings.Split(name, ".") with a len==2 guard silently skipped these.
func TestNewTrackerNode_InterfaceMethodDottedPath(t *testing.T) {
	meta := pmcTestMeta()
	meta.Callers = map[string][]*metadata.CallGraphEdge{}
	sp := meta.StringPool

	// Interface api.Svc, implemented by impl.concreteSvc — both under a dotted
	// module path.
	const apiPkg = "github.com/x/api"
	const implPkg = "github.com/x/impl"
	meta.Packages[apiPkg] = &metadata.Package{Files: map[string]*metadata.File{
		"api.go": {Types: map[string]*metadata.Type{
			"Svc": {Name: sp.Get("Svc"), Pkg: sp.Get(apiPkg), Kind: sp.Get("interface"),
				ImplementedBy: []int{sp.Get(implPkg + ".concreteSvc")}},
		}},
	}}
	meta.Packages[implPkg] = &metadata.Package{Files: map[string]*metadata.File{
		"impl.go": {Types: map[string]*metadata.Type{
			"concreteSvc": {Name: sp.Get("concreteSvc"), Pkg: sp.Get(implPkg),
				Methods: []metadata.Method{{Name: sp.Get("Do")}}},
		}},
	}}

	// The concrete method's body call, keyed by its method base ID.
	concreteEdge := buildCallGraphEdge(meta, "Do", implPkg, "writeResponse", implPkg, nil)
	meta.Callers[implPkg+".concreteSvc.Do"] = []*metadata.CallGraphEdge{concreteEdge}

	// A node whose callee is the *interface* method api.Svc.Do.
	parentEdge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: sp.Get("handler"), Pkg: sp.Get("main")},
		Callee: metadata.Call{Meta: meta, Name: sp.Get("Do"), Pkg: sp.Get(apiPkg), RecvType: sp.Get("Svc")},
	}

	tree := NewTrackerTree(meta, hfLimits())
	node := NewTrackerNode(tree, meta, "", apiPkg+".Svc.Do", parentEdge, nil, map[string]int{}, &assigmentIndexMap{}, hfLimits())
	require.NotNil(t, node)

	found := false
	for _, child := range node.Children {
		if child.CallGraphEdge == concreteEdge {
			found = true
		}
	}
	assert.True(t, found, "concrete implementation's method edge attached despite dotted import path")
}

func TestAttachReturnedClosureBody_Guards(t *testing.T) {
	meta := pmcTestMeta()
	tree := NewTrackerTree(meta, hfLimits())
	arg := buildTrackerNode(buildCallGraphEdge(meta, "register", "main", "POST", "echo", nil))

	// No panic, no children for any guard-tripping input.
	tree.attachReturnedClosureBody(meta, nil, "api.Handlers", "Create", "api", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	tree.attachReturnedClosureBody(meta, arg, "api.Handlers", "", "api", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	tree.attachReturnedClosureBody(meta, arg, "api.Handlers", "Create", "", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	// recvDecl that strips to empty ("*" alone) -> early return.
	tree.attachReturnedClosureBody(meta, arg, "*", "Create", "api", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	assert.Empty(t, arg.GetChildren())
}

func TestAttachReturnedClosureBody_AttachesClosureAndDedups(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool

	// api.Handlers (interface) implemented by handlers.userHandlers.
	hfInterfaceType(meta, "api", "Handlers", "handlers.userHandlers")

	// A closure-body call: Caller is the func literal, ParentFunction is the
	// factory method Create on *userHandlers.
	closureEdge := buildCallGraphEdge(meta, "FuncLit:handlers.go:1:1", "handlers", "Bind", "echo", nil)
	closureEdge.ParentFunction = &metadata.Call{
		Meta: meta, Name: sp.Get("Create"), Pkg: sp.Get("handlers"), RecvType: sp.Get("*userHandlers"),
	}
	meta.ParentFunctions["pf"] = []*metadata.CallGraphEdge{closureEdge}

	tree := NewTrackerTree(meta, hfLimits())
	arg := buildTrackerNode(buildCallGraphEdge(meta, "RegisterRoutes", "api", "POST", "echo", nil))

	// The receiver's declared type is the interface; the closure lives on the
	// concrete implementer in a different package.
	tree.attachReturnedClosureBody(meta, arg, "api.Handlers", "Create", "api", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	require.Len(t, arg.GetChildren(), 1, "closure body attached via interface implementer")

	// A second attach of the same (pkg, method, type) is deduped.
	tree.attachReturnedClosureBody(meta, arg, "api.Handlers", "Create", "api", map[string]int{}, &assigmentIndexMap{}, hfLimits())
	assert.Len(t, arg.GetChildren(), 1, "closureAttached dedupes a repeated factory body")
}

// TestAttachReturnedClosureBody_DepthCap covers the recursion-depth clamp:
// a generous incoming limit is capped to maxFactoryClosureDepth for the
// closure descent.
func TestAttachReturnedClosureBody_DepthCap(t *testing.T) {
	meta := pmcTestMeta()
	sp := meta.StringPool
	hfInterfaceType(meta, "api", "Handlers", "handlers.userHandlers")
	closureEdge := buildCallGraphEdge(meta, "FuncLit:h:1:1", "handlers", "JSON", "echo", nil)
	closureEdge.ParentFunction = &metadata.Call{
		Meta: meta, Name: sp.Get("Get"), Pkg: sp.Get("handlers"), RecvType: sp.Get("userHandlers"),
	}
	meta.ParentFunctions["pf"] = []*metadata.CallGraphEdge{closureEdge}

	tree := NewTrackerTree(meta, hfLimits())
	arg := buildTrackerNode(buildCallGraphEdge(meta, "RegisterRoutes", "api", "GET", "echo", nil))

	big := metadata.TrackerLimits{MaxRecursionDepth: 50, MaxNodesPerTree: 1000, MaxChildrenPerNode: 100}
	tree.attachReturnedClosureBody(meta, arg, "Handlers", "Get", "api", map[string]int{}, &assigmentIndexMap{}, big)
	assert.Len(t, arg.GetChildren(), 1)
}
