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
	"io"
	"math"
	rand "math/rand/v2"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// chunkWriter records the bytes of every Write call separately.
type chunkWriter struct {
	chunks []string
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	w.chunks = append(w.chunks, string(p))
	return len(p), nil
}

func (w *chunkWriter) String() string {
	return strings.Join(w.chunks, "")
}

// writeCompletionsLineByLine is the previous implementation of writeCompletions, which
// printed every line with its own Write call. It is the oracle for the byte stream.
func writeCompletionsLineByLine(out io.Writer, completions []Completion, directive ShellCompDirective, noDescriptions, noActiveHelp bool) {
	for _, comp := range completions {
		if noActiveHelp && strings.HasPrefix(comp, activeHelpMarker) {
			continue
		}
		if noDescriptions {
			comp = strings.SplitN(comp, "\t", 2)[0]
		}
		comp = strings.SplitN(comp, "\n", 2)[0]
		comp = strings.TrimSpace(comp)
		fmt.Fprintln(out, comp)
	}
	fmt.Fprintf(out, ":%d\n", directive)
}

// checkCompletionChunks compares the output of writeCompletions with the oracle and
// checks how it was split into Write calls: one call when the output fits into
// maxCompletionWriteSize, and never a line split across two calls.
func checkCompletionChunks(t *testing.T, completions []Completion, directive ShellCompDirective, noDescriptions, noActiveHelp bool) {
	t.Helper()
	var want strings.Builder
	writeCompletionsLineByLine(&want, completions, directive, noDescriptions, noActiveHelp)
	got := &chunkWriter{}
	writeCompletions(got, completions, directive, noDescriptions, noActiveHelp)

	if diff := cmp.Diff(want.String(), got.String()); diff != "" {
		t.Fatalf("output mismatch for %d completions, directive %d, noDescriptions=%t, noActiveHelp=%t (-want +got):\n%s",
			len(completions), directive, noDescriptions, noActiveHelp, diff)
	}
	if want.Len() <= maxCompletionWriteSize {
		if diff := cmp.Diff(1, len(got.chunks)); diff != "" {
			t.Fatalf("%d bytes of output: Write calls (-want +got):\n%s", want.Len(), diff)
		}
	}
	for i, chunk := range got.chunks {
		if !strings.HasSuffix(chunk, "\n") {
			t.Fatalf("Write call %d of %d does not end at a line boundary: %q", i, len(got.chunks), chunk[max(0, len(chunk)-40):])
		}
	}
}

func repeatedCompletions(n, width int) []Completion {
	comps := make([]Completion, n)
	for i := range comps {
		name := fmt.Sprintf("item-%05d", i)
		comps[i] = name + "\t" + strings.Repeat("d", max(0, width-len(name)-1))
	}
	return comps
}

func TestWriteCompletionsMatchesLineByLineOutput(t *testing.T) {
	longItem := "huge\t" + strings.Repeat("x", 100<<10)
	withLongItem := repeatedCompletions(3000, 40)
	withLongItem[1500] = longItem
	// 1638 lines of 40 bytes, a 12-byte line and the directive line ":16\n" fill the bound exactly.
	nearBound := repeatedCompletions(1638, 39)
	nearBound = append(nearBound, "tail-of-ten")

	tests := map[string]struct {
		completions    []Completion
		directive      ShellCompDirective
		noDescriptions bool
		noActiveHelp   bool
	}{
		"empty list":                            {directive: ShellCompDirectiveNoFileComp},
		"plain names":                           {completions: []Completion{"alpha", "beta", "gamma"}, directive: ShellCompDirectiveDefault},
		"descriptions kept":                     {completions: []Completion{"alpha\tfirst", "beta\tsecond\tthird"}, directive: ShellCompDirectiveNoSpace},
		"descriptions removed":                  {completions: []Completion{"alpha\tfirst", "beta\tsecond\tthird", "\tonly a description"}, noDescriptions: true},
		"active help kept":                      {completions: []Completion{activeHelpMarker + "some help", "alpha"}, directive: ShellCompDirectiveNoFileComp},
		"active help removed":                   {completions: []Completion{activeHelpMarker + "some help", "alpha", activeHelpMarker}, noActiveHelp: true},
		"line breaks and surrounding space":     {completions: []Completion{"  spaced  ", "a\tdesc\nsecond line", "b\nc\td", "\n", "", "\t", "x\r\n", " \t \n "}},
		"line breaks without descriptions":      {completions: []Completion{"a\tdesc\nsecond line", "b\nc\td", "\tx\ny"}, noDescriptions: true},
		"multibyte and invalid UTF-8":           {completions: []Completion{"größe\tbeschreibung", "命令\t説明", "\xff\xfe\tbad", "é \t"}},
		"error directive":                       {completions: []Completion{"a"}, directive: ShellCompDirectiveError},
		"negative directive":                    {completions: []Completion{"a"}, directive: -1},
		"smallest directive":                    {completions: []Completion{"a"}, directive: math.MinInt},
		"largest directive":                     {completions: []Completion{"a"}, directive: math.MaxInt},
		"1000 completions":                      {completions: comps1000, directive: ShellCompDirectiveNoFileComp},
		"1000 completions without descriptions": {completions: comps1000, directive: ShellCompDirectiveNoFileComp, noDescriptions: true},
		"output exactly at the bound":           {completions: nearBound, directive: ShellCompDirectiveFilterDirs},
		"output above the bound":                {completions: repeatedCompletions(3000, 40), directive: ShellCompDirectiveNoFileComp},
		"one line longer than the bound":        {completions: withLongItem, directive: ShellCompDirectiveNoFileComp},
		"only a line longer than the bound":     {completions: []Completion{longItem}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			checkCompletionChunks(t, tt.completions, tt.directive, tt.noDescriptions, tt.noActiveHelp)
		})
	}
}

// TestWriteCompletionsRandomLists compares writeCompletions with the oracle on seeded
// random lists whose items mix tabs, line breaks, spaces, active-help markers and
// multibyte text, and whose total size ranges from empty to several buffers.
func TestWriteCompletionsRandomLists(t *testing.T) {
	pieces := []string{"a", "pod-01", "größe", "命令", " ", "  ", "\t", "\n", "\r", "\x00", "\xff", activeHelpMarker, "desc", strings.Repeat("w", 300)}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // seeded so that every run checks the same lists; not a security context
	for range 2000 {
		n := rng.IntN(400)
		if rng.IntN(10) == 0 {
			n = 3000 + rng.IntN(3000)
		}
		comps := make([]Completion, n)
		for i := range comps {
			var b strings.Builder
			for range rng.IntN(6) {
				b.WriteString(pieces[rng.IntN(len(pieces))])
			}
			comps[i] = b.String()
		}
		directive := ShellCompDirective(rng.IntN(130) - 2)
		checkCompletionChunks(t, comps, directive, rng.IntN(2) == 0, rng.IntN(2) == 0)
	}
}

// TestCompletionRequestOutputWrites checks the Write calls a __complete request makes
// on the command's output and error writers, and their order on a shared writer.
func TestCompletionRequestOutputWrites(t *testing.T) {
	withLongItem := repeatedCompletions(3000, 40)
	withLongItem[1500] = "huge\t" + strings.Repeat("x", 100<<10)

	tests := map[string]struct {
		completions    []Completion
		requestCmd     string
		sharedWriter   bool
		noDescriptions bool
		wantOutWrites  int // 0: not checked
	}{
		"1000 completions in one write": {
			completions:   comps1000,
			requestCmd:    ShellCompRequestCmd,
			wantOutWrites: 1,
		},
		"1000 completions without descriptions in one write": {
			completions:    comps1000,
			requestCmd:     ShellCompNoDescRequestCmd,
			noDescriptions: true,
			wantOutWrites:  1,
		},
		"shared writer: the directive message follows the completions": {
			completions:  comps1000,
			requestCmd:   ShellCompRequestCmd,
			sharedWriter: true,
		},
		"shared writer: a line longer than the buffer": {
			completions:  withLongItem,
			requestCmd:   ShellCompRequestCmd,
			sharedWriter: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			root := &Command{Use: "root", Run: emptyRun}
			child := &Command{
				Use: "child",
				Run: emptyRun,
				ValidArgsFunction: func(*Command, []string, string) ([]Completion, ShellCompDirective) {
					return tt.completions, ShellCompDirectiveNoFileComp
				},
			}
			root.AddCommand(child)
			out, errOut := &chunkWriter{}, &chunkWriter{}
			root.SetOut(out)
			if tt.sharedWriter {
				root.SetErr(out)
			} else {
				root.SetErr(errOut)
			}
			root.SetArgs([]string{tt.requestCmd, "child", ""})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			var want strings.Builder
			writeCompletionsLineByLine(&want, tt.completions, ShellCompDirectiveNoFileComp, tt.noDescriptions, false)
			directiveMsg := "Completion ended with directive: ShellCompDirectiveNoFileComp\n"
			if tt.sharedWriter {
				want.WriteString(directiveMsg)
				if diff := cmp.Diff(want.String(), out.String()); diff != "" {
					t.Fatalf("shared stream mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(directiveMsg, out.chunks[len(out.chunks)-1]); diff != "" {
					t.Errorf("last Write call on the shared writer (-want +got):\n%s", diff)
				}
				return
			}
			if diff := cmp.Diff(want.String(), out.String()); diff != "" {
				t.Fatalf("stdout mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(directiveMsg, errOut.String()); diff != "" {
				t.Errorf("stderr mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantOutWrites, len(out.chunks)); diff != "" {
				t.Errorf("stdout Write calls (-want +got):\n%s", diff)
			}
		})
	}
}
