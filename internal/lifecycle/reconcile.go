package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	shippedfilters "go-command-compression-proxy/filters"
)

const (
	startupMaintenanceLockMaxAge = 10 * time.Second
	configDirName                = ".config"
)

var (
	startupMaintenanceNow  = time.Now
	materializeHomeFilters = shippedfilters.MaterializeShipped
)

func RunStartupMaintenance() error {
	scopeRoot, err := initDetectRoot()
	if err != nil {
		return nil
	}
	if err := cleanupLegacyProjectInitState(scopeRoot); err != nil {
		return err
	}

	lockPath, err := startupMaintenanceLockPath()
	if err != nil {
		return nil
	}
	release, err := acquireStartupMaintenanceLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer release()

	return syncCanonicalHomeLayout()
}

func startupMaintenanceLockPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configDirName, "ccp", "repair.lock"), nil
}

func acquireStartupMaintenanceLock(lockPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	release, err := createStartupMaintenanceLock(lockPath)
	if errors.Is(err, os.ErrExist) {
		removed, staleErr := removeStaleStartupMaintenanceLock(lockPath)
		if staleErr != nil {
			return nil, staleErr
		}
		if removed {
			return createStartupMaintenanceLock(lockPath)
		}
	}
	return release, err
}

func createStartupMaintenanceLock(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = f.Close()
		_ = os.Remove(lockPath)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(lockPath)
		return nil, err
	}
	return func() { _ = os.Remove(lockPath) }, nil
}

func removeStaleStartupMaintenanceLock(lockPath string) (bool, error) {
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if startupMaintenanceNow().Sub(info.ModTime()) < startupMaintenanceLockMaxAge {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func cleanupLegacyProjectInitState(scopeRoot string) error {
	ccpDir := filepath.Join(scopeRoot, ".ccp")
	targets := []string{filepath.Join(ccpDir, "init.json")}
	matches, err := filepath.Glob(filepath.Join(ccpDir, "init.json.bak.*"))
	if err != nil {
		return err
	}
	targets = append(targets, matches...)
	for _, path := range targets {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func syncCanonicalHomeLayout() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if err := cleanupManagedConfigDir(homeDir); err != nil {
		return err
	}
	return syncPackagedFilters(homeDir)
}

func cleanupManagedConfigDir(homeDir string) error {
	configDir := filepath.Join(homeDir, configDirName, "ccp")
	if err := removeAllChildrenExcept(configDir, "repair.lock"); err != nil {
		return err
	}

	ccpDir := filepath.Join(homeDir, ".ccp")
	if err := os.RemoveAll(ccpDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeAllChildrenExcept(dir string, keep ...string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	keepSet := make(map[string]struct{}, len(keep))
	for _, name := range keep {
		keepSet[name] = struct{}{}
	}

	for _, entry := range entries {
		if _, ok := keepSet[entry.Name()]; ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func syncPackagedFilters(homeDir string) error {
	dstDir := filepath.Join(homeDir, configDirName, "ccp", "filters")
	if err := os.RemoveAll(dstDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return materializeHomeFilters(dstDir)
}
