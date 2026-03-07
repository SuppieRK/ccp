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
	AgentAmazonQ       ID = "amazon-q"
	AgentClaude        ID = "claude"
	AgentCodex         ID = "codex"
	AgentCursor        ID = "cursor"
	AgentGemini        ID = "gemini"
	AgentGitHubCopilot ID = "github-copilot"
	AgentOpenCode      ID = "opencode"
	AgentWindsurf      ID = "windsurf"
)

type Adapter interface {
	ID() string
	DetectRoot(scopeRoot string) string
	Install(ctx Context, write WriterFunc) (InstallResult, error)
	Plan(ctx Context) []PlannedArtifact
	Verify(ctx Context) error
}

// Uninstaller is an optional adapter capability for removing installed artifacts.
type Uninstaller interface {
	Uninstall(ctx Context) (InstallResult, error)
}

func SupportedTools(adapters map[string]Adapter) []string {
	keys := make([]string, 0, len(adapters))
	for k := range adapters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func ValidateSelectedTools(tools []string, adapters map[string]Adapter) error {
	for _, t := range tools {
		if _, ok := adapters[t]; !ok {
			return fmt.Errorf("unsupported tool %q (supported: %s)", t, strings.Join(SupportedTools(adapters), ", "))
		}
	}
	return nil
}

func DetectTools(scopeRoot string, adapters map[string]Adapter) []string {
	detected := make([]string, 0, len(adapters))
	for _, id := range SupportedTools(adapters) {
		if st, err := os.Stat(adapters[id].DetectRoot(scopeRoot)); err == nil && st.IsDir() {
			detected = append(detected, id)
		}
	}
	return detected
}

func DefaultAdapters() map[string]Adapter {
	return map[string]Adapter{
		string(AgentAmazonQ):       NewAmazonQAdapter(),
		string(AgentClaude):        NewClaudeAdapter(),
		string(AgentCodex):         NewCodexAdapter(),
		string(AgentCursor):        NewCursorAdapter(),
		string(AgentGemini):        NewGeminiAdapter(),
		string(AgentGitHubCopilot): NewGitHubCopilotAdapter(),
		string(AgentOpenCode):      NewOpenCodeAdapter(),
		string(AgentWindsurf):      NewWindsurfAdapter(),
	}
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
