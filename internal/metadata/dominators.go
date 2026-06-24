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

// computeDominators returns the immediate-dominator array for a CFG given its
// successor adjacency (succs[i] = successor block indices of block i), with the
// entry at index 0. idom[0] (the entry) is -1 (it has no dominator); for every
// other block reachable from the entry, idom[b] is its immediate dominator.
// Blocks unreachable from the entry get idom = -1.
//
// Implements the Cooper-Harvey-Kennedy "A Simple, Fast Dominance Algorithm"
// iterative data-flow formulation. It terminates on cyclic graphs (loops /
// back-edges) — FR-010, spec 009 — because the data-flow lattice is finite and
// monotone. O(blocks·edges) on the tiny per-function CFGs this analyzer builds.
func computeDominators(succs [][]int32) []int32 { //nolint:gocyclo // cohesive Cooper-Harvey-Kennedy dominator fixpoint
	n := len(succs)
	idom := make([]int32, n)
	for i := range idom {
		idom[i] = -1
	}
	if n == 0 {
		return idom
	}

	// Predecessors.
	preds := make([][]int32, n)
	for b := range succs {
		for _, s := range succs[b] {
			if int(s) >= 0 && int(s) < n {
				preds[s] = append(preds[s], int32(b))
			}
		}
	}

	// Iterative DFS postorder from the entry. post[b] = postorder index (entry
	// gets the highest); -1 for blocks unreachable from the entry.
	post := make([]int32, n)
	for i := range post {
		post[i] = -1
	}
	order := make([]int32, 0, n)
	visited := make([]bool, n)
	type frame struct {
		b    int32
		next int
	}
	stack := []frame{{b: 0}}
	visited[0] = true
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next < len(succs[top.b]) {
			s := succs[top.b][top.next]
			top.next++
			if int(s) >= 0 && int(s) < n && !visited[s] {
				visited[s] = true
				stack = append(stack, frame{b: s})
			}
		} else {
			post[top.b] = int32(len(order)) //nolint:gosec // CFG block count is small (tens); no int32 overflow
			order = append(order, top.b)
			stack = stack[:len(stack)-1]
		}
	}

	// Reverse postorder (excluding entry) is the worklist order.
	rpo := make([]int32, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		if order[i] != 0 {
			rpo = append(rpo, order[i])
		}
	}

	doms := make([]int32, n)
	for i := range doms {
		doms[i] = -1
	}
	doms[0] = 0 // entry dominates itself during the fixpoint; reset to -1 at the end

	intersect := func(b1, b2 int32) int32 {
		for b1 != b2 {
			for post[b1] < post[b2] {
				b1 = doms[b1]
			}
			for post[b2] < post[b1] {
				b2 = doms[b2]
			}
		}
		return b1
	}

	for changed := true; changed; {
		changed = false
		for _, b := range rpo {
			var newIdom int32 = -1
			for _, p := range preds[b] {
				if doms[p] == -1 {
					continue // predecessor not yet processed (or unreachable)
				}
				if newIdom == -1 {
					newIdom = p
				} else {
					newIdom = intersect(p, newIdom)
				}
			}
			if newIdom != -1 && doms[b] != newIdom {
				doms[b] = newIdom
				changed = true
			}
		}
	}

	copy(idom, doms)
	idom[0] = -1 // entry has no dominator
	return idom
}

// blockDominates reports whether block a dominates block b, given the immediate-
// dominator array from computeDominators (a is on b's idom chain, inclusive of b
// and the entry). Returns false for out-of-range or unreachable blocks.
func blockDominates(idom []int32, a, b int32) bool {
	for x := b; ; {
		if x == a {
			return true
		}
		if int(x) < 0 || int(x) >= len(idom) {
			return false
		}
		p := idom[x]
		if p < 0 || p == x {
			return false // reached the entry without finding a
		}
		x = p
	}
}
