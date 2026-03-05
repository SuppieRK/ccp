package filters

import "testing"

func TestGoParentPrepareRoutingAndPassthrough(t *testing.T) {
	f := NewGoToolFilter()
	for _, tc := range []struct {
		name      string
		args      []string
		dispatch  string
		passthru  bool
		ambiguous bool
		reason    string
	}{
		{name: "test-routed", args: []string{"test", "./..."}, dispatch: "go test"},
		{name: "test-json-passthrough", args: []string{"test", "-json", "./..."}, passthru: true, ambiguous: true, reason: "structured output mode"},
		{name: "build-routed", args: []string{"build", "./..."}, dispatch: "go build"},
		{name: "build-trace-routed", args: []string{"build", "-x", "./..."}, dispatch: "go build|x=1"},
		{name: "vet-passthrough", args: []string{"vet", "./..."}, passthru: true},
		{name: "run-passthrough", args: []string{"run", "."}, passthru: true},
		{name: "no-args-passthrough", args: nil, passthru: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if tc.passthru != prep.ForcePassthrough {
				t.Fatalf("args=%#v unexpected passthrough=%v", tc.args, prep.ForcePassthrough)
			}
			if tc.dispatch != prep.DispatchKey {
				t.Fatalf("args=%#v unexpected dispatch=%q", tc.args, prep.DispatchKey)
			}
			if tc.ambiguous != prep.Ambiguous {
				t.Fatalf("args=%#v unexpected ambiguous=%v", tc.args, prep.Ambiguous)
			}
			if tc.reason != prep.Reason {
				t.Fatalf("args=%#v unexpected reason=%q", tc.args, prep.Reason)
			}
		})
	}
}

func TestGoParentPrepareMoveLeadingFlags(t *testing.T) {
	f := NewGoToolFilter()
	cases := []struct {
		name        string
		args        []string
		dispatch    string
		passthrough bool
	}{
		{name: "leading -C test", args: []string{"-C", ".", "test", "./..."}, dispatch: "go test"},
		{name: "leading -C eq build", args: []string{"-C=.", "build", "./..."}, dispatch: "go build"},
		{name: "leading unknown flag", args: []string{"-modfile=x", "test", "./..."}, passthrough: true},
		{name: "leading -C test json", args: []string{"-C", ".", "test", "-json", "./..."}, passthrough: true},
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
