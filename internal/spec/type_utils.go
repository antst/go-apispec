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
	"github.com/antst/go-apispec/internal/metadata"
)

// derefPointerRef unwraps one pointer layer from ref to stay in lockstep with a
// body/param STRING from which the caller has just stripped a leading "*"
// (pattern.Deref). ParseTypeRef("*X") is RefPointer{Elem}, so the unwrapped Elem
// renders to the stripped string — keeping ref == ParseTypeRef(string) without a
// re-parse. Returns ref unchanged when it is not a pointer (the string had no "*"
// to strip, or resolution produced no ref), so callers can apply it
// unconditionally after the string strip.
func derefPointerRef(ref *metadata.TypeRef) *metadata.TypeRef {
	if ref != nil && ref.Kind == metadata.RefPointer && ref.Elem != nil {
		return ref.Elem
	}
	return ref
}

// sharedResolveTypeOrigin consolidates the common type origin resolution logic
// used by RequestPatternMatcherImpl, ResponsePatternMatcherImpl, and ParamPatternMatcherImpl.
//
// It checks (in order):
//  1. Whether the argument has a resolved type (arg.GetResolvedType)
//  2. Whether the argument is a generic type with a concrete mapping in the node's type param map
//  3. Whether the argument's variable has assignments with concrete types (for KindIdent,
//     and optionally KindFuncLit when checkFuncLit is true)
//  4. Falls back to originalType
//
// It returns the resolved type both as the (naming) string and as a structured
// *TypeRef (Phase 3). For the resolved-type branch the materialized
// arg.ResolvedTypeRef is reused (it is kept in lockstep with the string by
// SetResolvedType); the other branches parse their own resolved string at this
// boundary (research D1). The ref always equals ParseTypeRef of the returned
// string, so threading it into schemaForType is byte-identical to re-parsing.
//
// SC-001 (spec 008): these ParseTypeRef calls are the resolution boundary where
// the parse LIVES — moved off the schema path, not added. Each call here lets the
// downstream schemaForType consume the tree directly instead of re-parsing the
// resolved string (mapper.go schemaFromParsedString documents the retained-parse
// counterpart).
func sharedResolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string, contextProvider ContextProvider, checkFuncLit bool) (string, *metadata.TypeRef) {
	// If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType, arg.ResolvedTypeRef
	}

	// If it's a generic type with a concrete resolution, use it
	if arg.IsGenericType && arg.GenericTypeName != -1 {
		if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
			return concreteType, metadata.ParseTypeRef(concreteType)
		}
	}

	// Check if this variable has assignments that might give us more type information
	kind := arg.GetKind()
	if kind == metadata.KindIdent || (checkFuncLit && kind == metadata.KindFuncLit) {
		edge := node.GetEdge()
		if assignments, exists := edge.AssignmentMap[arg.GetName()]; exists {
			for _, assignment := range assignments {
				if assignment.ConcreteType != 0 {
					concreteType := contextProvider.GetString(assignment.ConcreteType)
					if concreteType != "" {
						return concreteType, metadata.ParseTypeRef(concreteType)
					}
				}
			}
		}
	}

	return originalType, metadata.ParseTypeRef(originalType)
}
