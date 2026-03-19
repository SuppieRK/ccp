package lifecycle

import (
	"os/exec"
	"strings"
)

const repairCutoffVersion = "0.5.1"

type releaseVersion struct {
	major int
	minor int
	patch int
}

func currentInstalledVersion(exePath string) (string, error) {
	out, err := exec.Command(exePath, "--version").CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		return trimmed, nil
	}
	return "", err
}

func shouldRunLegacyRepair(version string) bool {
	installed, ok := parseReleaseVersion(version)
	if !ok {
		return false
	}
	cutoff, ok := parseReleaseVersion(repairCutoffVersion)
	if !ok {
		return false
	}
	return compareReleaseVersion(installed, cutoff) < 0
}

func parseReleaseVersion(raw string) (releaseVersion, bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "v")
	trimmed, _, _ = strings.Cut(trimmed, "-")
	trimmed, _, _ = strings.Cut(trimmed, "+")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return releaseVersion{}, false
	}

	var parsed releaseVersion
	values := []*int{&parsed.major, &parsed.minor, &parsed.patch}
	for i, part := range parts {
		if part == "" {
			return releaseVersion{}, false
		}
		value, ok := parseDecimal(part)
		if !ok {
			return releaseVersion{}, false
		}
		*values[i] = value
	}
	return parsed, true
}

func parseDecimal(raw string) (int, bool) {
	value := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
		value = value*10 + int(r-'0')
	}
	return value, true
}

func compareReleaseVersion(left, right releaseVersion) int {
	switch {
	case left.major != right.major:
		return compareInt(left.major, right.major)
	case left.minor != right.minor:
		return compareInt(left.minor, right.minor)
	default:
		return compareInt(left.patch, right.patch)
	}
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
