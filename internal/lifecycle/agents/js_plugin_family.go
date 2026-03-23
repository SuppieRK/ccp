package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type jsPluginVerifyRequirement struct {
	Snippet string
	Msg     string
}

type ManagedJSPluginAdapterSpec struct {
	ID                 ID
	DetectRootPath     string
	ConfigDirName      string
	MissingFileFmt     string
	VerifyRequirements []jsPluginVerifyRequirement
}

type ManagedJSPluginAdapter struct {
	spec ManagedJSPluginAdapterSpec
}

const managedJSPluginFileName = "ccp-rewrite.js"

var openCodeJSPluginSpec = ManagedJSPluginAdapterSpec{
	ID:             AgentOpenCode,
	DetectRootPath: ".opencode",
	ConfigDirName:  "opencode",
	MissingFileFmt: "missing opencode plugin file: %s",
	VerifyRequirements: []jsPluginVerifyRequirement{
		{Snippet: `"tool.execute.before"`, Msg: "opencode plugin missing tool.execute.before hook: %s"},
		{Snippet: `input.tool !== "bash"`, Msg: "opencode plugin missing bash-only guard: %s"},
	},
}

func NewManagedJSPluginAdapter(spec ManagedJSPluginAdapterSpec) ManagedJSPluginAdapter {
	return ManagedJSPluginAdapter{spec: spec}
}

func (a ManagedJSPluginAdapter) ID() string { return string(a.spec.ID) }

func (a ManagedJSPluginAdapter) DetectRoot(scopeRoot string) string {
	return ResolveRepoScopedPath(scopeRoot, a.spec.DetectRootPath)
}

func (a ManagedJSPluginAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    managedJSPluginPath(ctx, a.spec.ConfigDirName),
		Content: managedBashRewritePluginContent(),
		Perm:    0o644,
	}}
}

func (a ManagedJSPluginAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return InstallPlannedArtifacts(a.Plan(ctx), write)
}

func (a ManagedJSPluginAdapter) Verify(ctx Context) error {
	pluginPath := managedJSPluginPath(ctx, a.spec.ConfigDirName)
	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf(a.spec.MissingFileFmt, pluginPath)
	}
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		return err
	}
	content := string(data)
	for _, req := range a.spec.VerifyRequirements {
		if !strings.Contains(content, req.Snippet) {
			return fmt.Errorf(req.Msg, pluginPath)
		}
	}
	return nil
}

func (a ManagedJSPluginAdapter) Uninstall(ctx Context) (InstallResult, error) {
	pluginPath := managedJSPluginPath(ctx, a.spec.ConfigDirName)
	removed, err := removeFileIfExists(pluginPath)
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		return InstallResult{Applied: 1}, nil
	}
	return InstallResult{Noop: 1}, nil
}

func managedJSPluginPath(ctx Context, configDirName string) string {
	return filepath.Join(managedJSPluginConfigRoot(ctx, configDirName), "plugins", managedJSPluginFileName)
}

func managedJSPluginConfigRoot(ctx Context, configDirName string) string {
	if strings.TrimSpace(ctx.HomeDir) != "" {
		return filepath.Join(ctx.HomeDir, ".config", configDirName)
	}
	return filepath.Join(ctx.ScopeRoot, "."+configDirName)
}

func NewOpenCodeAdapter() ManagedJSPluginAdapter {
	return NewManagedJSPluginAdapter(openCodeJSPluginSpec)
}

func managedBashRewritePluginContent() string {
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
	      if (/\$\(|\$\{|<</.test(command)) {
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
