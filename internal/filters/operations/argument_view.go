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
			view.raw = slices.Clone(args)
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
		if name, value, ok := strings.Cut(arg, "="); ok && isLongOptionName(name) {
			view.longOptions = append(view.longOptions, longOption{name: name, value: value, hasValue: true})
			view.normalizedPre = append(view.normalizedPre, name, value)
			view.normalized = append(view.normalized, name, value)
			continue
		}
		if isLongOptionName(arg) {
			option := longOption{name: arg}
			view.normalizedPre = append(view.normalizedPre, arg)
			view.normalized = append(view.normalized, arg)
			if slices.Contains(valueFlags, arg) && index+1 < len(args) {
				index++
				option.value = args[index]
				option.hasValue = true
				view.before = append(view.before, args[index])
				view.normalizedPre = append(view.normalizedPre, args[index])
				view.normalized = append(view.normalized, args[index])
			}
			view.longOptions = append(view.longOptions, option)
			continue
		}
		view.normalizedPre = append(view.normalizedPre, arg)
		view.normalized = append(view.normalized, arg)
		if takesStandaloneValue(arg, valueFlags) {
			if index+1 < len(args) {
				index++
				view.before = append(view.before, args[index])
				view.normalizedPre = append(view.normalizedPre, args[index])
				view.normalized = append(view.normalized, args[index])
			}
			continue
		}
		if len(arg) > 0 && arg[0] == '-' {
			continue
		}
		view.positionals = append(view.positionals, arg)
	}
	return view
}

func (v ArgumentView) MatchesHaveAny(wants []string) bool {
	if len(wants) == 0 {
		return true
	}
	for _, want := range wants {
		if strings.HasPrefix(want, "--") {
			if name, value, hasValue := strings.Cut(want, "="); hasValue {
				if v.HasLongOptionValue(name, value) {
					return true
				}
				continue
			}
			if v.HasLongOption(want) {
				return true
			}
			continue
		}
		if slices.Contains(v.raw, want) {
			return true
		}
	}
	return false
}

func (v ArgumentView) MatchesLackAny(disallowed []string) bool {
	return len(disallowed) == 0 || !v.MatchesHaveAny(disallowed)
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
