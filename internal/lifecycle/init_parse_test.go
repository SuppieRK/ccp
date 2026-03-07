package lifecycle

import (
	"slices"
	"strings"
	"testing"
)

func TestParseToolsNormalizesSortsAndDedupes(t *testing.T) {
	t.Parallel()

	got := parseTools(" Git , go ,git,  ,DOCKER,go ")
	want := []string{"docker", "git", "go"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseTools() = %v, want %v", got, want)
	}
}

func TestRunInitRequiresTools(t *testing.T) {
	newLifecycleWorkspace(t)

	if err := RunInit([]string{}); err == nil {
		t.Fatal("expected error when no tools are provided or detected")
	} else if !strings.Contains(err.Error(), "no tools detected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInitRejectsUnsupportedTool(t *testing.T) {
	newLifecycleWorkspace(t)

	if err := RunInit([]string{"--tools", "unknown"}); err == nil {
		t.Fatal("expected unsupported-tool error")
	} else if !strings.Contains(err.Error(), "unsupported tool") {
		t.Fatalf("unexpected error: %v", err)
	}
}
