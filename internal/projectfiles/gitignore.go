package projectfiles

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

const nestedCCPGitignoreContents = "gain.db\n.gitignore\n"

func EnsureNestedCCPGitignore(projectRoot string) error {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}

	gitignorePath := filepath.Join(projectRoot, ".ccp", ".gitignore")
	if err := RejectSymlinkPath(gitignorePath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(gitignorePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(gitignorePath, []byte(nestedCCPGitignoreContents), 0o644)
}

func RemoveLegacyRootCCPGitignoreEntries(projectRoot string) error {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	if err := RejectSymlinkPath(gitignorePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	updated, changed := removeLegacyCCPIgnoreLines(content)
	if !changed {
		return nil
	}
	return os.WriteFile(gitignorePath, updated, 0o644)
}

func removeLegacyCCPIgnoreLines(content []byte) ([]byte, bool) {
	var out bytes.Buffer
	changed := false
	for len(content) > 0 {
		line := content
		if idx := bytes.IndexByte(content, '\n'); idx >= 0 {
			line = content[:idx+1]
			content = content[idx+1:]
		} else {
			content = nil
		}
		trimTarget := bytes.TrimSuffix(line, []byte("\n"))
		trimTarget = bytes.TrimSuffix(trimTarget, []byte("\r"))
		trimmed := strings.TrimSpace(string(trimTarget))
		if trimmed == ".ccp" || trimmed == ".ccp/" {
			changed = true
			continue
		}
		out.Write(line)
	}
	if !changed {
		return nil, false
	}
	return out.Bytes(), true
}
