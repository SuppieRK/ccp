package operations

import (
	"slices"

	"go-command-compression-proxy/internal/contracts"
)

func MatchesFirstIs(args []string, first string) bool {
	return first == "" || (len(args) > 0 && args[0] == first)
}

func MatchesFirstIn(args, options []string) bool {
	return len(options) == 0 || (len(args) > 0 && slices.Contains(options, args[0]))
}

func MatchesHaveAny(args, wants []string) bool {
	return len(wants) == 0 || containsAny(args, wants)
}

func MatchesLackAny(args, disallowed []string) bool {
	return len(disallowed) == 0 || !containsAny(args, disallowed)
}

func MatchesHaveSequence(args, sequence []string) bool {
	return len(sequence) == 0 || containsSequence(args, sequence)
}

func MatchesHaveShortFlag(args, flags []string) bool {
	return len(flags) == 0 || containsShortFlag(args, flags)
}

func MatchesNotHaveShortFlag(args, flags []string) bool {
	return len(flags) == 0 || !containsShortFlag(args, flags)
}

func MatchesHaveAllShortFlags(args, flags []string) bool {
	return len(flags) == 0 || containsAllShortFlags(args, flags)
}

func MatchesNotHaveAllShortFlags(args, flags []string) bool {
	return len(flags) == 0 || !containsAllShortFlags(args, flags)
}

func MatchesPositionalsLackAny(args, disallowed, valueFlags []string) bool {
	return len(disallowed) == 0 || !containsAny(positionals(args, valueFlags), disallowed)
}

func HasExplicitPositionals(args, valueFlags []string) bool {
	return len(positionals(args, valueFlags)) > 0
}

func MatchesNoPositionals(args, valueFlags []string, want bool, allowLeadingCommand bool) bool {
	if !want {
		return true
	}
	if len(args) == 0 {
		return true
	}
	if allowLeadingCommand {
		return len(positionals(args[1:], valueFlags)) == 0
	}
	return len(positionals(args, valueFlags)) == 0
}

func ScopeForStream[T any](stream contracts.Stream, combined, stdout, stderr *T) (*T, bool) {
	switch stream {
	case contracts.StreamStdout:
		if stdout != nil {
			return stdout, true
		}
	case contracts.StreamStderr:
		if stderr != nil {
			return stderr, true
		}
	}
	return combined, combined != nil
}

func containsAny(args, wants []string) bool {
	for _, arg := range args {
		if slices.Contains(wants, arg) {
			return true
		}
	}
	return false
}

func containsSequence(args, sequence []string) bool {
	if len(sequence) == 0 {
		return true
	}
	if len(sequence) > len(args) {
		return false
	}
	for i := 0; i <= len(args)-len(sequence); i++ {
		if equalSequence(args[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func equalSequence(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func containsShortFlag(args, flags []string) bool {
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			continue
		}
		for _, flag := range flags {
			if len(flag) != 2 || flag[0] != '-' {
				continue
			}
			if containsRune(arg[1:], rune(flag[1])) {
				return true
			}
		}
	}
	return false
}

func containsAllShortFlags(args, flags []string) bool {
	for _, flag := range flags {
		if len(flag) != 2 || flag[0] != '-' || !containsShortFlag(args, []string{flag}) {
			return false
		}
	}
	return true
}

func containsRune(value string, want rune) bool {
	for _, current := range value {
		if current == want {
			return true
		}
	}
	return false
}

func positionals(args, valueFlags []string) []string {
	out := make([]string, 0, len(args))
	afterSeparator := false
	skipNextValue := false
	for _, arg := range args {
		if skipNextValue {
			skipNextValue = false
			continue
		}
		if afterSeparator {
			out = append(out, arg)
			continue
		}
		if arg == "--" {
			afterSeparator = true
			continue
		}
		if takesStandaloneValue(arg, valueFlags) {
			skipNextValue = true
			continue
		}
		if len(arg) > 0 && arg[0] == '-' {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func takesStandaloneValue(arg string, flags []string) bool {
	return slices.Contains(flags, arg)
}
