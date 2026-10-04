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

// ldReference is the straightforward full-matrix levenshtein implementation.
// It is intentionally kept as dumb as possible so that it can be trusted as
// the oracle for the single-row implementation used by ld.
func ldReference(s, t string, ignoreCase bool) int {
	if ignoreCase {
		s = strings.ToLower(s)
		t = strings.ToLower(t)
	}
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			if s[i-1] == t[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				d[i][j] = min(d[i-1][j], d[i][j-1], d[i-1][j-1]) + 1
			}
		}
	}
	return d[len(s)][len(t)]
}

func TestLdBasicCases(t *testing.T) {
	tests := map[string]struct {
		s, t       string
		ignoreCase bool
		want       int
	}{
		"both empty":          {s: "", t: "", want: 0},
		"empty source":        {s: "", t: "abc", want: 3},
		"empty target":        {s: "abc", t: "", want: 3},
		"same string":         {s: "hello", t: "hello", want: 0},
		"single replace":      {s: "a", t: "b", want: 1},
		"insert":              {s: "abc", t: "abcd", want: 1},
		"delete":              {s: "abcd", t: "abc", want: 1},
		"replace":             {s: "kitten", t: "sitten", want: 1},
		"swap to shorter row": {s: "a", t: "abcdefgh", want: 7},
		"longer source":       {s: "abcdefgh", t: "a", want: 7},
		"ignore case":         {s: "Hello", t: "hello", ignoreCase: true, want: 0},
		"multi byte string":   {s: "café", t: "cafe", want: 2},
		"shorter string of 63 bytes uses the stack row": {
			s: strings.Repeat("a", 63), t: strings.Repeat("a", 62) + "b", want: 1,
		},
		"shorter string of 64 bytes allocates the row": {
			s: strings.Repeat("a", 64), t: "b" + strings.Repeat("a", 64), want: 1,
		},
		"long strings": {
			s: strings.Repeat("abcde-", 20), t: strings.Repeat("abdce-", 20), want: 40,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, ld(tt.s, tt.t, tt.ignoreCase)); diff != "" {
				t.Errorf("ld(%q, %q, %t) mismatch (-want +got):\n%s", tt.s, tt.t, tt.ignoreCase, diff)
			}
			if diff := cmp.Diff(tt.want, ldReference(tt.s, tt.t, tt.ignoreCase)); diff != "" {
				t.Errorf("ldReference(%q, %q, %t) mismatch (-want +got):\n%s", tt.s, tt.t, tt.ignoreCase, diff)
			}
		})
	}
}

func TestLdMatchesReference(t *testing.T) {
	alphabet := []string{"a", "A", "b", "B", "c", "-", "é"}

	// Fixed seed: any failure reported below is reproducible as-is.
	rng := rand.New(rand.NewPCG(1, 1)) //nolint:gosec // seeded so that every run checks the same inputs; not a security context
	randString := func(n int) string {
		var sb strings.Builder
		for range n {
			sb.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		return sb.String()
	}
	randLen := func() int {
		// Mostly command-name lengths, sometimes past the stack row.
		if rng.IntN(20) == 0 {
			return 50 + rng.IntN(40)
		}
		return rng.IntN(13)
	}

	for range 20000 {
		s := randString(randLen())
		u := randString(randLen())

		for _, ignoreCase := range []bool{false, true} {
			want := ldReference(s, u, ignoreCase)
			if got := ld(s, u, ignoreCase); got != want {
				t.Fatalf("ld(%q, %q, %t) = %d, want %d", s, u, ignoreCase, got, want)
			}

			// The distance is symmetric; ld swaps its arguments internally, so
			// assert the caller cannot observe that.
			if rev := ld(u, s, ignoreCase); rev != want {
				t.Fatalf("ld(%q, %q, %t) = %d, want %d (asymmetric)", u, s, ignoreCase, rev, want)
			}

			// Distance is bounded by the length difference from below and by
			// the longer string from above. The bounds hold for the byte
			// lengths only when no case folding changed them.
			lo, hi := max(len(s)-len(u), len(u)-len(s)), max(len(s), len(u))
			if !ignoreCase && (want < lo || want > hi) {
				t.Fatalf("ld(%q, %q, false) = %d, want within [%d, %d]", s, u, want, lo, hi)
			}
		}

		if d := ld(s, s, false); d != 0 {
			t.Fatalf("ld(%q, %q, false) = %d, want 0", s, s, d)
		}
	}
}
