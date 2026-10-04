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
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
)

// configEnvVarOracleRegexp and configEnvVarOracle are the previous implementation of
// configEnvVar, kept as the reference the single-pass version must reproduce.
var configEnvVarOracleRegexp = regexp.MustCompile(`[^A-Z0-9_]`)

func configEnvVarOracle(name, suffix string) string {
	return configEnvVarOracleRegexp.ReplaceAllString(strings.ToUpper(fmt.Sprintf("%s_%s", name, suffix)), "_")
}

func TestConfigEnvVar(t *testing.T) {
	tests := map[string]struct {
		name   string
		suffix string
		want   string
	}{
		"success: lower-case ASCII name": {
			name: "root", suffix: activeHelpEnvVarSuffix, want: "ROOT_ACTIVE_HELP",
		},
		"success: dash becomes underscore": {
			name: "my-prog", suffix: configEnvVarSuffixDescriptions, want: "MY_PROG_COMPLETION_DESCRIPTIONS",
		},
		"success: digits and underscores are kept": {
			name: "prog_2", suffix: "x", want: "PROG_2_X",
		},
		"success: empty name": {
			name: "", suffix: "X", want: "_X",
		},
		"success: dotless i upper-cases to ASCII I": {
			name: "\u0131d", suffix: "S", want: "ID_S",
		},
		"success: long s upper-cases to ASCII S": {
			name: "\u017fh", suffix: "S", want: "SH_S",
		},
		"success: sharp s has no single-rune upper case": {
			name: "\u00df", suffix: "S", want: "__S",
		},
		"success: dotted capital I is not ASCII": {
			name: "\u0130", suffix: "S", want: "__S",
		},
		"success: title-case dz is one rune": {
			name: "\u01c5", suffix: "S", want: "__S",
		},
		"success: ligature ff is one rune": {
			name: "\ufb00", suffix: "S", want: "__S",
		},
		"success: Kelvin sign is not ASCII K": {
			name: "\u212a", suffix: "S", want: "__S",
		},
		"success: each CJK rune becomes one underscore": {
			name: "日本", suffix: "X", want: "___X",
		},
		"success: combining mark is its own rune": {
			name: "e\u0301", suffix: "X", want: "E__X",
		},
		"success: each invalid byte becomes one underscore": {
			name: "a\xffb", suffix: "X", want: "A_B_X",
		},
		"success: truncated multibyte sequence": {
			name: "a\xe6\x97", suffix: "X", want: "A___X",
		},
		"success: invalid byte in the suffix": {
			name: "prog", suffix: "\xc3", want: "PROG__",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := configEnvVar(tt.name, tt.suffix)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("configEnvVar(%q, %q) mismatch (-want +got):\n%s", tt.name, tt.suffix, diff)
			}
			if diff := cmp.Diff(configEnvVarOracle(tt.name, tt.suffix), got); diff != "" {
				t.Errorf("configEnvVar(%q, %q) differs from the regexp implementation (-regexp +got):\n%s", tt.name, tt.suffix, diff)
			}
		})
	}
}

// TestConfigEnvVarMatchesRegexpOracle compares configEnvVar with the regexp-based
// implementation for every rune in three positions and for seeded random inputs that
// mix ASCII, Latin-1, runes with unusual case mappings, CJK, combining marks and
// invalid or truncated UTF-8.
func TestConfigEnvVarMatchesRegexpOracle(t *testing.T) {
	mismatches := 0
	check := func(name, suffix string) {
		t.Helper()
		got, want := configEnvVar(name, suffix), configEnvVarOracle(name, suffix)
		if got == want {
			return
		}
		mismatches++
		if mismatches <= 10 {
			t.Errorf("configEnvVar(%q, %q) = %q, regexp implementation gives %q", name, suffix, got, want)
		}
	}

	// Every rune, in batches: string(r) is always valid UTF-8 (a surrogate becomes
	// U+FFFD), and both implementations map each valid rune on its own, so a batch
	// checks each of its runes in the same three positions as one call per rune
	// would, at a fraction of the regexp calls. A failing batch is rescanned one rune
	// at a time so the report names the runes.
	const batch = 4096
	for lo := rune(0); lo <= unicode.MaxRune; lo += batch {
		hi := min(lo+batch, unicode.MaxRune+1)
		var led, between strings.Builder
		for r := lo; r < hi; r++ {
			led.WriteString(string(r))
			between.WriteString("a" + string(r) + "b")
		}
		before := mismatches
		check(led.String(), "SUFFIX")
		check(between.String(), "x")
		check("prog", led.String())
		if mismatches == before {
			continue
		}
		for r := lo; r < hi; r++ {
			s := string(r)
			check(s, "SUFFIX")
			check("a"+s+"b", "x")
			check("prog", s)
		}
	}

	pieces := []string{
		"a", "z", "A", "Z", "0", "9", "_", "-", ".", " ", "/",
		"\u00e9", "\u00ff", "\u00b5", "\u00df",
		"\u0131", "\u017f", "\u0130", "\u01c5", "\ufb00", "\u212a",
		"日", "本", "語",
		"\u0301", "\u0308",
		"\x80", "\xff", "\xc3", "\xe6\x97", "\xf0\x9f\x98",
	}
	rng := rand.New(rand.NewPCG(1, 0)) //nolint:gosec // deterministic test inputs, not a security context
	randomString := func(maxPieces int) string {
		var sb strings.Builder
		for range rng.IntN(maxPieces + 1) {
			if rng.IntN(4) == 0 {
				sb.WriteRune(rng.Int32N(unicode.MaxRune + 1))
				continue
			}
			sb.WriteString(pieces[rng.IntN(len(pieces))])
		}
		return sb.String()
	}
	for range 300_000 {
		check(randomString(12), randomString(4))
	}
	if mismatches > 0 {
		t.Fatalf("%d inputs differ from the regexp implementation", mismatches)
	}
}
