package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	core "github.com/SuppieRK/cmdshape/internal"
	"github.com/SuppieRK/cmdshape/internal/audit"
	"github.com/SuppieRK/cmdshape/internal/replay"
)

type verifyRunner interface {
	ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (core.ReplayResult, error)
}

var newVerifyRunner = func() verifyRunner {
	return core.NewRunnerWithOptions(core.Options{})
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

	fs := newLifecycleFlagSet("verify")
	dirFlag := fs.String("dir", "", "fixture directory containing command.yaml and replay stream files")
	setLifecycleUsage(
		fs,
		"replay captured fixtures through the current filter",
		[]string{"cmdshape verify [--dir <path>]"},
		"verify reads command.yaml and optional sequenced stdout.txt/stderr.txt from the fixture directory.",
		"missing stdout.txt or stderr.txt means that stream is empty.",
		"verify always writes verify-output.txt, verify-stdout.txt, verify-stderr.txt, verify-decisions.txt, and verify-dispatch.txt into the fixture directory.",
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
		return recordFailure("", "validate_flags", errors.New("verify does not accept command arguments; use command.yaml in the fixture directory"))
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
	artifacts := []struct {
		stage string
		path  string
		body  string
	}{
		{stage: "write_output", path: fixture.VerifyOutput, body: replayed.Output},
		{stage: "write_stdout", path: fixture.VerifyStdout, body: replayed.Stdout},
		{stage: "write_stderr", path: fixture.VerifyStderr, body: replayed.Stderr},
		{stage: "write_decisions", path: fixture.VerifyDecisions, body: replayed.Decisions},
		{stage: "write_dispatch", path: fixture.VerifyDispatch, body: replayed.Dispatch + "\n"},
	}
	for _, artifact := range artifacts {
		if err := replay.WriteArtifact(artifact.path, []byte(artifact.body), 0o644); err != nil {
			return recordFailure(dir, artifact.stage, err)
		}
	}

	if err := audit.Append("verify_invocation_finish", map[string]any{
		"dir":             dir,
		"command":         strings.Join(fixture.Command.Argv, " "),
		"verify_output":   fixture.VerifyOutput,
		"verify_stdout":   fixture.VerifyStdout,
		"verify_stderr":   fixture.VerifyStderr,
		"verify_decided":  fixture.VerifyDecisions,
		"verify_dispatch": fixture.VerifyDispatch,
		"success":         true,
		"output_bytes":    len(replayed.Output),
		"decision_bytes":  len(replayed.Decisions),
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
