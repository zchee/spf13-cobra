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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

// getFlagNameCompletionsOracle is the previous flag-name completion, which returned a
// new slice per flag for the caller to append; it is the reference for the
// append-based version.
func getFlagNameCompletionsOracle(flag *pflag.Flag, toComplete string) []Completion {
	if nonCompletableFlag(flag) {
		return []Completion{}
	}

	var completions []Completion
	flagName := "--" + flag.Name
	if strings.HasPrefix(flagName, toComplete) {
		completions = append(completions, CompletionWithDesc(flagName, flag.Usage))
	}

	flagName = "-" + flag.Shorthand
	if len(flag.Shorthand) > 0 && strings.HasPrefix(flagName, toComplete) {
		completions = append(completions, CompletionWithDesc(flagName, flag.Usage))
	}

	return completions
}

// completeRequireFlagsOracle is completeRequireFlags as it was, on top of the oracle.
func completeRequireFlagsOracle(finalCmd *Command, toComplete string) []Completion {
	var completions []Completion
	doCompleteRequiredFlags := func(flag *pflag.Flag) {
		if _, present := flag.Annotations[BashCompOneRequiredFlag]; present {
			if !flag.Changed {
				completions = append(completions, getFlagNameCompletionsOracle(flag, toComplete)...)
			}
		}
	}
	finalCmd.InheritedFlags().VisitAll(doCompleteRequiredFlags)
	finalCmd.NonInheritedFlags().VisitAll(doCompleteRequiredFlags)
	return completions
}

// flagBranchOracle is the flag-name branch of getCompletions as it was, on top of the
// oracle: required flags first, and all completable flags only when no required flag
// matched.
func flagBranchOracle(finalCmd *Command, toComplete string) []Completion {
	completions := completeRequireFlagsOracle(finalCmd, toComplete)
	if len(completions) == 0 {
		doCompleteFlags := func(flag *pflag.Flag) {
			_, acceptsMultiple := flag.Value.(SliceValue)
			acceptsMultiple = acceptsMultiple ||
				strings.Contains(flag.Value.Type(), "Slice") ||
				strings.Contains(flag.Value.Type(), "Array") ||
				strings.HasPrefix(flag.Value.Type(), "stringTo")
			if !flag.Changed || acceptsMultiple {
				completions = append(completions, getFlagNameCompletionsOracle(flag, toComplete)...)
			}
		}
		finalCmd.InheritedFlags().VisitAll(doCompleteFlags)
		finalCmd.NonInheritedFlags().VisitAll(doCompleteFlags)
	}
	return completions
}

func TestAppendFlagNameCompletions(t *testing.T) {
	newFlag := func(name, shorthand string, hidden bool) *pflag.Flag {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.StringP(name, shorthand, "", "usage of "+name)
		if hidden {
			if err := fs.MarkHidden(name); err != nil {
				t.Fatal(err)
			}
		}
		return fs.Lookup(name)
	}
	tests := map[string]struct {
		flag       *pflag.Flag
		dst        []Completion
		toComplete string
		want       []Completion
	}{
		"success: empty prefix matches long and short names": {
			flag: newFlag("name", "n", false), toComplete: "",
			want: []Completion{"--name\tusage of name", "-n\tusage of name"},
		},
		"success: single dash without a shorthand matches only the long name": {
			flag: newFlag("name", "", false), toComplete: "-",
			want: []Completion{"--name\tusage of name"},
		},
		"success: empty prefix without a shorthand matches only the long name": {
			flag: newFlag("name", "", false), toComplete: "",
			want: []Completion{"--name\tusage of name"},
		},
		"success: shorthand prefix": {
			flag: newFlag("name", "n", false), toComplete: "-n",
			want: []Completion{"-n\tusage of name"},
		},
		"success: long-name prefix": {
			flag: newFlag("name", "n", false), toComplete: "--na",
			want: []Completion{"--name\tusage of name"},
		},
		"success: non-ASCII long name": {
			flag: newFlag("\u00fcber", "", false), toComplete: "--\u00fc",
			want: []Completion{"--\u00fcber\tusage of \u00fcber"},
		},
		"success: appends after existing completions": {
			flag: newFlag("name", "n", false), dst: []Completion{"sub"}, toComplete: "-",
			want: []Completion{"sub", "--name\tusage of name", "-n\tusage of name"},
		},
		"success: no match leaves a nil slice nil": {
			flag: newFlag("name", "n", false), toComplete: "x",
			want: nil,
		},
		"success: hidden flag leaves the slice unchanged": {
			flag: newFlag("name", "n", true), dst: []Completion{"sub"}, toComplete: "",
			want: []Completion{"sub"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := appendFlagNameCompletions(slices.Clone(tt.dst), tt.flag, tt.toComplete)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("appendFlagNameCompletions(%q, --%s, %q) mismatch (-want +got):\n%s", tt.dst, tt.flag.Name, tt.toComplete, diff)
			}
			oracle := append(slices.Clone(tt.dst), getFlagNameCompletionsOracle(tt.flag, tt.toComplete)...)
			if diff := cmp.Diff(oracle, got); diff != "" {
				t.Errorf("differs from appending the previous per-flag result (-previous +got):\n%s", diff)
			}
		})
	}
}

// randomFlagSpec describes one flag of a random tree (a root with persistent flags
// and two subcommands, the first with local flags): its kind, and whether it is
// required, hidden, deprecated or set on the command line.
type randomFlagSpec struct {
	name, shorthand, usage string
	kind                   int // 0 string, 1 bool, 2 string slice
	persistent             bool
	required               bool
	hidden                 bool
	deprecated             bool
	set                    bool
}

func newRandomFlagSpecs(rng *rand.Rand) []randomFlagSpec {
	shorthands := []byte("abcdefgijklmnopqrstuwxyz") // h stays with --help
	rng.Shuffle(len(shorthands), func(i, j int) { shorthands[i], shorthands[j] = shorthands[j], shorthands[i] })
	starts := []string{"f", "fo", "g", "x", "\u00fc", "日"}
	letters := []string{"a", "o", "r", "z", "-", "\u00e9"}
	seen := map[string]bool{"help": true}
	var specs []randomFlagSpec
	for i := range 1 + rng.IntN(10) {
		name := starts[rng.IntN(len(starts))]
		for range rng.IntN(5) {
			name += letters[rng.IntN(len(letters))]
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		spec := randomFlagSpec{
			name:       name,
			usage:      fmt.Sprintf("usage %d of %s", i, name),
			kind:       rng.IntN(3),
			persistent: rng.IntN(3) == 0,
			required:   rng.IntN(4) == 0,
			hidden:     rng.IntN(8) == 0,
			deprecated: rng.IntN(8) == 0,
			set:        rng.IntN(4) == 0,
		}
		if rng.IntN(2) == 0 {
			spec.shorthand = string(shorthands[i])
		}
		specs = append(specs, spec)
	}
	return specs
}

// buildRandomFlagTree builds the tree for specs and returns the root and the
// arguments (before the word being completed) that select the first subcommand and
// set the flags marked as set.
func buildRandomFlagTree(t *testing.T, specs []randomFlagSpec) (*Command, []string) {
	t.Helper()
	root := &Command{Use: "root"}
	sub := &Command{Use: "sub", Run: emptyRun}
	root.AddCommand(sub, &Command{Use: "other", Run: emptyRun})
	args := []string{"sub"}
	for _, s := range specs {
		fs := sub.Flags()
		if s.persistent {
			fs = root.PersistentFlags()
		}
		switch s.kind {
		case 0:
			fs.StringP(s.name, s.shorthand, "", s.usage)
		case 1:
			fs.BoolP(s.name, s.shorthand, false, s.usage)
		default:
			fs.StringSliceP(s.name, s.shorthand, nil, s.usage)
		}
		must := func(err error) {
			if err != nil {
				t.Fatal(err)
			}
		}
		if s.required {
			must(fs.SetAnnotation(s.name, BashCompOneRequiredFlag, []string{"true"}))
		}
		if s.hidden {
			must(fs.MarkHidden(s.name))
		}
		if s.deprecated {
			must(fs.MarkDeprecated(s.name, "do not use"))
		}
		// The "=" form keeps a set flag from being read as a flag awaiting its value
		// when it directly precedes the word being completed; deprecated flags are
		// not set, which only keeps their warning off the test output.
		if s.set && !s.deprecated {
			value := "v"
			if s.kind == 1 {
				value = "true"
			}
			args = append(args, "--"+s.name+"="+value)
		}
	}
	return root, args
}

// TestAppendFlagNameCompletionsMatchesPrevious compares the completions produced
// through both call sites, the flag-name branch of getCompletions and
// completeRequireFlags, with the previous implementation over seeded random flag
// sets, including flags without a shorthand and non-ASCII names.
func TestAppendFlagNameCompletionsMatchesPrevious(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 0)) //nolint:gosec // deterministic test inputs, not a security context
	compared := 0
	for range 400 {
		specs := newRandomFlagSpecs(rng)
		toCompletes := []string{"", "x", "-", "--", "--f", "--\u00fc", "-a", "-z"}
		for _, s := range specs {
			toCompletes = append(toCompletes, "--"+s.name[:min(len(s.name), 2)])
			if s.shorthand != "" {
				toCompletes = append(toCompletes, "-"+s.shorthand)
			}
		}
		for _, toComplete := range toCompletes {
			root, args := buildRandomFlagTree(t, specs)
			finalCmd, got, _, err := root.getCompletions(append(args, toComplete))
			if err != nil {
				t.Fatalf("getCompletions(%q): %v", append(args, toComplete), err)
			}
			if strings.HasPrefix(toComplete, "-") {
				if diff := cmp.Diff(flagBranchOracle(finalCmd, toComplete), got); diff != "" {
					t.Fatalf("flag-name completion for %q with flags %+v differs from the previous implementation (-previous +got):\n%s", toComplete, specs, diff)
				}
			}
			if diff := cmp.Diff(completeRequireFlagsOracle(finalCmd, toComplete), completeRequireFlags(finalCmd, toComplete)); diff != "" {
				t.Fatalf("completeRequireFlags(%q) with flags %+v differs from the previous implementation (-previous +got):\n%s", toComplete, specs, diff)
			}
			finalCmd.Flags().VisitAll(func(flag *pflag.Flag) {
				if diff := cmp.Diff(getFlagNameCompletionsOracle(flag, toComplete), getFlagNameCompletions(flag, toComplete)); diff != "" {
					t.Fatalf("getFlagNameCompletions(--%s, %q) differs from the previous implementation (-previous +got):\n%s", flag.Name, toComplete, diff)
				}
			})
			compared++
		}
	}
	t.Logf("compared %d completion requests", compared)
}
