package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/spec"
)

func TestGenerateFromDirectory_EmptyString(t *testing.T) {
	gen := NewGenerator(nil)
	_, err := gen.GenerateFromDirectory("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory path is required")
}

func TestGenerateFromDirectory_Success(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "go.mod"),
		[]byte("module testapp\n\ngo 1.21\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(tempDir, "main.go"),
		[]byte(`package main

import "net/http"

func main() {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	http.ListenAndServe(":8080", nil)
}
`),
		0644,
	))

	gen := NewGenerator(nil)
	result, err := gen.GenerateFromDirectory(tempDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OpenAPI)
}

func TestGenerateFromDirectory_NonExistentDir(t *testing.T) {
	gen := NewGenerator(spec.DefaultHTTPConfig())
	_, err := gen.GenerateFromDirectory("/totally/bogus/dir/that/does/not/exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input directory")
}

func TestGenerateFromDirectory_DirWithoutGoMod(t *testing.T) {
	tempDir := t.TempDir()

	gen := NewGenerator(spec.DefaultHTTPConfig())
	_, err := gen.GenerateFromDirectory(tempDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod")
}

func TestNewGenerator_NilConfig(t *testing.T) {
	gen := NewGenerator(nil)
	require.NotNil(t, gen)
	assert.Nil(t, gen.config)
	assert.NotNil(t, gen.engine)
}

func TestNewGenerator_WithConfig(t *testing.T) {
	cfg := spec.DefaultChiConfig()
	gen := NewGenerator(cfg)
	require.NotNil(t, gen)
	assert.Equal(t, cfg, gen.config)
	assert.NotNil(t, gen.engine)
}
