package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	crushContextRelPath = ".config/crush/CRUSH.md"
	crushConfigRelPath  = ".config/crush/crush.json"
	crushConfigErrFmt   = "invalid crush config file: %s"
)

type CrushAdapter struct {
	ManagedInstructionFileAdapter
}

func NewCrushAdapter() CrushAdapter {
	return CrushAdapter{
		ManagedInstructionFileAdapter: NewManagedInstructionFileAdapter(
			string(AgentCrush),
			".crush",
			crushContextRelPath,
			"missing crush context file: %s",
			"missing crush managed block markers in %s",
		),
	}
}

func (a CrushAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".crush")
}

func (a CrushAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	res, err := a.ManagedInstructionFileAdapter.Install(ctx, write)
	if err != nil {
		return InstallResult{}, err
	}
	configPath := ResolveHomeScopedPath(ctx.HomeDir, crushConfigRelPath)
	contextPath := ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath)
	content, err := upsertCrushConfig(configPath, contextPath)
	if err != nil {
		return InstallResult{}, err
	}
	changed, err := write(configPath, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	if changed {
		res.Applied++
	} else {
		res.Noop++
	}
	return res, nil
}

func (a CrushAdapter) Verify(ctx Context) error {
	if err := a.ManagedInstructionFileAdapter.Verify(ctx); err != nil {
		return err
	}
	configPath := ResolveHomeScopedPath(ctx.HomeDir, crushConfigRelPath)
	contextPath := ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath)
	ok, err := crushConfigUsesContext(configPath, contextPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing crush managed context path in %s", configPath)
	}
	return nil
}

func (a CrushAdapter) Uninstall(ctx Context) (InstallResult, error) {
	res, err := a.ManagedInstructionFileAdapter.Uninstall(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	configPath := ResolveHomeScopedPath(ctx.HomeDir, crushConfigRelPath)
	contextPath := ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath)
	updated, changed, removeAll, err := removeCrushContextPath(configPath, contextPath)
	if err != nil {
		return InstallResult{}, err
	}
	if !changed {
		res.Noop++
		return res, nil
	}
	if removeAll {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		res.Applied++
		return res, nil
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	res.Applied++
	return res, nil
}

func upsertCrushConfig(configPath, contextPath string) (string, error) {
	root := map[string]any{}
	if raw, err := os.ReadFile(configPath); err == nil {
		if json.Unmarshal(raw, &root) != nil {
			return "", fmt.Errorf(crushConfigErrFmt, configPath)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	options, _ := root["options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
		root["options"] = options
	}
	paths := crushContextPaths(options["context_paths"])
	if !slicesContainsPath(paths, contextPath) {
		paths = append(paths, contextPath)
	}
	options["context_paths"] = paths
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(out, '\n')), nil
}

func crushConfigUsesContext(configPath, contextPath string) (bool, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return false, fmt.Errorf(crushConfigErrFmt, configPath)
	}
	options, _ := root["options"].(map[string]any)
	if options == nil {
		return false, nil
	}
	return slicesContainsPath(crushContextPaths(options["context_paths"]), contextPath), nil
}

func removeCrushContextPath(configPath, contextPath string) (updated string, changed bool, removeAll bool, err error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return "", false, false, fmt.Errorf(crushConfigErrFmt, configPath)
	}
	options, _ := root["options"].(map[string]any)
	if options == nil {
		return "", false, false, nil
	}
	paths := crushContextPaths(options["context_paths"])
	next := make([]string, 0, len(paths))
	found := false
	for _, path := range paths {
		if filepath.Clean(strings.TrimSpace(path)) == filepath.Clean(strings.TrimSpace(contextPath)) {
			found = true
			continue
		}
		next = append(next, path)
	}
	if !found {
		return "", false, false, nil
	}
	if len(next) == 0 {
		delete(options, "context_paths")
	} else {
		options["context_paths"] = next
	}
	if len(options) == 0 {
		delete(root, "options")
	}
	if len(root) == 0 {
		return "", true, true, nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", false, false, err
	}
	return string(append(out, '\n')), true, false, nil
}

func crushContextPaths(v any) []string {
	items, _ := v.([]any)
	if len(items) == 0 {
		if strings.TrimSpace(fmt.Sprint(v)) == "" || v == nil {
			return nil
		}
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		paths = append(paths, s)
	}
	return paths
}

func slicesContainsPath(paths []string, contextPath string) bool {
	normalized := filepath.Clean(strings.TrimSpace(contextPath))
	for _, path := range paths {
		if filepath.Clean(strings.TrimSpace(path)) == normalized {
			return true
		}
	}
	return false
}
