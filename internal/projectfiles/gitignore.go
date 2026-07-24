package projectfiles

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

const nestedCmdshapeGitignoreContents = "gain.db\n.gitignore\n"

func EnsureNestedCmdshapeGitignore(projectRoot string) error {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}

	gitignorePath := filepath.Join(projectRoot, ".cmdshape", ".gitignore")
	if err := RejectSymlinkPath(gitignorePath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(gitignorePath), 0o755); err != nil {
		return err
	}
	return AtomicWriteFileBeneath(projectRoot, gitignorePath, []byte(nestedCmdshapeGitignoreContents), 0o644)
}

func RemoveProductRootGitignoreEntries(projectRoot string) error {
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

	updated, changed := removeProductIgnoreLines(content)
	if !changed {
		return nil
	}
	return AtomicWriteFileBeneath(projectRoot, gitignorePath, updated, 0o644)
}

func removeProductIgnoreLines(content []byte) ([]byte, bool) {
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
		if trimmed == ".cmdshape" || trimmed == ".cmdshape/" ||
			trimmed == ".ccp" || trimmed == ".ccp/" {
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
