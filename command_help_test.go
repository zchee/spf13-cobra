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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// defaultHelpFuncOracle is defaultHelpFunc as it was before the text and the blank
// line after it were written in one call; it is the reference for the bytes and
// their order relative to the usage.
func defaultHelpFuncOracle(w io.Writer, in any) error {
	c := in.(*Command)
	usage := c.Long
	if usage == "" {
		usage = c.Short
	}
	usage = trimRightSpace(usage)
	if usage != "" {
		fmt.Fprintln(w, usage)
		fmt.Fprintln(w)
	}
	if c.Runnable() || c.HasSubCommands() {
		fmt.Fprint(w, c.UsageString())
	}
	return nil
}

// renderHelp renders c's help with fn the way the default help function does.
func renderHelp(c *Command, fn func(io.Writer, any) error) error {
	c.mergePersistentFlags()
	return fn(c.OutOrStdout(), c)
}

func TestDefaultHelpFuncWrites(t *testing.T) {
	tests := map[string]struct {
		build      func() *Command
		wantWrites int
	}{
		"success: long text and usage": {
			build: func() *Command {
				root := &Command{Use: "root", Short: "root short", Long: "root long text\n\n", Run: emptyRun}
				root.Flags().Bool("flag", false, "a flag")
				return root
			},
			wantWrites: 2,
		},
		"success: short text only and usage": {
			build: func() *Command {
				return &Command{Use: "root", Short: "root short", Run: emptyRun}
			},
			wantWrites: 2,
		},
		"success: no text, usage only": {
			build: func() *Command {
				return &Command{Use: "root", Run: emptyRun}
			},
			wantWrites: 1,
		},
		"success: help topic, text only": {
			build: func() *Command {
				return &Command{Use: "topic", Long: "a help topic"}
			},
			wantWrites: 1,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var want, got recordingWriter
			oracleCmd := tt.build()
			oracleCmd.SetOut(&want)
			if err := renderHelp(oracleCmd, defaultHelpFuncOracle); err != nil {
				t.Fatal(err)
			}
			cmd := tt.build()
			cmd.SetOut(&got)
			cmd.HelpFunc()(cmd, nil)
			if diff := cmp.Diff(want.String(), got.String()); diff != "" {
				t.Errorf("help text differs from the previous implementation (-previous +got):\n%s", diff)
			}
			if len(got.chunks) != tt.wantWrites {
				t.Errorf("help issued %d Write calls, want %d: %q", len(got.chunks), tt.wantWrites, got.chunks)
			}
		})
	}
}

// TestDefaultHelpFuncSharedWriterOrder checks that a custom usage function writing
// to the help's writer directly still finds the help text already written.
func TestDefaultHelpFuncSharedWriterOrder(t *testing.T) {
	render := func(fn func(io.Writer, any) error) string {
		var w recordingWriter
		root := &Command{Use: "root", Long: "LONG TEXT", Run: emptyRun}
		root.SetOut(&w)
		root.SetUsageFunc(func(c *Command) error {
			fmt.Fprint(&w, "DIRECT\n")
			fmt.Fprint(c.OutOrStderr(), "VIA COMMAND\n")
			return nil
		})
		if err := renderHelp(root, fn); err != nil {
			t.Fatal(err)
		}
		return w.String()
	}
	want := render(defaultHelpFuncOracle)
	if diff := cmp.Diff("LONG TEXT\n\nDIRECT\nVIA COMMAND\n", want); diff != "" {
		t.Fatalf("unexpected output of the previous implementation (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want, render(defaultHelpFunc)); diff != "" {
		t.Errorf("output differs from the previous implementation (-previous +got):\n%s", diff)
	}
}

const helpExitHelperEnv = "COBRA_TEST_HELP_EXIT_HELPER"

// TestDefaultHelpFuncExitHelperProcess is not a test on its own: it runs in a child
// process started by TestDefaultHelpFuncUsageErrorExits and prints help whose usage
// function fails, which makes UsageString exit the process.
func TestDefaultHelpFuncExitHelperProcess(t *testing.T) {
	impl := os.Getenv(helpExitHelperEnv)
	if impl == "" {
		t.Skip("helper process for TestDefaultHelpFuncUsageErrorExits")
	}
	root := &Command{Use: "root", Long: "LONG TEXT", Run: emptyRun}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetUsageFunc(func(*Command) error { return errors.New("usage failed") })
	fn := defaultHelpFunc
	if impl == "previous" {
		fn = defaultHelpFuncOracle
	}
	_ = renderHelp(root, fn)
	os.Exit(0)
}

func TestDefaultHelpFuncUsageErrorExits(t *testing.T) {
	run := func(impl string) (string, string, int) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultHelpFuncExitHelperProcess$") //nolint:gosec // re-runs this test binary
		cmd.Env = append(os.Environ(), helpExitHelperEnv+"="+impl)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return stdout.String(), stderr.String(), code
	}
	for _, impl := range []string{"previous", "current"} {
		stdout, stderr, code := run(impl)
		if diff := cmp.Diff("LONG TEXT\n\n", stdout); diff != "" {
			t.Errorf("%s implementation: stdout mismatch (-want +got):\n%s", impl, diff)
		}
		if diff := cmp.Diff("Error: usage failed\n", stderr); diff != "" {
			t.Errorf("%s implementation: stderr mismatch (-want +got):\n%s", impl, diff)
		}
		if code != 1 {
			t.Errorf("%s implementation: exit code %d, want 1", impl, code)
		}
	}
}
