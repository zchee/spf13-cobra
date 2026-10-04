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
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// rpadFmt is the previous implementation of rpad, which built a format string with
// the padding as its width. It is the oracle for rpad.
func rpadFmt(s string, padding int) string {
	formattedString := fmt.Sprintf("%%-%ds", padding)
	return fmt.Sprintf(formattedString, s)
}

func TestRpadMatchesFmtWidth(t *testing.T) {
	short := []string{"", "a", "cobra", "größe", "命令行", "é́", "\xff\xfe", "tab\there", "  spaced  "}
	paddingRange := func(from, to int) []int {
		out := make([]int, 0, to-from+1)
		for p := from; p <= to; p++ {
			out = append(out, p)
		}
		return out
	}
	tests := map[string]struct {
		strings  []string
		paddings []int
	}{
		"paddings from -2000 to 2000": {
			strings:  short,
			paddings: paddingRange(-2000, 2000),
		},
		"around the switch to fmt at one million": {
			strings:  []string{"", "x", "größe"},
			paddings: append(paddingRange(999_990, 1_000_010), paddingRange(-1_000_010, -999_990)...),
		},
		"around the largest width fmt accepts": {
			strings:  []string{"x"},
			paddings: append(paddingRange(10_000_000, 10_000_020), -10_000_005, -10_000_015),
		},
		"integer bounds": {
			strings:  []string{"", "x", "größe"},
			paddings: []int{math.MinInt32, math.MaxInt32, math.MinInt64, math.MaxInt64, math.MinInt64 + 1},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, s := range tt.strings {
				for _, padding := range tt.paddings {
					want, got := rpadFmt(s, padding), rpad(s, padding)
					if want == got {
						continue
					}
					if len(want) > 200 || len(got) > 200 {
						t.Fatalf("rpad(%q, %d): length %d, want %d", s, padding, len(got), len(want))
					}
					t.Fatalf("rpad(%q, %d) mismatch (-want +got):\n%s", s, padding, cmp.Diff(want, got))
				}
			}
		})
	}
}
