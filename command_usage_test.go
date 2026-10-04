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
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

// defaultUsageFuncOracle is defaultUsageFunc as it was before its output was
// batched: one fmt call per piece, straight into w. It is the reference for the
// bytes, for their position relative to output written by user code, and for the
// partial output left when user code panics or exits.
func defaultUsageFuncOracle(w io.Writer, in any) error {
	c := in.(*Command)
	fmt.Fprint(w, "Usage:")
	if c.Runnable() {
		fmt.Fprintf(w, "\n  %s", c.UseLine())
	}
	if c.HasAvailableSubCommands() {
		fmt.Fprintf(w, "\n  %s [command]", c.CommandPath())
	}
	if len(c.Aliases) > 0 {
		fmt.Fprintf(w, "\n\nAliases:\n")
		fmt.Fprintf(w, "  %s", c.NameAndAliases())
	}
	if c.HasExample() {
		fmt.Fprintf(w, "\n\nExamples:\n")
		fmt.Fprintf(w, "%s", c.Example)
	}
	if c.HasAvailableSubCommands() {
		cmds := c.Commands()
		if len(c.Groups()) == 0 {
			fmt.Fprintf(w, "\n\nAvailable Commands:")
			for _, subcmd := range cmds {
				if subcmd.IsAvailableCommand() || subcmd.Name() == helpCommandName {
					fmt.Fprintf(w, "\n  %s %s", rpad(subcmd.Name(), subcmd.NamePadding()), subcmd.Short)
				}
			}
		} else {
			for _, group := range c.Groups() {
				fmt.Fprintf(w, "\n\n%s", group.Title)
				for _, subcmd := range cmds {
					if subcmd.GroupID == group.ID && (subcmd.IsAvailableCommand() || subcmd.Name() == helpCommandName) {
						fmt.Fprintf(w, "\n  %s %s", rpad(subcmd.Name(), subcmd.NamePadding()), subcmd.Short)
					}
				}
			}
			if !c.AllChildCommandsHaveGroup() {
				fmt.Fprintf(w, "\n\nAdditional Commands:")
				for _, subcmd := range cmds {
					if subcmd.GroupID == "" && (subcmd.IsAvailableCommand() || subcmd.Name() == helpCommandName) {
						fmt.Fprintf(w, "\n  %s %s", rpad(subcmd.Name(), subcmd.NamePadding()), subcmd.Short)
					}
				}
			}
		}
	}
	if c.HasAvailableLocalFlags() {
		fmt.Fprintf(w, "\n\nFlags:\n")
		fmt.Fprint(w, trimRightSpace(c.LocalFlags().FlagUsages()))
	}
	if c.HasAvailableInheritedFlags() {
		fmt.Fprintf(w, "\n\nGlobal Flags:\n")
		fmt.Fprint(w, trimRightSpace(c.InheritedFlags().FlagUsages()))
	}
	if c.HasHelpSubCommands() {
		fmt.Fprintf(w, "\n\nAdditional help topics:")
		for _, subcmd := range c.Commands() {
			if subcmd.IsAdditionalHelpTopicCommand() {
				fmt.Fprintf(w, "\n  %s %s", rpad(subcmd.CommandPath(), subcmd.CommandPathPadding()), subcmd.Short)
			}
		}
	}
	if c.HasAvailableSubCommands() {
		fmt.Fprintf(w, "\n\nUse \"%s [command] --help\" for more information about a command.", c.CommandPath())
	}
	fmt.Fprintln(w)
	return nil
}

// recordingWriter keeps every Write call separately. It is deliberately not a
// *bytes.Buffer, so defaultUsageFunc takes its buffered path.
type recordingWriter struct {
	chunks []string
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.chunks = append(w.chunks, string(p))
	return len(p), nil
}

func (w *recordingWriter) String() string { return strings.Join(w.chunks, "") }

// renderUsage renders c's usage with fn the way the default usage function does:
// persistent flags merged first, then fn writes to c.OutOrStderr().
func renderUsage(c *Command, fn func(io.Writer, any) error) error {
	c.mergePersistentFlags()
	return fn(c.OutOrStderr(), c)
}

// usageShapes builds the command trees whose usage the tests render; each call
// returns a fresh tree and the command to render.
var usageShapes = map[string]struct {
	build      func() *Command
	wantWrites int // 0: only the upper bound applies
}{
	"subcommand sub03 of a 30-subcommand tree": {
		build: func() *Command {
			root := buildTree(30, 10, 2)
			root.SetArgs([]string{})
			_ = root.Execute()
			sub, _, _ := root.Find([]string{"sub03"})
			return sub
		},
		wantWrites: 5,
	},
	"root of a 100-subcommand tree": {
		build: func() *Command {
			root := buildTree(100, 10, 2)
			root.SetArgs([]string{})
			_ = root.Execute()
			return root
		},
		wantWrites: 4,
	},
	"command without flags": {
		build: func() *Command {
			root := &Command{Use: "root"}
			sub := &Command{Use: "sub", Run: emptyRun}
			root.AddCommand(sub)
			return sub
		},
		wantWrites: 2,
	},
	"groups, ungrouped commands, aliases, example and help topics": {
		build: func() *Command {
			root := &Command{Use: "root", Aliases: []string{"r", "rt"}, Example: "  root a --flag", Run: emptyRun}
			root.AddGroup(&Group{ID: "one", Title: "Group one:"}, &Group{ID: "two", Title: "Group two:"})
			root.PersistentFlags().String("config", "", "config file")
			root.Flags().Bool("local", false, "local flag")
			root.AddCommand(
				&Command{Use: "a", Short: "a short", GroupID: "one", Run: emptyRun},
				&Command{Use: "bbbbbb", Short: "b short", GroupID: "two", Run: emptyRun},
				&Command{Use: "ungrouped", Short: "no group", Run: emptyRun},
				&Command{Use: "hidden", Hidden: true, Run: emptyRun},
				&Command{Use: "old", Deprecated: "gone", Run: emptyRun},
				&Command{Use: "topic", Short: "a help topic"},
			)
			return root
		},
	},
	"leaf with inherited flags only": {
		build: func() *Command {
			root := &Command{Use: "root"}
			root.PersistentFlags().StringP("config", "c", "", "config file")
			mid := &Command{Use: "mid"}
			leaf := &Command{Use: "leaf", Run: emptyRun, DisableFlagsInUseLine: true}
			mid.AddCommand(leaf)
			root.AddCommand(mid)
			return leaf
		},
	},
}

func TestDefaultUsageFuncWrites(t *testing.T) {
	for name, shape := range usageShapes {
		t.Run(name, func(t *testing.T) {
			oracleCmd := shape.build()
			var want recordingWriter
			oracleCmd.SetOut(&want)
			if err := renderUsage(oracleCmd, defaultUsageFuncOracle); err != nil {
				t.Fatal(err)
			}

			cmd := shape.build()
			var got recordingWriter
			cmd.SetOut(&got)
			if err := cmd.Usage(); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want.String(), got.String()); diff != "" {
				t.Errorf("usage text differs from the previous implementation (-previous +got):\n%s", diff)
			}
			if diff := cmp.Diff(want.String(), cmd.UsageString()); diff != "" {
				t.Errorf("UsageString differs from the previous implementation (-previous +got):\n%s", diff)
			}
			if slices.Contains(got.chunks, "") {
				t.Errorf("an empty Write was issued: %q", got.chunks)
			}
			if len(got.chunks) > 6 {
				t.Errorf("Usage issued %d Write calls, want at most 6: %q", len(got.chunks), got.chunks)
			}
			if shape.wantWrites != 0 && len(got.chunks) != shape.wantWrites {
				t.Errorf("Usage issued %d Write calls, want %d: %q", len(got.chunks), shape.wantWrites, got.chunks)
			}
			t.Logf("Write calls: previous %d, now %d", len(want.chunks), len(got.chunks))
		})
	}
}

// noisyFlagValue is a string flag value whose Type method writes a marker to w,
// or panics when panics is set, standing in for user code that runs while usage
// is rendered.
type noisyFlagValue struct {
	w      io.Writer
	name   string
	panics bool
	value  string
}

func (v *noisyFlagValue) String() string     { return v.value }
func (v *noisyFlagValue) Set(s string) error { v.value = s; return nil }
func (v *noisyFlagValue) Type() string {
	if v.panics {
		panic("Type of " + v.name)
	}
	fmt.Fprintf(v.w, "\x00T:%s\x00", v.name)
	return "noisy"
}

// callbackTree builds a root and a subcommand whose usage runs user code: a global
// normalization function and pflag.Value.Type methods that write markers to w (or
// panic on the flag named panicOn).
func callbackTree(w io.Writer, writingNormalizer bool, panicOn string) (*Command, *Command) {
	root := &Command{Use: "root", Short: "root short"}
	if writingNormalizer {
		root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
			if name == panicOn {
				panic("normalizing " + name)
			}
			fmt.Fprintf(w, "\x00N:%s\x00", name)
			return pflag.NormalizedName(name)
		})
	}
	root.PersistentFlags().Var(&noisyFlagValue{w: w, name: "ptyped", panics: panicOn == "ptyped"}, "ptyped", "persistent typed flag")
	root.PersistentFlags().Bool("verbose", false, "verbose")
	sub := &Command{Use: "sub", Aliases: []string{"s"}, Short: "sub short", Run: emptyRun}
	sub.Flags().Var(&noisyFlagValue{w: w, name: "typed", panics: panicOn == "typed"}, "typed", "typed flag")
	sub.Flags().String("plain", "", "plain flag")
	sub.AddCommand(&Command{Use: "leaf", Short: "leaf short", Run: emptyRun})
	root.AddCommand(sub, &Command{Use: "other", Short: "other short", Run: emptyRun})
	root.SetOut(w)
	return root, sub
}

// canonicalStream splits a stream into the text written by the usage function and
// the markers written by user code, and sorts the markers between two pieces of text:
// a global normalization function runs over a map, so only the multiset of its
// calls between two writes is deterministic, not their order.
func canonicalStream(s string) []string {
	var out, group []string
	for i, part := range strings.Split(s, "\x00") {
		if i%2 == 1 {
			group = append(group, part)
			continue
		}
		if part == "" {
			continue
		}
		slices.Sort(group)
		out = append(out, "markers "+strings.Join(group, ","), "text "+part)
		group = nil
	}
	slices.Sort(group)
	return append(out, "markers "+strings.Join(group, ","))
}

type usageWriter interface {
	io.Writer
	String() string
}

func TestDefaultUsageFuncCallbackInterleaving(t *testing.T) {
	writers := map[string]func() usageWriter{
		"buffered path (writer is not a *bytes.Buffer)": func() usageWriter { return &recordingWriter{} },
		"direct path (writer is a *bytes.Buffer)":       func() usageWriter { return &bytes.Buffer{} },
	}
	tests := map[string]struct {
		writingNormalizer bool
		root              bool
	}{
		"success: Type writes, subcommand usage":                      {},
		"success: Type and global normalizer write, subcommand usage": {writingNormalizer: true},
		"success: Type and global normalizer write, root usage":       {writingNormalizer: true, root: true},
	}
	for wname, newWriter := range writers {
		for name, tt := range tests {
			t.Run(wname+"/"+name, func(t *testing.T) {
				render := func(fn func(io.Writer, any) error) string {
					w := newWriter()
					root, sub := callbackTree(w, tt.writingNormalizer, "")
					cmd := sub
					if tt.root {
						cmd = root
					}
					if err := renderUsage(cmd, fn); err != nil {
						t.Fatal(err)
					}
					return w.String()
				}
				want, got := render(defaultUsageFuncOracle), render(defaultUsageFunc)
				if !strings.Contains(got, "\x00T:") {
					t.Fatalf("no Type marker in the output, the test does not exercise user code: %q", got)
				}
				if diff := cmp.Diff(canonicalStream(want), canonicalStream(got)); diff != "" {
					t.Errorf("text or user-code output moved relative to the previous implementation (-previous +got):\n%s", diff)
				}
			})
		}
	}
}

func TestDefaultUsageFuncPanicInUserCode(t *testing.T) {
	tests := map[string]struct {
		panickingNormalizer bool
		panicOn             string
		root                bool
	}{
		"success: local flag Type panics":           {panicOn: "typed"},
		"success: inherited flag Type panics":       {panicOn: "ptyped"},
		"success: root persistent flag Type panics": {panicOn: "ptyped", root: true},
		"success: normalizer panics on merge":       {panickingNormalizer: true, panicOn: "plain"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			render := func(fn func(io.Writer, any) error) (out string, recovered any) {
				w := &recordingWriter{}
				// The tree is built without the writing normalizer: setting a global
				// normalization function runs it over the flags in map order, and
				// its output would make the recorded streams differ between runs.
				panicOn := ""
				if !tt.panickingNormalizer {
					panicOn = tt.panicOn
				}
				root, sub := callbackTree(w, false, panicOn)
				cmd := sub
				if tt.root {
					cmd = root
				}
				// A global normalization function runs over every flag as soon as it is
				// set and on every merge, so it is armed only after the merge that
				// precedes the usage function: it then panics at the first call site
				// inside the usage function.
				armed := false
				if tt.panickingNormalizer {
					root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
						if armed && name == tt.panicOn {
							panic("normalizing " + name)
						}
						return pflag.NormalizedName(name)
					})
				}
				defer func() {
					recovered = recover()
					out = w.String()
				}()
				cmd.mergePersistentFlags()
				armed = true
				_ = fn(cmd.OutOrStderr(), cmd)
				return w.String(), nil
			}
			wantOut, wantPanic := render(defaultUsageFuncOracle)
			gotOut, gotPanic := render(defaultUsageFunc)
			if wantPanic == nil || wantOut == "" {
				t.Fatalf("the previous implementation did not panic after writing output; the test does not exercise a partial write")
			}
			if diff := cmp.Diff(fmt.Sprint(wantPanic), fmt.Sprint(gotPanic)); diff != "" {
				t.Errorf("panic value differs (-previous +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantOut, gotOut); diff != "" {
				t.Errorf("output written before the panic differs (-previous +got):\n%s", diff)
			}
		})
	}
}

const usageExitHelperEnv = "COBRA_TEST_USAGE_EXIT_HELPER"

// TestDefaultUsageFuncExitHelperProcess is not a test on its own: it runs in a child
// process started by TestDefaultUsageFuncExitInUserCode and renders usage to stdout
// with a pflag.Value.Type that exits the process.
func TestDefaultUsageFuncExitHelperProcess(t *testing.T) {
	impl := os.Getenv(usageExitHelperEnv)
	if impl == "" {
		t.Skip("helper process for TestDefaultUsageFuncExitInUserCode")
	}
	root := &Command{Use: "root"}
	root.PersistentFlags().Bool("verbose", false, "verbose")
	sub := &Command{Use: "sub", Short: "sub short", Run: emptyRun}
	sub.Flags().String("aaa", "", "first flag")
	sub.Flags().Var(exitingValue{}, "exiting", "a flag whose Type exits")
	root.AddCommand(sub)
	root.SetOut(os.Stdout)
	fn := defaultUsageFunc
	if impl == "previous" {
		fn = defaultUsageFuncOracle
	}
	_ = renderUsage(sub, fn)
	os.Exit(0)
}

type exitingValue struct{}

func (exitingValue) String() string   { return "" }
func (exitingValue) Set(string) error { return nil }
func (exitingValue) Type() string {
	fmt.Fprint(os.Stdout, "<exiting>")
	os.Exit(7)
	return ""
}

func TestDefaultUsageFuncExitInUserCode(t *testing.T) {
	run := func(impl string) (string, int) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultUsageFuncExitHelperProcess$") //nolint:gosec // re-runs this test binary
		cmd.Env = append(os.Environ(), usageExitHelperEnv+"="+impl)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := cmd.Run()
		code := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return stdout.String(), code
	}
	wantOut, wantCode := run("previous")
	gotOut, gotCode := run("current")
	if wantCode != 7 || !strings.Contains(wantOut, "<exiting>") {
		t.Fatalf("the helper did not exit from user code: code %d, output %q", wantCode, wantOut)
	}
	if gotCode != wantCode {
		t.Errorf("exit code %d, previous implementation %d", gotCode, wantCode)
	}
	if diff := cmp.Diff(wantOut, gotOut); diff != "" {
		t.Errorf("output before the exit differs (-previous +got):\n%s", diff)
	}
}
