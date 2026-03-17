package main

import (
	"fmt"
	"os"
	"path/filepath"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/cli"
	"go-command-compression-proxy/internal/lifecycle"
	"go-command-compression-proxy/internal/version"
)

var lifecycleDispatch = runLifecycleCommand

type executionRuntime interface {
	Run(args []string) int
}

func main() {
	if err := audit.ConfigureDefault(); err != nil {
		exitWithErr(1, err)
	}

	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		exitWithErr(2, err)
	}

	if opts.ShowHelp {
		fmt.Println(usageText())
		return
	}

	if opts.ShowVersion {
		fmt.Println(version.Version)
		return
	}

	if len(opts.CommandArgs) == 0 {
		exitWithMsg(2, usageText())
	}

	if handled, exitCode, err := runInvocation(opts); handled {
		if err != nil {
			exitWithErr(exitCode, err)
		}
		return
	}
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
	os.Exit(r.Run(opts.CommandArgs))
	return true, 0, nil
}

func isLifecycleCommand(args []string) bool {
	return cli.IsManagedArgs(args)
}

func runLifecycleCommand(args []string) (bool, error) {
	if !isLifecycleCommand(args) {
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
	case "verify":
		return true, lifecycle.RunVerify(tail)
	case "upgrade":
		return true, lifecycle.RunUpgrade(tail)
	case "uninstall":
		return true, lifecycle.RunUninstall(tail)
	case "repair":
		return true, lifecycle.RunRepair(tail)
	case "filter":
		return true, lifecycle.RunFilter(tail)
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
	if _, werr := fmt.Fprintln(os.Stderr, msg); werr != nil {
		os.Exit(1)
	}
	os.Exit(code)
}

func defaultMetricsPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, ".ccp", "gain.db")
}

func usageText() string {
	return `ccp - command compression proxy for coding-agent workflows

Usage:
  ccp [execution flags] <command> [args...]
  ccp <lifecycle-command> [args...]

Execution flags:
  --raw                 Bypass semantic compaction and pass through native output
  --confidential        Redact comma-separated substrings from emitted output
  --help, -h            Show help
  --version             Show version

Lifecycle commands:
  capture               Write command.yaml, sequenced streams, and replay output artifacts
  init                  Install or update supported agent integrations
  gain                  Show token savings summary and recent proof output
  filter                YAML filter authoring helpers
  history               Show recorded command history
  repair                Rewrite managed CCP home state to canonical shipped content
  verify                Replay one fixture directory through the current filter
  upgrade               Upgrade ccp
  uninstall             Remove ccp integrations

Notes:
  - Run ccp gain after install or init to verify savings on real work.
  - Structured or precision-sensitive output may pass through unchanged.
  - --raw preserves native output unless --confidential is also used.`
}

func buildRuntime(opts cli.Options) (executionRuntime, error) {
	return &coreExecutionRuntime{
		runner: core.NewRunnerWithOptions(core.Options{
			Raw:          opts.Raw,
			Confidential: opts.ConfidentialRedactions,
			MetricsPath:  defaultMetricsPath(),
		}),
	}, nil
}

type coreExecutionRuntime struct {
	runner *core.Runner
}

func (r *coreExecutionRuntime) Run(args []string) int {
	if len(args) == 0 {
		return 2
	}
	code, err := r.runner.Run(args)
	if err != nil {
		exitWithErr(code, err)
	}
	return code
}
