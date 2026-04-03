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

// Package spec exposes a stable public API for configuration and OpenAPI types,
// re-exported from the internal spec package.
package spec

import intspec "github.com/antst/go-apispec/internal/spec"

// APISpecConfig is a re-export of the internal APISpecConfig type.
type APISpecConfig = intspec.APISpecConfig
type Info = intspec.Info
type Server = intspec.Server
type SecurityRequirement = intspec.SecurityRequirement
type SecurityScheme = intspec.SecurityScheme
type Tag = intspec.Tag
type ExternalDocumentation = intspec.ExternalDocumentation
type Schema = intspec.Schema
type Components = intspec.Components
type OpenAPISpec = intspec.OpenAPISpec

// DefaultGinConfig returns the default configuration for the Gin framework.
func DefaultGinConfig() *APISpecConfig   { return intspec.DefaultGinConfig() }
func DefaultChiConfig() *APISpecConfig   { return intspec.DefaultChiConfig() }
func DefaultEchoConfig() *APISpecConfig  { return intspec.DefaultEchoConfig() }
func DefaultFiberConfig() *APISpecConfig { return intspec.DefaultFiberConfig() }
func DefaultMuxConfig() *APISpecConfig   { return intspec.DefaultMuxConfig() }
func DefaultHTTPConfig() *APISpecConfig  { return intspec.DefaultHTTPConfig() }

// LoadAPISpecConfig loads a YAML configuration file.
func LoadAPISpecConfig(path string) (*APISpecConfig, error) { return intspec.LoadAPISpecConfig(path) }
