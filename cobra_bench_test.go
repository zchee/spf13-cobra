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
	"testing"

	"github.com/spf13/pflag"
)

var benchInt int

// benchActiveHelp is a variable, not a constant: a constant string operand is boxed
// statically and would hide the allocation a real caller pays.
var benchActiveHelp = "this is active help"

// benchPadName and benchPadding feed rpad through variables for the same reason.
var (
	benchPadName = "sub03"
	benchPadding = 12
)

// BenchmarkLd computes the edit distance between sibling-sized command names, as the
// suggestions for an unknown command do (lower-cased, case-insensitive).
func BenchmarkLd(b *testing.B) {
	pairs := [][2]string{{"sub5x", "sub57"}, {"completion", "complete"}, {"help", "hlep"}, {"namespace", "namespaces"}}
	b.ReportAllocs()
	for b.Loop() {
		for _, p := range pairs {
			benchInt += ld(p[0], p[1], true)
		}
	}
}

func BenchmarkRpad(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		benchSink = rpad(benchPadName, benchPadding)
	}
}

func BenchmarkConfigEnvVar(b *testing.B) {
	for _, name := range []string{"root", "my-prog"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchSink = configEnvVar(name, activeHelpEnvVarSuffix)
			}
		})
	}
}

// BenchmarkColdHelpers covers helpers off the hot paths; the completion helpers
// append into a slice that already has spare capacity.
func BenchmarkColdHelpers(b *testing.B) {
	b.Run("AppendActiveHelp", func(b *testing.B) {
		dst := make([]Completion, 0, 4)
		b.ReportAllocs()
		for b.Loop() {
			dst = AppendActiveHelp(dst[:0], benchActiveHelp)
		}
	})
	b.Run("OnlyValidArgs", func(b *testing.B) {
		cmd := &Command{Use: "c", ValidArgs: []string{"one\tfirst", "two\tsecond", "three"}}
		args := []string{"two", "three"}
		b.ReportAllocs()
		for b.Loop() {
			if err := OnlyValidArgs(cmd, args); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("appendFlagNameCompletions", func(b *testing.B) {
		fs := pflag.NewFlagSet("bench", pflag.ContinueOnError)
		fs.StringP("config", "c", "", "config file")
		flag := fs.Lookup("config")
		dst := make([]Completion, 0, 4)
		b.ReportAllocs()
		for b.Loop() {
			dst = append(dst[:0], getFlagNameCompletions(flag, "")...)
		}
		if len(dst) != 2 {
			b.Fatalf("got %d completions, want 2", len(dst))
		}
	})
}
