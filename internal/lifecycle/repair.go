package lifecycle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SuppieRK/cmdshape/internal/audit"
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
	yes := fs.Bool("yes", false, "rewrite managed cmdshape home state without prompting")
	fs.BoolVar(yes, "y", false, "shorthand for --yes")
	no := fs.Bool("no", false, "preserve existing home filters and add only missing shipped content")
	setLifecycleUsage(
		fs,
		"rewrite managed cmdshape home state to canonical shipped content",
		[]string{"cmdshape repair [--yes|--no]"},
		"Repair rewrites managed ~/.config/cmdshape state and restores ~/.config/cmdshape/filters from shipped content embedded in the binary.",
		"Project filter approvals in ~/.config/cmdshape/filter-trust.json are preserved.",
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
	return withLifecycleLock(func() error {
		if mode == repairModePreserve {
			return addMissingPackagedFilters()
		}
		return rewriteManagedRepairStateLocked()
	})
}

func addMissingPackagedFilters() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := syncMissingPackagedFilters(homeDir); err != nil {
		return err
	}
	_, err = fmt.Fprintln(repairStdout, "cmdshape repair: added missing shipped filters and mappings")
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

	_, err := fmt.Fprintln(repairStdout, "cmdshape repair: rewrote managed cmdshape home state")
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
	_, err := fmt.Fprintln(repairStdout, "cmdshape repair will rewrite the fully managed ~/.config/cmdshape state.")
	if err != nil {
		return false, err
	}
	_, err = fmt.Fprint(repairStdout, "Continue? [y/N]: ")
	if err != nil {
		return false, err
	}

	line, err := bufio.NewReader(repairStdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}
