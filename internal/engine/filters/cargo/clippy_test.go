package cargofilters

import (
	"strings"
	"testing"
)

func TestCargoClippyGrouping(t *testing.T) {
	raw := strings.Join([]string{
		"warning: unneeded return statement (clippy::needless_return)",
		"warning: unneeded return statement (clippy::needless_return)",
		"warning: writing `&String` instead of `&str` (clippy::ptr_arg)",
	}, "\n") + "\n"
	out := compactClippyMust(t, raw)
	assertContains(t, out, "cargo clippy: 3 findings across 2 lint rules")
	assertContains(t, out, "clippy::needless_return: 2")
}

func TestCargoClippyNormalizesHyphenatedRuleAndAttachesNotes(t *testing.T) {
	raw := strings.Join([]string{
		"error: length comparison to zero",
		"= note: `-D clippy::len-zero` implied by `-D warnings`",
		"= help: to override `-D warnings` add `#[allow(clippy::len_zero)]`",
	}, "\n") + "\n"
	out := compactClippyMust(t, raw)
	assertContains(t, out, "clippy::len_zero: 1")
	if strings.Contains(out, "clippy::len:") {
		t.Fatalf("expected output to omit %q, got %q", "clippy::len:", out)
	}
}

func TestCargoClippyNoIssueSummary(t *testing.T) {
	raw := strings.Join([]string{
		"Compiling app v0.1.0 (/repo)",
		"Finished dev [unoptimized + debuginfo] target(s) in 0.10s",
	}, "\n") + "\n"
	out := compactClippyMust(t, raw)
	if strings.TrimSpace(out) != "cargo clippy: ok" {
		t.Fatalf("expected no-issue summary, got %q", out)
	}
}

func TestCargoClippyPerRuleExampleCap(t *testing.T) {
	raw := strings.Join([]string{
		"warning: issue 1 (clippy::ptr_arg)",
		"warning: issue 2 (clippy::ptr_arg)",
		"warning: issue 3 (clippy::ptr_arg)",
		"warning: issue 4 (clippy::ptr_arg)",
		"warning: issue 5 (clippy::ptr_arg)",
	}, "\n") + "\n"
	out := compactClippyMust(t, raw)
	assertContains(t, out, "clippy::ptr_arg: 5")
	assertContains(t, out, "... +2 more")
}

func TestCargoClippyLowConfidenceFallbackOnNUL(t *testing.T) {
	if out, ok := compactClippy("warning: bad\x00line (clippy::ptr_arg)\n"); ok || out != "" {
		t.Fatalf("expected low-confidence fallback, got out=%q ok=%v", out, ok)
	}
}

func compactClippyMust(t *testing.T, raw string) string {
	t.Helper()
	out, ok := compactClippy(raw)
	if !ok {
		t.Fatal("expected compact output")
	}
	return out
}

func assertContains(t *testing.T, got, needle string) {
	t.Helper()
	if !strings.Contains(got, needle) {
		t.Fatalf("expected output to contain %q, got %q", needle, got)
	}
}
