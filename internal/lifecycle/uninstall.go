package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

// RunUninstall removes tool-specific integration artifacts and updates init state.
func RunUninstall(args []string) error {
	fs := newLifecycleFlagSet("uninstall")
	toolsArg := fs.String("tools", "", "comma-separated tool names (optional: auto-detect when omitted)")
	setLifecycleUsage(
		fs,
		"remove supported agent integrations",
		[]string{"ccp uninstall [--tools <tool,tool,...>]"},
		"When --tools is omitted, ccp uses configured tools or auto-detection from the current repository.",
		"ccp removes its managed init state from ~/.config/ccp/init.json when no configured tools remain.",
		"Each integration decides which managed files are removed during uninstall.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	adapters := agents.DefaultAdapters()
	tools, err := resolveUninstallTools(*toolsArg, adapters)
	if err != nil {
		return err
	}
	if err := agents.ValidateSelectedTools(tools, adapters); err != nil {
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
	if _, err := applyUninstallAdapters(agents.Context{ScopeRoot: scopeRoot, HomeDir: homeDir}, tools, adapters); err != nil {
		return err
	}

	if err := updateInitConfigAfterUninstall(tools); err != nil {
		return err
	}
	return nil
}

func applyUninstallAdapters(ctx agents.Context, tools []string, adapters map[string]agents.Adapter) ([]toolState, error) {
	var states []toolState
	for _, tool := range tools {
		adapter := adapters[tool]
		uninstaller, ok := adapter.(agents.Uninstaller)
		if !ok {
			states = append(states, toolState{Tool: tool, Status: "noop", Reason: "no uninstall action"})
			fmt.Printf("ccp uninstall: [%s] status=noop (no uninstall action)\n", tool)
			continue
		}
		res, err := uninstaller.Uninstall(ctx)
		status := "removed"
		if err != nil {
			states = append(states, toolState{Tool: tool, Status: "failed", Reason: err.Error()})
			return states, err
		}
		reason := fmt.Sprintf("removed=%d noop=%d", res.Applied, res.Noop)
		if res.Applied == 0 {
			status = "noop"
		}
		states = append(states, toolState{Tool: tool, Status: status, Reason: reason})
		fmt.Printf("ccp uninstall: [%s] status=%s (%s)\n", tool, status, reason)
	}
	return states, nil
}

func updateInitConfigAfterUninstall(removedTools []string) error {
	path, err := initPath()
	if err != nil {
		return err
	}
	cfg, oldBytes, exists, err := readInitConfigForUninstall(path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	removedSet := buildRemovedToolSet(removedTools)
	nextTools, nextStates := filterUninstalledTools(cfg, removedSet)

	if len(nextTools) == 0 {
		return removeInitConfigAfterUninstall(path)
	}

	cfg.Tools = nextTools
	cfg.State = nextStates
	return writeUpdatedInitConfig(path, cfg, oldBytes)
}

func readInitConfigForUninstall(path string) (initConfig, []byte, bool, error) {
	oldBytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return initConfig{}, nil, false, nil
		}
		return initConfig{}, nil, false, err
	}
	var cfg initConfig
	if json.Unmarshal(oldBytes, &cfg) != nil {
		return initConfig{}, nil, false, nil
	}
	return cfg, oldBytes, true, nil
}

func buildRemovedToolSet(removedTools []string) map[string]bool {
	removedSet := map[string]bool{}
	for _, t := range removedTools {
		removedSet[t] = true
	}
	return removedSet
}

func filterUninstalledTools(cfg initConfig, removedSet map[string]bool) ([]string, []toolState) {
	nextTools := make([]string, 0, len(cfg.Tools))
	for _, t := range cfg.Tools {
		if !removedSet[t] {
			nextTools = append(nextTools, t)
		}
	}
	nextStates := make([]toolState, 0, len(cfg.State))
	for _, st := range cfg.State {
		if !removedSet[st.Tool] {
			nextStates = append(nextStates, st)
		}
	}
	return nextTools, nextStates
}

func removeInitConfigAfterUninstall(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("ccp uninstall: removed config at %s\n", path)
	return nil
}

func writeUpdatedInitConfig(path string, cfg initConfig, oldBytes []byte) error {
	newBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	newBytes = append(newBytes, '\n')
	if bytes.Equal(oldBytes, newBytes) {
		return nil
	}
	backupPath := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
	if err := os.WriteFile(backupPath, oldBytes, 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, newBytes, 0o644); err != nil {
		return err
	}
	fmt.Printf("ccp uninstall: updated config at %s\n", path)
	return nil
}

func loadConfiguredTools(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg initConfig
	if json.Unmarshal(b, &cfg) != nil {
		return nil, errors.New("invalid init config")
	}
	return cfg.Tools, nil
}

func joinTools(adapters map[string]agents.Adapter) string {
	return strings.Join(agents.SupportedTools(adapters), ", ")
}

func resolveUninstallTools(toolsArg string, adapters map[string]agents.Adapter) ([]string, error) {
	tools := parseTools(toolsArg)
	if len(tools) > 0 {
		return tools, nil
	}
	path, err := initPath()
	if err != nil {
		return nil, err
	}
	if cfgTools, err := loadConfiguredTools(path); err == nil && len(cfgTools) > 0 {
		return cfgTools, nil
	}

	scopeRoot, err := initDetectRoot()
	if err != nil {
		return nil, err
	}
	tools = agents.DetectTools(scopeRoot, adapters)
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools detected; specify --tools (%s)", joinTools(adapters))
	}
	fmt.Printf("ccp uninstall: detected tools: %v\n", tools)
	return tools, nil
}
