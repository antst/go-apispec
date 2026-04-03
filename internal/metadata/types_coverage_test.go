package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// makeTestMeta creates a Metadata with an initialized StringPool.
func makeTestMeta() *Metadata {
	return &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
	}
}

// makeTestArg creates a CallArgument wired to the given Metadata with all int fields at -1.
func makeTestArg(meta *Metadata) *CallArgument {
	return &CallArgument{
		Kind:            -1,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
		Meta:            meta,
	}
}

// --- StringPool ---

func TestStringPool_Finalize(t *testing.T) {
	sp := NewStringPool()
	sp.Get("hello")
	sp.Finalize()
	// Finalize currently does nothing (map nilling is commented out), but calling it must not panic.
	assert.Equal(t, "hello", sp.GetString(0))
}

func TestStringPool_GetEmptyString(t *testing.T) {
	sp := NewStringPool()
	assert.Equal(t, -1, sp.Get(""))
}

func TestStringPool_GetString_OutOfBounds(t *testing.T) {
	sp := NewStringPool()
	assert.Equal(t, "", sp.GetString(-1))
	assert.Equal(t, "", sp.GetString(999))
}

func TestStringPool_Get_NilMap(t *testing.T) {
	sp := &StringPool{} // strings map is nil
	assert.Equal(t, -1, sp.Get("anything"))
}

// --- Metadata: GetCallersOfFunction / GetCalleesOfFunction ---

func TestGetCallersOfFunction(t *testing.T) {
	meta := makeTestMeta()

	edge := CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Run"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		Callee: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Helper"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		meta: meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	callers := meta.GetCallersOfFunction("mypkg", "Run")
	assert.Len(t, callers, 1)

	// Non-existent function
	assert.Empty(t, meta.GetCallersOfFunction("mypkg", "NotExist"))
}

func TestGetCalleesOfFunction(t *testing.T) {
	meta := makeTestMeta()

	edge := CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Run"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		Callee: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Helper"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		meta: meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	callees := meta.GetCalleesOfFunction("mypkg", "Helper")
	assert.Len(t, callees, 1)

	assert.Empty(t, meta.GetCalleesOfFunction("mypkg", "NotExist"))
}

// --- Metadata: GetCallersOfMethod / GetCalleesOfMethod ---

func TestGetCallersOfMethod(t *testing.T) {
	meta := makeTestMeta()

	edge := CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Handle"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: meta.StringPool.Get("Server"),
		},
		Callee: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("logRequest"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		meta: meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	callers := meta.GetCallersOfMethod("mypkg", "Server", "Handle")
	assert.Len(t, callers, 1)

	assert.Empty(t, meta.GetCallersOfMethod("mypkg", "Server", "NotExist"))
}

func TestGetCalleesOfMethod(t *testing.T) {
	meta := makeTestMeta()

	edge := CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("main"),
			Pkg:      meta.StringPool.Get("main"),
			RecvType: -1,
		},
		Callee: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Serve"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: meta.StringPool.Get("Server"),
		},
		meta: meta,
	}
	meta.CallGraph = []CallGraphEdge{edge}
	meta.BuildCallGraphMaps()

	callees := meta.GetCalleesOfMethod("mypkg", "Server", "Serve")
	assert.Len(t, callees, 1)

	assert.Empty(t, meta.GetCalleesOfMethod("mypkg", "Server", "NotExist"))
}

// --- IsSubset ---

func TestIsSubset(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, []string{"x"}, true},
		{"a empty", []string{}, []string{"x"}, true},
		{"a subset of b", []string{"a", "b"}, []string{"a", "b", "c"}, true},
		{"a equals b", []string{"a"}, []string{"a"}, true},
		{"a not subset", []string{"d"}, []string{"a", "b"}, false},
		{"partial overlap", []string{"a", "d"}, []string{"a", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSubset(tt.a, tt.b))
		})
	}
}

// --- ExtractGenericTypes ---

func TestExtractGenericTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no brackets", "pkg.Func", []string{}},
		{"simple types", "pkg.Func[int,string]", []string{"int", "string"}},
		{"key=value", "pkg.Func[T=int,U=string]", []string{"int", "string"}},
		{"mixed", "pkg.Func[T=int,string]", []string{"int", "string"}},
		{"single type", "pkg.Func[MyType]", []string{"MyType"}},
		{"empty brackets", "pkg.Func[]", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGenericTypes(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- TypeEdges ---

func TestTypeEdges_NoGenericTypes(t *testing.T) {
	meta := makeTestMeta()

	edge := &CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Run"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
		},
		meta: meta,
	}
	edges := []*CallGraphEdge{edge}

	// id with no brackets -> returns all edges
	result := TypeEdges("mypkg.Run", edges)
	assert.Len(t, result, 1)
}

func TestTypeEdges_WithGenericTypes(t *testing.T) {
	meta := makeTestMeta()

	edge := &CallGraphEdge{
		Caller: Call{
			Meta:     meta,
			Name:     meta.StringPool.Get("Run"),
			Pkg:      meta.StringPool.Get("mypkg"),
			RecvType: -1,
			Position: meta.StringPool.Get("file.go:10"),
		},
		TypeParamMap: map[string]string{"T": "int"},
		meta:         meta,
	}
	// Set Edge on the Caller so buildIdentifier picks up TypeParamMap
	edge.Caller.Edge = edge
	edges := []*CallGraphEdge{edge}

	// id has generic type that matches
	result := TypeEdges("mypkg.Run[T=int]", edges)
	assert.Len(t, result, 1)

	// id has generic type that does not match
	result = TypeEdges("mypkg.Run[T=string]", edges)
	assert.Empty(t, result)
}

// --- Call.GetScope / Call.SetScope ---

func TestCall_GetScope_SetScope(t *testing.T) {
	meta := makeTestMeta()
	c := &Call{Meta: meta, Scope: -1, RecvType: -1}
	assert.Equal(t, "", c.GetScope())

	c.SetScope("private")
	assert.Equal(t, "private", c.GetScope())
}

// --- Call.ID, GenericID, BaseID ---

func TestCall_ID(t *testing.T) {
	meta := makeTestMeta()
	c := &Call{
		Meta:     meta,
		Name:     meta.StringPool.Get("Run"),
		Pkg:      meta.StringPool.Get("mypkg"),
		Position: meta.StringPool.Get("file.go:5"),
		RecvType: -1,
	}
	// ID() defaults to InstanceID()
	id := c.ID()
	assert.Contains(t, id, "mypkg")
	assert.Contains(t, id, "Run")
}

func TestCall_GenericID(t *testing.T) {
	meta := makeTestMeta()
	edge := &CallGraphEdge{
		TypeParamMap: map[string]string{"T": "int"},
		meta:         meta,
	}
	c := &Call{
		Meta:     meta,
		Edge:     edge,
		Name:     meta.StringPool.Get("Run"),
		Pkg:      meta.StringPool.Get("mypkg"),
		Position: meta.StringPool.Get("file.go:5"),
		RecvType: -1,
	}
	genID := c.GenericID()
	assert.Contains(t, genID, "T=int")
	assert.NotContains(t, genID, "file.go:5")
}

func TestCall_BaseID_Cached(t *testing.T) {
	meta := makeTestMeta()
	c := &Call{
		Meta:     meta,
		Name:     meta.StringPool.Get("Run"),
		Pkg:      meta.StringPool.Get("mypkg"),
		RecvType: -1,
	}
	base1 := c.BaseID()
	base2 := c.BaseID() // should hit cache
	assert.Equal(t, base1, base2)
}

func TestCall_BuildIdentifier_NilMeta(t *testing.T) {
	c := &Call{}
	// Should not panic; builds identifier with empty strings
	id := c.BaseID()
	// With empty pkg and name, the identifier constructs "." (pkg.name with empty values)
	assert.NotEmpty(t, id) // exercises the nil Meta path in buildIdentifier
}

// --- CallArgument setter/getter pairs ---

func TestCallArgument_SetRaw_GetRaw(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	assert.Equal(t, "", arg.GetRaw())

	arg.SetRaw("someRaw")
	assert.Equal(t, "someRaw", arg.GetRaw())
}

func TestCallArgument_SetGenericTypeName_GetGenericTypeName(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	assert.Equal(t, "", arg.GetGenericTypeName())

	arg.SetGenericTypeName("TRequest")
	assert.Equal(t, "TRequest", arg.GetGenericTypeName())
}

func TestCallArgument_SetName_GetName(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	assert.Equal(t, "", arg.GetName())

	arg.SetName("myName")
	assert.Equal(t, "myName", arg.GetName())
}

func TestCallArgument_GetRaw_NilMeta(t *testing.T) {
	// GetRaw checks a != nil && a.Meta != nil
	var nilArg *CallArgument
	assert.Equal(t, "", nilArg.GetRaw())

	arg := &CallArgument{Raw: -1}
	assert.Equal(t, "", arg.GetRaw())
}

func TestCallArgument_GetKind_NilChecks(t *testing.T) {
	// nil CallArgument
	var nilArg *CallArgument
	assert.Equal(t, "", nilArg.GetKind())

	// Non-nil but Meta is nil
	arg := &CallArgument{Kind: 0}
	assert.Equal(t, "", arg.GetKind())
}

// --- NewCallArgument ---

func TestNewCallArgument_Success(t *testing.T) {
	meta := makeTestMeta()
	arg := NewCallArgument(meta)
	assert.NotNil(t, arg)
	assert.Equal(t, -1, arg.Kind)
	assert.Equal(t, -1, arg.Name)
	assert.Equal(t, -1, arg.Value)
	assert.Equal(t, -1, arg.Raw)
	assert.Equal(t, -1, arg.Pkg)
	assert.Equal(t, -1, arg.Type)
	assert.Equal(t, -1, arg.Position)
	assert.Equal(t, -1, arg.ResolvedType)
	assert.Equal(t, -1, arg.GenericTypeName)
	assert.Same(t, meta, arg.Meta)
}

func TestNewCallArgument_PanicsOnNilMeta(t *testing.T) {
	assert.Panics(t, func() {
		NewCallArgument(nil)
	})
}

// --- CallArgument.GetResolvedType ---

func TestCallArgument_GetResolvedType_Set(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetResolvedType("*User")
	assert.Equal(t, "*User", arg.GetResolvedType())
}

func TestCallArgument_GetResolvedType_Unset_NilMeta(t *testing.T) {
	arg := &CallArgument{ResolvedType: -1, Meta: nil}
	// When ResolvedType is -1 and Meta is nil, should return ""
	// Can't call processFunctionCallReturnType without Meta
	assert.Equal(t, "", arg.GetResolvedType())
}

func TestCallArgument_GetResolvedType_TriggersProcessFunctionCallReturnType(t *testing.T) {
	meta := makeTestMeta()

	// Set up a function with a resolved return type in the metadata
	pkgName := "mypkg"
	funcName := "NewUser"
	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Functions: map[string]*Function{
					funcName: {
						Name: meta.StringPool.Get(funcName),
						Pkg:  meta.StringPool.Get(pkgName),
						Signature: CallArgument{
							Meta:         meta,
							Kind:         meta.StringPool.Get(KindFuncType),
							ResolvedType: meta.StringPool.Get("*User"),
						},
					},
				},
			},
		},
	}

	// Create a call argument that represents a call to NewUser
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)
	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName(funcName)
	funArg.SetPkg(pkgName)
	arg.Fun = funArg

	// GetResolvedType should trigger processFunctionCallReturnType
	// (even though the final return of GetResolvedType is "" for the fallthrough case,
	// it still exercises the code)
	_ = arg.GetResolvedType()
	// After processing, the arg should have a resolved type set
	assert.Equal(t, "*User", arg.GetResolvedType())
}

// --- CallArgument.ID ---

func TestCallArgument_ID_Ident(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("myVar")
	arg.SetPkg("mypkg")

	id := arg.ID()
	assert.Equal(t, "mypkg.myVar", id)
}

func TestCallArgument_ID_Ident_WithPosition(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("myVar")
	arg.SetPosition("file.go:10")

	id := arg.ID()
	assert.Equal(t, "myVar@file.go:10", id)
}

func TestCallArgument_ID_Literal(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindLiteral)
	arg.SetValue("42")

	id := arg.ID()
	assert.Equal(t, "42", id)
}

func TestCallArgument_ID_Selector(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("http")
	xArg.SetPkg("net/http")
	arg.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("StatusOK")
	arg.Sel = selArg

	id := arg.ID()
	assert.Contains(t, id, "StatusOK")
}

func TestCallArgument_ID_Selector_WithReceiverType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("s")
	xArg.SetType("Server")
	arg.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Handle")
	arg.Sel = selArg

	recvType := makeTestArg(meta)
	recvType.SetName("Server")
	recvType.SetPkg("mypkg")
	arg.ReceiverType = recvType

	id := arg.ID()
	assert.Contains(t, id, "mypkg")
	assert.Contains(t, id, "Server")
	assert.Contains(t, id, "Handle")
}

func TestCallArgument_ID_Selector_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Foo")
	arg.Sel = selArg

	id := arg.ID()
	assert.Equal(t, "Foo", id)
}

func TestCallArgument_ID_Call(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("MakeUser")
	funArg.SetPkg("mypkg")
	arg.Fun = funArg

	id := arg.ID()
	assert.Contains(t, id, "mypkg.MakeUser")
}

func TestCallArgument_ID_Call_NoFun(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	id := arg.ID()
	assert.Equal(t, KindCall, id)
}

func TestCallArgument_ID_FuncLit(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindFuncLit)
	arg.SetPkg("mypkg")
	arg.SetName("anonFunc")

	id := arg.ID()
	assert.Contains(t, id, "mypkg")
}

func TestCallArgument_ID_TypeConversion(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindTypeConversion)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("int")
	arg.Fun = funArg

	id := arg.ID()
	assert.Contains(t, id, "int")
}

func TestCallArgument_ID_TypeConversion_NoFun(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindTypeConversion)

	id := arg.ID()
	assert.Equal(t, KindTypeConversion, id)
}

func TestCallArgument_ID_Unary(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindUnary)
	arg.SetValue("&")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("myStruct")
	// No pkg, so id("/") returns name "myStruct"
	arg.X = xArg

	id := arg.ID()
	// id = "&" + "myStruct", then trimmed of leading "&"
	assert.Contains(t, id, "myStruct")
}

func TestCallArgument_ID_Unary_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindUnary)

	idStr, tp := arg.id(".")
	assert.Equal(t, "", idStr)
	assert.Equal(t, "", tp)
}

func TestCallArgument_ID_CompositeLit(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCompositeLit)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	xArg.SetType("Config")
	arg.X = xArg

	id := arg.ID()
	assert.Contains(t, id, "Config")
}

func TestCallArgument_ID_CompositeLit_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCompositeLit)

	idStr, _ := arg.id(".")
	assert.Equal(t, "", idStr)
}

func TestCallArgument_ID_Index(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIndex)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("myMap")
	// No pkg, so id("/") returns name "myMap"
	arg.X = xArg

	id := arg.ID()
	assert.Contains(t, id, "myMap")
}

func TestCallArgument_ID_Index_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIndex)

	idStr, _ := arg.id(".")
	assert.Equal(t, "", idStr)
}

func TestCallArgument_ID_Default_UsesRaw(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindRaw)
	arg.SetRaw("rawExpr")

	id := arg.ID()
	assert.Equal(t, "rawExpr", id)
}

func TestCallArgument_ID_Cached(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("x")

	id1 := arg.ID()
	id2 := arg.ID() // should hit cache
	assert.Equal(t, id1, id2)
}

func TestCallArgument_ID_WithTypeParams(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("Run")
	arg.SetPkg("mypkg")
	arg.TypeParamMap = map[string]string{"T": "int"}

	id := arg.ID()
	assert.Contains(t, id, "[T=int]")
}

// --- AssignmentKey.String ---

func TestAssignmentKey_String(t *testing.T) {
	k := AssignmentKey{
		Name:      "myVar",
		Pkg:       "mypkg",
		Type:      "string",
		Container: "MyFunc",
	}
	assert.Equal(t, "mypkgstringmyVarMyFunc", k.String())
}

// --- InterfaceResolutionKey.String ---

func TestInterfaceResolutionKey_String(t *testing.T) {
	k := InterfaceResolutionKey{
		InterfaceType: "Handler",
		StructType:    "MyHandler",
		Pkg:           "mypkg",
	}
	assert.Equal(t, "mypkgMyHandlerHandler", k.String())
}

// --- RegisterInterfaceResolution / GetInterfaceResolution / GetAllInterfaceResolutions ---

func TestInterfaceResolutions(t *testing.T) {
	meta := makeTestMeta()

	// Initially no resolutions
	_, ok := meta.GetInterfaceResolution("Handler", "MyHandler", "mypkg")
	assert.False(t, ok)

	all := meta.GetAllInterfaceResolutions()
	assert.Empty(t, all)

	// Register one
	meta.RegisterInterfaceResolution("Handler", "MyHandler", "mypkg", "*MyHandler", "file.go:10")

	concreteType, ok := meta.GetInterfaceResolution("Handler", "MyHandler", "mypkg")
	assert.True(t, ok)
	assert.Equal(t, "*MyHandler", concreteType)

	// Not found key
	_, ok = meta.GetInterfaceResolution("Handler", "OtherHandler", "mypkg")
	assert.False(t, ok)

	// GetAll returns a copy
	all = meta.GetAllInterfaceResolutions()
	assert.Len(t, all, 1)
}

func TestRegisterInterfaceResolution_Overwrite(t *testing.T) {
	meta := makeTestMeta()
	meta.RegisterInterfaceResolution("Handler", "MyHandler", "mypkg", "*MyHandler", "file.go:10")
	meta.RegisterInterfaceResolution("Handler", "MyHandler", "mypkg", "*NewHandler", "file.go:20")

	ct, ok := meta.GetInterfaceResolution("Handler", "MyHandler", "mypkg")
	assert.True(t, ok)
	assert.Equal(t, "*NewHandler", ct)
}

// --- determineResolvedTypeFromReturnVar ---

func TestDetermineResolvedTypeFromReturnVar_Ident(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindIdent)
	rv.SetName("myVar")
	// No package context; should return the variable name
	result := meta.determineResolvedTypeFromReturnVar(rv, "nonexistent", "somefunc")
	assert.Equal(t, "myVar", result)
}

func TestDetermineResolvedTypeFromReturnVar_Literal(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindLiteral)
	rv.SetType("string")
	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "string", result)
}

func TestDetermineResolvedTypeFromReturnVar_Default(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindBinary)
	rv.SetType("int")
	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "int", result)
}

func TestDetermineResolvedTypeFromReturnVar_Call(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("CreateUser")
	rv.Fun = funArg

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "CreateUser", result)
}

func TestDetermineResolvedTypeFromReturnVar_Call_NoFun(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "func()", result)
}

func TestDetermineResolvedTypeFromReturnVar_CompositeLit(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCompositeLit)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	rv.X = xArg

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "Config", result)
}

func TestDetermineResolvedTypeFromReturnVar_CompositeLit_NoX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCompositeLit)
	rv.SetType("MyStruct")

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "MyStruct", result)
}

func TestDetermineResolvedTypeFromReturnVar_Unary(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	// value is "&", type is "" => will add "*" prefix
	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	rv.X = xArg

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "*Config", result)
}

func TestDetermineResolvedTypeFromReturnVar_Unary_NoX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	rv.SetType("*int")

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "*int", result)
}

func TestDetermineResolvedTypeFromReturnVar_Star(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindStar)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	rv.X = xArg

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "*Config", result)
}

func TestDetermineResolvedTypeFromReturnVar_Selector(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("obj")
	rv.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Field")
	rv.Sel = selArg

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "obj.Field", result)
}

func TestDetermineResolvedTypeFromReturnVar_Selector_NilX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindSelector)
	rv.SetType("string")

	result := meta.determineResolvedTypeFromReturnVar(rv, "", "")
	assert.Equal(t, "string", result)
}

// --- resolveIdentReturnType ---

func TestResolveIdentReturnType_FromVariable(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"
	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Variables: map[string]*Variable{
					"myVar": {
						Name: meta.StringPool.Get("myVar"),
						Type: meta.StringPool.Get("string"),
					},
				},
				Functions: map[string]*Function{},
			},
		},
	}

	rv := makeTestArg(meta)
	rv.SetKind(KindIdent)
	rv.SetName("myVar")

	result := meta.resolveIdentReturnType(rv, pkgName, "")
	assert.Equal(t, "string", result)
}

func TestResolveIdentReturnType_FromAssignment(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"
	funcName := "CreateUser"

	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Variables: map[string]*Variable{},
				Functions: map[string]*Function{
					funcName: {
						Name: meta.StringPool.Get(funcName),
						AssignmentMap: map[string][]Assignment{
							"result": {
								{
									ConcreteType: meta.StringPool.Get("*User"),
								},
							},
						},
					},
				},
			},
		},
	}

	rv := makeTestArg(meta)
	rv.SetKind(KindIdent)
	rv.SetName("result")

	result := meta.resolveIdentReturnType(rv, pkgName, funcName)
	assert.Equal(t, "*User", result)
}

func TestResolveIdentReturnType_FromAssignment_ResolvesValue(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"
	funcName := "CreateUser"

	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Variables: map[string]*Variable{},
				Functions: map[string]*Function{
					funcName: {
						Name: meta.StringPool.Get(funcName),
						AssignmentMap: map[string][]Assignment{
							"result": {
								{
									ConcreteType: -1,
									Value: CallArgument{
										Kind:            meta.StringPool.Get(KindLiteral),
										Type:            meta.StringPool.Get("int"),
										Name:            -1,
										Value:           -1,
										Raw:             -1,
										Pkg:             -1,
										Position:        -1,
										ResolvedType:    -1,
										GenericTypeName: -1,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	rv := makeTestArg(meta)
	rv.SetKind(KindIdent)
	rv.SetName("result")

	result := meta.resolveIdentReturnType(rv, pkgName, funcName)
	assert.Equal(t, "int", result)
}

// --- resolveSelectorReturnType ---

func TestResolveSelectorReturnType_NilXOrSel(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindSelector)
	rv.SetType("fallback")

	result := meta.resolveSelectorReturnType(rv, "")
	assert.Equal(t, "fallback", result)
}

func TestResolveSelectorReturnType_FieldLookup(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"

	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Types: map[string]*Type{
					"User": {
						Name: meta.StringPool.Get("User"),
						Fields: []Field{
							{
								Name: meta.StringPool.Get("Name"),
								Type: meta.StringPool.Get("string"),
							},
						},
					},
				},
				Variables: map[string]*Variable{
					"u": {
						Name: meta.StringPool.Get("u"),
						Type: meta.StringPool.Get("User"),
					},
				},
				Functions: map[string]*Function{},
			},
		},
	}

	rv := makeTestArg(meta)
	rv.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("u")
	rv.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Name")
	rv.Sel = selArg

	result := meta.resolveSelectorReturnType(rv, pkgName)
	assert.Equal(t, "string", result)
}

// --- resolveCallReturnType ---

func TestResolveCallReturnType_NilFun(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	result := meta.resolveCallReturnType(rv, "")
	assert.Equal(t, "func()", result)
}

func TestResolveCallReturnType_FuncPrefix(t *testing.T) {
	meta := makeTestMeta()
	// A call whose "function" resolves to a func type string
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("func() string")
	rv.Fun = funArg

	result := meta.resolveCallReturnType(rv, "")
	assert.Equal(t, "string", result)
}

func TestResolveCallReturnType_NonFunc(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("CreateUser")
	rv.Fun = funArg

	result := meta.resolveCallReturnType(rv, "")
	assert.Equal(t, "CreateUser", result)
}

// --- resolveCompositeReturnType ---

func TestResolveCompositeReturnType_NilX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCompositeLit)
	rv.SetType("MyStruct")

	result := meta.resolveCompositeReturnType(rv, "")
	assert.Equal(t, "MyStruct", result)
}

func TestResolveCompositeReturnType_WithX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCompositeLit)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	rv.X = xArg

	result := meta.resolveCompositeReturnType(rv, "")
	assert.Equal(t, "Config", result)
}

// --- resolveUnaryReturnType ---

func TestResolveUnaryReturnType_NilX(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	rv.SetType("*int")

	result := meta.resolveUnaryReturnType(rv, "")
	assert.Equal(t, "*int", result)
}

func TestResolveUnaryReturnType_Dereference(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	rv.SetType("*int") // type starts with "*" -> dereference path

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("*Config")
	rv.X = xArg

	result := meta.resolveUnaryReturnType(rv, "")
	// baseType is "*Config", type is "*int" -> dereference -> "Config"
	assert.Equal(t, "Config", result)
}

func TestResolveUnaryReturnType_AddPointer(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	// Type does not start with "*", so pointer is added

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	rv.X = xArg

	result := meta.resolveUnaryReturnType(rv, "")
	assert.Equal(t, "*Config", result)
}

func TestResolveUnaryReturnType_DereferenceNoPrefix(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindUnary)
	rv.SetType("*int")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config") // no * prefix, so CutPrefix returns false
	rv.X = xArg

	result := meta.resolveUnaryReturnType(rv, "")
	assert.Equal(t, "Config", result)
}

// --- processFunctionCallReturnType ---

func TestProcessFunctionCallReturnType_NilFun(_ *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)
	// Fun is nil -> should return early without panic
	meta.processFunctionCallReturnType(arg)
}

func TestProcessFunctionCallReturnType_SelectorFun(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"
	methodName := "GetValue"

	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Types: map[string]*Type{
					"Service": {
						Name: meta.StringPool.Get("Service"),
						Methods: []Method{
							{
								Name: meta.StringPool.Get(methodName),
								Signature: CallArgument{
									Meta:            meta,
									Kind:            meta.StringPool.Get(KindFuncType),
									ResolvedType:    meta.StringPool.Get("string"),
									Name:            -1,
									Value:           -1,
									Raw:             -1,
									Pkg:             -1,
									Type:            -1,
									Position:        -1,
									GenericTypeName: -1,
								},
							},
						},
					},
				},
				Functions: map[string]*Function{},
			},
		},
	}

	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindSelector)
	funArg.SetPkg(pkgName)

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName(methodName)
	funArg.Sel = selArg

	arg.Fun = funArg

	meta.processFunctionCallReturnType(arg)
	assert.Equal(t, "string", arg.GetResolvedType())
}

func TestProcessFunctionCallReturnType_IdentFun(t *testing.T) {
	meta := makeTestMeta()
	pkgName := "mypkg"
	funcName := "NewService"

	meta.Packages[pkgName] = &Package{
		Files: map[string]*File{
			"file.go": {
				Functions: map[string]*Function{
					funcName: {
						Name: meta.StringPool.Get(funcName),
						Pkg:  meta.StringPool.Get(pkgName),
						Signature: CallArgument{
							Meta:            meta,
							Kind:            meta.StringPool.Get(KindFuncType),
							ResolvedType:    meta.StringPool.Get("*Service"),
							Name:            -1,
							Value:           -1,
							Raw:             -1,
							Pkg:             -1,
							Type:            -1,
							Position:        -1,
							GenericTypeName: -1,
						},
					},
				},
			},
		},
	}

	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName(funcName)
	funArg.SetPkg(pkgName)
	arg.Fun = funArg

	meta.processFunctionCallReturnType(arg)
	assert.Equal(t, "*Service", arg.GetResolvedType())
}

func TestProcessFunctionCallReturnType_EmptyFuncName(_ *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	// Name and Pkg are -1 (empty) -> funcName will be "" -> early return
	arg.Fun = funArg

	meta.processFunctionCallReturnType(arg) // should not panic
}

func TestProcessFunctionCallReturnType_WithSel(_ *testing.T) {
	// Tests the path where arg.Sel != nil, triggering recursive call
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Something")
	arg.Sel = selArg

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("Func")
	arg.Fun = funArg

	meta.processFunctionCallReturnType(arg) // exercises the Sel branch
}

func TestProcessFunctionCallReturnType_CallKindFun(_ *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindCall)
	funArg.SetName("SomeFunc")
	funArg.SetPkg("mypkg")
	arg.Fun = funArg

	meta.processFunctionCallReturnType(arg) // exercises the KindCall case for Fun
}

// --- extractReturnTypeFromSignature ---

func TestExtractReturnTypeFromSignature_FuncType(t *testing.T) {
	meta := makeTestMeta()

	resultsArg := makeTestArg(meta)
	resultsArg.SetKind(KindFuncResults)

	returnTypeArg := makeTestArg(meta)
	returnTypeArg.SetKind(KindIdent)
	returnTypeArg.SetType("*User")
	resultsArg.Args = []*CallArgument{returnTypeArg}

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindFuncType),
		Fun:             resultsArg,
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "*User", result)
}

func TestExtractReturnTypeFromSignature_FuncType_NoResults(t *testing.T) {
	meta := makeTestMeta()

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindFuncType),
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "", result)
}

func TestExtractReturnTypeFromSignature_Call(t *testing.T) {
	meta := makeTestMeta()

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("CreateUser")

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindCall),
		Fun:             funArg,
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "CreateUser", result)
}

func TestExtractReturnTypeFromSignature_Default(t *testing.T) {
	meta := makeTestMeta()

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindIdent),
		Meta:            meta,
		Name:            meta.StringPool.Get("int"),
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "int", result)
}

// --- extractTypeFromCallArgument ---

func TestExtractTypeFromCallArgument_Ident_WithType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetType("*Config")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "*Config", result)
}

func TestExtractTypeFromCallArgument_Ident_NoType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("myVar")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "myVar", result)
}

func TestExtractTypeFromCallArgument_Literal(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindLiteral)
	arg.SetType("int")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "int", result)
}

func TestExtractTypeFromCallArgument_ArrayType_WithValue(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindArrayType)
	arg.SetValue("5")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("int")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "[5]int", result)
}

func TestExtractTypeFromCallArgument_ArrayType_NoValue(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindArrayType)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("int")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "[]int", result)
}

func TestExtractTypeFromCallArgument_ArrayType_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindArrayType)
	arg.SetType("string")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "[]string", result)
}

func TestExtractTypeFromCallArgument_Slice_WithX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSlice)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("byte")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "[]byte", result)
}

func TestExtractTypeFromCallArgument_Slice_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSlice)
	arg.SetType("string")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "[]string", result)
}

func TestExtractTypeFromCallArgument_MapType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindMapType)

	keyArg := makeTestArg(meta)
	keyArg.SetKind(KindIdent)
	keyArg.SetName("string")
	arg.X = keyArg

	valArg := makeTestArg(meta)
	valArg.SetKind(KindIdent)
	valArg.SetName("int")
	arg.Fun = valArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "map[string]int", result)
}

func TestExtractTypeFromCallArgument_MapType_NoXFun(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindMapType)
	arg.SetType("map[string]int")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "map[string]int", result)
}

func TestExtractTypeFromCallArgument_MapType_NoType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindMapType)

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "map", result)
}

func TestExtractTypeFromCallArgument_Star(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindStar)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "*Config", result)
}

func TestExtractTypeFromCallArgument_Star_NoX(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindStar)
	arg.SetType("int")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "*int", result)
}

func TestExtractTypeFromCallArgument_Selector(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("http")
	arg.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Request")
	arg.Sel = selArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Contains(t, result, "Request")
}

func TestExtractTypeFromCallArgument_Call(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("CreateUser")
	arg.Fun = funArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "CreateUser", result)
}

func TestExtractTypeFromCallArgument_CompositeLit(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCompositeLit)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "Config", result)
}

func TestExtractTypeFromCallArgument_Unary(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindUnary)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("Config")
	arg.X = xArg

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "*Config", result)
}

func TestExtractTypeFromCallArgument_Default(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindBinary)
	arg.SetType("int")

	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "int", result)
}

// --- CallArgument.TypeParams ---

func TestCallArgument_TypeParams_InitializesMap(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)

	tp := arg.TypeParams()
	assert.NotNil(t, tp)
	assert.Empty(t, tp)
}

func TestCallArgument_TypeParams_PropagatesFromEdge(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.Edge = &CallGraphEdge{
		TypeParamMap: map[string]string{"T": "int", "U": "string"},
	}

	tp := arg.TypeParams()
	assert.Equal(t, "int", tp["T"])
	assert.Equal(t, "string", tp["U"])
}

// --- CallGraphEdge.NewCall ---

func TestCallGraphEdge_NewCall(t *testing.T) {
	meta := makeTestMeta()
	edge := &CallGraphEdge{meta: meta}

	nameID := meta.StringPool.Get("Run")
	pkgID := meta.StringPool.Get("mypkg")
	posID := meta.StringPool.Get("file.go:5")
	recvID := meta.StringPool.Get("Server")
	scopeID := meta.StringPool.Get("exported")

	call := edge.NewCall(nameID, pkgID, posID, recvID, scopeID)
	require.NotNil(t, call)
	assert.Equal(t, nameID, call.Name)
	assert.Equal(t, pkgID, call.Pkg)
	assert.Equal(t, posID, call.Position)
	assert.Equal(t, recvID, call.RecvType)
	assert.Equal(t, scopeID, call.Scope)
	assert.Same(t, meta, call.Meta)
	assert.Same(t, edge, call.Edge)
}

// --- Call.InstanceID caching ---

func TestCall_InstanceID_Cached(t *testing.T) {
	meta := makeTestMeta()
	c := &Call{
		Meta:     meta,
		Name:     meta.StringPool.Get("Run"),
		Pkg:      meta.StringPool.Get("mypkg"),
		Position: meta.StringPool.Get("file.go:10"),
		RecvType: -1,
	}
	id1 := c.InstanceID()
	id2 := c.InstanceID() // should hit cache
	assert.Equal(t, id1, id2)
	assert.NotEmpty(t, id1)
}

// --- Selector ID with X having Type set ---

func TestCallArgument_ID_Selector_XWithType(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetName("s")
	xArg.SetType("Server")
	xArg.SetPkg("mypkg")
	arg.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Handle")
	arg.Sel = selArg

	id := arg.ID()
	// When X has Type, sep is set to "."
	assert.Contains(t, id, "Server.Handle")
}

// --- Selector ID with xID empty ---

func TestCallArgument_ID_Selector_XIdEmpty_FallbackToSelPkg(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindSelector)

	// When X has type set, id("/") returns the type, so xID won't be empty
	// To test the fallback to Sel.GetPkg(), X must return empty id
	// That happens when kind is ident with pkg set and sep is "/"
	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetPkg("somepkg") // pkg set but no type -> id("/") returns ""
	arg.X = xArg

	selArg := makeTestArg(meta)
	selArg.SetKind(KindIdent)
	selArg.SetName("Method")
	selArg.SetPkg("fallbackpkg")
	arg.Sel = selArg

	id := arg.ID()
	assert.Contains(t, id, "fallbackpkg")
}

// --- Unary with X empty pkg fallback ---

func TestCallArgument_ID_Unary_XEmptyFallbackToPkg(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindUnary)
	arg.SetValue("&")
	arg.SetPkg("mypkg")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetType("SomeType") // type is set -> id("/"") returns SomeType
	arg.X = xArg

	id := arg.ID()
	assert.NotEmpty(t, id)
}

// --- CompositeLit with X empty fallback to pkg ---

func TestCallArgument_ID_CompositeLit_XEmptyFallbackToPkg(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindCompositeLit)
	arg.SetPkg("mypkg")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetType("SomeType") // type is set -> id("/") returns SomeType
	arg.X = xArg

	id := arg.ID()
	assert.NotEmpty(t, id)
}

// --- Index with X empty fallback to pkg ---

func TestCallArgument_ID_Index_XEmptyFallbackToPkg(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIndex)
	arg.SetPkg("mypkg")

	xArg := makeTestArg(meta)
	xArg.SetKind(KindIdent)
	xArg.SetType("SomeType") // type is set -> id("/") returns SomeType
	arg.X = xArg

	id := arg.ID()
	assert.NotEmpty(t, id)
}

// --- Ident ID with type and sep="/" ---

func TestCallArgument_id_Ident_WithType_SlashSep(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetType("MyType")
	arg.SetName("x")
	arg.SetPkg("mypkg")

	idStr, _ := arg.id("/")
	assert.Equal(t, "MyType", idStr)
}

func TestCallArgument_id_Ident_WithPkg_SlashSep(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindIdent)
	arg.SetName("x")
	arg.SetPkg("mypkg")

	idStr, _ := arg.id("/")
	// When sep is "/" and type is empty but pkg is set, returns ""
	assert.Equal(t, "", idStr)
}

// --- Call.ID with RecvType ---

func TestCall_ID_WithRecvType(t *testing.T) {
	meta := makeTestMeta()
	c := &Call{
		Meta:     meta,
		Name:     meta.StringPool.Get("Handle"),
		Pkg:      meta.StringPool.Get("mypkg"),
		RecvType: meta.StringPool.Get("Server"),
		Position: meta.StringPool.Get("file.go:5"),
	}
	id := c.ID()
	assert.Contains(t, id, "Server")
	assert.Contains(t, id, "Handle")
}

// --- FuncType with Fun not KindFuncResults ---

func TestExtractReturnTypeFromSignature_FuncType_FunNotFuncResults(t *testing.T) {
	meta := makeTestMeta()

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent) // Not KindFuncResults

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindFuncType),
		Fun:             funArg,
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "", result)
}

// --- FuncResults with empty Args ---

func TestExtractReturnTypeFromSignature_FuncType_EmptyArgs(t *testing.T) {
	meta := makeTestMeta()

	resultsArg := makeTestArg(meta)
	resultsArg.SetKind(KindFuncResults)
	resultsArg.Args = []*CallArgument{} // empty

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindFuncType),
		Fun:             resultsArg,
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "", result)
}

// --- Call without Fun ---

func TestExtractReturnTypeFromSignature_Call_NoFun(t *testing.T) {
	meta := makeTestMeta()

	sig := CallArgument{
		Kind:            meta.StringPool.Get(KindCall),
		Meta:            meta,
		Name:            -1,
		Value:           -1,
		Raw:             -1,
		Pkg:             -1,
		Type:            -1,
		Position:        -1,
		ResolvedType:    -1,
		GenericTypeName: -1,
	}

	result := meta.extractReturnTypeFromSignature(sig)
	assert.Equal(t, "", result)
}

// --- MapType with X and Fun but empty results ---

func TestExtractTypeFromCallArgument_MapType_EmptyKeyOrValue(t *testing.T) {
	meta := makeTestMeta()
	arg := makeTestArg(meta)
	arg.SetKind(KindMapType)

	keyArg := makeTestArg(meta)
	keyArg.SetKind(KindIdent)
	// Name and Type are -1 -> extractTypeFromCallArgument returns ""
	arg.X = keyArg

	valArg := makeTestArg(meta)
	valArg.SetKind(KindIdent)
	valArg.SetName("int")
	arg.Fun = valArg

	// keyType is "", so falls through to Type field or "map"
	result := meta.extractTypeFromCallArgument(arg)
	assert.Equal(t, "map", result)
}

// --- resolveCallReturnType with func() that has no return type ---

func TestResolveCallReturnType_FuncNoReturn(t *testing.T) {
	meta := makeTestMeta()
	rv := makeTestArg(meta)
	rv.SetKind(KindCall)

	funArg := makeTestArg(meta)
	funArg.SetKind(KindIdent)
	funArg.SetName("func()")
	rv.Fun = funArg

	result := meta.resolveCallReturnType(rv, "")
	// "func()" splits into ["func(", ""] -> returnType is "" -> falls through
	assert.Equal(t, "func()", result)
}
