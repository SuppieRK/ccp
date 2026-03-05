package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
	filtercommon "go-command-compression-proxy/internal/engine/filters/common"
	kubectlfilters "go-command-compression-proxy/internal/engine/filters/kubectl"
)

var kubectlLeadingGlobalFlags = map[string]struct{}{
	"-n":          {},
	"--namespace": {},
	"--context":   {},
	"--cluster":   {},
	"--user":      {},
	"--server":    {},
}

// NewKubectlToolFilter builds a kubectl parent filter that owns subcommand dispatch.
func NewKubectlToolFilter() engine.ToolFilter {
	reg, initErr := buildKubectlSubcommandRegistry()
	return &kubectlToolFilter{subcommands: reg, initErr: initErr}
}

type kubectlToolFilter struct {
	subcommands *engine.ToolFilterRegistry
	initErr     error
}

func (k *kubectlToolFilter) Tool() string { return "kubectl" }

func (k *kubectlToolFilter) Aliases() []string { return []string{"kubectl.exe"} }

func (k *kubectlToolFilter) Prepare(args []string) engine.PrepareResult {
	if prep, ok := prepareParentInitError(args, k.Tool(), k.initErr); ok {
		return prep
	}
	if len(args) == 0 {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	if hasStructuredOutputFlag(args) {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}

	f, subArgs := resolveKubectlSubcommandFromArgs(k.subcommands, args)
	if f == nil {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}

	prep := f.Prepare(subArgs)
	if prep.ForcePassthrough {
		return engine.PrepareResult{NormalizedArgs: args, ForcePassthrough: true}
	}
	return engine.PrepareResult{NormalizedArgs: args, DispatchKey: f.Tool()}
}

func (k *kubectlToolFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(k.Tool(), ev, k.resolve)
}

func (k *kubectlToolFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, k.resolve)
}

func (k *kubectlToolFilter) MaskingHorizon() int { return 0 }

func (k *kubectlToolFilter) resolve(ev engine.Event) engine.ToolFilter {
	if k.initErr != nil {
		return nil
	}
	if ev.Dispatch == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(ev.Dispatch), "kubectl ") {
		return nil
	}
	return k.subcommands.Resolve(strings.ToLower(strings.TrimSpace(ev.Dispatch)))
}

func buildKubectlSubcommandRegistry() (*engine.ToolFilterRegistry, error) {
	return newSubcommandRegistry(
		kubectlfilters.NewKubectlGetPodsFilter(),
		kubectlfilters.NewKubectlGetNodesFilter(),
		kubectlfilters.NewKubectlGetServicesFilter(),
		kubectlfilters.NewKubectlLogsFilter(),
	)
}

func resolveKubectlSubcommandFromArgs(reg *engine.ToolFilterRegistry, args []string) (engine.ToolFilter, []string) {
	if reg == nil || len(args) == 0 {
		return nil, nil
	}

	reordered, moved := moveLeadingKubectlFlags(args)
	if len(reordered) == 0 {
		return nil, nil
	}

	sub := strings.ToLower(strings.TrimSpace(reordered[0]))
	if sub == "logs" {
		return resolveKubectlLogsSubcommand(reg, reordered[1:], moved)
	}
	if sub != "get" {
		return nil, nil
	}
	return resolveKubectlGetSubcommand(reg, reordered, moved)
}

func kubectlGetDispatchKey(resource string) string {
	switch resource {
	case "pod", "pods":
		return "kubectl get pods"
	case "node", "nodes":
		return "kubectl get nodes"
	case "svc", "service", "services":
		return "kubectl get services"
	default:
		return ""
	}
}

func moveLeadingKubectlFlags(args []string) ([]string, []string) {
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
		if !isKnownLeadingKubectlGlobalFlag(arg) {
			break
		}
		leading = append(leading, args[i])
		if kubectlGlobalFlagNeedsValue(arg) && i+1 < len(args) {
			i++
			leading = append(leading, args[i])
		}
		i++
	}
	return args[i:], leading
}

func isKnownLeadingKubectlGlobalFlag(arg string) bool {
	if strings.HasPrefix(arg, "--namespace=") ||
		strings.HasPrefix(arg, "--context=") ||
		strings.HasPrefix(arg, "--cluster=") ||
		strings.HasPrefix(arg, "--user=") ||
		strings.HasPrefix(arg, "--server=") {
		return true
	}
	_, ok := kubectlLeadingGlobalFlags[arg]
	return ok
}

func kubectlGlobalFlagNeedsValue(arg string) bool {
	if strings.HasPrefix(arg, "--namespace=") ||
		strings.HasPrefix(arg, "--context=") ||
		strings.HasPrefix(arg, "--cluster=") ||
		strings.HasPrefix(arg, "--user=") ||
		strings.HasPrefix(arg, "--server=") {
		return false
	}
	_, ok := kubectlLeadingGlobalFlags[arg]
	return ok
}

func resolveKubectlLogsSubcommand(reg *engine.ToolFilterRegistry, reorderedTail, moved []string) (engine.ToolFilter, []string) {
	subArgs := append([]string{}, reorderedTail...)
	subArgs = append(subArgs, moved...)
	if hasKubectlLogsFollowFlag(subArgs) {
		return nil, nil
	}
	if f := reg.Resolve("kubectl logs"); f != nil {
		return f, subArgs
	}
	return nil, nil
}

func resolveKubectlGetSubcommand(reg *engine.ToolFilterRegistry, reordered, moved []string) (engine.ToolFilter, []string) {
	if len(reordered) < 2 {
		return nil, nil
	}
	resource := strings.ToLower(strings.TrimSpace(reordered[1]))
	key := kubectlGetDispatchKey(resource)
	if key == "" {
		return nil, nil
	}
	f := reg.Resolve(key)
	if f == nil {
		return nil, nil
	}
	subArgs := append([]string{}, reordered[2:]...)
	subArgs = append(subArgs, moved...)
	return f, subArgs
}

func hasKubectlLogsFollowFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-f" || arg == "--follow" {
			return true
		}
	}
	return false
}

func hasStructuredOutputFlag(args []string) bool {
	v, ok := filtercommon.OptionValueAny(args, "-o", "--output")
	if !ok {
		return false
	}
	value := filtercommon.LowerTrim(v)
	return value == "yaml" || value == "json" || strings.HasPrefix(value, "jsonpath") || value == "name"
}
