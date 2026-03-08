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
	AgentAmazonQ       ID = "amazon-q"
	AgentCline         ID = "cline"
	AgentClaude        ID = "claude"
	AgentContinue      ID = "continue"
	AgentCodex         ID = "codex"
	AgentCursor        ID = "cursor"
	AgentGemini        ID = "gemini"
	AgentGitHubCopilot ID = "github-copilot"
	AgentKiro          ID = "kiro"
	AgentOpenCode      ID = "opencode"
	AgentQwen          ID = "qwen"
	AgentRooCode       ID = "roocode"
	AgentTrae          ID = "trae"
	AgentWindsurf      ID = "windsurf"
)

type Adapter interface {
	ID() string
	DetectRoot(scopeRoot string) string
	Install(ctx Context, write WriterFunc) (InstallResult, error)
	Plan(ctx Context) []PlannedArtifact
	Verify(ctx Context) error
}

// Detector allows adapters to override the default directory-based detection check.
type Detector interface {
	Detect(scopeRoot string) bool
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

func DefaultAdapters() map[string]Adapter {
	return map[string]Adapter{
		string(AgentAider):         NewAiderAdapter(),
		string(AgentAmazonQ):       NewAmazonQAdapter(),
		string(AgentCline):         NewClineAdapter(),
		string(AgentClaude):        NewClaudeAdapter(),
		string(AgentContinue):      NewContinueAdapter(),
		string(AgentCodex):         NewCodexAdapter(),
		string(AgentCursor):        NewCursorAdapter(),
		string(AgentGemini):        NewGeminiAdapter(),
		string(AgentGitHubCopilot): NewGitHubCopilotAdapter(),
		string(AgentKiro):          NewKiroAdapter(),
		string(AgentOpenCode):      NewOpenCodeAdapter(),
		string(AgentQwen):          NewQwenAdapter(),
		string(AgentRooCode):       NewRooCodeAdapter(),
		string(AgentTrae):          NewTraeAdapter(),
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
