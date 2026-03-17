package projectfiles

import (
	"os"
	"path/filepath"
	"strings"
)

func EnsureGitignoreEntry(projectRoot, entry string) error {
	projectRoot = strings.TrimSpace(projectRoot)
	entry = strings.TrimSpace(entry)
	if projectRoot == "" || entry == "" {
		return nil
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	suffix := entry + "\n"
	if len(content) > 0 && content[len(content)-1] != '\n' {
		suffix = "\n" + suffix
	}
	f, err := os.OpenFile(gitignorePath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(suffix)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
