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

package metadata

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/cfg"
)

// BuildFunctionCFGs builds control-flow graphs for the given function
// declarations and annotates existing CallGraphEdge and Assignment entries
// in the metadata with BranchContext information.
//
// This is additive — edges/assignments without branch context (unconditional
// code) keep Branch == nil. Only statements inside if/else/switch branches
// get annotated.
func BuildFunctionCFGs(funcDecls []*ast.FuncDecl, fset *token.FileSet, meta *Metadata) {
	if meta == nil || len(funcDecls) == 0 {
		return
	}

	// Build position→edge and position→assignment indexes for fast lookup
	edgesByPos := buildEdgePositionIndex(meta, fset)
	assignsByPos := buildAssignmentPositionIndex(meta, fset)

	for _, decl := range funcDecls {
		if decl.Body == nil {
			continue
		}
		// Build CFG for this function. Conservative mayReturn: always true.
		graph := cfg.New(decl.Body, func(*ast.CallExpr) bool { return true })
		annotateBranches(graph, fset, meta, edgesByPos, assignsByPos)
	}
}

// buildEdgePositionIndex creates a map from source position string to
// CallGraphEdge pointers for fast lookup during CFG annotation.
func buildEdgePositionIndex(meta *Metadata, _ *token.FileSet) map[string]*CallGraphEdge {
	index := make(map[string]*CallGraphEdge, len(meta.CallGraph))
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		pos := meta.StringPool.GetString(edge.Position)
		if pos != "" {
			index[pos] = edge
		}
	}
	return index
}

// buildAssignmentPositionIndex creates a map from source position string to
// Assignment pointers across all call graph edges.
func buildAssignmentPositionIndex(meta *Metadata, _ *token.FileSet) map[string]*Assignment {
	index := make(map[string]*Assignment)
	for i := range meta.CallGraph {
		for varName := range meta.CallGraph[i].AssignmentMap {
			for j := range meta.CallGraph[i].AssignmentMap[varName] {
				assign := &meta.CallGraph[i].AssignmentMap[varName][j]
				pos := meta.StringPool.GetString(assign.Position)
				if pos != "" {
					index[pos] = assign
				}
			}
		}
	}
	return index
}

// annotateBranches walks the CFG blocks and tags edges/assignments with
// BranchContext based on the block's Kind and parent statement.
func annotateBranches(graph *cfg.CFG, fset *token.FileSet, meta *Metadata,
	edgesByPos map[string]*CallGraphEdge, assignsByPos map[string]*Assignment) {
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}

		// Determine branch kind from block's Kind
		branchKind := mapBlockKind(block.Kind)
		if branchKind == "" {
			continue // Unconditional block — no annotation needed
		}

		// Get parent statement position for context
		var parentStmtPos int
		if block.Stmt != nil {
			parentStmtPos = meta.StringPool.Get(fset.Position(block.Stmt.Pos()).String())
		}

		ctx := &BranchContext{
			BlockIndex:    block.Index,
			BlockKind:     branchKind,
			ParentStmtPos: parentStmtPos,
		}

		// For switch-case blocks, extract case clause literal values
		if branchKind == "switch-case" && block.Stmt != nil {
			ctx.CaseValues = extractCaseValues(block.Stmt)
		}

		// Walk all AST nodes in this block and tag matching edges/assignments
		for _, node := range block.Nodes {
			pos := fset.Position(node.Pos()).String()

			if edge, ok := edgesByPos[pos]; ok {
				edge.Branch = ctx
			}
			if assign, ok := assignsByPos[pos]; ok {
				assign.Branch = ctx
			}
		}
	}
}

// extractCaseValues extracts string literal values from a case clause statement.
// For example, `case "GET", "HEAD":` returns ["GET", "HEAD"].
func extractCaseValues(stmt ast.Stmt) []string {
	cc, ok := stmt.(*ast.CaseClause)
	if !ok || cc == nil {
		return nil
	}
	var values []string
	for _, expr := range cc.List {
		if lit, ok := expr.(*ast.BasicLit); ok {
			// Strip quotes from string literals
			val := lit.Value
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			values = append(values, val)
		}
	}
	return values
}

// mapBlockKind converts a cfg.BlockKind to a human-readable branch kind string.
// Returns "" for unconditional blocks (no annotation needed).
func mapBlockKind(kind cfg.BlockKind) string {
	switch kind {
	case cfg.KindIfThen:
		return "if-then"
	case cfg.KindIfElse:
		return "if-else"
	case cfg.KindIfDone:
		return "" // Post-if merge point — unconditional
	case cfg.KindSwitchCaseBody:
		return "switch-case"
	case cfg.KindSwitchNextCase:
		return "" // Case condition testing — not a body
	case cfg.KindSwitchDone:
		return "" // Post-switch merge point
	case cfg.KindForBody:
		return "" // Loop body — not needed for method/status analysis
	case cfg.KindForDone:
		return ""
	case cfg.KindSelectCaseBody:
		return "select-case"
	case cfg.KindSelectDone:
		return ""
	default:
		return "" // Entry, return, and other blocks are unconditional
	}
}
