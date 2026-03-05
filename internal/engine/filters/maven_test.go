package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	mavenBuildFailureLine    = "[INFO] BUILD FAILURE"
	mavenCompactSuccessError = "expected compact success"
)

func TestMavenFilterMetadataAndPrepare(t *testing.T) {
	f := NewMavenFilter()
	if f.Tool() != "maven" {
		t.Fatalf("expected tool maven, got %q", f.Tool())
	}
	wantAliases := []string{"mvnw", "./mvnw", "mvnw.cmd", "./mvnw.cmd", "mvn.cmd", "./mvn.cmd", "mvn.bat", "./mvn.bat"}
	if !slices.Equal(f.Aliases(), wantAliases) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", wantAliases, f.Aliases())
	}
	prep := f.Prepare([]string{"-T", "2", "test"})
	if prep.DispatchKey != "maven|parallel=1" {
		t.Fatalf("expected parallel dispatch, got %q", prep.DispatchKey)
	}
	if !slices.Equal(prep.NormalizedArgs, []string{"-T", "2", "test"}) {
		t.Fatalf("unexpected normalized args: %#v", prep.NormalizedArgs)
	}

	nonParallel := f.Prepare([]string{"test"})
	if nonParallel.DispatchKey != "maven|parallel=0" {
		t.Fatalf("expected non-parallel dispatch, got %q", nonParallel.DispatchKey)
	}
}

func TestMavenFilterSharedContextKey(t *testing.T) {
	f := NewMavenFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "mvn test", Tool: "maven", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "mvn test", Tool: "maven", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared context key, got %q vs %q", stdout, stderr)
	}
}

func TestClassifyMavenLine(t *testing.T) {
	tests := []struct {
		line string
		want mavenOutputClass
	}{
		{line: "[INFO] --- maven-compiler-plugin:3.14.0:compile (default-compile) @ app ---", want: mavenClassScopeBoundary},
		{line: "Downloading from central: https://repo1.maven.org/x.jar", want: mavenClassProgressNoise},
		{line: "[ERROR] Failed to execute goal", want: mavenClassFailureMarker},
		{line: "Caused by: java.lang.IllegalStateException: boom", want: mavenClassCausedBy},
		{line: "\tat org.example.App.main(App.java:12)", want: mavenClassStacktrace},
		{line: "[INFO] Reactor Summary:", want: mavenClassReactorSummary},
		{line: mavenBuildFailureLine, want: mavenClassFinalStatus},
	}
	for _, tc := range tests {
		got, _ := classifyMavenLine(tc.line)
		if got != tc.want {
			t.Fatalf("line=%q want=%d got=%d", tc.line, tc.want, got)
		}
	}
}

func TestCompactMavenOutputIgnoresTransferProgressOnSuccess(t *testing.T) {
	raw := strings.Join([]string{
		"[INFO] --- maven-resources-plugin:3.3.1:resources (default-resources) @ app ---",
		"Downloading from central: https://repo1.maven.org/artifact.jar",
		"Downloaded from central: https://repo1.maven.org/artifact.jar (10 kB at 20 kB/s)",
		"[INFO] --- maven-compiler-plugin:3.14.0:compile (default-compile) @ app ---",
		"[INFO] BUILD SUCCESS",
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{})
	if !ok {
		t.Fatal(mavenCompactSuccessError)
	}
	if strings.Contains(out, "Downloading from") || strings.Contains(out, "Downloaded from") {
		t.Fatalf("expected transfer progress dropped, got %q", out)
	}
	if !strings.Contains(out, "[ok] app : resources") || !strings.Contains(out, "[ok] app : compile") {
		t.Fatalf("expected collapsed success summaries with module/goal identity, got %q", out)
	}
}

func TestCompactMavenOutputRetainsTransferDiagnosticsOnFailure(t *testing.T) {
	raw := strings.Join([]string{
		"[INFO] --- maven-dependency-plugin:3.8.1:resolve (default-cli) @ app ---",
		"Downloading from central: https://repo1.maven.org/artifact.jar",
		"[ERROR] Could not transfer artifact org.example:bad:jar:1.0.0 from/to central (https://repo1.maven.org)",
		mavenBuildFailureLine,
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{})
	if !ok {
		t.Fatal(mavenCompactSuccessError)
	}
	if !strings.Contains(out, "Could not transfer artifact") {
		t.Fatalf("expected transfer failure retained, got %q", out)
	}
	if !strings.Contains(out, "Downloading from") {
		t.Fatalf("expected transfer context retained on failure, got %q", out)
	}
}

func TestCompactMavenOutputRetainsFailureDiagnostics(t *testing.T) {
	raw := strings.Join([]string{
		"[INFO] --- maven-compiler-plugin:3.14.0:testCompile (default-testCompile) @ app ---",
		"[ERROR] Failed to execute goal org.apache.maven.plugins:maven-compiler-plugin:3.14.0:testCompile (default-testCompile) on project app: Compilation failure",
		"Caused by: java.lang.IllegalArgumentException: bad",
		"\tat org.example.Test.main(Test.java:9)",
		mavenBuildFailureLine,
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{})
	if !ok {
		t.Fatal(mavenCompactSuccessError)
	}
	checks := []string{"[x] app : testCompile", "Failed to execute goal", "Caused by:", "org.example.Test.main", "BUILD FAILURE"}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("expected %q in output, got %q", check, out)
		}
	}
}

func TestCompactMavenOutputReactorSummaryPriority(t *testing.T) {
	raw := strings.Join([]string{
		"[INFO] --- maven-compiler-plugin:3.14.0:compile (default-compile) @ core ---",
		"[INFO] Reactor Summary:",
		"[INFO] core ........................................ SUCCESS [  1.234 s]",
		"[INFO] api ......................................... FAILURE [  0.321 s]",
		mavenBuildFailureLine,
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{})
	if !ok {
		t.Fatal(mavenCompactSuccessError)
	}
	if !strings.Contains(out, "Reactor Summary") || !strings.Contains(out, "api ......................................... FAILURE") {
		t.Fatalf("expected reactor summary retained, got %q", out)
	}
}

func TestCompactMavenOutputParallelLowConfidenceFallback(t *testing.T) {
	raw := strings.Join([]string{
		"[Thread-1] [INFO] Building module-a 1.0.0",
		"[Thread-2] [INFO] Building module-b 1.0.0",
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{parallel: true})
	if ok {
		t.Fatalf("expected low-confidence fallback in parallel mode, got ok=true output=%q", out)
	}
	if out != raw {
		t.Fatalf("expected raw passthrough output, got %q", out)
	}
}

func TestCompactMavenOutputThreadPrefixNormalizedForDedup(t *testing.T) {
	raw := strings.Join([]string{
		"[INFO] --- maven-surefire-plugin:3.2.5:test (default-test) @ app ---",
		"[Thread-1] [ERROR] Failed to execute goal org.apache.maven.plugins:maven-surefire-plugin:3.2.5:test (default-test) on project app: There are test failures.",
		"[Thread-2] [ERROR] Failed to execute goal org.apache.maven.plugins:maven-surefire-plugin:3.2.5:test (default-test) on project app: There are test failures.",
		mavenBuildFailureLine,
	}, "\n") + "\n"

	out, ok := compactMavenOutput(raw, mavenDispatch{parallel: true})
	if !ok {
		t.Fatal(mavenCompactSuccessError)
	}
	if strings.Count(out, "Failed to execute goal") != 1 {
		t.Fatalf("expected deduped failure line across thread prefixes, got %q", out)
	}
}

func TestMavenProcessCollectsUntilExit(t *testing.T) {
	f := NewMavenFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Tool: "maven", Stream: engine.StdoutStream, Line: "[INFO] line\n"},
		{Type: engine.EventTick, Tool: "maven", Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Tool: "maven", Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestMavenProcessExitCases(t *testing.T) {
	f := NewMavenFilter()
	raw1 := "[Thread-1] [INFO] Building module-a 1.0.0\n"
	raw2 := "[Thread-2] [INFO] Building module-b 1.0.0\n"
	cases := []struct {
		name       string
		dispatch   string
		lines      []string
		wantAction engine.Action
		wantOutput string
	}{
		{
			name:       "empty-buffer-ignores",
			dispatch:   "",
			lines:      nil,
			wantAction: engine.ActionIgnore,
			wantOutput: "",
		},
		{
			name:       "low-confidence-fallback-flushes-raw",
			dispatch:   "maven|parallel=1",
			lines:      []string{raw1, raw2},
			wantAction: engine.ActionFlush,
			wantOutput: raw1 + raw2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			d := f.Process(engine.Event{
				Type:     engine.EventExit,
				Tool:     "maven",
				Dispatch: tc.dispatch,
				Stream:   engine.StdoutStream,
			}, mem)
			if d.Action != tc.wantAction || d.Output != tc.wantOutput {
				t.Fatalf("unexpected decision: got %#v", d)
			}
		})
	}
}
