package filters

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	gradleFailureHeader   = "FAILURE: Build failed with an exception."
	gradleHelpPointerLine = "* Get more help at https://help.gradle.org"
	gradleCompactSuccess  = "expected compact success"
)

func TestCanonicalGradleLineStripsCarriageReturn(t *testing.T) {
	got := canonicalGradleLine("Download https://example.com\r\n")
	if got != "Download https://example.com" {
		t.Fatalf("unexpected canonical line: %q", got)
	}
}

func TestClassifyGradleLineClasses(t *testing.T) {
	tests := []struct {
		name string
		line string
		want gradleOutputClass
	}{
		{name: "task", line: "> Task :app:test", want: gradleClassTaskBoundary},
		{name: "progress", line: "Download https://repo (50%)", want: gradleClassProgressNoise},
		{name: "failure", line: gradleFailureHeader, want: gradleClassFailureMarker},
		{name: "caused", line: "Caused by: java.lang.RuntimeException: boom", want: gradleClassCausedBy},
		{name: "help", line: gradleHelpPointerLine, want: gradleClassHelpPointer},
		{name: "stack", line: "\tat org.example.Main.main(Main.java:42)", want: gradleClassStacktrace},
		{name: "success", line: "BUILD SUCCESSFUL in 2s", want: gradleClassSuccessSummary},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := classifyGradleLine(tc.line)
			if got != tc.want {
				t.Fatalf("line %q: want class %d, got %d", tc.line, tc.want, got)
			}
		})
	}
}

func TestGradleFilterMetadataAndPrepare(t *testing.T) {
	f := NewGradleFilter()
	if f.Tool() != "gradle" {
		t.Fatalf("expected tool gradle, got %q", f.Tool())
	}
	aliases := f.Aliases()
	wantAliases := []string{"gradlew", "./gradlew", "gradlew.bat", "./gradlew.bat"}
	if !slices.Equal(aliases, wantAliases) {
		t.Fatalf("unexpected aliases: want %#v, got %#v", wantAliases, aliases)
	}
	prep := f.Prepare([]string{"build", "--stacktrace"})
	wantArgs := []string{"build", "--stacktrace"}
	if !slices.Equal(prep.NormalizedArgs, wantArgs) {
		t.Fatalf("unexpected normalized args: want %#v, got %#v", wantArgs, prep.NormalizedArgs)
	}
}

func TestGradleFilterUsesSharedContextAcrossStreams(t *testing.T) {
	f := NewGradleFilter()
	stdoutKey := f.ContextKey(engine.Event{
		CommandID: "gradle test",
		Tool:      "gradle",
		Stream:    engine.StdoutStream,
	})
	stderrKey := f.ContextKey(engine.Event{
		CommandID: "gradle test",
		Tool:      "gradle",
		Stream:    engine.StderrStream,
	})
	if stdoutKey != stderrKey {
		t.Fatalf("expected shared context key for stdout/stderr, got %q != %q", stdoutKey, stderrKey)
	}
}

func TestCompactGradleOutputRetainsFailureContext(t *testing.T) {
	raw := strings.Join([]string{
		"> Task :test FAILED",
		gradleFailureHeader,
		"* What went wrong:",
		"Execution failed for task ':test'.",
		"Caused by: java.lang.AssertionError: expected true",
		"\tat org.example.Test.test(Test.java:12)",
		gradleHelpPointerLine,
	}, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if !ok {
		t.Fatal(gradleCompactSuccess)
	}
	checks := []string{
		gradleFailureHeader,
		"Caused by:",
		"help.gradle.org",
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Fatalf("expected failure context %q in output: %q", c, out)
		}
	}
}

func TestCompactGradleOutputRetainsFailureLocationBeforeMarker(t *testing.T) {
	raw := strings.Join([]string{
		"> Task :compileJava FAILED",
		"/repo/src/App.java:160: error: incompatible types: Duration cannot be converted to boolean",
		"  if (timeToLive) {",
		"      ^",
		"1 error",
		gradleFailureHeader,
		"* What went wrong:",
		"Execution failed for task ':compileJava'.",
	}, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if !ok {
		t.Fatal(gradleCompactSuccess)
	}
	if !strings.Contains(out, "/repo/src/App.java:160: error: incompatible types") {
		t.Fatalf("expected compiler failure location retained, got %q", out)
	}
	if !strings.Contains(out, "if (timeToLive)") || !strings.Contains(out, "^") || !strings.Contains(out, "1 error") {
		t.Fatalf("expected compiler context lines retained, got %q", out)
	}
}

func TestCompactGradleOutputSuppressesLowSignalFrameworkStacks(t *testing.T) {
	raw := strings.Join([]string{
		"> Task :spotlessJavaApply FAILED",
		gradleFailureHeader,
		"Caused by: java.lang.IllegalStateException: bad formatting",
		"\tat org.gradle.internal.work.DefaultWorkerLeaseService.withLocks(DefaultWorkerLeaseService.java:263)",
		"\tat java.base/jdk.internal.reflect.DirectMethodHandleAccessor.invoke(DirectMethodHandleAccessor.java:103)",
		"\tat org.example.App.main(App.java:42)",
		"12:15:15.921 [SpringApplicationShutdownHook] INFO  com.zaxxer.hikari.HikariDataSource - HikariPool-1 - Shutdown initiated...",
		"12:15:15.922 [SpringApplicationShutdownHook] INFO  com.zaxxer.hikari.HikariDataSource - HikariPool-1 - Shutdown completed.",
		gradleHelpPointerLine,
	}, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if !ok {
		t.Fatal(gradleCompactSuccess)
	}
	if strings.Contains(out, "at org.gradle.") {
		t.Fatalf("expected org.gradle stack frames suppressed, got %q", out)
	}
	if strings.Contains(out, "at java.base") {
		t.Fatalf("expected java.base stack frames suppressed, got %q", out)
	}
	if strings.Contains(out, "Shutdown initiated") || strings.Contains(out, "Shutdown completed") {
		t.Fatalf("expected shutdown hook noise suppressed, got %q", out)
	}
	if !strings.Contains(out, "at org.example.App.main") {
		t.Fatalf("expected user stack frame retained, got %q", out)
	}
}

func TestCompactGradleOutputFallsBackOnLowConfidenceInterleaving(t *testing.T) {
	raw := strings.Join([]string{
		"> Task :app:test",
		":lib:test FAILED",
		gradleFailureHeader,
	}, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if ok {
		t.Fatalf("expected fallback for low-confidence interleaving, got ok=true output=%q", out)
	}
	if out != raw {
		t.Fatalf("expected raw passthrough on fallback, got %q", out)
	}
}

func TestCompactGradleOutputCollapsesSuccessfulTaskDetails(t *testing.T) {
	raw := strings.Join([]string{
		"> Task :compileJava",
		"note: compilation detail that should collapse",
		"> Task :test",
		"BUILD SUCCESSFUL in 1s",
	}, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if !ok {
		t.Fatal(gradleCompactSuccess)
	}
	if !strings.Contains(out, "[ok] :compileJava") {
		t.Fatalf("expected successful task summary, got %q", out)
	}
	if strings.Contains(out, "compilation detail") {
		t.Fatalf("expected successful task details collapsed, got %q", out)
	}
}

func TestGradleCompactionThresholdOnNoisyFixture(t *testing.T) {
	var lines []string
	lines = append(lines, "> Task :compileJava")
	for i := 0; i < 30; i++ {
		lines = append(lines, "Download https://repo.example/artifact.jar ("+strconv.Itoa(i)+"%)")
	}
	lines = append(lines, "> Task :test")
	lines = append(lines, "BUILD SUCCESSFUL in 2s")
	raw := strings.Join(lines, "\n") + "\n"

	out, ok := compactGradleOutput(raw)
	if !ok {
		t.Fatal("expected successful compaction")
	}

	rawLines := countNonEmptyLines(raw)
	outLines := countNonEmptyLines(out)
	if rawLines == 0 {
		t.Fatal("raw fixture unexpectedly empty")
	}
	dropRatio := float64(rawLines-outLines) / float64(rawLines)
	if dropRatio < 0.15 {
		t.Fatalf("expected drop ratio >= 0.15, got %.2f (raw=%d out=%d)", dropRatio, rawLines, outLines)
	}
}

func TestGradleProcessCollectsUntilExit(t *testing.T) {
	f := NewGradleFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Tool: "gradle", Stream: engine.StdoutStream, Line: "> Task :test\n"},
		{Type: engine.EventTick, Tool: "gradle", Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Tool: "gradle", Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestGradleProcessExitWithEmptyBufferIgnores(t *testing.T) {
	f := NewGradleFilter()
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "gradle", Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore for empty exit buffer, got %#v", d)
	}
}

func TestGradleProcessExitNonEmptyBufferCases(t *testing.T) {
	f := NewGradleFilter()
	cases := []struct {
		name         string
		raw          string
		wantRaw      bool
		mustContain  []string
		mustNotMatch []string
	}{
		{
			name: "compacts-success-output",
			raw: strings.Join([]string{
				"> Task :compileJava",
				"Download https://repo.example/a.jar (10%)",
				"> Task :test",
				"BUILD SUCCESSFUL in 1s",
			}, "\n") + "\n",
			mustContain:  []string{"[ok] :compileJava", "BUILD SUCCESSFUL"},
			mustNotMatch: []string{"Download https://repo.example/a.jar"},
		},
		{
			name: "fallback-raw-on-low-confidence",
			raw: strings.Join([]string{
				"> Task :app:test",
				":lib:test FAILED",
				gradleFailureHeader,
			}, "\n") + "\n",
			wantRaw: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(tc.raw, tc.raw, 1)
			d := f.Process(engine.Event{Type: engine.EventExit, Tool: "gradle", Stream: engine.StdoutStream}, mem)
			if d.Action != engine.ActionFlush {
				t.Fatalf("expected flush for non-empty exit buffer, got %#v", d)
			}
			if tc.wantRaw {
				if d.Output != tc.raw {
					t.Fatalf("expected raw fallback output, got %q", d.Output)
				}
				return
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got %q", want, d.Output)
				}
			}
			for _, forbidden := range tc.mustNotMatch {
				if strings.Contains(d.Output, forbidden) {
					t.Fatalf("expected output to omit %q, got %q", forbidden, d.Output)
				}
			}
		})
	}
}

func countNonEmptyLines(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
