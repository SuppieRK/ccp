package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	core "go-command-compression-proxy/internal"
	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/replay"
	"go-command-compression-proxy/internal/version"
)

type verifyRunner interface {
	Replay(args []string, events []replay.Event) (core.ReplayResult, error)
	ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error)
}

var newVerifyRunner = func() verifyRunner {
	return core.NewRunner()
}

func RunVerify(args []string) error {
	recordFailure := func(dir, stage string, err error) error {
		auditErr := audit.Append("verify_invocation_finish", map[string]any{
			"dir":     dir,
			"success": false,
			"stage":   stage,
			"error":   err.Error(),
		})
		return errors.Join(err, auditErr)
	}

	if version.Version != "dev" {
		err := fmt.Errorf("ccp verify is only available in dev builds")
		return errors.Join(err, audit.Append("verify_invocation_finish", map[string]any{
			"success": false,
			"stage":   "version_gate",
			"error":   err.Error(),
		}))
	}

	fs := newLifecycleFlagSet("verify")
	dirFlag := fs.String("dir", "", "fixture directory containing command.yaml and replay stream files")
	setLifecycleUsage(
		fs,
		"replay captured fixtures through the current filter",
		[]string{"ccp verify [--dir <path>]"},
		"verify reads command.yaml and optional sequenced stdout.txt/stderr.txt from the fixture directory.",
		"missing stdout.txt or stderr.txt means that stream is empty.",
		"verify always writes verify-output.txt and verify-decisions.txt into the fixture directory.",
		"verify fails when sequence prefixes break cross-stream ordering integrity.",
	)
	handled, err := parseLifecycleFlags(fs, args)
	if err != nil {
		return recordFailure("", "parse_flags", err)
	}
	if handled {
		return audit.Append("verify_invocation_finish", map[string]any{
			"success": true,
			"stage":   "help",
		})
	}
	if len(fs.Args()) > 0 {
		return recordFailure("", "validate_flags", fmt.Errorf("verify does not accept command arguments; use command.yaml in the fixture directory"))
	}

	dir, err := resolveVerifyDir(strings.TrimSpace(*dirFlag))
	if err != nil {
		return recordFailure("", "resolve_dir", err)
	}
	if err := audit.Append("verify_invocation_start", map[string]any{
		"dir": dir,
	}); err != nil {
		return err
	}

	fixture, err := replay.LoadFixture(dir)
	if err != nil {
		return recordFailure(dir, "load_fixture", err)
	}
	events, err := replay.ReadEvents(fixture.StdoutPath, fixture.StderrPath)
	if err != nil {
		return recordFailure(dir, "read_events", err)
	}

	replayed, err := newVerifyRunner().ReplayWithExitCode(fixture.Command.Argv, events, fixture.Command.ExitCode)
	if err != nil {
		return recordFailure(dir, "runner_replay", err)
	}
	if err := os.WriteFile(fixture.VerifyOutput, []byte(replayed.Output), 0o644); err != nil {
		return recordFailure(dir, "write_output", err)
	}
	if err := os.WriteFile(fixture.VerifyDecisions, []byte(replayed.Decisions), 0o644); err != nil {
		return recordFailure(dir, "write_decisions", err)
	}

	if err := audit.Append("verify_invocation_finish", map[string]any{
		"dir":            dir,
		"command":        strings.Join(fixture.Command.Argv, " "),
		"verify_output":  fixture.VerifyOutput,
		"verify_decided": fixture.VerifyDecisions,
		"success":        true,
		"output_bytes":   len(replayed.Output),
		"decision_bytes": len(replayed.Decisions),
	}); err != nil {
		return err
	}
	return nil
}

func resolveVerifyDir(dir string) (string, error) {
	if dir != "" {
		return filepath.Abs(dir)
	}
	return os.Getwd()
}
