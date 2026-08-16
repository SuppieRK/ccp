package projectfiles

import (
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
