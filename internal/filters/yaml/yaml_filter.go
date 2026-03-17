package yaml

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/filters/operations"
)

type YamlFilter struct {
	spec  *FilterDefinition
	cases []compiledCase

	activeArgs string
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
		spec:  spec,
		cases: cases,
	}, nil
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
	f.prepareInvocation(context.Args())
	cs, ok := f.caseForArgs(context.Args())
	if !ok || cs.passthrough {
		return contracts.Action{Kind: contracts.ActionKeep}
	}
	scope := cs.scopeForExit(contracts.StreamStdout)
	output := renderStdoutExitOutput(strings.Join(context.BufferedLines(contracts.StreamStdout), ""), scope)
	if cs.onExit != nil {
		output = appendCaseExitPrint(output, cs.onExit, cs.variables)
	}
	return exitActionForOutput(output)
}

func renderStdoutExitOutput(output string, scope *compiledScope) string {
	if renderedGroups := renderScopeGroups(scope); renderedGroups != "" {
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

func appendCaseExitPrint(output string, onExit *compiledOnExit, variables map[string]string) string {
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

func exitActionForOutput(output string) contracts.Action {
	if output == "" {
		return contracts.Action{Kind: contracts.ActionKeep}
	}
	return contracts.Action{Kind: contracts.ActionReplace, Output: output}
}

func (f *YamlFilter) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	cs, ok := f.caseForArgs(command.Args)
	if !ok || cs.command == nil {
		return command, nil
	}

	mutated := command
	mutated.Args = applyCommandMutations(command.Args, cs.command)
	return mutated, nil
}

func (f *YamlFilter) Dispatch(command contracts.Command) string {
	cs, ok := f.caseForArgs(command.Args)
	if !ok {
		return f.spec.Filter
	}
	if cs.id == "" {
		return f.spec.Filter
	}
	return f.spec.Filter + "|" + cs.id
}

type compiledCase struct {
	id          string
	passthrough bool
	when        compiledWhen
	variables   map[string]string
	command     *compiledCommand
	stdout      *compiledScope
	stderr      *compiledScope
	shared      *compiledScope
	onExit      *compiledOnExit
}

type compiledWhen struct {
	firstIs            string
	firstIn            []string
	haveAny            []string
	lackAny            []string
	haveSequence       []string
	haveShortFlag      []string
	positionalsLackAny []string
}

type compiledCommand struct {
	appendIfMissing []string
	addShortFlags   []string
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
	count int
	print string
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
	compiled := compiledCase{
		id:          cs.ID,
		passthrough: cs.Passthrough,
		when:        compileWhenArguments(cs.WhenArguments),
		variables:   compileVariables(cs.Variables),
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

func compileVariables(variables []Variable) map[string]string {
	if len(variables) == 0 {
		return nil
	}
	compiled := make(map[string]string, len(variables))
	for _, variable := range variables {
		switch variable.Type {
		case "number":
			if variable.InitialValue != nil {
				compiled[variable.Name] = *variable.InitialValue
			} else {
				compiled[variable.Name] = "0"
			}
		default:
			if variable.InitialValue != nil {
				compiled[variable.Name] = *variable.InitialValue
			} else {
				compiled[variable.Name] = ""
			}
		}
	}
	return compiled
}

func compileOnExit(onExit *OnExit) *compiledOnExit {
	if onExit == nil {
		return nil
	}
	return &compiledOnExit{print: onExit.Print}
}

func compileCommandMutation(command *CommandMutation) *compiledCommand {
	if command == nil {
		return nil
	}
	return &compiledCommand{
		appendIfMissing: cloneStrings(command.AppendIfMissing),
		addShortFlags:   cloneStrings(command.AddShortFlags),
	}
}

func compileWhenArguments(when *WhenArguments) compiledWhen {
	if when == nil {
		return compiledWhen{}
	}
	return compiledWhen{
		firstIs:            when.FirstIs,
		firstIn:            cloneStrings(when.FirstIn),
		haveAny:            cloneStrings(when.HaveAny),
		lackAny:            cloneStrings(when.LackAny),
		haveSequence:       cloneStrings(when.HaveSequence),
		haveShortFlag:      cloneStrings(when.HaveShortFlag),
		positionalsLackAny: cloneStrings(when.PositionalsLackAny),
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
	if lines.Tail != nil {
		return nil, fmt.Errorf("scope %q lines.tail is not supported by the generic operations runtime yet", name)
	}
	if lines.Truncate != nil {
		return nil, fmt.Errorf("scope %q lines.truncate is not supported by the generic operations runtime yet", name)
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
	return &compiledMax{
		count: rule.Count,
		print: rule.Print,
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
	for _, current := range re.SubexpNames() {
		if current == name {
			return true
		}
	}
	return false
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

func (c *compiledCase) scopeForExit(stream contracts.Stream) *compiledScope {
	scope, _ := c.scope(stream)
	return scope
}

func (f *YamlFilter) caseForArgs(args []string) (*compiledCase, bool) {
	filteredArgs := filterArgs(args)
	for i := range f.cases {
		if matchesWhenArguments(f.cases[i].when, filteredArgs) {
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
	for name := range c.variables {
		if _, err := strconv.Atoi(c.variables[name]); err == nil {
			c.variables[name] = "0"
			continue
		}
		c.variables[name] = ""
	}
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

func renderScopeGroups(scope *compiledScope) string {
	if scope == nil || len(scope.groups) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, group := range scope.groups {
		builder.WriteString(group.render())
	}
	return builder.String()
}

func renderScopeMaxOverflow(scope *compiledScope) string {
	if scope == nil || scope.max == nil || scope.hidden == 0 {
		return ""
	}
	return renderMaxPrint(scope.max.print, scope.hidden)
}

func applyRenderedMax(rendered string, max *compiledMax) string {
	if max == nil || rendered == "" {
		return rendered
	}
	trimmed := strings.TrimRight(rendered, "\n")
	if trimmed == "" {
		return rendered
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= max.count {
		return rendered
	}
	out := strings.Join(lines[:max.count], "\n")
	printed := renderMaxPrint(max.print, len(lines)-max.count)
	if printed == "" {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return out
	}
	if out != "" && !strings.HasPrefix(printed, "\n") {
		out += "\n"
	}
	out += printed
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func (g *compiledGroup) render() string {
	switch g.mode {
	case groupModeBoundary:
		return g.renderBoundary()
	default:
		return g.renderCollected()
	}
}

func (g *compiledGroup) renderCollected() string {
	if len(g.items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(g.items))
	for key := range g.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		g.renderGroup(&builder, g.items[key])
	}
	return builder.String()
}

func (g *compiledGroup) renderBoundary() string {
	if len(g.sections) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, section := range g.sections {
		g.renderBoundarySection(&builder, section)
	}
	return builder.String()
}

func (g *compiledGroup) renderGroup(builder *strings.Builder, groupItems []compiledGroupItem) {
	if len(groupItems) == 0 {
		return
	}
	var groupBuilder strings.Builder
	emitted, hidden := g.renderGroupItems(&groupBuilder, groupItems)
	if !emitted && (g.lines == nil || g.lines.max == nil || hidden == 0 || renderMaxPrint(g.lines.max.print, hidden) == "") {
		return
	}
	g.writeGroupStage(builder, g.initially, groupItems[0].vars)
	builder.WriteString(groupBuilder.String())
	if g.lines != nil && g.lines.max != nil && hidden > 0 {
		if printed := renderMaxPrint(g.lines.max.print, hidden); printed != "" {
			writeRenderedLine(builder, printed)
		}
	}
	g.writeGroupStage(builder, g.finally, groupItems[0].vars)
}

func (g *compiledGroup) renderBoundarySection(builder *strings.Builder, section compiledBoundarySection) {
	var sectionBuilder strings.Builder
	emitted, hidden := g.renderGroupItems(&sectionBuilder, section.items)
	if !emitted && (g.lines == nil || g.lines.max == nil || hidden == 0 || renderMaxPrint(g.lines.max.print, hidden) == "") {
		return
	}
	g.writeGroupStage(builder, g.initially, section.vars)
	builder.WriteString(sectionBuilder.String())
	if g.lines != nil && g.lines.max != nil && hidden > 0 {
		if printed := renderMaxPrint(g.lines.max.print, hidden); printed != "" {
			writeRenderedLine(builder, printed)
		}
	}
	g.writeGroupStage(builder, g.finally, section.vars)
}

func (g *compiledGroup) renderGroupItems(builder *strings.Builder, groupItems []compiledGroupItem) (bool, int) {
	emitted := 0
	hidden := 0
	for _, item := range groupItems {
		rendered, ok, limited := g.renderGroupItem(item, emitted)
		if limited {
			hidden++
			continue
		}
		if !ok {
			continue
		}
		writeRenderedLine(builder, rendered)
		emitted++
	}
	return emitted > 0, hidden
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

func (g *compiledGroup) writeGroupStage(builder *strings.Builder, stage *compiledOnExit, vars map[string]string) {
	if stage == nil {
		return
	}
	printed := renderTemplate(stage.print, vars)
	if printed == "" {
		return
	}
	writeRenderedLine(builder, printed)
}

func writeRenderedLine(builder *strings.Builder, line string) {
	builder.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		builder.WriteString("\n")
	}
}

func renderTemplate(template string, values map[string]string) string {
	out := template
	for name, value := range values {
		out = strings.ReplaceAll(out, "{{"+name+"}}", value)
	}
	return out
}

func renderMaxPrint(template string, value int) string {
	if template == "" {
		return ""
	}
	return strings.ReplaceAll(template, "{{value}}", strconv.Itoa(value))
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

func applyCommandMutations(args []string, command *compiledCommand) []string {
	if command == nil {
		return cloneStrings(args)
	}

	mutated := cloneStrings(args)
	for _, flag := range command.addShortFlags {
		mutated = addShortFlagIfMissing(mutated, flag)
	}
	for _, arg := range command.appendIfMissing {
		if containsArg(mutated, arg) {
			continue
		}
		mutated = append(mutated, arg)
	}
	return mutated
}

func addShortFlagIfMissing(args []string, flag string) []string {
	if !isShortFlag(flag) || containsShortFlag(args, rune(flag[1])) {
		return args
	}
	return append(args, flag)
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
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

func matchesWhenArguments(when compiledWhen, args []string) bool {
	return operations.MatchesFirstIs(args, when.firstIs) &&
		operations.MatchesFirstIn(args, when.firstIn) &&
		operations.MatchesHaveAny(args, when.haveAny) &&
		operations.MatchesLackAny(args, when.lackAny) &&
		operations.MatchesHaveSequence(args, when.haveSequence) &&
		operations.MatchesHaveShortFlag(args, when.haveShortFlag) &&
		operations.MatchesPositionalsLackAny(args, when.positionalsLackAny)
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
