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

// sharedResolveTypeOrigin consolidates the common type origin resolution logic
// used by RequestPatternMatcherImpl, ResponsePatternMatcherImpl, and ParamPatternMatcherImpl.
//
// It checks (in order):
//  1. Whether the argument has a resolved type (arg.GetResolvedType)
//  2. Whether the argument is a generic type with a concrete mapping in the node's type param map
//  3. Whether the argument's variable has assignments with concrete types (for KindIdent,
//     and optionally KindFuncLit when checkFuncLit is true)
//  4. Falls back to originalType
func sharedResolveTypeOrigin(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string, contextProvider ContextProvider, checkFuncLit bool) string {
	// If the argument has resolved type information, use it
	if resolvedType := arg.GetResolvedType(); resolvedType != "" {
		return resolvedType
	}

	// If it's a generic type with a concrete resolution, use it
	if arg.IsGenericType && arg.GenericTypeName != -1 {
		if concreteType, exists := node.GetTypeParamMap()[arg.GetGenericTypeName()]; exists {
			return concreteType
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
						return concreteType
					}
				}
			}
		}
	}

	return originalType
}
