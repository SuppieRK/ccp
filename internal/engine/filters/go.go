package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	gofilters "go-command-compression-proxy/internal/engine/filters/go"
)

// NewGoToolFilter builds a go parent filter that owns subcommand dispatch.
func NewGoToolFilter() engine.ToolFilter {
	reg, initErr := newSubcommandRegistry(
		gofilters.NewTestFilter(),
		gofilters.NewBuildFilter(),
	)
	return &goToolFilter{subcommands: reg, initErr: initErr}
}

type goToolFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (g *goToolFilter) Tool() string { return "go" }

func (g *goToolFilter) Aliases() []string { return []string{"go.exe"} }

func (g *goToolFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, g.Tool(), g.initErr); ok {
		return prep
	}
	passthrough := engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	if len(args) == 0 {
		return passthrough
	}
	reordered, moved := moveLeadingGoFlags(args)
	if len(reordered) == 0 {
		return passthrough
	}
	sub := strings.ToLower(strings.TrimSpace(reordered[0]))
	subArgs := append([]string{}, reordered[1:]...)
	subArgs = append(subArgs, moved...)
	switch sub {
	case "test":
		for _, arg := range subArgs {
			if strings.EqualFold(strings.TrimSpace(arg), "-json") {
				return engine.PrepareResult{
					NormalizedArgs:   args,
					ForcePassthrough: true,
					Ambiguous:        true,
					Reason:           "structured output mode",
				}
			}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "go test"}
	case "build":
		for _, arg := range subArgs {
			if strings.TrimSpace(arg) == "-x" {
				return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "go build|x=1"}
			}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "go build"}
	default:
		return passthrough
	}
}

func moveLeadingGoFlags(args []string) ([]string, []string) {
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
		if !strings.HasPrefix(arg, "-C=") && arg != "-C" {
			break
		}
		leading = append(leading, args[i])
		if arg == "-C" && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func (g *goToolFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(g.Tool(), ev, g.resolve)
}

func (g *goToolFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, g.resolve)
}

func (g *goToolFilter) MaskingHorizon() int { return 0 }

func (g *goToolFilter) resolve(ev engine.Event) engine.ToolFilter {
	if g.initErr != nil {
		return nil
	}
	dispatch := strings.ToLower(strings.TrimSpace(ev.Dispatch))
	if dispatch == "" || !strings.HasPrefix(dispatch, "go ") {
		return nil
	}
	if i := strings.IndexByte(dispatch, '|'); i >= 0 {
		dispatch = dispatch[:i]
	}
	return g.subcommands.Resolve(dispatch)
}
