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

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

// TestDerefPointerRef locks the Phase-4 invariant that unwrapping one pointer
// layer from a ref stays in lockstep with stripping a leading "*" from the
// parallel type string: ParseTypeRef("*X").Elem must render to ParseTypeRef("X").
func TestDerefPointerRef(t *testing.T) {
	t.Run("pointer unwraps to its element", func(t *testing.T) {
		ref := metadata.ParseTypeRef("*pkg.User")
		if assert.NotNil(t, ref) {
			assert.Equal(t, metadata.RefPointer, ref.Kind)
		}
		got := derefPointerRef(ref)
		// Equivalent to stripping the leading "*" from the string.
		assert.Equal(t, metadata.ParseTypeRef("pkg.User").String(), got.String())
	})

	t.Run("double pointer unwraps one layer", func(t *testing.T) {
		got := derefPointerRef(metadata.ParseTypeRef("**pkg.User"))
		assert.Equal(t, metadata.ParseTypeRef("*pkg.User").String(), got.String())
	})

	t.Run("non-pointer is returned unchanged", func(t *testing.T) {
		ref := metadata.ParseTypeRef("pkg.User")
		assert.Same(t, ref, derefPointerRef(ref))
	})

	t.Run("nil is returned unchanged", func(t *testing.T) {
		assert.Nil(t, derefPointerRef(nil))
	})

	t.Run("pointer with nil element is returned unchanged", func(t *testing.T) {
		ref := &metadata.TypeRef{Kind: metadata.RefPointer} // Elem == nil
		assert.Same(t, ref, derefPointerRef(ref))
	})
}
