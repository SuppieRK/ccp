package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const opencodePluginName = "ccp-rewrite.js"

type OpenCodeAdapter struct{}

func NewOpenCodeAdapter() OpenCodeAdapter { return OpenCodeAdapter{} }
func (a OpenCodeAdapter) ID() string      { return string(AgentOpenCode) }
func (a OpenCodeAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, ".opencode")
}

func (a OpenCodeAdapter) Plan(ctx Context) []PlannedArtifact {
	pluginPath := opencodePluginPath(ctx)
	return []PlannedArtifact{
		{
			Kind:    ArtifactSettings,
			Path:    pluginPath,
			Content: opencodePluginContent(),
			Perm:    0o644,
		},
	}
}

func (a OpenCodeAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return InstallPlannedArtifacts(a.Plan(ctx), write)
}

func (a OpenCodeAdapter) Verify(ctx Context) error {
	pluginPath := opencodePluginPath(ctx)
	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf("missing opencode plugin file: %s", pluginPath)
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
		{snippet: `"tool.execute.before"`, msg: "opencode plugin missing tool.execute.before hook: %s"},
		{snippet: `input.tool !== "bash"`, msg: "opencode plugin missing bash-only guard: %s"},
	} {
		if !strings.Contains(content, req.snippet) {
			return fmt.Errorf(req.msg, pluginPath)
		}
	}
	return nil
}

func (a OpenCodeAdapter) Uninstall(ctx Context) (InstallResult, error) {
	pluginPath := opencodePluginPath(ctx)
	removed, err := removeFileIfExists(pluginPath)
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		return InstallResult{Applied: 1}, nil
	}
	return InstallResult{Noop: 1}, nil
}

func opencodePluginPath(ctx Context) string {
	return filepath.Join(opencodeConfigRoot(ctx), "plugins", opencodePluginName)
}

func opencodeConfigRoot(ctx Context) string {
	if strings.TrimSpace(ctx.HomeDir) != "" && filepath.Clean(ctx.ScopeRoot) == filepath.Clean(ctx.HomeDir) {
		return filepath.Join(ctx.HomeDir, ".config", "opencode")
	}
	return filepath.Join(ctx.ScopeRoot, ".opencode")
}

func opencodePluginContent() string {
	return `export default async function ccpRewritePlugin() {
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") {
        return;
      }
      const command = output?.args?.command;
      if (typeof command !== "string") {
        return;
      }
      const trimmed = command.trimStart();
      if (trimmed === "ccp" || trimmed.startsWith("ccp ")) {
        return;
      }
      // Conservative safety fallback: avoid rewrite for complex quoting/substitution shapes.
      if (/['"\\]|\$\(|\$\{|<</.test(command)) {
        return;
      }
      const rewritten = command.replace(/(^|\|\||&&|\||;)\s*(?!ccp\b)/g, "$1 ccp ");
      if (rewritten === command) {
        return;
      }
      output.args = output.args || {};
      output.args.command = rewritten;
    },
  };
}
`
}
