package common

import "testing"

const (
	flagFormat = "--format"
	flagMode   = "--mode"
	flagOutput = "--output"
)

func TestCopyArgsProducesIndependentSlice(t *testing.T) {
	t.Parallel()

	in := []string{"a", "b"}
	out := CopyArgs(in)
	if len(out) != 2 || out[0] != "a" || out[1] != "b" {
		t.Fatalf("unexpected copy: %#v", out)
	}
	out[0] = "z"
	if in[0] != "a" {
		t.Fatalf("copy should not mutate input, got input=%#v", in)
	}
}

func TestHasExactFlag(t *testing.T) {
	t.Parallel()

	if !HasExactFlag([]string{" --RAW ", "x"}, "--raw") {
		t.Fatal("expected case/space-insensitive exact flag match")
	}
	if HasExactFlag([]string{"--raw=true"}, "--raw") {
		t.Fatal("did not expect exact flag match for value form")
	}
}

func TestHasAnyFlag(t *testing.T) {
	t.Parallel()

	args := []string{"-f", "--other"}
	if !HasAnyFlag(args, "--follow", "-f") {
		t.Fatalf("expected one-of flag match in %v", args)
	}
	if HasAnyFlag(args, "--missing") {
		t.Fatal("did not expect missing flag to match")
	}
}

func TestHasOptionAndValue(t *testing.T) {
	t.Parallel()

	args := []string{flagFormat, "json", flagMode + "=fast"}
	if !HasOption(args, flagFormat) || !HasOption(args, flagMode) {
		t.Fatalf("expected options to be found in %v", args)
	}
	if v, ok := OptionValue(args, flagFormat); !ok || v != "json" {
		t.Fatalf("OptionValue(--format) = (%q,%v), want (json,true)", v, ok)
	}
	if v, ok := OptionValue(args, flagMode); !ok || v != "fast" {
		t.Fatalf("OptionValue(--mode) = (%q,%v), want (fast,true)", v, ok)
	}
	if _, ok := OptionValue(args, "--missing"); ok {
		t.Fatal("did not expect missing option value")
	}
}

func TestHasOptionAnyAndOptionValueAny(t *testing.T) {
	t.Parallel()

	args := []string{"-o=jsonpath={.items[*]}", flagMode, "fast"}
	if !HasOptionAny(args, "-o", flagOutput) {
		t.Fatalf("expected short option match in %v", args)
	}
	if v, ok := OptionValueAny(args, "-o", flagOutput); !ok || v != "jsonpath={.items[*]}" {
		t.Fatalf("OptionValueAny(-o/--output) = (%q,%v)", v, ok)
	}
	if v, ok := OptionValueAny(args, flagMode); !ok || v != "fast" {
		t.Fatalf("OptionValueAny(--mode) = (%q,%v)", v, ok)
	}
}

func TestHasOptionValue(t *testing.T) {
	t.Parallel()

	args := []string{flagOutput, "WIDE"}
	if !HasOptionValue(args, flagOutput, "wide") {
		t.Fatalf("expected option value match in %v", args)
	}
	if HasOptionValue(args, flagOutput, "json") {
		t.Fatal("did not expect mismatched option value to match")
	}
}

func TestParsePositiveIntOptionAny(t *testing.T) {
	t.Parallel()

	args := []string{"--ignored=0", "--max-results=25", "-m", "9"}
	if got := ParsePositiveIntOptionAny(args, 50, "-m", "--max-results", "--max_results", "--max-count"); got != 25 {
		t.Fatalf("ParsePositiveIntOptionAny = %d, want 25", got)
	}
	if got := ParsePositiveIntOptionAny([]string{"-m", "bad"}, 7, "-m"); got != 7 {
		t.Fatalf("fallback = %d, want 7", got)
	}
}
