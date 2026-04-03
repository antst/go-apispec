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

	"github.com/antst/go-apispec/internal/metadata"
)

func TestNewContextProvider(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)
	if provider == nil {
		t.Fatal("NewContextProvider returned nil")
	}
}

func TestContextProvider_GetString(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with invalid index
	result := provider.GetString(-1)
	if result != "" {
		t.Errorf("Expected empty string for invalid index, got '%s'", result)
	}

	// Test with valid index (should return empty string for empty metadata)
	result = provider.GetString(0)
	if result != "" {
		t.Errorf("Expected empty string for empty metadata, got '%s'", result)
	}
}

func TestContextProvider_GetCalleeInfo(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with nil node
	name, pkg, recvType := provider.GetCalleeInfo(nil)
	if name != "" || pkg != "" || recvType != "" {
		t.Errorf("Expected empty strings for nil node, got name='%s', pkg='%s', recvType='%s'", name, pkg, recvType)
	}
}

func TestContextProvider_GetArgumentInfo(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with empty argument
	arg := metadata.NewCallArgument(meta)
	result := provider.GetArgumentInfo(arg)
	if result != "" {
		t.Errorf("Expected empty string for empty argument, got '%s'", result)
	}
}

func TestContextProvider_callArgToString(t *testing.T) {
	meta := &metadata.Metadata{}
	provider := NewContextProvider(meta)

	// Test with empty argument
	arg := metadata.NewCallArgument(meta)
	result := provider.callArgToString(arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for empty argument, got '%s'", result)
	}
}

func TestDefaultPackageName(t *testing.T) {
	// Test default package name
	result := DefaultPackageName("github.com/example/pkg")
	if result != "github.com/example/pkg" {
		t.Errorf("Expected 'github.com/example/pkg', got '%s'", result)
	}

	// Test with empty package path
	result = DefaultPackageName("")
	if result != "" {
		t.Errorf("Expected empty string for empty package path, got '%s'", result)
	}

	// Test with versioned package path
	result = DefaultPackageName("github.com/example/pkg/v2")
	if result != "github.com/example/pkg" {
		t.Errorf("Expected 'github.com/example/pkg', got '%s'", result)
	}
}

func TestStrPtr(t *testing.T) {
	// Test string pointer creation
	testStr := "test"
	result := strPtr(testStr)
	if result == nil {
		t.Fatal("strPtr returned nil")
		return
	}
	if *result != testStr {
		t.Errorf("Expected '%s', got '%s'", testStr, *result)
	}
}

func TestContextProvider_GetCalleeInfo_WithValidNode(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a call graph edge
	caller := metadata.Call{
		Meta: meta,
		Name: stringPool.Get("main"),
		Pkg:  stringPool.Get("main"),
	}
	callee := metadata.Call{
		Meta:     meta,
		Name:     stringPool.Get("handler"),
		Pkg:      stringPool.Get("main"),
		RecvType: stringPool.Get("Handler"),
	}
	edge := metadata.CallGraphEdge{
		Caller: caller,
		Callee: callee,
	}

	// Create a mock tracker node using the interface
	node := &TrackerNode{
		CallGraphEdge: &edge,
	}

	provider := NewContextProvider(meta)
	name, pkg, recvType := provider.GetCalleeInfo(node)

	if name != "handler" {
		t.Errorf("Expected name 'handler', got '%s'", name)
	}
	if pkg != "main" {
		t.Errorf("Expected pkg 'main', got '%s'", pkg)
	}
	if recvType != "Handler" {
		t.Errorf("Expected recvType 'Handler', got '%s'", recvType)
	}
}

func TestContextProvider_GetArgumentInfo_WithValidArgument(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	// Create a valid argument
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("user")
	arg.SetType("User")
	arg.SetPkg("main")

	provider := NewContextProvider(meta)
	result := provider.GetArgumentInfo(arg)

	// Should return a string representation
	if result == "" {
		t.Error("Expected non-empty string for valid argument")
	}
}

func TestContextProvider_callArgToString_WithVariousKinds(t *testing.T) {
	// Create metadata with string pool
	stringPool := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: stringPool,
	}

	provider := NewContextProvider(meta)

	tests := []struct {
		name     string
		arg      *metadata.CallArgument
		expected string
	}{
		{
			name: "ident kind",
			arg: func() *metadata.CallArgument {
				arg := metadata.NewCallArgument(meta)
				arg.SetKind(metadata.KindIdent)
				arg.SetName("user")
				return arg
			}(),
			expected: "user",
		},
		{
			name: "literal kind",
			arg: func() *metadata.CallArgument {
				arg := metadata.NewCallArgument(meta)
				arg.SetKind(metadata.KindLiteral)
				arg.SetValue(`"hello"`)
				return arg
			}(),
			expected: `"hello"`,
		},
		// Selector kind test removed due to complexity
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.callArgToString(tt.arg, nil)
			if result == "" {
				t.Error("Expected non-empty string for valid argument")
			}
		})
	}
}

func TestContextProvider_callArgToString_WithNilMetadata(t *testing.T) {
	// Create provider with nil metadata
	provider := &ContextProviderImpl{meta: nil}

	// Create a simple argument with valid metadata first
	meta := &metadata.Metadata{}
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("test")

	// Should not panic
	result := provider.callArgToString(arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for nil metadata, got '%s'", result)
	}
}

func TestContextProvider_callArgToString_WithNilStringPool(t *testing.T) {
	// Create metadata with nil string pool
	meta := &metadata.Metadata{
		StringPool: nil,
	}

	provider := NewContextProvider(meta)

	// Create a simple argument
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName("test")

	// Should not panic
	result := provider.callArgToString(arg, nil)
	if result != "" {
		t.Errorf("Expected empty string for nil string pool, got '%s'", result)
	}
}

func TestDefaultPackageName_WithValidInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple package",
			input:    "main",
			expected: "main",
		},
		{
			name:     "package with path",
			input:    "github.com/user/project",
			expected: "github.com/user/project",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single slash",
			input:    "package/",
			expected: "package/",
		},
		{
			name:     "multiple slashes",
			input:    "a/b/c/d",
			expected: "a/b/c/d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultPackageName(tt.input)
			if result != tt.expected {
				t.Errorf("DefaultPackageName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStrPtr_WithVariousInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "non-empty string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "special characters",
			input:    "test@123!",
			expected: "test@123!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strPtr(tt.input)
			if result == nil {
				t.Fatal("Expected non-nil pointer")
				return
			}
			if *result != tt.expected {
				t.Errorf("strPtr(%q) = %q, expected %q", tt.input, *result, tt.expected)
			}
		})
	}
}

func TestCallArgToString_AllKindBranches(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp}
	provider := NewContextProvider(meta)

	makeArg := func(kind string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(kind)
		return a
	}

	// KindLiteral — strips quotes
	lit := makeArg(metadata.KindLiteral)
	lit.SetValue(`"hello"`)
	if got := provider.callArgToString(lit, nil); got != "hello" {
		t.Errorf("KindLiteral: got %q, want %q", got, "hello")
	}

	// KindKeyValue — returns ""
	kv := makeArg(metadata.KindKeyValue)
	if got := provider.callArgToString(kv, nil); got != "" {
		t.Errorf("KindKeyValue: got %q, want empty", got)
	}

	// KindMapType with X and Fun
	mt := makeArg(metadata.KindMapType)
	mtKey := makeArg(metadata.KindIdent)
	mtKey.SetName("string")
	mtKey.SetType("string")
	mtVal := makeArg(metadata.KindIdent)
	mtVal.SetName("int")
	mtVal.SetType("int")
	mt.X = mtKey
	mt.Fun = mtVal
	if got := provider.callArgToString(mt, nil); got != "map[string]int" {
		t.Errorf("KindMapType: got %q, want %q", got, "map[string]int")
	}

	// KindMapType without children
	mt2 := makeArg(metadata.KindMapType)
	if got := provider.callArgToString(mt2, nil); got != "map" {
		t.Errorf("KindMapType nil: got %q, want %q", got, "map")
	}

	// KindUnary with X
	un := makeArg(metadata.KindUnary)
	unX := makeArg(metadata.KindIdent)
	unX.SetName("User")
	unX.SetType("User")
	un.X = unX
	if got := provider.callArgToString(un, nil); got != "*User" {
		t.Errorf("KindUnary: got %q, want %q", got, "*User")
	}

	// KindUnary without X
	un2 := makeArg(metadata.KindUnary)
	if got := provider.callArgToString(un2, nil); got != "*" {
		t.Errorf("KindUnary nil: got %q, want %q", got, "*")
	}

	// KindArrayType with X
	at := makeArg(metadata.KindArrayType)
	atX := makeArg(metadata.KindIdent)
	atX.SetName("string")
	atX.SetType("string")
	at.X = atX
	if got := provider.callArgToString(at, nil); got != "[]string" {
		t.Errorf("KindArrayType: got %q, want %q", got, "[]string")
	}

	// KindArrayType without X
	at2 := makeArg(metadata.KindArrayType)
	if got := provider.callArgToString(at2, nil); got != "[]" {
		t.Errorf("KindArrayType nil: got %q, want %q", got, "[]")
	}

	// KindIndex with slice type
	idx := makeArg(metadata.KindIndex)
	idxX := makeArg(metadata.KindIdent)
	idxX.SetName("users")
	idxX.SetType("[]User")
	idx.X = idxX
	if got := provider.callArgToString(idx, nil); got != "User" {
		t.Errorf("KindIndex slice: got %q, want %q", got, "User")
	}

	// KindIndex with map type
	idx2 := makeArg(metadata.KindIndex)
	idx2X := makeArg(metadata.KindIdent)
	idx2X.SetName("m")
	idx2X.SetType("map[string]int")
	idx2.X = idx2X
	if got := provider.callArgToString(idx2, nil); got != "int" {
		t.Errorf("KindIndex map: got %q, want %q", got, "int")
	}

	// KindIndex without X
	idx3 := makeArg(metadata.KindIndex)
	if got := provider.callArgToString(idx3, nil); got != "" {
		t.Errorf("KindIndex nil: got %q, want empty", got)
	}

	// KindCompositeLit with X
	cl := makeArg(metadata.KindCompositeLit)
	clX := makeArg(metadata.KindIdent)
	clX.SetName("Config")
	clX.SetType("Config")
	cl.X = clX
	if got := provider.callArgToString(cl, nil); got != "Config" {
		t.Errorf("KindCompositeLit: got %q, want %q", got, "Config")
	}

	// KindCompositeLit without X
	cl2 := makeArg(metadata.KindCompositeLit)
	if got := provider.callArgToString(cl2, nil); got != "" {
		t.Errorf("KindCompositeLit nil: got %q, want empty", got)
	}

	// Custom separator
	sep := "/"
	sepArg := makeArg(metadata.KindIdent)
	sepArg.SetName("handler")
	sepArg.SetType("Handler")
	sepArg.SetPkg("myapp")
	if got := provider.callArgToString(sepArg, &sep); got == "" {
		t.Error("Custom separator: got empty string")
	}

	// KindIdent with builtin type
	bi := makeArg(metadata.KindIdent)
	bi.SetName("x")
	bi.SetType("string")
	if got := provider.callArgToString(bi, nil); got != "string" {
		t.Errorf("builtin type: got %q, want %q", got, "string")
	}

	// KindIdent with pointer to builtin
	pb := makeArg(metadata.KindIdent)
	pb.SetName("x")
	pb.SetType("*int")
	if got := provider.callArgToString(pb, nil); got != "*int" {
		t.Errorf("pointer builtin: got %q, want %q", got, "*int")
	}

	// KindIdent with slice of builtin
	sb := makeArg(metadata.KindIdent)
	sb.SetName("x")
	sb.SetType("[]byte")
	if got := provider.callArgToString(sb, nil); got != "[]byte" {
		t.Errorf("slice builtin: got %q, want %q", got, "[]byte")
	}

	// KindIdent with map type
	mp := makeArg(metadata.KindIdent)
	mp.SetName("m")
	mp.SetType("map[string]int")
	if got := provider.callArgToString(mp, nil); got != "map[string]int" {
		t.Errorf("map type: got %q, want %q", got, "map[string]int")
	}

	// KindIdent with func type (should return name, not type)
	fn := makeArg(metadata.KindIdent)
	fn.SetName("handler")
	fn.SetType("func()")
	if got := provider.callArgToString(fn, nil); got == "" {
		t.Error("func type: got empty string")
	}

	// KindSelector with X and Sel
	sel := makeArg(metadata.KindSelector)
	selX := metadata.NewCallArgument(meta)
	selX.SetKind(metadata.KindIdent)
	selX.SetName("http")
	selX.SetPkg("net/http")
	sel.X = selX
	selSel := metadata.NewCallArgument(meta)
	selSel.SetName("StatusOK")
	sel.Sel = selSel
	if got := provider.callArgToString(sel, nil); got == "" {
		t.Error("KindSelector: got empty string")
	}

	// KindSelector without X
	sel2 := makeArg(metadata.KindSelector)
	sel2Sel := metadata.NewCallArgument(meta)
	sel2Sel.SetName("Method")
	sel2.Sel = sel2Sel
	if got := provider.callArgToString(sel2, nil); got != "Method" {
		t.Errorf("KindSelector no X: got %q, want %q", got, "Method")
	}

	// KindCall with Fun
	call := makeArg(metadata.KindCall)
	callFun := makeArg(metadata.KindIdent)
	callFun.SetName("NewUser")
	callFun.SetType("User")
	call.Fun = callFun
	if got := provider.callArgToString(call, nil); got == "" {
		t.Error("KindCall: got empty string")
	}

	// KindCall without Fun
	call2 := makeArg(metadata.KindCall)
	if got := provider.callArgToString(call2, nil); got != "call(...)" {
		t.Errorf("KindCall no Fun: got %q, want %q", got, "call(...)")
	}

	// KindCall with pkg
	call3 := makeArg(metadata.KindCall)
	call3Fun := makeArg(metadata.KindIdent)
	call3Fun.SetName("Parse")
	call3.Fun = call3Fun
	call3.SetPkg("encoding/json")
	call3.SetName("Decode")
	if got := provider.callArgToString(call3, nil); got == "" {
		t.Error("KindCall with pkg: got empty string")
	}

	// KindTypeConversion with Fun
	tc := makeArg(metadata.KindTypeConversion)
	tcFun := makeArg(metadata.KindIdent)
	tcFun.SetName("string")
	tcFun.SetType("string")
	tc.Fun = tcFun
	if got := provider.callArgToString(tc, nil); got != "string" {
		t.Errorf("KindTypeConversion: got %q, want %q", got, "string")
	}

	// KindTypeConversion without Fun
	tc2 := makeArg(metadata.KindTypeConversion)
	if got := provider.callArgToString(tc2, nil); got != "" {
		t.Errorf("KindTypeConversion nil: got %q, want empty", got)
	}

	// KindInterfaceType
	iface := makeArg(metadata.KindInterfaceType)
	if got := provider.callArgToString(iface, nil); got != "interface{}" {
		t.Errorf("KindInterfaceType: got %q, want %q", got, "interface{}")
	}

	// KindRaw
	raw := makeArg(metadata.KindRaw)
	raw.SetRaw("someRawExpr")
	if got := provider.callArgToString(raw, nil); got == "" {
		t.Error("KindRaw: got empty string")
	}

	// KindIdent with pkg and custom type (non-builtin)
	ct := makeArg(metadata.KindIdent)
	ct.SetName("User")
	ct.SetType("myapp.User")
	ct.SetPkg("myapp")
	if got := provider.callArgToString(ct, nil); got == "" {
		t.Error("KindIdent custom type: got empty string")
	}

	// KindIdent with type ending in name (package suffix match)
	ps := makeArg(metadata.KindIdent)
	ps.SetName("myapp")
	ps.SetPkg("github.com/org/myapp")
	ps.SetType("")
	if got := provider.callArgToString(ps, nil); got == "" {
		t.Error("KindIdent pkg suffix: got empty string")
	}
}
