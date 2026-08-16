package lifecycle

import (
	"errors"
	"os"

	"github.com/SuppieRK/cmdshape/internal/version"
)

// RunFilterBootstrap adds shipped filters on first use of a release binary.
// Development builds load repository filters directly.
func RunFilterBootstrap() error {
	if version.Version == "dev" {
		return nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	lockPath, err := startupMaintenanceLockPath()
	if err != nil {
		return err
	}
	release, err := acquireStartupMaintenanceLock(lockPath)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()
	return syncMissingPackagedFilters(homeDir)
}
