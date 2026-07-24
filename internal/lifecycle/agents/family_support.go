package agents

import (
	"github.com/SuppieRK/cmdshape/internal/projectfiles"
	"os"
)

func updateInstallResult(res *InstallResult, changed bool) {
	if changed {
		res.Applied++
		return
	}
	res.Noop++
}

func writeManagedArtifact(path string, body []byte, perm os.FileMode) error {
	return projectfiles.AtomicWriteFile(path, body, perm)
}

func applyManagedFileChange(path, updated string, changed bool, removeAll bool) (InstallResult, error) {
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	if removeAll {
		removed, err := removeFileIfExists(path)
		if err != nil {
			return InstallResult{}, err
		}
		if !removed {
			return InstallResult{Noop: 1}, nil
		}
		return InstallResult{Applied: 1}, nil
	}
	if err := writeManagedArtifact(path, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}
