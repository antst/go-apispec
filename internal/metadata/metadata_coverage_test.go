// Copyright 2025-2026 Anton Starikov
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

package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestMetadata creates a minimal Metadata suitable for unit tests.
func newTestMetadata() *Metadata {
	return &Metadata{
		StringPool:           NewStringPool(),
		Packages:             make(map[string]*Package),
		CallGraph:            make([]CallGraphEdge, 0),
		Callers:              make(map[string][]*CallGraphEdge),
		Callees:              make(map[string][]*CallGraphEdge),
		Args:                 make(map[string][]*CallGraphEdge),
		ParentFunctions:      make(map[string][]*CallGraphEdge),
		callDepth:            make(map[string]int),
		traceVariableCache:   make(map[string]TraceVariableResult),
		methodLookupCache:    make(map[string]*Method),
		interfaceResolutions: make(map[InterfaceResolutionKey]*InterfaceResolution),
		CurrentModulePath:    "github.com/test/project",
	}
}

// newCallArg is a test helper that creates a CallArgument with kind and type set.
func newCallArg(meta *Metadata, kind, name, typ string) *CallArgument {
	arg := NewCallArgument(meta)
	arg.SetKind(kind)
	arg.SetName(name)
	arg.SetType(typ)
	return arg
}

// --- ClassifyArgument ---

func TestClassifyArgument_Call(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindCall, "fn", "func()")
	assert.Equal(t, ArgTypeFunctionCall, m.ClassifyArgument(arg))
}

func TestClassifyArgument_FuncLit(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindFuncLit, "anon", "func()")
	assert.Equal(t, ArgTypeFunctionCall, m.ClassifyArgument(arg))
}

func TestClassifyArgument_TypeConversion(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindTypeConversion, "conv", "int")
	assert.Equal(t, ArgTypeComplex, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Ident_FuncType(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindIdent, "handler", "func(int) error")
	assert.Equal(t, ArgTypeFunctionCall, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Ident_Variable(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindIdent, "x", "int")
	assert.Equal(t, ArgTypeVariable, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Literal(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindLiteral, "", "string")
	arg.SetValue("hello")
	assert.Equal(t, ArgTypeLiteral, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Selector(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindSelector, "obj.Field", "string")
	assert.Equal(t, ArgTypeSelector, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Unary(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindUnary, "", "*int")
	assert.Equal(t, ArgTypeUnary, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Binary(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindBinary, "", "int")
	assert.Equal(t, ArgTypeBinary, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Index(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindIndex, "", "int")
	assert.Equal(t, ArgTypeIndex, m.ClassifyArgument(arg))
}

func TestClassifyArgument_CompositeLit(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindCompositeLit, "", "MyStruct")
	assert.Equal(t, ArgTypeComposite, m.ClassifyArgument(arg))
}

func TestClassifyArgument_TypeAssert(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindTypeAssert, "", "int")
	assert.Equal(t, ArgTypeTypeAssert, m.ClassifyArgument(arg))
}

func TestClassifyArgument_Default(t *testing.T) {
	m := newTestMetadata()
	arg := newCallArg(m, KindRaw, "", "")
	assert.Equal(t, ArgTypeComplex, m.ClassifyArgument(arg))
}

// --- BuildAssignmentRelationships / GetAssignmentRelationships ---

func TestBuildAssignmentRelationships_Empty(t *testing.T) {
	m := newTestMetadata()
	result := m.BuildAssignmentRelationships()
	assert.Empty(t, result)
}

func TestGetAssignmentRelationships_CachesResult(t *testing.T) {
	m := newTestMetadata()
	// First call builds
	r1 := m.GetAssignmentRelationships()
	assert.NotNil(t, r1)
	// Second call returns cached
	r2 := m.GetAssignmentRelationships()
	assert.Equal(t, r1, r2)
}

func TestBuildAssignmentRelationships_WithEdges(t *testing.T) {
	m := newTestMetadata()
	m.Packages["mypkg"] = &Package{
		Files: map[string]*File{
			"main.go": {
				Functions: map[string]*Function{
					"main": {
						Name: m.StringPool.Get("main"),
						Pkg:  m.StringPool.Get("mypkg"),
						AssignmentMap: map[string][]Assignment{
							"app": {
								{
									VariableName: m.StringPool.Get("app"),
									Pkg:          m.StringPool.Get("mypkg"),
									ConcreteType: m.StringPool.Get("*App"),
									Func:         m.StringPool.Get("main"),
								},
							},
						},
					},
				},
				Types: map[string]*Type{},
			},
		},
	}

	callerCall := Call{
		Meta:     m,
		Name:     m.StringPool.Get("main"),
		Pkg:      m.StringPool.Get("mypkg"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("unexported"),
	}
	calleeCall := Call{
		Meta:     m,
		Name:     m.StringPool.Get("Run"),
		Pkg:      m.StringPool.Get("mypkg"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("exported"),
	}

	edge := CallGraphEdge{
		Caller:            callerCall,
		Callee:            calleeCall,
		CalleeRecvVarName: "app",
		AssignmentMap:     make(map[string][]Assignment),
		meta:              m,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	edge.Caller.buildIdentifier()
	edge.Callee.buildIdentifier()

	m.CallGraph = append(m.CallGraph, edge)
	m.BuildCallGraphMaps()

	result := m.BuildAssignmentRelationships()
	assert.NotEmpty(t, result)
}

func TestBuildAssignmentRelationships_EdgeAssignments(t *testing.T) {
	m := newTestMetadata()

	callerCall := Call{
		Meta:     m,
		Name:     m.StringPool.Get("setup"),
		Pkg:      m.StringPool.Get("pkg"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("unexported"),
	}
	calleeCall := Call{
		Meta:     m,
		Name:     m.StringPool.Get("handler"),
		Pkg:      m.StringPool.Get("pkg"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("unexported"),
	}

	edge := CallGraphEdge{
		Caller: callerCall,
		Callee: calleeCall,
		AssignmentMap: map[string][]Assignment{
			"svc": {
				{
					VariableName: m.StringPool.Get("svc"),
					Pkg:          m.StringPool.Get("pkg"),
					ConcreteType: m.StringPool.Get("*Service"),
					Func:         m.StringPool.Get("setup"),
				},
			},
		},
		meta: m,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	edge.Caller.buildIdentifier()
	edge.Callee.buildIdentifier()

	m.CallGraph = append(m.CallGraph, edge)
	m.BuildCallGraphMaps()

	result := m.BuildAssignmentRelationships()
	assert.NotEmpty(t, result)
}

// --- TraverseCallGraph ---

func TestTraverseCallGraph_EmptyGraph(t *testing.T) {
	m := newTestMetadata()
	count := 0
	m.TraverseCallGraph("pkg.main", func(_ *CallGraphEdge, _ int) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count)
}

func TestTraverseCallGraph_WithEdges(t *testing.T) {
	m := newTestMetadata()

	// Create a simple call chain: main -> handler -> db.Query
	edges := createSimpleCallChain(m, "pkg", "main", "handler", "Query")
	m.CallGraph = edges
	m.BuildCallGraphMaps()

	var visited []string
	m.TraverseCallGraph("pkg.main", func(edge *CallGraphEdge, _ int) bool {
		visited = append(visited, m.StringPool.GetString(edge.Callee.Name))
		return true
	})
	assert.NotEmpty(t, visited)
}

func TestTraverseCallGraph_StopsOnFalse(t *testing.T) {
	m := newTestMetadata()
	edges := createSimpleCallChain(m, "pkg", "main", "a", "b")
	m.CallGraph = edges
	m.BuildCallGraphMaps()

	count := 0
	m.TraverseCallGraph("pkg.main", func(_ *CallGraphEdge, _ int) bool {
		count++
		return false // stop after first
	})
	assert.Equal(t, 1, count)
}

func TestTraverseCallGraph_CycleDetection(t *testing.T) {
	m := newTestMetadata()

	// Create a cycle: A -> B -> A
	edgeAB := makeEdge(m, "pkg", "A", "B")
	edgeBA := makeEdge(m, "pkg", "B", "A")
	m.CallGraph = []CallGraphEdge{edgeAB, edgeBA}
	m.BuildCallGraphMaps()

	count := 0
	m.TraverseCallGraph("pkg.A", func(_ *CallGraphEdge, _ int) bool {
		count++
		return true
	})
	// Should not loop infinitely; visited map stops it
	assert.LessOrEqual(t, count, 2)
}

// --- GetCallDepth ---

func TestGetCallDepth_NoCallees(t *testing.T) {
	m := newTestMetadata()
	depth := m.GetCallDepth("pkg.main")
	assert.Equal(t, 0, depth)
}

func TestGetCallDepth_Cached(t *testing.T) {
	m := newTestMetadata()
	m.callDepth["pkg.func1"] = 3
	assert.Equal(t, 3, m.GetCallDepth("pkg.func1"))
}

func TestGetCallDepth_TraversesUp(t *testing.T) {
	m := newTestMetadata()

	// main calls handler; handler is callee of main
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	// GetCallDepth traverses Callees (who calls this func?) upward
	depth := m.GetCallDepth("pkg.handler")
	assert.GreaterOrEqual(t, depth, 1)
}

// --- GetFunctionsAtDepth ---

func TestGetFunctionsAtDepth_Empty(t *testing.T) {
	m := newTestMetadata()
	result := m.GetFunctionsAtDepth(0)
	assert.Empty(t, result)
}

func TestGetFunctionsAtDepth_ReturnsMatching(t *testing.T) {
	m := newTestMetadata()
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	// main has depth 0 in Callers
	result := m.GetFunctionsAtDepth(0)
	// Result includes edges at depth 0
	assert.NotNil(t, result)
}

// --- IsReachableFrom ---

func TestIsReachableFrom_Same(t *testing.T) {
	m := newTestMetadata()
	assert.True(t, m.IsReachableFrom("pkg.A", "pkg.A"))
}

func TestIsReachableFrom_DirectEdge(t *testing.T) {
	m := newTestMetadata()
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()
	assert.True(t, m.IsReachableFrom("pkg.main", "pkg.handler"))
}

func TestIsReachableFrom_NotReachable(t *testing.T) {
	m := newTestMetadata()
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()
	assert.False(t, m.IsReachableFrom("pkg.handler", "pkg.main"))
}

func TestIsReachableFrom_Transitive(t *testing.T) {
	m := newTestMetadata()
	edges := createSimpleCallChain(m, "pkg", "main", "mid", "end")
	m.CallGraph = edges
	m.BuildCallGraphMaps()
	assert.True(t, m.IsReachableFrom("pkg.main", "pkg.end"))
}

func TestIsReachableFrom_CycleDoesNotHang(t *testing.T) {
	m := newTestMetadata()
	edgeAB := makeEdge(m, "pkg", "A", "B")
	edgeBA := makeEdge(m, "pkg", "B", "A")
	m.CallGraph = []CallGraphEdge{edgeAB, edgeBA}
	m.BuildCallGraphMaps()
	// Should not hang; A -> B -> A (cycle), but C is not reachable
	assert.False(t, m.IsReachableFrom("pkg.A", "pkg.C"))
}

// --- GetCallPath ---

func TestGetCallPath_Direct(t *testing.T) {
	m := newTestMetadata()
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	path := m.GetCallPath("pkg.main", "pkg.handler")
	assert.NotNil(t, path)
	assert.Len(t, path, 1)
}

func TestGetCallPath_NotFound(t *testing.T) {
	m := newTestMetadata()
	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	path := m.GetCallPath("pkg.handler", "pkg.main")
	assert.Nil(t, path)
}

func TestGetCallPath_Transitive(t *testing.T) {
	m := newTestMetadata()
	edges := createSimpleCallChain(m, "pkg", "main", "mid", "end")
	m.CallGraph = edges
	m.BuildCallGraphMaps()

	path := m.GetCallPath("pkg.main", "pkg.end")
	assert.NotNil(t, path)
	assert.Len(t, path, 2)
}

func TestGetCallPath_BacktrackOnDeadEnd(t *testing.T) {
	m := newTestMetadata()
	// main -> A and main -> B -> C
	edgeMA := makeEdge(m, "pkg", "main", "A")
	edgeMB := makeEdge(m, "pkg", "main", "B")
	edgeBC := makeEdge(m, "pkg", "B", "C")
	m.CallGraph = []CallGraphEdge{edgeMA, edgeMB, edgeBC}
	m.BuildCallGraphMaps()

	path := m.GetCallPath("pkg.main", "pkg.C")
	assert.NotNil(t, path)
}

// --- isEmbeddedInterfaceAssignment ---

func TestIsEmbeddedInterfaceAssignment_UnaryAnd(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Handler"},
		Value: &ast.UnaryExpr{
			Op: token.AND,
			X: &ast.CompositeLit{
				Type: &ast.Ident{Name: "myHandler"},
			},
		},
	}
	assert.True(t, isEmbeddedInterfaceAssignment(kv, nil))
}

func TestIsEmbeddedInterfaceAssignment_UnaryAnd_DottedType(t *testing.T) {
	info := &types.Info{
		Uses: make(map[*ast.Ident]types.Object),
	}
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Handler"},
		Value: &ast.UnaryExpr{
			Op: token.AND,
			X: &ast.CompositeLit{
				Type: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "pkg"},
					Sel: &ast.Ident{Name: "Handler"},
				},
			},
		},
	}
	// Contains "." => returns false
	assert.False(t, isEmbeddedInterfaceAssignment(kv, info))
}

func TestIsEmbeddedInterfaceAssignment_CompositeLit(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Svc"},
		Value: &ast.CompositeLit{
			Type: &ast.Ident{Name: "service"},
		},
	}
	assert.True(t, isEmbeddedInterfaceAssignment(kv, nil))
}

func TestIsEmbeddedInterfaceAssignment_CompositeLit_DottedType(t *testing.T) {
	info := &types.Info{
		Uses: make(map[*ast.Ident]types.Object),
	}
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Svc"},
		Value: &ast.CompositeLit{
			Type: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "pkg"},
				Sel: &ast.Ident{Name: "Service"},
			},
		},
	}
	// Contains "." => false
	assert.False(t, isEmbeddedInterfaceAssignment(kv, info))
}

func TestIsEmbeddedInterfaceAssignment_OtherValue(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key:   &ast.Ident{Name: "X"},
		Value: &ast.Ident{Name: "someVar"},
	}
	assert.False(t, isEmbeddedInterfaceAssignment(kv, nil))
}

func TestIsEmbeddedInterfaceAssignment_UnaryNonAnd(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "X"},
		Value: &ast.UnaryExpr{
			Op: token.NOT,
			X:  &ast.Ident{Name: "y"},
		},
	}
	assert.False(t, isEmbeddedInterfaceAssignment(kv, nil))
}

func TestIsEmbeddedInterfaceAssignment_UnaryAnd_NonCompositeLit(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "X"},
		Value: &ast.UnaryExpr{
			Op: token.AND,
			X:  &ast.Ident{Name: "y"},
		},
	}
	assert.False(t, isEmbeddedInterfaceAssignment(kv, nil))
}

func TestIsEmbeddedInterfaceAssignment_CompositeLit_NilType(t *testing.T) {
	kv := &ast.KeyValueExpr{
		Key:   &ast.Ident{Name: "X"},
		Value: &ast.CompositeLit{},
	}
	// getTypeName on nil returns ""
	assert.False(t, isEmbeddedInterfaceAssignment(kv, nil))
}

// --- registerEmbeddedInterfaceResolution ---

func TestRegisterEmbeddedInterfaceResolution_UnaryAnd(t *testing.T) {
	m := newTestMetadata()
	fset := token.NewFileSet()
	fset.AddFile("test.go", -1, 100)

	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Handler"},
		Value: &ast.UnaryExpr{
			Op: token.AND,
			X: &ast.CompositeLit{
				Type: &ast.Ident{Name: "myHandler"},
			},
		},
	}
	registerEmbeddedInterfaceResolution(kv, "Config", "mypkg", m, nil, fset)

	assert.NotEmpty(t, m.interfaceResolutions)
	key := InterfaceResolutionKey{
		InterfaceType: "Handler",
		StructType:    "Config",
		Pkg:           "mypkg",
	}
	res, ok := m.interfaceResolutions[key]
	assert.True(t, ok)
	assert.Equal(t, "*myHandler", res.ConcreteType)
}

func TestRegisterEmbeddedInterfaceResolution_CompositeLit(t *testing.T) {
	m := newTestMetadata()
	fset := token.NewFileSet()
	fset.AddFile("test.go", -1, 100)

	kv := &ast.KeyValueExpr{
		Key: &ast.Ident{Name: "Svc"},
		Value: &ast.CompositeLit{
			Type: &ast.Ident{Name: "service"},
		},
	}
	registerEmbeddedInterfaceResolution(kv, "App", "pkg", m, nil, fset)
	assert.NotEmpty(t, m.interfaceResolutions)
}

func TestRegisterEmbeddedInterfaceResolution_NonIdentKey(t *testing.T) {
	m := newTestMetadata()
	fset := token.NewFileSet()
	fset.AddFile("test.go", -1, 100)

	// Key is not *ast.Ident => early return
	kv := &ast.KeyValueExpr{
		Key: &ast.BasicLit{Kind: token.STRING, Value: `"key"`},
		Value: &ast.CompositeLit{
			Type: &ast.Ident{Name: "svc"},
		},
	}
	registerEmbeddedInterfaceResolution(kv, "App", "pkg", m, nil, fset)
	assert.Empty(t, m.interfaceResolutions)
}

func TestRegisterEmbeddedInterfaceResolution_OtherValueType(t *testing.T) {
	m := newTestMetadata()
	fset := token.NewFileSet()
	fset.AddFile("test.go", -1, 100)

	kv := &ast.KeyValueExpr{
		Key:   &ast.Ident{Name: "X"},
		Value: &ast.Ident{Name: "someVar"},
	}
	registerEmbeddedInterfaceResolution(kv, "S", "pkg", m, nil, fset)
	// concreteType is "" so nothing is registered
	assert.Empty(t, m.interfaceResolutions)
}

// --- extractParamsAndTypeParams ---

func TestExtractParamsAndTypeParams_SimpleCall(t *testing.T) {
	src := `package test
import "fmt"
func main() {
	fmt.Println("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	require.NoError(t, err)

	conf := types.Config{Error: func(_ error) {}}
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Uses:      make(map[*ast.Ident]types.Object),
		Defs:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}
	_, _ = conf.Check("test", fset, []*ast.File{file}, info)

	m := newTestMetadata()
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
			return false
		}
		return true
	})

	if callExpr != nil {
		paramArgMap := make(map[string]CallArgument)
		typeParamMap := make(map[string]string)
		args := make([]*CallArgument, len(callExpr.Args))
		for i, a := range callExpr.Args {
			args[i] = ExprToCallArgument(a, info, "test", fset, m)
		}
		// Should not panic; may or may not populate maps depending on type resolution
		extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	}
}

func TestExtractParamsAndTypeParams_NilFuncObj(t *testing.T) {
	// Call with a function literal where funcObj is nil
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Uses:      make(map[*ast.Ident]types.Object),
		Defs:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Create a call expression where funcObj would be nil
	callExpr := &ast.CallExpr{
		Fun:  &ast.FuncLit{Type: &ast.FuncType{}}, // FuncLit won't match any case in the switch
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	// Should not panic
	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
	assert.Empty(t, paramArgMap)
	assert.Empty(t, typeParamMap)
}

func TestExtractParamsAndTypeParams_IndexExpr(_ *testing.T) {
	// Tests the IndexExpr branch (Func[T]() style calls)
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Uses:      make(map[*ast.Ident]types.Object),
		Defs:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     &ast.Ident{Name: "MyFunc"},
			Index: &ast.Ident{Name: "int"},
		},
		Args: []ast.Expr{},
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
	// No panic = success; funcObj will be nil since we don't have real type info
}

func TestExtractParamsAndTypeParams_IndexListExpr(_ *testing.T) {
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Uses:      make(map[*ast.Ident]types.Object),
		Defs:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexListExpr{
			X:       &ast.Ident{Name: "MyFunc"},
			Indices: []ast.Expr{&ast.Ident{Name: "int"}, &ast.Ident{Name: "string"}},
		},
		Args: []ast.Expr{},
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

// --- isExternalPackage ---

func TestIsExternalPackage_StdLib(t *testing.T) {
	// Standard library (no "/" or ".") => not external
	assert.False(t, isExternalPackage("fmt", "github.com/test/project"))
}

func TestIsExternalPackage_StdLibNoSlash(t *testing.T) {
	assert.False(t, isExternalPackage("strings", "github.com/test/project"))
}

func TestIsExternalPackage_InternalPackage(t *testing.T) {
	assert.False(t, isExternalPackage("github.com/test/project/internal/svc", "github.com/test/project"))
}

func TestIsExternalPackage_ExternalPackage(t *testing.T) {
	assert.True(t, isExternalPackage("github.com/other/lib", "github.com/test/project"))
}

func TestIsExternalPackage_EmptyModule(t *testing.T) {
	// With empty module path, HasPrefix("...", "") is always true,
	// so it's considered internal (not external)
	assert.False(t, isExternalPackage("github.com/foo/bar", ""))
}

// --- isExternalType ---

func TestIsExternalType_Nil(t *testing.T) {
	// Passing a nil type
	assert.False(t, isExternalType(nil, "github.com/test"))
}

func TestIsExternalType_BasicType(t *testing.T) {
	// Basic types are not external
	bt := types.Typ[types.Int]
	assert.False(t, isExternalType(bt, "github.com/test"))
}

func TestIsExternalType_Pointer(t *testing.T) {
	bt := types.Typ[types.Int]
	pt := types.NewPointer(bt)
	assert.False(t, isExternalType(pt, "github.com/test"))
}

func TestIsExternalType_Slice(t *testing.T) {
	bt := types.Typ[types.String]
	sl := types.NewSlice(bt)
	assert.False(t, isExternalType(sl, "github.com/test"))
}

func TestIsExternalType_Array(t *testing.T) {
	bt := types.Typ[types.Int]
	arr := types.NewArray(bt, 10)
	assert.False(t, isExternalType(arr, "github.com/test"))
}

func TestIsExternalType_Map(t *testing.T) {
	keyT := types.Typ[types.String]
	valT := types.Typ[types.Int]
	mp := types.NewMap(keyT, valT)
	assert.False(t, isExternalType(mp, "github.com/test"))
}

func TestIsExternalType_Chan(t *testing.T) {
	elemT := types.Typ[types.Int]
	ch := types.NewChan(types.SendRecv, elemT)
	assert.False(t, isExternalType(ch, "github.com/test"))
}

func TestIsExternalType_Named_NilPkg(t *testing.T) {
	// Named type with no package (like error interface)
	// Create a named type object without a package
	obj := types.NewTypeName(token.NoPos, nil, "error", nil)
	named := types.NewNamed(obj, types.Typ[types.String].Underlying(), nil)
	assert.False(t, isExternalType(named, "github.com/test"))
}

func TestIsExternalType_Named_WithPkg_Internal(t *testing.T) {
	pkg := types.NewPackage("github.com/test/internal/model", "model")
	obj := types.NewTypeName(token.NoPos, pkg, "User", nil)
	named := types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	assert.False(t, isExternalType(named, "github.com/test"))
}

func TestIsExternalType_Named_WithPkg_External(t *testing.T) {
	pkg := types.NewPackage("github.com/other/lib", "lib")
	obj := types.NewTypeName(token.NoPos, pkg, "Widget", nil)
	named := types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	assert.True(t, isExternalType(named, "github.com/test"))
}

// --- processTypeKind ---

func TestProcessTypeKind_Struct(t *testing.T) {
	m := newTestMetadata()
	allTypes := make(map[string]*Type)
	typ := &Type{}

	tspec := &ast.TypeSpec{
		Name: &ast.Ident{Name: "MyStruct"},
		Type: &ast.StructType{
			Fields: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{{Name: "Name"}},
						Type:  &ast.Ident{Name: "string"},
					},
				},
			},
		},
	}

	fset := token.NewFileSet()
	processTypeKind(tspec, nil, "pkg", fset, typ, allTypes, m)
	assert.Equal(t, "struct", m.StringPool.GetString(typ.Kind))
	assert.Contains(t, allTypes, "MyStruct")
}

func TestProcessTypeKind_Interface(t *testing.T) {
	m := newTestMetadata()
	allTypes := make(map[string]*Type)
	typ := &Type{}

	tspec := &ast.TypeSpec{
		Name: &ast.Ident{Name: "MyInterface"},
		Type: &ast.InterfaceType{
			Methods: &ast.FieldList{},
		},
	}

	fset := token.NewFileSet()
	processTypeKind(tspec, nil, "pkg", fset, typ, allTypes, m)
	assert.Equal(t, "interface", m.StringPool.GetString(typ.Kind))
	assert.Contains(t, allTypes, "MyInterface")
}

func TestProcessTypeKind_Alias(t *testing.T) {
	m := newTestMetadata()
	allTypes := make(map[string]*Type)
	typ := &Type{}

	tspec := &ast.TypeSpec{
		Name: &ast.Ident{Name: "MyAlias"},
		Type: &ast.Ident{Name: "string"},
	}

	fset := token.NewFileSet()
	processTypeKind(tspec, nil, "pkg", fset, typ, allTypes, m)
	assert.Equal(t, "alias", m.StringPool.GetString(typ.Kind))
	assert.Equal(t, "string", m.StringPool.GetString(typ.Target))
	assert.Contains(t, allTypes, "MyAlias")
}

func TestProcessTypeKind_Other(t *testing.T) {
	m := newTestMetadata()
	allTypes := make(map[string]*Type)
	typ := &Type{}

	// A FuncType is "other"
	tspec := &ast.TypeSpec{
		Name: &ast.Ident{Name: "HandlerFunc"},
		Type: &ast.FuncType{},
	}

	fset := token.NewFileSet()
	processTypeKind(tspec, nil, "pkg", fset, typ, allTypes, m)
	assert.Equal(t, "other", m.StringPool.GetString(typ.Kind))
	assert.Contains(t, allTypes, "HandlerFunc")
}

// --- helpers for test setup ---

func makeEdge(m *Metadata, pkg, callerName, calleeName string) CallGraphEdge {
	edge := CallGraphEdge{
		Caller: Call{
			Meta:     m,
			Name:     m.StringPool.Get(callerName),
			Pkg:      m.StringPool.Get(pkg),
			Position: -1,
			RecvType: -1,
			Scope:    m.StringPool.Get(getScope(callerName)),
		},
		Callee: Call{
			Meta:     m,
			Name:     m.StringPool.Get(calleeName),
			Pkg:      m.StringPool.Get(pkg),
			Position: -1,
			RecvType: -1,
			Scope:    m.StringPool.Get(getScope(calleeName)),
		},
		AssignmentMap: make(map[string][]Assignment),
		meta:          m,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	edge.Caller.buildIdentifier()
	edge.Callee.buildIdentifier()
	return edge
}

func createSimpleCallChain(m *Metadata, pkg, first, second, third string) []CallGraphEdge {
	e1 := makeEdge(m, pkg, first, second)
	e2 := makeEdge(m, pkg, second, third)
	return []CallGraphEdge{e1, e2}
}
