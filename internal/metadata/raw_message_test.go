// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package metadata_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/internal/metadata"
)

func TestIsJSONRawMessageRef(t *testing.T) {
	raw := &metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "encoding/json", Name: "RawMessage"}
	assert.True(t, metadata.IsJSONRawMessageRef(raw))

	// Wrong package / name / kind / nil are all false.
	assert.False(t, metadata.IsJSONRawMessageRef(nil))
	assert.False(t, metadata.IsJSONRawMessageRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "x", Name: "RawMessage"}))
	assert.False(t, metadata.IsJSONRawMessageRef(&metadata.TypeRef{Kind: metadata.RefNamed, Pkg: "encoding/json", Name: "Other"}))
	assert.False(t, metadata.IsJSONRawMessageRef(&metadata.TypeRef{Kind: metadata.RefBasic, Name: "byte"}))
	// IsJSONRawMessageRef is a single-node check: a slice OF RawMessage is not
	// itself RawMessage (the caller unwraps before checking when it needs to).
	assert.False(t, metadata.IsJSONRawMessageRef(&metadata.TypeRef{Kind: metadata.RefSlice, Elem: raw}))
}

// processStructFields must NOT collapse json.RawMessage to its []byte underlying:
// the field keeps its named encoding/json.RawMessage ref (direct, pointer, and
// slice positions) while a plain []byte field still collapses to a byte slice.
func TestRawMessageFieldNotCollapsed(t *testing.T) {
	meta := metaFromModule(t, map[string]interface{}{
		"app.go": `package app

import "encoding/json"

type Doc struct {
	Raw     json.RawMessage
	RawPtr  *json.RawMessage
	Patches []json.RawMessage
	Blob    []byte
}
`,
	})

	typ := findType(meta, "app", "Doc")
	require.NotNil(t, typ)

	got := map[string]string{}
	for _, f := range typ.Fields {
		got[meta.StringPool.GetString(f.Name)] = f.TypeRef.String()
	}

	assert.Equal(t, "encoding/json.RawMessage", got["Raw"])
	assert.Equal(t, "*encoding/json.RawMessage", got["RawPtr"])
	assert.Equal(t, "[]encoding/json.RawMessage", got["Patches"])
	// A genuine []byte field is unaffected and still collapses to the byte slice.
	assert.Equal(t, "[]byte", got["Blob"])
}
