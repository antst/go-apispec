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
	"go/types"
	"strings"

	"golang.org/x/tools/go/cfg"
)

// BuildFunctionCFGs builds control-flow graphs for the given function
// declarations, annotates existing CallGraphEdge and Assignment entries with
// BranchContext, and retains a compact per-function reachability model in
// meta.FunctionCFGs (spec 009, FR-001). declInfo provides each function's
// *types.Info for type-switch case-type capture (nil-tolerant).
//
// The raw *cfg.CFG is local and dropped after build; only the compact model and
// the BranchContext annotations survive.
func BuildFunctionCFGs(funcDecls []*ast.FuncDecl, declInfo map[*ast.FuncDecl]*types.Info, fset *token.FileSet, meta *Metadata) {
	if meta == nil || len(funcDecls) == 0 {
		return
	}

	edgesByPos := buildEdgePositionIndex(meta, fset)
	assignsByPos := buildAssignmentPositionIndex(meta, fset)
	if meta.FunctionCFGs == nil {
		meta.FunctionCFGs = make(map[string]*FunctionCFG, len(funcDecls))
	}
	if meta.cfgPosToFn == nil {
		meta.cfgPosToFn = make(map[string]string)
	}

	for _, decl := range funcDecls {
		if decl.Body == nil {
			continue
		}
		// Build CFG for this function. Conservative mayReturn: always true.
		graph := cfg.New(decl.Body, func(*ast.CallExpr) bool { return true })
		fnKey := fset.Position(decl.Body.Pos()).String() // unique per function body
		tsOperands := typeSwitchOperands(decl.Body)
		tsDefaults := typeSwitchDefaultArms(decl.Body)
		annotateBranches(graph, fset, declInfo[decl], meta, edgesByPos, assignsByPos, fnKey, tsOperands, tsDefaults)
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

// annotateBranches walks the CFG blocks: it (1) builds the compact FunctionCFG
// for fnKey — successor adjacency + a position→block map over ALL live blocks
// (research R3: the response call usually sits in the post-if merge block, which
// the BranchContext gate below skips) + dominators — and (2) tags edges/assignments
// with BranchContext (including type-switch case types).
func annotateBranches(graph *cfg.CFG, fset *token.FileSet, info *types.Info, meta *Metadata, //nolint:gocyclo // single CFG walk doing reachability capture + branch annotation
	edgesByPos map[string]*CallGraphEdge, assignsByPos map[string][]*Assignment, fnKey string,
	tsOperands map[token.Pos]string, tsDefaults []tsDefaultArm) {
	nb := len(graph.Blocks)
	fc := &FunctionCFG{
		Blocks:     make([]BlockInfo, nb),
		Succs:      make([][]int32, nb),
		PosToBlock: make(map[string]BlockLoc),
	}
	for i := range fc.Blocks {
		fc.Blocks[i] = BlockInfo{Index: int32(i)} //nolint:gosec // block counts are small
	}

	// recordPos stores a position → block location in both the raw and
	// repo-root-stripped forms (consumers carry the stripped form — #27).
	//
	// First-write-wins over blocks iterated in index order. A position go/cfg emits
	// in two live blocks (e.g. a `select` comm-clause receive ident that appears in
	// both the dispatch block and the SelectCaseBody) is therefore claimed by the
	// lower-indexed block. This is a rare imprecision (no panic; the status consumer
	// would treat a select-conditional value as unconditional) — left as-is rather
	// than tracked because no realistic handler shape triggers it.
	recordPos := func(p string, loc BlockLoc) {
		if _, exists := fc.PosToBlock[p]; !exists {
			fc.PosToBlock[p] = loc
		}
		meta.cfgPosToFn[p] = fnKey
		if repoRoot != "" {
			if stripped := strings.TrimPrefix(p, repoRoot); stripped != p {
				if _, exists := fc.PosToBlock[stripped]; !exists {
					fc.PosToBlock[stripped] = loc
				}
				meta.cfgPosToFn[stripped] = fnKey
			}
		}
	}

	for _, block := range graph.Blocks {
		if !block.Live || int(block.Index) >= nb {
			continue
		}
		bi := block.Index
		branchKind := mapBlockKind(block.Kind)

		succs := make([]int32, 0, len(block.Succs))
		for _, s := range block.Succs {
			if s != nil {
				succs = append(succs, s.Index)
			}
		}
		fc.Blocks[bi] = BlockInfo{Index: bi, Kind: branchKind, NodeCount: int32(len(block.Nodes))} //nolint:gosec // small counts
		fc.Succs[bi] = succs

		// (1) Reachability capture: ALL live blocks, every node → (block, node index).
		// The same walk annotates type-switch default-arm writes (go/cfg folds the
		// default body into the post-switch block, which the branch pass below skips).
		//
		// Do NOT descend into nested function literals: go/cfg builds no CFG for a
		// closure body, so mapping a closure's inner positions to THIS function's
		// block would place them in the wrong control flow and let a closure's
		// statuses bleed into the enclosing handler's reachability. Closures get no
		// model and their consumers degrade cleanly (BlockFor → not found).
		for nodeIdx, node := range block.Nodes {
			loc := BlockLoc{Block: bi, Node: int32(nodeIdx)} //nolint:gosec // small counts
			ast.Inspect(node, func(nn ast.Node) bool {
				if nn == nil {
					return false
				}
				if _, isFuncLit := nn.(*ast.FuncLit); isFuncLit {
					return false // a closure has its own scope; don't fold it into this block
				}
				posStr := fset.Position(nn.Pos()).String()
				recordPos(posStr, loc)
				annotateDefaultArm(nn.Pos(), posStr, tsDefaults, edgesByPos, assignsByPos)
				return true
			})
		}

		// (2) BranchContext annotation — only for if/else/switch/select body blocks.
		if branchKind == "" {
			continue
		}
		var parentStmtPos int
		if block.Stmt != nil {
			parentStmtPos = meta.StringPool.Get(fset.Position(block.Stmt.Pos()).String())
		}
		ctx := &BranchContext{
			BlockIndex:    bi,
			BlockKind:     branchKind,
			ParentStmtPos: parentStmtPos,
		}
		if branchKind == "switch-case" && block.Stmt != nil {
			ctx.CaseValues = extractCaseValues(block.Stmt)
			ctx.CaseTypeRefs = extractCaseTypeRefs(block.Stmt, info)
			ctx.SwitchOperand = tsOperands[block.Stmt.Pos()]
		}
		// An `if r.Method == <M>` guard makes its then-arm method-conditional, the
		// same way a `switch r.Method` case is (spec 009, US2/FR-003). Recording the
		// method in CaseValues lets splitByConditionalMethods fan the handler out per
		// method whether it branches via switch or if.
		if branchKind == "if-then" && block.Stmt != nil {
			ctx.CaseValues = extractMethodGuard(block.Stmt, info)
		}
		for _, node := range block.Nodes {
			ast.Inspect(node, func(nn ast.Node) bool {
				if nn == nil {
					return false
				}
				if _, isFuncLit := nn.(*ast.FuncLit); isFuncLit {
					return false // a closure's edges belong to its own scope, not this branch
				}
				pos := fset.Position(nn.Pos()).String()
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

	fc.Dominators = computeDominators(fc.Succs)
	meta.FunctionCFGs[fnKey] = fc
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

// extractMethodGuard returns the HTTP method named by an `if <req>.Method == <M>`
// (or `<M> == <req>.Method`) condition — "POST" for both `== "POST"` and
// `== http.MethodPost` — so a method-conditional `if` dispatch fans out per method
// the same way a `switch r.Method` does (spec 009, US2/FR-003).
//
// It is gated on TYPE INFO: the `.Method` operand must resolve to
// net/http.Request.Method, and a `MethodXxx` constant operand must resolve from the
// net/http package. This prevents unrelated business logic (`s.Method == "GET"`,
// `foo.MethodPost == r.Method`) from being misread as route method dispatch.
// Returns nil for any other condition (and when type info is unavailable).
func extractMethodGuard(stmt ast.Stmt, info *types.Info) []string {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Cond == nil {
		return nil
	}
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return nil
	}
	if isHTTPRequestMethod(bin.X, info) {
		if m := httpMethodName(bin.Y, info); m != "" {
			return []string{m}
		}
	}
	if isHTTPRequestMethod(bin.Y, info) {
		if m := httpMethodName(bin.X, info); m != "" {
			return []string{m}
		}
	}
	return nil
}

// isHTTPRequestMethod reports whether e is a `<req>.Method` selector whose receiver
// type-resolves to net/http.Request (value or pointer). Requires type info; without
// it (or for any non-Request `.Method`) returns false — conservative, so a business
// type's `.Method` field is never mistaken for the HTTP method.
func isHTTPRequestMethod(e ast.Expr, info *types.Info) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Method" || info == nil {
		return false
	}
	return isNetHTTPNamed(info.TypeOf(sel.X), "Request")
}

// isNetHTTPNamed reports whether t (after one optional pointer deref) is the named
// type `name` declared in package net/http. Type aliases are resolved: under Go's
// default gotypesalias=1 an alias like `type Req = http.Request` is a *types.Alias,
// not a *types.Named, so Unalias is needed before/after the deref to recognise an
// aliased (or pointer-to-aliased) request type.
func isNetHTTPNamed(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "net/http" && obj.Name() == name
}

// httpMethodName resolves an HTTP-method expression to its canonical method string:
// a string literal ("post"), or an `http.MethodXxx` selector that type info confirms
// resolves from net/http (MethodPost → POST). Returns "" otherwise.
func httpMethodName(e ast.Expr, info *types.Info) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			if s := strings.ToUpper(strings.Trim(v.Value, `"`)); isHTTPMethodName(s) {
				return s
			}
		}
	case *ast.SelectorExpr:
		if v.Sel == nil || info == nil {
			return ""
		}
		// Only a constant declared in net/http (http.MethodXxx) counts.
		obj := info.ObjectOf(v.Sel)
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "net/http" {
			return ""
		}
		if strings.HasPrefix(v.Sel.Name, "Method") && len(v.Sel.Name) > len("Method") {
			if m := strings.ToUpper(strings.TrimPrefix(v.Sel.Name, "Method")); isHTTPMethodName(m) {
				return m
			}
		}
	}
	return ""
}

// isHTTPMethodName reports whether s (already upper-cased) is an HTTP method the
// analyzer splits operations on. It is deliberately kept in sync with the spec-side
// consumer (isValidHTTPMethodStr in internal/spec): capturing a method here that the
// consumer would later drop would silently no-op, so both sides list the same seven
// methods (CONNECT/TRACE are intentionally excluded — they are not REST operations).
func isHTTPMethodName(s string) bool {
	switch s {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
		return true
	}
	return false
}

// extractCaseTypeRefs extracts the case TYPES from a type-switch case clause
// (`switch x.(type) { case *T, S: ... }`) as TypeRefs, using go/types to confirm
// each case expression is a type (not a value, which would be a value-switch).
// Returns nil for value-switches, for `case nil:`/`default:` (the unconditional
// arm), and when type info is unavailable (spec 009, FR-011/R5).
func extractCaseTypeRefs(stmt ast.Stmt, info *types.Info) []*TypeRef {
	cc, ok := stmt.(*ast.CaseClause)
	if !ok || cc == nil || info == nil {
		return nil
	}
	var refs []*TypeRef
	for _, expr := range cc.List {
		tv, known := info.Types[expr]
		if !known || !tv.IsType() {
			continue // a value expression (value-switch) or untyped nil — not a type case
		}
		if ref := TypeRefFromExpr(expr, info); ref != nil {
			refs = append(refs, ref)
		}
	}
	return refs
}

// typeSwitchOperands maps each type-switch CaseClause (by its Pos) to the name of
// the variable being switched on — `switch x := v.(type)` and `switch v.(type)`
// both yield "v". This lets the per-case BranchContext record which parameter the
// switch discriminates, so a consumer can bind it to the call-site argument via
// ParamArgMap (spec 009, FR-011). Only simple ident operands are recorded; a
// selector/index operand yields "" (the consumer then degrades).
func typeSwitchOperands(body *ast.BlockStmt) map[token.Pos]string {
	out := make(map[token.Pos]string)
	ast.Inspect(body, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok || ts.Body == nil {
			return true
		}
		operand := typeSwitchOperandName(ts)
		if operand == "" {
			return true
		}
		for _, stmt := range ts.Body.List {
			if cc, ok := stmt.(*ast.CaseClause); ok {
				out[cc.Pos()] = operand
			}
		}
		return true
	})
	return out
}

// tsDefaultArm is the source-position span of a type-switch `default:` clause body
// plus the switched operand, used to annotate default-arm writes that go/cfg folds
// into the post-switch block (see annotateDefaultArm).
type tsDefaultArm struct {
	lo, hi  token.Pos
	operand string
}

// typeSwitchDefaultArms returns the body spans of every type-switch `default:`
// clause in body, each paired with its switched operand. Value/expression switch
// defaults are NOT included (only *ast.TypeSwitchStmt).
func typeSwitchDefaultArms(body *ast.BlockStmt) []tsDefaultArm {
	var out []tsDefaultArm
	ast.Inspect(body, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok || ts.Body == nil {
			return true
		}
		operand := typeSwitchOperandName(ts)
		if operand == "" {
			return true // a non-ident operand cannot be bound to a call-site argument
		}
		for _, stmt := range ts.Body.List {
			if cc, ok := stmt.(*ast.CaseClause); ok && cc.List == nil { // nil List ⇒ default clause
				out = append(out, tsDefaultArm{lo: cc.Pos(), hi: cc.End(), operand: operand})
			}
		}
		return true
	})
	return out
}

// annotateDefaultArm tags an edge/assignment inside a type-switch `default:` clause
// with a switch-case BranchContext carrying the operand and NO case types (the
// default marker), so a consumer can distinguish the default arm from code outside
// the switch (spec 009, FR-011). It only sets a Branch that is still nil, never
// overriding a real case annotation. When type-switch defaults nest, the TIGHTEST
// enclosing span wins, so an inner default binds to the inner operand rather than
// the lexically-enclosing outer one.
func annotateDefaultArm(pos token.Pos, posStr string, defaults []tsDefaultArm,
	edgesByPos map[string]*CallGraphEdge, assignsByPos map[string][]*Assignment) {
	best := -1
	var bestWidth token.Pos
	for i := range defaults {
		if pos < defaults[i].lo || pos >= defaults[i].hi {
			continue
		}
		if w := defaults[i].hi - defaults[i].lo; best == -1 || w < bestWidth {
			best, bestWidth = i, w
		}
	}
	if best == -1 {
		return
	}
	ctx := &BranchContext{BlockKind: "switch-case", SwitchOperand: defaults[best].operand}
	if edge, ok := edgesByPos[posStr]; ok && edge.Branch == nil {
		edge.Branch = ctx
	}
	for _, assign := range assignsByPos[posStr] {
		if assign.Branch == nil {
			assign.Branch = ctx
		}
	}
}

// typeSwitchOperandName returns the operand identifier of a type switch — the X in
// `X.(type)`, whether wrapped in an assignment (`x := X.(type)`) or a bare guard
// (`X.(type)`). Returns "" when X is not a simple ident.
func typeSwitchOperandName(ts *ast.TypeSwitchStmt) string {
	var assertX ast.Expr
	switch a := ts.Assign.(type) {
	case *ast.AssignStmt:
		if len(a.Rhs) == 1 {
			if ta, ok := a.Rhs[0].(*ast.TypeAssertExpr); ok {
				assertX = ta.X
			}
		}
	case *ast.ExprStmt:
		if ta, ok := a.X.(*ast.TypeAssertExpr); ok {
			assertX = ta.X
		}
	}
	if id, ok := assertX.(*ast.Ident); ok {
		return id.Name
	}
	return ""
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
