package common

import "strings"

func CopyArgs(args []string) []string {
	return append([]string{}, args...)
}

func HasExactFlag(args []string, flag string) bool {
	for _, a := range args {
		if strings.EqualFold(strings.TrimSpace(a), flag) {
			return true
		}
	}
	return false
}

func HasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		trimmed := strings.TrimSpace(a)
		for _, flag := range flags {
			if strings.EqualFold(trimmed, strings.TrimSpace(flag)) {
				return true
			}
		}
	}
	return false
}

func HasOption(args []string, key string) bool {
	lk := strings.ToLower(strings.TrimSpace(key))
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		la := strings.ToLower(a)
		if la == lk && i+1 < len(args) {
			return true
		}
		if strings.HasPrefix(la, lk+"=") {
			return true
		}
	}
	return false
}

func HasOptionAny(args []string, keys ...string) bool {
	for _, k := range keys {
		if HasOption(args, k) {
			return true
		}
	}
	return false
}

func OptionValue(args []string, key string) (string, bool) {
	trimmedKey := strings.TrimSpace(key)
	lk := strings.ToLower(trimmedKey)
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		la := strings.ToLower(a)
		if la == lk && i+1 < len(args) {
			return strings.TrimSpace(args[i+1]), true
		}
		if strings.HasPrefix(la, lk+"=") {
			return strings.TrimSpace(a[len(trimmedKey)+1:]), true
		}
	}
	return "", false
}

func OptionValueAny(args []string, keys ...string) (string, bool) {
	trimmedKeys := make([]string, len(keys))
	lowerKeys := make([]string, len(keys))
	for i, key := range keys {
		trimmed := strings.TrimSpace(key)
		trimmedKeys[i] = trimmed
		lowerKeys[i] = strings.ToLower(trimmed)
	}
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		la := strings.ToLower(a)
		for j, lk := range lowerKeys {
			if la == lk && i+1 < len(args) {
				return strings.TrimSpace(args[i+1]), true
			}
			if strings.HasPrefix(la, lk+"=") {
				return strings.TrimSpace(a[len(trimmedKeys[j])+1:]), true
			}
		}
	}
	return "", false
}

func HasOptionValue(args []string, key string, value string) bool {
	v, ok := OptionValue(args, key)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(v), value)
}

func ParsePositiveIntOptionAny(args []string, fallback int, keys ...string) int {
	trimmedKeys := make([]string, len(keys))
	lowerKeys := make([]string, len(keys))
	for i, key := range keys {
		trimmed := strings.TrimSpace(key)
		trimmedKeys[i] = trimmed
		lowerKeys[i] = strings.ToLower(trimmed)
	}
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		la := strings.ToLower(a)
		value, consumedNext, matched := matchPositiveIntOptionArg(args, i, a, la, trimmedKeys, lowerKeys)
		if !matched {
			continue
		}
		if n := ParsePositiveInt(value, 0); n > 0 {
			return n
		}
		if consumedNext {
			i++
		}
	}
	return fallback
}

func matchPositiveIntOptionArg(args []string, index int, arg, lowerArg string, trimmedKeys, lowerKeys []string) (value string, consumedNext bool, matched bool) {
	for j, lk := range lowerKeys {
		if lowerArg == lk {
			if index+1 >= len(args) {
				return "", false, true
			}
			return strings.TrimSpace(args[index+1]), true, true
		}
		if strings.HasPrefix(lowerArg, lk+"=") {
			return strings.TrimSpace(arg[len(trimmedKeys[j])+1:]), false, true
		}
	}
	return "", false, false
}
