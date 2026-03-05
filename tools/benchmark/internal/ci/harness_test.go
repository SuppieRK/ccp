package ci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	scenariosFileName    = "scenarios.json"
	inputFileName        = "in.txt"
	runErrFmt            = "run: %v"
	projectAName         = "project-a"
	projectBName         = "project-b"
	sameLine             = "same\n"
	expectedOneResultFmt = "expected 1 result, got %d"
	lsEmptyScenarioName  = "ls-empty"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func catArgs(file string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", "type", file}
	}
	return []string{"cat", file}
}

func writeScenarios(t *testing.T, path string, scenarios []Scenario) {
	t.Helper()
	b, err := json.MarshalIndent(scenarios, "", "  ")
	if err != nil {
		t.Fatalf("marshal scenarios: %v", err)
	}
	writeFile(t, path, string(b))
}

func TestLoadScenarioValidationAndIgnoreLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, scenariosFileName)
	writeScenarios(t, path, []Scenario{
		{
			Name:        "test",
			Tool:        "ls",
			Project:     "project",
			Native:      []string{"echo", "ok"},
			ExpectExit:  0,
			IgnoreLines: []string{`^ts=[0-9]+$`},
		},
	})

	scenarios, err := loadScenarios(path)
	if err != nil {
		t.Fatalf("loadScenarios: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("scenarios length = %d", len(scenarios))
	}
	s := scenarios[0]
	if got, want := s.Name, "test"; got != want {
		t.Fatalf("name = %q want %q", got, want)
	}
	if len(s.IgnoreLines) != 1 {
		t.Fatalf("ignore_lines length = %d", len(s.IgnoreLines))
	}
}

func TestLoadScenariosRejectsInvalidIgnoreLinesRegex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, scenariosFileName)
	writeScenarios(t, path, []Scenario{
		{
			Name:        "test",
			Tool:        "ls",
			Project:     "project",
			Native:      []string{"echo", "ok"},
			ExpectExit:  0,
			IgnoreLines: []string{"["},
		},
	})

	_, err := loadScenarios(path)
	if err == nil || !strings.Contains(err.Error(), "invalid ignore_lines regex") {
		t.Fatalf("expected invalid ignore_lines regex error, got %v", err)
	}
}

func TestLoadScenariosValidatesLifecycleCommands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, scenariosFileName)
	writeScenarios(t, path, []Scenario{
		{
			Name:        "test",
			Tool:        "ls",
			Native:      []string{"echo", "ok"},
			BeforeStart: [][]string{{"echo", "pre"}, {}},
			ExpectExit:  0,
		},
	})
	_, err := loadScenarios(path)
	if err == nil || !strings.Contains(err.Error(), "before_start[1] command is required") {
		t.Fatalf("expected lifecycle validation error, got %v", err)
	}
}

func TestDiscoverScenariosRequiresDefinitionForProjectDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls"), 0o755); err != nil {
		t.Fatal(err)
	}
	scenarios, err := discoverScenarios(root)
	if err != nil {
		t.Fatalf("discoverScenarios unexpected error: %v", err)
	}
	if len(scenarios) != 0 {
		t.Fatalf("expected no scenarios, got %d", len(scenarios))
	}
}

func TestDiscoverScenariosRequiresProjectDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{
			Name:       "ls-run",
			Tool:       "ls",
			Project:    "missing-project",
			Native:     []string{"echo", "ok"},
			ExpectExit: 0,
		},
	})
	_, err := discoverScenarios(root)
	if err == nil || !strings.Contains(err.Error(), "project directory not found") {
		t.Fatalf("expected project directory error, got %v", err)
	}
}

func TestDiscoverScenariosAllowsMissingProjectForTextOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "maven"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(root, "maven", scenariosFileName), []Scenario{
		{
			Name:       "maven-successful-build",
			Tool:       "maven",
			Project:    "project/example",
			TextOnly:   true,
			Native:     []string{"mvn", "test"},
			ExpectExit: 0,
		},
	})
	scenarios, err := discoverScenarios(root)
	if err != nil {
		t.Fatalf("discoverScenarios unexpected error: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
}

func TestDiscoverScenariosSupportsGlobFixturesRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "group", "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(root, "group", "ls", scenariosFileName), []Scenario{
		{
			Name:       "ls-run",
			Tool:       "ls",
			Project:    "project",
			Native:     []string{"echo", "ok"},
			ExpectExit: 0,
		},
	})

	pattern := filepath.Join(root, "group", "*")
	scenarios, err := discoverScenarios(pattern)
	if err != nil {
		t.Fatalf("discoverScenarios with glob root: %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}
	if scenarios[0].FixtureKey != "ls" {
		t.Fatalf("fixture key = %q, want ls", scenarios[0].FixtureKey)
	}
}

func TestExpandFixtureRootsSupportsGlobStar(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}

	pattern := filepath.Join(root, "**")
	roots, err := expandFixtureRoots(pattern)
	if err != nil {
		t.Fatalf("expandFixtureRoots: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected at least one expanded root")
	}
	foundA := false
	foundB := false
	for _, r := range roots {
		if r == a {
			foundA = true
		}
		if r == b {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("expected globstar roots to include %q and %q, got %v", a, b, roots)
	}
}

func TestLoadScenariosSkipsEmptyObjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, scenariosFileName)
	writeScenarios(t, path, []Scenario{
		{},
		{Name: "one", Tool: "ls", Project: "project", Native: []string{"echo", "ok"}, ExpectExit: 0},
		{},
		{Name: "two", Tool: "ls", Project: "project", Native: []string{"echo", "ok"}, ExpectExit: 0},
	})
	scenarios, err := loadScenarios(path)
	if err != nil {
		t.Fatalf("loadScenarios: %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
}

func TestDiscoverScenariosRejectsDuplicateNamesInSameFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "dup", Tool: "ls", Project: "project", Native: []string{"echo", "ok"}, ExpectExit: 0},
		{Name: "dup", Tool: "ls", Project: "project", Native: []string{"echo", "ok"}, ExpectExit: 0},
	})
	_, err := discoverScenarios(root)
	if err == nil || !strings.Contains(err.Error(), "duplicate scenario name") {
		t.Fatalf("expected duplicate scenario error, got %v", err)
	}
}

func TestRunBootstrapAndSummaryWarning(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), "a\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "ls", Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})

	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results = %d", len(report.Results))
	}
	if report.Results[0].FixtureKey != "ls" {
		t.Fatalf("fixture key = %q", report.Results[0].FixtureKey)
	}
	if report.Results[0].Pwd == "" || strings.HasPrefix(report.Results[0].Pwd, "/") {
		t.Fatalf("expected relative pwd, got %q", report.Results[0].Pwd)
	}
	if report.Results[0].Native.ExitCode != 0 || report.Results[0].Proxy.ExitCode != 0 {
		t.Fatalf("expected zero exit codes, got native=%d proxy=%d", report.Results[0].Native.ExitCode, report.Results[0].Proxy.ExitCode)
	}
	if report.Results[0].Native.DurationMs < 0 || report.Results[0].Proxy.DurationMs < 0 {
		t.Fatalf("expected non-negative durations, got native=%d proxy=%d", report.Results[0].Native.DurationMs, report.Results[0].Proxy.DurationMs)
	}
}

func TestRunAppliesArtifactTruncation(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 4096)
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), big)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "ls", Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], MaxArtifactBytes: 64, Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results = %d", len(report.Results))
	}
	joined := strings.Join(report.Results[0].Warnings, " ")
	if !strings.Contains(joined, "truncation") {
		t.Fatalf("expected truncation warning, got %q", joined)
	}
}

func TestRunFailsRequiredScenarioOnSafetyViolation(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), "hello\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{
			Name:       "ls",
			Tool:       "ls",
			Project:    "project",
			Native:     native,
			ExpectExit: 0,
			MustContain: []string{
				"MISSING",
			},
			Required: true,
		},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if !report.FailedRequired {
		t.Fatal("expected failed required scenario")
	}
	if !report.FailedSafety {
		t.Fatal("expected failed safety scenario")
	}
	if len(report.Results) != 1 || report.Results[0].Success {
		t.Fatal("expected failing scenario result")
	}
	r := report.Results[0]
	if r.Proxy.Artifact == "" {
		t.Fatal("missing proxy artifact path")
	}
	if _, err := os.Stat(r.Proxy.Artifact); err != nil {
		t.Fatalf("proxy artifact missing: %v", err)
	}
	if filepath.Base(r.Proxy.Artifact) != benchmarkOutputFileName {
		t.Fatalf("proxy artifact name = %q", filepath.Base(r.Proxy.Artifact))
	}
	if filepath.Base(filepath.Dir(r.Proxy.Artifact)) != "ls" {
		t.Fatalf("proxy artifact scenario dir = %q", filepath.Base(filepath.Dir(r.Proxy.Artifact)))
	}
	if filepath.Base(filepath.Dir(filepath.Dir(r.Proxy.Artifact))) != "ls" {
		t.Fatalf("proxy artifact tool dir = %q", filepath.Base(filepath.Dir(filepath.Dir(r.Proxy.Artifact))))
	}
}

func TestRunFlagsSafetyFailureEvenWhenScenarioNotRequired(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), "hello\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{
			Name:        "ls-non-required",
			Tool:        "ls",
			Project:     "project",
			Native:      native,
			ExpectExit:  0,
			MustContain: []string{"MISSING"},
			Required:    false,
		},
	})

	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if report.FailedRequired {
		t.Fatal("did not expect failed required scenario")
	}
	if !report.FailedSafety {
		t.Fatal("expected failed safety scenario")
	}
	if len(report.Results) != 1 || report.Results[0].SafetyPassed {
		t.Fatal("expected non-required scenario safety failure")
	}
}

func TestRunExecutesAllScenariosInScenariosFile(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tool", projectAName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tool", projectBName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "tool", projectAName, inputFileName), "A\n")
	writeFile(t, filepath.Join(root, "tool", projectBName, inputFileName), "B\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "tool", scenariosFileName), []Scenario{
		{Name: "tool-a", Tool: "tool", Project: projectAName, Native: native, ExpectExit: 0, Required: true},
		{Name: "tool-b", Tool: "tool", Project: projectBName, Native: native, ExpectExit: 0, Required: false},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		artifactPath := filepath.ToSlash(r.Proxy.Artifact)
		expected := "/" + r.Tool + "/" + r.Scenario + "/"
		if !strings.Contains(artifactPath, expected) {
			t.Fatalf("expected artifact path to include tool/scenario for %s, got %s", r.Scenario, r.Proxy.Artifact)
		}
	}
}

func TestRunSeparatesArtifactsByScenarioAndProject(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tool", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "tool", "project", inputFileName), sameLine)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "tool", scenariosFileName), []Scenario{
		{Name: "tool-a", Tool: "tool", Project: "project", Native: native, ExpectExit: 0, Required: true},
		{Name: "tool-b", Tool: "tool", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	if report.Results[0].Proxy.Artifact == report.Results[1].Proxy.Artifact {
		t.Fatalf("proxy artifacts collided: %s", report.Results[0].Proxy.Artifact)
	}
	for _, r := range report.Results {
		p := filepath.ToSlash(r.Proxy.Artifact)
		if !strings.Contains(p, "/tool/"+r.Scenario+"/") {
			t.Fatalf("artifact path does not include scenario: %s", r.Proxy.Artifact)
		}
	}
}

func TestRunUsesScenarioNameForArtifactPath(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tool", "projects", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "tool", "projects", "nested", inputFileName), sameLine)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "tool", scenariosFileName), []Scenario{
		{Name: "tool-nested", Tool: "tool", Project: "projects/nested", Native: native, ExpectExit: 0, Required: true},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	p := filepath.ToSlash(report.Results[0].Proxy.Artifact)
	if !strings.Contains(p, "/tool/tool-nested/") {
		t.Fatalf("artifact path does not use scenario folder name: %s", report.Results[0].Proxy.Artifact)
	}
	if strings.Contains(p, "/nested/") || strings.Contains(p, "/projects-nested/") {
		t.Fatalf("artifact path unexpectedly includes project path segments: %s", report.Results[0].Proxy.Artifact)
	}
}

func TestRunAllowsEmptyProjectRoot(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "tool", inputFileName), "root\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "tool", scenariosFileName), []Scenario{
		{Name: "tool-root", Tool: "tool", Native: native, ExpectExit: 0, Required: true},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	if r.Pwd != "tool" {
		t.Fatalf("expected root tool pwd, got %q", r.Pwd)
	}
	p := filepath.ToSlash(r.Proxy.Artifact)
	if !strings.Contains(p, "/tool/tool-root/") {
		t.Fatalf("artifact path does not include tool/scenario: %s", r.Proxy.Artifact)
	}
}

func TestRunWarnsOnCompactionDropWhenInputHashMatches(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	prevPath := filepath.Join(t.TempDir(), "previous-report.json")
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), sameLine)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "ls", Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})

	first, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf("run first: %v", err)
	}
	if len(first.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(first.Results))
	}
	prev := RunReport{
		Generated: first.Generated,
		Tokenizer: first.Tokenizer,
		Results: []ScenarioResult{
			{
				Scenario:             "ls",
				Tool:                 "ls",
				RawInputHash:         first.Results[0].RawInputHash,
				TokenCompactionRatio: 2.0,
			},
		},
	}
	b, err := json.Marshal(prev)
	if err != nil {
		t.Fatalf("marshal previous report: %v", err)
	}
	if err := os.WriteFile(prevPath, b, 0o644); err != nil {
		t.Fatalf("write previous report: %v", err)
	}

	second, err := Run(RunOptions{
		FixturesRoot:   root,
		ArtifactsDir:   artifacts,
		ProxyBinary:    native[0],
		PreviousReport: prevPath,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("run second: %v", err)
	}
	if len(second.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(second.Results))
	}
	if !strings.Contains(strings.Join(second.Results[0].Warnings, " "), "token compaction ratio dropped") {
		t.Fatalf("expected compaction drop warning, got %q", strings.Join(second.Results[0].Warnings, "; "))
	}
}

func TestRunSkipsCompactionDropWarningWhenInputHashDiffers(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	prevPath := filepath.Join(t.TempDir(), "previous-report.json")
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), sameLine)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "ls", Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})

	prev := RunReport{
		Generated: time.Now().UTC(),
		Tokenizer: "test",
		Results: []ScenarioResult{
			{
				Scenario:             "ls",
				Tool:                 "ls",
				RawInputHash:         "different-hash",
				TokenCompactionRatio: 2.0,
			},
		},
	}
	b, err := json.Marshal(prev)
	if err != nil {
		t.Fatalf("marshal previous report: %v", err)
	}
	if err := os.WriteFile(prevPath, b, 0o644); err != nil {
		t.Fatalf("write previous report: %v", err)
	}

	report, err := Run(RunOptions{
		FixturesRoot:   root,
		ArtifactsDir:   artifacts,
		ProxyBinary:    native[0],
		PreviousReport: prevPath,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	if strings.Contains(strings.Join(report.Results[0].Warnings, " "), "token compaction ratio dropped") {
		t.Fatalf("did not expect compaction drop warning for hash mismatch, got %q", strings.Join(report.Results[0].Warnings, "; "))
	}
}

func TestRunExecutesAfterStopOnBeforeStartFailure(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), sameLine)
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{
			Name:        "ls",
			Tool:        "ls",
			Project:     "project",
			Native:      native,
			ExpectExit:  0,
			BeforeStart: [][]string{{"__definitely_missing_before_start_cmd__"}},
			AfterStop:   [][]string{{"__definitely_missing_after_stop_cmd__"}},
			Required:    true,
		},
	})
	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	if r.Success {
		t.Fatal("expected scenario failure")
	}
	w := strings.Join(r.Warnings, "\n")
	if !strings.Contains(w, "before_start hook failed") {
		t.Fatalf("expected before_start hook warning, got %q", w)
	}
	if !strings.Contains(w, "after_stop hook failed") {
		t.Fatalf("expected after_stop hook warning, got %q", w)
	}
}

func TestLoadSeverityThresholdsDefaultAndOverride(t *testing.T) {
	unset := []string{
		"BENCH_YELLOW_OVERHEAD_ABS_MS",
		"BENCH_YELLOW_OVERHEAD_REL_PCT",
		"BENCH_YELLOW_MIN_NATIVE_MS",
	}
	type envRestore struct {
		key   string
		value string
		set   bool
	}
	restores := make([]envRestore, 0, len(unset))
	for _, k := range unset {
		old, ok := os.LookupEnv(k)
		restores = append(restores, envRestore{key: k, value: old, set: ok})
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, r := range restores {
			if r.set {
				_ = os.Setenv(r.key, r.value)
				continue
			}
			_ = os.Unsetenv(r.key)
		}
	})
	defaults := loadSeverityThresholds()
	if defaults.YellowOverheadAbsMs != 20 || defaults.YellowOverheadRel != 0.25 || defaults.YellowMinNativeMs != 50 {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	if err := os.Setenv("BENCH_YELLOW_OVERHEAD_ABS_MS", "30"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("BENCH_YELLOW_OVERHEAD_REL_PCT", "40"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("BENCH_YELLOW_MIN_NATIVE_MS", "80"); err != nil {
		t.Fatal(err)
	}
	overrides := loadSeverityThresholds()
	if overrides.YellowOverheadAbsMs != 30 || overrides.YellowOverheadRel != 0.40 || overrides.YellowMinNativeMs != 80 {
		t.Fatalf("unexpected overrides: %+v", overrides)
	}
}

func TestApplyScenarioStatus(t *testing.T) {
	thresholds := severityThresholds{
		YellowOverheadAbsMs: 20,
		YellowOverheadRel:   0.25,
		YellowMinNativeMs:   50,
	}

	red := ScenarioResult{
		Required:      true,
		Success:       false,
		SafetyPassed:  false,
		ExitCodeMatch: false,
	}
	applyScenarioStatus(&red, thresholds)
	if red.Status != "red" {
		t.Fatalf("status=%q want red", red.Status)
	}

	yellow := ScenarioResult{
		SafetyPassed:       true,
		ExitCodeMatch:      true,
		Success:            true,
		Native:             CommandResult{DurationMs: 100},
		Proxy:              CommandResult{DurationMs: 140},
		ProxyOverheadMs:    40,
		ProxyOverheadRatio: 1.4,
	}
	applyScenarioStatus(&yellow, thresholds)
	if yellow.Status != "yellow" {
		t.Fatalf("status=%q want yellow", yellow.Status)
	}
	if !strings.Contains(strings.Join(yellow.Warnings, "; "), "overhead regression") {
		t.Fatalf("expected overhead warning, got %q", strings.Join(yellow.Warnings, "; "))
	}

	green := ScenarioResult{
		SafetyPassed:       true,
		ExitCodeMatch:      true,
		Success:            true,
		Native:             CommandResult{DurationMs: 120},
		ProxyOverheadMs:    10,
		ProxyOverheadRatio: 1.05,
	}
	applyScenarioStatus(&green, thresholds)
	if green.Status != "green" {
		t.Fatalf("status=%q want green", green.Status)
	}
}

func TestWriteSummaryStatusColumnFirst(t *testing.T) {
	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	if err := os.Setenv("GITHUB_STEP_SUMMARY", summaryPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Unsetenv("GITHUB_STEP_SUMMARY"); err != nil {
			t.Fatalf("unset GITHUB_STEP_SUMMARY: %v", err)
		}
	})

	report := RunReport{
		Tokenizer: "test",
		Results: []ScenarioResult{
			{
				Scenario:        "example",
				Status:          "green",
				Required:        true,
				Native:          CommandResult{Spec: "go test ./...", DurationMs: 100, TokenCount: 100},
				Proxy:           CommandResult{DurationMs: 110, TokenCount: 90},
				ProxyOverheadMs: 10,
				SafetyPassed:    true,
				ExitCodeMatch:   true,
			},
		},
	}
	if err := writeSummary(report); err != nil {
		t.Fatalf("writeSummary: %v", err)
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("unexpected summary content: %q", string(data))
	}
	if !strings.HasPrefix(lines[2], "| Status | Scenario |") {
		t.Fatalf("expected status-first header, got %q", lines[2])
	}
}

func TestEnvIntFallbackOnInvalid(t *testing.T) {
	if err := os.Setenv("BENCH_YELLOW_OVERHEAD_ABS_MS", "invalid"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Unsetenv("BENCH_YELLOW_OVERHEAD_ABS_MS"); err != nil {
			t.Fatalf("unset BENCH_YELLOW_OVERHEAD_ABS_MS: %v", err)
		}
	})
	if got := envInt("BENCH_YELLOW_OVERHEAD_ABS_MS", 99); got != 99 {
		t.Fatalf("envInt invalid fallback = %d, want %d", got, 99)
	}
	if err := os.Setenv("BENCH_YELLOW_OVERHEAD_ABS_MS", strconv.Itoa(17)); err != nil {
		t.Fatal(err)
	}
	if got := envInt("BENCH_YELLOW_OVERHEAD_ABS_MS", 99); got != 17 {
		t.Fatalf("envInt parsed = %d, want %d", got, 17)
	}
}

func TestRunTextOnlyStripsSequencePrefixesAndCombinesStreamsForTokenCount(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	toolDir := filepath.Join(root, "demo")
	scenarioName := "demo-text-only"
	scenarioDir := filepath.Join(toolDir, scenarioName)
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(toolDir, scenariosFileName), []Scenario{
		{
			Name:       scenarioName,
			Tool:       "demo",
			TextOnly:   true,
			Native:     []string{"demo", "run"},
			ExpectExit: 0,
			Required:   true,
		},
	})
	writeFile(t, filepath.Join(scenarioDir, "input-stdout.txt"), "00000|stdout-line\n")
	writeFile(t, filepath.Join(scenarioDir, "input-stderr.txt"), "00001|stderr-line\n")
	writeFile(t, filepath.Join(scenarioDir, benchmarkOutputFileName), "proxy-out\n")

	report, err := Run(RunOptions{
		FixturesRoot: root,
		ArtifactsDir: artifacts,
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]

	mergedWithoutPrefixes := "stdout-line\nstderr-line\n"
	wantNativeTokens := CountTokens(mergedWithoutPrefixes)
	if r.Native.TokenCount != wantNativeTokens {
		t.Fatalf("native token count = %d, want %d", r.Native.TokenCount, wantNativeTokens)
	}
	if r.RawInputHash != sha256Hex(mergedWithoutPrefixes) {
		t.Fatalf("raw input hash = %s, want %s", r.RawInputHash, sha256Hex(mergedWithoutPrefixes))
	}
}

func TestRunTextOnlyPrefersInputTxtOverSequencedStreams(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	toolDir := filepath.Join(root, "demo")
	scenarioName := "demo-text-only-input-priority"
	scenarioDir := filepath.Join(toolDir, scenarioName)
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeScenarios(t, filepath.Join(toolDir, scenariosFileName), []Scenario{
		{
			Name:       scenarioName,
			Tool:       "demo",
			TextOnly:   true,
			Native:     []string{"demo", "run"},
			ExpectExit: 0,
			Required:   true,
		},
	})
	writeFile(t, filepath.Join(scenarioDir, benchmarkInputFileName), "input-file-wins\n")
	writeFile(t, filepath.Join(scenarioDir, "input-stdout.txt"), "00000|stdout-ignored\n")
	writeFile(t, filepath.Join(scenarioDir, "input-stderr.txt"), "00001|stderr-ignored\n")
	writeFile(t, filepath.Join(scenarioDir, benchmarkOutputFileName), "proxy-out\n")

	report, err := Run(RunOptions{
		FixturesRoot: root,
		ArtifactsDir: artifacts,
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	wantNativeTokens := CountTokens("input-file-wins\n")
	if r.Native.TokenCount != wantNativeTokens {
		t.Fatalf("native token count = %d, want %d", r.Native.TokenCount, wantNativeTokens)
	}
	if r.RawInputHash != sha256Hex("input-file-wins\n") {
		t.Fatalf("raw input hash = %s, want %s", r.RawInputHash, sha256Hex("input-file-wins\n"))
	}
}

func TestRunIsolatesWarmupStateButKeepsSharedResources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses POSIX sh")
	}

	root := t.TempDir()
	artifacts := t.TempDir()
	projectDir := filepath.Join(root, "demo", "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(projectDir, "before.sh"), "#!/usr/bin/env sh\nset -eu\nmkdir -p .cache\ntest -f .cache/deps || echo ready > .cache/deps\necho 0 > state.txt\n")
	writeFile(t, filepath.Join(projectDir, "after.sh"), "#!/usr/bin/env sh\nset -eu\nrm -f state.txt\n")
	if err := os.Chmod(filepath.Join(projectDir, "before.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(projectDir, "after.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	native := []string{
		"sh",
		"-c",
		`n=$(cat state.txt); n=$((n+1)); echo "$n" > state.txt; test -f .cache/deps && echo "cache=1"; echo "n=$n"`,
	}
	writeScenarios(t, filepath.Join(root, "demo", scenariosFileName), []Scenario{
		{
			Name:       "warmup-isolation",
			Tool:       "demo",
			Project:    "project",
			Native:     native,
			ExpectExit: 0,
			Required:   true,
			BeforeStart: [][]string{
				{"./before.sh"},
			},
			AfterStop: [][]string{
				{"./after.sh"},
			},
			StructuredOutput: true,
			MustContain:      []string{"cache=1", "n=1"},
		},
	})

	report, err := Run(RunOptions{
		FixturesRoot: root,
		ArtifactsDir: artifacts,
		ProxyBinary:  native[0], // run same command shape as proxy surrogate
		Timeout:      2 * time.Second,
	})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	if !r.Success || !r.SafetyPassed {
		t.Fatalf("expected successful isolated run, got success=%t safety=%t warnings=%v", r.Success, r.SafetyPassed, r.Warnings)
	}
	if r.Native.TokenCount <= 0 || r.Proxy.TokenCount <= 0 {
		t.Fatalf("expected token counts to be computed, got native=%d proxy=%d", r.Native.TokenCount, r.Proxy.TokenCount)
	}
}

func TestRunSkipsCaptureRawArtifactsForNonCCPProxy(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", inputFileName), "hello\n")
	native := catArgs(inputFileName)
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: "ls-no-capture-raw", Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})

	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	if r.Proxy.StdoutArtifact != "" || r.Proxy.StderrArtifact != "" {
		t.Fatalf("expected no capture-raw artifacts for non-ccp proxy, got stdout=%q stderr=%q", r.Proxy.StdoutArtifact, r.Proxy.StderrArtifact)
	}
}

func TestSupportsCaptureRawByProxyBinaryName(t *testing.T) {
	if !supportsCaptureRaw("ccp") {
		t.Fatal("expected ccp to support capture-raw")
	}
	if !supportsCaptureRaw("/tmp/bin/ccp.exe") {
		t.Fatal("expected ccp.exe to support capture-raw")
	}
	if supportsCaptureRaw("cat") {
		t.Fatal("expected non-ccp binary to skip capture-raw support")
	}
}

func TestRunDoesNotWriteEmptyProxyOutputArtifact(t *testing.T) {
	root := t.TempDir()
	artifacts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ls", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "ls", "project", "empty.txt"), "")
	native := catArgs("empty.txt")
	writeScenarios(t, filepath.Join(root, "ls", scenariosFileName), []Scenario{
		{Name: lsEmptyScenarioName, Tool: "ls", Project: "project", Native: native, ExpectExit: 0, Required: true},
	})

	report, err := Run(RunOptions{FixturesRoot: root, ArtifactsDir: artifacts, ProxyBinary: native[0], Timeout: time.Second})
	if err != nil {
		t.Fatalf(runErrFmt, err)
	}
	if len(report.Results) != 1 {
		t.Fatalf(expectedOneResultFmt, len(report.Results))
	}
	r := report.Results[0]
	if r.Proxy.Artifact != "" {
		t.Fatalf("expected empty proxy artifact path for empty output, got %q", r.Proxy.Artifact)
	}
	path := filepath.Join(artifacts, "ls", lsEmptyScenarioName, benchmarkOutputFileName)
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("expected no output artifact file, but found %s", path)
	}
	scenarioDir := filepath.Join(artifacts, "ls", lsEmptyScenarioName)
	if _, statErr := os.Stat(scenarioDir); statErr == nil {
		t.Fatalf("expected empty scenario artifact directory to be removed, but found %s", scenarioDir)
	}
}
