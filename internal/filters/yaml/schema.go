package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"go-command-compression-proxy/internal/contracts"

	"gopkg.in/yaml.v3"
)

const (
	variableNameEmptyMessage   = "variable name must not be empty"
	variableTypeInvalidMessage = "variable type must be number or string"
	variableNameUniqueMessage  = "variable name must be unique"
	indexedPathFormat          = "%s[%d]"
)

type FilterDefinition struct {
	Version               int          `yaml:"version"`
	Filter                string       `yaml:"filter"`
	About                 string       `yaml:"about"`
	FlagsConsumingNextArg []string     `yaml:"flags_consuming_next_arg"`
	Cases                 []CaseClause `yaml:"cases"`
}

type CaseClause struct {
	ID               string           `yaml:"id"`
	Variables        []Variable       `yaml:"variables"`
	WhenArguments    *WhenArguments   `yaml:"when_arguments"`
	NormalizeCommand *CommandMutation `yaml:"normalize_command"`
	Passthrough      bool             `yaml:"passthrough"`
	CompressOutput   *OutputShape     `yaml:"compress_output"`
	Finally          *OnExit          `yaml:"finally"`
}

type Variable struct {
	Name         string  `yaml:"name"`
	Type         string  `yaml:"type"`
	InitialValue *string `yaml:"initial_value"`
	RegexGroup   string  `yaml:"regex_group"`
	DefaultValue string  `yaml:"default_value"`
}

type WhenArguments struct {
	FirstIs              string   `yaml:"first_is"`
	FirstIn              []string `yaml:"first_in"`
	HaveAny              []string `yaml:"have_any"`
	LackAny              []string `yaml:"lack_any"`
	HaveSequence         []string `yaml:"have_sequence"`
	HaveShortFlag        []string `yaml:"have_short_flag"`
	NotHaveShortFlag     []string `yaml:"not_have_short_flag"`
	HaveAllShortFlags    []string `yaml:"have_all_short_flags"`
	NotHaveAllShortFlags []string `yaml:"not_have_all_short_flags"`
	PositionalsLackAny   []string `yaml:"positionals_lack_any"`
	NoPositionals        bool     `yaml:"no_positionals"`
}

type CommandMutation struct {
	AppendIfMissing       []string `yaml:"append_if_missing"`
	AppendIfNoPositionals []string `yaml:"append_if_no_positionals"`
	AddShortFlags         []string `yaml:"add_short_flags"`
}

type OutputShape struct {
	Combined *OutputScope `yaml:"combined"`
	Stdout   *OutputScope `yaml:"stdout"`
	Stderr   *OutputScope `yaml:"stderr"`
}

type OutputScope struct {
	Lines  *OutputLines  `yaml:"lines"`
	Groups []OutputGroup `yaml:"groups"`
}

type OutputLines struct {
	Replace []ReplaceRule    `yaml:"replace"`
	Skip    []SkipOrKeepRule `yaml:"skip"`
	Keep    []SkipOrKeepRule `yaml:"keep"`
	Max     *MaxRule         `yaml:"max"`
}

type MaxRule struct {
	Count         int               `yaml:"count"`
	Print         string            `yaml:"print"`
	GroupsSummary *MaxGroupsSummary `yaml:"groups_summary"`
}

type MaxGroupsSummary struct {
	Show      int    `yaml:"show"`
	Print     string `yaml:"print"`
	Delimiter string `yaml:"delimiter"`
	Prefix    string `yaml:"prefix"`
	Suffix    string `yaml:"suffix"`
}

type ReplaceRule struct {
	Regex      string        `yaml:"regex,omitempty"`
	StartsWith string        `yaml:"starts_with,omitempty"`
	Contains   string        `yaml:"contains,omitempty"`
	EndsWith   string        `yaml:"ends_with,omitempty"`
	To         *string       `yaml:"to"`
	OnMatch    []MatchAction `yaml:"on_match"`
}

type SkipOrKeepRule struct {
	Regex      string `yaml:"regex,omitempty"`
	StartsWith string `yaml:"starts_with,omitempty"`
	Contains   string `yaml:"contains,omitempty"`
	EndsWith   string `yaml:"ends_with,omitempty"`
}

type MatchAction struct {
	Variable  string `yaml:"variable"`
	Increment *int   `yaml:"increment"`
}

type OnExit struct {
	Print string `yaml:"print"`
}

type OutputGroup struct {
	ID           string       `yaml:"id"`
	StartsWith   string       `yaml:"starts_with"`
	StartsRegex  string       `yaml:"starts_with_regex"`
	MatchesRegex string       `yaml:"matches_regex"`
	Variables    []Variable   `yaml:"variables"`
	GroupBy      string       `yaml:"group_by"`
	Initially    *OnExit      `yaml:"initially"`
	Lines        *OutputLines `yaml:"lines"`
	Finally      *OnExit      `yaml:"finally"`
}

type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func ParseDefinition(raw []byte) (*FilterDefinition, error) {
	var spec FilterDefinition
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}
	if err := consumeTrailingYAMLDocuments(dec); err != nil {
		return nil, err
	}
	if err := ValidateDefinition(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func consumeTrailingYAMLDocuments(dec *yaml.Decoder) error {
	var tail struct{}
	if err := dec.Decode(&tail); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected additional YAML document")
}

func ValidateDefinition(spec *FilterDefinition) error {
	if spec == nil {
		return ValidationError{Path: "root", Message: "filter definition is nil"}
	}
	if spec.Version != 1 {
		return ValidationError{Path: "version", Message: "version must be exactly 1"}
	}
	if spec.Filter == "" {
		return ValidationError{Path: "filter", Message: "filter id must not be empty"}
	}
	if err := validateFlagsConsumingNextArg(spec.FlagsConsumingNextArg, validationPath("flags_consuming_next_arg")); err != nil {
		return err
	}
	if len(spec.Cases) == 0 {
		return ValidationError{Path: "cases", Message: "at least one case is required"}
	}

	seenCases := map[string]struct{}{}
	for i := range spec.Cases {
		if err := validateCase(&spec.Cases[i], i); err != nil {
			return err
		}
		id := spec.Cases[i].ID
		if _, ok := seenCases[id]; ok {
			return ValidationError{Path: string(casePath(i).Path("id")), Message: "case id must be unique"}
		}
		seenCases[id] = struct{}{}
	}
	return nil
}

func validateFlagsConsumingNextArg(flags []string, path validationPath) error {
	for i, flag := range flags {
		if flag == "" {
			return ValidationError{Path: string(indexedValidationPath(path, i)), Message: "flags_consuming_next_arg entries must not be empty"}
		}
		if flag[0] != '-' {
			return ValidationError{Path: string(indexedValidationPath(path, i)), Message: "flags_consuming_next_arg entries must start with '-'"}
		}
	}
	return nil
}

func validateCase(cs *CaseClause, index int) error {
	path := casePath(index)
	if cs.ID == "" {
		return ValidationError{Path: string(path.Path("id")), Message: "case id must not be empty"}
	}
	variables, err := validateCaseVariables(cs.Variables, path.Path("variables"))
	if err != nil {
		return err
	}
	if err := validateCaseConfiguration(cs, path); err != nil {
		return err
	}
	return validateCaseOutput(cs, variables, path)
}

func validateCaseConfiguration(cs *CaseClause, path validationPath) error {
	if cs.NormalizeCommand != nil {
		if err := validateCommand(cs.NormalizeCommand, path.Path("normalize_command")); err != nil {
			return err
		}
	}
	if cs.WhenArguments != nil {
		if err := validateWhenArguments(cs.WhenArguments, path.Path("when_arguments")); err != nil {
			return err
		}
	}
	if cs.Passthrough && cs.CompressOutput != nil {
		return ValidationError{Path: string(path.Path("compress_output")), Message: "passthrough case must not define compress_output"}
	}
	if cs.Finally != nil {
		if err := validateOnExit(cs.Finally, path.Path("finally")); err != nil {
			return err
		}
	}
	return nil
}

func validateCaseOutput(cs *CaseClause, variables map[string]struct{}, path validationPath) error {
	if cs.CompressOutput != nil {
		if err := validateOutput(cs.CompressOutput, path.Path("compress_output")); err != nil {
			return err
		}
	}
	if cs.CompressOutput != nil {
		if err := validateReplaceActions(cs.CompressOutput, variables, path.Path("compress_output")); err != nil {
			return err
		}
	}
	return nil
}

func validateCaseVariables(variables []Variable, path validationPath) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(variables))
	for i, variable := range variables {
		itemPath := indexedValidationPath(path, i)
		if variable.Name == "" {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameEmptyMessage}
		}
		if variable.Type != "number" && variable.Type != "string" {
			return nil, ValidationError{Path: string(itemPath.Path("type")), Message: variableTypeInvalidMessage}
		}
		if variable.RegexGroup != "" {
			return nil, ValidationError{Path: string(itemPath.Path("regex_group")), Message: "case variables must not define regex_group"}
		}
		if _, ok := seen[variable.Name]; ok {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameUniqueMessage}
		}
		seen[variable.Name] = struct{}{}
	}
	return seen, nil
}

func validateGroupVariables(variables []Variable, path validationPath, regex string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(variables))
	compiled, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	for i, variable := range variables {
		itemPath := indexedValidationPath(path, i)
		if variable.Name == "" {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameEmptyMessage}
		}
		if variable.Type != "number" && variable.Type != "string" {
			return nil, ValidationError{Path: string(itemPath.Path("type")), Message: variableTypeInvalidMessage}
		}
		if variable.RegexGroup != "" && !regexpHasNamedCapture(compiled, variable.RegexGroup) {
			return nil, ValidationError{Path: string(itemPath.Path("regex_group")), Message: "regex_group must reference a named capture from matches_regex"}
		}
		if variable.Type == "number" && variable.RegexGroup != "" {
			return nil, ValidationError{Path: string(itemPath.Path("regex_group")), Message: "number variables must not define regex_group"}
		}
		if _, ok := seen[variable.Name]; ok {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameUniqueMessage}
		}
		seen[variable.Name] = struct{}{}
	}
	return seen, nil
}

func validateCommand(cmd *CommandMutation, path validationPath) error {
	if len(cmd.AppendIfMissing) == 0 && len(cmd.AppendIfNoPositionals) == 0 && len(cmd.AddShortFlags) == 0 {
		return ValidationError{Path: string(path), Message: "command block must define append_if_missing, append_if_no_positionals or add_short_flags"}
	}
	return nil
}

func validateWhenArguments(wa *WhenArguments, path validationPath) error {
	if len(wa.FirstIs) == 0 && len(wa.FirstIn) == 0 && len(wa.HaveAny) == 0 &&
		len(wa.LackAny) == 0 && len(wa.HaveSequence) == 0 && len(wa.HaveShortFlag) == 0 &&
		len(wa.NotHaveShortFlag) == 0 && len(wa.HaveAllShortFlags) == 0 &&
		len(wa.NotHaveAllShortFlags) == 0 &&
		len(wa.PositionalsLackAny) == 0 && !wa.NoPositionals {
		return ValidationError{Path: string(path), Message: "when_arguments must set at least one predicate"}
	}
	return nil
}

func validateOutput(out *OutputShape, path validationPath) error {
	if out == nil {
		return nil
	}
	scopeCount := 0
	for _, scope := range []struct {
		name  string
		value *OutputScope
	}{
		{name: string(contracts.StreamCombined), value: out.Combined},
		{name: string(contracts.StreamStdout), value: out.Stdout},
		{name: string(contracts.StreamStderr), value: out.Stderr},
	} {
		if scope.value == nil {
			continue
		}
		scopeCount++
		if err := validateOutputScope(scope.value, path.Path(scope.name)); err != nil {
			return err
		}
	}
	if scopeCount == 0 {
		return ValidationError{Path: string(path), Message: "output must define at least one scope"}
	}
	if out.Combined != nil && (out.Stdout != nil || out.Stderr != nil) {
		return ValidationError{Path: string(path), Message: "output.combined must not be mixed with stream-specific scopes"}
	}
	return nil
}

func validateGroup(g *OutputGroup, path validationPath) error {
	if g.ID == "" {
		return ValidationError{Path: string(path.Path("id")), Message: "group id must not be empty"}
	}

	hasBoundaryFields := g.StartsWith != "" || g.StartsRegex != ""
	hasCollectFields := g.MatchesRegex != "" || g.GroupBy != ""
	if hasBoundaryFields && hasCollectFields {
		return ValidationError{Path: string(path), Message: "group must not mix boundary grouping with collect grouping"}
	}
	if hasBoundaryFields {
		return validateBoundaryGroup(g, path)
	}
	if hasCollectFields {
		return validateCollectGroup(g, path)
	}
	if len(g.Variables) > 0 || g.Initially != nil || g.Lines != nil || g.Finally != nil {
		return ValidationError{Path: string(path), Message: "group must define starts_with, starts_with_regex, or matches_regex"}
	}
	return ValidationError{Path: string(path), Message: "group must define starts_with, starts_with_regex, or matches_regex"}
}

func validateCollectGroup(g *OutputGroup, path validationPath) error {
	if g.MatchesRegex == "" {
		return ValidationError{Path: string(path.Path("matches_regex")), Message: "collect group must define matches_regex"}
	}
	if g.GroupBy == "" {
		return ValidationError{Path: string(path.Path("group_by")), Message: "collect group must define group_by"}
	}
	declared, err := validateGroupVariables(g.Variables, path.Path("variables"), g.MatchesRegex)
	if err != nil {
		return err
	}
	if err := validateTemplateVariables(g.GroupBy, declared, path.Path("group_by")); err != nil {
		return err
	}
	if err := validateGroupLifecycle(g, declared, path); err != nil {
		return err
	}
	if g.Initially == nil && g.Lines == nil && g.Finally == nil {
		return ValidationError{Path: string(path), Message: "collect group must define initially, lines, or finally"}
	}
	return nil
}

func validateGroupLifecycle(g *OutputGroup, declared map[string]struct{}, path validationPath) error {
	if err := validateGroupStage(g.Initially, declared, path.Path("initially")); err != nil {
		return err
	}
	if g.Lines != nil {
		if err := validateLines(g.Lines, path.Path("lines"), linesValidationMode{}); err != nil {
			return err
		}
		if err := validateLineTemplates(g.Lines, declared, path.Path("lines")); err != nil {
			return err
		}
	}
	return validateGroupStage(g.Finally, declared, path.Path("finally"))
}

func validateGroupStage(stage *OnExit, declared map[string]struct{}, path validationPath) error {
	if stage == nil {
		return nil
	}
	if err := validateOnExit(stage, path); err != nil {
		return err
	}
	return validateTemplateVariables(stage.Print, declared, path.Path("print"))
}

func validateBoundaryGroup(g *OutputGroup, path validationPath) error {
	if g.StartsWith != "" && g.StartsRegex != "" {
		return ValidationError{Path: string(path), Message: "group must set only one of starts_with or starts_with_regex"}
	}
	declared, err := validateBoundaryGroupVariables(g.Variables, path.Path("variables"), g.StartsRegex)
	if err != nil {
		return err
	}
	if err := validateGroupLifecycle(g, declared, path); err != nil {
		return err
	}
	if g.Initially == nil && g.Lines == nil && g.Finally == nil {
		return ValidationError{Path: string(path), Message: "boundary group must define initially, lines, or finally"}
	}
	return nil
}

func validateBoundaryGroupVariables(variables []Variable, path validationPath, startsRegex string) (map[string]struct{}, error) {
	if startsRegex != "" {
		return validateGroupVariables(variables, path, startsRegex)
	}
	return validateBoundaryGroupVariablesWithoutRegex(variables, path)
}

func validateBoundaryGroupVariablesWithoutRegex(variables []Variable, path validationPath) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(variables))
	for i, variable := range variables {
		itemPath := indexedValidationPath(path, i)
		if variable.Name == "" {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameEmptyMessage}
		}
		if variable.Type != "number" && variable.Type != "string" {
			return nil, ValidationError{Path: string(itemPath.Path("type")), Message: variableTypeInvalidMessage}
		}
		if variable.RegexGroup != "" {
			return nil, ValidationError{Path: string(itemPath.Path("regex_group")), Message: "boundary groups without starts_with_regex must not define regex_group"}
		}
		if _, ok := seen[variable.Name]; ok {
			return nil, ValidationError{Path: string(itemPath.Path("name")), Message: variableNameUniqueMessage}
		}
		seen[variable.Name] = struct{}{}
	}
	return seen, nil
}

var templateVariablePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+)\}\}`)

func validateTemplateVariables(template string, declared map[string]struct{}, path validationPath) error {
	for _, match := range templateVariablePattern.FindAllStringSubmatch(template, -1) {
		if len(match) != 2 {
			continue
		}
		if _, ok := declared[match[1]]; ok {
			continue
		}
		return ValidationError{Path: string(path), Message: fmt.Sprintf("template references undeclared variable %q", match[1])}
	}
	return nil
}

func validateLineTemplates(lines *OutputLines, declared map[string]struct{}, path validationPath) error {
	for i := range lines.Replace {
		rule := &lines.Replace[i]
		if rule.To == nil {
			continue
		}
		if err := validateTemplateVariables(*rule.To, declared, path.Path(fmt.Sprintf("replace[%d].to", i))); err != nil {
			return err
		}
	}
	return nil
}

func validateOutputScope(scope *OutputScope, path validationPath) error {
	if scope == nil {
		return nil
	}
	if scope.Lines != nil {
		if err := validateLines(scope.Lines, path.Path("lines"), linesValidationMode{
			allowGroupsSummary: scopeHasCollectGroups(scope.Groups),
		}); err != nil {
			return err
		}
	}
	for i := range scope.Groups {
		if err := validateGroup(&scope.Groups[i], path.Path(fmt.Sprintf("groups[%d]", i))); err != nil {
			return err
		}
	}
	if scope.Lines == nil && len(scope.Groups) == 0 {
		return ValidationError{Path: string(path), Message: "output scope must define lines or groups"}
	}
	return nil
}

type linesValidationMode struct {
	allowGroupsSummary bool
}

func validateLines(lines *OutputLines, path validationPath, mode linesValidationMode) error {
	if lines == nil {
		return nil
	}
	if err := validateReplaceRules(lines.Replace, path.Path("replace")); err != nil {
		return err
	}
	if err := validateSkipOrKeepRules(lines.Skip, path.Path("skip")); err != nil {
		return err
	}
	if err := validateSkipOrKeepRules(lines.Keep, path.Path("keep")); err != nil {
		return err
	}
	if err := validateLinesMax(lines.Max, path.Path("max"), mode); err != nil {
		return err
	}
	return nil
}

func validateReplaceRules(rules []ReplaceRule, path validationPath) error {
	for i := range rules {
		if err := validateReplaceRule(&rules[i], indexedValidationPath(path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateSkipOrKeepRules(rules []SkipOrKeepRule, path validationPath) error {
	for i := range rules {
		if err := validateSkipOrKeepRule(&rules[i], indexedValidationPath(path, i)); err != nil {
			return err
		}
	}
	return nil
}

func indexedValidationPath(path validationPath, index int) validationPath {
	return validationPath(fmt.Sprintf(indexedPathFormat, path, index))
}

func validateLinesMax(rule *MaxRule, path validationPath, mode linesValidationMode) error {
	if rule == nil {
		return nil
	}
	if rule.Count <= 0 {
		return ValidationError{Path: string(path.Path("count")), Message: "max.count must be positive"}
	}
	if rule.GroupsSummary != nil {
		if !mode.allowGroupsSummary {
			return ValidationError{Path: string(path.Path("groups_summary")), Message: "max.groups_summary requires collect groups in the same output scope"}
		}
		if err := validateMaxGroupsSummary(rule.GroupsSummary, path.Path("groups_summary")); err != nil {
			return err
		}
	}
	if rule.Print == "" {
		return nil
	}
	return validateMaxPrint(rule.Print, path.Path("print"), rule.GroupsSummary != nil)
}

func validateReplaceRule(rule *ReplaceRule, path validationPath) error {
	if matcherCount(rule.Regex, rule.StartsWith, rule.Contains, rule.EndsWith) != 1 {
		return ValidationError{Path: string(path), Message: "replace rule must set exactly one of regex, starts_with, contains, or ends_with"}
	}
	if rule.To == nil {
		return ValidationError{Path: string(path.Path("to")), Message: "replace rule must define to"}
	}
	for i := range rule.OnMatch {
		action := rule.OnMatch[i]
		actionPath := path.Path(fmt.Sprintf("on_match[%d]", i))
		if action.Variable == "" {
			return ValidationError{Path: string(actionPath.Path("variable")), Message: "match action variable must not be empty"}
		}
		if action.Increment == nil {
			return ValidationError{Path: string(actionPath.Path("increment")), Message: "match action must define increment"}
		}
	}
	return nil
}

func validateSkipOrKeepRule(rule *SkipOrKeepRule, path validationPath) error {
	if matcherCount(rule.Regex, rule.StartsWith, rule.Contains, rule.EndsWith) != 1 {
		return ValidationError{Path: string(path), Message: "skip/keep rule must set exactly one of regex, starts_with, contains, or ends_with"}
	}
	return nil
}

func validateMaxPrint(template string, path validationPath, allowGroupsSummary bool) error {
	allowed := []string{"{{value}}"}
	if allowGroupsSummary {
		allowed = append(allowed, "{{groups_summary}}")
	}
	for _, match := range templateVariablePattern.FindAllStringSubmatch(template, -1) {
		if len(match) != 2 {
			continue
		}
		if match[1] == "value" {
			continue
		}
		if allowGroupsSummary && match[1] == "groups_summary" {
			continue
		}
		return ValidationError{Path: string(path), Message: fmt.Sprintf("max.print must reference only %q, got %q", strings.Join(allowed, ", "), match[1])}
	}
	return nil
}

func validateMaxGroupsSummary(summary *MaxGroupsSummary, path validationPath) error {
	if summary == nil {
		return nil
	}
	if summary.Show <= 0 {
		return ValidationError{Path: string(path.Path("show")), Message: "groups_summary.show must be positive"}
	}
	if summary.Print == "" {
		return ValidationError{Path: string(path.Path("print")), Message: "groups_summary.print must not be empty"}
	}
	if err := validateTemplateVariables(summary.Print, map[string]struct{}{
		"key":   {},
		"count": {},
	}, path.Path("print")); err != nil {
		return err
	}
	if err := validateTemplateVariables(summary.Delimiter, map[string]struct{}{}, path.Path("delimiter")); err != nil {
		return err
	}
	if err := validateTemplateVariables(summary.Prefix, map[string]struct{}{}, path.Path("prefix")); err != nil {
		return err
	}
	if err := validateTemplateVariables(summary.Suffix, map[string]struct{}{
		"remaining": {},
	}, path.Path("suffix")); err != nil {
		return err
	}
	return nil
}

func scopeHasCollectGroups(groups []OutputGroup) bool {
	for _, group := range groups {
		if group.MatchesRegex != "" && group.GroupBy != "" {
			return true
		}
	}
	return false
}

func matcherCount(values ...string) int {
	count := 0
	for _, value := range values {
		if value != "" {
			count++
		}
	}
	return count
}

func validateOnExit(onExit *OnExit, path validationPath) error {
	if onExit.Print == "" {
		return ValidationError{Path: string(path), Message: "finally must define print"}
	}
	return nil
}

func validateReplaceActions(out *OutputShape, variables map[string]struct{}, path validationPath) error {
	for _, scope := range []struct {
		name  string
		value *OutputScope
	}{
		{name: string(contracts.StreamCombined), value: out.Combined},
		{name: string(contracts.StreamStdout), value: out.Stdout},
		{name: string(contracts.StreamStderr), value: out.Stderr},
	} {
		if scope.value == nil || scope.value.Lines == nil {
			continue
		}
		for i := range scope.value.Lines.Replace {
			for j := range scope.value.Lines.Replace[i].OnMatch {
				action := scope.value.Lines.Replace[i].OnMatch[j]
				if _, ok := variables[action.Variable]; !ok {
					return ValidationError{
						Path:    string(path.Path(scope.name).Path("lines").Path(fmt.Sprintf("replace[%d]", i)).Path(fmt.Sprintf("on_match[%d]", j)).Path("variable")),
						Message: "match action variable must reference a declared variable",
					}
				}
			}
		}
	}
	return nil
}

type validationPath string

func casePath(index int) validationPath {
	return validationPath(fmt.Sprintf("cases[%d]", index))
}

func (p validationPath) Path(part string) validationPath {
	if p == "" {
		return validationPath(part)
	}
	if part == "" {
		return p
	}
	return validationPath(string(p) + "." + part)
}
