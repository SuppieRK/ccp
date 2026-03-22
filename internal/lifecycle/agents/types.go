package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ArtifactKind string

const (
	ArtifactHook      ArtifactKind = "hook"
	ArtifactSettings  ArtifactKind = "settings"
	ArtifactAwareness ArtifactKind = "awareness"
)

type PlannedArtifact struct {
	Kind    ArtifactKind
	Path    string
	Content string
	Perm    os.FileMode
}

type InstallResult struct {
	Applied int
	Noop    int
}

type WriterFunc func(path string, data []byte, perm os.FileMode) (changed bool, err error)

type Context struct {
	ScopeRoot string
	HomeDir   string
}

type ID string

const (
	AgentAider         ID = "aider"
	AgentAuggie        ID = "auggie"
	AgentAntigravity   ID = "antigravity"
	AgentAmazonQ       ID = "amazon-q"
	AgentCodeBuddy     ID = "codebuddy"
	AgentClaude        ID = "claude"
	AgentCline         ID = "cline"
	AgentCostrict      ID = "costrict"
	AgentCodex         ID = "codex"
	AgentCrush         ID = "crush"
	AgentCursor        ID = "cursor"
	AgentFactory       ID = "factory"
	AgentGemini        ID = "gemini"
	AgentGitHubCopilot ID = "github-copilot"
	AgentKiro          ID = "kiro"
	AgentKilocode      ID = "kilocode"
	AgentOpenCode      ID = "opencode"
	AgentPi            ID = "pi"
	AgentQoder         ID = "qoder"
	AgentQwen          ID = "qwen"
	AgentRooCode       ID = "roocode"
	AgentTrae          ID = "trae"
	AgentWindsurf      ID = "windsurf"
)

var toolAliases = map[string]string{
	string(AgentCostrict): string(AgentRooCode),
}

func NormalizeToolID(id string) string {
	if canonical, ok := toolAliases[id]; ok {
		return canonical
	}
	return id
}

type Adapter interface {
	ID() string
	DetectRoot(scopeRoot string) string
	Install(ctx Context, write WriterFunc) (InstallResult, error)
	Plan(ctx Context) []PlannedArtifact
	Verify(ctx Context) error
}

type Detector interface {
	Detect(scopeRoot string) bool
}

type Uninstaller interface {
	Uninstall(ctx Context) (InstallResult, error)
}

func SupportedTools(adapters map[string]Adapter) []string {
	keys := make([]string, 0, len(adapters)+len(toolAliases))
	for k := range adapters {
		keys = append(keys, k)
	}
	for alias, canonical := range toolAliases {
		if _, ok := adapters[canonical]; ok {
			keys = append(keys, alias)
		}
	}
	sort.Strings(keys)
	return keys
}

func ValidateSelectedTools(tools []string, adapters map[string]Adapter) error {
	for _, t := range tools {
		if _, ok := adapters[NormalizeToolID(t)]; !ok {
			return fmt.Errorf("unsupported tool %q (supported: %s)", t, strings.Join(SupportedTools(adapters), ", "))
		}
	}
	return nil
}

func DetectTools(scopeRoot string, adapters map[string]Adapter) []string {
	detected := make([]string, 0, len(adapters))
	ids := make([]string, 0, len(adapters))
	for id := range adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if detector, ok := adapters[id].(Detector); ok {
			if detector.Detect(scopeRoot) {
				detected = append(detected, id)
			}
			continue
		}
		if st, err := os.Stat(adapters[id].DetectRoot(scopeRoot)); err == nil && st.IsDir() {
			detected = append(detected, id)
		}
	}
	return detected
}

func InstallPlannedArtifacts(plan []PlannedArtifact, write WriterFunc) (InstallResult, error) {
	var res InstallResult
	for _, item := range plan {
		changed, err := write(item.Path, []byte(item.Content), item.Perm)
		if err != nil {
			return res, err
		}
		if item.Kind == ArtifactHook {
			if err := os.Chmod(item.Path, 0o755); err != nil {
				return res, err
			}
		}
		if changed {
			res.Applied++
		} else {
			res.Noop++
		}
	}
	return res, nil
}

func ResolveHomeScopedPath(home, rel string) string {
	base := strings.TrimSpace(home)
	if base != "" {
		return filepath.Join(base, rel)
	}
	return rel
}

func ResolveRepoScopedPath(scopeRoot, rel string) string {
	base := strings.TrimSpace(scopeRoot)
	if base != "" {
		return filepath.Join(base, rel)
	}
	return rel
}
