package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/SuppieRK/cmdshape/internal/lifecycle/agents"
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"github.com/SuppieRK/cmdshape/internal/workspaces"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type toolState struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func RunInit(args []string) error {
	fs := newLifecycleFlagSet("init")
	toolsArg := fs.String("tools", "", "comma-separated tool names (optional: auto-detect when omitted)")
	setLifecycleUsage(
		fs,
		"install or update supported agent integrations",
		[]string{"cmdshape init [--tools <tool,tool,...>]"},
		"When --tools is omitted, cmdshape auto-detects supported tools from the current repository.",
		"cmdshape refreshes the fully managed ~/.config/cmdshape directory, including ~/.config/cmdshape/filters, from shipped resources embedded in the binary.",
		"Repository markers drive detection only; each integration manages its own canonical install target.",
		"Agent adapters may install into home-scoped locations when that is the supported integration surface.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	adapters, err := agents.NewBuiltInAdapters()
	if err != nil {
		return err
	}
	tools, err := resolveInitTools(*toolsArg, adapters)
	if err != nil {
		return err
	}
	if err := agents.ValidateSelectedTools(tools, adapters); err != nil {
		return err
	}
	scopeRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	states, err := applyAdapters(
		agents.Context{ScopeRoot: scopeRoot, HomeDir: homeDir},
		tools,
		adapters,
	)
	if err != nil {
		return err
	}
	registryPath, err := workspaces.DefaultPath()
	if err == nil {
		err = workspaces.UpsertPath(registryPath, scopeRoot, "")
	}
	if err != nil {
		writeLifecycleWarning("cmdshape init: warning: could not update workspace registry: %v\n", err)
	}
	if allToolStatesNoop(states) {
		fmt.Printf("cmdshape init: already configured\n")
		return nil
	}
	fmt.Printf("cmdshape init: configured integrations\n")
	return nil
}

func resolveInitTools(toolsArg string, adapters map[string]agents.Adapter) ([]string, error) {
	tools := parseTools(toolsArg)
	if len(tools) > 0 {
		return tools, nil
	}
	scopeRoot, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	tools = agents.DetectTools(scopeRoot, adapters)
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools detected; specify --tools (%s)", strings.Join(agents.SupportedTools(adapters), ", "))
	}
	fmt.Printf("cmdshape init: detected tools: %s\n", strings.Join(tools, ", "))
	return tools, nil
}

func parseTools(input string) []string {
	parts := strings.Split(input, ",")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		t := agents.NormalizeToolID(strings.TrimSpace(strings.ToLower(p)))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func applyAdapters(scope agents.Context, tools []string, adapters map[string]agents.Adapter) ([]toolState, error) {
	var states []toolState
	for _, tool := range tools {
		adapter := adapters[tool]
		plan := adapter.Plan(scope)
		if len(plan) > 0 {
			fmt.Printf("cmdshape init: planned %d changes for %s\n", len(plan), tool)
		}
		res, err := adapter.Install(scope, writeManagedBytes)
		if err != nil {
			states = append(states, toolState{Tool: tool, Status: "failed", Reason: err.Error()})
			return states, err
		}
		applied, noops := res.Applied, res.Noop

		if err := adapter.Verify(scope); err != nil {
			states = append(states, toolState{Tool: tool, Status: "failed", Reason: err.Error()})
			return states, err
		}

		status := "applied"
		reason := fmt.Sprintf("applied=%d noop=%d", applied, noops)
		if applied == 0 && noops > 0 {
			status = "noop"
		}
		states = append(states, toolState{Tool: tool, Status: status, Reason: reason})
		fmt.Printf("cmdshape init: [%s] status=%s (%s)\n", tool, status, reason)
	}
	return states, nil
}

func allToolStatesNoop(states []toolState) bool {
	if len(states) == 0 {
		return true
	}
	for _, state := range states {
		if state.Status != "noop" {
			return false
		}
	}
	return true
}

func writeManagedBytes(path string, data []byte, perm os.FileMode) (changed bool, err error) {
	if err := projectfiles.RejectSymlinkPath(path); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if bytes.Equal(old, data) {
		return false, nil
	}
	if err := projectfiles.AtomicWriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}
