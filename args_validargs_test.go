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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// onlyValidArgsSplitN is the previous implementation of OnlyValidArgs, which cut the
// descriptions with strings.SplitN. It is the oracle for OnlyValidArgs.
func onlyValidArgsSplitN(cmd *Command, args []string) error {
	if len(cmd.ValidArgs) > 0 {
		validArgs := make([]string, 0, len(cmd.ValidArgs))
		for _, v := range cmd.ValidArgs {
			validArgs = append(validArgs, strings.SplitN(v, "\t", 2)[0])
		}
		for _, v := range args {
			if !stringInSlice(v, validArgs) {
				return fmt.Errorf("invalid argument %q for %q%s", v, cmd.CommandPath(), cmd.findSuggestions(args[0]))
			}
		}
	}
	return nil
}

func TestOnlyValidArgsMatchesSplitN(t *testing.T) {
	validArgs := []string{"one\tthe first", "two\tsecond\twith a tab", "three", "\tdescription only", "four\t", "größe\tmultibyte"}
	tests := map[string]struct {
		validArgs []string
		args      []string
	}{
		"no valid args declared":                 {args: []string{"anything"}},
		"no args":                                {validArgs: validArgs},
		"names with and without descriptions":    {validArgs: validArgs, args: []string{"one", "two", "three", "four", "größe"}},
		"empty name of a description-only entry": {validArgs: validArgs, args: []string{""}},
		"a description is not a valid arg":       {validArgs: validArgs, args: []string{"the first"}},
		"name with its tab is not valid":         {validArgs: validArgs, args: []string{"four\t"}},
		"invalid arg with a suggestion":          {validArgs: validArgs, args: []string{"thre"}},
		"invalid arg after valid ones":           {validArgs: validArgs, args: []string{"one", "five"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			newCmd := func() *Command {
				root := &Command{Use: "root"}
				child := &Command{Use: "child", ValidArgs: tt.validArgs, Run: emptyRun}
				root.AddCommand(child)
				return child
			}
			errString := func(err error) string {
				if err == nil {
					return "<nil>"
				}
				return err.Error()
			}
			want := errString(onlyValidArgsSplitN(newCmd(), tt.args))
			got := errString(OnlyValidArgs(newCmd(), tt.args))
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("OnlyValidArgs(%q) mismatch (-want +got):\n%s", tt.args, diff)
			}
		})
	}
}
