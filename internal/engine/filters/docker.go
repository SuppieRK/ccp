package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
	dockerfilters "go-command-compression-proxy/internal/engine/filters/docker"
)

const dockerImagesStructuredFormat = "{{.Repository}}:{{.Tag}}\t{{.Size}}"
const dockerFormatFlag = "--format"

var dockerGlobalFlags = map[string]struct{}{
	"--context":   {},
	"--host":      {},
	"-H":          {},
	"--config":    {},
	"--log-level": {},
	"-D":          {},
	"--debug":     {},
	"-l":          {},
	"--tls":       {},
	"--tlsverify": {},
}

var dockerGlobalFlagsNeedValue = map[string]struct{}{
	"--context":   {},
	"--host":      {},
	"-H":          {},
	"--config":    {},
	"--log-level": {},
}

// NewDockerToolFilter builds a docker parent filter that owns subcommand dispatch.
func NewDockerToolFilter() engine.ToolFilter {
	reg, initErr := newSubcommandRegistry(
		dockerfilters.NewPSFilter(),
		dockerfilters.NewImagesFilter(),
		dockerfilters.NewLogsFilter(),
	)
	return &dockerToolFilter{subcommands: reg, initErr: initErr}
}

type dockerToolFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (d *dockerToolFilter) Tool() string { return "docker" }

func (d *dockerToolFilter) Aliases() []string { return []string{"docker.exe"} }

func (d *dockerToolFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, d.Tool(), d.initErr); ok {
		return prep
	}
	passthrough := engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	if len(args) == 0 {
		return passthrough
	}

	reordered, moved := moveLeadingDockerFlags(args)
	if len(reordered) == 0 {
		return passthrough
	}
	sub := strings.ToLower(strings.TrimSpace(reordered[0]))
	subArgs := append([]string{}, reordered[1:]...)
	subArgs = append(subArgs, moved...)
	switch sub {
	case "compose":
		return passthrough
	case "exec", "pull", "build":
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "interactive or tty-heavy docker shape"}
	case "ps":
		if filtercommon.HasOption(subArgs, dockerFormatFlag) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "structured output mode"}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker ps"}
	case "images":
		if filtercommon.HasOption(subArgs, dockerFormatFlag) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "structured output mode"}
		}
		if len(subArgs) == 0 {
			return engine.PrepareResult{
				NormalizedArgs: []string{"images", dockerFormatFlag, dockerImagesStructuredFormat},
				DispatchKey:    "docker images",
			}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker images"}
	case "logs":
		if filtercommon.HasAnyFlag(subArgs, "-f", "--follow") {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "follow-mode docker logs passthrough"}
		}
		container := dockerLogsContainer(subArgs)
		if container == "" {
			return passthrough
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker logs|container=" + container}
	default:
		return passthrough
	}
}

func (d *dockerToolFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(d.Tool(), ev, d.resolve)
}

func (d *dockerToolFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, d.resolve)
}

func (d *dockerToolFilter) MaskingHorizon() int { return 0 }

func (d *dockerToolFilter) resolve(ev engine.Event) engine.ToolFilter {
	if d.initErr != nil {
		return nil
	}
	dispatch := strings.ToLower(strings.TrimSpace(ev.Dispatch))
	if dispatch == "" || !strings.HasPrefix(dispatch, "docker ") {
		return nil
	}
	if strings.HasPrefix(dispatch, "docker logs") {
		return d.subcommands.Resolve("docker logs")
	}
	return d.subcommands.Resolve(dispatch)
}

func moveLeadingDockerFlags(args []string) ([]string, []string) {
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
		if !isDockerKnownGlobalFlag(arg) {
			break
		}
		leading = append(leading, args[i])
		needsValue := dockerFlagNeedsValue(arg)
		if needsValue && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func dockerLogsContainer(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if skip, consumeNext := dockerLogsLongFlag(arg); skip {
			if consumeNext && i+1 < len(args) {
				i++
			}
			continue
		}
		if skip, consumeNext := dockerLogsShortFlag(arg); skip {
			if consumeNext && i+1 < len(args) {
				i++
			}
			continue
		}
		return arg
	}
	return ""
}

func isDockerKnownGlobalFlag(arg string) bool {
	if strings.HasPrefix(arg, "--context=") ||
		strings.HasPrefix(arg, "--host=") ||
		strings.HasPrefix(arg, "--config=") ||
		strings.HasPrefix(arg, "--log-level=") {
		return true
	}
	_, ok := dockerGlobalFlags[arg]
	return ok
}

func dockerFlagNeedsValue(arg string) bool {
	if strings.HasPrefix(arg, "--context=") ||
		strings.HasPrefix(arg, "--host=") ||
		strings.HasPrefix(arg, "--config=") ||
		strings.HasPrefix(arg, "--log-level=") {
		return false
	}
	_, ok := dockerGlobalFlagsNeedValue[arg]
	return ok
}

func dockerLogsLongFlag(arg string) (skip bool, consumeNext bool) {
	if !strings.HasPrefix(arg, "--") {
		return false, false
	}
	if strings.Contains(arg, "=") {
		return true, false
	}
	switch arg {
	case "--tail", "--since", "--until":
		return true, true
	default:
		return true, false
	}
}

func dockerLogsShortFlag(arg string) (skip bool, consumeNext bool) {
	if !strings.HasPrefix(arg, "-") {
		return false, false
	}
	if arg == "-n" {
		return true, true
	}
	return true, false
}
