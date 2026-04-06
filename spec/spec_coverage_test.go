package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMuxConfig(t *testing.T) {
	cfg := DefaultMuxConfig()
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Framework.RoutePatterns, "mux config should have route patterns")
}

func TestDefaultMuxConfig_HasDefaults(t *testing.T) {
	cfg := DefaultMuxConfig()
	require.NotNil(t, cfg)
	// Verify defaults section is populated
	assert.NotEmpty(t, cfg.Defaults.ResponseContentType, "mux config should have default response content type")
	assert.NotZero(t, cfg.Defaults.ResponseStatus, "mux config should have default response status")
}
