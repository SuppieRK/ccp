package version

import (
	"fmt"
	"strconv"
	"strings"
)

type Semantic struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func Current() (Semantic, bool) {
	return Parse(Version)
}

func Parse(raw string) (Semantic, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Semantic{}, false
	}

	major, ok := parseComponent(parts[0])
	if !ok {
		return Semantic{}, false
	}
	minor, ok := parseComponent(parts[1])
	if !ok {
		return Semantic{}, false
	}
	patch, ok := parseComponent(parts[2])
	if !ok {
		return Semantic{}, false
	}

	return Semantic{Major: major, Minor: minor, Patch: patch}, true
}

func parseComponent(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	if len(raw) > 1 && raw[0] == '0' {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func (s Semantic) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

func (s Semantic) Compare(other Semantic) int {
	if s.Major != other.Major {
		if s.Major < other.Major {
			return -1
		}
		return 1
	}
	if s.Minor != other.Minor {
		if s.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if s.Patch != other.Patch {
		if s.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

func (s Semantic) Less(other Semantic) bool {
	return s.Compare(other) < 0
}

func (s Semantic) AtLeast(other Semantic) bool {
	return s.Compare(other) >= 0
}
