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
	rand "math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestCommandPathLenMatchesCommandPath checks commandPathLen against the length of the
// string CommandPath builds, on seeded random trees whose names mix ASCII, multibyte and
// combining characters and whose commands sometimes carry a display-name annotation.
func TestCommandPathLenMatchesCommandPath(t *testing.T) {
	names := []string{"a", "sub", "größe", "命令", "éclair", "x-y_z", "ünïcode", "日本語のコマンド", "k"}
	displayNames := []string{"kubectl plugin", "ü", "", "表示 名", "a b c"}
	rng := rand.New(rand.NewPCG(1, 1)) //nolint:gosec // seeded so that every run checks the same trees; not a security context

	var newCommand func(depth int) *Command
	newCommand = func(depth int) *Command {
		use := names[rng.IntN(len(names))] + strings.Repeat("z", rng.IntN(3))
		if rng.IntN(2) == 0 {
			use += " [args]"
		}
		c := &Command{Use: use}
		if rng.IntN(4) == 0 {
			c.Annotations = map[string]string{CommandDisplayNameAnnotation: displayNames[rng.IntN(len(displayNames))]}
		}
		if depth > 0 {
			for range rng.IntN(4) {
				c.AddCommand(newCommand(depth - 1))
			}
		}
		return c
	}

	var walk func(c *Command, visit func(*Command))
	walk = func(c *Command, visit func(*Command)) {
		visit(c)
		for _, child := range c.commands {
			walk(child, visit)
		}
	}

	checked := 0
	for tree := range 1000 {
		root := newCommand(1 + rng.IntN(4))
		walk(root, func(c *Command) {
			checked++
			if diff := cmp.Diff(len(c.CommandPath()), c.commandPathLen()); diff != "" {
				t.Fatalf("tree %d, command %q (path %q): commandPathLen mismatch (-want +got):\n%s", tree, c.Use, c.CommandPath(), diff)
			}
		})
	}
	t.Logf("compared %d commands in 1000 trees", checked)
}

// paddingState is the padding bookkeeping a parent keeps for its children, together with
// the paddings one of those children reports.
type paddingState struct {
	MaxUseLen          int
	MaxCommandPathLen  int
	MaxNameLen         int
	NamePadding        int
	UsagePadding       int
	CommandPathPadding int
}

func paddingsOf(child *Command) paddingState {
	p := child.parent
	return paddingState{
		MaxUseLen:          p.commandsMaxUseLen,
		MaxCommandPathLen:  p.commandsMaxCommandPathLen,
		MaxNameLen:         p.commandsMaxNameLen,
		NamePadding:        child.NamePadding(),
		UsagePadding:       child.UsagePadding(),
		CommandPathPadding: child.CommandPathPadding(),
	}
}

func findChild(t *testing.T, parent *Command, name string) *Command {
	t.Helper()
	for _, c := range parent.commands {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("%q has no child %q", parent.Name(), name)
	return nil
}

func executeWith(t *testing.T, root *Command, args ...string) {
	t.Helper()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%q): %v", args, err)
	}
}

// TestPaddingsOnBenchmarkTrees pins the padding values that AddCommand and RemoveCommand
// maintain, on the benchmark tree shapes and on trees whose names exceed the minimum
// paddings. The expected values were recorded from the implementation that built every
// command path as a string.
func TestPaddingsOnBenchmarkTrees(t *testing.T) {
	benchArgs := []string{"sub03", "--flag01", "x", "--count", "3"}
	tests := map[string]struct {
		build func(t *testing.T) *Command // returns the child whose parent is inspected
		want  paddingState
	}{
		"subs=30: fresh tree": {
			build: func(t *testing.T) *Command { return findChild(t, buildTree(30, 10, 2), "sub03") },
			want:  paddingState{MaxUseLen: 12, MaxCommandPathLen: 10, MaxNameLen: 5, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 11},
		},
		"subs=30: after the first Execute": {
			build: func(t *testing.T) *Command {
				root := buildTree(30, 10, 2)
				executeWith(t, root, benchArgs...)
				return findChild(t, root, "sub03")
			},
			want: paddingState{MaxUseLen: 14, MaxCommandPathLen: 15, MaxNameLen: 10, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 15},
		},
		"subs=30: leaf after the first Execute": {
			build: func(t *testing.T) *Command {
				root := buildTree(30, 10, 2)
				executeWith(t, root, benchArgs...)
				return findChild(t, findChild(t, root, "sub03"), "leaf")
			},
			want: paddingState{MaxUseLen: 4, MaxCommandPathLen: 10, MaxNameLen: 4, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 11},
		},
		"subs=30: after a second Execute": {
			build: func(t *testing.T) *Command {
				root := buildTree(30, 10, 2)
				executeWith(t, root, benchArgs...)
				executeWith(t, root, benchArgs...)
				return findChild(t, root, "sub03")
			},
			want: paddingState{MaxUseLen: 14, MaxCommandPathLen: 15, MaxNameLen: 10, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 15},
		},
		"subs=30: after a completion request": {
			build: func(t *testing.T) *Command {
				root := buildTree(30, 10, 2)
				executeWith(t, root, benchArgs...)
				executeWith(t, root, ShellCompRequestCmd, "sub03", "--fl")
				return findChild(t, root, "sub03")
			},
			want: paddingState{MaxUseLen: 25, MaxCommandPathLen: 15, MaxNameLen: 10, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 15},
		},
		"subs=30: after RemoveCommand": {
			build: func(t *testing.T) *Command {
				root := buildTree(30, 10, 2)
				executeWith(t, root, benchArgs...)
				root.RemoveCommand(findChild(t, root, "sub03"), findChild(t, root, "completion"))
				return findChild(t, root, "sub04")
			},
			want: paddingState{MaxUseLen: 14, MaxCommandPathLen: 10, MaxNameLen: 5, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 11},
		},
		"subs=100: fresh tree": {
			build: func(t *testing.T) *Command { return findChild(t, buildTree(100, 10, 2), "sub03") },
			want:  paddingState{MaxUseLen: 12, MaxCommandPathLen: 10, MaxNameLen: 5, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 11},
		},
		"subs=100: after the first Execute": {
			build: func(t *testing.T) *Command {
				root := buildTree(100, 10, 2)
				executeWith(t, root, benchArgs...)
				return findChild(t, root, "sub03")
			},
			want: paddingState{MaxUseLen: 14, MaxCommandPathLen: 15, MaxNameLen: 10, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 15},
		},
		"long names above the minimum paddings": {
			build: func(t *testing.T) *Command {
				root := &Command{Use: "a-rather-long-root-command"}
				mid := &Command{Use: "intermediate-command-name [flags]"}
				root.AddCommand(mid)
				mid.AddCommand(
					&Command{Use: "short", Run: emptyRun},
					&Command{Use: "a-much-longer-subcommand-name <first-argument> <second-argument>", Run: emptyRun},
				)
				return findChild(t, mid, "short")
			},
			want: paddingState{MaxUseLen: 64, MaxCommandPathLen: 82, MaxNameLen: 29, NamePadding: 29, UsagePadding: 64, CommandPathPadding: 82},
		},
		"long names after RemoveCommand": {
			build: func(t *testing.T) *Command {
				root := &Command{Use: "a-rather-long-root-command"}
				mid := &Command{Use: "intermediate-command-name [flags]"}
				root.AddCommand(mid)
				long := &Command{Use: "a-much-longer-subcommand-name <first-argument> <second-argument>", Run: emptyRun}
				mid.AddCommand(&Command{Use: "short-but-not-too-short", Run: emptyRun}, long)
				mid.RemoveCommand(long)
				return findChild(t, mid, "short-but-not-too-short")
			},
			want: paddingState{MaxUseLen: 23, MaxCommandPathLen: 76, MaxNameLen: 23, NamePadding: 23, UsagePadding: 25, CommandPathPadding: 76},
		},
		"display-name annotation with multibyte names": {
			build: func(t *testing.T) *Command {
				root := &Command{Use: "kubectl", Annotations: map[string]string{CommandDisplayNameAnnotation: "kubectl plugin-ünïcode"}}
				root.AddCommand(&Command{Use: "größe [args]", Run: emptyRun}, &Command{Use: "命令", Run: emptyRun})
				return findChild(t, root, "größe")
			},
			want: paddingState{MaxUseLen: 14, MaxCommandPathLen: 32, MaxNameLen: 7, NamePadding: 11, UsagePadding: 25, CommandPathPadding: 32},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := paddingsOf(tt.build(t))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("paddings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
