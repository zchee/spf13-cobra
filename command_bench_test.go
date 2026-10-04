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
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

// countingWriter counts Write calls, a proxy for write(2) system calls on an
// unbuffered *os.File, and the bytes written.
type countingWriter struct {
	writes int
	bytes  int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	w.bytes += len(p)
	return len(p), nil
}

// reportWrites reports the average number of Write calls per benchmark iteration.
func reportWrites(b *testing.B, w *countingWriter) {
	b.ReportMetric(float64(w.writes)/float64(b.N), "writes/op")
}

// buildTree builds a root with subs subcommands named sub00, sub01, ...; each has
// flagsPer string flags plus --count and --dry-run, and, when depth > 1, a "leaf"
// child with flagsPer flags of its own. The root has three persistent flags.
// Output goes to io.Discard.
func buildTree(subs, flagsPer, depth int) *Command {
	root := &Command{Use: "root", Short: "root short", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringP("config", "c", "", "config file")
	root.PersistentFlags().BoolP("verbose", "v", false, "verbose")
	root.PersistentFlags().String("namespace", "default", "namespace")
	for i := range subs {
		sub := &Command{Use: fmt.Sprintf("sub%02d [args]", i), Short: fmt.Sprintf("sub %d short", i), Run: emptyRun}
		for f := range flagsPer {
			sub.Flags().String(fmt.Sprintf("flag%02d", f), "", fmt.Sprintf("flag %d usage", f))
		}
		sub.Flags().Int("count", 1, "count")
		sub.Flags().Bool("dry-run", false, "dry run")
		if depth > 1 {
			leaf := &Command{Use: "leaf", Short: "leaf short", Run: emptyRun}
			for f := range flagsPer {
				leaf.Flags().String(fmt.Sprintf("lflag%02d", f), "", "leaf flag")
			}
			sub.AddCommand(leaf)
		}
		root.AddCommand(sub)
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

// benchShapes are the tree shapes of the Execute benchmarks: 30 and 100 subcommands,
// 10 flags each, depth 2.
var benchShapes = []int{30, 100}

var executeArgs = []string{"sub03", "--flag01", "x", "--count", "3"}

func BenchmarkExecuteFreshTree(b *testing.B) {
	for _, subs := range benchShapes {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				root := buildTree(subs, 10, 2)
				root.SetArgs(executeArgs)
				b.StartTimer()
				if err := root.Execute(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkExecuteSteady(b *testing.B) {
	for _, subs := range benchShapes {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			root := buildTree(subs, 10, 2)
			root.SetArgs(executeArgs)
			if err := root.Execute(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := root.Execute(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFind(b *testing.B) {
	root := buildTree(100, 10, 2)
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	args := []string{"--namespace", "ns", "sub57", "leaf", "--lflag01", "x", "pos"}
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := root.Find(args); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHelp(b *testing.B) {
	root := buildTree(30, 10, 2)
	root.SetArgs([]string{"sub03", "--help"})
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	w := &countingWriter{}
	root.SetOut(w)
	b.ReportAllocs()
	for b.Loop() {
		if err := root.Execute(); err != nil {
			b.Fatal(err)
		}
	}
	reportWrites(b, w)
}

func BenchmarkRootHelp(b *testing.B) {
	root := buildTree(100, 10, 2)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	w := &countingWriter{}
	root.SetOut(w)
	b.ReportAllocs()
	for b.Loop() {
		if err := root.Execute(); err != nil {
			b.Fatal(err)
		}
	}
	reportWrites(b, w)
}

// BenchmarkUsage renders usage straight into a writer that is not a *bytes.Buffer:
// subs=30 is the subcommand sub03 of a 30-subcommand tree (12 local flags, 3
// inherited), subs=100 is the root of a 100-subcommand tree.
func BenchmarkUsage(b *testing.B) {
	for _, subs := range benchShapes {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			root := buildTree(subs, 10, 2)
			if err := root.Execute(); err != nil {
				b.Fatal(err)
			}
			cmd := root
			if subs == 30 {
				var err error
				if cmd, _, err = root.Find([]string{"sub03"}); err != nil {
					b.Fatal(err)
				}
			}
			w := &countingWriter{}
			root.SetOut(w)
			b.ReportAllocs()
			for b.Loop() {
				if err := cmd.Usage(); err != nil {
					b.Fatal(err)
				}
			}
			reportWrites(b, w)
		})
	}
}

var benchSink string

func BenchmarkUsageString(b *testing.B) {
	root := buildTree(30, 10, 2)
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	sub, _, err := root.Find([]string{"sub03"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		benchSink = sub.UsageString()
	}
	if !strings.Contains(benchSink, "Usage:") {
		b.Fatalf("unexpected usage output: %q", benchSink)
	}
}

func BenchmarkSuggestionsUnknownCommand(b *testing.B) {
	root := buildTree(100, 10, 2)
	root.SetArgs([]string{"sub5x"})
	if err := root.Execute(); err == nil {
		b.Fatal("expected an unknown command error")
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = root.Execute()
	}
}

func BenchmarkValidateFlagGroups(b *testing.B) {
	root := buildTree(30, 10, 2)
	sub := root.Commands()[3]
	sub.MarkFlagsRequiredTogether("flag01", "flag02")
	sub.MarkFlagsMutuallyExclusive("flag03", "flag04")
	sub.MarkFlagsOneRequired("flag05", "flag06")
	root.SetArgs([]string{sub.Name(), "--flag01", "a", "--flag02", "b", "--flag05", "c"})
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := root.Execute(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTraverse(b *testing.B) {
	root := buildTree(30, 10, 2)
	root.TraverseChildren = true
	root.SetArgs([]string{"-v", "sub03", "leaf", "--lflag01", "x"})
	if err := root.Execute(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := root.Execute(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrintErrln(b *testing.B) {
	root := buildTree(5, 5, 1)
	var buf bytes.Buffer
	root.SetErr(&buf)
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		root.PrintErrln("Error:", "unknown command")
	}
}

// BenchmarkSortCommands sorts 100 subcommands from the same scrambled order on every
// iteration (restoring the order is a copy without allocation).
func BenchmarkSortCommands(b *testing.B) {
	root := buildTree(100, 0, 1)
	sorted := slices.Clone(root.Commands())
	// A fixed scrambled order: stepping by 37 visits every index of 100 once.
	scrambled := make([]*Command, len(sorted))
	for i := range sorted {
		scrambled[i] = sorted[(i*37)%len(sorted)]
	}
	b.ReportAllocs()
	for b.Loop() {
		copy(root.commands, scrambled)
		root.commandsAreSorted = false
		if cmds := root.Commands(); cmds[0].Name() != "sub00" {
			b.Fatalf("first command after sorting is %q", cmds[0].Name())
		}
	}
}
