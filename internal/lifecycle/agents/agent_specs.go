package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type BuiltInAdapterSpec struct {
	ID  ID
	New func() Adapter
}

const (
	codexAgentsPath               = ".codex/AGENTS.md"
	geminiInstructionsPath        = ".gemini/GEMINI.md"
	githubCopilotInstructionsPath = ".copilot/copilot-instructions.md"
	qoderAgentsPath               = ".qoder/AGENTS.md"
	kiroSteeringPath              = ".kiro/steering/AGENTS.md"
	factoryAgentsPath             = ".factory/AGENTS.md"
	auggieAgentsPath              = "AGENTS.md"
	piAppendSystemPath            = ".pi/APPEND_SYSTEM.md"
)

type managedRuleFileAdapterSpec struct {
	ID             ID
	DetectRootPath string
	TargetRelPath  string
	MissingFmt     string
	GuidanceFmt    string
	Render         func() string
	VerifyRequired []string
	TargetScope    managedRuleFileTargetScope
}

type managedRuleFileTargetScope string

const (
	managedRuleFileTargetRepo managedRuleFileTargetScope = "repo"
	managedRuleFileTargetHome managedRuleFileTargetScope = "home"
)

var managedRuleFileAdapterSpecs = []managedRuleFileAdapterSpec{
	{
		ID:             AgentAmazonQ,
		DetectRootPath: ".amazonq",
		TargetRelPath:  ".amazonq/rules/cmdshape.md",
		MissingFmt:     "missing amazon q rule file: %s",
		GuidanceFmt:    "missing amazon q managed guidance in %s",
		Render:         cmdshapeManagedGuidanceMarkdown,
		VerifyRequired: canonicalRuleVerificationSnippets(),
		TargetScope:    managedRuleFileTargetRepo,
	},
	{
		ID:             AgentCline,
		DetectRootPath: ".clinerules",
		TargetRelPath:  ".clinerules/cmdshape.md",
		MissingFmt:     "missing cline rule file: %s",
		GuidanceFmt:    "missing cline managed guidance in %s",
		Render:         cmdshapeManagedGuidanceMarkdown,
		VerifyRequired: canonicalRuleVerificationSnippets(),
		TargetScope:    managedRuleFileTargetRepo,
	},
	{
		ID:             AgentCursor,
		DetectRootPath: ".cursor",
		TargetRelPath:  ".cursor/rules/cmdshape.mdc",
		MissingFmt:     "missing cursor rule file: %s",
		GuidanceFmt:    "missing cursor managed guidance in %s",
		Render:         cursorRuleContent,
		VerifyRequired: []string{
			"alwaysApply: true",
			"Use `cmdshape` as the command prefix for every executable in shell commands",
			"`cmdshape nl -ba spec.md | cmdshape sed -n '1,260p'`",
			cmdshapeRawEscapeHatch,
			cmdshapeFilterPromptHint,
		},
		TargetScope: managedRuleFileTargetRepo,
	},
	{
		ID:             AgentRooCode,
		DetectRootPath: ".roo",
		TargetRelPath:  ".roo/rules/cmdshape.md",
		MissingFmt:     "missing roocode rule file: %s",
		GuidanceFmt:    "missing roocode managed guidance in %s",
		Render:         cmdshapeManagedGuidanceMarkdown,
		VerifyRequired: canonicalRuleVerificationSnippets(),
		TargetScope:    managedRuleFileTargetHome,
	},
	{
		ID:             AgentTrae,
		DetectRootPath: ".trae",
		TargetRelPath:  ".trae/rules/cmdshape.md",
		MissingFmt:     "missing trae rule file: %s",
		GuidanceFmt:    "missing trae managed guidance in %s",
		Render:         cmdshapeManagedGuidanceMarkdown,
		VerifyRequired: canonicalRuleVerificationSnippets(),
		TargetScope:    managedRuleFileTargetRepo,
	},
	{
		ID:             AgentWindsurf,
		DetectRootPath: ".windsurf",
		TargetRelPath:  ".windsurf/rules/cmdshape.md",
		MissingFmt:     "missing windsurf rule file: %s",
		GuidanceFmt:    "missing windsurf managed guidance in %s",
		Render:         cmdshapeManagedGuidanceMarkdown,
		VerifyRequired: canonicalRuleVerificationSnippets(),
		TargetScope:    managedRuleFileTargetRepo,
	},
}

var (
	antigravityContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentAntigravity,
		DetectRootPath: ".agent",
		TargetRelPath:  geminiInstructionsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing antigravity gemini-family instructions file: %s",
		MarkersFmt:     "missing antigravity managed block markers in %s",
	}
	auggieContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentAuggie,
		DetectRootPath: ".augment",
		TargetRelPath:  auggieAgentsPath,
		TargetScope:    managedContextTargetRepo,
		MissingFmt:     "missing auggie agents file: %s",
		MarkersFmt:     "missing auggie managed block markers in %s",
	}
	codexContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentCodex,
		DetectRootPath: ".codex",
		TargetRelPath:  codexAgentsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing codex agents file: %s",
		MarkersFmt:     "missing codex managed block markers in %s",
	}
	factoryContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentFactory,
		DetectRootPath: ".factory",
		TargetRelPath:  factoryAgentsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing factory agents file: %s",
		MarkersFmt:     "missing factory managed block markers in %s",
	}
	geminiContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentGemini,
		DetectRootPath: ".gemini",
		TargetRelPath:  geminiInstructionsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing gemini instructions file: %s",
		MarkersFmt:     "missing gemini managed block markers in %s",
	}
	githubCopilotContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentGitHubCopilot,
		DetectRootPath: ".github",
		TargetRelPath:  githubCopilotInstructionsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing github copilot instructions file: %s",
		MarkersFmt:     "missing github copilot managed block markers in %s",
	}
	kiroContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentKiro,
		DetectRootPath: ".kiro",
		TargetRelPath:  kiroSteeringPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing kiro steering file: %s",
		MarkersFmt:     "missing kiro managed block markers in %s",
	}
	piContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentPi,
		DetectRootPath: ".pi",
		TargetRelPath:  piAppendSystemPath,
		TargetScope:    managedContextTargetRepo,
		MissingFmt:     "missing pi append system file: %s",
		MarkersFmt:     "missing pi managed block markers in %s",
	}
	qoderContextSpec = ManagedContextFileAdapterSpec{
		ID:             AgentQoder,
		DetectRootPath: ".qoder",
		TargetRelPath:  qoderAgentsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing qoder agents file: %s",
		MarkersFmt:     "missing qoder managed block markers in %s",
	}
)

var simpleContextAdapterSpecs = []ManagedContextFileAdapterSpec{
	antigravityContextSpec,
	auggieContextSpec,
	codexContextSpec,
	factoryContextSpec,
	geminiContextSpec,
	githubCopilotContextSpec,
	kiroContextSpec,
	piContextSpec,
	qoderContextSpec,
}

var managedJSPluginAdapterSpecs = []ManagedJSPluginAdapterSpec{
	openCodeJSPluginSpec,
	kilocodeJSPluginSpec,
}

var managedContextLinkAdapterSpecs = []ManagedContextLinkAdapterSpec{
	aiderContextLinkSpec,
	crushContextLinkSpec,
	qwenContextLinkSpec,
}

var managedHookSettingsAdapterSpecs = []ManagedHookSettingsAdapterSpec{
	codebuddyHookSettingsSpec,
}

var kilocodeJSPluginSpec = ManagedJSPluginAdapterSpec{
	ID:             AgentKilocode,
	DetectRootPath: ".kilocode",
	ConfigDirName:  "kilocode",
	MissingFileFmt: "missing kilocode plugin file: %s",
	VerifyRequirements: []jsPluginVerifyRequirement{
		{Snippet: `"tool.execute.before"`, Msg: "kilocode plugin missing tool.execute.before hook: %s"},
		{Snippet: `input.tool !== "bash"`, Msg: "kilocode plugin missing bash-only guard: %s"},
	},
}

var aiderContextLinkSpec = ManagedContextLinkAdapterSpec{
	ID:             AgentAider,
	DetectRootPath: aiderConfigPath,
	Detect: func(scopeRoot string) bool {
		st, err := os.Stat(filepath.Join(scopeRoot, aiderConfigPath))
		return err == nil && !st.IsDir()
	},
	ContextSpec: ManagedContextFileAdapterSpec{
		ID:             AgentAider,
		DetectRootPath: aiderConfigPath,
		TargetRelPath:  aiderRulesPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing aider rules file: %s",
		MarkersFmt:     "missing aider managed block markers in %s",
	},
	ConfigPath: func(ctx Context) string {
		return ResolveHomeScopedPath(ctx.HomeDir, aiderConfigPath)
	},
	ConfigPlanContent: func(ctx Context) string {
		return fmt.Sprintf("read:\n  - %s\n", ResolveHomeScopedPath(ctx.HomeDir, aiderRulesPath))
	},
	UpsertConfig: func(configPath string, ctx Context) (string, error) {
		return upsertAiderReadConfig(configPath, ResolveHomeScopedPath(ctx.HomeDir, aiderRulesPath))
	},
	VerifyConfig: func(configPath string, ctx Context) error {
		ok, err := aiderConfigHasRead(configPath, ResolveHomeScopedPath(ctx.HomeDir, aiderRulesPath))
		if err != nil {
			return fmt.Errorf("missing aider config file: %s", configPath)
		}
		if !ok {
			return fmt.Errorf("missing aider managed read guidance in %s", configPath)
		}
		return nil
	},
	RemoveConfig: func(configPath string, ctx Context) (string, bool, bool, error) {
		return removeAiderReadConfig(configPath, ResolveHomeScopedPath(ctx.HomeDir, aiderRulesPath))
	},
}

var crushContextLinkSpec = ManagedContextLinkAdapterSpec{
	ID:             AgentCrush,
	DetectRootPath: ".crush",
	ContextSpec: ManagedContextFileAdapterSpec{
		ID:             AgentCrush,
		DetectRootPath: ".crush",
		TargetRelPath:  crushContextRelPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing crush context file: %s",
		MarkersFmt:     "missing crush managed block markers in %s",
	},
	ConfigPath: func(ctx Context) string {
		return ResolveHomeScopedPath(ctx.HomeDir, crushConfigRelPath)
	},
	ConfigPlanContent: func(ctx Context) string {
		return "{\n  \"options\": {\n    \"context_paths\": [\n      \"" + ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath) + "\"\n    ]\n  }\n}\n"
	},
	UpsertConfig: func(configPath string, ctx Context) (string, error) {
		return upsertCrushConfig(configPath, ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath))
	},
	VerifyConfig: func(configPath string, ctx Context) error {
		ok, err := crushConfigUsesContext(configPath, ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing crush managed context path in %s", configPath)
		}
		return nil
	},
	RemoveConfig: func(configPath string, ctx Context) (string, bool, bool, error) {
		return removeCrushContextPath(configPath, ResolveHomeScopedPath(ctx.HomeDir, crushContextRelPath))
	},
}

var qwenContextLinkSpec = ManagedContextLinkAdapterSpec{
	ID:             AgentQwen,
	DetectRootPath: ".qwen",
	ContextSpec: ManagedContextFileAdapterSpec{
		ID:             AgentQwen,
		DetectRootPath: ".qwen",
		TargetRelPath:  qwenAgentsPath,
		TargetScope:    managedContextTargetHome,
		MissingFmt:     "missing qwen agents file: %s",
		MarkersFmt:     "missing qwen managed block markers in %s",
	},
	ConfigPath: func(ctx Context) string {
		return ResolveHomeScopedPath(ctx.HomeDir, qwenSettingsPath)
	},
	ConfigPlanContent: func(Context) string { return qwenContextSettingsPlanContent() },
	UpsertConfig: func(configPath string, _ Context) (string, error) {
		return upsertQwenSettings(configPath)
	},
	VerifyConfig: func(configPath string, _ Context) error {
		ok, err := qwenSettingsUseAgents(configPath)
		if err != nil {
			return fmt.Errorf("missing qwen settings file: %s", configPath)
		}
		if !ok {
			return fmt.Errorf("missing qwen managed context filename in %s", configPath)
		}
		return nil
	},
	RemoveConfig: func(configPath string, _ Context) (string, bool, bool, error) {
		return removeQwenSettings(configPath)
	},
}

var codebuddyHookSettingsSpec = ManagedHookSettingsAdapterSpec{
	ID:             AgentCodeBuddy,
	DetectRootPath: ".codebuddy",
	Root:           codebuddyRoot,
	HookScriptName: codebuddyHookScriptName,
	SettingsName:   codebuddySettingsName,
	HookContent: func() string {
		return bashRewriteHookScriptContent("codebuddy", "cmdshape-codebuddy-hook.log")
	},
	PlanSettingsContent: preToolUseCommandSettingsContent,
	UpsertSettings:      upsertCodeBuddySettings,
	VerifySettings: func(settingsPath, hookPath string) error {
		ok, err := codebuddySettingsUseHook(settingsPath, hookPath)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing codebuddy managed hook contribution in %s", settingsPath)
		}
		return nil
	},
	VerifyHook: verifyBashRewriteHook,
	UninstallSettings: func(settingsPath, hookPath string) (InstallResult, error) {
		changed, err := removePreToolUseCommandHook(settingsPath, hookPath)
		if err != nil {
			return InstallResult{}, err
		}
		if changed {
			return InstallResult{Applied: 1}, nil
		}
		return InstallResult{Noop: 1}, nil
	},
	MissingHookFmt:     "missing codebuddy hook script: %s",
	MissingSettingsFmt: "missing codebuddy settings file: %s",
}

func canonicalRuleVerificationSnippets() []string {
	return []string{
		"## cmdshape Integration (Managed)",
		"Use `cmdshape` as the command prefix for every executable in shell commands",
		"`cmdshape nl -ba spec.md | cmdshape sed -n '1,260p'`",
		cmdshapeRawEscapeHatch,
		cmdshapeFilterPromptHint,
	}
}

func builtInRuleFileAdapterCatalog() []BuiltInAdapterSpec {
	return buildAdapterCatalog(
		managedRuleFileAdapterSpecs,
		func(spec managedRuleFileAdapterSpec) ID { return spec.ID },
		newManagedRuleFileAdapterFromSpec,
	)
}

func builtInJSPluginAdapterCatalog() []BuiltInAdapterSpec {
	return buildAdapterCatalog(
		managedJSPluginAdapterSpecs,
		func(spec ManagedJSPluginAdapterSpec) ID { return spec.ID },
		func(spec ManagedJSPluginAdapterSpec) Adapter { return NewManagedJSPluginAdapter(spec) },
	)
}

func builtInContextAdapterCatalog() []BuiltInAdapterSpec {
	return buildAdapterCatalog(
		simpleContextAdapterSpecs,
		func(spec ManagedContextFileAdapterSpec) ID { return spec.ID },
		func(spec ManagedContextFileAdapterSpec) Adapter { return NewManagedContextAdapter(spec) },
	)
}

func builtInContextLinkAdapterCatalog() []BuiltInAdapterSpec {
	return buildAdapterCatalog(
		managedContextLinkAdapterSpecs,
		func(spec ManagedContextLinkAdapterSpec) ID { return spec.ID },
		func(spec ManagedContextLinkAdapterSpec) Adapter { return NewManagedContextLinkAdapter(spec) },
	)
}

func builtInHookSettingsAdapterCatalog() []BuiltInAdapterSpec {
	return buildAdapterCatalog(
		managedHookSettingsAdapterSpecs,
		func(spec ManagedHookSettingsAdapterSpec) ID { return spec.ID },
		func(spec ManagedHookSettingsAdapterSpec) Adapter { return NewManagedHookSettingsAdapter(spec) },
	)
}

func buildAdapterCatalog[S any](specs []S, id func(S) ID, build func(S) Adapter) []BuiltInAdapterSpec {
	catalog := make([]BuiltInAdapterSpec, 0, len(specs))
	for _, spec := range specs {
		catalog = append(catalog, BuiltInAdapterSpec{
			ID:  id(spec),
			New: func() Adapter { return build(spec) },
		})
	}
	return catalog
}

func newManagedRuleFileAdapterFromSpec(spec managedRuleFileAdapterSpec) Adapter {
	switch spec.TargetScope {
	case managedRuleFileTargetHome:
		return NewManagedHomeRuleFileAdapter(
			string(spec.ID),
			spec.DetectRootPath,
			spec.TargetRelPath,
			spec.MissingFmt,
			spec.GuidanceFmt,
			spec.Render,
			spec.VerifyRequired,
		)
	default:
		return NewManagedRepoRuleFileAdapter(
			string(spec.ID),
			spec.DetectRootPath,
			spec.TargetRelPath,
			spec.MissingFmt,
			spec.GuidanceFmt,
			spec.Render,
			spec.VerifyRequired,
		)
	}
}

func cursorRuleContent() string {
	return "---\n" +
		"description: Route shell commands through cmdshape\n" +
		"alwaysApply: true\n" +
		"---\n\n" +
		cmdshapeManagedGuidanceMarkdown()
}

func expectedRuleTarget(ctx Context, spec managedRuleFileAdapterSpec) string {
	switch spec.TargetScope {
	case managedRuleFileTargetHome:
		return ResolveHomeScopedPath(ctx.HomeDir, spec.TargetRelPath)
	default:
		return ResolveRepoScopedPath(ctx.ScopeRoot, spec.TargetRelPath)
	}
}

var builtInAdapterCatalog = func() []BuiltInAdapterSpec {
	catalog := []BuiltInAdapterSpec{
		{ID: AgentClaude, New: func() Adapter { return ClaudeAdapter{} }},
	}
	catalog = append(catalog, builtInRuleFileAdapterCatalog()...)
	catalog = append(catalog, builtInHookSettingsAdapterCatalog()...)
	catalog = append(catalog, builtInContextLinkAdapterCatalog()...)
	catalog = append(catalog, builtInJSPluginAdapterCatalog()...)
	return append(catalog, builtInContextAdapterCatalog()...)
}()

func BuiltInAdapterCatalog() []BuiltInAdapterSpec {
	return slices.Clone(builtInAdapterCatalog)
}

func NewBuiltInAdapters() (map[string]Adapter, error) {
	return adaptersFromCatalog(builtInAdapterCatalog)
}

func adaptersFromCatalog(catalog []BuiltInAdapterSpec) (map[string]Adapter, error) {
	adapters := make(map[string]Adapter, len(catalog))
	for _, spec := range catalog {
		id := string(spec.ID)
		if _, exists := adapters[id]; exists {
			return nil, fmt.Errorf("duplicate adapter registration: %s", id)
		}
		adapter := spec.New()
		if adapter == nil {
			return nil, fmt.Errorf("nil adapter for %s", id)
		}
		if got := adapter.ID(); got != id {
			return nil, fmt.Errorf("adapter id mismatch: catalog=%s adapter=%s", id, got)
		}
		adapters[id] = adapter
	}
	return adapters, nil
}
