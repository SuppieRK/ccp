package main

import (
	"fmt"
	"io"
	"os"

	core "github.com/SuppieRK/cmdshape/internal"
	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/cli"
	"github.com/SuppieRK/cmdshape/internal/lifecycle"
	"github.com/SuppieRK/cmdshape/internal/metrics"
	"github.com/SuppieRK/cmdshape/internal/version"
)

var lifecycleDispatch = runLifecycleCommand

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if err := lifecycle.RunBrandMigrationAuto(); err != nil {
		return writeErr(stderr, 1, err)
	}
	// Audit is intentionally best-effort: startup must never fail before argument parsing
	// or command execution just because the audit log path is blocked or unwritable.
	_ = audit.ConfigureDefault()

	opts, err := cli.Parse(args)
	if err != nil {
		return writeErr(stderr, 2, err)
	}

	if opts.ShowHelp {
		return writeMsg(stdout, 0, usageText())
	}

	if opts.ShowVersion {
		return writeMsg(stdout, 0, version.Version)
	}

	if len(opts.CommandArgs) == 0 {
		return writeMsg(stderr, 2, usageText())
	}

	if handled, exitCode, err := runInvocation(opts); handled {
		if err != nil {
			return writeErr(stderr, exitCode, err)
		}
		return exitCode
	}

	return 0
}

func runInvocation(opts cli.Options) (handled bool, exitCode int, err error) {
	if handled, err := lifecycleDispatch(opts.CommandArgs); handled {
		if err != nil {
			return true, 1, err
		}
		return true, 0, nil
	}
	r, err := buildRuntime(opts)
	if err != nil {
		return true, 1, err
	}
	code, err := r.Run(opts.CommandArgs)
	return true, code, err
}

func runLifecycleCommand(args []string) (bool, error) {
	if !cli.IsManagedArgs(args) {
		return false, nil
	}
	tail := args[1:]
	switch args[0] {
	case "init":
		return true, lifecycle.RunInit(tail)
	case "capture":
		return true, lifecycle.RunCapture(tail)
	case "gain":
		return true, lifecycle.RunGain(tail, defaultMetricsPath())
	case "history":
		return true, lifecycle.RunHistory(tail, defaultMetricsPath())
	case "recovery":
		return true, lifecycle.RunRecovery(tail)
	case "verify":
		return true, lifecycle.RunVerify(tail)
	case "upgrade":
		return true, lifecycle.RunUpgrade(tail)
	case "uninstall":
		return true, lifecycle.RunUninstall(tail)
	case "repair":
		return true, lifecycle.RunRepair(tail)
	case "migrate":
		return true, lifecycle.RunBrandMigration(tail)
	case "filter":
		return true, lifecycle.RunFilterWithMetrics(tail, defaultMetricsPath())
	default:
		return false, nil
	}
}

func exitWithErr(code int, err error) {
	if err != nil {
		exitWithMsg(code, err.Error())
		return
	}
	os.Exit(code)
}

func exitWithMsg(code int, msg string) {
	os.Exit(writeMsg(os.Stderr, code, msg))
}

func writeErr(w io.Writer, code int, err error) int {
	if err == nil {
		return code
	}
	return writeMsg(w, code, err.Error())
}

func writeMsg(w io.Writer, code int, msg string) int {
	if _, err := fmt.Fprintln(w, msg); err != nil {
		return 1
	}
	return code
}

func defaultMetricsPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return metrics.ProjectPath(cwd)
}

func usageText() string {
	return `cmdshape — Shape command output. Preserve command truth.

Usage:
  cmdshape [execution flags] <command> [args...]
  cmdshape <lifecycle-command> [args...]

Execution flags:
  --raw                 Bypass semantic compaction and pass through native output
  --confidential        Redact comma-separated substrings from emitted output
  --help, -h            Show help
  --version             Show version

Lifecycle commands:
  capture               Write command.yaml, sequenced streams, and replay output artifacts
  init                  Install or update supported agent integrations
  gain                  Show token savings summary and recent proof output (--global supported)
  filter                YAML filter authoring helpers
  history               Show recorded command history (--global supported)
  recovery              Manage opt-in bounded raw failure recovery
  migrate               Inspect or retry previous-installation cleanup
  repair                Rewrite managed cmdshape home state to canonical shipped content
  verify                Replay one fixture directory through the current filter
  upgrade               Upgrade cmdshape
  uninstall             Remove selected integrations or fully uninstall cmdshape

Notes:
  - Run cmdshape gain after install or init to verify savings on real work.
  - Structured or precision-sensitive output may pass through unchanged.
  - --raw preserves native output unless --confidential is also used.`
}

func buildRuntime(opts cli.Options) (*core.Runner, error) {
	return core.NewRunnerWithOptions(core.Options{
		Raw:          opts.Raw,
		Confidential: opts.ConfidentialRedactions,
		MetricsPath:  defaultMetricsPath(),
	}), nil
}
