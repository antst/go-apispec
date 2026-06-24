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
	"testing"

	"github.com/stretchr/testify/assert"
)

// newFnCFG builds a FunctionCFG from a successor adjacency, computing dominators
// (so computeDominators is exercised). Block 0 is the entry.
func newFnCFG(succs [][]int32) *FunctionCFG {
	blocks := make([]BlockInfo, len(succs))
	for i := range succs {
		blocks[i] = BlockInfo{Index: int32(i)}
	}
	return &FunctionCFG{
		Blocks:     blocks,
		Succs:      succs,
		Dominators: computeDominators(succs),
		PosToBlock: map[string]BlockLoc{},
	}
}

func metaWith(fc *FunctionCFG) *Metadata {
	return &Metadata{FunctionCFGs: map[string]*FunctionCFG{"f": fc}}
}

func TestReachability_StraightLine(t *testing.T) {
	m := metaWith(newFnCFG([][]int32{{1}, {2}, {}}))
	assert.True(t, m.Reaches("f", BlockLoc{Block: 0}, BlockLoc{Block: 2}))
	assert.False(t, m.Reaches("f", BlockLoc{Block: 2}, BlockLoc{Block: 0}))
	assert.True(t, m.Dominates("f", 0, 2))
	assert.True(t, m.Dominates("f", 1, 2))
}

func TestReachability_IfElseSiblings(t *testing.T) {
	// 0 -> {1,2}; 1 -> 3; 2 -> 3 (merge)
	m := metaWith(newFnCFG([][]int32{{1, 2}, {3}, {3}, {}}))
	// both arms reach the merge → fan-out
	assert.True(t, m.Reaches("f", BlockLoc{Block: 1}, BlockLoc{Block: 3}))
	assert.True(t, m.Reaches("f", BlockLoc{Block: 2}, BlockLoc{Block: 3}))
	// siblings do not reach each other (no shadow)
	assert.False(t, m.Reaches("f", BlockLoc{Block: 1}, BlockLoc{Block: 2}))
	assert.False(t, m.Reaches("f", BlockLoc{Block: 2}, BlockLoc{Block: 1}))
	// neither arm dominates the merge; the entry does
	assert.False(t, m.Dominates("f", 1, 3))
	assert.False(t, m.Dominates("f", 2, 3))
	assert.True(t, m.Dominates("f", 0, 3))
}

func TestReachability_UnconditionalKill(t *testing.T) {
	// 0 -> 1 -> 2 ; an unconditional store in block 1 dominates the use in block 2.
	m := metaWith(newFnCFG([][]int32{{1}, {2}, {}}))
	assert.True(t, m.Dominates("f", 1, 2)) // later unconditional store kills earlier
}

func TestReachability_Loop(t *testing.T) {
	// 0 -> 1 ; 1 -> {2,3} ; 2 -> 1 (back-edge) ; 3 exit — FR-010: terminates on the cycle.
	m := metaWith(newFnCFG([][]int32{{1}, {2, 3}, {1}, {}}))
	assert.True(t, m.Reaches("f", BlockLoc{Block: 1}, BlockLoc{Block: 3}))
	assert.True(t, m.Reaches("f", BlockLoc{Block: 2}, BlockLoc{Block: 1})) // back-edge
	assert.True(t, m.Dominates("f", 1, 3))                                 // loop header dominates exit
}

func TestReachability_EarlyReturn(t *testing.T) {
	// 0 -> {1,2} ; 1 returns (no succ) ; 2 -> 3
	m := metaWith(newFnCFG([][]int32{{1, 2}, {}, {3}, {}}))
	assert.False(t, m.Reaches("f", BlockLoc{Block: 1}, BlockLoc{Block: 3})) // returns before 3
	assert.True(t, m.Reaches("f", BlockLoc{Block: 2}, BlockLoc{Block: 3}))
}

func TestReachability_IntraBlockNodeOrder(t *testing.T) {
	m := metaWith(newFnCFG([][]int32{{}}))
	assert.True(t, m.Reaches("f", BlockLoc{Block: 0, Node: 1}, BlockLoc{Block: 0, Node: 3}))
	assert.False(t, m.Reaches("f", BlockLoc{Block: 0, Node: 3}, BlockLoc{Block: 0, Node: 1}))
}

func TestReachability_BlockForAndDegrade(t *testing.T) {
	fc := newFnCFG([][]int32{{}})
	fc.PosToBlock["f.go:10:2"] = BlockLoc{Block: 0, Node: 1}
	m := metaWith(fc)

	loc, ok := m.BlockFor("f", "f.go:10:2")
	assert.True(t, ok)
	assert.Equal(t, int32(0), loc.Block)

	_, ok = m.BlockFor("f", "missing")
	assert.False(t, ok)

	// Absent/nil model degrades to false, never panics (FR-008).
	_, ok = m.BlockFor("absent", "f.go:10:2")
	assert.False(t, ok)
	assert.False(t, m.Reaches("absent", BlockLoc{}, BlockLoc{}))
	assert.False(t, m.Dominates("absent", 0, 1))
	var nilM *Metadata
	assert.Nil(t, nilM.fnCFG("x"))
	_, ok = nilM.BlockFor("x", "y")
	assert.False(t, ok)
}

func TestComputeDominators_Diamond(t *testing.T) {
	doms := computeDominators([][]int32{{1, 2}, {3}, {3}, {}})
	assert.Equal(t, int32(-1), doms[0]) // entry has no dominator
	assert.Equal(t, int32(0), doms[1])
	assert.Equal(t, int32(0), doms[2])
	assert.Equal(t, int32(0), doms[3]) // merge dominated by entry, not by either arm
	assert.True(t, blockDominates(doms, 0, 3))
	assert.True(t, blockDominates(doms, 3, 3)) // reflexive
	assert.False(t, blockDominates(doms, 1, 3))
	assert.False(t, blockDominates(doms, 1, 2))
}

func TestComputeDominators_Empty(t *testing.T) {
	assert.Empty(t, computeDominators(nil))
}

func TestReachability_OutOfRange(t *testing.T) {
	m := metaWith(newFnCFG([][]int32{{1}, {}}))
	// A from-block out of range degrades to false, never panics.
	assert.False(t, m.Reaches("f", BlockLoc{Block: 9}, BlockLoc{Block: 0}))
	// Dominance with an out-of-range target block degrades to false.
	fc := m.fnCFG("f")
	assert.False(t, blockDominates(fc.Dominators, 0, 9))
}
