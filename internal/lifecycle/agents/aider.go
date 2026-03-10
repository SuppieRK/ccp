package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	aiderConfigPath = ".aider.conf.yml"
	aiderRulesPath  = ".aider.rules.md"
)

type AiderAdapter struct{}

func NewAiderAdapter() AiderAdapter {
	return AiderAdapter{}
}

func (a AiderAdapter) ID() string { return string(AgentAider) }

func (a AiderAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, aiderConfigPath)
}

func (a AiderAdapter) Detect(scopeRoot string) bool {
	st, err := os.Stat(a.DetectRoot(scopeRoot))
	return err == nil && !st.IsDir()
}

func (a AiderAdapter) targetPath(ctx Context) string {
	return ResolveHomeScopedPath(ctx.HomeDir, aiderConfigPath)
}

func (a AiderAdapter) rulesPath(ctx Context) string {
	return ResolveHomeScopedPath(ctx.HomeDir, aiderRulesPath)
}

func (a AiderAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{
		{
			Kind:    ArtifactSettings,
			Path:    a.targetPath(ctx),
			Content: fmt.Sprintf("read:\n  - %s\n", a.rulesPath(ctx)),
			Perm:    0o644,
		},
		{
			Kind:    ArtifactSettings,
			Path:    a.rulesPath(ctx),
			Content: ccpManagedBlockTemplate(),
			Perm:    0o644,
		},
	}
}

func (a AiderAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	target := a.targetPath(ctx)
	readPath := a.rulesPath(ctx)
	content, err := upsertAiderReadConfig(target, readPath)
	if err != nil {
		return InstallResult{}, err
	}
	configChanged, err := write(target, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	rulesContent, err := upsertManagedInstructionBlock(readPath)
	if err != nil {
		return InstallResult{}, err
	}
	rulesChanged, err := write(readPath, []byte(rulesContent), 0o644)
	if err != nil {
		return InstallResult{}, err
	}

	res := InstallResult{}
	for _, changed := range []bool{configChanged, rulesChanged} {
		if changed {
			res.Applied++
		} else {
			res.Noop++
		}
	}
	return res, nil
}

func (a AiderAdapter) Verify(ctx Context) error {
	target := a.targetPath(ctx)
	readPath := a.rulesPath(ctx)
	ok, err := aiderConfigHasRead(target, readPath)
	if err != nil {
		return fmt.Errorf("missing aider config file: %s", target)
	}
	if !ok {
		return fmt.Errorf("missing aider managed read guidance in %s", target)
	}
	if err := verifyManagedInstructionBlock(readPath, "missing aider rules file: %s", "missing aider managed block markers in %s"); err != nil {
		return err
	}
	return nil
}

func (a AiderAdapter) Uninstall(ctx Context) (InstallResult, error) {
	target := a.targetPath(ctx)
	readPath := a.rulesPath(ctx)
	updated, changed, removeAll, err := removeAiderReadConfig(target, readPath)
	if err != nil {
		return InstallResult{}, err
	}
	res, err := applyAiderUninstallChange(target, updated, changed, removeAll)
	if err != nil {
		return InstallResult{}, err
	}

	rulesUpdated, rulesChanged, rulesRemoveAll, err := removeManagedInstructionBlock(readPath)
	if err != nil {
		return InstallResult{}, err
	}
	rulesRes, err := applyAiderUninstallChange(readPath, rulesUpdated, rulesChanged, rulesRemoveAll)
	if err != nil {
		return InstallResult{}, err
	}
	res.Applied += rulesRes.Applied
	res.Noop += rulesRes.Noop
	return res, nil
}

func applyAiderUninstallChange(path, updated string, changed, removeAll bool) (InstallResult, error) {
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	if removeAll {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		return InstallResult{Applied: 1}, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}

func upsertAiderReadConfig(path, readPath string) (string, error) {
	cfg, err := readAiderConfig(path)
	if err != nil {
		return "", err
	}
	read := normalizeAiderRead(cfg["read"])
	if !containsAiderReadEntry(read, readPath) {
		read = append(read, readPath)
	}
	cfg["read"] = read
	return marshalAiderConfig(cfg)
}

func removeAiderReadConfig(path, readPath string) (updated string, changed bool, removeAll bool, err error) {
	cfg, err := readAiderConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	read := normalizeAiderRead(cfg["read"])
	next := make([]string, 0, len(read))
	found := false
	for _, entry := range read {
		if entry == readPath {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return "", false, false, nil
	}
	if len(next) == 0 {
		delete(cfg, "read")
	} else {
		cfg["read"] = next
	}
	if len(cfg) == 0 {
		return "", true, true, nil
	}
	content, err := marshalAiderConfig(cfg)
	if err != nil {
		return "", false, false, err
	}
	return content, true, false, nil
}

func aiderConfigHasRead(path, readPath string) (bool, error) {
	cfg, err := readAiderConfig(path)
	if err != nil {
		return false, err
	}
	return containsAiderReadEntry(normalizeAiderRead(cfg["read"]), readPath), nil
}

func readAiderConfig(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	cfg := map[string]any{}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func marshalAiderConfig(cfg map[string]any) (string, error) {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func normalizeAiderRead(v any) []string {
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		if val == "" {
			return nil
		}
		return []string{val}
	case []string:
		out := make([]string, 0, len(val))
		for _, entry := range val {
			if entry != "" {
				out = append(out, entry)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(val))
		for _, raw := range val {
			if s, ok := raw.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsAiderReadEntry(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
