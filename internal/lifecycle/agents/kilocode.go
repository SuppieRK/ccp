package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type KilocodeAdapter struct{}

func NewKilocodeAdapter() KilocodeAdapter { return KilocodeAdapter{} }
func (a KilocodeAdapter) ID() string      { return string(AgentKilocode) }

func (a KilocodeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".kilocode")
}

func (a KilocodeAdapter) Plan(ctx Context) []PlannedArtifact {
	pluginPath := kilocodePluginPath(ctx)
	return []PlannedArtifact{
		{
			Kind:    ArtifactSettings,
			Path:    pluginPath,
			Content: opencodePluginContent(),
			Perm:    0o644,
		},
	}
}

func (a KilocodeAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	res, err := InstallPlannedArtifacts(a.Plan(ctx), write)
	if err != nil {
		return InstallResult{}, err
	}
	return res, nil
}

func (a KilocodeAdapter) Verify(ctx Context) error {
	pluginPath := kilocodePluginPath(ctx)
	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf("missing kilocode plugin file: %s", pluginPath)
	}
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		return err
	}
	content := string(data)
	for _, req := range []struct {
		snippet string
		msg     string
	}{
		{snippet: `"tool.execute.before"`, msg: "kilocode plugin missing tool.execute.before hook: %s"},
		{snippet: `input.tool !== "bash"`, msg: "kilocode plugin missing bash-only guard: %s"},
	} {
		if !strings.Contains(content, req.snippet) {
			return fmt.Errorf(req.msg, pluginPath)
		}
	}
	return nil
}

func (a KilocodeAdapter) Uninstall(ctx Context) (InstallResult, error) {
	pluginPath := kilocodePluginPath(ctx)
	removed, err := removeFileIfExists(pluginPath)
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		return InstallResult{Applied: 1}, nil
	}
	return InstallResult{Noop: 1}, nil
}

func kilocodePluginPath(ctx Context) string {
	return filepath.Join(kilocodeConfigRoot(ctx), "plugins", opencodePluginName)
}

func kilocodeConfigRoot(ctx Context) string {
	if strings.TrimSpace(ctx.HomeDir) != "" {
		return filepath.Join(ctx.HomeDir, ".config", "kilocode")
	}
	return filepath.Join(ctx.ScopeRoot, ".kilocode")
}
