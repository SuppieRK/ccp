package yaml

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/filters/operations"
)

type YamlFilter struct {
	spec                  *FilterDefinition
	flagsConsumingNextArg []string
	cases                 []compiledCase
	provenance            contracts.FilterProvenance

	activeArgs string
}

func (f *YamlFilter) CloneFilter() contracts.Filter {
	if f == nil {
		return nil
	}
	return &YamlFilter{
		spec:                  f.spec,
		flagsConsumingNextArg: cloneStrings(f.flagsConsumingNextArg),
		cases:                 cloneCompiledCases(f.cases),
		provenance:            f.provenance,
	}
}

func NewFilter(spec *FilterDefinition) (*YamlFilter, error) {
	if err := ValidateDefinition(spec); err != nil {
		return nil, err
	}

	cases := make([]compiledCase, len(spec.Cases))
	for i := range spec.Cases {
		current, err := buildCompiledCase(&spec.Cases[i])
		if err != nil {
			return nil, fmt.Errorf("compile case %q: %w", spec.Cases[i].ID, err)
		}
		cases[i] = current
	}
	return &YamlFilter{
		spec:                  spec,
		flagsConsumingNextArg: cloneStrings(spec.FlagsConsumingNextArg),
		cases:                 cases,
	}, nil
}

func (f *YamlFilter) WithProvenance(provenance contracts.FilterProvenance) *YamlFilter {
	if f == nil {
		return nil
	}
	f.provenance = provenance
	return f
}

func (f *YamlFilter) FilterProvenance() contracts.FilterProvenance {
	if f == nil {
		return contracts.FilterProvenance{}
	}
	return f.provenance
}

func (f *YamlFilter) OnStdout(line string, context contracts.Context) contracts.Action {
	f.prepareInvocation(context.Args())
	return f.onStream(contracts.StreamStdout, line, context)
}

func (f *YamlFilter) OnStderr(line string, context contracts.Context) contracts.Action {
	f.prepareInvocation(context.Args())
	return f.onStream(contracts.StreamStderr, line, context)
}

func (f *YamlFilter) OnStdoutExit(context contracts.Context) contracts.Action {
	actions := f.OnStdoutExitActions(context)
	if len(actions) == 0 {
		return contracts.Action{Kind: contracts.ActionKeep}
	}
	return actions[0]
}

func (f *YamlFilter) OnStdoutExitActions(context contracts.Context) []contracts.Action {
	f.prepareInvocation(context.Args())
	cs, ok := f.caseForArgs(context.Args())
	if !ok || cs.passthrough {
		return []contracts.Action{{Kind: contracts.ActionKeep}}
	}
	switch {
	case cs.shared != nil:
		return []contracts.Action{renderScopedExitAction(context, contracts.StreamCombined, cs.shared, cs, true)}
	case cs.stdout == nil && cs.stderr == nil:
		return []contracts.Action{renderUnscopedExitAction(context, cs)}
	default:
		return renderStreamExitActions(context, cs)
	}
}

func renderStreamExitActions(context contracts.Context, cs *compiledCase) []contracts.Action {
	actions := make([]contracts.Action, 0, 2)
	if cs.stdout != nil {
		actions = append(actions, renderScopedExitAction(context, contracts.StreamStdout, cs.stdout, cs, true))
	}
	if cs.stderr != nil {
		actions = append(actions, renderScopedExitAction(context, contracts.StreamStderr, cs.stderr, cs, cs.stdout == nil))
	}
	return actions
}

func renderScopedExitAction(context contracts.Context, stream contracts.Stream, scope *compiledScope, cs *compiledCase, includeFinally bool) contracts.Action {
	output := renderStdoutExitOutput(strings.Join(context.BufferedLines(stream), ""), scope, context.ExitCode())
	if includeFinally && cs.onExit != nil {
		output = appendCaseExitPrint(output, cs.onExit, cs.variables, context.ExitCode())
	}
	return exitActionForOutput(output, stream)
}

func renderUnscopedExitAction(context contracts.Context, cs *compiledCase) contracts.Action {
	output := strings.Join(context.BufferedLines(contracts.StreamStdout), "")
	if cs.onExit != nil {
		output = appendCaseExitPrint(output, cs.onExit, cs.variables, context.ExitCode())
	}
	return exitActionForOutput(output, contracts.StreamStdout)
}

func renderStdoutExitOutput(output string, scope *compiledScope, exitCode int) string {
	if renderedGroups := renderScopeGroups(scope, exitCode); len(renderedGroups) > 0 {
		return applyRenderedMax(renderedGroups, scope.max)
	}
	return appendScopeMaxOverflow(output, scope)
}

func appendScopeMaxOverflow(output string, scope *compiledScope) string {
	overflow := renderScopeMaxOverflow(scope)
	if overflow == "" {
		return output
	}
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return output + overflow
}

func appendCaseExitPrint(output string, onExit *compiledOnExit, variables map[string]string, exitCode int) string {
	if !shouldRenderFinally(exitCode) {
		return output
	}
	printed := renderExitPrint(onExit.print, variables)
	if printed == "" {
		return output
	}
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	output += printed
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return output
}

func exitActionForOutput(output string, stream contracts.Stream) contracts.Action {
	if output == "" {
		return contracts.Action{Kind: contracts.ActionKeep}
	}
	action := contracts.Action{Kind: contracts.ActionReplace, Output: output}
	action.Stream = stream
	return action
}

func (f *YamlFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	cs, ok := f.caseForArgs(command.Args)
	if !ok || cs.command == nil {
		return command, nil
	}

	mutated := command
	mutated.Args = applyCommandMutations(command.Args, cs.when, f.flagsConsumingNextArg, cs.command)
	return mutated, nil
}

func (f *YamlFilter) Dispatch(command contracts.Command) string {
	cs, ok := f.caseForArgs(command.ArgsForMatching())
	if !ok {
		return f.spec.Filter
	}
	if cs.id == "" {
		return f.spec.Filter
	}
	return f.spec.Filter + "|" + cs.id
}

func (f *YamlFilter) ReportsPassthrough(command contracts.Command) bool {
	cs, ok := f.caseForArgs(command.ArgsForMatching())
	return ok && cs.passthrough
}

type compiledCase struct {
	id          string
	passthrough bool
	when        compiledWhen
	variables   map[string]string
	initials    map[string]string
	command     *compiledCommand
	stdout      *compiledScope
	stderr      *compiledScope
	shared      *compiledScope
	onExit      *compiledOnExit
}

type compiledWhen struct {
	firstIs              string
	firstIn              []string
	haveAny              []string
	lackAny              []string
	haveSequence         []string
	haveShortFlag        []string
	notHaveShortFlag     []string
	haveAllShortFlags    []string
	notHaveAllShortFlags []string
	positionalsLackAny   []string
	noPositionals        bool
}

type compiledCommand struct {
	appendIfMissing       []string
	appendIfNoPositionals []string
	addShortFlags         []string
}

type compiledScope struct {
	replace        []compiledReplace
	keep           []compiledMatcher
	skip           []compiledMatcher
	max            *compiledMax
	hidden         int
	groups         []compiledGroup
	activeBoundary *activeBoundaryGroup
}

type compiledReplace struct {
	regex       *regexp.Regexp
	startsWith  string
	contains    string
	endsWith    string
	replacement string
	onMatch     []compiledMatchAction
}

type compiledMatcher struct {
	regex      *regexp.Regexp
	startsWith string
	contains   string
	endsWith   string
}

type compiledMax struct {
	count         int
	print         string
	groupsSummary *compiledMaxGroupsSummary
}

type compiledMaxGroupsSummary struct {
	show      int
	print     string
	delimiter string
	prefix    string
	suffix    string
}

type compiledMatchAction struct {
	variable  string
	increment int
}

type compiledOnExit struct {
	print string
}

type compiledVariable struct {
	name         string
	kind         string
	initialValue string
	regexGroup   string
	defaultValue string
}

type compiledGroupItem struct {
	line string
	vars map[string]string
}

type renderedLine struct {
	text      string
	groupKey  string
	groupItem bool
}

type omittedGroupSummary struct {
	key   string
	count int
}

type compiledBoundarySection struct {
	vars  map[string]string
	items []compiledGroupItem
}

type compiledGroupMode int

const (
	groupModeCollect compiledGroupMode = iota
	groupModeBoundary
)

type activeBoundaryGroup struct {
	groupIndex   int
	sectionIndex int
}

type compiledGroup struct {
	mode        compiledGroupMode
	regex       *regexp.Regexp
	startsWith  string
	startsRegex *regexp.Regexp
	variables   []compiledVariable
	groupBy     string
	initially   *compiledOnExit
	lines       *compiledScope
	finally     *compiledOnExit
	items       map[string][]compiledGroupItem
	sections    []compiledBoundarySection
}

func buildCompiledCase(cs *CaseClause) (compiledCase, error) {
	variables, initials := compileVariables(cs.Variables)
	compiled := compiledCase{
		id:          cs.ID,
		passthrough: cs.Passthrough,
		when:        compileWhenArguments(cs.WhenArguments),
		variables:   variables,
		initials:    initials,
		command:     compileCommandMutation(cs.NormalizeCommand),
		onExit:      compileOnExit(cs.Finally),
	}
	for _, current := range []struct {
		name   string
		source *OutputScope
		assign func(*compiledScope)
	}{
		{name: "combined", source: outputCombined(cs.CompressOutput), assign: func(scope *compiledScope) { compiled.shared = scope }},
		{name: "stdout", source: outputStdout(cs.CompressOutput), assign: func(scope *compiledScope) { compiled.stdout = scope }},
		{name: "stderr", source: outputStderr(cs.CompressOutput), assign: func(scope *compiledScope) { compiled.stderr = scope }},
	} {
		scope, err := buildCompiledScope(current.name, current.source)
		if err != nil {
			return compiledCase{}, err
		}
		current.assign(scope)
	}
	return compiled, nil
}

func compileVariables(variables []Variable) (map[string]string, map[string]string) {
	if len(variables) == 0 {
		return nil, nil
	}
	compiled := make(map[string]string, len(variables))
	initials := make(map[string]string, len(variables))
	for _, variable := range variables {
		value := ""
		switch variable.Type {
		case "number":
			if variable.InitialValue != nil {
				value = *variable.InitialValue
			} else {
				value = "0"
			}
		default:
			if variable.InitialValue != nil {
				value = *variable.InitialValue
			} else {
				value = ""
			}
		}
		compiled[variable.Name] = value
		initials[variable.Name] = value
	}
	return compiled, initials
}

func compileOnExit(onExit *OnExit) *compiledOnExit {
	if onExit == nil {
		return nil
	}
	return &compiledOnExit{print: onExit.Print}
}

func cloneCompiledCases(cases []compiledCase) []compiledCase {
	if len(cases) == 0 {
		return nil
	}
	cloned := make([]compiledCase, len(cases))
	for i := range cases {
		cloned[i] = cloneCompiledCase(cases[i])
	}
	return cloned
}

func cloneCompiledCase(src compiledCase) compiledCase {
	return compiledCase{
		id:          src.id,
		passthrough: src.passthrough,
		when:        src.when,
		variables:   cloneVariableValues(src.variables),
		initials:    cloneVariableValues(src.initials),
		command:     src.command,
		stdout:      cloneCompiledScope(src.stdout),
		stderr:      cloneCompiledScope(src.stderr),
		shared:      cloneCompiledScope(src.shared),
		onExit:      src.onExit,
	}
}

func cloneVariableValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	maps.Copy(cloned, values)
	return cloned
}

func cloneCompiledScope(src *compiledScope) *compiledScope {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.hidden = 0
	cloned.activeBoundary = nil
	if len(src.groups) > 0 {
		cloned.groups = make([]compiledGroup, len(src.groups))
		for i := range src.groups {
			cloned.groups[i] = cloneCompiledGroup(src.groups[i])
		}
	}
	return &cloned
}

func cloneCompiledGroup(src compiledGroup) compiledGroup {
	cloned := src
	cloned.items = make(map[string][]compiledGroupItem)
	cloned.sections = nil
	cloned.lines = cloneCompiledScope(src.lines)
	return cloned
}

func compileCommandMutation(command *CommandMutation) *compiledCommand {
	if command == nil {
		return nil
	}
	return &compiledCommand{
		appendIfMissing:       cloneStrings(command.AppendIfMissing),
		appendIfNoPositionals: cloneStrings(command.AppendIfNoPositionals),
		addShortFlags:         cloneStrings(command.AddShortFlags),
	}
}

func compileWhenArguments(when *WhenArguments) compiledWhen {
	if when == nil {
		return compiledWhen{}
	}
	return compiledWhen{
		firstIs:              when.FirstIs,
		firstIn:              cloneStrings(when.FirstIn),
		haveAny:              cloneStrings(when.HaveAny),
		lackAny:              cloneStrings(when.LackAny),
		haveSequence:         cloneStrings(when.HaveSequence),
		haveShortFlag:        cloneStrings(when.HaveShortFlag),
		notHaveShortFlag:     cloneStrings(when.NotHaveShortFlag),
		haveAllShortFlags:    cloneStrings(when.HaveAllShortFlags),
		notHaveAllShortFlags: cloneStrings(when.NotHaveAllShortFlags),
		positionalsLackAny:   cloneStrings(when.PositionalsLackAny),
		noPositionals:        when.NoPositionals,
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func buildCompiledScope(name string, scope *OutputScope) (*compiledScope, error) {
	if scope == nil {
		return nil, nil
	}
	compiled, err := buildCompiledLines(name, scopeLines(scope))
	if err != nil {
		return nil, err
	}
	groups, err := buildCompiledGroups(name, scope.Groups)
	if err != nil {
		return nil, err
	}
	compiled.groups = groups
	return compiled, nil
}

func buildCompiledLines(name string, lines *OutputLines) (*compiledScope, error) {
	if lines == nil {
		return &compiledScope{}, nil
	}

	replace, err := compileReplaceRules(lines.Replace)
	if err != nil {
		return nil, fmt.Errorf("scope %q lines.replace: %w", name, err)
	}
	keep, err := compileMatchers(lines.Keep)
	if err != nil {
		return nil, fmt.Errorf("scope %q lines.keep: %w", name, err)
	}
	skip, err := compileMatchers(lines.Skip)
	if err != nil {
		return nil, fmt.Errorf("scope %q lines.skip: %w", name, err)
	}
	return &compiledScope{
		replace: replace,
		keep:    keep,
		skip:    skip,
		max:     compileMax(lines.Max),
	}, nil
}

func compileMax(rule *MaxRule) *compiledMax {
	if rule == nil {
		return nil
	}
	var groupsSummary *compiledMaxGroupsSummary
	if rule.GroupsSummary != nil {
		groupsSummary = &compiledMaxGroupsSummary{
			show:      rule.GroupsSummary.Show,
			print:     rule.GroupsSummary.Print,
			delimiter: rule.GroupsSummary.Delimiter,
			prefix:    rule.GroupsSummary.Prefix,
			suffix:    rule.GroupsSummary.Suffix,
		}
	}
	return &compiledMax{
		count:         rule.Count,
		print:         rule.Print,
		groupsSummary: groupsSummary,
	}
}

func compileReplaceRules(rules []ReplaceRule) ([]compiledReplace, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	compiled := make([]compiledReplace, 0, len(rules))
	for _, rule := range rules {
		current := compiledReplace{replacement: *rule.To}
		if rule.Regex != "" {
			re, err := regexp.Compile(rule.Regex)
			if err != nil {
				return nil, err
			}
			current.regex = re
		}
		current.startsWith = rule.StartsWith
		current.contains = rule.Contains
		current.endsWith = rule.EndsWith
		current.onMatch = compileMatchActions(rule.OnMatch)
		compiled = append(compiled, current)
	}
	return compiled, nil
}

func compileMatchActions(actions []MatchAction) []compiledMatchAction {
	if len(actions) == 0 {
		return nil
	}
	compiled := make([]compiledMatchAction, 0, len(actions))
	for _, action := range actions {
		compiled = append(compiled, compiledMatchAction{
			variable:  action.Variable,
			increment: *action.Increment,
		})
	}
	return compiled
}

func compileMatchers(rules []SkipOrKeepRule) ([]compiledMatcher, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	compiled := make([]compiledMatcher, 0, len(rules))
	for _, rule := range rules {
		current := compiledMatcher{
			startsWith: rule.StartsWith,
			contains:   rule.Contains,
			endsWith:   rule.EndsWith,
		}
		if rule.Regex != "" {
			re, err := regexp.Compile(rule.Regex)
			if err != nil {
				return nil, err
			}
			current.regex = re
		}
		compiled = append(compiled, current)
	}
	return compiled, nil
}

func buildCompiledGroups(name string, groups []OutputGroup) ([]compiledGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	compiled := make([]compiledGroup, 0, len(groups))
	for _, group := range groups {
		if group.MatchesRegex != "" {
			current, err := compileCollectedGroup(name, group)
			if err != nil {
				return nil, err
			}
			compiled = append(compiled, current)
			continue
		}
		current, err := compileBoundaryGroup(name, group)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, current)
	}
	return compiled, nil
}

func compileCollectedGroup(name string, group OutputGroup) (compiledGroup, error) {
	re, err := regexp.Compile(group.MatchesRegex)
	if err != nil {
		return compiledGroup{}, fmt.Errorf("scope %q group %q matches_regex: %w", name, group.ID, err)
	}
	vars, err := compileGroupVariables(group.Variables)
	if err != nil {
		return compiledGroup{}, fmt.Errorf("scope %q group %q variables: %w", name, group.ID, err)
	}
	for _, variable := range vars {
		if variable.regexGroup != "" && !regexpHasNamedCapture(re, variable.regexGroup) {
			return compiledGroup{}, fmt.Errorf("scope %q group %q variable %q regex_group must reference a named capture from matches_regex", name, group.ID, variable.name)
		}
	}
	lines, err := buildCompiledLines(name+" group "+group.ID, group.Lines)
	if err != nil {
		return compiledGroup{}, err
	}
	return compiledGroup{
		mode:      groupModeCollect,
		regex:     re,
		variables: vars,
		groupBy:   group.GroupBy,
		initially: compileOnExit(group.Initially),
		lines:     lines,
		finally:   compileOnExit(group.Finally),
		items:     map[string][]compiledGroupItem{},
	}, nil
}

func compileBoundaryGroup(name string, group OutputGroup) (compiledGroup, error) {
	var startsRegex *regexp.Regexp
	if group.StartsRegex != "" {
		compiled, err := regexp.Compile(group.StartsRegex)
		if err != nil {
			return compiledGroup{}, fmt.Errorf("scope %q group %q starts_with_regex: %w", name, group.ID, err)
		}
		startsRegex = compiled
	}
	vars, err := compileGroupVariables(group.Variables)
	if err != nil {
		return compiledGroup{}, fmt.Errorf("scope %q group %q variables: %w", name, group.ID, err)
	}
	for _, variable := range vars {
		if variable.regexGroup == "" {
			continue
		}
		if startsRegex == nil {
			return compiledGroup{}, fmt.Errorf("scope %q group %q variable %q regex_group requires starts_with_regex", name, group.ID, variable.name)
		}
		if !regexpHasNamedCapture(startsRegex, variable.regexGroup) {
			return compiledGroup{}, fmt.Errorf("scope %q group %q variable %q regex_group must reference a named capture from starts_with_regex", name, group.ID, variable.name)
		}
	}
	lines, err := buildCompiledLines(name+" group "+group.ID, group.Lines)
	if err != nil {
		return compiledGroup{}, err
	}
	return compiledGroup{
		mode:        groupModeBoundary,
		startsWith:  group.StartsWith,
		startsRegex: startsRegex,
		variables:   vars,
		initially:   compileOnExit(group.Initially),
		lines:       lines,
		finally:     compileOnExit(group.Finally),
	}, nil
}

func compileGroupVariables(variables []Variable) ([]compiledVariable, error) {
	if len(variables) == 0 {
		return nil, nil
	}
	compiled := make([]compiledVariable, 0, len(variables))
	for _, variable := range variables {
		initialValue := ""
		if variable.InitialValue != nil {
			initialValue = *variable.InitialValue
		}
		compiled = append(compiled, compiledVariable{
			name:         variable.Name,
			kind:         variable.Type,
			initialValue: initialValue,
			regexGroup:   variable.RegexGroup,
			defaultValue: variable.DefaultValue,
		})
	}
	return compiled, nil
}

func regexpHasNamedCapture(re *regexp.Regexp, name string) bool {
	return slices.Contains(re.SubexpNames(), name)
}

func (f *YamlFilter) onStream(stream contracts.Stream, line string, context contracts.Context) contracts.Action {
	cs, scope, ok := f.scopeForArgs(context.Args(), stream)
	if !ok {
		return contracts.Action{Kind: contracts.ActionEmit}
	}
	if scope.collectGroupLine(line) {
		return contracts.Action{Kind: contracts.ActionIgnore}
	}
	return scope.actionForLine(line, len(context.BufferedLines(stream)), cs.variables)
}

func (f *YamlFilter) scopeForArgs(args []string, stream contracts.Stream) (*compiledCase, *compiledScope, bool) {
	cs, ok := f.caseForArgs(args)
	if !ok || cs.passthrough {
		return nil, nil, false
	}
	scope, ok := cs.scope(stream)
	if !ok {
		return nil, nil, false
	}
	return cs, scope, true
}

func (c *compiledCase) scope(stream contracts.Stream) (*compiledScope, bool) {
	return operations.ScopeForStream(stream, c.shared, c.stdout, c.stderr)
}

func (f *YamlFilter) caseForArgs(args []string) (*compiledCase, bool) {
	filteredArgs := filterArgs(args)
	for i := range f.cases {
		if matchesWhenArguments(f.cases[i].when, f.flagsConsumingNextArg, filteredArgs) {
			return &f.cases[i], true
		}
	}
	return nil, false
}

func (f *YamlFilter) prepareInvocation(args []string) {
	key := strings.Join(args, "\x00")
	if f.activeArgs == key {
		return
	}
	f.activeArgs = key
	for i := range f.cases {
		f.cases[i].resetState()
	}
}

func (c *compiledCase) resetState() {
	maps.Copy(c.variables, c.initials)
	c.shared.resetState()
	c.stdout.resetState()
	c.stderr.resetState()
}

func (c *compiledScope) resetState() {
	if c == nil {
		return
	}
	c.hidden = 0
	c.activeBoundary = nil
	for i := range c.groups {
		c.groups[i].items = map[string][]compiledGroupItem{}
		c.groups[i].sections = nil
	}
}

func (c *compiledScope) actionForLine(line string, bufferedCount int, variables map[string]string) contracts.Action {
	if c == nil {
		return contracts.Action{Kind: contracts.ActionEmit}
	}
	content := trimLineEnding(line)
	action := c.baseActionForLine(line, content, variables)
	if action.Kind == contracts.ActionIgnore {
		return action
	}
	if c.max != nil && bufferedCount >= c.max.count {
		c.hidden++
		return contracts.Action{Kind: contracts.ActionIgnore}
	}
	return action
}

func (c *compiledScope) baseActionForLine(line, content string, variables map[string]string) contracts.Action {
	if len(c.keep) > 0 && matchesAnyMatcher(c.keep, content) {
		return contracts.Action{Kind: contracts.ActionKeep}
	}
	if replaced, ok := c.replaceLine(line, content, variables); ok {
		return contracts.Action{Kind: contracts.ActionReplace, Output: replaced, ReplaceCount: 1}
	}
	if len(c.skip) > 0 && matchesAnyMatcher(c.skip, content) {
		return contracts.Action{Kind: contracts.ActionIgnore}
	}
	if len(c.keep) > 0 {
		return contracts.Action{Kind: contracts.ActionIgnore}
	}
	return contracts.Action{Kind: contracts.ActionKeep}
}

func (c *compiledScope) collectGroupLine(line string) bool {
	if c == nil || len(c.groups) == 0 {
		return false
	}
	content := trimLineEnding(line)
	if c.startBoundaryGroup(content) {
		return true
	}
	if c.appendBoundaryLine(content) {
		return true
	}
	for i := range c.groups {
		if c.groups[i].mode == groupModeCollect && c.groups[i].collect(content) {
			return true
		}
	}
	return false
}

func (c *compiledScope) startBoundaryGroup(content string) bool {
	for i := range c.groups {
		if c.groups[i].mode != groupModeBoundary {
			continue
		}
		values, ok := c.groups[i].matchBoundaryStart(content)
		if !ok {
			continue
		}
		c.activeBoundary = &activeBoundaryGroup{
			groupIndex:   i,
			sectionIndex: len(c.groups[i].sections),
		}
		c.groups[i].sections = append(c.groups[i].sections, compiledBoundarySection{vars: values})
		return true
	}
	return false
}

func (c *compiledScope) appendBoundaryLine(content string) bool {
	if c.activeBoundary == nil {
		return false
	}
	active := c.activeBoundary
	group := &c.groups[active.groupIndex]
	section := &group.sections[active.sectionIndex]
	section.items = append(section.items, compiledGroupItem{
		line: content,
		vars: section.vars,
	})
	return true
}

func (c *compiledScope) replaceLine(line, content string, variables map[string]string) (string, bool) {
	for _, rule := range c.replace {
		replaced, ok := rule.replace(content, variables)
		if !ok {
			continue
		}
		applyMatchActions(rule.onMatch, variables)
		if strings.HasSuffix(line, "\n") {
			replaced += "\n"
		}
		return replaced, true
	}
	return "", false
}

func (r compiledReplace) replace(content string, variables map[string]string) (string, bool) {
	switch {
	case r.regex != nil:
		if !r.regex.MatchString(content) {
			return "", false
		}
		return renderTemplate(r.regex.ReplaceAllString(content, r.replacement), variables), true
	case r.startsWith != "":
		if !strings.HasPrefix(content, r.startsWith) {
			return "", false
		}
	case r.contains != "":
		if !strings.Contains(content, r.contains) {
			return "", false
		}
	case r.endsWith != "":
		if !strings.HasSuffix(content, r.endsWith) {
			return "", false
		}
	}
	return renderTemplate(r.replacement, variables), true
}

func applyMatchActions(actions []compiledMatchAction, variables map[string]string) {
	for _, action := range actions {
		current, _ := strconv.Atoi(variables[action.variable])
		variables[action.variable] = strconv.Itoa(current + action.increment)
	}
}

func (g *compiledGroup) collect(content string) bool {
	matches := g.regex.FindStringSubmatch(content)
	if matches == nil {
		return false
	}
	captures := map[string]string{}
	for i, name := range g.regex.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		captures[name] = matches[i]
	}
	values := g.resolveVariables(captures)
	key := renderTemplate(g.groupBy, values)
	g.items[key] = append(g.items[key], compiledGroupItem{line: content, vars: values})
	return true
}

func (g *compiledGroup) matchBoundaryStart(content string) (map[string]string, bool) {
	switch {
	case g.startsRegex != nil:
		matches := g.startsRegex.FindStringSubmatch(content)
		if matches == nil {
			return nil, false
		}
		captures := map[string]string{}
		for i, name := range g.startsRegex.SubexpNames() {
			if i == 0 || name == "" {
				continue
			}
			captures[name] = matches[i]
		}
		return g.resolveVariables(captures), true
	case g.startsWith != "":
		if !strings.HasPrefix(content, g.startsWith) {
			return nil, false
		}
		return g.resolveVariables(nil), true
	default:
		return nil, false
	}
}

func (g *compiledGroup) resolveVariables(captures map[string]string) map[string]string {
	values := map[string]string{}
	for _, variable := range g.variables {
		value := variable.initialValue
		if variable.regexGroup != "" {
			value = captures[variable.regexGroup]
		}
		if value == "" {
			value = variable.defaultValue
		}
		values[variable.name] = value
	}
	return values
}

func renderExitPrint(template string, variables map[string]string) string {
	return renderTemplate(template, variables)
}

func renderScopeGroups(scope *compiledScope, exitCode int) []renderedLine {
	if scope == nil || len(scope.groups) == 0 {
		return nil
	}
	rendered := make([]renderedLine, 0)
	for _, group := range scope.groups {
		rendered = append(rendered, group.render(exitCode)...)
	}
	return rendered
}

func renderScopeMaxOverflow(scope *compiledScope) string {
	if scope == nil || scope.max == nil || scope.hidden == 0 {
		return ""
	}
	return renderMaxPrint(scope.max.print, scope.hidden, "")
}

func applyRenderedMax(rendered []renderedLine, max *compiledMax) string {
	if len(rendered) == 0 {
		return ""
	}
	if max == nil {
		return renderRenderedLines(rendered)
	}
	if len(rendered) <= max.count {
		return renderRenderedLines(rendered)
	}
	visible := renderRenderedLines(rendered[:max.count])
	visible = strings.TrimRight(visible, "\n")
	hidden := len(rendered) - max.count
	groupsSummary := renderGroupsSummary(max.groupsSummary, rendered[max.count:])
	printed := renderMaxPrint(max.print, hidden, groupsSummary)
	if printed == "" {
		if visible != "" && !strings.HasSuffix(visible, "\n") {
			visible += "\n"
		}
		return visible
	}
	if visible != "" && !strings.HasPrefix(printed, "\n") {
		visible += "\n"
	}
	visible += printed
	if !strings.HasSuffix(visible, "\n") {
		visible += "\n"
	}
	return visible
}

func (g *compiledGroup) render(exitCode int) []renderedLine {
	switch g.mode {
	case groupModeBoundary:
		return g.renderBoundary(exitCode)
	default:
		return g.renderCollected(exitCode)
	}
}

func (g *compiledGroup) renderCollected(exitCode int) []renderedLine {
	if len(g.items) == 0 {
		return nil
	}
	keys := make([]string, 0, len(g.items))
	for key := range g.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rendered := make([]renderedLine, 0)
	for _, key := range keys {
		rendered = append(rendered, g.renderGroup(g.items[key], key, exitCode)...)
	}
	return rendered
}

func (g *compiledGroup) renderBoundary(exitCode int) []renderedLine {
	if len(g.sections) == 0 {
		return nil
	}
	rendered := make([]renderedLine, 0)
	for _, section := range g.sections {
		rendered = append(rendered, g.renderBoundarySection(section, exitCode)...)
	}
	return rendered
}

func (g *compiledGroup) renderGroup(groupItems []compiledGroupItem, groupKey string, exitCode int) []renderedLine {
	if len(groupItems) == 0 {
		return nil
	}
	renderedItems, emitted, hidden := g.renderGroupItems(groupItems, groupKey, true)
	if !emitted && (g.lines == nil || g.lines.max == nil || hidden == 0 || renderMaxPrint(g.lines.max.print, hidden, "") == "") {
		return nil
	}
	lines := make([]renderedLine, 0)
	lines = append(lines, renderGroupStage(g.initially, groupItems[0].vars)...)
	lines = append(lines, renderedItems...)
	if g.lines != nil && g.lines.max != nil && hidden > 0 {
		if printed := renderMaxPrint(g.lines.max.print, hidden, ""); printed != "" {
			lines = append(lines, renderedLine{text: printed})
		}
	}
	lines = append(lines, renderGroupFinalStage(g.finally, groupItems[0].vars, exitCode)...)
	return lines
}

func (g *compiledGroup) renderBoundarySection(section compiledBoundarySection, exitCode int) []renderedLine {
	renderedItems, emitted, hidden := g.renderGroupItems(section.items, "", false)
	if !emitted && (g.lines == nil || g.lines.max == nil || hidden == 0 || renderMaxPrint(g.lines.max.print, hidden, "") == "") {
		return nil
	}
	lines := make([]renderedLine, 0)
	lines = append(lines, renderGroupStage(g.initially, section.vars)...)
	lines = append(lines, renderedItems...)
	if g.lines != nil && g.lines.max != nil && hidden > 0 {
		if printed := renderMaxPrint(g.lines.max.print, hidden, ""); printed != "" {
			lines = append(lines, renderedLine{text: printed})
		}
	}
	lines = append(lines, renderGroupFinalStage(g.finally, section.vars, exitCode)...)
	return lines
}

func (g *compiledGroup) renderGroupItems(groupItems []compiledGroupItem, groupKey string, countTowardsSummary bool) ([]renderedLine, bool, int) {
	emitted := 0
	hidden := 0
	rendered := make([]renderedLine, 0, len(groupItems))
	for _, item := range groupItems {
		line, ok, limited := g.renderGroupItem(item, emitted)
		if limited {
			hidden++
			continue
		}
		if !ok {
			continue
		}
		current := renderedLine{text: line}
		if countTowardsSummary {
			current.groupKey = groupKey
			current.groupItem = true
		}
		rendered = append(rendered, current)
		emitted++
	}
	return rendered, emitted > 0, hidden
}

func (g *compiledGroup) renderGroupItem(item compiledGroupItem, emitted int) (string, bool, bool) {
	if g.lines == nil {
		return item.line, true, false
	}
	action := g.lines.baseActionForLine(item.line, trimLineEnding(item.line), item.vars)
	if action.Kind == contracts.ActionIgnore {
		return "", false, false
	}
	if g.lines.max != nil && emitted >= g.lines.max.count {
		return "", false, true
	}
	switch action.Kind {
	case contracts.ActionReplace:
		return action.Output, true, false
	case contracts.ActionKeep, contracts.ActionEmit:
		return item.line, true, false
	default:
		return "", false, false
	}
}

func renderGroupStage(stage *compiledOnExit, vars map[string]string) []renderedLine {
	if stage == nil {
		return nil
	}
	printed := renderTemplate(stage.print, vars)
	if printed == "" {
		return nil
	}
	return []renderedLine{{text: printed}}
}

func renderGroupFinalStage(stage *compiledOnExit, vars map[string]string, exitCode int) []renderedLine {
	if !shouldRenderFinally(exitCode) {
		return nil
	}
	return renderGroupStage(stage, vars)
}

func shouldRenderFinally(exitCode int) bool {
	return exitCode == 0
}

func renderRenderedLines(lines []renderedLine) string {
	if len(lines) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line.text)
		if !strings.HasSuffix(line.text, "\n") {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func renderTemplate(template string, values map[string]string) string {
	out := template
	for name, value := range values {
		out = strings.ReplaceAll(out, "{{"+name+"}}", value)
	}
	return out
}

func renderMaxPrint(template string, value int, groupsSummary string) string {
	if template == "" {
		return ""
	}
	rendered := renderTemplate(template, map[string]string{
		"value":          strconv.Itoa(value),
		"groups_summary": groupsSummary,
	})
	rendered = strings.ReplaceAll(rendered, " \n", "\n")
	return strings.TrimRight(rendered, " \t")
}

func renderGroupsSummary(summary *compiledMaxGroupsSummary, hidden []renderedLine) string {
	if summary == nil {
		return ""
	}
	items := collectOmittedGroupSummaries(hidden)
	if len(items) == 0 {
		return ""
	}

	show := min(summary.show, len(items))
	parts := make([]string, 0, show)
	for _, item := range items[:show] {
		part := renderTemplate(summary.print, map[string]string{
			"key":   item.key,
			"count": strconv.Itoa(item.count),
		})
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}

	rendered := strings.Join(parts, summary.delimiter)
	if summary.prefix != "" {
		rendered = summary.prefix + rendered
	}
	if remaining := len(items) - show; remaining > 0 && summary.suffix != "" {
		rendered += renderTemplate(summary.suffix, map[string]string{
			"remaining": strconv.Itoa(remaining),
		})
	}
	return rendered
}

func collectOmittedGroupSummaries(hidden []renderedLine) []omittedGroupSummary {
	omittedByGroup := map[string]int{}
	for _, line := range hidden {
		if !line.groupItem || line.groupKey == "" {
			continue
		}
		omittedByGroup[line.groupKey]++
	}
	if len(omittedByGroup) == 0 {
		return nil
	}

	items := make([]omittedGroupSummary, 0, len(omittedByGroup))
	for key, count := range omittedByGroup {
		items = append(items, omittedGroupSummary{key: key, count: count})
	}
	slices.SortFunc(items, func(left, right omittedGroupSummary) int {
		if byCount := cmp.Compare(right.count, left.count); byCount != 0 {
			return byCount
		}
		return cmp.Compare(left.key, right.key)
	})
	return items
}

func trimLineEnding(line string) string {
	return strings.TrimRight(line, "\n")
}

func matchesAnyMatcher(patterns []compiledMatcher, line string) bool {
	for _, pattern := range patterns {
		if pattern.matches(line) {
			return true
		}
	}
	return false
}

func (m compiledMatcher) matches(content string) bool {
	switch {
	case m.regex != nil:
		return m.regex.MatchString(content)
	case m.startsWith != "":
		return strings.HasPrefix(content, m.startsWith)
	case m.contains != "":
		return strings.Contains(content, m.contains)
	case m.endsWith != "":
		return strings.HasSuffix(content, m.endsWith)
	default:
		return false
	}
}

func filterArgs(args []string) []string {
	if len(args) <= 1 {
		return nil
	}
	return args[1:]
}

func applyCommandMutations(args []string, when compiledWhen, flagsWithValues []string, command *compiledCommand) []string {
	if command == nil {
		return cloneStrings(args)
	}

	mutated := cloneStrings(args)
	for _, flag := range command.addShortFlags {
		mutated = addShortFlagIfMissing(mutated, flag)
	}
	for _, arg := range command.appendIfMissing {
		if argumentPresent(mutated, arg, flagsWithValues) {
			continue
		}
		mutated = append(mutated, arg)
	}
	filtered := mutated[1:]
	if when.firstIs != "" || len(when.firstIn) > 0 {
		filtered = filterArgs(filtered)
	}
	if !operations.HasExplicitPositionals(filtered, flagsWithValues) {
		mutated = append(mutated, command.appendIfNoPositionals...)
	}
	return mutated
}

func argumentPresent(args []string, want string, flagsWithValues []string) bool {
	if strings.HasPrefix(want, "--") {
		view := operations.ParseArguments(args, flagsWithValues)
		if name, value, ok := strings.Cut(want, "="); ok {
			return view.HasLongOptionValue(name, value)
		}
		return view.HasLongOption(want)
	}
	return slices.Contains(args, want)
}

func addShortFlagIfMissing(args []string, flag string) []string {
	if !isShortFlag(flag) || containsShortFlag(args, rune(flag[1])) {
		return args
	}
	return append(args, flag)
}

func isShortFlag(flag string) bool {
	return len(flag) == 2 && flag[0] == '-'
}

func containsShortFlag(args []string, want rune) bool {
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			continue
		}
		for _, current := range arg[1:] {
			if current == want {
				return true
			}
		}
	}
	return false
}

func matchesWhenArguments(when compiledWhen, flagsWithValues, args []string) bool {
	leadingCommandContext := when.firstIs != "" || len(when.firstIn) > 0
	view := operations.ParseArguments(args, flagsWithValues)
	return operations.MatchesFirstIs(args, when.firstIs) &&
		operations.MatchesFirstIn(args, when.firstIn) &&
		view.MatchesHaveAny(when.haveAny) &&
		view.MatchesLackAny(when.lackAny) &&
		view.MatchesHaveSequence(when.haveSequence) &&
		operations.MatchesHaveShortFlag(view.BeforeSeparator(), when.haveShortFlag) &&
		operations.MatchesNotHaveShortFlag(view.BeforeSeparator(), when.notHaveShortFlag) &&
		operations.MatchesHaveAllShortFlags(view.BeforeSeparator(), when.haveAllShortFlags) &&
		operations.MatchesNotHaveAllShortFlags(view.BeforeSeparator(), when.notHaveAllShortFlags) &&
		operations.MatchesPositionalsLackAny(args, when.positionalsLackAny, flagsWithValues) &&
		operations.MatchesNoPositionals(args, flagsWithValues, when.noPositionals, leadingCommandContext)
}

func outputCombined(out *OutputShape) *OutputScope {
	if out == nil {
		return nil
	}
	return out.Combined
}

func outputStdout(out *OutputShape) *OutputScope {
	if out == nil {
		return nil
	}
	return out.Stdout
}

func outputStderr(out *OutputShape) *OutputScope {
	if out == nil {
		return nil
	}
	return out.Stderr
}

func scopeLines(scope *OutputScope) *OutputLines {
	if scope == nil {
		return nil
	}
	return scope.Lines
}
