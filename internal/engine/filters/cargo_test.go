package filters

import "testing"

func TestCargoParentPrepareRoutingAndPassthrough(t *testing.T) {
	f := NewCargoToolFilter()
	for _, tc := range []struct {
		args      []string
		dispatch  string
		passthru  bool
		ambiguous bool
	}{
		{args: []string{"test", "./..."}, dispatch: "cargo test"},
		{args: []string{"build"}, dispatch: "cargo build"},
		{args: []string{"check"}, dispatch: "cargo check"},
		{args: []string{"clippy"}, dispatch: "cargo clippy"},
		{args: []string{"build", "--message-format=json"}, passthru: true, ambiguous: true},
		{args: []string{"run", "--", "x"}, passthru: true, ambiguous: true},
		{args: []string{"fmt"}, passthru: true},
	} {
		prep := f.Prepare(tc.args)
		if prep.DispatchKey != tc.dispatch {
			t.Fatalf("args=%#v dispatch=%q want=%q", tc.args, prep.DispatchKey, tc.dispatch)
		}
		if prep.ForcePassthrough != tc.passthru {
			t.Fatalf("args=%#v passthrough=%v want=%v", tc.args, prep.ForcePassthrough, tc.passthru)
		}
		if prep.Ambiguous != tc.ambiguous {
			t.Fatalf("args=%#v ambiguous=%v want=%v", tc.args, prep.Ambiguous, tc.ambiguous)
		}
	}
}

func TestCargoParentPrepareMoveLeadingFlags(t *testing.T) {
	f := NewCargoToolFilter()
	cases := []struct {
		name        string
		args        []string
		dispatch    string
		passthrough bool
	}{
		{name: "leading toolchain test", args: []string{"+stable", "test"}, dispatch: "cargo test"},
		{name: "leading toolchain and config", args: []string{"+nightly", "--config", "profile.dev.debug=false", "build"}, dispatch: "cargo build"},
		{name: "leading color check", args: []string{"--color", "always", "check"}, dispatch: "cargo check"},
		{name: "leading -Z clippy", args: []string{"-Z", "unstable-options", "clippy"}, dispatch: "cargo clippy"},
		{name: "unknown leading passthrough", args: []string{"--unknown-global", "build"}, passthrough: true},
		{name: "leading toolchain run passthru", args: []string{"+stable", "run"}, passthrough: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if tc.passthrough {
				if !prep.ForcePassthrough {
					t.Fatalf("expected passthrough for args=%#v got %#v", tc.args, prep)
				}
				return
			}
			if prep.ForcePassthrough {
				t.Fatalf("expected dispatch for args=%#v got %#v", tc.args, prep)
			}
			if prep.DispatchKey != tc.dispatch {
				t.Fatalf("dispatch mismatch: want %q got %q", tc.dispatch, prep.DispatchKey)
			}
		})
	}
}
