package yaml

import (
	"slices"

	"go-command-compression-proxy/internal/filters/operations"
)

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
		if slices.Contains(mutated, arg) {
			continue
		}
		mutated = append(mutated, arg)
	}
	if len(mutated) == 0 {
		return mutated
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
	return operations.MatchesFirstIs(args, when.firstIs) &&
		operations.MatchesFirstIn(args, when.firstIn) &&
		operations.MatchesHaveAny(args, when.haveAny) &&
		operations.MatchesLackAny(args, when.lackAny) &&
		operations.MatchesHaveSequence(args, when.haveSequence) &&
		operations.MatchesHaveShortFlag(args, when.haveShortFlag) &&
		operations.MatchesNotHaveShortFlag(args, when.notHaveShortFlag) &&
		operations.MatchesHaveAllShortFlags(args, when.haveAllShortFlags) &&
		operations.MatchesNotHaveAllShortFlags(args, when.notHaveAllShortFlags) &&
		operations.MatchesPositionalsLackAny(args, when.positionalsLackAny, flagsWithValues) &&
		operations.MatchesNoPositionals(args, flagsWithValues, when.noPositionals, leadingCommandContext)
}
