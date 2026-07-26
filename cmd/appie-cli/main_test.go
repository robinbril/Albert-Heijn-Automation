package main

import (
	"flag"
	"reflect"
	"testing"
)

// newTestFlagSet mirrors a command with one string, one int and one bool flag.
func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("match", "", "")
	fs.Int("limit", 10, "")
	fs.Bool("pii", false, "")
	return fs
}

func TestPermute(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flags after positional are hoisted",
			args: []string{"pasta", "-limit", "3"},
			want: []string{"-limit", "3", "pasta"},
		},
		{
			name: "bool flag consumes nothing",
			args: []string{"-pii", "melk"},
			want: []string{"-pii", "melk"},
		},
		{
			name: "bool flag before positional stays put",
			args: []string{"melk", "-pii"},
			want: []string{"-pii", "melk"},
		},
		{
			name: "equals form is kept whole",
			args: []string{"pasta", "-limit=3"},
			want: []string{"-limit=3", "pasta"},
		},
		{
			name: "double dash ends flag parsing",
			args: []string{"--", "-not-a-flag"},
			want: []string{"-not-a-flag"},
		},
		{
			name: "several positionals keep their order",
			args: []string{"111", "222", "-limit", "5", "333"},
			want: []string{"-limit", "5", "111", "222", "333"},
		},
		{
			name: "no flags is unchanged",
			args: []string{"111", "222"},
			want: []string{"111", "222"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permute(newTestFlagSet(), tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("permute(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestPermuteThenParse(t *testing.T) {
	fs := newTestFlagSet()
	if err := fs.Parse(permute(fs, []string{"pasta", "-limit", "3", "-pii"})); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := fs.Lookup("limit").Value.String(); got != "3" {
		t.Errorf("limit = %s, want 3", got)
	}
	if got := fs.Lookup("pii").Value.String(); got != "true" {
		t.Errorf("pii = %s, want true", got)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "pasta" {
		t.Errorf("positionals = %v, want [pasta]", fs.Args())
	}
}

func TestCommandNamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, cmd := range commands() {
		if seen[cmd.name] {
			t.Errorf("duplicate command name: %s", cmd.name)
		}
		seen[cmd.name] = true

		if cmd.summary == "" {
			t.Errorf("command %s has no summary", cmd.name)
		}
		if cmd.run == nil {
			t.Errorf("command %s has no run function", cmd.name)
		}
	}
}
