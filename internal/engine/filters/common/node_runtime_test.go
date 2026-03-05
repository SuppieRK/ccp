package common

import "testing"

func TestNodeNormalizeWarningKey(t *testing.T) {
	t.Parallel()

	in := " (node:12345) ExperimentalWarning: X "
	got := NodeNormalizeWarningKey(in)
	want := "(node:pid) experimentalwarning: x"
	if got != want {
		t.Fatalf("NodeNormalizeWarningKey() = %q, want %q", got, want)
	}
}

func TestNodeWarningAndFailureClassification(t *testing.T) {
	t.Parallel()

	warningCases := []struct {
		line string
		want bool
	}{
		{line: "(node:123) ExperimentalWarning: foo", want: true},
		{line: "to load an es module, set \"type\": \"module\"", want: true},
		{line: "plain output line", want: false},
	}

	for _, tc := range warningCases {
		if got := NodeIsRuntimeWarning(tc.line); got != tc.want {
			t.Fatalf("NodeIsRuntimeWarning(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}

	failureCases := []struct {
		line string
		want bool
	}{
		{line: "Unhandled rejection: boom", want: true},
		{line: "handled successfully", want: false},
	}

	for _, tc := range failureCases {
		if got := NodeIsUnhandledFailure(tc.line); got != tc.want {
			t.Fatalf("NodeIsUnhandledFailure(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

func TestNodeProgressAndCanonicalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		line string
		last string
		want bool
	}{
		{line: "step\rnext", last: "step", want: true},
		{line: "x", last: "x \x1b[32m", want: true},
		{line: "x", last: "⠋ loading", want: true},
		{line: "line", last: "line", want: false},
	}
	for _, tc := range cases {
		if got := NodeIsProgressNoise(tc.line, tc.last); got != tc.want {
			t.Fatalf("NodeIsProgressNoise(%q,%q) = %v, want %v", tc.line, tc.last, got, tc.want)
		}
	}

	if got := NodeCanonicalLine("a\r\n"); got != "a" {
		t.Fatalf("NodeCanonicalLine() = %q, want a", got)
	}
}

func TestNodeInteractiveInvocation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "nil-args", args: nil, want: true},
		{name: "interactive-flag", args: []string{"-i"}, want: true},
		{name: "script-file", args: []string{"script.js"}, want: false},
		{name: "bare-separator", args: []string{"--"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NodeIsInteractiveInvocation(tc.args); got != tc.want {
				t.Fatalf("NodeIsInteractiveInvocation(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestNodeLowConfidenceOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{in: "a\x00b", want: true},
		{in: string([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), want: true},
		{in: "normal\ntext\n", want: false},
	}
	for _, tc := range cases {
		if got := NodeLowConfidenceOutput(tc.in); got != tc.want {
			t.Fatalf("NodeLowConfidenceOutput(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
