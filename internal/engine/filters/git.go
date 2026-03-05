package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	gitfilters "go-command-compression-proxy/internal/engine/filters/git"
)

var gitGlobalFlags = map[string]struct{}{
	"-C":           {},
	"-c":           {},
	"--git-dir":    {},
	"--work-tree":  {},
	"--namespace":  {},
	"--config-env": {},
	"--exec-path":  {},
	"--no-pager":   {},
	"--paginate":   {},
	"--version":    {},
	"--help":       {},
}

var gitGlobalFlagsNeedValue = map[string]struct{}{
	"-C":           {},
	"-c":           {},
	"--git-dir":    {},
	"--work-tree":  {},
	"--namespace":  {},
	"--config-env": {},
	"--exec-path":  {},
}

// NewGitToolFilter builds a git parent filter that owns subcommand dispatch.
func NewGitToolFilter() engine.ToolFilter {
	reg, initErr := buildGitSubcommandRegistry()
	return &gitToolFilter{subcommands: reg, initErr: initErr}
}

type gitToolFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (g *gitToolFilter) Tool() string { return "git" }

func (g *gitToolFilter) Aliases() []string { return []string{"git.exe"} }

func (g *gitToolFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, g.Tool(), g.initErr); ok {
		return prep
	}
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	f, consumed := resolveGitSubcommandFromArgs(g.subcommands, args)
	if f == nil {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}

	prep := f.Prepare(args[consumed:])
	return applyDelegatedPrepare(args, consumed, prep, f.Tool(), false)
}

func (g *gitToolFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(g.Tool(), ev, g.resolve)
}

func (g *gitToolFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, g.resolve)
}

func (g *gitToolFilter) MaskingHorizon() int { return 0 }

func (g *gitToolFilter) resolve(ev engine.Event) engine.ToolFilter {
	if g.initErr != nil {
		return nil
	}
	dispatch := strings.ToLower(strings.TrimSpace(ev.Dispatch))
	if dispatch == "" || !strings.HasPrefix(dispatch, "git ") {
		return nil
	}
	return g.subcommands.Resolve(dispatch)
}

func buildGitSubcommandRegistry() (*engine.ToolFilterRegistry, error) {
	return newSubcommandRegistry(
		gitfilters.NewGitStatusFilter(),
		gitfilters.NewGitDiffFilter(),
		gitfilters.NewGitLogFilter(),
		gitfilters.NewGitCommitFilter(),
		gitfilters.NewGitPushFilter(),
		gitfilters.NewGitPullFilter(),
		gitfilters.NewGitMergeFilter(),
		gitfilters.NewGitRebaseFilter(),
		gitfilters.NewGitBlameFilter(),
	)
}

func resolveGitSubcommandFromArgs(reg *engine.ToolFilterRegistry, args []string) (engine.ToolFilter, int) {
	if reg == nil || len(args) == 0 {
		return nil, 0
	}
	reordered, moved := moveLeadingGitFlags(args)
	if len(reordered) == 0 {
		return nil, 0
	}
	maxPrefix := 2
	if len(reordered) < maxPrefix {
		maxPrefix = len(reordered)
	}
	for n := maxPrefix; n >= 1; n-- {
		parts := make([]string, 0, n)
		valid := true
		for i := 0; i < n; i++ {
			part := strings.ToLower(strings.TrimSpace(reordered[i]))
			if part == "" {
				valid = false
				break
			}
			parts = append(parts, part)
		}
		if !valid {
			continue
		}
		key := "git " + strings.Join(parts, " ")
		if f := reg.Resolve(key); f != nil {
			return f, len(moved) + n
		}
	}
	return nil, 0
}

func moveLeadingGitFlags(args []string) ([]string, []string) {
	if len(args) == 0 {
		return nil, nil
	}
	leading := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		arg := strings.TrimSpace(args[i])
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}
		if !isKnownLeadingGitGlobalFlag(arg) {
			break
		}
		leading = append(leading, args[i])
		needsValue := gitLeadingFlagNeedsValue(arg)
		if needsValue && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func isKnownLeadingGitGlobalFlag(arg string) bool {
	if strings.HasPrefix(arg, "--git-dir=") ||
		strings.HasPrefix(arg, "--work-tree=") ||
		strings.HasPrefix(arg, "--namespace=") ||
		strings.HasPrefix(arg, "--config-env=") {
		return true
	}
	_, ok := gitGlobalFlags[arg]
	return ok
}

func gitLeadingFlagNeedsValue(arg string) bool {
	if strings.HasPrefix(arg, "--git-dir=") ||
		strings.HasPrefix(arg, "--work-tree=") ||
		strings.HasPrefix(arg, "--namespace=") ||
		strings.HasPrefix(arg, "--config-env=") {
		return false
	}
	_, ok := gitGlobalFlagsNeedValue[arg]
	return ok
}
