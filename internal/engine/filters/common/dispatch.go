package common

import (
	"strconv"
	"strings"
)

// ParseDispatchMap decodes a dispatch string of "k=v" segments separated by "|".
func ParseDispatchMap(dispatch string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(dispatch, "|") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func DispatchValue(dispatch, key string) string {
	return ParseDispatchMap(dispatch)[key]
}

func ParseBool01(v string) bool {
	return v == "1" || strings.EqualFold(v, "true")
}

func ParsePositiveInt(v string, fallback int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return n
	}
	return fallback
}
