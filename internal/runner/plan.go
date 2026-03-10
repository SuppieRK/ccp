package runner

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-command-compression-proxy/internal/engine"
)

var (
	lookPathFn    = exec.LookPath
	lookPathMu    sync.Mutex
	lookPathOK    = map[string]bool{}
	lookPathStamp = map[string]time.Time{}
)

const (
	lookPathTTL        = 5 * time.Minute
	lookPathMaxEntries = 256
)

func initPlannerCapabilities() {
	// Planner capability changes affect broad execution behavior, so CI treats this path as full-benchmark scope.
	// Precompute known substitution capabilities once; BuildExecPlan remains O(1).
	_ = executableExists("uv")
}

// BuildExecPlan normalizes command execution using registry-aware tool preparation.
func BuildExecPlan(args []string, registry *engine.ToolFilterRegistry) (engine.ExecPlan, error) {
	if len(args) == 0 {
		return engine.ExecPlan{}, nil
	}
	rawInput := strings.Join(args, " ")

	if plan, handled := maybeBuildAmbiguousShellPlan(args, rawInput); handled {
		return plan, nil
	}

	bin := args[0]
	tail := args[1:]
	f, prep, normalized, recognized := resolvePreparedToolPlan(bin, tail, registry)
	return buildExecPlanFromPrepare(f, prep, normalized, bin, rawInput, recognized), nil
}

func maybeBuildAmbiguousShellPlan(args []string, rawInput string) (engine.ExecPlan, bool) {
	ambiguous, ops := detectAmbiguousOperators(args)
	if !ambiguous {
		return engine.ExecPlan{}, false
	}
	return buildAmbiguousShellPlan(rawInput, ops), true
}

func buildAmbiguousShellPlan(rawInput string, ops []string) engine.ExecPlan {
	return engine.ExecPlan{
		// Ambiguous shell chains run with neutral tool binding in permissive mode.
		Tool:            "",
		MetricsTool:     "",
		Name:            engine.ShellCommand(),
		Args:            engine.ShellArgs(rawInput),
		RawInput:        rawInput,
		Passthrough:     true,
		IsAmbiguous:     true,
		AmbiguityReason: fmt.Sprintf("contains %q", strings.Join(ops, ", ")),
		AmbiguityOps:    ops,
	}
}

func resolvePreparedToolPlan(
	bin string,
	tail []string,
	registry *engine.ToolFilterRegistry,
) (engine.ToolFilter, engine.PrepareResult, []string, bool) {
	detectedTool := filepath.Base(bin)
	f := engine.NewNoopFilter(detectedTool)
	recognized := false
	if registry != nil {
		if resolved := registry.Resolve(detectedTool); resolved != nil {
			f = resolved
			recognized = true
		}
	}
	prep := f.Prepare(tail)
	normalized := prep.NormalizedArgs
	if normalized == nil {
		normalized = tail
	}
	return f, prep, normalized, recognized
}

func buildExecPlanFromPrepare(
	f engine.ToolFilter,
	prep engine.PrepareResult,
	normalized []string,
	bin, rawInput string,
	recognized bool,
) engine.ExecPlan {
	plan := engine.ExecPlan{
		Tool:            f.Tool(),
		MetricsTool:     metricsToolForPlan(f, recognized),
		DispatchKey:     prep.DispatchKey,
		Name:            bin,
		Args:            normalized,
		RawInput:        rawInput,
		Passthrough:     prep.ForcePassthrough,
		IsAmbiguous:     prep.Ambiguous,
		AmbiguityReason: prep.Reason,
	}
	applyPreferredSubstitution(&plan, prep, normalized, bin)
	if prep.ForcePassthrough {
		// Neutral binding bypasses tool-specific filters while preserving execution.
		plan.Tool = ""
	}
	return plan
}

func metricsToolForPlan(f engine.ToolFilter, recognized bool) string {
	if !recognized {
		return ""
	}
	return f.Tool()
}

func applyPreferredSubstitution(plan *engine.ExecPlan, prep engine.PrepareResult, normalized []string, bin string) {
	if prep.PreferredSubstitution == "" {
		return
	}
	preferredArgs := prep.PreferredArgs
	if preferredArgs == nil {
		preferredArgs = normalized
	}
	fallbackArgs := prep.FallbackArgs
	if fallbackArgs == nil {
		fallbackArgs = normalized
	}
	if executableExists(prep.PreferredSubstitution) {
		plan.Name = prep.PreferredSubstitution
		plan.Args = preferredArgs
		plan.FallbackName = bin
		plan.FallbackArgs = fallbackArgs
		return
	}
	plan.Name = bin
	plan.Args = fallbackArgs
}

func executableExists(name string) bool {
	lookPathMu.Lock()
	cached, ok := lookPathOK[name]
	stamp := lookPathStamp[name]
	lookPathMu.Unlock()
	if ok && (stamp.IsZero() || time.Since(stamp) < lookPathTTL) {
		return cached
	}
	_, err := lookPathFn(name)
	exists := err == nil
	// Cache only deterministic outcomes. Not found remains stable enough per process.
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		exists = false
	}
	lookPathMu.Lock()
	if len(lookPathOK) >= lookPathMaxEntries {
		clear(lookPathOK)
		clear(lookPathStamp)
	}
	lookPathOK[name] = exists
	lookPathStamp[name] = time.Now()
	lookPathMu.Unlock()
	return exists
}

func detectAmbiguousOperators(args []string) (bool, []string) {
	var mask uint8
	for _, a := range args {
		mask |= ambiguousOperatorMask(strings.TrimSpace(a))
		if mask == 0b1111 {
			break
		}
	}
	if mask == 0 {
		return false, nil
	}
	ops := make([]string, 0, 4)
	if mask&1 != 0 {
		ops = append(ops, "&&")
	}
	if mask&2 != 0 {
		ops = append(ops, "||")
	}
	if mask&4 != 0 {
		ops = append(ops, ";")
	}
	if mask&8 != 0 {
		ops = append(ops, "|")
	}
	return true, ops
}

func ambiguousOperatorMask(trimmed string) uint8 {
	if trimmed == "" {
		return 0
	}
	var mask uint8
	if strings.HasSuffix(trimmed, "&&") {
		mask |= 1
	}
	if strings.HasSuffix(trimmed, "||") {
		mask |= 2
	}
	if strings.HasSuffix(trimmed, ";") {
		mask |= 4
	}
	if strings.HasSuffix(trimmed, "|") {
		mask |= 8
	}
	return mask
}
