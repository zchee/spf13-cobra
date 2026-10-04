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
	"testing"
)

// benchCompletionFresh runs one completion request per iteration on a newly built
// tree, as a shell does; building the tree is excluded from the measurement.
func benchCompletionFresh(b *testing.B, subs, flagsPer int, mutate func(*Command), args []string) {
	w := &countingWriter{}
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		root := buildTree(subs, flagsPer, 2)
		if mutate != nil {
			mutate(root)
		}
		root.SetOut(w)
		root.SetArgs(args)
		b.StartTimer()
		if err := root.Execute(); err != nil {
			b.Fatal(err)
		}
	}
	reportWrites(b, w)
}

func BenchmarkCompleteSubcommands(b *testing.B) {
	benchCompletionFresh(b, 100, 10, nil, []string{ShellCompRequestCmd, "su"})
}

func BenchmarkCompleteFlagNames(b *testing.B) {
	benchCompletionFresh(b, 30, 30, nil, []string{ShellCompRequestCmd, "sub03", "--"})
}

var comps1000 = func() []Completion {
	out := make([]Completion, 0, 1000)
	for i := range 1000 {
		out = append(out, fmt.Sprintf("pod-%04d\tdescription of pod %d", i, i))
	}
	return out
}()

func BenchmarkCompleteValidArgsFunction1000(b *testing.B) {
	benchCompletionFresh(b, 30, 10, func(root *Command) {
		root.Commands()[0].ValidArgsFunction = func(*Command, []string, string) ([]Completion, ShellCompDirective) {
			return comps1000, ShellCompDirectiveNoFileComp
		}
	}, []string{ShellCompRequestCmd, "sub00", ""})
}
