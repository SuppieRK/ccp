package operations

import (
	"slices"
	"strings"
)

// ArgumentView is a match-only interpretation of argv. It never changes the
// slice passed to the child process.
type ArgumentView struct {
	raw            []string
	before         []string
	normalized     []string
	normalizedPre  []string
	positionals    []string
	longOptions    []longOption
	valueFlagNames []string
}

type longOption struct {
	name     string
	value    string
	hasValue bool
}

func ParseArguments(args, valueFlags []string) ArgumentView {
	view := ArgumentView{
		raw:            slices.Clone(args),
		valueFlagNames: slices.Clone(valueFlags),
	}
	afterSeparator := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if afterSeparator {
			view.normalized = append(view.normalized, arg)
			view.positionals = append(view.positionals, arg)
			continue
		}
		if arg == "--" {
			afterSeparator = true
			view.normalized = append(view.normalized, arg)
			continue
		}

		view.before = append(view.before, arg)
		index += view.appendBeforeSeparator(args, index, valueFlags)
	}
	return view
}

func (v *ArgumentView) appendBeforeSeparator(args []string, index int, valueFlags []string) int {
	arg := args[index]
	if name, value, ok := strings.Cut(arg, "="); ok && isLongOptionName(name) {
		v.longOptions = append(v.longOptions, longOption{name: name, value: value, hasValue: true})
		v.appendNormalized(name, value)
		return 0
	}
	if isLongOptionName(arg) {
		return v.appendLongOption(args, index, valueFlags)
	}
	v.appendNormalized(arg)
	if takesStandaloneValue(arg, valueFlags) {
		return v.appendStandaloneValue(args, index)
	}
	if !strings.HasPrefix(arg, "-") {
		v.positionals = append(v.positionals, arg)
	}
	return 0
}

func (v *ArgumentView) appendLongOption(args []string, index int, valueFlags []string) int {
	name := args[index]
	option := longOption{name: name}
	v.appendNormalized(name)
	if !slices.Contains(valueFlags, name) || index+1 >= len(args) {
		v.longOptions = append(v.longOptions, option)
		return 0
	}

	value := args[index+1]
	option.value = value
	option.hasValue = true
	v.before = append(v.before, value)
	v.appendNormalized(value)
	v.longOptions = append(v.longOptions, option)
	return 1
}

func (v *ArgumentView) appendStandaloneValue(args []string, index int) int {
	if index+1 >= len(args) {
		return 0
	}
	value := args[index+1]
	v.before = append(v.before, value)
	v.appendNormalized(value)
	return 1
}

func (v *ArgumentView) appendNormalized(values ...string) {
	v.normalizedPre = append(v.normalizedPre, values...)
	v.normalized = append(v.normalized, values...)
}

func (v ArgumentView) MatchesHaveAny(wants []string) bool {
	if len(wants) == 0 {
		return true
	}
	for _, want := range wants {
		if v.matchesWantedArgument(want) {
			return true
		}
	}
	return false
}

func (v ArgumentView) matchesWantedArgument(want string) bool {
	if !strings.HasPrefix(want, "--") {
		return slices.Contains(v.raw, want)
	}
	if name, value, hasValue := strings.Cut(want, "="); hasValue {
		return v.HasLongOptionValue(name, value)
	}
	return v.HasLongOption(want)
}

func (v ArgumentView) MatchesLackAny(disallowed []string) bool {
	return len(disallowed) == 0 || !v.MatchesHaveAny(disallowed)
}

func (v ArgumentView) MatchesPositionalsLackAny(disallowed []string) bool {
	return len(disallowed) == 0 || !containsAny(v.positionals, disallowed)
}

func (v ArgumentView) MatchesNoPositionals(want, allowLeadingCommand bool) bool {
	if !want {
		return true
	}
	allowed := 0
	if allowLeadingCommand && len(v.raw) > 0 {
		allowed = 1
	}
	return len(v.positionals) <= allowed
}

func (v ArgumentView) MatchesHaveSequence(sequence []string) bool {
	if len(sequence) == 0 {
		return true
	}
	if len(sequence) > 0 && strings.HasPrefix(sequence[0], "--") {
		return containsSequence(v.normalizedPre, normalizeMatcherSequence(sequence))
	}
	return containsSequence(v.normalized, normalizeMatcherSequence(sequence))
}

func (v ArgumentView) HasLongOption(name string) bool {
	for _, option := range v.longOptions {
		if option.name == name {
			return true
		}
	}
	return false
}

func (v ArgumentView) HasLongOptionValue(name, value string) bool {
	for _, option := range v.longOptions {
		if option.name == name && option.hasValue && option.value == value {
			return true
		}
	}
	return false
}

func (v ArgumentView) LastLongOptionValue(name string) (string, bool) {
	for index := len(v.longOptions) - 1; index >= 0; index-- {
		option := v.longOptions[index]
		if option.name == name && option.hasValue {
			return option.value, true
		}
	}
	return "", false
}

func (v ArgumentView) Positionals() []string {
	return slices.Clone(v.positionals)
}

func (v ArgumentView) BeforeSeparator() []string {
	return slices.Clone(v.before)
}

func normalizeMatcherSequence(sequence []string) []string {
	normalized := make([]string, 0, len(sequence)+1)
	for _, token := range sequence {
		if name, value, ok := strings.Cut(token, "="); ok && isLongOptionName(name) {
			normalized = append(normalized, name, value)
			continue
		}
		normalized = append(normalized, token)
	}
	return normalized
}

func isLongOptionName(value string) bool {
	return len(value) > 2 && strings.HasPrefix(value, "--")
}
