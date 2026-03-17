package lifecycle

import (
	"fmt"
	"os"
	"strings"

	"go-command-compression-proxy/internal/lifecycle/agents"
)

func RunUninstall(args []string) error {
	fs := newLifecycleFlagSet("uninstall")
	toolsArg := fs.String("tools", "", "comma-separated tool names (optional: auto-detect when omitted)")
	setLifecycleUsage(
		fs,
		"remove supported agent integrations",
		[]string{"ccp uninstall [--tools <tool,tool,...>]"},
		"When --tools is omitted, ccp uses auto-detection from the current repository.",
		"Each integration removes managed artifacts from the same canonical install target used during init.",
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

func joinTools(adapters map[string]agents.Adapter) string {
	return strings.Join(agents.SupportedTools(adapters), ", ")
}

func resolveUninstallTools(toolsArg string, adapters map[string]agents.Adapter) ([]string, error) {
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
		return nil, fmt.Errorf("no tools detected; specify --tools (%s)", joinTools(adapters))
	}
	fmt.Printf("ccp uninstall: detected tools: %v\n", tools)
	return tools, nil
}
