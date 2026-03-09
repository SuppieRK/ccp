package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go-command-compression-proxy/internal/lifecycle/agents"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type initConfig struct {
	Tools []string    `json:"tools"`
	State []toolState `json:"state"`
}

type toolState struct {
	Tool   string `json:"tool"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// RunInit persists tool-selection configuration into ccp managed state.
func RunInit(args []string) error {
	fs := newLifecycleFlagSet("init")
	toolsArg := fs.String("tools", "", "comma-separated tool names (optional: auto-detect when omitted)")
	setLifecycleUsage(
		fs,
		"install or update supported agent integrations",
		[]string{"ccp init [--tools <tool,tool,...>]"},
		"When --tools is omitted, ccp auto-detects supported tools from the current repository.",
		"ccp stores init state at ~/.config/ccp/init.json.",
		"Each integration manages its own install target; agent adapters may use home-scoped locations.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	adapters := agents.DefaultAdapters()
	tools, err := resolveInitTools(*toolsArg, adapters)
	if err != nil {
		return err
	}
	if err := agents.ValidateSelectedTools(tools, adapters); err != nil {
		return err
	}

	path, err := initPath()
	if err != nil {
		return err
	}
	scopeRoot, err := initDetectRoot()
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
	cfg := initConfig{
		Tools: tools,
		State: states,
	}
	changed, err := persistInitConfig(path, cfg)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("ccp init: already configured (%s)\n", path)
		return nil
	}
	fmt.Printf("ccp init: configured integrations at %s\n", path)
	return nil
}

func resolveInitTools(toolsArg string, adapters map[string]agents.Adapter) ([]string, error) {
	tools := parseTools(toolsArg)
	if len(tools) > 0 {
		return tools, nil
	}
	scopeRoot, err := initDetectRoot()
	if err != nil {
		return nil, err
	}
	tools = agents.DetectTools(scopeRoot, adapters)
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools detected; specify --tools (%s)", strings.Join(agents.SupportedTools(adapters), ", "))
	}
	fmt.Printf("ccp init: detected tools: %s\n", strings.Join(tools, ", "))
	return tools, nil
}

func persistInitConfig(path string, cfg initConfig) (bool, error) {
	newBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	newBytes = append(newBytes, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	oldBytes, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if bytes.Equal(oldBytes, newBytes) {
		return false, nil
	}
	if len(oldBytes) > 0 {
		backupPath := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
		if err := os.WriteFile(backupPath, oldBytes, 0o644); err != nil {
			return false, err
		}
		fmt.Printf("ccp init: backup created at %s\n", backupPath)
	}
	if err := os.WriteFile(path, newBytes, 0o644); err != nil {
		return false, err
	}
	return true, nil
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

func initDetectRoot() (string, error) {
	return os.Getwd()
}

func initPath() (string, error) {
	base, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, ".config", "ccp", "init.json"), nil
}

func applyAdapters(scope agents.Context, tools []string, adapters map[string]agents.Adapter) ([]toolState, error) {
	var states []toolState
	for _, tool := range tools {
		adapter := adapters[tool]
		plan := adapter.Plan(scope)
		if len(plan) > 0 {
			fmt.Printf("ccp init: planned %d changes for %s\n", len(plan), tool)
		}
		res, err := adapter.Install(scope, writeManagedFile)
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
		fmt.Printf("ccp init: [%s] status=%s (%s)\n", tool, status, reason)
	}
	return states, nil
}

func writeManagedFile(path string, data []byte, perm os.FileMode) (changed bool, err error) {
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
	if len(old) > 0 {
		backupPath := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
		if err := os.WriteFile(backupPath, old, 0o644); err != nil {
			return false, err
		}
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}
