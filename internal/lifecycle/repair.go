package lifecycle

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	repairStdin  io.Reader = os.Stdin
	repairStdout io.Writer = os.Stdout
)

func RunRepair(args []string) error {
	fs := newLifecycleFlagSet("repair")
	yes := fs.Bool("yes", false, "rewrite managed CCP home state without prompting")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	setLifecycleUsage(
		fs,
		"rewrite managed CCP home state to canonical shipped content",
		[]string{"ccp repair [--yes]"},
		"Repair rewrites the fully managed ~/.config/ccp directory and restores ~/.config/ccp/filters from shipped content embedded in the binary.",
		"Repair also removes obsolete managed ~/.ccp remnants.",
		"Repair is interactive by default; declining the prompt adds only missing shipped filters and mappings.",
		"Use --yes for upgrade or installer automation.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	return executeRepair(*yes)
}

func executeRepair(yes bool) error {
	if !yes {
		ok, err := confirmRepair()
		if err != nil {
			return err
		}
		if !ok {
			return addMissingPackagedFilters()
		}
	}
	return rewriteManagedRepairState()
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
	lockPath, err := startupMaintenanceLockPath()
	if err != nil {
		return err
	}
	release, err := acquireStartupMaintenanceLock(lockPath)
	if err != nil {
		return err
	}
	defer release()

	if err := syncCanonicalHomeLayout(); err != nil {
		return err
	}

	_, err = fmt.Fprintln(repairStdout, "ccp repair: rewrote managed CCP home state")
	return err
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
