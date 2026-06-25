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

// The reachability query layer over the compact per-function control-flow model
// (spec 009, FR-001). Consumers (the conditional-analysis migration) resolve a
// metadata position to a block with BlockFor, then ask Reaches / Dominates.

// InstallFunctionCFGForTest installs a per-function CFG (from a successor adjacency
// and a position→block map, computing dominators) and registers its positions in the
// cfgPosToFn index, so cross-package tests of the reachability consumers can model a
// control-flow scenario without parsing real source. Test-only.
func (m *Metadata) InstallFunctionCFGForTest(fnKey string, succs [][]int32, posBlocks map[string]BlockLoc) {
	if m.FunctionCFGs == nil {
		m.FunctionCFGs = make(map[string]*FunctionCFG)
	}
	if m.cfgPosToFn == nil {
		m.cfgPosToFn = make(map[string]string)
	}
	blocks := make([]BlockInfo, len(succs))
	for i := range blocks {
		blocks[i] = BlockInfo{Index: int32(i)} //nolint:gosec // test block counts are small
	}
	m.FunctionCFGs[fnKey] = &FunctionCFG{
		Blocks:     blocks,
		Succs:      succs,
		Dominators: computeDominators(succs),
		PosToBlock: posBlocks,
	}
	for pos := range posBlocks {
		m.cfgPosToFn[pos] = fnKey
	}
}

// FnKeyForPos returns the FunctionCFGs key of the function containing the given
// source position (raw or repo-root-stripped form), or "" if unknown. A consumer
// holding only a position (a CallGraphEdge.Position) uses this to find the function
// whose model to query.
func (m *Metadata) FnKeyForPos(pos string) string {
	if m == nil || m.cfgPosToFn == nil {
		return ""
	}
	return m.cfgPosToFn[pos]
}

// fnCFG returns the compact control-flow model for the given function key, or nil.
func (m *Metadata) fnCFG(fnKey string) *FunctionCFG {
	if m == nil || m.FunctionCFGs == nil {
		return nil
	}
	return m.FunctionCFGs[fnKey]
}

// BlockFor resolves a metadata position string (a CallGraphEdge.Position or
// Assignment.Position) to its location within fnKey's CFG. ok=false when the
// function has no model or the position is in no live block — the caller then
// degrades to the single-path result (FR-008), never panicking.
func (m *Metadata) BlockFor(fnKey, pos string) (BlockLoc, bool) {
	fc := m.fnCFG(fnKey)
	if fc == nil || pos == "" || fc.PosToBlock == nil {
		return BlockLoc{}, false
	}
	loc, ok := fc.PosToBlock[pos]
	return loc, ok
}

// Reaches reports whether `to` is reachable from `from` along some control-flow
// path within fnKey. Within one block, reachability follows node order
// (from.Node <= to.Node). Across blocks it is the transitive closure of the
// successor graph, which terminates on loops/back-edges via a visited set.
//
// Intra-block reachability deliberately ignores a block's own back-edge: in a
// single-block loop (go/cfg emits a self-successor block) a node later in the
// block is NOT reported as reaching an earlier one. This conservatively
// UNDER-approximates (a loop-carried value can be missed), which is the safe
// direction for the status consumer — admitting the back-edge here would make the
// dominator-based kill predicate mutually overwrite both ends of the loop body and
// drop BOTH statuses. A reaching-definitions pass would resolve it precisely.
func (m *Metadata) Reaches(fnKey string, from, to BlockLoc) bool {
	fc := m.fnCFG(fnKey)
	if fc == nil {
		return false
	}
	n := int32(len(fc.Succs)) //nolint:gosec // CFG block count is small (tens); no int32 overflow
	if from.Block < 0 || from.Block >= n || to.Block < 0 || to.Block >= n {
		return false // a non-existent block reaches nothing
	}
	if from.Block == to.Block {
		return from.Node <= to.Node
	}
	return fc.blockReaches(from.Block, to.Block)
}

// Dominates reports whether block a dominates block b within fnKey (a is on every
// control-flow path from the entry to b).
func (m *Metadata) Dominates(fnKey string, a, b int32) bool {
	fc := m.fnCFG(fnKey)
	if fc == nil {
		return false
	}
	return blockDominates(fc.Dominators, a, b)
}

// DispatchArms returns the block indices of every arm of the method-dispatch group
// `group` within fnKey (every method-`switch` case INCLUDING the default, or every
// `if r.Method ==` chain arm), or nil. The common dominator of the returned blocks is
// that dispatch's tag, so a consumer can scope the dispatch region exactly — per
// dispatch, including arms whose response was lost, with no combined-case collapse.
//
// The returned slice aliases the stored model — treat it as READ-ONLY; do not sort or
// append in place (the sole consumer, contributingDispatchArms, only copies elements out).
func (m *Metadata) DispatchArms(fnKey string, group int) []int32 {
	fc := m.fnCFG(fnKey)
	if fc == nil {
		return nil
	}
	return fc.DispatchArms[group]
}

// IDom returns the immediate dominator of block b within fnKey (the branch point
// that routes to b) and ok=true. Returns (-1, false) when the function has no
// model, b is out of range, or b is the entry / unreachable (idom = -1). A
// consumer uses it to group a `switch r.Method` / `if r.Method ==` dispatch's arms
// by their shared branch point, so a fallback arm (switch default / bare else) can
// be told apart from an independent conditional with a different branch point.
func (m *Metadata) IDom(fnKey string, b int32) (int32, bool) {
	fc := m.fnCFG(fnKey)
	if fc == nil || int(b) < 0 || int(b) >= len(fc.Dominators) {
		return -1, false
	}
	d := fc.Dominators[b]
	if d < 0 {
		return -1, false
	}
	return d, true
}

// blockReaches does a breadth-first search over the successor graph with a visited
// set, so cyclic graphs (loops) terminate.
func (fc *FunctionCFG) blockReaches(from, to int32) bool {
	n := int32(len(fc.Succs)) //nolint:gosec // CFG block count is small (tens); no int32 overflow
	if from < 0 || from >= n {
		return false
	}
	visited := make([]bool, n)
	visited[from] = true
	queue := []int32{from}
	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		for _, s := range fc.Succs[b] {
			if s == to {
				return true
			}
			if s >= 0 && s < n && !visited[s] {
				visited[s] = true
				queue = append(queue, s)
			}
		}
	}
	return false
}
