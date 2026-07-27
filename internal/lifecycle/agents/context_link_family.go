package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type ManagedContextLinkAdapterSpec struct {
	ID                ID
	DetectRootPath    string
	Detect            func(scopeRoot string) bool
	ContextSpec       ManagedContextFileAdapterSpec
	ConfigPath        func(ctx Context) string
	ConfigPlanContent func(ctx Context) string
	UpsertConfig      func(configPath string, ctx Context) (string, error)
	VerifyConfig      func(configPath string, ctx Context) error
	RemoveConfig      func(configPath string, ctx Context) (updated string, changed bool, removeAll bool, err error)
}

type ManagedContextLinkAdapter struct {
	spec    ManagedContextLinkAdapterSpec
	context ManagedContextAdapter
}

func NewManagedContextLinkAdapter(spec ManagedContextLinkAdapterSpec) ManagedContextLinkAdapter {
	return ManagedContextLinkAdapter{
		spec:    spec,
		context: NewManagedContextAdapter(spec.ContextSpec),
	}
}

func (a ManagedContextLinkAdapter) ID() string { return string(a.spec.ID) }

func (a ManagedContextLinkAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.spec.DetectRootPath)
}

func (a ManagedContextLinkAdapter) Detect(scopeRoot string) bool {
	if a.spec.Detect != nil {
		return a.spec.Detect(scopeRoot)
	}
	_, err := os.Stat(a.DetectRoot(scopeRoot))
	return err == nil
}

func (a ManagedContextLinkAdapter) Plan(ctx Context) []PlannedArtifact {
	return append([]PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    a.spec.ConfigPath(ctx),
		Content: a.spec.ConfigPlanContent(ctx),
		Perm:    0o644,
	}}, a.context.Plan(ctx)...)
}

func (a ManagedContextLinkAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	res, err := a.context.Install(ctx, write)
	if err != nil {
		return InstallResult{}, err
	}
	content, err := a.spec.UpsertConfig(a.spec.ConfigPath(ctx), ctx)
	if err != nil {
		return InstallResult{}, err
	}
	changed, err := write(a.spec.ConfigPath(ctx), []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	updateInstallResult(&res, changed)
	return res, nil
}

func (a ManagedContextLinkAdapter) Verify(ctx Context) error {
	if err := a.context.Verify(ctx); err != nil {
		return err
	}
	return a.spec.VerifyConfig(a.spec.ConfigPath(ctx), ctx)
}

func (a ManagedContextLinkAdapter) Uninstall(ctx Context) (InstallResult, error) {
	res, err := a.context.Uninstall(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	updated, changed, removeAll, err := a.spec.RemoveConfig(a.spec.ConfigPath(ctx), ctx)
	if err != nil {
		return InstallResult{}, err
	}
	configRes, err := applyManagedFileChange(a.spec.ConfigPath(ctx), updated, changed, removeAll)
	if err != nil {
		return InstallResult{}, err
	}
	res.Applied += configRes.Applied
	res.Noop += configRes.Noop
	return res, nil
}

type ManagedHookSettingsAdapterSpec struct {
	ID                  ID
	DetectRootPath      string
	Root                func(ctx Context) string
	HookScriptName      string
	SettingsName        string
	HookContent         func() string
	PlanSettingsContent func(hookPath string) string
	UpsertSettings      func(settingsPath, hookPath string) (string, error)
	VerifySettings      func(settingsPath, hookPath string) error
	VerifyHook          func(hookPath string) error
	UninstallSettings   func(settingsPath, hookPath string) (InstallResult, error)
	MissingHookFmt      string
	MissingSettingsFmt  string
}

type ManagedHookSettingsAdapter struct {
	spec ManagedHookSettingsAdapterSpec
}

func NewManagedHookSettingsAdapter(spec ManagedHookSettingsAdapterSpec) ManagedHookSettingsAdapter {
	return ManagedHookSettingsAdapter{spec: spec}
}

func (a ManagedHookSettingsAdapter) ID() string { return string(a.spec.ID) }

func (a ManagedHookSettingsAdapter) DetectRoot(scopeRoot string) string {
	return filepath.Join(scopeRoot, a.spec.DetectRootPath)
}

func (a ManagedHookSettingsAdapter) hookPath(ctx Context) string {
	return filepath.Join(a.spec.Root(ctx), "hooks", a.spec.HookScriptName)
}

func (a ManagedHookSettingsAdapter) settingsPath(ctx Context) string {
	return filepath.Join(a.spec.Root(ctx), a.spec.SettingsName)
}

func (a ManagedHookSettingsAdapter) Plan(ctx Context) []PlannedArtifact {
	hookPath := a.hookPath(ctx)
	return []PlannedArtifact{
		{
			Kind:    ArtifactHook,
			Path:    hookPath,
			Content: a.spec.HookContent(),
			Perm:    privateHookMode,
		},
		{
			Kind:    ArtifactSettings,
			Path:    a.settingsPath(ctx),
			Content: a.spec.PlanSettingsContent(hookPath),
			Perm:    0o644,
		},
	}
}

func (a ManagedHookSettingsAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	var res InstallResult
	hookPath := a.hookPath(ctx)
	hookChanged, err := write(hookPath, []byte(a.spec.HookContent()), privateHookMode)
	if err != nil {
		return InstallResult{}, err
	}
	if err := ensureHookArtifactExecutable(hookPath); err != nil {
		return InstallResult{}, err
	}
	updateInstallResult(&res, hookChanged)

	settingsPath := a.settingsPath(ctx)
	content, err := a.spec.UpsertSettings(settingsPath, hookPath)
	if err != nil {
		return InstallResult{}, err
	}
	settingsChanged, err := write(settingsPath, []byte(content), 0o644)
	if err != nil {
		return InstallResult{}, err
	}
	updateInstallResult(&res, settingsChanged)
	return res, nil
}

func (a ManagedHookSettingsAdapter) Verify(ctx Context) error {
	hookPath := a.hookPath(ctx)
	if err := verifyArtifactFiles(
		artifactCheck{path: hookPath, msg: a.spec.MissingHookFmt},
		artifactCheck{path: a.settingsPath(ctx), msg: a.spec.MissingSettingsFmt},
	); err != nil {
		return err
	}
	if err := verifyHookArtifactExecutable(hookPath); err != nil {
		return err
	}
	if a.spec.VerifyHook != nil {
		if err := a.spec.VerifyHook(hookPath); err != nil {
			return err
		}
	}
	if a.spec.VerifySettings == nil {
		return nil
	}
	return a.spec.VerifySettings(a.settingsPath(ctx), hookPath)
}

func (a ManagedHookSettingsAdapter) Uninstall(ctx Context) (InstallResult, error) {
	var res InstallResult
	removed, err := removeFileIfExists(a.hookPath(ctx))
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		res.Applied++
	}
	settingsRes, err := a.spec.UninstallSettings(a.settingsPath(ctx), a.hookPath(ctx))
	if err != nil {
		return InstallResult{}, err
	}
	res.Applied += settingsRes.Applied
	res.Noop += settingsRes.Noop
	return res, nil
}

const (
	aiderConfigPath = ".aider.conf.yml"
	aiderRulesPath  = ".aider.rules.md"

	crushContextRelPath = ".config/crush/CRUSH.md"
	crushConfigRelPath  = ".config/crush/crush.json"
	crushConfigErrFmt   = "invalid crush config file: %s"

	qwenAgentsPath   = ".qwen/AGENTS.md"
	qwenSettingsPath = ".qwen/settings.json"
	qwenAgentsFile   = "AGENTS.md"
)

func upsertAiderReadConfig(path, readPath string) (string, error) {
	cfg, err := readAiderConfig(path)
	if err != nil {
		return "", err
	}
	read := normalizeAiderRead(cfg["read"])
	if !slices.Contains(read, readPath) {
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
	return slices.Contains(normalizeAiderRead(cfg["read"]), readPath), nil
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

func upsertQwenSettings(path string) (string, error) {
	root, err := readQwenSettings(path)
	if err != nil {
		return "", err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	contextMap["fileName"] = qwenAgentsFile
	root["context"] = contextMap
	return marshalQwenSettings(root)
}

func removeQwenSettings(path string) (updated string, changed bool, removeAll bool, err error) {
	root, err := readQwenSettings(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		return "", false, false, nil
	}
	if v, _ := contextMap["fileName"].(string); v != qwenAgentsFile {
		return "", false, false, nil
	}
	delete(contextMap, "fileName")
	if len(contextMap) == 0 {
		delete(root, "context")
	} else {
		root["context"] = contextMap
	}
	if len(root) == 0 {
		return "", true, true, nil
	}
	content, err := marshalQwenSettings(root)
	if err != nil {
		return "", false, false, err
	}
	return content, true, false, nil
}

func qwenSettingsUseAgents(path string) (bool, error) {
	root, err := readQwenSettings(path)
	if err != nil {
		return false, err
	}
	contextMap, _ := root["context"].(map[string]any)
	if contextMap == nil {
		return false, nil
	}
	return contextMap["fileName"] == qwenAgentsFile, nil
}

func qwenContextSettingsPlanContent() string {
	return "{\n  \"context\": {\n    \"fileName\": \"AGENTS.md\"\n  }\n}\n"
}

func readQwenSettings(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	root := map[string]any{}
	if len(raw) == 0 {
		return root, nil
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func marshalQwenSettings(root map[string]any) (string, error) {
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(out, '\n')), nil
}
