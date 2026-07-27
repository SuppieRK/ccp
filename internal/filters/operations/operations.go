package operations

import (
	"slices"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/contracts"
)

func MatchesFirstIs(args []string, first string) bool {
	return first == "" || (len(args) > 0 && args[0] == first)
}

func MatchesFirstIn(args, options []string) bool {
	return len(options) == 0 || (len(args) > 0 && slices.Contains(options, args[0]))
}

func MatchesHaveAny(args, wants []string) bool {
	return len(wants) == 0 || slices.ContainsFunc(args, func(arg string) bool {
		return slices.Contains(wants, arg)
	})
}

func MatchesLackAny(args, disallowed []string) bool {
	return len(disallowed) == 0 || !slices.ContainsFunc(args, func(arg string) bool {
		return slices.Contains(disallowed, arg)
	})
}

func MatchesHaveSequence(args, sequence []string) bool {
	return len(sequence) == 0 || containsSequence(args, sequence)
}

func MatchesHaveShortFlag(args, flags []string) bool {
	return len(flags) == 0 || slices.ContainsFunc(args, func(arg string) bool {
		return len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' &&
			slices.ContainsFunc(flags, func(flag string) bool {
				return len(flag) == 2 && flag[0] == '-' && strings.ContainsRune(arg[1:], rune(flag[1]))
			})
	})
}

func MatchesNotHaveShortFlag(args, flags []string) bool {
	return len(flags) == 0 || !slices.ContainsFunc(args, func(arg string) bool {
		return len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' &&
			slices.ContainsFunc(flags, func(flag string) bool {
				return len(flag) == 2 && flag[0] == '-' && strings.ContainsRune(arg[1:], rune(flag[1]))
			})
	})
}

func MatchesHaveAllShortFlags(args, flags []string) bool {
	return len(flags) == 0 || !slices.ContainsFunc(flags, func(flag string) bool {
		return len(flag) != 2 || flag[0] != '-' || !slices.ContainsFunc(args, func(arg string) bool {
			return len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg[1:], rune(flag[1]))
		})
	})
}

func MatchesNotHaveAllShortFlags(args, flags []string) bool {
	return len(flags) == 0 || slices.ContainsFunc(flags, func(flag string) bool {
		return len(flag) != 2 || flag[0] != '-' || !slices.ContainsFunc(args, func(arg string) bool {
			return len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg[1:], rune(flag[1]))
		})
	})
}

func MatchesPositionalsLackAny(args, disallowed, valueFlags []string) bool {
	return ParseArguments(args, valueFlags).MatchesPositionalsLackAny(disallowed)
}

func HasExplicitPositionals(args, valueFlags []string) bool {
	return len(ParseArguments(args, valueFlags).Positionals()) > 0
}

func MatchesNoPositionals(args, valueFlags []string, want bool, allowLeadingCommand bool) bool {
	return ParseArguments(args, valueFlags).MatchesNoPositionals(want, allowLeadingCommand)
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

func containsSequence(args, sequence []string) bool {
	if len(sequence) == 0 {
		return true
	}
	if len(sequence) > len(args) {
		return false
	}
	for i := range len(args) - len(sequence) + 1 {
		if slices.Equal(args[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
