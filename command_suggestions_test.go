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

// suggestionsForOracle is SuggestionsFor as it was before the names were lower-cased
// once per call; it is the reference for the suggestions and their order.
func suggestionsForOracle(c *Command, typedName string) []string {
	suggestions := []string{}
	for _, cmd := range c.commands {
		if cmd.IsAvailableCommand() {
			levenshteinDistance := ld(typedName, cmd.Name(), true)
			suggestByLevenshtein := levenshteinDistance <= c.SuggestionsMinimumDistance
			suggestByPrefix := strings.HasPrefix(strings.ToLower(cmd.Name()), strings.ToLower(typedName))
			if suggestByLevenshtein || suggestByPrefix {
				suggestions = append(suggestions, cmd.Name())
			}
			for _, explicitSuggestion := range cmd.SuggestFor {
				if strings.EqualFold(typedName, explicitSuggestion) {
					suggestions = append(suggestions, cmd.Name())
				}
			}
		}
	}
	return suggestions
}

func TestSuggestionsForLowersOnce(t *testing.T) {
	newRoot := func() *Command {
		root := &Command{Use: "root", SuggestionsMinimumDistance: 2}
		root.AddCommand(
			&Command{Use: "Server [args]", Run: emptyRun},
			&Command{Use: "status", Run: emptyRun, SuggestFor: []string{"State", "info"}},
			&Command{Use: "stats", Run: emptyRun},
			&Command{Use: "hidden", Run: emptyRun, Hidden: true},
			&Command{Use: "\u00c9tat", Run: emptyRun},
			&Command{Use: "topic"},
		)
		return root
	}
	tests := map[string]struct {
		typed string
		want  []string
	}{
		"success: case-insensitive distance": {typed: "SERVR", want: []string{"Server"}},
		"success: prefix and distance":       {typed: "sta", want: []string{"status", "stats"}},
		"success: explicit suggestion":       {typed: "STATE", want: []string{"status", "status", "stats"}},
		"success: hidden command is skipped": {typed: "hidden", want: []string{}},
		"success: non-ASCII upper case":      {typed: "\u00e9ta", want: []string{"\u00c9tat"}},
		"success: nothing close":             {typed: "zzzzzzzz", want: []string{}},
		"success: empty typed name":          {typed: "", want: []string{"Server", "status", "stats", "\u00c9tat"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			root := newRoot()
			got := root.SuggestionsFor(tt.typed)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("SuggestionsFor(%q) mismatch (-want +got):\n%s", tt.typed, diff)
			}
			if diff := cmp.Diff(suggestionsForOracle(root, tt.typed), got); diff != "" {
				t.Errorf("SuggestionsFor(%q) differs from the previous implementation (-previous +got):\n%s", tt.typed, diff)
			}
		})
	}
}

// TestSuggestionsForMatchesPrevious compares SuggestionsFor with the previous
// implementation over seeded random sibling sets and typed names, including mixed
// case, non-ASCII letters whose case mapping changes their length, explicit
// suggestions and unavailable commands.
func TestSuggestionsForMatchesPrevious(t *testing.T) {
	pieces := []string{"a", "b", "s", "t", "A", "S", "T", "-", "\u00e9", "\u00c9", "\u0130", "\u0131", "\u00df", "\u1e9e", "\u65e5"}
	rng := rand.New(rand.NewPCG(1, 0)) //nolint:gosec // deterministic test inputs, not a security context
	word := func(maxLen int) string {
		var sb strings.Builder
		for range rng.IntN(maxLen + 1) {
			sb.WriteString(pieces[rng.IntN(len(pieces))])
		}
		return sb.String()
	}
	for range 2000 {
		root := &Command{Use: "root", SuggestionsMinimumDistance: 1 + rng.IntN(3)}
		for range 1 + rng.IntN(8) {
			name := word(6)
			if name == "" {
				name = "x"
			}
			sub := &Command{Use: name, Hidden: rng.IntN(6) == 0}
			if rng.IntN(5) != 0 {
				sub.Run = emptyRun
			}
			if rng.IntN(4) == 0 {
				sub.SuggestFor = []string{word(4), word(4)}
			}
			root.AddCommand(sub)
		}
		for range 5 {
			typed := word(7)
			if diff := cmp.Diff(suggestionsForOracle(root, typed), root.SuggestionsFor(typed)); diff != "" {
				t.Fatalf("SuggestionsFor(%q) differs from the previous implementation (-previous +got):\n%s", typed, diff)
			}
		}
	}
}
