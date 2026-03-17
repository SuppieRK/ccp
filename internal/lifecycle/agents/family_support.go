package agents

import "os"

func updateInstallResult(res *InstallResult, changed bool) {
	if changed {
		res.Applied++
		return
	}
	res.Noop++
}

func applyManagedFileChange(path, updated string, changed bool, removeAll bool) (InstallResult, error) {
	if !changed {
		return InstallResult{Noop: 1}, nil
	}
	if removeAll {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return InstallResult{}, err
		}
		return InstallResult{Applied: 1}, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: 1}, nil
}
