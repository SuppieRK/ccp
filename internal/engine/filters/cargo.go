package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	cargofilters "go-command-compression-proxy/internal/engine/filters/cargo"
)

var cargoGlobalFlags = map[string]struct{}{
	"--config":  {},
	"--color":   {},
	"-Z":        {},
	"-q":        {},
	"--quiet":   {},
	"-v":        {},
	"--verbose": {},
	"--frozen":  {},
	"--locked":  {},
	"--offline": {},
}

var cargoGlobalFlagsNeedingValue = map[string]struct{}{
	"--config": {},
	"--color":  {},
	"-Z":       {},
}

// NewCargoToolFilter builds a cargo parent filter that owns subcommand dispatch.
func NewCargoToolFilter() engine.ToolFilter {
	reg, initErr := newSubcommandRegistry(
		cargofilters.NewTestFilter(),
		cargofilters.NewBuildFilter(),
		cargofilters.NewCheckFilter(),
		cargofilters.NewClippyFilter(),
	)
	return &cargoToolFilter{subcommands: reg, initErr: initErr}
}

type cargoToolFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (c *cargoToolFilter) Tool() string { return "cargo" }

func (c *cargoToolFilter) Aliases() []string { return []string{"cargo.exe"} }

func (c *cargoToolFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, c.Tool(), c.initErr); ok {
		return prep
	}
	passthrough := engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	if len(args) == 0 {
		return passthrough
	}
	reordered, moved := moveLeadingCargoFlags(args)
	if len(reordered) == 0 {
		return passthrough
	}
	sub := strings.ToLower(strings.TrimSpace(reordered[0]))
	subArgs := append([]string{}, reordered[1:]...)
	subArgs = append(subArgs, moved...)
	if hasCargoStructuredOutput(subArgs) {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "structured output mode"}
	}
	switch sub {
	case "test", "build", "check", "clippy":
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "cargo " + sub}
	case "run":
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "cargo run application output passthrough"}
	default:
		return passthrough
	}
}

func (c *cargoToolFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(c.Tool(), ev, c.resolve)
}

func (c *cargoToolFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, c.resolve)
}

func (c *cargoToolFilter) MaskingHorizon() int { return 0 }

func (c *cargoToolFilter) resolve(ev engine.Event) engine.ToolFilter {
	if c.initErr != nil {
		return nil
	}
	dispatch := strings.ToLower(strings.TrimSpace(ev.Dispatch))
	if dispatch == "" || !strings.HasPrefix(dispatch, "cargo ") {
		return nil
	}
	return c.subcommands.Resolve(dispatch)
}

func hasCargoStructuredOutput(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(strings.TrimSpace(args[i]))
		if arg == "--message-format" && i+1 < len(args) {
			value := strings.ToLower(strings.TrimSpace(args[i+1]))
			if strings.Contains(value, "json") {
				return true
			}
			continue
		}
		if strings.HasPrefix(arg, "--message-format=") && strings.Contains(arg, "json") {
			return true
		}
	}
	return false
}

func moveLeadingCargoFlags(args []string) ([]string, []string) {
	if len(args) == 0 {
		return nil, nil
	}
	leading := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		arg := strings.TrimSpace(args[i])
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "+") && len(arg) > 1 {
			leading = append(leading, args[i])
			i++
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if !cargoKnownGlobalFlag(arg) {
			break
		}
		leading = append(leading, args[i])
		if cargoGlobalFlagNeedsValue(arg) && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func cargoKnownGlobalFlag(arg string) bool {
	if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--color=") || strings.HasPrefix(arg, "-Z") {
		return true
	}
	_, ok := cargoGlobalFlags[arg]
	return ok
}

func cargoGlobalFlagNeedsValue(arg string) bool {
	if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--color=") || (strings.HasPrefix(arg, "-Z") && arg != "-Z") {
		return false
	}
	_, ok := cargoGlobalFlagsNeedingValue[arg]
	return ok
}
