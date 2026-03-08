package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const gitStatusDispatch = "git status"

func TestGitToolPrepareDelegatesStatusSubcommand(t *testing.T) {
	f := NewGitToolFilter()
	prep := f.Prepare([]string{"status"})
	if prep.ForcePassthrough {
		t.Fatal("did not expect passthrough for known subcommand")
	}
	want := []string{"status", "--porcelain"}
	if !slices.Equal(prep.NormalizedArgs, want) {
		t.Fatalf("unexpected args: got %#v want %#v", prep.NormalizedArgs, want)
	}
	if prep.DispatchKey != gitStatusDispatch {
		t.Fatalf("expected dispatch key %q, got %q", gitStatusDispatch, prep.DispatchKey)
	}
}

func TestGitToolIdentityAliasesAndDefaults(t *testing.T) {
	f := NewGitToolFilter()
	if got := f.Tool(); got != "git" {
		t.Fatalf("Tool() = %q, want %q", got, "git")
	}
	aliases := f.Aliases()
	if len(aliases) != 1 || aliases[0] != "git.exe" {
		t.Fatalf("Aliases() = %#v, want [\"git.exe\"]", aliases)
	}
	if got := f.MaskingHorizon(); got != 0 {
		t.Fatalf("MaskingHorizon() = %d, want 0", got)
	}
}

func TestGitToolPreparePassthroughCases(t *testing.T) {
	f := NewGitToolFilter()
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown-subcommand", args: []string{"mystery", "--x"}},
		{name: "empty-args", args: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !prep.ForcePassthrough {
				t.Fatalf("expected passthrough for %v, got %#v", tc.args, prep)
			}
		})
	}
}

func TestGitToolProcessDelegatesByDispatchKey(t *testing.T) {
	eng := engine.NewEngine(engine.Config{})
	if err := eng.RegisterFilter(NewGitToolFilter()); err != nil {
		t.Fatalf("register git filter: %v", err)
	}

	_ = eng.Process("stdout", "git", engine.Input{Dispatch: gitStatusDispatch, Line: "## main"})
	_ = eng.Process("stdout", "git", engine.Input{Dispatch: gitStatusDispatch, Line: " M README.md"})
	out := eng.Process("stdout", "git", engine.Input{Dispatch: gitStatusDispatch, EOF: true})
	if !out.Ready {
		t.Fatal("expected ready output on eof")
	}
	if !strings.Contains(out.Output, "## main") {
		t.Fatalf("expected passthrough status output, got %q", out.Output)
	}
}

func TestResolveGitSubcommandFromArgsMoveLeadingFlags(t *testing.T) {
	reg, err := buildGitSubcommandRegistry()
	if err != nil {
		t.Fatalf("build git subcommand registry: %v", err)
	}
	cases := map[string]struct {
		args         []string
		wantDispatch string
		wantSubArgs  []string
	}{
		"status plain":                     {args: []string{"status"}, wantDispatch: gitStatusDispatch},
		"log plain":                        {args: []string{"log"}, wantDispatch: "git log"},
		"show plain":                       {args: []string{"show", "HEAD"}, wantDispatch: "git show", wantSubArgs: []string{"HEAD"}},
		"blame plain":                      {args: []string{"blame", "x.txt"}, wantDispatch: "git blame", wantSubArgs: []string{"x.txt"}},
		"commit plain":                     {args: []string{"commit", "-m", "x"}, wantDispatch: "git commit", wantSubArgs: []string{"-m", "x"}},
		"leading -C":                       {args: []string{"-C", "repo", "status"}, wantDispatch: gitStatusDispatch},
		"leading --git-dir eq":             {args: []string{"--git-dir=.git", "status"}, wantDispatch: gitStatusDispatch},
		"leading --work-tree pair":         {args: []string{"--work-tree", ".", "status"}, wantDispatch: gitStatusDispatch},
		"leading -c pair":                  {args: []string{"-c", "core.editor=vim", "log", "--oneline"}, wantDispatch: "git log", wantSubArgs: []string{"--oneline"}},
		"leading --namespace eq":           {args: []string{"--namespace=x", "status"}, wantDispatch: gitStatusDispatch},
		"leading --config-env pair":        {args: []string{"--config-env", "core.sshCommand=SSH", "status"}, wantDispatch: gitStatusDispatch},
		"leading --exec-path pair":         {args: []string{"--exec-path", "/usr/lib/git-core", "status"}, wantDispatch: gitStatusDispatch},
		"leading --no-pager bool":          {args: []string{"--no-pager", "status"}, wantDispatch: gitStatusDispatch},
		"leading --help bool":              {args: []string{"--help", "status"}, wantDispatch: gitStatusDispatch},
		"leading --version bool":           {args: []string{"--version", "status"}, wantDispatch: gitStatusDispatch},
		"unknown leading flag passthrough": {args: []string{"--unknown-global", "status"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assertResolvedGitSubcommand(t, reg, tc.args, tc.wantDispatch, tc.wantSubArgs)
		})
	}
}

func TestBuildGitSubcommandRegistryContainsSupportedHandlers(t *testing.T) {
	reg, err := buildGitSubcommandRegistry()
	if err != nil {
		t.Fatalf("build git subcommand registry: %v", err)
	}
	for _, key := range []string{
		"git status",
		"git diff",
		"git log",
		"git show",
		"git commit",
		"git push",
		"git pull",
		"git merge",
		"git rebase",
		"git blame",
	} {
		if reg.Resolve(key) == nil {
			t.Fatalf("expected registry to resolve %q", key)
		}
	}
}

func TestResolveGitSubcommandPrefersLongestPrefix(t *testing.T) {
	reg := engine.NewToolFilterRegistry()
	if err := reg.Register(testGitDispatchFilter{tool: "git foo"}); err != nil {
		t.Fatalf("register git foo: %v", err)
	}
	if err := reg.Register(testGitDispatchFilter{tool: "git foo bar"}); err != nil {
		t.Fatalf("register git foo bar: %v", err)
	}

	f, consumed := resolveGitSubcommandFromArgs(reg, []string{"foo", "bar", "--x"})
	if f == nil {
		t.Fatal("expected longest-prefix dispatch to resolve")
	}
	if got := f.Tool(); got != "git foo bar" {
		t.Fatalf("expected longest-prefix dispatch, got %q", got)
	}
	if consumed != 2 {
		t.Fatalf("consumed = %d, want 2", consumed)
	}
}

func TestGitToolProcessFallsBackToNoopForMissingOrUnknownDispatch(t *testing.T) {
	f := NewGitToolFilter()
	cases := []struct {
		name        string
		dispatchKey string
		line        string
	}{
		{name: "empty-dispatch", dispatchKey: "", line: "fallback-empty\n"},
		{name: "non-git-dispatch", dispatchKey: "not-git status", line: "fallback-other\n"},
		{name: "unknown-git-dispatch", dispatchKey: "git mystery", line: "fallback-unknown\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := f.Process(engine.Event{
				Type:     engine.EventLine,
				Tool:     "git",
				Dispatch: tc.dispatchKey,
				Stream:   engine.StdoutStream,
				Line:     tc.line,
			}, engine.NewOrderedSetBuffer())
			if got.Action != engine.ActionImmediate || got.Output != tc.line {
				t.Fatalf("expected noop fallback, got %#v", got)
			}
		})
	}
}

func assertResolvedGitSubcommand(t *testing.T, reg *engine.ToolFilterRegistry, args []string, wantDispatch string, wantSubArgs []string) {
	t.Helper()
	f, consumed := resolveGitSubcommandFromArgs(reg, args)
	if wantDispatch == "" {
		if f != nil {
			t.Fatalf("expected nil dispatch, got %q", f.Tool())
		}
		return
	}
	if f == nil {
		t.Fatalf("expected dispatch %q, got nil", wantDispatch)
	}
	if got := f.Tool(); got != wantDispatch {
		t.Fatalf("dispatch mismatch: want %q got %q", wantDispatch, got)
	}
	subArgs := args[consumed:]
	if !slices.Equal(subArgs, wantSubArgs) {
		t.Fatalf("sub-args mismatch: want %#v got %#v", wantSubArgs, subArgs)
	}
}

type testGitDispatchFilter struct {
	tool string
}

func (t testGitDispatchFilter) Tool() string { return t.tool }

func (t testGitDispatchFilter) Aliases() []string { return nil }

func (t testGitDispatchFilter) Prepare(args []string) engine.PrepareResult {
	return engine.PrepareResult{NormalizedArgs: args}
}

func (t testGitDispatchFilter) ContextKey(_ engine.Event) string {
	return "test-git-dispatch"
}

func (t testGitDispatchFilter) Process(_ engine.Event, _ *engine.OrderedSetBuffer) engine.Decision {
	return engine.Decision{Action: engine.ActionCollect}
}

func (t testGitDispatchFilter) MaskingHorizon() int { return 0 }
