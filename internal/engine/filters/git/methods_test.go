package gitfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGitFilterMethodCoverage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		filter   engine.ToolFilter
		wantTool string
	}{
		{name: "commit", filter: NewGitCommitFilter(), wantTool: "git commit"},
		{name: "pull", filter: NewGitPullFilter(), wantTool: "git pull"},
		{name: "push", filter: NewGitPushFilter(), wantTool: "git push"},
		{name: "merge", filter: NewGitMergeFilter(), wantTool: "git merge"},
		{name: "rebase", filter: NewGitRebaseFilter(), wantTool: "git rebase"},
		{name: "status", filter: NewGitStatusFilter(), wantTool: "git status"},
		{name: "log", filter: NewGitLogFilter(), wantTool: "git log"},
		{name: "show", filter: NewGitShowFilter(), wantTool: "git show"},
		{name: "fetch", filter: NewGitFetchFilter(), wantTool: "git fetch"},
		{name: "diff", filter: NewGitDiffFilter(), wantTool: "git diff"},
		{name: "blame", filter: NewGitBlameFilter(), wantTool: "git blame"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Tool(); got != tc.wantTool {
				t.Fatalf("Tool() = %q, want %q", got, tc.wantTool)
			}
			if got := tc.filter.Aliases(); got != nil {
				t.Fatalf("Aliases() = %v, want nil", got)
			}
			prep := tc.filter.Prepare([]string{"--x"})
			if len(prep.NormalizedArgs) == 0 {
				t.Fatalf("Prepare() produced empty args: %#v", prep)
			}
			if tc.filter.ContextKey(engine.Event{CommandID: "c", Tool: tc.wantTool, Stream: engine.StdoutStream}) == "" {
				t.Fatal("ContextKey() returned empty key")
			}
			_ = tc.filter.MaskingHorizon()
		})
	}
}

func TestGitCommonHelpers(t *testing.T) {
	t.Parallel()

	if got := firstInt("x y 42 z"); got != 42 {
		t.Fatalf("firstInt() = %d, want 42", got)
	}
	if got := firstInt("no-int"); got != 0 {
		t.Fatalf("firstInt(no-int) = %d, want 0", got)
	}

	files, adds, dels := extractChangeSummary(" 2 files changed, 10 insertions(+), 3 deletions(-)\n")
	if files != 2 || adds != 10 || dels != 3 {
		t.Fatalf("extractChangeSummary mismatch: files=%d adds=%d dels=%d", files, adds, dels)
	}

	mem := engine.NewOrderedSetBuffer()
	collect := passthroughOnExit(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream}, mem)
	if collect.Action != engine.ActionCollect {
		t.Fatalf("passthroughOnExit line action = %q, want collect", collect.Action)
	}
}
