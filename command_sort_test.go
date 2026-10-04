// Copyright 2013-2023 The Cobra Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cobra

import (
	"fmt"
	rand "math/rand/v2"
	"slices"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// commandsByNameOracle is the sort.Interface that Commands sorted with before it used
// slices.SortFunc.
type commandsByNameOracle []*Command

func (c commandsByNameOracle) Len() int           { return len(c) }
func (c commandsByNameOracle) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c commandsByNameOracle) Less(i, j int) bool { return c[i].Name() < c[j].Name() }

// TestCommandsSortPermutationMatchesSortSort checks that Commands orders children with
// equal names exactly as sort.Sort did. Both sorts are unstable; they agree because the
// standard library generates both from the same pdqsort template, which Go does not
// document as a guarantee, so this test detects a divergence at the Go version in use.
func TestCommandsSortPermutationMatchesSortSort(t *testing.T) {
	defer func(enabled bool) { EnableCommandSorting = enabled }(EnableCommandSorting)
	EnableCommandSorting = true

	pool := make([]*Command, 300)
	for i := range pool {
		pool[i] = &Command{}
	}
	positions := func(cmds []*Command, index map[*Command]int) []int {
		out := make([]int, len(cmds))
		for i, c := range cmds {
			out[i] = index[c]
		}
		return out
	}

	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // seeded so that every run checks the same inputs; not a security context
	for trial := range 20000 {
		n := rng.IntN(len(pool))
		children := pool[:n]
		index := make(map[*Command]int, n)
		for i, c := range children {
			// Few distinct names, so most names occur several times.
			c.Use = fmt.Sprintf("c%d [args]", rng.IntN(max(1, n/3)))
			index[c] = i
		}

		want := slices.Clone(children)
		sort.Sort(commandsByNameOracle(want))
		parent := &Command{Use: "parent", commands: slices.Clone(children)}
		got := parent.Commands()

		if diff := cmp.Diff(positions(want, index), positions(got, index)); diff != "" {
			t.Fatalf("trial %d with %d commands: order of equally named commands differs from sort.Sort (-want +got):\n%s", trial, n, diff)
		}
	}
}
