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
//
// Edge positions are stored via getPosition() which strips the repo-root
// prefix when one is set (see SetRepoRoot). CFG-side lookups use
// fset.Position(...).String() directly and would miss every edge unless we
// store both forms in the index — silently disabling all branch annotation
// (issue #27 was an instance of this mismatch).
func buildEdgePositionIndex(meta *Metadata, _ *token.FileSet) map[string]*CallGraphEdge {
	index := make(map[string]*CallGraphEdge, len(meta.CallGraph))
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		pos := meta.StringPool.GetString(edge.Position)
		if pos != "" {
			index[pos] = edge
			if repoRoot != "" {
				index[repoRoot+pos] = edge
			}
		}
	}
	return index
}

// buildAssignmentPositionIndex creates a map from source position string to
// the Assignment pointers at that position. It covers the call-graph edge maps
// AND the function/method AssignmentMaps — the latter are distinct copies that
// downstream reachability analysis (expandStatusesFromIdent, issue #50) reads,
// so they must be annotated too. Several assignments can share a position
// (an edge copy and a function copy), hence the slice value. Mirrors
// buildEdgePositionIndex's repoRoot-stripped + raw double-keying.
func buildAssignmentPositionIndex(meta *Metadata, _ *token.FileSet) map[string][]*Assignment {
	index := make(map[string][]*Assignment)
	add := func(assigns []Assignment) {
		for j := range assigns {
			assign := &assigns[j]
			pos := meta.StringPool.GetString(assign.Position)
			if pos == "" {
				continue
			}
			index[pos] = append(index[pos], assign)
			if repoRoot != "" {
				index[repoRoot+pos] = append(index[repoRoot+pos], assign)
			}
		}
	}
	for i := range meta.CallGraph {
		for varName := range meta.CallGraph[i].AssignmentMap {
			add(meta.CallGraph[i].AssignmentMap[varName])
		}
	}
	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			for _, fn := range file.Functions {
				for varName := range fn.AssignmentMap {
					add(fn.AssignmentMap[varName])
				}
			}
			for _, typ := range file.Types {
				for m := range typ.Methods {
					for varName := range typ.Methods[m].AssignmentMap {
						add(typ.Methods[m].AssignmentMap[varName])
					}
				}
			}
		}
	}
	return index
}

// annotateBranches walks the CFG blocks and tags edges/assignments with
// BranchContext based on the block's Kind and parent statement.
func annotateBranches(graph *cfg.CFG, fset *token.FileSet, meta *Metadata,
	edgesByPos map[string]*CallGraphEdge, assignsByPos map[string][]*Assignment) {
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

		// Walk all AST nodes in this block and tag matching edges/assignments.
		// block.Nodes are statement-level; the position of an AssignStmt like
		// `_, _ = w.Write(...)` is the underscore, while the call edge is
		// indexed at the inner CallExpr's position. Descend into every node
		// so each inner CallExpr/AssignStmt gets a chance to match.
		for _, node := range block.Nodes {
			ast.Inspect(node, func(n ast.Node) bool {
				if n == nil {
					return false
				}
				pos := fset.Position(n.Pos()).String()
				if edge, ok := edgesByPos[pos]; ok {
					edge.Branch = ctx
				}
				for _, assign := range assignsByPos[pos] {
					assign.Branch = ctx
				}
				return true
			})
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
