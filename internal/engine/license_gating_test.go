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

package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/antst/go-apispec/spec"
)

// TestApplyConfigDefaults_LicenseGating covers issue #47 and its edge case
// (raised by Copilot review): a license block is emitted only when a license
// NAME is configured, since OpenAPI requires license.name. A URL without a name
// must NOT produce an invalid `license: {name: ""}`.
func TestApplyConfigDefaults_LicenseGating(t *testing.T) {
	t.Run("no license configured -> no block", func(t *testing.T) {
		e := NewEngine(&EngineConfig{})
		cfg := &spec.APISpecConfig{}
		e.applyConfigDefaults(cfg)
		assert.Nil(t, cfg.Info.License, "empty license must not be emitted")
	})

	t.Run("URL only, no name -> no block (name is required in OpenAPI)", func(t *testing.T) {
		e := NewEngine(&EngineConfig{LicenseURL: "https://example.com/license"})
		cfg := &spec.APISpecConfig{}
		e.applyConfigDefaults(cfg)
		assert.Nil(t, cfg.Info.License, "URL without a name can't form a valid license block")
	})

	t.Run("name set -> block emitted with name and url", func(t *testing.T) {
		e := NewEngine(&EngineConfig{LicenseName: "MIT", LicenseURL: "https://opensource.org/licenses/MIT"})
		cfg := &spec.APISpecConfig{}
		e.applyConfigDefaults(cfg)
		require.NotNil(t, cfg.Info.License)
		assert.Equal(t, "MIT", cfg.Info.License.Name)
		assert.Equal(t, "https://opensource.org/licenses/MIT", cfg.Info.License.URL)
	})
}
