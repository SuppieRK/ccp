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
	"git-show",
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
	cases, err := collectFixtureCases(root)
	if err != nil {
		t.Fatalf("collect fixture cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no fixture cases found")
	}
	for _, c := range cases {
		t.Run(c.caseID(), func(t *testing.T) {
			assertFixtureCaseMatches(t, c)
		})
	}
}

func collectFixtureCases(root string) ([]fixtureCase, error) {
	tools, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	cases := make([]fixtureCase, 0, 128)
	for _, toolEntry := range tools {
		if !toolEntry.IsDir() {
			continue
		}
		toolCases, err := collectFixtureCasesForTool(root, toolEntry.Name())
		if err != nil {
			return nil, err
		}
		cases = append(cases, toolCases...)
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].caseID() < cases[j].caseID()
	})
	return cases, nil
}

func collectFixtureCasesForTool(root, fixtureSet string) ([]fixtureCase, error) {
	toolDir := filepath.Join(root, fixtureSet)
	scenarioMeta := loadScenarioMetaMap(toolDir)
	entries, err := os.ReadDir(toolDir)
	if err != nil {
		return nil, err
	}
	scenarios := fixtureScenarioNames(scenarioMeta, entries)
	if len(scenarioMeta) == 0 && len(scenarios) == 0 {
		return nil, nil
	}
	cases := make([]fixtureCase, 0, len(scenarios))
	for _, scenario := range scenarios {
		meta := fixtureCaseMeta(toolDir, scenario, scenarioMeta)
		if meta.Tool == "" {
			continue
		}
		scenarioDir := filepath.Join(toolDir, scenario)
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
	return cases, nil
}

func fixtureScenarioNames(scenarioMeta map[string]fixtureMeta, entries []os.DirEntry) []string {
	scenarioSeen := map[string]struct{}{}
	scenarios := make([]string, 0, len(scenarioMeta)+len(entries))
	addScenario := func(name string) {
		n := strings.TrimSpace(name)
		if n == "" || n == "projects" {
			return
		}
		if _, ok := scenarioSeen[n]; ok {
			return
		}
		scenarioSeen[n] = struct{}{}
		scenarios = append(scenarios, n)
	}
	for name := range scenarioMeta {
		addScenario(name)
	}
	for _, e := range entries {
		if e.IsDir() {
			addScenario(e.Name())
		}
	}
	sort.Strings(scenarios)
	return scenarios
}

func fixtureCaseMeta(toolDir, scenario string, scenarioMeta map[string]fixtureMeta) fixtureMeta {
	if meta, ok := scenarioMeta[scenario]; ok {
		return meta
	}
	return fixtureMeta{
		Tool:     toolNameFromFolder(filepath.Base(toolDir)),
		Dispatch: "",
		Trigger:  "both",
		ExitCode: 0,
		Stream:   string(engine.StdoutStream),
	}
}

func assertFixtureCaseMatches(t *testing.T, c fixtureCase) {
	t.Helper()
	meta := c.meta
	if meta.Tool == "" {
		t.Fatalf("fixture %s missing tool in meta", c.caseID())
	}
	got := actualFixtureOutput(t, c)
	if hasFixtureContainmentRules(meta) {
		assertFixtureContainment(t, got, meta)
		return
	}
	if meta.StructuredOutput {
		assertFixtureStructuredOutput(t, got, c)
		return
	}
	want := expectedFixtureOutput(t, c.outputPath)
	if got != want {
		t.Fatalf("output mismatch\nwant:\n%s\n---\ngot:\n%s", want, got)
	}
}

func actualFixtureOutput(t *testing.T, c fixtureCase) string {
	t.Helper()
	if st, err := os.Stat(c.dir); err == nil && st.IsDir() {
		return normalizeFixtureOutput(runFixtureCase(t, c.meta, c, c.scenario))
	}
	return ""
}

func hasFixtureContainmentRules(meta fixtureMeta) bool {
	return len(meta.MustContain) > 0 || len(meta.MustNotContain) > 0
}

func assertFixtureContainment(t *testing.T, got string, meta fixtureMeta) {
	t.Helper()
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
}

func assertFixtureStructuredOutput(t *testing.T, got string, c fixtureCase) {
	t.Helper()
	wantStructured := normalizeFixtureOutput(buildStructuredFixtureOutput(t, c))
	if got != wantStructured {
		t.Fatalf("structured output mismatch\nwant:\n%s\n---\ngot:\n%s", wantStructured, got)
	}
}

func buildStructuredFixtureOutput(t *testing.T, c fixtureCase) string {
	t.Helper()
	if c.stdoutPath != "" || c.stderrPath != "" {
		events, err := readSequencedEvents(c.stdoutPath, c.stderrPath)
		if err != nil {
			t.Fatalf("build structured expected output: %v", err)
		}
		var b strings.Builder
		for _, ev := range events {
			b.WriteString(fixtureANSIEscapeRe.ReplaceAllString(ev.line, ""))
		}
		return normalizeNewlines(b.String())
	}
	if c.inputPath == "" {
		return ""
	}
	in, err := os.ReadFile(c.inputPath)
	if err != nil {
		t.Fatalf("build structured expected output: %v", err)
	}
	return normalizeNewlines(fixtureANSIEscapeRe.ReplaceAllString(string(in), ""))
}

func expectedFixtureOutput(t *testing.T, path string) string {
	t.Helper()
	if path == "" {
		return ""
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output fixture: %v", err)
	}
	return normalizeFixtureOutput(string(wantBytes))
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
	restore := chdirFixtureProject(t, meta.Project, paths.caseDir)
	defer restore()

	registry := newFixtureRegistry(t)
	eng := engine.NewEngine(engine.Config{Registry: registry, NeverDropPatterns: engine.DefaultNeverDropPatterns()})
	eng.SetCommandID("fixture:" + caseName)
	dispatch := resolveFixtureDispatch(meta, registry)

	var out strings.Builder
	stream := fixtureStream(meta)
	trigger := fixtureTrigger(meta)
	seenStream, hasSplit := processFixtureInputEvents(t, &out, eng, meta.Tool, dispatch, stream, paths)
	emitFixtureEOF(&out, eng, meta.Tool, dispatch, stream, trigger, hasSplit)
	emitFixtureExit(&out, eng, meta.Tool, dispatch, stream, trigger, hasSplit, seenStream, meta.ExitCode)
	return normalizeNewlines(out.String())
}

func chdirFixtureProject(t *testing.T, project, caseDir string) func() {
	t.Helper()
	restore := func() {}
	if strings.TrimSpace(project) == "" {
		return restore
	}
	projectDir := filepath.Clean(filepath.Join(filepath.Dir(caseDir), project))
	if st, err := os.Stat(projectDir); err == nil && st.IsDir() {
		if origWD, err := os.Getwd(); err == nil && os.Chdir(projectDir) == nil {
			restore = func() { _ = os.Chdir(origWD) }
		}
	}
	return restore
}

func newFixtureRegistry(t *testing.T) *engine.ToolFilterRegistry {
	t.Helper()
	registry := engine.NewToolFilterRegistry()
	for _, filter := range []engine.ToolFilter{
		NewLSCompactor(),
		NewGitToolFilter(),
		NewGradleFilter(),
		NewMavenFilter(),
		NewDenoFilter(),
		NewNodeFilter(),
		NewPythonFilter(),
		NewPytestFilter(),
		NewPIPFilter(),
		NewNPMFilter(),
		NewPNPMFilter(),
		NewYarnFilter(),
		NewNPXFilter(),
		NewGrepFilter(),
		NewFindFilter(),
		NewKubectlToolFilter(),
		NewDockerToolFilter(),
		NewGoToolFilter(),
		NewCargoToolFilter(),
	} {
		mustRegister(t, registry, filter)
	}
	return registry
}

func resolveFixtureDispatch(meta fixtureMeta, registry *engine.ToolFilterRegistry) string {
	dispatch := strings.TrimSpace(meta.Dispatch)
	if meta.Tool != "" && len(meta.Native) > 0 {
		if f := registry.Resolve(meta.Tool); f != nil {
			tail := meta.Native
			if len(tail) > 0 {
				tail = tail[1:]
			}
			dispatch = dispatchFromPreparedArgs(meta.Tool, dispatch, f.Prepare(tail))
		}
	}
	if dispatch == "" {
		return meta.Tool
	}
	return dispatch
}

func dispatchFromPreparedArgs(tool, fallback string, prep engine.PrepareResult) string {
	if strings.TrimSpace(prep.DispatchKey) != "" {
		return prep.DispatchKey
	}
	if len(prep.NormalizedArgs) > 0 {
		return tool + " " + strings.Join(prep.NormalizedArgs, " ")
	}
	return fallback
}

func fixtureStream(meta fixtureMeta) string {
	if meta.Stream == "" {
		return string(engine.StdoutStream)
	}
	return meta.Stream
}

func fixtureTrigger(meta fixtureMeta) string {
	trigger := strings.ToLower(strings.TrimSpace(meta.Trigger))
	if trigger == "" {
		return "eof"
	}
	return trigger
}

func processFixtureInputEvents(t *testing.T, out *strings.Builder, eng *engine.Engine, tool, dispatch, stream string, paths fixtureCasePaths) (map[string]bool, bool) {
	t.Helper()
	seenStream := map[string]bool{}
	if paths.hasSplit {
		processSplitFixtureEvents(t, out, eng, tool, dispatch, paths, seenStream)
		return seenStream, true
	}
	processSingleFixtureInput(t, out, eng, tool, dispatch, stream, paths.input, seenStream)
	return seenStream, false
}

func processSplitFixtureEvents(t *testing.T, out *strings.Builder, eng *engine.Engine, tool, dispatch string, paths fixtureCasePaths, seenStream map[string]bool) {
	t.Helper()
	events, err := readSequencedEvents(paths.stdout, paths.stderr)
	if err != nil {
		t.Fatalf("read sequenced fixture events: %v", err)
	}
	for _, ev := range events {
		seenStream[ev.stream] = true
		line := fixtureANSIEscapeRe.ReplaceAllString(ev.line, "")
		appendFixtureDecision(out, eng.Process(ev.stream, tool, engine.Input{Line: line, Dispatch: dispatch}))
	}
}

func processSingleFixtureInput(t *testing.T, out *strings.Builder, eng *engine.Engine, tool, dispatch, stream, inputPath string, seenStream map[string]bool) {
	t.Helper()
	if inputPath == "" {
		return
	}
	in, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read input fixture: %v", err)
	}
	for _, line := range splitInputLines(string(in)) {
		seenStream[stream] = true
		line = fixtureANSIEscapeRe.ReplaceAllString(line, "")
		appendFixtureDecision(out, eng.Process(stream, tool, engine.Input{Line: line, Dispatch: dispatch}))
	}
}

func emitFixtureEOF(out *strings.Builder, eng *engine.Engine, tool, dispatch, stream, trigger string, hasSplit bool) {
	if trigger != "eof" && trigger != "both" && trigger != "exit" {
		return
	}
	for _, s := range fixtureEOFStreams(stream, hasSplit) {
		appendFixtureDecision(out, eng.Process(s, tool, engine.Input{EOF: true, Dispatch: dispatch}))
	}
}

func fixtureEOFStreams(stream string, hasSplit bool) []string {
	if hasSplit {
		return []string{string(engine.StdoutStream), string(engine.StderrStream)}
	}
	return []string{stream}
}

func emitFixtureExit(out *strings.Builder, eng *engine.Engine, tool, dispatch, stream, trigger string, hasSplit bool, seenStream map[string]bool, exitCode int) {
	if trigger != "exit" && trigger != "both" {
		return
	}
	exitStream := fixtureExitStream(stream, hasSplit, seenStream)
	appendFixtureDecision(out, eng.Process(exitStream, tool, engine.Input{Exit: true, Code: exitCode, Dispatch: dispatch}))
}

func fixtureExitStream(stream string, hasSplit bool, seenStream map[string]bool) string {
	if !hasSplit {
		return stream
	}
	if seenStream[string(engine.StdoutStream)] {
		return string(engine.StdoutStream)
	}
	if seenStream[string(engine.StderrStream)] {
		return string(engine.StderrStream)
	}
	return stream
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
