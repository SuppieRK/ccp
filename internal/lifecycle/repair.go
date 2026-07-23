package lifecycle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go-command-compression-proxy/internal/audit"
)

var (
	repairStdin  io.Reader = os.Stdin
	repairStdout io.Writer = os.Stdout
)

type repairMode string

const (
	repairModeRewrite  repairMode = "rewrite"
	repairModePreserve repairMode = "preserve"
	repairModePrompt   repairMode = "prompt"
)

func RunRepair(args []string) error {
	fs := newLifecycleFlagSet("repair")
	yes := fs.Bool("yes", false, "rewrite managed CCP home state without prompting")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	no := fs.Bool("no", false, "preserve existing home filters and add only missing shipped content")
	setLifecycleUsage(
		fs,
		"rewrite managed CCP home state to canonical shipped content",
		[]string{"ccp repair [--yes|--no]"},
		"Repair rewrites managed ~/.config/ccp state and restores ~/.config/ccp/filters from shipped content embedded in the binary.",
		"Project filter approvals in ~/.config/ccp/filter-trust.json are preserved.",
		"Repair also removes obsolete managed ~/.ccp remnants.",
		"Rewrite repair also runs guarded current-repository CCP migrations, including repo-local .ccp ignore migration.",
		"Repair is interactive by default; declining the prompt adds only missing shipped filters and mappings without mutating repository files.",
		"Use --yes for destructive rewrite automation; use --no for additive preserve-existing automation.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	mode, err := repairModeFromFlags(*yes, *no)
	if err != nil {
		return err
	}
	return executeRepair(mode)
}

func repairModeFromFlags(yes, no bool) (repairMode, error) {
	if yes && no {
		return "", errors.New("cannot use both --yes and --no")
	}
	if yes {
		return repairModeRewrite, nil
	}
	if no {
		return repairModePreserve, nil
	}
	return repairModePrompt, nil
}

func executeRepair(mode repairMode) error {
	if mode == repairModePrompt {
		ok, err := confirmRepair()
		if err != nil {
			return err
		}
		if !ok {
			mode = repairModePreserve
		} else {
			mode = repairModeRewrite
		}
	}
	ctx, err := newMigrationContext(mode)
	if err != nil {
		return err
	}
	return withLifecycleLock(func() error {
		if mode == repairModePreserve {
			return addMissingPackagedFilters()
		}
		if err := runMigrations(migrationSurfaceRepo, "", ctx); err != nil {
			return err
		}
		if err := runMigrations(migrationSurfaceHome, "", ctx); err != nil {
			return err
		}
		return rewriteManagedRepairStateLocked()
	})
}

func newMigrationContext(mode repairMode) (migrationContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return migrationContext{}, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return migrationContext{}, err
	}
	return migrationContext{homeDir: homeDir, cwd: cwd, mode: mode}, nil
}

func addMissingPackagedFilters() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := syncMissingPackagedFilters(homeDir); err != nil {
		return err
	}
	_, err = fmt.Fprintln(repairStdout, "ccp repair: added missing shipped filters and mappings")
	return err
}

func rewriteManagedRepairState() error {
	return withLifecycleLock(rewriteManagedRepairStateLocked)
}

func rewriteManagedRepairStateLocked() error {
	// The rewrite removes managed home state, including audit logs. Windows
	// cannot remove the active audit file while lumberjack still has it open.
	audit.Reset()
	if err := syncCanonicalHomeLayout(); err != nil {
		return err
	}

	_, err := fmt.Fprintln(repairStdout, "ccp repair: rewrote managed CCP home state")
	return err
}

func withLifecycleLock(fn func() error) error {
	lockPath, err := startupMaintenanceLockPath()
	if err != nil {
		return err
	}
	release, err := acquireStartupMaintenanceLock(lockPath)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

func confirmRepair() (bool, error) {
	_, err := fmt.Fprintln(repairStdout, "ccp repair will rewrite the fully managed ~/.config/ccp state and remove obsolete managed ~/.ccp files.")
	if err != nil {
		return false, err
	}
	_, err = fmt.Fprint(repairStdout, "Continue? [y/N]: ")
	if err != nil {
		return false, err
	}

	line, err := bufio.NewReader(repairStdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}
