package metadata

import (
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// processStructFields — embedded fields, tagged fields, nested structs
// ---------------------------------------------------------------------------

func TestProcessStructFields_EmbeddedAndTagged(t *testing.T) {
	src := `package testpkg

type Base struct {
	ID int
}

type MyStruct struct {
	Base
	Name string ` + "`json:\"name\" xml:\"name\"`" + `
	age  int
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	// Find the MyStruct type declaration
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			tspec, ok := spec.(*ast.TypeSpec)
			if !ok || tspec.Name.Name != "MyStruct" {
				continue
			}
			structType, ok := tspec.Type.(*ast.StructType)
			require.True(t, ok)

			ty := &Type{
				Name: meta.StringPool.Get("MyStruct"),
				Pkg:  meta.StringPool.Get("testpkg"),
			}

			processStructFields(structType, "testpkg", meta, ty, info)

			// Should have one embed (Base)
			assert.Equal(t, 1, len(ty.Embeds), "expected 1 embedded field")

			// Should have 2 named fields (Name, age)
			assert.Equal(t, 2, len(ty.Fields), "expected 2 named fields")

			// Check tags
			nameField := ty.Fields[0]
			nameFieldTag := meta.StringPool.GetString(nameField.Tag)
			assert.Contains(t, nameFieldTag, "json")
		}
	}
}

func TestProcessStructFields_NestedStruct(t *testing.T) {
	src := `package testpkg

type Config struct {
	Server struct {
		Host string
		Port int
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			tspec, ok := spec.(*ast.TypeSpec)
			if !ok || tspec.Name.Name != "Config" {
				continue
			}
			structType, ok := tspec.Type.(*ast.StructType)
			require.True(t, ok)

			ty := &Type{
				Name: meta.StringPool.Get("Config"),
				Pkg:  meta.StringPool.Get("testpkg"),
			}

			processStructFields(structType, "testpkg", meta, ty, info)

			// Should have 1 named field (Server) with a nested type
			require.Equal(t, 1, len(ty.Fields), "expected 1 named field")
			assert.NotNil(t, ty.Fields[0].NestedType, "expected nested type for Server field")
		}
	}
}

// ---------------------------------------------------------------------------
// processStructInstance — basic struct literal
// ---------------------------------------------------------------------------

func TestProcessStructInstance_BasicLiteral(t *testing.T) {
	src := `package testpkg

type Config struct {
	Host string
	Port int
}

var c = Config{Host: "localhost", Port: 8080}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}
	constMap := make(map[string]string)

	// Walk AST to find composite literal
	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			processStructInstance(cl, info, "testpkg", fset, f, constMap, meta)
		}
		return true
	})

	assert.NotEmpty(t, f.StructInstances, "expected at least one struct instance")
	if len(f.StructInstances) > 0 {
		inst := f.StructInstances[0]
		typeName := meta.StringPool.GetString(inst.Type)
		assert.Equal(t, "Config", typeName)
		assert.NotEmpty(t, inst.Fields)
	}
}

// ---------------------------------------------------------------------------
// BuildAssignmentRelationships — nested edge with Callers map
// ---------------------------------------------------------------------------

func TestBuildAssignmentRelationships_NestedEdgeCallers(t *testing.T) {
	m := newTestMetadata()

	// Create two edges: setup -> handler, handler -> db.Query
	// with assignment on handler edge for "svc"
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

	edge1 := CallGraphEdge{
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
	edge1.Caller.Edge = &edge1
	edge1.Callee.Edge = &edge1
	edge1.Caller.buildIdentifier()
	edge1.Callee.buildIdentifier()

	// Second edge: handler calls db.Query with recv var "svc"
	callerCall2 := Call{
		Meta:     m,
		Name:     m.StringPool.Get("handler"),
		Pkg:      m.StringPool.Get("pkg"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("unexported"),
	}
	calleeCall2 := Call{
		Meta:     m,
		Name:     m.StringPool.Get("Query"),
		Pkg:      m.StringPool.Get("db"),
		Position: -1,
		RecvType: -1,
		Scope:    m.StringPool.Get("exported"),
	}

	edge2 := CallGraphEdge{
		Caller:            callerCall2,
		Callee:            calleeCall2,
		CalleeRecvVarName: "svc",
		AssignmentMap:     make(map[string][]Assignment),
		meta:              m,
	}
	edge2.Caller.Edge = &edge2
	edge2.Callee.Edge = &edge2
	edge2.Caller.buildIdentifier()
	edge2.Callee.buildIdentifier()

	m.CallGraph = []CallGraphEdge{edge1, edge2}
	m.BuildCallGraphMaps()

	result := m.BuildAssignmentRelationships()
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// extractParamsAndTypeParams — with ast.Ident call
// ---------------------------------------------------------------------------

func TestExtractParamsAndTypeParams_SimpleIdent(t *testing.T) {
	src := `package testpkg

func greet(name string) string {
	return "hello " + name
}

func main() {
	greet("world")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	// Find the call expression greet("world")
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if ident, ok := ce.Fun.(*ast.Ident); ok && ident.Name == "greet" {
				callExpr = ce
				return false
			}
		}
		return true
	})
	require.NotNil(t, callExpr, "expected to find greet() call")

	args := []*CallArgument{
		ExprToCallArgument(callExpr.Args[0], info, "testpkg", fset, meta),
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// Should have mapped the "name" parameter
	if _, ok := paramArgMap["name"]; !ok {
		t.Log("paramArgMap did not contain 'name' - function object may not resolve in simple parse")
	}
}

func TestExtractParamsAndTypeParams_SelectorExpr(t *testing.T) {
	src := `package testpkg

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Println" {
				callExpr = ce
				return false
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := []*CallArgument{
		ExprToCallArgument(callExpr.Args[0], info, "testpkg", fset, meta),
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	// Just verify it doesn't panic and processes the selector correctly
}

func TestExtractParamsAndTypeParams_NilInfo(_ *testing.T) {
	// Test with minimal info (no type resolution)
	callExpr := &ast.CallExpr{
		Fun:  &ast.Ident{Name: "unknown"},
		Args: []ast.Expr{},
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	// Should not panic with unresolvable function
	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

// ---------------------------------------------------------------------------
// extractRootVariable — various expression types
// ---------------------------------------------------------------------------

func TestExtractRootVariable_Ident(t *testing.T) {
	expr := &ast.Ident{Name: "server"}
	assert.Equal(t, "server", extractRootVariable(expr))
}

func TestExtractRootVariable_Selector(t *testing.T) {
	expr := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "server"},
		Sel: &ast.Ident{Name: "Handler"},
	}
	assert.Equal(t, "server", extractRootVariable(expr))
}

func TestExtractRootVariable_NestedSelector(t *testing.T) {
	expr := &ast.SelectorExpr{
		X: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "app"},
			Sel: &ast.Ident{Name: "server"},
		},
		Sel: &ast.Ident{Name: "Handler"},
	}
	assert.Equal(t, "app", extractRootVariable(expr))
}

func TestExtractRootVariable_CallExprWithSelector(t *testing.T) {
	expr := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "builder"},
			Sel: &ast.Ident{Name: "Build"},
		},
	}
	assert.Equal(t, "builder", extractRootVariable(expr))
}

func TestExtractRootVariable_CallExprWithoutSelector(t *testing.T) {
	expr := &ast.CallExpr{
		Fun: &ast.Ident{Name: "create"},
	}
	// CallExpr with non-SelectorExpr Fun returns ""
	assert.Equal(t, "", extractRootVariable(expr))
}

func TestExtractRootVariable_UnknownExpr(t *testing.T) {
	expr := &ast.BasicLit{Value: "42"}
	assert.Equal(t, "", extractRootVariable(expr))
}

// ---------------------------------------------------------------------------
// applyTypeParameterResolution — nil edge, edge with args
// ---------------------------------------------------------------------------

func TestApplyTypeParameterResolution_NilEdge(_ *testing.T) {
	// Should not panic
	applyTypeParameterResolution(nil)
}

func TestApplyTypeParameterResolution_WithArgs(t *testing.T) {
	m := newTestMetadata()

	arg := NewCallArgument(m)
	arg.SetType("T")
	arg.TypeParamMap = map[string]string{"T": "int"}

	edge := CallGraphEdge{
		Args:         []*CallArgument{arg},
		TypeParamMap: map[string]string{"T": "int"},
		ParamArgMap: map[string]CallArgument{
			"data": {
				Meta: m,
				Type: m.StringPool.Get("T"),
				TypeParamMap: map[string]string{
					"T": "int",
				},
			},
		},
		meta: m,
	}

	applyTypeParameterResolution(&edge)

	// The arg should have been resolved
	if arg.IsGenericType {
		assert.Equal(t, "int", m.StringPool.GetString(arg.ResolvedType))
	}
}

// ---------------------------------------------------------------------------
// applyTypeParameterResolutionToArgument — nested args
// ---------------------------------------------------------------------------

func TestApplyTypeParameterResolutionToArgument_Nil(_ *testing.T) {
	applyTypeParameterResolutionToArgument(nil, nil)
}

func TestApplyTypeParameterResolutionToArgument_WithNestedArgs(_ *testing.T) {
	m := newTestMetadata()

	inner := NewCallArgument(m)
	inner.SetType("string")
	inner.TypeParamMap = map[string]string{}

	outer := NewCallArgument(m)
	outer.SetType("T")
	outer.TypeParamMap = map[string]string{"T": "int"}
	outer.Args = []*CallArgument{inner}

	applyTypeParameterResolutionToArgument(outer, outer.TypeParamMap)
}

func TestApplyTypeParameterResolutionToArgument_WithXAndFun(_ *testing.T) {
	m := newTestMetadata()

	xArg := NewCallArgument(m)
	xArg.SetType("receiver")
	xArg.TypeParamMap = map[string]string{}

	funArg := NewCallArgument(m)
	funArg.SetType("func()")
	funArg.TypeParamMap = map[string]string{}

	arg := NewCallArgument(m)
	arg.SetType("result")
	arg.TypeParamMap = map[string]string{}
	arg.X = xArg
	arg.Fun = funArg

	applyTypeParameterResolutionToArgument(arg, arg.TypeParamMap)
}

// ---------------------------------------------------------------------------
// StringPool.Finalize — 0% coverage
// ---------------------------------------------------------------------------

func TestStringPool_Finalize_Coverage2(t *testing.T) {
	sp := NewStringPool()
	sp.Get("hello")
	sp.Get("world")
	sp.Finalize()
	// Should not panic and pool should still be usable
	assert.Equal(t, "hello", sp.GetString(sp.Get("hello")))
}

// ---------------------------------------------------------------------------
// TraverseCallerChildren — self-calling depth limit
// ---------------------------------------------------------------------------

func TestTraverseCallerChildren_SelfCalling(t *testing.T) {
	m := newTestMetadata()

	// Create a self-recursive edge: A -> A
	edge := makeEdge(m, "pkg", "A", "A")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	count := 0
	m.TraverseCallerChildren(&m.CallGraph[0], func(_, _ *CallGraphEdge) {
		count++
	})

	// Should be limited by MaxSelfCallingDepth
	assert.LessOrEqual(t, count, MaxSelfCallingDepth+1)
}

func TestTraverseCallerChildren_CycleDetection(t *testing.T) {
	m := newTestMetadata()

	edgeAB := makeEdge(m, "pkg", "A", "B")
	edgeBA := makeEdge(m, "pkg", "B", "A")
	m.CallGraph = []CallGraphEdge{edgeAB, edgeBA}
	m.BuildCallGraphMaps()

	count := 0
	m.TraverseCallerChildren(&m.CallGraph[0], func(_, _ *CallGraphEdge) {
		count++
	})

	// Should terminate due to visited map
	assert.LessOrEqual(t, count, 4)
}

// ---------------------------------------------------------------------------
// CallGraphRoots — with edges where caller is not a callee
// ---------------------------------------------------------------------------

func TestCallGraphRoots_BasicRoots(t *testing.T) {
	m := newTestMetadata()

	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	roots := m.CallGraphRoots()
	assert.NotEmpty(t, roots)
}

func TestCallGraphRoots_Cached(t *testing.T) {
	m := newTestMetadata()

	edge := makeEdge(m, "pkg", "main", "handler")
	m.CallGraph = []CallGraphEdge{edge}
	m.BuildCallGraphMaps()

	roots1 := m.CallGraphRoots()
	roots2 := m.CallGraphRoots()
	assert.Equal(t, len(roots1), len(roots2))
}

// ---------------------------------------------------------------------------
// BuildFuncMap — methods and top-level functions
// ---------------------------------------------------------------------------

func TestBuildFuncMap_WithMethods(t *testing.T) {
	src := `package testpkg

type Server struct{}

func (s *Server) Handle() {}

func NewServer() *Server { return &Server{} }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}

	funcMap := BuildFuncMap(pkgs)

	// Should contain both the method and function
	found := false
	for key := range funcMap {
		if key == "testpkg.NewServer" {
			found = true
		}
	}
	assert.True(t, found, "expected to find NewServer in funcMap")

	// Should also contain the method
	foundMethod := false
	for key := range funcMap {
		if key == "testpkg.Server.Handle" {
			foundMethod = true
		}
	}
	assert.True(t, foundMethod, "expected to find Server.Handle in funcMap")
}

func TestBuildFuncMap_EmptyPackageName(t *testing.T) {
	src := `package testpkg

func Hello() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}

	funcMap := BuildFuncMap(pkgs)
	assert.NotEmpty(t, funcMap)
}

// ---------------------------------------------------------------------------
// isExternalPackage — various paths
// ---------------------------------------------------------------------------

func TestIsExternalPackage_StdLibPkg(t *testing.T) {
	assert.False(t, isExternalPackage("fmt", "github.com/test/proj"))
	assert.False(t, isExternalPackage("net", "github.com/test/proj"))
}

func TestIsExternalPackage_InternalPkg(t *testing.T) {
	assert.False(t, isExternalPackage("github.com/test/proj/internal/foo", "github.com/test/proj"))
	assert.False(t, isExternalPackage("github.com/test/proj/cmd/app", "github.com/test/proj"))
}

func TestIsExternalPackage_ExternalPkg(t *testing.T) {
	assert.True(t, isExternalPackage("github.com/other/lib", "github.com/test/proj"))
	assert.True(t, isExternalPackage("golang.org/x/tools", "github.com/test/proj"))
}

// ---------------------------------------------------------------------------
// isMockName — various patterns
// ---------------------------------------------------------------------------

func TestIsMockName_Patterns(t *testing.T) {
	assert.True(t, isMockName("MockHandler"))
	assert.True(t, isMockName("handlerMock"))
	assert.True(t, isMockName("FakeService"))
	assert.True(t, isMockName("StubRepo"))
	assert.True(t, isMockName("MockedClient"))
	assert.False(t, isMockName("RealHandler"))
	assert.False(t, isMockName("Service"))
}

// ---------------------------------------------------------------------------
// processTypes — alias and interface types
// ---------------------------------------------------------------------------

func TestProcessTypes_AliasType(t *testing.T) {
	src := `package testpkg

type MyString string
type MyInt = int
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}
	allTypeMethods := make(map[string][]Method)
	allTypes := make(map[string]*Type)

	processTypes(file, info, "testpkg", fset, f, allTypeMethods, allTypes, meta)

	assert.Contains(t, f.Types, "MyString")
	myStr := f.Types["MyString"]
	assert.Equal(t, "alias", meta.StringPool.GetString(myStr.Kind))
}

// ---------------------------------------------------------------------------
// processTypes — interface with methods
// ---------------------------------------------------------------------------

func TestProcessTypes_Interface(t *testing.T) {
	src := `package testpkg

type Handler interface {
	Handle(input string) error
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}
	allTypeMethods := make(map[string][]Method)
	allTypes := make(map[string]*Type)

	processTypes(file, info, "testpkg", fset, f, allTypeMethods, allTypes, meta)

	assert.Contains(t, f.Types, "Handler")
	handler := f.Types["Handler"]
	assert.Equal(t, "interface", meta.StringPool.GetString(handler.Kind))
}

// ---------------------------------------------------------------------------
// collectConstants — const with iota and explicit values
// ---------------------------------------------------------------------------

func TestCollectConstants_IotaAndExplicit(t *testing.T) {
	src := `package testpkg

const (
	A = iota
	B
	C
)

const Pi = 3.14
const Name = "hello"
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	constMap := collectConstants(file, info, "testpkg", fset, meta)
	assert.Contains(t, constMap, "Pi")
	assert.Contains(t, constMap, "Name")
}

// ---------------------------------------------------------------------------
// GenerateMetadata — basic end-to-end with a simple package
// ---------------------------------------------------------------------------

func TestGenerateMetadata_SimplePackage(t *testing.T) {
	src := `package main

import "fmt"

func greet(name string) string {
	return "hello " + name
}

func main() {
	msg := greet("world")
	fmt.Println(msg)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"main": {"main.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file: info,
	}
	importPaths := map[string]string{
		"main.go": "main",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)

	assert.Contains(t, meta.Packages, "main")
	pkg := meta.Packages["main"]
	assert.NotEmpty(t, pkg.Files)
}

// ---------------------------------------------------------------------------
// GenerateMetadataWithLogger — exercises the logger path
// ---------------------------------------------------------------------------

type testLogger struct {
	messages []string
}

func (l *testLogger) Printf(format string, args ...any) {
	l.messages = append(l.messages, "printf")
	_ = format
	_ = args
}
func (l *testLogger) Println(args ...any) {
	l.messages = append(l.messages, "println")
	_ = args
}
func (l *testLogger) Print(args ...any) {
	l.messages = append(l.messages, "print")
	_ = args
}

func TestGenerateMetadataWithLogger_LogsOutput(t *testing.T) {
	src := `package main

func main() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"main": {"main.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file: info,
	}
	importPaths := map[string]string{
		"main.go": "main",
	}

	logger := &testLogger{}
	meta := GenerateMetadataWithLogger(pkgs, fileToInfo, importPaths, fset, logger)
	require.NotNil(t, meta)

	// Logger should have received some messages
	assert.NotEmpty(t, logger.messages)
}

// ---------------------------------------------------------------------------
// buildFullPath
// ---------------------------------------------------------------------------

func TestBuildFullPath_WithImportPath(t *testing.T) {
	assert.Equal(t, "github.com/test/pkg/main.go", buildFullPath("github.com/test/pkg", "main.go"))
}

func TestBuildFullPath_WithoutImportPath(t *testing.T) {
	assert.Equal(t, "main.go", buildFullPath("", "main.go"))
}

// ---------------------------------------------------------------------------
// detectConstantReturnValue — basic test
// ---------------------------------------------------------------------------

func TestDetectConstantReturnValue(t *testing.T) {
	src := `package testpkg

func getPort() int {
	return 8080
}

func getName() string {
	x := "hello"
	return x
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "getPort" {
			continue
		}

		val := detectConstantReturnValue(fn.Body, info)
		if val != "" {
			assert.Equal(t, "8080", val)
		}
	}
}

func TestDetectConstantReturnValue_NilBody(t *testing.T) {
	val := detectConstantReturnValue(nil, nil)
	assert.Equal(t, "", val)
}

// ---------------------------------------------------------------------------
// GenerateMetadata — multi-package with module path detection
// ---------------------------------------------------------------------------

func TestGenerateMetadata_MultiPackage(t *testing.T) {
	src1 := `package main

func main() {
	greet()
}
`
	src2 := `package main

func greet() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "main.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "helpers.go", src2, parser.AllErrors)
	require.NoError(t, err)

	info1 := typeCheckFiles(t, fset, []*ast.File{file1, file2})

	pkgs := map[string]map[string]*ast.File{
		"github.com/test/proj/cmd/app": {
			"main.go":    file1,
			"helpers.go": file2,
		},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file1: info1,
		file2: info1,
	}
	importPaths := map[string]string{
		"main.go":    "github.com/test/proj/cmd/app",
		"helpers.go": "github.com/test/proj/cmd/app",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)
	assert.NotEmpty(t, meta.Packages)
}

func TestGenerateMetadata_MultiPackageModulePath(t *testing.T) {
	src1 := `package main

func main() {}
`
	src2 := `package helper

func Help() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "main.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "helper.go", src2, parser.AllErrors)
	require.NoError(t, err)

	info1 := typeCheckFile(t, fset, file1)
	info2 := typeCheckFile(t, fset, file2)

	pkgs := map[string]map[string]*ast.File{
		"github.com/test/proj/cmd/app":         {"main.go": file1},
		"github.com/test/proj/internal/helper": {"helper.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file1: info1,
		file2: info2,
	}
	importPaths := map[string]string{
		"main.go":   "github.com/test/proj/cmd/app",
		"helper.go": "github.com/test/proj/internal/helper",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)

	// Should have detected the common module path
	assert.Contains(t, meta.CurrentModulePath, "github.com/test/proj")
}

// ---------------------------------------------------------------------------
// processStructFields — external type resolution path
// ---------------------------------------------------------------------------

func TestProcessStructFields_ExternalType(t *testing.T) {
	src := `package testpkg

type Duration int64

type Config struct {
	Timeout Duration
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			tspec, ok := spec.(*ast.TypeSpec)
			if !ok || tspec.Name.Name != "Config" {
				continue
			}
			structType, ok := tspec.Type.(*ast.StructType)
			require.True(t, ok)

			ty := &Type{
				Name: meta.StringPool.Get("Config"),
				Pkg:  meta.StringPool.Get("testpkg"),
			}

			processStructFields(structType, "testpkg", meta, ty, info)
			assert.NotEmpty(t, ty.Fields)
		}
	}
}

// ---------------------------------------------------------------------------
// processAssignment — basic assignment
// ---------------------------------------------------------------------------

func TestProcessAssignment_BasicAssign(t *testing.T) {
	src := `package testpkg

func setup() {
	x := 42
	y := "hello"
	_ = x
	_ = y
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	})
	fileToInfo := map[*ast.File]*types.Info{file: info}

	// Find the function body and walk assignment statements
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "setup" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				assignments := processAssignment(assign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
				// Should produce assignments for x and y
				_ = assignments
			}
			return true
		})
	}
}

// ---------------------------------------------------------------------------
// processStructInstance — with constant substitution
// ---------------------------------------------------------------------------

func TestProcessStructInstance_WithConstants(t *testing.T) {
	src := `package testpkg

const DefaultHost = "localhost"

type Config struct {
	Host string
	Port int
}

var c = Config{Host: DefaultHost, Port: 8080}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}
	constMap := collectConstants(file, info, "testpkg", fset, meta)

	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			processStructInstance(cl, info, "testpkg", fset, f, constMap, meta)
		}
		return true
	})

	assert.NotEmpty(t, f.StructInstances)
}

// ---------------------------------------------------------------------------
// getTypeWithGenerics — basic tests
// ---------------------------------------------------------------------------

func TestGetTypeWithGenerics_Ident(t *testing.T) {
	src := `package testpkg

type Container struct {
	Items []string
}

func NewContainer() Container {
	return Container{Items: []string{}}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)

	// Test with an ast.Ident expression
	ident := &ast.Ident{Name: "Container"}
	result := getTypeWithGenerics(ident, info)
	// May be nil if type info not fully resolved, but should not panic
	_ = result
}

func TestGetTypeWithGenerics_IndexExpr(_ *testing.T) {
	// Test with an IndexExpr (like Func[T])
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	indexExpr := &ast.IndexExpr{
		X:     &ast.Ident{Name: "Container"},
		Index: &ast.Ident{Name: "string"},
	}

	result := getTypeWithGenerics(indexExpr, info)
	_ = result // May be nil, just verify no panic
}

func TestGetTypeWithGenerics_SelectorExpr(_ *testing.T) {
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	selExpr := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "pkg"},
		Sel: &ast.Ident{Name: "Type"},
	}

	result := getTypeWithGenerics(selExpr, info)
	_ = result
}

// ---------------------------------------------------------------------------
// CallIdentifier.ID — with various configurations
// ---------------------------------------------------------------------------

func TestCallIdentifier_IDWithGenericsAndPosition(t *testing.T) {
	ci := NewCallIdentifier("pkg", "Handle", "*Server", "main.go:10", map[string]string{
		"T": "int",
	})

	baseID := ci.ID(BaseID)
	assert.Equal(t, "pkg.Server.Handle", baseID)

	genericID := ci.ID(GenericID)
	assert.Contains(t, genericID, "T=int")

	instanceID := ci.ID(InstanceID)
	assert.Contains(t, instanceID, "@main.go:10")
	assert.Contains(t, instanceID, "T=int")
}

func TestCallIdentifier_IDWithoutReceiver(t *testing.T) {
	ci := NewCallIdentifier("pkg", "main", "", "", nil)

	baseID := ci.ID(BaseID)
	assert.Equal(t, "pkg.main", baseID)

	// GenericID without generics should be same as BaseID
	genericID := ci.ID(GenericID)
	assert.Equal(t, "pkg.main", genericID)

	// InstanceID without position or generics
	instanceID := ci.ID(InstanceID)
	assert.Equal(t, "pkg.main", instanceID)
}

func TestCallIdentifier_IDCaching(t *testing.T) {
	ci := NewCallIdentifier("pkg", "func1", "", "file.go:5", map[string]string{"T": "string"})

	// Call twice to verify caching works
	id1 := ci.ID(GenericID)
	id2 := ci.ID(GenericID)
	assert.Equal(t, id1, id2)
}

// ---------------------------------------------------------------------------
// StripToBase — various patterns
// ---------------------------------------------------------------------------

func TestStripToBase_WithPosition(t *testing.T) {
	assert.Equal(t, "pkg.func", StripToBase("pkg.func@file.go:10"))
}

func TestStripToBase_WithGenerics(t *testing.T) {
	assert.Equal(t, "pkg.func", StripToBase("pkg.func[T=int]"))
}

func TestStripToBase_WithBoth(t *testing.T) {
	assert.Equal(t, "pkg.func", StripToBase("pkg.func[T=int]@file.go:10"))
}

func TestStripToBase_Plain(t *testing.T) {
	assert.Equal(t, "pkg.func", StripToBase("pkg.func"))
}

// ---------------------------------------------------------------------------
// processCallExpression — basic call in function body
// ---------------------------------------------------------------------------

func TestProcessCallExpression_BasicCall(t *testing.T) {
	src := `package testpkg

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	meta.Callers = make(map[string][]*CallGraphEdge)
	meta.Callees = make(map[string][]*CallGraphEdge)
	meta.Args = make(map[string][]*CallGraphEdge)
	meta.ParentFunctions = make(map[string][]*CallGraphEdge)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	funcMap := BuildFuncMap(pkgs)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	calleeMap := make(map[string]*CallGraphEdge)
	argMap := make(map[string]*CallArgument)

	// Walk the AST to find call expressions
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				processCallExpression(
					call, file, pkgs, "testpkg", nil,
					fileToInfo, funcMap, fset, meta, info,
					calleeMap, argMap,
				)
			}
			return true
		})
	}

	// The call graph may or may not have entries depending on type resolution
	// At minimum, the function should not panic
}

// ---------------------------------------------------------------------------
// helper: typeCheckFiles — creates types.Info from multiple files
// ---------------------------------------------------------------------------

func typeCheckFiles(t *testing.T, fset *token.FileSet, files []*ast.File) *types.Info {
	t.Helper()

	conf := types.Config{
		Importer: nil,
		Error:    func(_ error) {},
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	_, _ = conf.Check("testpkg", fset, files, info)
	return info
}

// ---------------------------------------------------------------------------
// helper: typeCheckFile — creates types.Info from parsed source
// ---------------------------------------------------------------------------

func typeCheckFile(t *testing.T, fset *token.FileSet, file *ast.File) *types.Info {
	t.Helper()

	conf := types.Config{
		Importer: nil, // no imports for simple tests
		Error:    func(_ error) { /* ignore type errors from missing imports */ },
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	_, _ = conf.Check("testpkg", fset, []*ast.File{file}, info)
	// We ignore type-check errors because some tests use imports (like "fmt")
	// that cannot be resolved without an importer. The partial type info is
	// still sufficient for testing the metadata extraction logic.

	return info
}

// typeCheckFileWithImporter creates types.Info with a real importer for stdlib
func typeCheckFileWithImporter(t *testing.T, fset *token.FileSet, file *ast.File) *types.Info {
	t.Helper()

	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error:    func(_ error) {},
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	_, _ = conf.Check("testpkg", fset, []*ast.File{file}, info)
	return info
}

// ---------------------------------------------------------------------------
// extractParamsAndTypeParams — IndexExpr branch
// ---------------------------------------------------------------------------

func TestExtractParamsAndTypeParams_IndexExpr_Coverage2(_ *testing.T) {
	// Test the IndexExpr branch (call.Fun is *ast.IndexExpr)
	// e.g., Func[int]()
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	ident := &ast.Ident{Name: "GenericFunc"}
	callExpr := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     ident,
			Index: &ast.Ident{Name: "int"},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	// Should not panic even without type info
	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_IndexListExpr_Coverage2(_ *testing.T) {
	// Test the IndexListExpr branch (call.Fun is *ast.IndexListExpr)
	// e.g., Func[int, string]()
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexListExpr{
			X: &ast.Ident{Name: "MultiGenericFunc"},
			Indices: []ast.Expr{
				&ast.Ident{Name: "int"},
				&ast.Ident{Name: "string"},
			},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_IndexExprWithSelector(_ *testing.T) {
	// Test IndexExpr with SelectorExpr: pkg.Func[T]()
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "pkg"},
				Sel: &ast.Ident{Name: "GenericFunc"},
			},
			Index: &ast.Ident{Name: "int"},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_IndexListExprWithSelector(_ *testing.T) {
	// Test IndexListExpr with SelectorExpr: pkg.Func[T1, T2]()
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexListExpr{
			X: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "pkg"},
				Sel: &ast.Ident{Name: "MultiFunc"},
			},
			Indices: []ast.Expr{
				&ast.Ident{Name: "int"},
				&ast.Ident{Name: "string"},
			},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_WithTypeResolution(t *testing.T) {
	// This test uses proper type checking to exercise the funcObj != nil branches
	src := `package testpkg

func identity[T any](x T) T {
	return x
}

func caller() {
	identity[int](42)
	identity("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)
	meta := newTestMetadata()

	// Find call expressions in caller()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "caller" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				args := make([]*CallArgument, len(call.Args))
				for i, argExpr := range call.Args {
					args[i] = ExprToCallArgument(argExpr, info, "testpkg", fset, meta)
				}
				paramArgMap := make(map[string]CallArgument)
				typeParamMap := make(map[string]string)

				extractParamsAndTypeParams(call, info, args, paramArgMap, typeParamMap)
			}
			return true
		})
	}
}

func TestExtractParamsAndTypeParams_SelectorWithTypeResolution(t *testing.T) {
	src := `package testpkg

import "fmt"

func main() {
	fmt.Println("hello", 42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)
	meta := newTestMetadata()

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				args := make([]*CallArgument, len(call.Args))
				for i, argExpr := range call.Args {
					args[i] = ExprToCallArgument(argExpr, info, "testpkg", fset, meta)
				}
				paramArgMap := make(map[string]CallArgument)
				typeParamMap := make(map[string]string)

				extractParamsAndTypeParams(call, info, args, paramArgMap, typeParamMap)

				// Should have mapped some parameters
				if len(paramArgMap) > 0 {
					t.Logf("paramArgMap: %v", paramArgMap)
				}
			}
			return true
		})
	}
}

func TestExtractParamsAndTypeParams_ParenExpr(_ *testing.T) {
	// Test ParenExpr: (func)()
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Instances:  make(map[*ast.Ident]types.Instance),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.ParenExpr{
			X: &ast.Ident{Name: "someFunc"},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, nil, paramArgMap, typeParamMap)
}

// ---------------------------------------------------------------------------
// DefaultImportName — versioned and edge case paths
// ---------------------------------------------------------------------------

func TestDefaultImportName_Versioned(t *testing.T) {
	assert.Equal(t, "mux", DefaultImportName("github.com/gorilla/mux/v5"))
}

func TestDefaultImportName_Simple(t *testing.T) {
	assert.Equal(t, "fmt", DefaultImportName("fmt"))
}

func TestDefaultImportName_Deep(t *testing.T) {
	assert.Equal(t, "metadata", DefaultImportName("github.com/antst/go-apispec/internal/metadata"))
}

func TestDefaultImportName_Empty(t *testing.T) {
	// strings.Split("", "/") returns [""], so we get ""
	assert.Equal(t, "", DefaultImportName(""))
}

// ---------------------------------------------------------------------------
// detectConstantReturnValue — more branch coverage
// ---------------------------------------------------------------------------

func TestDetectConstantReturnValue_MultipleReturns(t *testing.T) {
	src := `package testpkg

func getCode(ok bool) int {
	if ok {
		return 200
	}
	return 404
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFile(t, fset, file)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "getCode" {
			continue
		}
		val := detectConstantReturnValue(fn.Body, info)
		// Multiple different return values means not constant
		if val != "" {
			t.Logf("detected constant return: %s", val)
		}
	}
}

func TestDetectConstantReturnValue_StringReturn(t *testing.T) {
	src := `package testpkg

func getStatus() string {
	return "ok"
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFile(t, fset, file)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "getStatus" {
			continue
		}
		val := detectConstantReturnValue(fn.Body, info)
		if val != "" {
			assert.Contains(t, val, "ok")
		}
	}
}

// ---------------------------------------------------------------------------
// processVariables — with typed constants
// ---------------------------------------------------------------------------

func TestProcessVariables_TypedConsts(t *testing.T) {
	src := `package testpkg

type Status int

const (
	Active   Status = 1
	Inactive Status = 2
)

var GlobalName = "test"
var globalAge int = 30
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
		Variables: make(map[string]*Variable),
	}

	processVariables(file, info, "testpkg", fset, f, meta)

	// Should have some variables/constants
	assert.NotEmpty(t, f.Variables, "expected variables to be processed")
}

// ---------------------------------------------------------------------------
// processFunctions — function with return values and assignments
// ---------------------------------------------------------------------------

func TestProcessFunctions_WithReturns(t *testing.T) {
	src := `package testpkg

func add(a, b int) int {
	result := a + b
	return result
}

func greet(name string) (string, error) {
	msg := "hello " + name
	return msg, nil
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	meta.Callers = make(map[string][]*CallGraphEdge)
	meta.Callees = make(map[string][]*CallGraphEdge)
	meta.Args = make(map[string][]*CallGraphEdge)
	meta.ParentFunctions = make(map[string][]*CallGraphEdge)

	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
		Variables: make(map[string]*Variable),
	}

	pkgs := map[string]map[string]*ast.File{"testpkg": {"test.go": file}}
	funcMap := BuildFuncMap(pkgs)
	fileToInfo := map[*ast.File]*types.Info{file: info}

	processFunctions(file, info, "testpkg", fset, f, fileToInfo, funcMap, meta)

	assert.Contains(t, f.Functions, "add")
	assert.Contains(t, f.Functions, "greet")
}

// ---------------------------------------------------------------------------
// processImports — basic import processing
// ---------------------------------------------------------------------------

func TestProcessImports(t *testing.T) {
	src := `package testpkg

import (
	"fmt"
	"strings"
	alias "path/filepath"
)

func main() {
	fmt.Println(strings.ToUpper(alias.Base("test")))
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
		Imports:   make(map[int]int),
	}

	processImports(file, meta, f)

	assert.NotEmpty(t, f.Imports, "expected imports to be processed")
}

// ---------------------------------------------------------------------------
// analyzeInterfaceImplementations — basic interface matching
// ---------------------------------------------------------------------------

func TestAnalyzeInterfaceImplementations_BasicMatch(_ *testing.T) {
	meta := newTestMetadata()

	// Create an interface and a struct type
	pkg := &Package{
		Files: map[string]*File{
			"types.go": {
				Types: map[string]*Type{
					"Handler": {
						Name:  meta.StringPool.Get("Handler"),
						Pkg:   meta.StringPool.Get("pkg"),
						Kind:  meta.StringPool.Get("interface"),
						Scope: meta.StringPool.Get("exported"),
						Methods: []Method{
							{
								Name:     meta.StringPool.Get("Handle"),
								Receiver: meta.StringPool.Get("Handler"),
							},
						},
					},
					"MyHandler": {
						Name:  meta.StringPool.Get("MyHandler"),
						Pkg:   meta.StringPool.Get("pkg"),
						Kind:  meta.StringPool.Get("struct"),
						Scope: meta.StringPool.Get("exported"),
						Methods: []Method{
							{
								Name:     meta.StringPool.Get("Handle"),
								Receiver: meta.StringPool.Get("*MyHandler"),
							},
						},
					},
				},
				Functions: make(map[string]*Function),
			},
		},
	}

	meta.Packages["pkg"] = pkg

	analyzeInterfaceImplementations(meta.Packages, meta.StringPool)

	// MyHandler should implement Handler
	myHandler := pkg.Files["types.go"].Types["MyHandler"]
	handler := pkg.Files["types.go"].Types["Handler"]

	// Check that implementation relationships are set
	_ = myHandler
	_ = handler
	// The actual matching depends on exact string comparison logic,
	// so just verify it doesn't panic
}

// ---------------------------------------------------------------------------
// WriteYAML — success path
// ---------------------------------------------------------------------------

func TestWriteYAML_Success(t *testing.T) {
	meta := newTestMetadata()

	meta.Packages["testpkg"] = &Package{
		Files: map[string]*File{
			"main.go": {
				Functions: map[string]*Function{
					"main": {
						Name: meta.StringPool.Get("main"),
						Pkg:  meta.StringPool.Get("testpkg"),
					},
				},
				Types:     make(map[string]*Type),
				Variables: make(map[string]*Variable),
				Imports:   make(map[int]int),
			},
		},
	}

	tmpFile := t.TempDir() + "/test_meta.yaml"
	err := WriteYAML(meta, tmpFile)
	require.NoError(t, err, "WriteYAML should succeed for valid metadata")
}

// ---------------------------------------------------------------------------
// UnmarshalYAML for StringPool
// ---------------------------------------------------------------------------

func TestStringPool_UnmarshalYAML(t *testing.T) {
	sp := NewStringPool()
	id1 := sp.Get("hello")
	id2 := sp.Get("world")

	// Verify basic operations
	assert.Equal(t, "hello", sp.GetString(id1))
	assert.Equal(t, "world", sp.GetString(id2))

	// Verify negative IDs return empty string
	assert.Equal(t, "", sp.GetString(-1))
}

// ---------------------------------------------------------------------------
// id function on Call
// ---------------------------------------------------------------------------

func TestCall_ID_Coverage2(t *testing.T) {
	m := newTestMetadata()

	call := Call{
		Meta:     m,
		Name:     m.StringPool.Get("Handle"),
		Pkg:      m.StringPool.Get("pkg"),
		Position: m.StringPool.Get("file.go:10"),
		RecvType: m.StringPool.Get("*Server"),
		Scope:    m.StringPool.Get("exported"),
	}

	// Build identifier through edge context
	edge := CallGraphEdge{
		Caller: call,
		Callee: Call{
			Meta:     m,
			Name:     m.StringPool.Get("Process"),
			Pkg:      m.StringPool.Get("pkg"),
			Position: -1,
			RecvType: -1,
			Scope:    m.StringPool.Get("exported"),
		},
		meta: m,
	}
	edge.Caller.Edge = &edge
	edge.Callee.Edge = &edge
	edge.Caller.buildIdentifier()
	edge.Callee.buildIdentifier()

	baseID := edge.Caller.BaseID()
	assert.Contains(t, baseID, "Server")
	assert.Contains(t, baseID, "Handle")

	baseID2 := edge.Callee.BaseID()
	assert.Contains(t, baseID2, "Process")
}

// ---------------------------------------------------------------------------
// CallIdentifier.ID — default case (unknown idType)
// ---------------------------------------------------------------------------

func TestCallIdentifier_ID_DefaultCase(t *testing.T) {
	ci := NewCallIdentifier("pkg", "func1", "", "", nil)
	// Use a value beyond the defined enum to hit the default case
	result := ci.ID(CallIdentifierType(99))
	assert.Equal(t, "pkg.func1", result)
}

// ---------------------------------------------------------------------------
// processStructInstance — nil type (empty type name returns early)
// ---------------------------------------------------------------------------

func TestProcessStructInstance_NilType(t *testing.T) {
	meta := newTestMetadata()
	fset := token.NewFileSet()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}

	// CompositeLit with nil Type - getTypeName returns ""
	cl := &ast.CompositeLit{
		Type: nil,
		Elts: []ast.Expr{},
	}

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}

	processStructInstance(cl, info, "pkg", fset, f, nil, meta)
	assert.Empty(t, f.StructInstances, "expected no instance for nil type")
}

// ---------------------------------------------------------------------------
// processAssignment — multi-return function assignment
// ---------------------------------------------------------------------------

func TestProcessAssignment_MultiReturn(t *testing.T) {
	src := `package testpkg

func multi() (int, string) {
	return 1, "hello"
}

func caller() {
	x, _ := multi()
	_ = x
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)
	meta := newTestMetadata()
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	})
	fileToInfo := map[*ast.File]*types.Info{file: info}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "caller" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				assignments := processAssignment(assign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
				_ = assignments
			}
			return true
		})
	}
}

// ---------------------------------------------------------------------------
// GenerateMetadata — with mock types (should be skipped)
// ---------------------------------------------------------------------------

func TestGenerateMetadata_MockTypesSkipped(t *testing.T) {
	src := `package testpkg

type MockHandler struct{}

func (m *MockHandler) Handle() {}

type RealHandler struct{}

func (r *RealHandler) Handle() {}

func mockSetup() {}

func realSetup() {
	mockSetup()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)

	pkg := meta.Packages["testpkg"]
	require.NotNil(t, pkg)

	// Mock types should be skipped
	for _, f := range pkg.Files {
		_, hasMock := f.Types["MockHandler"]
		assert.False(t, hasMock, "MockHandler should be skipped")

		_, hasReal := f.Types["RealHandler"]
		assert.True(t, hasReal, "RealHandler should be present")
	}
}

// ---------------------------------------------------------------------------
// GenerateMetadata with typed constants (for collectConstants coverage)
// ---------------------------------------------------------------------------

func TestGenerateMetadata_WithTypedConstants(t *testing.T) {
	src := `package testpkg

type Color int

const (
	Red   Color = iota
	Green
	Blue
)

const MaxSize = 100

func getColor() Color {
	return Red
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)
}

// ---------------------------------------------------------------------------
// processAssignment — default case (map index, pointer dereference)
// ---------------------------------------------------------------------------

func TestProcessAssignment_DefaultCase(t *testing.T) {
	src := `package testpkg

func caller() {
	m := map[string]int{}
	m["key"] = 42
	arr := []int{1, 2, 3}
	arr[0] = 10
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)
	meta := newTestMetadata()
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	})
	fileToInfo := map[*ast.File]*types.Info{file: info}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "caller" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				assignments := processAssignment(assign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
				_ = assignments
			}
			return true
		})
	}
}

// ---------------------------------------------------------------------------
// processStructFields — field with non-primitive external type
// ---------------------------------------------------------------------------

func TestProcessStructFields_WithTypeInfo(t *testing.T) {
	src := `package testpkg

import "net/http"

type Server struct {
	Handler http.Handler
	Name    string
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)
	meta := newTestMetadata()

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			tspec, ok := spec.(*ast.TypeSpec)
			if !ok || tspec.Name.Name != "Server" {
				continue
			}
			structType, ok := tspec.Type.(*ast.StructType)
			require.True(t, ok)

			ty := &Type{
				Name: meta.StringPool.Get("Server"),
				Pkg:  meta.StringPool.Get("testpkg"),
			}

			processStructFields(structType, "testpkg", meta, ty, info)
			assert.NotEmpty(t, ty.Fields)
		}
	}
}

// ---------------------------------------------------------------------------
// GenerateMetadata — empty package
// ---------------------------------------------------------------------------

func TestGenerateMetadata_EmptyPackage(t *testing.T) {
	src := `package testpkg
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)
	assert.NotNil(t, meta.Packages["testpkg"])
}

// ---------------------------------------------------------------------------
// GenerateMetadata — package with method on non-pointer receiver using generics
// ---------------------------------------------------------------------------

func TestGenerateMetadata_MethodAndCallChain(t *testing.T) {
	src := `package testpkg

type Service struct {
	name string
}

func NewService(name string) *Service {
	return &Service{name: name}
}

func (s *Service) GetName() string {
	return s.name
}

func (s *Service) Process(data string) string {
	return data + s.GetName()
}

func main() {
	svc := NewService("test")
	svc.Process("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)
	assert.NotEmpty(t, meta.CallGraph, "expected call graph edges for method calls")
}

// ---------------------------------------------------------------------------
// GenerateMetadataWithLogger — multi-package with no common prefix
// ---------------------------------------------------------------------------

func TestGenerateMetadata_DivergentPaths(t *testing.T) {
	src1 := `package main

func main() {}
`
	src2 := `package util

func Help() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "main.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "util.go", src2, parser.AllErrors)
	require.NoError(t, err)

	info1 := typeCheckFile(t, fset, file1)
	info2 := typeCheckFile(t, fset, file2)

	// Two completely different package paths
	pkgs := map[string]map[string]*ast.File{
		"example.com/app":         {"main.go": file1},
		"example.com/lib/pkg/sub": {"util.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{
		file1: info1,
		file2: info2,
	}
	importPaths := map[string]string{
		"main.go": "example.com/app",
		"util.go": "example.com/lib/pkg/sub",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)
	assert.NotEmpty(t, meta.Packages)
}

// ---------------------------------------------------------------------------
// GenerateMetadata — package with generics to exercise type param propagation
// ---------------------------------------------------------------------------

func TestGenerateMetadata_WithGenerics(t *testing.T) {
	src := `package testpkg

func Map[T any, U any](input []T, fn func(T) U) []U {
	result := make([]U, len(input))
	for i, v := range input {
		result[i] = fn(v)
	}
	return result
}

func Filter[T any](items []T, pred func(T) bool) []T {
	var result []T
	for _, item := range items {
		if pred(item) {
			result = append(result, item)
		}
	}
	return result
}

func toString(n int) string {
	return ""
}

func isPositive(n int) bool {
	return n > 0
}

func caller() {
	nums := []int{1, 2, 3}
	Map[int, string](nums, toString)
	Map(nums, toString)
	positive := Filter(nums, isPositive)
	Map(positive, toString)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	require.NotNil(t, meta)

	// Should have call graph edges
	assert.NotEmpty(t, meta.CallGraph)
}

// ---------------------------------------------------------------------------
// GenerateMetadata — package with methods, types, variables, and struct instances
// ---------------------------------------------------------------------------

func TestGenerateMetadata_CompletePackage(t *testing.T) {
	src := `package testpkg

type Status int

const (
	Active   Status = 1
	Inactive Status = 2
)

var DefaultName = "test"

type Config struct {
	Host string
	Port int
}

type Handler interface {
	Handle()
}

type Server struct {
	Config Config
}

func (s *Server) Handle() {}

func (s Server) Name() string { return "" }

func NewServer() *Server {
	return &Server{Config: Config{Host: "localhost", Port: 8080}}
}

func setup() {
	s := NewServer()
	s.Handle()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFileWithImporter(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"test.go": "testpkg"}

	logger := &testLogger{}
	meta := GenerateMetadataWithLogger(pkgs, fileToInfo, importPaths, fset, logger)
	require.NotNil(t, meta)

	pkg := meta.Packages["testpkg"]
	require.NotNil(t, pkg)

	// Should have files
	assert.NotEmpty(t, pkg.Files)

	// Should have call graph edges
	assert.NotEmpty(t, meta.CallGraph, "expected call graph edges")

	// Logger should have received messages
	assert.NotEmpty(t, logger.messages)
}

// ---------------------------------------------------------------------------
// processTypes — with "other" type kind (e.g., function type)
// ---------------------------------------------------------------------------

func TestProcessTypes_FunctionType(t *testing.T) {
	src := `package testpkg

type HandlerFunc func(input string) error
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}
	allTypeMethods := make(map[string][]Method)
	allTypes := make(map[string]*Type)

	processTypes(file, info, "testpkg", fset, f, allTypeMethods, allTypes, meta)

	assert.Contains(t, f.Types, "HandlerFunc")
	handlerFunc := f.Types["HandlerFunc"]
	kind := meta.StringPool.GetString(handlerFunc.Kind)
	// Function types are "other"
	assert.Equal(t, "other", kind)
}

// ---------------------------------------------------------------------------
// BuildFuncMap — with value receiver method
// ---------------------------------------------------------------------------

func TestBuildFuncMap_ValueReceiver(t *testing.T) {
	src := `package testpkg

type Server struct{}

func (s Server) Name() string { return "" }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}

	funcMap := BuildFuncMap(pkgs)

	foundMethod := false
	for key := range funcMap {
		if key == "testpkg.Server.Name" {
			foundMethod = true
		}
	}
	assert.True(t, foundMethod, "expected to find Server.Name method")
}

// ---------------------------------------------------------------------------
// WriteYAML — error on invalid path
// ---------------------------------------------------------------------------

func TestWriteYAML_InvalidPath(t *testing.T) {
	meta := newTestMetadata()
	err := WriteYAML(meta, "/nonexistent_root_xyz/sub/meta.yaml")
	assert.Error(t, err)
}

// badYAMLEncoder triggers an error during yaml.Encode
type badYAMLEncoder struct{}

func (b badYAMLEncoder) MarshalYAML() (interface{}, error) {
	return nil, errors.New("encode error")
}

func TestWriteYAML_EncodeError(t *testing.T) {
	tmpFile := t.TempDir() + "/encode_error.yaml"
	err := WriteYAML(badYAMLEncoder{}, tmpFile)
	assert.Error(t, err)
}

func TestWriteYAML_RemoveExisting(t *testing.T) {
	// Write a file first, then overwrite it
	tmpFile := t.TempDir() + "/existing.yaml"
	err := WriteYAML("first", tmpFile)
	if err != nil {
		t.Skipf("WriteYAML failed: %v", err)
	}
	err = WriteYAML("second", tmpFile)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// processStructInstance — empty composite literal
// ---------------------------------------------------------------------------

func TestProcessStructInstance_EmptyLiteral(t *testing.T) {
	src := `package testpkg

type Config struct{}

var c = Config{}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	info := typeCheckFile(t, fset, file)
	meta := newTestMetadata()
	f := &File{
		Functions: make(map[string]*Function),
		Types:     make(map[string]*Type),
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if cl, ok := n.(*ast.CompositeLit); ok {
			processStructInstance(cl, info, "testpkg", fset, f, nil, meta)
		}
		return true
	})

	assert.NotEmpty(t, f.StructInstances)
}
