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
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// printHelper is one of the formatting print helpers, called the way it is now and the
// way it was before (formatting, then c.Print or c.PrintErr, which are unchanged).
type printHelper struct {
	current  func(c *Command, format string, args ...any)
	previous func(c *Command, format string, args ...any)
	toErr    bool // writes to ErrOrStderr instead of OutOrStderr
}

var printHelpers = map[string]printHelper{
	"Println": {
		current:  func(c *Command, _ string, args ...any) { c.Println(args...) },
		previous: func(c *Command, _ string, args ...any) { c.Print(fmt.Sprintln(args...)) },
	},
	"Printf": {
		current:  func(c *Command, format string, args ...any) { c.Printf(format, args...) },
		previous: func(c *Command, format string, args ...any) { c.Print(fmt.Sprintf(format, args...)) },
	},
	"PrintErrln": {
		current:  func(c *Command, _ string, args ...any) { c.PrintErrln(args...) },
		previous: func(c *Command, _ string, args ...any) { c.PrintErr(fmt.Sprintln(args...)) },
		toErr:    true,
	},
	"PrintErrf": {
		current:  func(c *Command, format string, args ...any) { c.PrintErrf(format, args...) },
		previous: func(c *Command, format string, args ...any) { c.PrintErr(fmt.Sprintf(format, args...)) },
		toErr:    true,
	},
}

type printStringer string

func (s printStringer) String() string { return "<" + string(s) + ">" }

func TestPrintHelpersMatchPreviousOutput(t *testing.T) {
	tests := map[string]struct {
		format string
		args   []any
	}{
		"no arguments":           {format: "plain text"},
		"strings":                {format: "%s and %s", args: []any{"first", "second"}},
		"mixed operands":         {format: "%d %v %q", args: []any{42, 3.5, "quoted"}},
		"nil and error values":   {format: "%v %v", args: []any{nil, errors.New("boom")}},
		"stringer":               {format: "%v", args: []any{printStringer("value")}},
		"missing and extra args": {format: "%s %s", args: []any{"only"}},
		"extra operands":         {format: "%s", args: []any{"a", 1, true}},
		"multibyte text":         {format: "größe %s", args: []any{"命令"}},
		"empty format and args":  {},
	}
	for helperName, helper := range printHelpers {
		for name, tt := range tests {
			t.Run(helperName+"/"+name, func(t *testing.T) {
				run := func(call func(c *Command, format string, args ...any)) (out, errOut *chunkWriter) {
					out, errOut = &chunkWriter{}, &chunkWriter{}
					c := &Command{Use: "c"}
					c.SetOut(out)
					c.SetErr(errOut)
					call(c, tt.format, tt.args...)
					return out, errOut
				}
				wantOut, wantErr := run(helper.previous)
				gotOut, gotErr := run(helper.current)
				if diff := cmp.Diff(wantOut.chunks, gotOut.chunks); diff != "" {
					t.Errorf("Write calls on the output writer (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(wantErr.chunks, gotErr.chunks); diff != "" {
					t.Errorf("Write calls on the error writer (-want +got):\n%s", diff)
				}
			})
		}
	}
}

// redirectingStringer moves the command's output and error writers when it is
// formatted, which shows whether the writer is resolved before or after formatting.
type redirectingStringer struct {
	cmd *Command
	to  io.Writer
}

func (r redirectingStringer) String() string {
	r.cmd.SetOut(r.to)
	r.cmd.SetErr(r.to)
	return "redirected"
}

func TestPrintHelpersFormatBeforeResolvingWriter(t *testing.T) {
	for helperName, helper := range printHelpers {
		t.Run(helperName, func(t *testing.T) {
			run := func(call func(c *Command, format string, args ...any)) (first, second *chunkWriter) {
				first, second = &chunkWriter{}, &chunkWriter{}
				c := &Command{Use: "c"}
				c.SetOut(first)
				c.SetErr(first)
				call(c, "%v", redirectingStringer{cmd: c, to: second})
				return first, second
			}
			wantFirst, wantSecond := run(helper.previous)
			gotFirst, gotSecond := run(helper.current)
			if diff := cmp.Diff(wantFirst.chunks, gotFirst.chunks); diff != "" {
				t.Errorf("writes to the writer set before formatting (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantSecond.chunks, gotSecond.chunks); diff != "" {
				t.Errorf("writes to the writer set while formatting (-want +got):\n%s", diff)
			}
			if len(gotSecond.chunks) != 1 {
				t.Errorf("the writer set while formatting got %d Write calls, want 1", len(gotSecond.chunks))
			}
		})
	}
}
