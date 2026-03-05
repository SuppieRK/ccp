package ci

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Scenario describes one validation/benchmark case.
type Scenario struct {
	Name             string     `json:"name"`
	Tool             string     `json:"tool"`
	Project          string     `json:"project"`
	Native           []string   `json:"native"`
	TextOnly         bool       `json:"text_only"`
	BeforeStart      [][]string `json:"before_start"`
	AfterStop        [][]string `json:"after_stop"`
	ExpectExit       int        `json:"expect_exit"`
	MustContain      []string   `json:"must_contain"`
	MustNotContain   []string   `json:"must_not_contain"`
	IgnoreLines      []string   `json:"ignore_lines"`
	StructuredOutput bool       `json:"structured_output"`
	Required         bool       `json:"required"`
}

// DiscoveredScenario pairs a scenario with filesystem context.
type DiscoveredScenario struct {
	Path       string
	Dir        string
	FixtureKey string
	Spec       Scenario
}

const scenarioDefinitionsFile = "scenarios.json"

func loadScenarios(path string) ([]Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Scenario
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	filtered := make([]Scenario, 0, len(out))
	for _, s := range out {
		if isEmptyScenario(s) {
			continue
		}
		if err := validateScenario(s); err != nil {
			return nil, fmt.Errorf("validate %s: %w", path, err)
		}
		filtered = append(filtered, s)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("validate %s: at least one scenario is required", path)
	}
	return filtered, nil
}

func isEmptyScenario(s Scenario) bool {
	return s.Name == "" &&
		s.Tool == "" &&
		s.Project == "" &&
		len(s.Native) == 0 &&
		!s.TextOnly &&
		len(s.BeforeStart) == 0 &&
		len(s.AfterStop) == 0 &&
		s.ExpectExit == 0 &&
		len(s.MustContain) == 0 &&
		len(s.MustNotContain) == 0 &&
		len(s.IgnoreLines) == 0 &&
		!s.StructuredOutput &&
		!s.Required
}

func validateScenario(s Scenario) error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(s.Tool) == "" {
		return errors.New("tool is required")
	}
	if len(s.Native) == 0 {
		return errors.New("native command is required")
	}
	if err := validateCommand(s.Native, "native"); err != nil {
		return err
	}
	if err := validateHookCommands(s.BeforeStart, "before_start"); err != nil {
		return err
	}
	if err := validateHookCommands(s.AfterStop, "after_stop"); err != nil {
		return err
	}
	for _, line := range s.IgnoreLines {
		if strings.TrimSpace(line) == "" {
			return errors.New("ignore_lines entries must be non-empty")
		}
		if _, err := regexp.Compile(line); err != nil {
			return fmt.Errorf("invalid ignore_lines regex %q: %w", line, err)
		}
	}
	return nil
}

func validateHookCommands(cmds [][]string, field string) error {
	for i, cmd := range cmds {
		if err := validateCommand(cmd, fmt.Sprintf("%s[%d]", field, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateCommand(cmd []string, field string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("%s command is required", field)
	}
	for _, part := range cmd {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("%s command entries must be non-empty", field)
		}
	}
	return nil
}

// discoverScenarios scans benchmark tool directories containing scenario definitions.
func discoverScenarios(root string) ([]DiscoveredScenario, error) {
	out := make([]DiscoveredScenario, 0, 64)
	processedToolDirs := map[string]struct{}{}
	roots, err := expandFixtureRoots(root)
	if err != nil {
		return nil, err
	}

	for _, resolvedRoot := range roots {
		if err := discoverScenariosFromRoot(resolvedRoot, processedToolDirs, &out); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Spec.Name < out[j].Spec.Name
	})
	return out, nil
}

func discoverScenariosFromRoot(resolvedRoot string, processedToolDirs map[string]struct{}, out *[]DiscoveredScenario) error {
	// Support both roots:
	// 1) a parent directory containing many tool folders
	// 2) a single tool folder containing the scenario definitions file directly
	if hasScenarioDefinitions(resolvedRoot) {
		if err := appendScenariosFromToolDir(resolvedRoot, filepath.Base(resolvedRoot), processedToolDirs, out); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		toolDir := filepath.Join(resolvedRoot, e.Name())
		if !hasScenarioDefinitions(toolDir) {
			continue
		}
		if err := appendScenariosFromToolDir(toolDir, e.Name(), processedToolDirs, out); err != nil {
			return err
		}
	}
	return nil
}

func hasScenarioDefinitions(dir string) bool {
	if st, err := os.Stat(filepath.Join(dir, scenarioDefinitionsFile)); err == nil && !st.IsDir() {
		return true
	}
	return false
}

func appendScenariosFromToolDir(toolDir, fixtureKey string, processedToolDirs map[string]struct{}, out *[]DiscoveredScenario) error {
	if _, seen := processedToolDirs[toolDir]; seen {
		return nil
	}
	processedToolDirs[toolDir] = struct{}{}

	scenariosPath := filepath.Join(toolDir, scenarioDefinitionsFile)
	scenarios, err := loadScenarios(scenariosPath)
	if err != nil {
		return err
	}
	seenNames := map[string]struct{}{}
	for _, s := range scenarios {
		if err := appendDiscoveredScenario(s, scenariosPath, toolDir, fixtureKey, seenNames, out); err != nil {
			return err
		}
	}
	return nil
}

func appendDiscoveredScenario(s Scenario, scenariosPath, toolDir, fixtureKey string, seenNames map[string]struct{}, out *[]DiscoveredScenario) error {
	if _, ok := seenNames[s.Name]; ok {
		return fmt.Errorf("duplicate scenario name %q in %s", s.Name, scenariosPath)
	}
	seenNames[s.Name] = struct{}{}
	projectDir := scenarioProjectDir(toolDir, s.Project)
	// text_only scenarios rely on fixture I/O and do not execute native commands
	// from projectDir, so missing project directories are allowed.
	if !s.TextOnly {
		st, err := os.Stat(projectDir)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("scenario %q project directory not found: %s", s.Name, projectDir)
		}
	}
	*out = append(*out, DiscoveredScenario{
		Path:       scenariosPath,
		Dir:        toolDir,
		FixtureKey: fixtureKey,
		Spec:       s,
	})
	return nil
}

func scenarioProjectDir(toolDir, project string) string {
	if strings.TrimSpace(project) == "" {
		return toolDir
	}
	return filepath.Join(toolDir, project)
}

func expandFixtureRoots(root string) ([]string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, fmt.Errorf("fixtures root is required")
	}
	if !hasGlobMeta(trimmed) {
		return []string{trimmed}, nil
	}
	matches, err := doublestar.FilepathGlob(trimmed)
	if err != nil {
		return nil, fmt.Errorf("expand fixtures root pattern %q: %w", trimmed, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("fixtures root pattern %q matched no paths", trimmed)
	}
	roots := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		roots = append(roots, match)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("fixtures root pattern %q matched no directories", trimmed)
	}
	sort.Strings(roots)
	return roots, nil
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
