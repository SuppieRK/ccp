package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go-command-compression-proxy/internal/cli"
	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/engine/filters"
	"go-command-compression-proxy/internal/lifecycle"
	"go-command-compression-proxy/internal/runner"
	"go-command-compression-proxy/internal/version"
)

func main() {
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

	if handled, err := runLifecycleCommand(opts.CommandArgs); handled {
		if err != nil {
			exitWithErr(1, err)
		}
		return
	}

	r, err := buildRuntime(opts)
	if err != nil {
		exitWithErr(1, err)
	}
	os.Exit(r.Run(opts.CommandArgs))
}

func runLifecycleCommand(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	tail := args[1:]
	switch args[0] {
	case "init":
		return true, lifecycle.RunInit(tail)
	case "gain":
		return true, lifecycle.RunGain(tail, defaultMetricsPath())
	case "history":
		return true, lifecycle.RunHistory(tail, defaultMetricsPath())
	case "upgrade":
		return true, lifecycle.RunUpgrade(tail)
	case "uninstall":
		return true, lifecycle.RunUninstall(tail)
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
  --capture-raw         Save raw stdout/stderr capture files while executing
  --capture-raw-dir     Directory for capture files (requires --capture-raw)
  --confidential        Redact comma-separated substrings from emitted output
  --debug-filter        Emit filter metadata on stderr
  --help, -h            Show help
  --version             Show version

Lifecycle commands:
  init                  Install or update supported agent integrations
  gain                  Show token savings history
  history               Show recorded command history
  upgrade               Upgrade ccp
  uninstall             Remove ccp integrations

Notes:
  - Structured or precision-sensitive output may pass through unchanged.
  - --raw preserves native output unless --confidential is also used.
  - --capture-raw preserves execution semantics while writing capture artifacts.`
}

func buildRuntime(opts cli.Options) (*runner.Runner, error) {
	registry := engine.NewToolFilterRegistry()
	toolFilters := []engine.ToolFilter{
		filters.NewLSCompactor(),
		filters.NewGitToolFilter(),
		filters.NewGradleFilter(),
		filters.NewMavenFilter(),
		filters.NewDenoFilter(),
		filters.NewNodeFilter(),
		filters.NewPythonFilter(),
		filters.NewPytestFilter(),
		filters.NewPIPFilter(),
		filters.NewNPMFilter(),
		filters.NewPNPMFilter(),
		filters.NewYarnFilter(),
		filters.NewNPXFilter(),
		filters.NewGrepFilter(),
		filters.NewFindFilter(),
		filters.NewKubectlToolFilter(),
		filters.NewDockerToolFilter(),
		filters.NewGoToolFilter(),
		filters.NewCargoToolFilter(),
	}
	for _, f := range toolFilters {
		if err := registry.Register(f); err != nil {
			return nil, err
		}
	}

	var eng *engine.Engine
	if !opts.Raw {
		eng = engine.NewEngine(engine.Config{
			NeverDropPatterns: engine.DefaultNeverDropPatterns(),
			Registry:          registry,
			DisableAudit:      !opts.DebugFilter,
		})
	}
	return runner.New(runner.Options{
		Raw:           opts.Raw,
		CaptureRaw:    opts.CaptureRaw,
		CaptureRawDir: opts.CaptureRawDir,
		Confidential:  opts.ConfidentialRedactions,
		DebugFilter:   opts.DebugFilter,
		MetricsPath:   defaultMetricsPath(),
	}, eng, registry), nil
}
