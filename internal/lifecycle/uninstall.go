package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go-command-compression-proxy/internal/lifecycle/agents"
	"go-command-compression-proxy/internal/workspaces"
)

var (
	uninstallExecutablePath      = os.Executable
	uninstallScheduleSelfRemoval = scheduleExecutableRemoval
)

func RunUninstall(args []string) error {
	fs := newLifecycleFlagSet("uninstall")
	toolsArg := fs.String("tools", "", "comma-separated tool names (optional: limit uninstall to selected integrations)")
	setLifecycleUsage(
		fs,
		"remove ccp integrations or fully uninstall ccp",
		[]string{"ccp uninstall [--tools <tool,tool,...>]"},
		"When --tools is provided, ccp removes only the selected integrations from their canonical install targets.",
		"When --tools is omitted, ccp performs a complete uninstall of managed integrations, recorded workspace state, and global CCP files before removing the executable.",
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
	if tools := parseTools(*toolsArg); len(tools) > 0 {
		return runToolScopedUninstall(tools, adapters)
	}
	return runCompleteUninstall(adapters)
}

func runToolScopedUninstall(tools []string, adapters map[string]agents.Adapter) error {
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
	_, err = applyUninstallAdapters(agents.Context{ScopeRoot: scopeRoot, HomeDir: homeDir}, tools, adapters)
	return err
}

func runCompleteUninstall(adapters map[string]agents.Adapter) error {
	scopeRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	entries, err := workspaces.ListPath(workspaces.PathForHome(homeDir))
	if err != nil {
		writeLifecycleWarning("ccp uninstall: warning: could not read workspace registry: %v\n", err)
		writeLifecycleWarning("ccp uninstall: warning: repo-scoped CCP files in other workspaces may remain; manual repo cleanup example:\n  %s\n", manualRepoUninstallCommand())
		writeLifecycleWarning("ccp uninstall: warning: review mixed-content files like AGENTS.md in other repos for remaining CCP managed blocks\n")
		entries = nil
	}
	scopes := uninstallScopes(scopeRoot, entries)
	tools := canonicalUninstallTools(adapters)
	for _, scope := range scopes {
		if _, err := applyUninstallAdapters(agents.Context{ScopeRoot: scope, HomeDir: homeDir}, tools, adapters); err != nil {
			return err
		}
	}

	if err := removeWorkspaceState(scopes, entries); err != nil {
		return err
	}
	if err := removeGlobalCCPState(homeDir); err != nil {
		return err
	}

	exePath, err := uninstallExecutablePath()
	if err != nil {
		return err
	}
	if err := uninstallScheduleSelfRemoval(exePath); err != nil {
		return err
	}
	fmt.Printf("ccp uninstall: scheduled executable removal (%s)\n", exePath)
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

func canonicalUninstallTools(adapters map[string]agents.Adapter) []string {
	tools := make([]string, 0, len(adapters))
	for id := range adapters {
		tools = append(tools, id)
	}
	slices.Sort(tools)
	return tools
}

func uninstallScopes(currentScope string, entries []workspaces.Workspace) []string {
	scopes := make([]string, 0, len(entries)+1)
	seen := make(map[string]struct{}, len(entries)+1)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		scopes = append(scopes, path)
	}
	add(currentScope)
	for _, entry := range entries {
		add(entry.CWD)
	}
	return scopes
}

func removeWorkspaceState(scopes []string, entries []workspaces.Workspace) error {
	var errs []error
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if err := removeManagedWorkspaceState(scope); err != nil {
			errs = append(errs, err)
		}
	}
	for _, entry := range entries {
		metricsPath, ok := managedWorkspaceMetricsPath(entry)
		if !ok {
			continue
		}
		if err := os.Remove(metricsPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove metrics path %q: %w", entry.MetricsPath, err))
		}
	}
	return errors.Join(errs...)
}

func removeManagedWorkspaceState(scope string) error {
	ccpDir := filepath.Join(scope, ".ccp")
	targets, err := managedWorkspaceStatePaths(ccpDir)
	if err != nil {
		return fmt.Errorf("resolve workspace state %q: %w", ccpDir, err)
	}

	var errs []error
	for _, path := range targets {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove workspace state %q: %w", path, err))
		}
	}
	if err := removeEmptyDirsUnder(ccpDir); err != nil {
		errs = append(errs, fmt.Errorf("cleanup workspace state %q: %w", ccpDir, err))
	}
	return errors.Join(errs...)
}

func managedWorkspaceStatePaths(ccpDir string) ([]string, error) {
	targets := []string{
		filepath.Join(ccpDir, "gain.db"),
		filepath.Join(ccpDir, "init.json"),
	}
	matches, err := filepath.Glob(filepath.Join(ccpDir, "init.json.bak.*"))
	if err != nil {
		return nil, err
	}
	targets = append(targets, matches...)
	return targets, nil
}

func removeEmptyDirsUnder(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyDirsUnder(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(root); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func managedWorkspaceMetricsPath(entry workspaces.Workspace) (string, bool) {
	if strings.TrimSpace(entry.CWD) == "" || strings.TrimSpace(entry.MetricsPath) == "" {
		return "", false
	}
	workspaceRoot := filepath.Clean(entry.CWD)
	metricsPath := filepath.Clean(entry.MetricsPath)
	canonical := filepath.Join(workspaceRoot, ".ccp", "gain.db")
	if metricsPath != canonical {
		return "", false
	}
	return metricsPath, true
}

func removeGlobalCCPState(homeDir string) error {
	var errs []error
	for _, path := range []string{
		filepath.Join(homeDir, configDirName, "ccp"),
		filepath.Join(homeDir, ".ccp"),
	} {
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %q: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func manualRepoUninstallCommand() string {
	return `REPO=/path/to/repo && rm -f "$REPO/.ccp/gain.db" "$REPO/.ccp/init.json" "$REPO"/.ccp/init.json.bak.* "$REPO/.cursor/rules/ccp.mdc" "$REPO/.clinerules/ccp.md" "$REPO/.amazonq/rules/ccp.md" "$REPO/.trae/rules/ccp.md" "$REPO/.windsurf/rules/ccp.md"`
}

func writeLifecycleWarning(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
}
