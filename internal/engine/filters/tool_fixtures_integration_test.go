package filters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

var fixtureANSIEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var fixtureJSONTimeRe = regexp.MustCompile(`"Time":"[^"]+"`)

type fixtureMeta struct {
	Tool             string `json:"tool"`
	Dispatch         string `json:"dispatch"`
	Trigger          string `json:"trigger"`
	ExitCode         int    `json:"exitCode"`
	Stream           string `json:"stream"`
	StructuredOutput bool
	MustContain      []string
	MustNotContain   []string
	Native           []string
	Project          string
}

var toolSpecNames = []string{
	"cargo",
	"cargo-test",
	"cargo-build",
	"cargo-check",
	"cargo-clippy",
	"deno",
	"docker",
	"docker-ps",
	"docker-images",
	"docker-logs",
	"find",
	"go",
	"go-test",
	"go-build",
	"gradle",
	"grep",
	"ls",
	"kubectl",
	"kubectl-get-pods",
	"kubectl-get-nodes",
	"kubectl-get-services",
	"kubectl-logs",
	"maven",
	"node",
	"npm",
	"npx",
	"npx-tsc",
	"npx-eslint",
	"npx-prettier",
	"npx-prisma",
	"npx-node",
	"pip",
	"pnpm",
	"yarn",
	"git",
	"git-blame",
	"git-commit",
	"git-diff",
	"git-log",
	"git-merge",
	"git-pull",
	"git-push",
	"git-rebase",
	"git-status",
}

func TestToolFixtureCoverage(t *testing.T) {
	fixturesRoot := filepath.Join("..", "..", "..", "testdata", "tool-fixtures")
	for _, specName := range toolSpecNames {
		specPath := filepath.Join("..", "..", "..", "openspec", "specs", specName, "spec.md")
		if st, err := os.Stat(specPath); err != nil || st.IsDir() {
			t.Fatalf("missing tool spec: %s", specPath)
		}

		fixtureDir := filepath.Join(fixturesRoot, specName)
		if st, err := os.Stat(filepath.Join(fixtureDir, "scenarios.json")); err != nil || st.IsDir() {
			continue
		}
		meta := loadScenarioMetaMap(fixtureDir)
		covered := 0
		for _, m := range meta {
			if m.Tool == "" {
				continue
			}
			if m.Tool != specName && !strings.HasPrefix(specName, m.Tool+"-") {
				continue
			}
			covered++
		}
		if covered == 0 {
			t.Fatalf("missing fixture cases for spec %q in %s", specName, fixtureDir)
		}
	}
}

func TestToolFixturesMatchExpectedOutput(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "tool-fixtures")
	tools, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixture root: %v", err)
	}
	cases := make([]fixtureCase, 0, 128)
	for _, toolEntry := range tools {
		if !toolEntry.IsDir() {
			continue
		}
		fixtureSet := toolEntry.Name()
		toolDir := filepath.Join(root, fixtureSet)
		scenarioMeta := loadScenarioMetaMap(toolDir)
		entries, err := os.ReadDir(toolDir)
		if err != nil {
			t.Fatalf("read fixture set %s: %v", fixtureSet, err)
		}
		scenarioSeen := map[string]struct{}{}
		scenarios := make([]string, 0, len(scenarioMeta)+len(entries))
		for name := range scenarioMeta {
			n := strings.TrimSpace(name)
			if n == "" {
				continue
			}
			if _, ok := scenarioSeen[n]; ok {
				continue
			}
			scenarioSeen[n] = struct{}{}
			scenarios = append(scenarios, n)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			n := strings.TrimSpace(e.Name())
			if n == "" || n == "projects" {
				continue
			}
			if _, ok := scenarioSeen[n]; ok {
				continue
			}
			scenarioSeen[n] = struct{}{}
			scenarios = append(scenarios, n)
		}
		if len(scenarioMeta) == 0 && len(scenarios) == 0 {
			continue
		}
		sort.Strings(scenarios)
		for _, scenario := range scenarios {
			meta, hasMeta := scenarioMeta[scenario]
			if !hasMeta {
				meta = fixtureMeta{
					Tool:     toolNameFromFolder(filepath.Base(toolDir)),
					Dispatch: "",
					Trigger:  "both",
					ExitCode: 0,
					Stream:   string(engine.StdoutStream),
				}
			}
			scenarioDir := filepath.Join(toolDir, scenario)
			if meta.Tool == "" {
				continue
			}
			cases = append(cases, fixtureCase{
				fixtureSet: fixtureSet,
				scenario:   scenario,
				dir:        scenarioDir,
				outputPath: firstExisting(filepath.Join(scenarioDir, "output.txt")),
				inputPath:  firstExisting(filepath.Join(scenarioDir, "input.txt")),
				stdoutPath: firstExisting(filepath.Join(scenarioDir, "input-stdout.txt")),
				stderrPath: firstExisting(filepath.Join(scenarioDir, "input-stderr.txt")),
				meta:       meta,
			})
		}
	}
	if len(cases) == 0 {
		t.Fatal("no fixture cases found")
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].caseID() < cases[j].caseID()
	})

	for _, c := range cases {
		t.Run(c.caseID(), func(t *testing.T) {
			meta := c.meta
			if meta.Tool == "" {
				t.Fatalf("fixture %s missing tool in meta", c.caseID())
			}
			got := ""
			if st, err := os.Stat(c.dir); err == nil && st.IsDir() {
				got = normalizeFixtureOutput(runFixtureCase(t, c.meta, c, c.scenario))
			}
			if len(meta.MustContain) > 0 || len(meta.MustNotContain) > 0 {
				for _, expected := range meta.MustContain {
					if !strings.Contains(got, expected) {
						t.Fatalf("missing must_contain: %q\ngot:\n%s", expected, got)
					}
				}
				for _, forbidden := range meta.MustNotContain {
					if strings.Contains(got, forbidden) {
						t.Fatalf("unexpected must_not_contain: %q\ngot:\n%s", forbidden, got)
					}
				}
				return
			}
			if meta.StructuredOutput {
				wantStructured := ""
				if c.stdoutPath != "" || c.stderrPath != "" {
					events, err := readSequencedEvents(c.stdoutPath, c.stderrPath)
					if err != nil {
						t.Fatalf("build structured expected output: %v", err)
					}
					var b strings.Builder
					for _, ev := range events {
						b.WriteString(fixtureANSIEscapeRe.ReplaceAllString(ev.line, ""))
					}
					wantStructured = normalizeNewlines(b.String())
				} else if c.inputPath != "" {
					in, err := os.ReadFile(c.inputPath)
					if err != nil {
						t.Fatalf("build structured expected output: %v", err)
					}
					wantStructured = normalizeNewlines(fixtureANSIEscapeRe.ReplaceAllString(string(in), ""))
				}
				wantStructured = normalizeFixtureOutput(wantStructured)
				if got != wantStructured {
					t.Fatalf("structured output mismatch\nwant:\n%s\n---\ngot:\n%s", wantStructured, got)
				}
				return
			}
			want := ""
			if c.outputPath != "" {
				wantBytes, err := os.ReadFile(c.outputPath)
				if err != nil {
					t.Fatalf("read output fixture: %v", err)
				}
				want = normalizeFixtureOutput(string(wantBytes))
			}
			if got != want {
				t.Fatalf("output mismatch\nwant:\n%s\n---\ngot:\n%s", want, got)
			}
		})
	}
}

type fixtureCase struct {
	fixtureSet string
	scenario   string
	dir        string
	outputPath string
	inputPath  string
	stdoutPath string
	stderrPath string
	meta       fixtureMeta
}

func (c fixtureCase) caseID() string {
	return filepath.ToSlash(filepath.Join(c.fixtureSet, c.scenario))
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

type scenarioMetaFile struct {
	Name             string   `json:"name"`
	Tool             string   `json:"tool"`
	Native           []string `json:"native"`
	Project          string   `json:"project"`
	ExpectExit       int      `json:"expect_exit"`
	MustContain      []string `json:"must_contain"`
	MustNotContain   []string `json:"must_not_contain"`
	StructuredOutput bool     `json:"structured_output"`
}

func loadScenarioMetaMap(toolDir string) map[string]fixtureMeta {
	metaMap := map[string]fixtureMeta{}
	scenariosPath := filepath.Join(toolDir, "scenarios.json")
	b, err := os.ReadFile(scenariosPath)
	if err != nil {
		return metaMap
	}
	var defs []scenarioMetaFile
	if json.Unmarshal(b, &defs) != nil {
		return metaMap
	}
	for _, d := range defs {
		if d.Name == "" {
			continue
		}
		m := fixtureMeta{
			Tool:             d.Tool,
			Dispatch:         strings.TrimSpace(strings.Join(d.Native, " ")),
			Trigger:          "both",
			ExitCode:         d.ExpectExit,
			Stream:           string(engine.StdoutStream),
			StructuredOutput: d.StructuredOutput,
			MustContain:      append([]string{}, d.MustContain...),
			MustNotContain:   append([]string{}, d.MustNotContain...),
			Native:           append([]string{}, d.Native...),
			Project:          strings.TrimSpace(d.Project),
		}
		if m.Tool == "" {
			m.Tool = toolNameFromFolder(filepath.Base(toolDir))
		}
		if m.Dispatch == "" {
			m.Dispatch = m.Tool
		}
		metaMap[d.Name] = m
	}
	return metaMap
}

func toolNameFromFolder(folder string) string {
	parents := []string{"cargo", "docker", "git", "go", "kubectl", "npx"}
	for _, p := range parents {
		if folder == p || strings.HasPrefix(folder, p+"-") {
			return p
		}
	}
	return folder
}

type fixtureEvent struct {
	seq    int
	stream string
	line   string
}

func runFixtureCase(t *testing.T, meta fixtureMeta, c fixtureCase, caseName string) string {
	t.Helper()
	paths := fixtureCasePaths{
		stdout:   absoluteFixturePath(c.stdoutPath),
		stderr:   absoluteFixturePath(c.stderrPath),
		input:    absoluteFixturePath(c.inputPath),
		caseDir:  absoluteFixturePath(c.dir),
		hasSplit: c.stdoutPath != "" || c.stderrPath != "",
	}
	restore := func() {}
	if strings.TrimSpace(meta.Project) != "" {
		projectDir := filepath.Clean(filepath.Join(filepath.Dir(paths.caseDir), meta.Project))
		if st, err := os.Stat(projectDir); err == nil && st.IsDir() {
			if origWD, err := os.Getwd(); err == nil && os.Chdir(projectDir) == nil {
				restore = func() { _ = os.Chdir(origWD) }
			}
		}
	}
	defer restore()

	registry := engine.NewToolFilterRegistry()
	mustRegister(t, registry, NewLSCompactor())
	mustRegister(t, registry, NewGitToolFilter())
	mustRegister(t, registry, NewGradleFilter())
	mustRegister(t, registry, NewMavenFilter())
	mustRegister(t, registry, NewDenoFilter())
	mustRegister(t, registry, NewNodeFilter())
	mustRegister(t, registry, NewPythonFilter())
	mustRegister(t, registry, NewPytestFilter())
	mustRegister(t, registry, NewPIPFilter())
	mustRegister(t, registry, NewNPMFilter())
	mustRegister(t, registry, NewPNPMFilter())
	mustRegister(t, registry, NewYarnFilter())
	mustRegister(t, registry, NewNPXFilter())
	mustRegister(t, registry, NewGrepFilter())
	mustRegister(t, registry, NewFindFilter())
	mustRegister(t, registry, NewKubectlToolFilter())
	mustRegister(t, registry, NewDockerToolFilter())
	mustRegister(t, registry, NewGoToolFilter())
	mustRegister(t, registry, NewCargoToolFilter())
	eng := engine.NewEngine(engine.Config{
		Registry:          registry,
		NeverDropPatterns: engine.DefaultNeverDropPatterns(),
	})
	eng.SetCommandID("fixture:" + caseName)
	dispatch := strings.TrimSpace(meta.Dispatch)
	if meta.Tool != "" && len(meta.Native) > 0 {
		if f := registry.Resolve(meta.Tool); f != nil {
			tail := meta.Native
			if len(tail) > 0 {
				tail = tail[1:]
			}
			prep := f.Prepare(tail)
			if strings.TrimSpace(prep.DispatchKey) != "" {
				dispatch = prep.DispatchKey
			} else if len(prep.NormalizedArgs) > 0 {
				dispatch = meta.Tool + " " + strings.Join(prep.NormalizedArgs, " ")
			}
		}
	}
	if dispatch == "" {
		dispatch = meta.Tool
	}

	var out strings.Builder
	stream := meta.Stream
	if stream == "" {
		stream = string(engine.StdoutStream)
	}
	trigger := strings.ToLower(strings.TrimSpace(meta.Trigger))
	if trigger == "" {
		trigger = "eof"
	}
	seenStream := map[string]bool{}
	hasSplit := false
	if paths.hasSplit {
		events, err := readSequencedEvents(paths.stdout, paths.stderr)
		if err != nil {
			t.Fatalf("read sequenced fixture events: %v", err)
		}
		for _, ev := range events {
			seenStream[ev.stream] = true
			line := fixtureANSIEscapeRe.ReplaceAllString(ev.line, "")
			appendFixtureDecision(&out, eng.Process(ev.stream, meta.Tool, engine.Input{Line: line, Dispatch: dispatch}))
		}
		hasSplit = true
	} else if paths.input != "" {
		in, err := os.ReadFile(paths.input)
		if err != nil {
			t.Fatalf("read input fixture: %v", err)
		}
		for _, line := range splitInputLines(string(in)) {
			seenStream[stream] = true
			line = fixtureANSIEscapeRe.ReplaceAllString(line, "")
			appendFixtureDecision(&out, eng.Process(stream, meta.Tool, engine.Input{Line: line, Dispatch: dispatch}))
		}
	}
	if trigger == "eof" || trigger == "both" || trigger == "exit" {
		eofStreams := []string{stream}
		if hasSplit {
			eofStreams = []string{string(engine.StdoutStream), string(engine.StderrStream)}
		}
		for _, s := range eofStreams {
			appendFixtureDecision(&out, eng.Process(s, meta.Tool, engine.Input{EOF: true, Dispatch: dispatch}))
		}
	}
	if trigger == "exit" || trigger == "both" {
		exitStream := stream
		if hasSplit {
			if seenStream[string(engine.StdoutStream)] {
				exitStream = string(engine.StdoutStream)
			} else if seenStream[string(engine.StderrStream)] {
				exitStream = string(engine.StderrStream)
			}
		}
		appendFixtureDecision(&out, eng.Process(exitStream, meta.Tool, engine.Input{Exit: true, Code: meta.ExitCode, Dispatch: dispatch}))
	}
	return normalizeNewlines(out.String())
}

type fixtureCasePaths struct {
	stdout   string
	stderr   string
	input    string
	caseDir  string
	hasSplit bool
}

func absoluteFixturePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func appendFixtureDecision(out *strings.Builder, result engine.Output) {
	if result.Ready && result.Output != "" {
		out.WriteString(result.Output)
	}
}

func readSequencedEvents(stdoutPath, stderrPath string) ([]fixtureEvent, error) {
	events := make([]fixtureEvent, 0, 64)
	load := func(path, stream string) error {
		if path == "" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, raw := range splitInputLines(string(data)) {
			sep := strings.IndexByte(raw, '|')
			seq, line := len(events), raw
			if sep > 0 {
				seqText := strings.TrimSpace(raw[:sep])
				if parsed, err := strconv.Atoi(seqText); err == nil {
					seq = parsed
					line = raw[sep+1:]
				}
			}
			if sep <= 0 {
				seq = len(events)
				line = raw
			}
			events = append(events, fixtureEvent{seq: seq, stream: stream, line: line})
		}
		return nil
	}
	if err := load(stdoutPath, string(engine.StdoutStream)); err != nil {
		return nil, err
	}
	if err := load(stderrPath, string(engine.StderrStream)); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].seq < events[j].seq
	})
	return events, nil
}

func splitInputLines(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.SplitAfter(input, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func normalizeFixtureOutput(s string) string {
	s = normalizeNewlines(s)
	return fixtureJSONTimeRe.ReplaceAllString(s, `"Time":"<time>"`)
}

func mustRegister(t *testing.T, reg *engine.ToolFilterRegistry, f engine.ToolFilter) {
	t.Helper()
	if err := reg.Register(f); err != nil {
		t.Fatalf("register filter %q: %v", f.Tool(), err)
	}
}
