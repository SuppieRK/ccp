package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
	dockerfilters "go-command-compression-proxy/internal/engine/filters/docker"
)

const dockerImagesStructuredFormat = "{{.Repository}}:{{.Tag}}\t{{.Size}}"
const dockerFormatFlag = "--format"
const dockerStructuredOutputReason = "structured output mode"
const dockerComposeBuildDispatchKey = "docker compose build"
const dockerComposePSDispatchKey = "docker compose ps"

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
		dockerfilters.NewComposeBuildFilter(),
		dockerfilters.NewComposePSFilter(),
		dockerfilters.NewComposeLogsFilter(),
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
	switch sub {
	case "compose":
		return prepareDockerComposeRoute(args, reordered[1:], passthrough)
	case "exec", "pull", "build":
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "interactive or tty-heavy docker shape"}
	case "ps":
		subArgs := append([]string{}, reordered[1:]...)
		subArgs = append(subArgs, moved...)
		if filtercommon.HasOption(subArgs, dockerFormatFlag) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: dockerStructuredOutputReason}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker ps"}
	case "images":
		subArgs := append([]string{}, reordered[1:]...)
		subArgs = append(subArgs, moved...)
		if filtercommon.HasOption(subArgs, dockerFormatFlag) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: dockerStructuredOutputReason}
		}
		if len(subArgs) == 0 {
			return engine.PrepareResult{
				NormalizedArgs: []string{"images", dockerFormatFlag, dockerImagesStructuredFormat},
				DispatchKey:    "docker images",
			}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker images"}
	case "logs":
		subArgs := append([]string{}, reordered[1:]...)
		subArgs = append(subArgs, moved...)
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
	if strings.HasPrefix(dispatch, dockerComposeBuildDispatchKey) {
		return d.subcommands.Resolve(dockerComposeBuildDispatchKey)
	}
	if strings.HasPrefix(dispatch, dockerComposePSDispatchKey) {
		return d.subcommands.Resolve(dockerComposePSDispatchKey)
	}
	if strings.HasPrefix(dispatch, "docker compose logs") {
		return d.subcommands.Resolve("docker compose logs")
	}
	return d.subcommands.Resolve(dispatch)
}

func prepareDockerComposeRoute(args, subArgs []string, passthrough engine.PrepareResult) engine.PrepareResult {
	reordered, _ := moveLeadingDockerComposeFlags(subArgs)
	if len(reordered) == 0 {
		return passthrough
	}
	nested := strings.ToLower(strings.TrimSpace(reordered[0]))
	switch nested {
	case "logs":
		scope, follow, ok := dockerComposeLogsScope(reordered[1:])
		if follow {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: "follow-mode docker compose logs passthrough"}
		}
		if !ok {
			return passthrough
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: "docker compose logs|scope=" + scope}
	case "ps":
		if filtercommon.HasOption(reordered[1:], dockerFormatFlag) {
			return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true, Ambiguous: true, Reason: dockerStructuredOutputReason}
		}
		return engine.PrepareResult{NormalizedArgs: args, DispatchKey: dockerComposePSDispatchKey}
	case "build":
		if dockerComposeBuildArgsSupported(reordered[1:]) {
			return engine.PrepareResult{NormalizedArgs: args, DispatchKey: dockerComposeBuildDispatchKey}
		}
		return passthrough
	default:
		return passthrough
	}
}

func dockerComposeBuildArgsSupported(args []string) bool {
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return false
		}
	}
	return true
}

func moveLeadingDockerComposeFlags(args []string) ([]string, []string) {
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
		needsValue, known := dockerComposeGlobalFlag(arg)
		if !known {
			break
		}
		leading = append(leading, args[i])
		if needsValue && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func dockerComposeGlobalFlag(arg string) (needsValue bool, known bool) {
	switch {
	case strings.HasPrefix(arg, "--file="),
		strings.HasPrefix(arg, "--project-name="),
		strings.HasPrefix(arg, "--profile="),
		strings.HasPrefix(arg, "--env-file="),
		strings.HasPrefix(arg, "--project-directory="),
		strings.HasPrefix(arg, "--parallel="),
		strings.HasPrefix(arg, "--ansi="):
		return false, true
	}
	switch arg {
	case "-f", "--file", "-p", "--project-name", "--profile", "--env-file", "--project-directory", "--parallel", "--ansi":
		return true, true
	case "--compatibility", "--progress", "--dry-run":
		return false, true
	default:
		return false, false
	}
}

func dockerComposeLogsScope(args []string) (scope string, follow bool, ok bool) {
	services := make([]string, 0, 2)
	for i := 0; i < len(args); i++ {
		arg, decision, consumeNext := dockerComposeLogsArgDecision(args[i])
		if consumeNext && i+1 < len(args) {
			i++
		}
		switch decision {
		case dockerComposeLogsSkipArg:
			continue
		case dockerComposeLogsFollowArg:
			return "", true, true
		case dockerComposeLogsInvalidArg:
			return "", false, false
		case dockerComposeLogsServiceArg:
			services = append(services, arg)
		}
	}
	if len(services) == 0 {
		return "all", false, true
	}
	return strings.Join(services, ","), false, true
}

type dockerComposeLogsArgKind int

const (
	dockerComposeLogsSkipArg dockerComposeLogsArgKind = iota
	dockerComposeLogsFollowArg
	dockerComposeLogsInvalidArg
	dockerComposeLogsServiceArg
)

func dockerComposeLogsArgDecision(raw string) (arg string, kind dockerComposeLogsArgKind, consumeNext bool) {
	arg = strings.TrimSpace(raw)
	if arg == "" {
		return "", dockerComposeLogsSkipArg, false
	}
	if arg == "-f" || arg == "--follow" {
		return "", dockerComposeLogsFollowArg, false
	}
	if skip, next, known := dockerComposeLogsFlag(arg); known {
		if skip {
			return "", dockerComposeLogsSkipArg, next
		}
		return "", dockerComposeLogsInvalidArg, next
	}
	if strings.HasPrefix(arg, "-") {
		return "", dockerComposeLogsInvalidArg, false
	}
	return arg, dockerComposeLogsServiceArg, false
}

func dockerComposeLogsFlag(arg string) (skip bool, consumeNext bool, known bool) {
	if !strings.HasPrefix(arg, "-") {
		return false, false, false
	}
	if strings.Contains(arg, "=") {
		switch {
		case strings.HasPrefix(arg, "--tail="),
			strings.HasPrefix(arg, "--since="),
			strings.HasPrefix(arg, "--until="),
			strings.HasPrefix(arg, "--index="),
			strings.HasPrefix(arg, "--timestamps="),
			strings.HasPrefix(arg, "--no-color="),
			strings.HasPrefix(arg, "--no-log-prefix="):
			return true, false, true
		default:
			return false, false, false
		}
	}
	switch arg {
	case "--tail", "--since", "--until", "--index":
		return true, true, true
	case "-n", "-t", "--timestamps", "--no-color", "--no-log-prefix":
		return true, false, true
	default:
		return false, false, false
	}
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
