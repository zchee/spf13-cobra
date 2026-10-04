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
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAppendActiveHelpConcatenation(t *testing.T) {
	tests := map[string]struct {
		comps []Completion
		help  string
	}{
		"success: nil slice":                  {help: "some help"},
		"success: after existing completions": {comps: []Completion{"one", "two\tdesc"}, help: "more help"},
		"success: empty help string":          {comps: []Completion{"one"}, help: ""},
		"success: verbs are not interpreted":  {help: "100% %s %d %!"},
		"success: multibyte and newline":      {help: "h\u00e9lp \u65e5\u672c\nnext"},
		"success: invalid UTF-8 is kept":      {help: "a\xffb"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// The previous implementation formatted the marker and the help string
			// with fmt.Sprintf("%s%s").
			want := append(slices.Clone(tt.comps), fmt.Sprintf("%s%s", activeHelpMarker, tt.help))
			got := AppendActiveHelp(slices.Clone(tt.comps), tt.help)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("AppendActiveHelp(%q, %q) mismatch (-previous +got):\n%s", tt.comps, tt.help, diff)
			}
		})
	}
}
