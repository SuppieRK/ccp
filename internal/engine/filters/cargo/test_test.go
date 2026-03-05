package cargofilters

import (
	"fmt"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestCompactCargoTestSuccessAndSections(t *testing.T) {
	raw := strings.Join([]string{
		"Running unittests src/lib.rs (target/debug/deps/app-abc)",
		"test result: ok. 3 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out",
		"Running tests/http.rs (target/debug/deps/http-abc)",
		"test result: ok. 2 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out",
		"Doc-tests app",
		"test result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out",
	}, "\n") + "\n"
	out := compactTestMust(t, raw)
	assertCargoTestContains(t, out, "cargo test: ok (6 passed, 0 failed, 0 ignored)")
	for _, want := range []string{
		"- unit: 3 passed, 0 failed, 0 ignored",
		"- integration: 2 passed, 0 failed, 0 ignored",
		"- doc: 1 passed, 0 failed, 0 ignored",
	} {
		assertCargoTestContains(t, out, want)
	}
}

func TestCompactCargoTestFailureRetentionAndDocLine(t *testing.T) {
	raw := strings.Join([]string{
		"Running unittests src/lib.rs (target/debug/deps/app-abc)",
		"test mod::tests::it_fails ... FAILED",
		"thread 'mod::tests::it_fails' panicked at 'assertion failed', src/lib.rs:42:9",
		"Doc-tests app",
		"src/lib.rs - (line 42)",
		"test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out",
	}, "\n") + "\n"
	out := compactTestMust(t, raw)
	assertCargoTestContains(t, out, "cargo test: failed")
	assertCargoTestContains(t, out, "src/lib.rs - (line 42)")
}

func TestCompactCargoTestCollapseFailureAndDropRedundant(t *testing.T) {
	raw := strings.Join([]string{
		"test tests::failing_case ... FAILED",
		"thread 'tests::failing_case' panicked at src/lib.rs:16:9:",
		"assertion `left == right` failed",
		"left: 6",
		"right: 7",
		"failures:",
		"---- tests::failing_case stdout ----",
		"error: test failed, to rerun pass `--lib`",
		"test result: FAILED. 1 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out",
	}, "\n") + "\n"
	out := compactTestMust(t, raw)
	assertCargoTestContains(t, out, "- tests::failing_case (src/lib.rs:16:9): assertion `left == right` failed")
	assertCargoTestNotContains(t, out, "---- tests::failing_case stdout ----")
	assertCargoTestNotContains(t, out, "error: test failed, to rerun pass")
}

func TestCompactCargoTestBoundedFailureOutput(t *testing.T) {
	lines := []string{
		"Running unittests src/lib.rs (target/debug/deps/app-abc)",
		"test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out",
	}
	for i := 0; i < 25; i++ {
		lines = append(lines, fmt.Sprintf("panic: failure detail %02d", i))
	}
	raw := strings.Join(lines, "\n") + "\n"
	out := compactTestMust(t, raw)
	assertCargoTestContains(t, out, "... +5 more")
}

func TestCompactCargoTestFailureDetailsPrioritizeErrorsBeforeWarnings(t *testing.T) {
	raw := strings.Join([]string{
		"Running unittests src/lib.rs (target/debug/deps/app-abc)",
		"warning: this warning should be lower priority",
		"panic: high priority failure",
		"test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out",
	}, "\n") + "\n"
	out := compactTestMust(t, raw)
	panicPos := strings.Index(out, "panic: high priority failure")
	warnPos := strings.Index(out, "warning: this warning should be lower priority")
	if panicPos < 0 || warnPos < 0 || panicPos > warnPos {
		t.Fatalf("expected panic/error detail before warning detail, got %q", out)
	}
}

func TestCompactCargoTestPackageOnlyFailureSuppressed(t *testing.T) {
	raw := "error: test failed, to rerun pass `--lib`\n"
	out := compactTestMust(t, raw)
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty compact output, got %q", out)
	}
}

func TestCargoTestFilterIgnoresPackageOnlyFailureOnEOF(t *testing.T) {
	f := NewTestFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "error: test failed, to rerun pass `--lib`\n"
	_ = mem.Add(raw, raw, 1)

	d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "cargo", Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected EOF ignore for package-only failure suppression, got %#v", d)
	}
}

func TestCompactCargoTestLowConfidenceFallbackOnNUL(t *testing.T) {
	if out, ok := compactTest("test result: ok. 1 passed; 0 failed\x00\n"); ok || out != "" {
		t.Fatalf("expected low-confidence fallback, got out=%q ok=%v", out, ok)
	}
}

func compactTestMust(t *testing.T, raw string) string {
	t.Helper()
	out, ok := compactTest(raw)
	if !ok {
		t.Fatal("expected compact output")
	}
	return out
}

func assertCargoTestContains(t *testing.T, got, needle string) {
	t.Helper()
	if !strings.Contains(got, needle) {
		t.Fatalf("expected output to contain %q, got %q", needle, got)
	}
}

func assertCargoTestNotContains(t *testing.T, got, needle string) {
	t.Helper()
	if strings.Contains(got, needle) {
		t.Fatalf("expected output to omit %q, got %q", needle, got)
	}
}
