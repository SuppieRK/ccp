package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	npxfilters "go-command-compression-proxy/internal/engine/filters/npx"
)

// NewNPXFilter builds an npx parent filter that owns subcommand dispatch.
func NewNPXFilter() engine.ToolFilter {
	reg, initErr := newSubcommandRegistry(
		npxfilters.NewNpxTscFilter(),
		npxfilters.NewNpxEslintFilter(),
		npxfilters.NewNpxPrettierFilter(),
		npxfilters.NewNpxPrismaFilter(),
		npxfilters.NewNpxNodeFilter(),
	)
	return &npxFilter{subcommands: reg, initErr: initErr}
}

type npxFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (n *npxFilter) Tool() string { return "npx" }

func (n *npxFilter) Aliases() []string {
	return []string{"npx.cmd", "./npx.cmd", "npx.exe", "./npx.exe"}
}

func (n *npxFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, n.Tool(), n.initErr); ok {
		return prep
	}
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	f, consumed, lowConfidence := resolveNpxSubcommandFromArgs(n.subcommands, args)
	if lowConfidence || f == nil {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}

	prep := f.Prepare(args[consumed:])
	return applyDelegatedPrepare(args, consumed, prep, f.Tool(), false)
}

func (n *npxFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(n.Tool(), ev, n.resolve)
}

func (n *npxFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, n.resolve)
}

func (n *npxFilter) MaskingHorizon() int { return 0 }

func (n *npxFilter) resolve(ev engine.Event) engine.ToolFilter {
	if n.initErr != nil {
		return nil
	}
	dispatch := strings.ToLower(strings.TrimSpace(ev.Dispatch))
	if dispatch == "" || !strings.HasPrefix(dispatch, "npx ") {
		return nil
	}
	return n.subcommands.Resolve(dispatch)
}

func resolveNpxSubcommandFromArgs(reg *engine.ToolFilterRegistry, args []string) (engine.ToolFilter, int, bool) {
	if reg == nil || len(args) == 0 {
		return nil, 0, true
	}
	idx := 0
	for idx < len(args) {
		token := strings.TrimSpace(args[idx])
		if token == "" {
			idx++
			continue
		}
		switch lower := strings.ToLower(token); {
		case lower == "-p", lower == "--package", strings.HasPrefix(lower, "-p="), strings.HasPrefix(lower, "--package="):
			return nil, 0, true
		case lower == "--":
			idx++
		case strings.HasPrefix(lower, "-"):
			idx++
			continue
		}
		break
	}
	if idx >= len(args) {
		return nil, 0, true
	}

	key := npxDispatchKey(args[idx])
	if key == "" {
		return nil, 0, false
	}
	if f := reg.Resolve(key); f != nil {
		return f, idx + 1, false
	}
	return nil, 0, false
}

func npxDispatchKey(token string) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "tsc", "typescript":
		return "npx tsc"
	case "eslint":
		return "npx eslint"
	case "prettier":
		return "npx prettier"
	case "prisma":
		return "npx prisma"
	case "node":
		return "npx node"
	default:
		return ""
	}
}
