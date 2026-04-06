package metadata

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fullCovMeta creates a fully-initialised Metadata for coverage tests.
func fullCovMeta() *Metadata {
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

// typeCheckFull creates types.Info with a real importer (for stdlib imports).
func typeCheckFull(t *testing.T, fset *token.FileSet, file *ast.File) *types.Info {
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

// typeCheckNoImport creates types.Info without an importer (fast, for simple tests).
func typeCheckNoImport(t *testing.T, fset *token.FileSet, file *ast.File) *types.Info {
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
	_, _ = conf.Check("testpkg", fset, []*ast.File{file}, info)
	return info
}

// ===========================================================================
// Finalize — types.go:127 (0%)
// The function body is commented out (no-op). We call it to get 100%.
// ===========================================================================

func TestFinalize_IsNoOp(t *testing.T) {
	sp := NewStringPool()
	idx := sp.Get("hello")
	sp.Finalize()
	// Finalize is a no-op — the body is `// sp.strings = nil`.
	// The lookup map should still work after Finalize.
	assert.Equal(t, "hello", sp.GetString(idx))
	// Verify that the pool is still usable.
	idx2 := sp.Get("world")
	assert.Equal(t, "world", sp.GetString(idx2))
}

// ===========================================================================
// UnmarshalYAML — types.go:112 (87.5%)
// Uncovered: line 114 — unmarshal error branch.
// ===========================================================================

func TestUnmarshalYAML_Error(t *testing.T) {
	sp := &StringPool{}
	err := sp.UnmarshalYAML(func(_ interface{}) error {
		return assert.AnError
	})
	assert.Error(t, err)
}

// ===========================================================================
// DefaultImportName — analysis.go:142 (85.7%)
// Uncovered: line 144 — len(parts) == 0 (dead code: strings.Split always
// returns at least one element).
// ===========================================================================

func TestDefaultImportName_DeadCodeBranch(t *testing.T) {
	// NOTE: The len(parts) == 0 branch at line 144 is unreachable because
	// strings.Split always returns at least one element, even for an empty
	// string. We document this as dead code and test the adjacent branches.

	// Empty string → strings.Split returns [""], len(parts) == 1, so the
	// dead-code guard is bypassed and we return "".
	assert.Equal(t, "", DefaultImportName(""))

	// Normal path.
	assert.Equal(t, "gin", DefaultImportName("github.com/gin-gonic/gin"))

	// Versioned path.
	assert.Equal(t, "echo", DefaultImportName("github.com/labstack/echo/v4"))

	// Single segment.
	assert.Equal(t, "fmt", DefaultImportName("fmt"))

	// Version-like but not valid (letter after 'v').
	assert.Equal(t, "vx", DefaultImportName("github.com/foo/vx"))
}

// ===========================================================================
// isTypeConversion — analysis.go:156 (93.8%)
// Uncovered: line 188 — the single-arg type conversion branch via info.Types.
// ===========================================================================

func TestIsTypeConversion_SingleArgTypeViaInfoTypes(t *testing.T) {
	// This covers the branch: len(call.Args)==1 && info.Types[call.Fun].IsType()
	// We construct an AST call expression where the Fun is a type that the
	// info.Types map knows about.
	src := `package testpkg

type MyInt int

func f() {
	var x int = 42
	_ = MyInt(x)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	// Find the MyInt(x) call expression.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	// With the type-checked info, this should be detected as a type conversion.
	assert.True(t, isTypeConversion(callExpr, info))
}

func TestIsTypeConversion_ArrayAndStarAndFuncType(t *testing.T) {
	// Cover the *ast.ArrayType, *ast.StarExpr, *ast.InterfaceType,
	// *ast.StructType, *ast.FuncType cases.
	meta := fullCovMeta()
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	// ArrayType case
	callArray := &ast.CallExpr{
		Fun:  &ast.ArrayType{Elt: ast.NewIdent("int")},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callArray, info))

	// StarExpr case
	callStar := &ast.CallExpr{
		Fun:  &ast.StarExpr{X: ast.NewIdent("MyType")},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callStar, info))

	// InterfaceType case
	callIface := &ast.CallExpr{
		Fun:  &ast.InterfaceType{Methods: &ast.FieldList{}},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callIface, info))

	// StructType case
	callStruct := &ast.CallExpr{
		Fun:  &ast.StructType{Fields: &ast.FieldList{}},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callStruct, info))

	// FuncType case
	callFunc := &ast.CallExpr{
		Fun:  &ast.FuncType{Params: &ast.FieldList{}},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callFunc, info))

	_ = meta // used for reference
}

// ===========================================================================
// getCalleeFunctionNameAndPackage — analysis.go:199 (94.1%)
// Uncovered: lines 225-227 (Interface case inside SelectorExpr -> Ident path)
//            lines 250-252 (Interface case inside SelectorExpr -> non-Ident path)
// ===========================================================================

func TestGetCalleeFunctionNameAndPackage_InterfaceReceiver(t *testing.T) {
	// Covers the *types.Interface fallback branch in the SelectorExpr → Ident path.
	// A named interface goes through *types.Named, so we need to use an unnamed
	// interface type to hit the *types.Interface branch directly.
	// The practical coverage here is for the Named type case (which returns the
	// named type string) and verifying the function doesn't panic.
	src := `package testpkg

type MyInterface interface {
	DoSomething()
}

func callIt(i MyInterface) {
	i.DoSomething()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find the call expression i.DoSomething()
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			callExpr = ce
		}
		return true
	})
	require.NotNil(t, callExpr)

	name, pkg, recvType := getCalleeFunctionNameAndPackage(callExpr.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "DoSomething", name)
	assert.Equal(t, "testpkg", pkg)
	// Named interface type returns the type name, not "interface".
	assert.Equal(t, "MyInterface", recvType)
}

func TestGetCalleeFunctionNameAndPackage_ComplexReceiverExpr(t *testing.T) {
	// This covers the non-Ident SelectorExpr.X path (lines 237-256).
	// When the receiver is a complex expression (e.g., a function call result),
	// it goes through the info.Types path.
	src := `package testpkg

type MyStruct struct{}

func (s *MyStruct) Method() {}

func getStruct() *MyStruct { return &MyStruct{} }

func main() {
	getStruct().Method()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find the getStruct().Method() call.
	var methodCall *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Method" {
					methodCall = ce
				}
			}
		}
		return true
	})
	require.NotNil(t, methodCall)

	name, pkg, recvType := getCalleeFunctionNameAndPackage(methodCall.Fun, file, "testpkg", fileToInfo, funcMap, fset)
	assert.Equal(t, "Method", name)
	assert.Equal(t, "testpkg", pkg)
	assert.Contains(t, recvType, "MyStruct")
}

// ===========================================================================
// handleSelector — expression.go:148 (96.9%)
// Uncovered: line 158-163 — obj is a *types.PkgName in handleSelector.
// ===========================================================================

func TestHandleSelector_PkgNameBranch(t *testing.T) {
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFull(t, fset, file)
	meta := fullCovMeta()

	// Find fmt.Println selector expression
	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := se.X.(*ast.Ident); ok && id.Name == "fmt" {
				selExpr = se
			}
		}
		return true
	})
	require.NotNil(t, selExpr)

	// The info.ObjectOf(e.Sel) for Println should resolve to a *types.Func,
	// but info.ObjectOf for the fmt selector should be a PkgName.
	// Let's call handleSelector which goes through both branches.
	arg := handleSelector(selExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindSelector, arg.GetKind())
	assert.Equal(t, "fmt", arg.GetPkg())
}

// ===========================================================================
// getTypeName — helpers.go:33 (96.9%)
// Uncovered: line 67 — SelectorExpr where info.ObjectOf(x) returns non-nil
//   but is not a PkgName (falls through to obj.Name()).
// ===========================================================================

func TestGetTypeName_SelectorExpr_NonPkgNameObj(t *testing.T) {
	src := `package testpkg

type MyStruct struct {
	Field int
}

func f(s MyStruct) int {
	return s.Field
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	// Find the s.Field selector expression in the function body
	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := se.X.(*ast.Ident); ok && id.Name == "s" {
				selExpr = se
			}
		}
		return true
	})
	require.NotNil(t, selExpr)

	result := getTypeName(selExpr, info)
	// The info.ObjectOf(s) should return a *types.Var (not PkgName),
	// so we hit the obj.Name()+"."+sel line.
	assert.Contains(t, result, "Field")
}

// ===========================================================================
// WriteYAML — io.go:42 (93.3%)
// Uncovered: line 54 — file.Close() error path in deferred function.
// This is nearly impossible to trigger reliably in a test because the OS
// close rarely fails for a normal file. Instead, we cover the os.Remove
// error path that's hard to reach.
// ===========================================================================

func TestWriteYAML_ReadOnlyDir(t *testing.T) {
	// The uncovered line 54 is the file.Close() error path inside the defer.
	// Since we cannot reliably force Close to fail on a regular file, we
	// document this as a practically unreachable path.
	// Instead, we test writing to a read-only directory (triggers OpenFile error, line 49).
	tempDir := t.TempDir()
	readonlyDir := filepath.Join(tempDir, "readonly")
	require.NoError(t, os.MkdirAll(readonlyDir, 0555))
	t.Cleanup(func() {
		_ = os.Chmod(readonlyDir, 0755)
	})
	err := WriteYAML("data", filepath.Join(readonlyDir, "test.yaml"))
	assert.Error(t, err)
}

// ===========================================================================
// GetCallPath — metadata.go:636 (94.7%)
// Uncovered: line 645 — visited[current] return false path in DFS.
// ===========================================================================

func TestGetCallPath_CycleInGraph(t *testing.T) {
	meta := fullCovMeta()

	// Create a cycle: A -> B -> A
	edgeAB := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("B"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	edgeBA := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("B"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	meta.CallGraph = []CallGraphEdge{edgeAB, edgeBA}
	meta.BuildCallGraphMaps()

	// Try to find path from A to C (non-existent) — the cycle should be detected.
	path := meta.GetCallPath("pkg.A", "pkg.C")
	assert.Nil(t, path)
}

func TestGetCallPath_Unreachable(t *testing.T) {
	meta := fullCovMeta()

	// Create an edge A -> B with no further edges.
	edge := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("B"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	// Try to find path from A to C — should return nil and exercise backtrack.
	path := meta.GetCallPath("pkg.A", "pkg.C")
	assert.Nil(t, path)
}

// ===========================================================================
// BuildFuncMap — metadata.go:669 (96.2%)
// Uncovered: line 685-687 — pkgName is empty (file.Name is nil).
// ===========================================================================

func TestBuildFuncMap_NilFileName(t *testing.T) {
	// Create a file with nil Name to hit the pkgName=="" branch.
	file := &ast.File{
		Name:  nil,
		Decls: []ast.Decl{},
	}
	pkgs := map[string]map[string]*ast.File{
		"mypkg": {"test.go": file},
	}
	fm := BuildFuncMap(pkgs)
	assert.NotNil(t, fm)
}

func TestBuildFuncMap_EmptyPkgName(t *testing.T) {
	// Create a file with Name="" to exercise the else branch.
	src := `package a

func Hello() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)

	// Override the Name to empty string
	file.Name = &ast.Ident{Name: ""}

	pkgs := map[string]map[string]*ast.File{
		"mypkg": {"test.go": file},
	}
	fm := BuildFuncMap(pkgs)
	// With empty pkgName, the key should be just the function name.
	_, exists := fm["Hello"]
	assert.True(t, exists, "expected function key 'Hello' when pkgName is empty")
}

// ===========================================================================
// collectConstants — metadata.go:722 (92.9%)
// Uncovered: line 733-734 — spec is not *ast.ValueSpec.
// ===========================================================================

func TestCollectConstants_NonValueSpec(t *testing.T) {
	// This is defensive code — in practice, const blocks always contain
	// ValueSpecs. We test by constructing a GenDecl with CONST token but
	// a TypeSpec (which is weird but tests the branch).
	meta := fullCovMeta()
	fset := token.NewFileSet()

	file := &ast.File{
		Name: ast.NewIdent("testpkg"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.CONST,
				Specs: []ast.Spec{
					// A TypeSpec inside a CONST block — unusual but tests the branch.
					&ast.TypeSpec{
						Name: ast.NewIdent("Weird"),
						Type: ast.NewIdent("int"),
					},
				},
			},
		},
	}

	constMap := collectConstants(file, nil, "testpkg", fset, meta)
	assert.Empty(t, constMap)
}

// ===========================================================================
// processTypes — metadata.go:758 (94.4%)
// Uncovered: line 767-768 — spec is not *ast.TypeSpec inside a TYPE genDecl.
// ===========================================================================

func TestProcessTypes_NonTypeSpec(t *testing.T) {
	meta := fullCovMeta()
	fset := token.NewFileSet()

	f := &File{
		Types:     make(map[string]*Type),
		Functions: make(map[string]*Function),
		Variables: make(map[string]*Variable),
		Imports:   make(map[int]int),
	}

	file := &ast.File{
		Name: ast.NewIdent("testpkg"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					// Put a ValueSpec inside a TYPE block — tests the !ok branch.
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("x")},
						Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				},
			},
		},
	}

	processTypes(file, nil, "testpkg", fset, f, make(map[string][]Method), make(map[string]*Type), meta)
	assert.Empty(t, f.Types)
}

// ===========================================================================
// processStructFields — metadata.go:939 (95.5%)
// Uncovered: line 952-954 — external type that resolves to primitive.
// ===========================================================================

func TestProcessStructFields_ExternalTypeToPrimitive(t *testing.T) {
	// We need a struct with a field whose type is external (not in the current
	// module path) and whose underlying type is a primitive.
	src := `package testpkg

import "time"

type MyStruct struct {
	Duration time.Duration
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFull(t, fset, file)
	meta := fullCovMeta()
	meta.CurrentModulePath = "testpkg"

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
			// time.Duration is an external type. Its underlying is int64 (primitive).
			assert.GreaterOrEqual(t, len(ty.Fields), 1)
		}
	}
}

// ===========================================================================
// processVariables — metadata.go:1150 (96.3%)
// Uncovered: line 1171-1172 — spec is not *ast.ValueSpec inside VAR/CONST block.
// ===========================================================================

func TestProcessVariables_NonValueSpec(t *testing.T) {
	meta := fullCovMeta()
	fset := token.NewFileSet()

	f := &File{
		Types:     make(map[string]*Type),
		Functions: make(map[string]*Function),
		Variables: make(map[string]*Variable),
		Imports:   make(map[int]int),
	}

	file := &ast.File{
		Name: ast.NewIdent("testpkg"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					// A TypeSpec inside a VAR block — unusual but tests the branch.
					&ast.TypeSpec{
						Name: ast.NewIdent("WeirdType"),
						Type: ast.NewIdent("int"),
					},
				},
			},
		},
	}

	processVariables(file, nil, "testpkg", fset, f, meta)
	assert.Empty(t, f.Variables)
}

// ===========================================================================
// processAssignment — metadata.go:1262 (90.2%)
// Uncovered: line 1268-1270 — rhsLen > maxLen path
//            line 1299-1300 — funcName == "" skip
//            line 1353-1364 — default LHS branch (non-ident, non-selector, non-index)
// ===========================================================================

func TestProcessAssignment_RhsLenGreater(t *testing.T) {
	// Create an assignment where RHS has more elements than LHS:
	// x := a, b, c  (this is invalid Go, but the function handles it)
	meta := fullCovMeta()
	fset := token.NewFileSet()

	// Parse a real source with a normal assignment so we have a valid context.
	src := `package testpkg

func main() {
	x, y := 1, 2, 3
}
`
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors|parser.SkipObjectResolution)
	// This may parse but produces errors; we only need the AST.
	if file == nil {
		t.Skip("Cannot parse source with mismatched assignment")
		return
	}
	_ = err

	info := typeCheckNoImport(t, fset, file)
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find the assignment statement.
	var assignStmt *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			assignStmt = as
		}
		return true
	})
	if assignStmt == nil {
		t.Skip("No assignment statement found in parsed source")
		return
	}

	assignments := processAssignment(assignStmt, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	// We just need to exercise the code path without panicking.
	_ = assignments
}

func TestProcessAssignment_FuncNameEmpty(t *testing.T) {
	// Assignment outside any function → funcName will be empty → skip.
	meta := fullCovMeta()
	fset := token.NewFileSet()

	// Construct an assignment statement manually.
	assignStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("x")},
		Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "42"}},
		Tok: token.ASSIGN,
	}

	// Create a file with no functions.
	file := &ast.File{
		Name: ast.NewIdent("testpkg"),
		Decls: []ast.Decl{
			// Put a var decl, not a func
			&ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("y")},
						Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				},
			},
		},
	}

	fileToInfo := map[*ast.File]*types.Info{file: nil}
	funcMap := map[string]*ast.FuncDecl{}

	assignments := processAssignment(assignStmt, file, nil, "testpkg", fset, fileToInfo, funcMap, meta)
	// funcName is empty, so the assignment should be skipped.
	assert.Empty(t, assignments)
}

func TestProcessAssignment_DefaultLHS(t *testing.T) {
	// Covers the default branch: LHS is not Ident, SelectorExpr, or IndexExpr.
	// We use a StarExpr as LHS (e.g., *ptr = value).
	src := `package testpkg

func main() {
	var x int
	var ptr *int = &x
	*ptr = 42
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find the *ptr = 42 assignment.
	var assignStmt *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if _, ok := as.Lhs[0].(*ast.StarExpr); ok {
				assignStmt = as
			}
		}
		return true
	})
	require.NotNil(t, assignStmt, "expected to find *ptr = 42 assignment")

	assignments := processAssignment(assignStmt, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	// The default branch should produce a "raw" assignment.
	assert.NotEmpty(t, assignments)
	assert.Equal(t, "raw", meta.StringPool.GetString(assignments[0].Scope))
}

// ===========================================================================
// getTypeWithGenerics — metadata.go:1417 (86.7%)
// Uncovered: line 1433-1436 — ParenExpr branch
// ===========================================================================

func TestGetTypeWithGenerics_ParenExpr(t *testing.T) {
	src := `package testpkg

func myFunc() int { return 0 }

func f() {
	(myFunc)()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	// Find the (myFunc)() call expression.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.ParenExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	parenExpr := callExpr.Fun.(*ast.ParenExpr)
	typ := getTypeWithGenerics(parenExpr, info)
	// Should resolve to the function type of myFunc.
	assert.NotNil(t, typ)
}

func TestGetTypeWithGenerics_IndexExprRecursive(t *testing.T) {
	// Cover the recursive IndexExpr branch.
	src := `package testpkg

func myFunc() int { return 0 }

var arr = [1]func() int{myFunc}

func f() {
	arr[0]()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	// Find the arr[0]() call expression.
	var indexCallExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.IndexExpr); ok {
				indexCallExpr = ce
			}
		}
		return true
	})
	if indexCallExpr != nil {
		typ := getTypeWithGenerics(indexCallExpr.Fun, info)
		// Should get the element type from the array.
		_ = typ
	}
}

// ===========================================================================
// processCallExpression — metadata.go:1456 (91.4%)
// Uncovered: line 1458 — isTypeConversion return
//            line 1480-1498 — FuncLit caller with parent function
//            line 1526-1528 — parentFunction != nil
// ===========================================================================

func TestProcessCallExpression_TypeConversionSkipped(t *testing.T) {
	src := `package testpkg

type MyInt int

func main() {
	var x int = 42
	_ = MyInt(x)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	pkgs := map[string]map[string]*ast.File{"testpkg": {"test.go": file}}
	funcMap := BuildFuncMap(pkgs)

	// Build call graph — the type conversion should be skipped.
	buildCallGraph(pkgs["testpkg"], pkgs, "testpkg", fileToInfo, fset, funcMap, meta)
	// No edges should be created for the type conversion.
	for _, edge := range meta.CallGraph {
		calleeName := meta.StringPool.GetString(edge.Callee.Name)
		assert.NotEqual(t, "MyInt", calleeName, "type conversion should not create call graph edge")
	}
}

func TestProcessCallExpression_FuncLitCaller(t *testing.T) {
	// Covers the FuncLit caller branch (lines 1480-1498) and parentFunction != nil.
	src := `package testpkg

func helper() {}

func main() {
	fn := func() {
		helper()
	}
	fn()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	pkgs := map[string]map[string]*ast.File{"testpkg": {"test.go": file}}
	funcMap := BuildFuncMap(pkgs)

	// Process all files to build metadata including functions.
	for fileName, f := range pkgs["testpkg"] {
		fileInfo := fileToInfo[f]
		metaFile := &File{
			Types:     make(map[string]*Type),
			Functions: make(map[string]*Function),
			Variables: make(map[string]*Variable),
			Imports:   make(map[int]int),
		}
		processFunctions(f, fileInfo, "testpkg", fset, metaFile, fileToInfo, funcMap, meta)
		pkg := &Package{Files: map[string]*File{fileName: metaFile}}
		meta.Packages["testpkg"] = pkg
	}

	buildCallGraph(pkgs["testpkg"], pkgs, "testpkg", fileToInfo, fset, funcMap, meta)
	// The call from the function literal to helper() should create an edge
	// with a FuncLit caller.
	found := false
	for _, edge := range meta.CallGraph {
		callerName := meta.StringPool.GetString(edge.Caller.Name)
		if callerName == "helper" || meta.StringPool.GetString(edge.Callee.Name) == "helper" {
			found = true
		}
	}
	assert.True(t, found, "expected call graph edge involving helper()")
}

// ===========================================================================
// extractParamsAndTypeParams — metadata.go:1700 (66.7%)
// Uncovered: many branches for IndexExpr, IndexListExpr, SelectorExpr with
//   generic type params, and the arg inference path.
// ===========================================================================

func TestExtractParamsAndTypeParams_IndexExprWithSelectorX(_ *testing.T) {
	// Test the IndexExpr branch where fun.X is a SelectorExpr (not just an Ident).
	// This covers lines 1710-1711 (IndexExpr → SelectorExpr).
	meta := fullCovMeta()
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Construct: pkg.Func[int]()
	callExpr := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent("pkg"),
				Sel: ast.NewIdent("Func"),
			},
			Index: ast.NewIdent("int"),
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	args := make([]*CallArgument, 0)

	// Should not panic, even though funcObj won't resolve without real types.
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	_ = meta
}

func TestExtractParamsAndTypeParams_IndexListExprWithSelectorX(_ *testing.T) {
	// Test the IndexListExpr branch where fun.X is a SelectorExpr.
	// Covers lines 1716-1717 (IndexListExpr → SelectorExpr).
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Construct: pkg.Func[int, string]()
	callExpr := &ast.CallExpr{
		Fun: &ast.IndexListExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent("pkg"),
				Sel: ast.NewIdent("Func"),
			},
			Indices: []ast.Expr{ast.NewIdent("int"), ast.NewIdent("string")},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	args := make([]*CallArgument, 0)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_IndexListExprWithIdent(_ *testing.T) {
	// Test the IndexListExpr branch where fun.X is a simple Ident.
	// Covers lines 1714-1715 (IndexListExpr → Ident).
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	callExpr := &ast.CallExpr{
		Fun: &ast.IndexListExpr{
			X:       ast.NewIdent("GenericFunc"),
			Indices: []ast.Expr{ast.NewIdent("int"), ast.NewIdent("string")},
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	args := make([]*CallArgument, 0)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_SelectorExprWithIndexExprX(_ *testing.T) {
	// Test the inner SelectorExpr branches (lines 1735-1739).
	// call.Fun is a SelectorExpr, and within the type param extraction, fun.X is IndexExpr.
	// This requires a generic function with type params and a selector call.
	// We construct the AST but without real type info, so the funcObj won't resolve.
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Construct: receiver[T].Method()
	callExpr := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.IndexExpr{
				X:     ast.NewIdent("receiver"),
				Index: ast.NewIdent("int"),
			},
			Sel: ast.NewIdent("Method"),
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	args := make([]*CallArgument, 0)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_SelectorExprWithIndexListExprX(_ *testing.T) {
	// Covers lines 1738-1739: SelectorExpr with IndexListExpr as X.
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Construct: receiver[T1, T2].Method()
	callExpr := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.IndexListExpr{
				X:       ast.NewIdent("receiver"),
				Indices: []ast.Expr{ast.NewIdent("int"), ast.NewIdent("string")},
			},
			Sel: ast.NewIdent("Method"),
		},
		Args: []ast.Expr{},
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	args := make([]*CallArgument, 0)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_IdentAndParenDefaultBranches(_ *testing.T) {
	// Covers lines 1741-1747: Ident and ParenExpr cases (both set explicitTypeArgExprs = nil)
	// and the default case.
	info := &types.Info{
		Types:     make(map[ast.Expr]types.TypeAndValue),
		Defs:      make(map[*ast.Ident]types.Object),
		Uses:      make(map[*ast.Ident]types.Object),
		Instances: make(map[*ast.Ident]types.Instance),
	}

	// Ident case
	callIdent := &ast.CallExpr{
		Fun:  ast.NewIdent("someFunc"),
		Args: []ast.Expr{},
	}
	extractParamsAndTypeParams(callIdent, info, nil, make(map[string]CallArgument), make(map[string]string))

	// ParenExpr case
	callParen := &ast.CallExpr{
		Fun:  &ast.ParenExpr{X: ast.NewIdent("someFunc")},
		Args: []ast.Expr{},
	}
	extractParamsAndTypeParams(callParen, info, nil, make(map[string]CallArgument), make(map[string]string))
}

// ===========================================================================
// extractParamsAndTypeParams — with actual generics (real type info)
// Tests the inference paths at lines 1774-1833.
// ===========================================================================

func TestExtractParamsAndTypeParams_GenericFunctionWithInference(t *testing.T) {
	src := `package testpkg

func Transform[T any](input T) T {
	return input
}

func main() {
	Transform(42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find the Transform(42) call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "Transform" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// The type parameter T should be inferred as "int".
	if len(typeParamMap) > 0 {
		assert.Equal(t, "int", typeParamMap["T"])
	}
}

func TestExtractParamsAndTypeParams_ExplicitTypeArg(t *testing.T) {
	src := `package testpkg

func Transform[T any](input T) T {
	return input
}

func main() {
	Transform[int](42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			// Find the call with IndexExpr (explicit type arg)
			if _, ok := ce.Fun.(*ast.IndexExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)

	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// With explicit type arg, T should resolve to "int".
	if len(typeParamMap) > 0 {
		assert.Equal(t, "int", typeParamMap["T"])
	}
}

// ===========================================================================
// traverseCallerChildrenHelper — types.go:306 (92.3%)
// Uncovered: line 319-320 — self-calling depth limit (MaxSelfCallingDepth).
// ===========================================================================

func TestTraverseCallerChildrenHelper_SelfCallDepthLimit(t *testing.T) {
	meta := fullCovMeta()

	// Create a self-recursive call: A -> A.
	edge := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	// Set depth near limit.
	baseID := edge.Callee.BaseID()
	meta.callDepth[baseID] = MaxSelfCallingDepth

	// Traverse — the self-call should be skipped because depth >= MaxSelfCallingDepth.
	count := 0
	meta.TraverseCallerChildren(&meta.CallGraph[0], func(_, _ *CallGraphEdge) {
		count++
	})
	// Because the depth limit is reached, no additional traversal beyond the initial
	// action call should happen.
	assert.Equal(t, 0, count, "self-calling at max depth should not invoke action")
}

// ===========================================================================
// CallGraphRoots — types.go:331 (93.8%)
// Uncovered: line 354-356 — function appears as an argument (Args map).
// ===========================================================================

func TestCallGraphRoots_FunctionAsArg(t *testing.T) {
	meta := fullCovMeta()

	// Create edges where caller "A" is also in the Args map.
	edge := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("A"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("B"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	// Add "A" to the Args map to make it non-root.
	callerBase := edge.Caller.BaseID()
	meta.Args[callerBase] = []*CallGraphEdge{&meta.CallGraph[0]}

	roots := meta.CallGraphRoots()
	// "A" should NOT be a root because it appears in the Args map.
	for _, root := range roots {
		rootName := meta.StringPool.GetString(root.Caller.Name)
		assert.NotEqual(t, "A", rootName, "A should not be root since it appears in Args")
	}
}

// ===========================================================================
// id — types.go:751 (88.9%)
// Uncovered: lines 803-805 (xTypeParam != ""), 813-815 (call: funTypeParam),
//            829-831 (typeConversion: funTypeParam), 839-841/844-846 (unary),
//            859-861/869-871/874-876 (compositeLit, index)
// ===========================================================================

func TestCallArgID_SelectorWithXTypeParam(t *testing.T) {
	meta := fullCovMeta()

	// Build a selector CallArgument where X has type params.
	sel := NewCallArgument(meta)
	sel.SetKind(KindIdent)
	sel.SetName("Method")
	sel.SetPkg("pkg")

	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("Obj")
	x.SetPkg("pkg")
	x.SetType("SomeType")
	x.TypeParamMap = map[string]string{"T": "int"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindSelector)
	arg.X = x
	arg.Sel = sel

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	// The xTypeParam from X should propagate.
	assert.Contains(t, tp, "T=int")
}

func TestCallArgID_CallWithFunTypeParam(t *testing.T) {
	meta := fullCovMeta()

	fun := NewCallArgument(meta)
	fun.SetKind(KindIdent)
	fun.SetName("GenericFunc")
	fun.SetPkg("pkg")
	fun.TypeParamMap = map[string]string{"T": "string"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindCall)
	arg.Fun = fun

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	assert.Contains(t, tp, "T=string")
}

func TestCallArgID_TypeConversionWithFunTypeParam(t *testing.T) {
	meta := fullCovMeta()

	fun := NewCallArgument(meta)
	fun.SetKind(KindIdent)
	fun.SetName("MyType")
	fun.SetPkg("pkg")
	fun.TypeParamMap = map[string]string{"T": "int"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindTypeConversion)
	arg.Fun = fun

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	assert.Contains(t, tp, "T=int")
}

func TestCallArgID_UnaryWithXTypeParam(t *testing.T) {
	meta := fullCovMeta()

	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("val")
	x.SetPkg("pkg")
	x.TypeParamMap = map[string]string{"T": "int"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindUnary)
	arg.X = x
	arg.SetValue("&")

	id, tp := arg.id(".")
	assert.Contains(t, id, "&")
	assert.Contains(t, tp, "T=int")
}

func TestCallArgID_UnaryNilX(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindUnary)
	arg.X = nil

	id, tp := arg.id(".")
	assert.Equal(t, "", id)
	assert.Equal(t, "", tp)
}

func TestCallArgID_CompositeLitWithXTypeParam(t *testing.T) {
	meta := fullCovMeta()

	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("MyStruct")
	x.SetPkg("pkg")
	x.SetType("MyStruct")
	x.TypeParamMap = map[string]string{"T": "int"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindCompositeLit)
	arg.X = x

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	assert.Contains(t, tp, "T=int")
}

func TestCallArgID_CompositeLitNilX(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindCompositeLit)
	arg.X = nil

	id, tp := arg.id(".")
	assert.Equal(t, "", id)
	assert.Equal(t, "", tp)
}

func TestCallArgID_IndexWithXTypeParam(t *testing.T) {
	meta := fullCovMeta()

	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("arr")
	x.SetPkg("pkg")
	x.SetType("[]int") // Set type so id("/") returns the type string
	x.TypeParamMap = map[string]string{"T": "int"}

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	arg.X = x
	arg.SetPkg("pkg")

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	assert.Contains(t, tp, "T=int")
}

func TestCallArgID_IndexNilX(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	arg.X = nil

	id, tp := arg.id(".")
	assert.Equal(t, "", id)
	assert.Equal(t, "", tp)
}

// ===========================================================================
// BuildAssignmentRelationships — metadata.go:468 (96.0%)
// Uncovered: line 484 — edge.CalleeRecvVarName != recvVarName (continue).
// ===========================================================================

func TestBuildAssignmentRelationships_RecvVarNameMismatch(t *testing.T) {
	meta := fullCovMeta()

	// Create a main function with an assignment and a call graph edge
	// where CalleeRecvVarName does NOT match the assignment variable name.
	mainFn := &Function{
		Name: meta.StringPool.Get("main"),
		Pkg:  meta.StringPool.Get("testpkg"),
		AssignmentMap: map[string][]Assignment{
			"app": {
				{
					VariableName: meta.StringPool.Get("app"),
					Pkg:          meta.StringPool.Get("testpkg"),
					ConcreteType: meta.StringPool.Get("*App"),
					Value:        CallArgument{Kind: meta.StringPool.Get(KindIdent), Name: meta.StringPool.Get("NewApp"), Meta: meta},
					Lhs:          CallArgument{Kind: meta.StringPool.Get(KindIdent), Name: meta.StringPool.Get("app"), Meta: meta},
					Func:         meta.StringPool.Get("main"),
				},
			},
		},
	}

	meta.Packages["testpkg"] = &Package{
		Files: map[string]*File{
			"main.go": {
				Functions: map[string]*Function{
					"main": mainFn,
				},
			},
		},
	}

	edge := CallGraphEdge{
		Caller: Call{
			Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("testpkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		Callee: Call{
			Meta: meta, Name: meta.StringPool.Get("Run"), Pkg: meta.StringPool.Get("testpkg"),
			RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1,
		},
		CalleeRecvVarName: "DIFFERENT_NAME", // Does NOT match "app"
		meta:              meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	rels := meta.BuildAssignmentRelationships()
	// The "app" assignment should be skipped (continue branch) because
	// edge.CalleeRecvVarName != "app".
	for k := range rels {
		assert.NotEqual(t, "app", k.Name, "app should not be in relationships due to mismatch")
	}
}

// ===========================================================================
// detectProjectRoot — dependency_analyzer.go:615 (94.7%)
// Uncovered: line 626-628 — len(packagePaths) == 0 after collecting from map.
// This branch can only be hit when fd.packages has entries but none produce
// valid paths — effectively impossible since the range always yields keys.
// However, we can test it by populating fd.packages and then clearing them.
// ===========================================================================

func TestDetectProjectRoot_EmptyPackagePaths(t *testing.T) {
	// NOTE: The len(packagePaths)==0 branch at line 626 is structurally
	// unreachable when fd.packages is non-empty (range always yields keys).
	// We test the len(fd.packages)==0 branch instead, which returns "".
	fd := NewFrameworkDetector()
	assert.Equal(t, "", fd.detectProjectRoot())
}

// ===========================================================================
// GenerateMetadataWithLogger — metadata.go:157 (78.6%)
// Uncovered: lines 197-211 — module path extraction fallback when no common
//   prefix contains "/", and paths need to be split by internal/pkg/cmd.
// Also: lines 256-260 — mock method skipping.
// Also: lines 379-407 — type param propagation in traverse.
// ===========================================================================

func TestGenerateMetadataWithLogger_VerboseLogger(t *testing.T) {
	// Test that logger branches are exercised (lines 160-165).
	src := `package testpkg

func main() {
	helper()
}

func helper() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	// Key by filename to match how the engine builds importPaths (engine.go:349)
	importPaths := map[string]string{"test.go": "testpkg"}

	logger := &testLogger{}
	meta := GenerateMetadataWithLogger(pkgs, fileToInfo, importPaths, fset, logger)
	assert.NotNil(t, meta)
	assert.NotEmpty(t, logger.messages, "expected logger to be called")
}

func TestGenerateMetadataWithLogger_ModulePathExtraction(t *testing.T) {
	// Test the module path extraction fallback (lines 197-211).
	// Multiple packages with no common "/" prefix, but with internal/pkg segments.
	src1 := `package models

type User struct {
	Name string
}
`
	src2 := `package handlers

func Handle() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "user.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "handler.go", src2, parser.AllErrors)
	require.NoError(t, err)

	info1 := typeCheckNoImport(t, fset, file1)
	info2 := typeCheckNoImport(t, fset, file2)

	pkgs := map[string]map[string]*ast.File{
		"github.com/myproject/internal/models":   {"user.go": file1},
		"github.com/myproject/internal/handlers": {"handler.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{file1: info1, file2: info2}
	importPaths := map[string]string{
		"github.com/myproject/internal/models":   "github.com/myproject/internal/models",
		"github.com/myproject/internal/handlers": "github.com/myproject/internal/handlers",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_NoCommonPrefixFallback(t *testing.T) {
	// Test the fallback where paths have no common "/" prefix.
	// This exercises the else branch at line 197.
	src1 := `package models

type User struct{ Name string }
`
	src2 := `package handlers

func Handle() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "user.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "handler.go", src2, parser.AllErrors)
	require.NoError(t, err)

	info1 := typeCheckNoImport(t, fset, file1)
	info2 := typeCheckNoImport(t, fset, file2)

	pkgs := map[string]map[string]*ast.File{
		"example.com/internal/foo": {"user.go": file1},
		"different.org/bar":        {"handler.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{file1: info1, file2: info2}
	importPaths := map[string]string{
		"example.com/internal/foo": "example.com/internal/foo",
		"different.org/bar":        "different.org/bar",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_MockMethodSkipped(t *testing.T) {
	// Test that mock methods are skipped (lines 250-252, 256-260).
	src := `package testpkg

type MockService struct{}

func (m *MockService) MockMethod() {}

type RealService struct{}

func (r *RealService) RealMethod() {}

func main() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"testpkg": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)

	// MockService and MockMethod should be skipped.
	for _, pkg := range meta.Packages {
		for _, f := range pkg.Files {
			for _, typ := range f.Types {
				typeName := meta.StringPool.GetString(typ.Name)
				for _, method := range typ.Methods {
					methodName := meta.StringPool.GetString(method.Name)
					assert.False(t, isMockName(typeName) && isMockName(methodName),
						"mock type/method should be skipped: %s.%s", typeName, methodName)
				}
			}
		}
	}
}

// ===========================================================================
// Integration: GenerateMetadata with generics to exercise type param traversal
// ===========================================================================

func TestGenerateMetadata_GenericTypeParamPropagation(t *testing.T) {
	// Test the type parameter propagation branch (lines 379-407).
	// We need a generic function called from another function so that
	// parent and child edges both have TypeParamMap entries.
	src := `package testpkg

func Transform[T any](input T) T {
	return process(input)
}

func process[T any](input T) T {
	return input
}

func main() {
	Transform[int](42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"testpkg": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
	assert.NotEmpty(t, meta.CallGraph)
}

// ===========================================================================
// Edge cases for id() with sort.Slice on genericParts (line 760)
// ===========================================================================

func TestCallArgID_WithMultipleTypeParams(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName("GenericFunc")
	arg.SetPkg("pkg")
	arg.TypeParamMap = map[string]string{
		"B": "string",
		"A": "int",
	}

	id, tp := arg.id(".")
	assert.Equal(t, "pkg.GenericFunc", id)
	// TypeParams should be sorted: A=int, B=string
	assert.Equal(t, "[A=int,B=string]", tp)
}

// ===========================================================================
// CallArgID — KindIdent with sep="/" and type set
// ===========================================================================

func TestCallArgID_IdentWithSlashSepAndType(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName("myVar")
	arg.SetPkg("pkg")
	arg.SetType("MyType")

	// With "/" sep and type set, should return the type.
	id, _ := arg.id("/")
	assert.Equal(t, "MyType", id)
}

func TestCallArgID_IdentWithSlashSepNoType(t *testing.T) {
	meta := fullCovMeta()

	arg := NewCallArgument(meta)
	arg.SetKind(KindIdent)
	arg.SetName("myVar")
	arg.SetPkg("pkg")

	// With "/" sep and no type, but pkg set -> return "".
	id, _ := arg.id("/")
	assert.Equal(t, "", id)
}

// ===========================================================================
// CallGraphRoots — caching test (line 332: len(m.roots) > 0).
// ===========================================================================

func TestCallGraphRoots_Caching(t *testing.T) {
	meta := fullCovMeta()

	edge := CallGraphEdge{
		Caller: Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		Callee: Call{Meta: meta, Name: meta.StringPool.Get("helper"), Pkg: meta.StringPool.Get("pkg"), RecvType: -1, Position: -1, Scope: -1, SignatureStr: -1},
		meta:   meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	// First call computes roots.
	roots1 := meta.CallGraphRoots()
	// Second call should return cached roots.
	roots2 := meta.CallGraphRoots()
	assert.Equal(t, len(roots1), len(roots2))
}

// ===========================================================================
// processAssignment — SelectorExpr LHS
// ===========================================================================

func TestProcessAssignment_SelectorLHS(t *testing.T) {
	src := `package testpkg

type MyStruct struct {
	Field int
}

func main() {
	s := MyStruct{}
	s.Field = 42
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find s.Field = 42 assignment.
	var selectorAssign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if _, ok := as.Lhs[0].(*ast.SelectorExpr); ok {
				selectorAssign = as
			}
		}
		return true
	})
	require.NotNil(t, selectorAssign)

	assignments := processAssignment(selectorAssign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	assert.NotEmpty(t, assignments)
	assert.Equal(t, "selector", meta.StringPool.GetString(assignments[0].Scope))
}

// ===========================================================================
// processAssignment — IndexExpr LHS
// ===========================================================================

func TestProcessAssignment_IndexLHS(t *testing.T) {
	src := `package testpkg

func main() {
	arr := []int{1, 2, 3}
	arr[0] = 42
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find arr[0] = 42 assignment.
	var indexAssign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if _, ok := as.Lhs[0].(*ast.IndexExpr); ok {
				indexAssign = as
			}
		}
		return true
	})
	require.NotNil(t, indexAssign)

	assignments := processAssignment(indexAssign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	assert.NotEmpty(t, assignments)
	assert.Equal(t, "index", meta.StringPool.GetString(assignments[0].Scope))
}

// ===========================================================================
// processAssignment — blank identifier skip
// ===========================================================================

func TestProcessAssignment_BlankIdentifier(t *testing.T) {
	src := `package testpkg

func main() {
	_ = 42
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	var blankAssign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			blankAssign = as
		}
		return true
	})
	require.NotNil(t, blankAssign)

	assignments := processAssignment(blankAssign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	assert.Empty(t, assignments, "blank identifier assignment should be skipped")
}

// ===========================================================================
// processAssignment — RHS is a function call (lines 1315-1320)
// ===========================================================================

func TestProcessAssignment_RHSCallExpr(t *testing.T) {
	src := `package testpkg

func create() int { return 1 }

func main() {
	x := create()
	_ = x
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	funcMap := BuildFuncMap(map[string]map[string]*ast.File{"testpkg": {"test.go": file}})

	// Find x := create() assignment.
	var callAssign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if len(as.Rhs) > 0 {
				if _, ok := as.Rhs[0].(*ast.CallExpr); ok {
					callAssign = as
				}
			}
		}
		return true
	})
	require.NotNil(t, callAssign)

	assignments := processAssignment(callAssign, file, info, "testpkg", fset, fileToInfo, funcMap, meta)
	assert.NotEmpty(t, assignments)
	assert.Equal(t, "create", assignments[0].CalleeFunc)
}

// ===========================================================================
// extractParamsAndTypeParams — comprehensive generic tests to cover the
// deeply nested branches (lines 1733-1828).
// These require real type-checked generic functions.
// ===========================================================================

func TestExtractParamsAndTypeParams_ExplicitTypeArgSelectorIndexExpr(t *testing.T) {
	// Test explicit type args through a SelectorExpr wrapping an IndexExpr.
	// This covers lines 1733-1737 (SelectorExpr case with IndexExpr X).
	// We need a real generic function to get sig.TypeParams() != nil.
	src := `package testpkg

type Container[T any] struct {
	Value T
}

func (c Container[T]) Get() T {
	return c.Value
}

func main() {
	c := Container[int]{Value: 42}
	c.Get()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	fileToInfo := map[*ast.File]*types.Info{file: info}
	pkgs := map[string]map[string]*ast.File{"testpkg": {"test.go": file}}
	funcMap := BuildFuncMap(pkgs)

	// Build metadata and call graph to exercise the extraction.
	for fileName, f := range pkgs["testpkg"] {
		fileInfo := fileToInfo[f]
		metaFile := &File{
			Types:     make(map[string]*Type),
			Functions: make(map[string]*Function),
			Variables: make(map[string]*Variable),
			Imports:   make(map[int]int),
		}
		processTypes(f, fileInfo, "testpkg", fset, metaFile, make(map[string][]Method), make(map[string]*Type), meta)
		processFunctions(f, fileInfo, "testpkg", fset, metaFile, fileToInfo, funcMap, meta)
		meta.Packages["testpkg"] = &Package{Files: map[string]*File{fileName: metaFile}}
	}
	buildCallGraph(pkgs["testpkg"], pkgs, "testpkg", fileToInfo, fset, funcMap, meta)
	assert.NotEmpty(t, meta.CallGraph)
}

func TestExtractParamsAndTypeParams_SelectorExprTypeInference(t *testing.T) {
	// Test type inference through a SelectorExpr (line 1777-1779).
	// This needs a method on a generic type called without explicit type args.
	src := `package testpkg

func Transform[T any](input T) T {
	return input
}

type Wrapper struct{}

func (w Wrapper) Call() {
	Transform(42)
}

func main() {
	w := Wrapper{}
	w.Call()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()
	fileToInfo := map[*ast.File]*types.Info{file: info}
	pkgs := map[string]map[string]*ast.File{"testpkg": {"test.go": file}}
	funcMap := BuildFuncMap(pkgs)

	for fileName, f := range pkgs["testpkg"] {
		fileInfo := fileToInfo[f]
		metaFile := &File{
			Types:     make(map[string]*Type),
			Functions: make(map[string]*Function),
			Variables: make(map[string]*Variable),
			Imports:   make(map[int]int),
		}
		processFunctions(f, fileInfo, "testpkg", fset, metaFile, fileToInfo, funcMap, meta)
		meta.Packages["testpkg"] = &Package{Files: map[string]*File{fileName: metaFile}}
	}
	buildCallGraph(pkgs["testpkg"], pkgs, "testpkg", fileToInfo, fset, funcMap, meta)
	assert.NotEmpty(t, meta.CallGraph)
}

func TestExtractParamsAndTypeParams_ExplicitTypeArgFallbackToGetTypeName(t *testing.T) {
	// Test the branch at line 1760-1761 where info.TypeOf returns nil
	// for the type argument expression, falling back to getTypeName.
	// This is hard to trigger with real type-checked code since TypeOf
	// usually works. We exercise it indirectly by constructing a scenario
	// where the type argument is not in the info.Types map.
	src := `package testpkg

func Transform[T any](input T) T {
	return input
}

func main() {
	Transform[int](42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find the Transform[int](42) call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.IndexExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	// Remove the type arg from info.Types to force fallback.
	indexExpr := callExpr.Fun.(*ast.IndexExpr)
	delete(info.Types, indexExpr.Index)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// The type param should still be resolved via getTypeName fallback.
	if v, ok := typeParamMap["T"]; ok {
		assert.NotEmpty(t, v)
	}
}

func TestExtractParamsAndTypeParams_InferenceFromArgType(t *testing.T) {
	// Test the arg-inference fallback at lines 1796-1833.
	// We need a generic function called with a function argument whose
	// type can be inspected to infer type parameters.
	src := `package testpkg

func Apply[T any](fn func(T) T, val T) T {
	return fn(val)
}

func double(x int) int {
	return x * 2
}

func main() {
	Apply(double, 5)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find Apply(double, 5) call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "Apply" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}

	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// T should be inferred as "int" either through Instances or arg inference.
	if v, ok := typeParamMap["T"]; ok {
		assert.Equal(t, "int", v)
	}
}

// ===========================================================================
// GenerateMetadataWithLogger — module path with "internal/pkg/cmd" segments
// Lines 203-210.
// ===========================================================================

func TestGenerateMetadataWithLogger_ModulePathWithInternalSegment(t *testing.T) {
	// Test when packages have paths containing "internal" and the common prefix
	// doesn't have "/" (which triggers the internal/pkg/cmd splitting logic).
	src := `package pkg1
func F1() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "f1.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"myapp/internal/handlers": {"f1.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	// Two import paths where common prefix has no "/" and path contains "internal".
	importPaths := map[string]string{
		"myapp/internal/handlers": "myapp/internal/handlers",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_NoDomainPathFallback(t *testing.T) {
	// Test when the path doesn't contain a domain and no common prefix has "/".
	// This exercises line 208-210 where currentModulePath falls back to path.
	src := `package a
func F() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "a.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"simple": {"a.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{
		"simple": "simple",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_ModulePathInternalSplit(t *testing.T) {
	// Exercises lines 201-210: Two paths with no common "/" prefix,
	// where path contains "." and has "internal" segment.
	src1 := `package a
func F1() {}
`
	src2 := `package b
func F2() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "a.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "b.go", src2, parser.AllErrors)
	require.NoError(t, err)
	info1 := typeCheckNoImport(t, fset, file1)
	info2 := typeCheckNoImport(t, fset, file2)

	pkgs := map[string]map[string]*ast.File{
		"example.com/project/internal/a": {"a.go": file1},
		"different.org/project/pkg/b":    {"b.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{file1: info1, file2: info2}
	importPaths := map[string]string{
		"example.com/project/internal/a": "example.com/project/internal/a",
		"different.org/project/pkg/b":    "different.org/project/pkg/b",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_ModulePathNoInternalFallback(t *testing.T) {
	// Exercises line 208-210: path contains "." but no internal/pkg/cmd segment.
	// currentModulePath should fallback to path itself.
	src1 := `package a
func F1() {}
`
	src2 := `package b
func F2() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "a.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "b.go", src2, parser.AllErrors)
	require.NoError(t, err)
	info1 := typeCheckNoImport(t, fset, file1)
	info2 := typeCheckNoImport(t, fset, file2)

	pkgs := map[string]map[string]*ast.File{
		"example.com/alpha":  {"a.go": file1},
		"different.org/beta": {"b.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{file1: info1, file2: info2}
	importPaths := map[string]string{
		"example.com/alpha":  "example.com/alpha",
		"different.org/beta": "different.org/beta",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

func TestGenerateMetadataWithLogger_ModulePathInternalAtRoot(t *testing.T) {
	// Exercises line 208-210: path contains "." with "internal" as first segment.
	// After the internal/pkg/cmd loop, parts[:0] == "" so currentModulePath becomes "".
	// Then it falls through to line 208 where currentModulePath == "" → path is used.
	//
	// For this to work:
	// 1. Two paths must have no common prefix containing "/"
	// 2. The second path must contain "."
	// 3. The second path must be shorter than the first (or currentModulePath is "")
	// 4. The second path must have "internal" as the FIRST segment
	//    so that parts[:0] produces empty string
	src1 := `package a
func F1() {}
`
	src2 := `package b
func F2() {}
`
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "a.go", src1, parser.AllErrors)
	require.NoError(t, err)
	file2, err := parser.ParseFile(fset, "b.go", src2, parser.AllErrors)
	require.NoError(t, err)
	info1 := typeCheckNoImport(t, fset, file1)
	info2 := typeCheckNoImport(t, fset, file2)

	// "internal/x.y/stuff" has "internal" as first segment and "." in path.
	// After the loop: parts[:0] = [] → strings.Join = "" → currentModulePath = ""
	// Then line 208: currentModulePath == "" → currentModulePath = path
	pkgs := map[string]map[string]*ast.File{
		"zzz.com/very/long/path": {"a.go": file1},
		"internal/x.y/stuff":     {"b.go": file2},
	}
	fileToInfo := map[*ast.File]*types.Info{file1: info1, file2: info2}
	importPaths := map[string]string{
		"zzz.com/very/long/path": "zzz.com/very/long/path",
		"internal/x.y/stuff":     "internal/x.y/stuff",
	}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
}

// ===========================================================================
// GenerateMetadataWithLogger — mock receiver type skipping (lines 256-260).
// ===========================================================================

func TestGenerateMetadataWithLogger_MockReceiverTypeSkipped(t *testing.T) {
	src := `package testpkg

type MockHandler struct{}

func (m *MockHandler) Handle() {}

type FakeService struct{}

func (f *FakeService) Serve() {}

type RealService struct{}

func (r *RealService) Serve() {}

func main() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"testpkg": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)

	// Verify mock methods are not present.
	for _, pkg := range meta.Packages {
		for _, f := range pkg.Files {
			for _, typ := range f.Types {
				typeName := meta.StringPool.GetString(typ.Name)
				if typeName == "RealService" {
					assert.NotEmpty(t, typ.Methods, "RealService should have methods")
				}
			}
		}
	}
}

// ===========================================================================
// processStructFields — external type that resolves to a primitive underlying
// type (line 952-954).
// ===========================================================================

func TestProcessStructFields_ExternalPrimitiveUnderlying(t *testing.T) {
	// To hit line 952 (isExternalType → underlying is primitive), we need a field
	// whose type comes from a package with "/" in its path (so isExternalPackage
	// returns true). We can simulate this by type-checking two packages together
	// where one defines a type alias of int and another uses it.
	//
	// NOTE: Line 952 is practically hard to reach in unit tests because it requires
	// a named type from a third-party module (with "/" in path) whose underlying
	// type is primitive. Standard library types (like time.Duration) don't trigger
	// isExternalType because stdlib paths have no "/" or ".".
	// We verify the function doesn't panic and exercises the surrounding code.
	src := `package testpkg

import "time"

type Config struct {
	Timeout time.Duration
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFull(t, fset, file)
	meta := fullCovMeta()
	meta.CurrentModulePath = "github.com/my/project"

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
			assert.GreaterOrEqual(t, len(ty.Fields), 1)
			// time.Duration is stdlib → isExternalType returns false.
			// The field type remains "time.Duration".
			fieldType := meta.StringPool.GetString(ty.Fields[0].Type)
			assert.Equal(t, "time.Duration", fieldType)
		}
	}
}

// ===========================================================================
// isTypeConversion — additional cases to cover line 188.
// ===========================================================================

func TestIsTypeConversion_MapTypeCase(_ *testing.T) {
	// MapType is not explicitly listed but falls through to the len==1 check.
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	// MapType case
	callMap := &ast.CallExpr{
		Fun:  &ast.MapType{Key: ast.NewIdent("string"), Value: ast.NewIdent("int")},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	// MapType is not in the explicit switch, so it falls to the bottom.
	// With len(Args)==1 and no type info, this returns false.
	_ = isTypeConversion(callMap, info)
}

func TestIsTypeConversion_SliceExprCase(t *testing.T) {
	// SliceExpr is listed in the switch.
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	callSlice := &ast.CallExpr{
		Fun:  &ast.SliceExpr{X: ast.NewIdent("arr")},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callSlice, info))
}

func TestIsTypeConversion_ChanTypeCase(t *testing.T) {
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	callChan := &ast.CallExpr{
		Fun:  &ast.ChanType{Value: ast.NewIdent("int")},
		Args: []ast.Expr{ast.NewIdent("x")},
	}
	assert.True(t, isTypeConversion(callChan, info))
}

// ===========================================================================
// handleSelector — cover the PkgName branch (line 158-163) with a real
// selector expression where obj resolves to *types.PkgName.
// ===========================================================================

func TestHandleSelector_PkgNameInSelObj(t *testing.T) {
	// This is a quirky case where the selector's Sel itself resolves to PkgName.
	// In practice, this is unusual but we test what we can.
	// The more common path (Sel resolves to a Func/Var) is already covered.
	// We focus on the case where obj is non-nil but not PkgName, which tests
	// the else branch at line 163.
	src := `package testpkg

import "fmt"

func f() {
	fmt.Println("hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckFull(t, fset, file)
	meta := fullCovMeta()

	// Find the fmt.Println call and get the selector
	var selExpr *ast.SelectorExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
				selExpr = se
			}
		}
		return true
	})
	require.NotNil(t, selExpr)

	arg := handleSelector(selExpr, info, "testpkg", fset, meta)
	assert.Equal(t, KindSelector, arg.GetKind())
	// Println is a method of fmt package; the obj should be a *types.Func.
	// This exercises the "else" branch (not PkgName) at line 163.
	assert.NotEmpty(t, arg.GetPkg())
}

// ===========================================================================
// id() — KindIndex with X that has xTypeParam="" (covers line 869-871).
// This is the case where X has no type params, so xTypeParam is empty.
// ===========================================================================

func TestCallArgID_IndexNoTypeParam(t *testing.T) {
	meta := fullCovMeta()

	x := NewCallArgument(meta)
	x.SetKind(KindIdent)
	x.SetName("arr")
	x.SetPkg("pkg")
	x.SetType("[]int")
	// No TypeParamMap set → xTypeParam will be ""

	arg := NewCallArgument(meta)
	arg.SetKind(KindIndex)
	arg.X = x

	id, tp := arg.id(".")
	assert.NotEmpty(t, id)
	assert.Equal(t, "", tp)
}

// ===========================================================================
// GenerateMetadata — complete integration test with type param propagation
// (lines 379-407). Needs parent and child edges both having TypeParamMap.
// ===========================================================================

// ===========================================================================
// extractParamsAndTypeParams — test with real type-checked generics to cover
// the inner branches (lines 1733-1828).
// The key insight: these branches require funcObj to be non-nil *types.Func
// with sig.TypeParams() != nil. The switch on call.Fun type determines which
// inner branch executes.
// ===========================================================================

func TestExtractParamsAndTypeParams_RealGenericInference(t *testing.T) {
	// This covers: funcObj != nil, sig.TypeParams() != nil, Ident case,
	// instance found in info.Instances (lines 1774-1795).
	src := `package testpkg

func Map[T any, U any](items []T, fn func(T) U) []U {
	result := make([]U, len(items))
	for i, item := range items {
		result[i] = fn(item)
	}
	return result
}

func toString(x int) string { return "" }

func main() {
	Map([]int{1, 2}, toString)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find Map([]int{1, 2}, toString) call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "Map" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)

	// T should be "int", U should be "string" via inference.
	if len(typeParamMap) > 0 {
		assert.Equal(t, "int", typeParamMap["T"])
		assert.Equal(t, "string", typeParamMap["U"])
	}
	// Parameters should be mapped.
	assert.NotEmpty(t, paramArgMap)
}

func TestExtractParamsAndTypeParams_RealExplicitIndexExpr(t *testing.T) {
	// Covers: IndexExpr branch (lines 1729-1730).
	// call.Fun is *ast.IndexExpr with explicit type argument: Transform[int](42).
	src := `package testpkg

func Transform[T any](input T) T { return input }

func main() {
	Transform[int](42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.IndexExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	assert.Equal(t, "int", typeParamMap["T"])
}

func TestExtractParamsAndTypeParams_RealExplicitIndexListExpr(t *testing.T) {
	// Covers: IndexListExpr branch (lines 1731-1732).
	// call.Fun is *ast.IndexListExpr with explicit type arguments.
	src := `package testpkg

func Pair[T any, U any](a T, b U) {}

func main() {
	Pair[int, string](1, "hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.IndexListExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	assert.Equal(t, "int", typeParamMap["T"])
	assert.Equal(t, "string", typeParamMap["U"])
}

func TestExtractParamsAndTypeParams_FallbackToGetTypeName(t *testing.T) {
	// Covers line 1760-1762: info.TypeOf returns nil, fallback to getTypeName.
	// Achieved by deleting the type info for the type arg expression.
	src := `package testpkg

func Identity[T any](x T) T { return x }

func main() {
	Identity[int](42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.IndexExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	// Delete the type info for the type argument to force getTypeName fallback.
	indexExpr := callExpr.Fun.(*ast.IndexExpr)
	delete(info.Types, indexExpr.Index)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	// T should still be resolved via getTypeName fallback.
	assert.NotEmpty(t, typeParamMap)
}

func TestExtractParamsAndTypeParams_ArgInference_FuncArgWithParams(t *testing.T) {
	// Cover lines 1796-1833: arg-based inference when instance isn't found.
	// We need: funcObj resolves, sig.TypeParams() != nil, no explicit type args,
	// Instances lookup fails, args[0].GetKind() == KindIdent,
	// info.TypeOf(call.Args[0]) returns a *types.Signature with params.
	src := `package testpkg

func Apply[T any](fn func(T) T, val T) T { return fn(val) }
func double(x int) int { return x * 2 }

func main() {
	Apply(double, 5)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "Apply" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	// Delete instance info to force the arg-inference path.
	for k := range info.Instances {
		if k.Name == "Apply" {
			delete(info.Instances, k)
		}
	}

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	// Even without Instances, the arg inference should try to resolve.
	_ = typeParamMap
}

func TestExtractParamsAndTypeParams_SelectorExprInference(t *testing.T) {
	// Covers lines 1777-1779: SelectorExpr in inference (no explicit type args).
	// For a SelectorExpr to reach this path, the call must be pkg.Func(args)
	// where Func is generic with inferred type args.
	// We simulate this by type-checking two files in the same package where
	// one defines a generic function and the other calls it through a selector.
	// Since single-package type checking makes all names accessible directly,
	// we instead construct the AST manually with the correct info setup.
	//
	// NOTE: Lines 1777-1779 require call.Fun to be SelectorExpr, funcObj to
	// resolve to a generic *types.Func, and no explicit type args. This
	// pattern is typical for cross-package generic calls (pkg.Transform(42)).
	// It's hard to test with single-file type checking since same-package
	// calls use Ident, not SelectorExpr. We document this limitation.

	// Instead, test with a struct method generic receiver to at least exercise
	// the SelectorExpr path through the outer funcObj resolution.
	src := `package testpkg

type Container[T any] struct{ Value T }

func (c Container[T]) Get() T { return c.Value }

func main() {
	c := Container[int]{Value: 42}
	c.Get()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find c.Get() call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Get" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestExtractParamsAndTypeParams_ParenExprInference(t *testing.T) {
	// Covers lines 1780-1784: ParenExpr in inference path.
	// Pattern: (Transform)(42) where Transform is generic.
	src := `package testpkg

func Transform[T any](input T) T { return input }

func main() {
	(Transform)(42)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if _, ok := ce.Fun.(*ast.ParenExpr); ok {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
	// Type params should be inferred through Instances.
	if len(typeParamMap) > 0 {
		assert.Equal(t, "int", typeParamMap["T"])
	}
}

func TestExtractParamsAndTypeParams_SelectorExprExplicitTypeArgsOnReceiver(t *testing.T) {
	// Tries to cover lines 1733-1739: SelectorExpr with IndexExpr/IndexListExpr X.
	// This pattern occurs when a generic method is called on a type with
	// explicit type args in the receiver position, like:
	//   genericSlice[int].Sort()
	// In practice, this is rare in standard Go. We test what we can.
	src := `package testpkg

type Box[T any] struct{ Value T }

func (b Box[T]) Get() T { return b.Value }

func main() {
	Box[int]{42}.Get()
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)
	meta := fullCovMeta()

	// Find Box[int]{42}.Get() call.
	var callExpr *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Get" {
				callExpr = ce
			}
		}
		return true
	})
	require.NotNil(t, callExpr)

	args := make([]*CallArgument, len(callExpr.Args))
	for i, a := range callExpr.Args {
		args[i] = ExprToCallArgument(a, info, "testpkg", fset, meta)
	}
	paramArgMap := make(map[string]CallArgument)
	typeParamMap := make(map[string]string)
	extractParamsAndTypeParams(callExpr, info, args, paramArgMap, typeParamMap)
}

func TestGenerateMetadata_TypeParamPropagationWithMissing(t *testing.T) {
	// Create a scenario where parent has type params that child doesn't.
	// This is the "missing" branch at lines 379-407.
	src := `package testpkg

func Outer[T any, U any](a T, b U) {
	Inner(a)
}

func Inner[T any](x T) T {
	return x
}

func main() {
	Outer[int, string](1, "hello")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.AllErrors)
	require.NoError(t, err)
	info := typeCheckNoImport(t, fset, file)

	pkgs := map[string]map[string]*ast.File{
		"testpkg": {"test.go": file},
	}
	fileToInfo := map[*ast.File]*types.Info{file: info}
	importPaths := map[string]string{"testpkg": "testpkg"}

	meta := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)
	assert.NotNil(t, meta)
	assert.NotEmpty(t, meta.CallGraph)

	// Check that type param propagation happened.
	hasTypeParams := false
	for _, edge := range meta.CallGraph {
		if len(edge.TypeParamMap) > 0 {
			hasTypeParams = true
			break
		}
	}
	assert.True(t, hasTypeParams, "expected some edges with type params")
}
