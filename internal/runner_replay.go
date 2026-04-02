package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"go-command-compression-proxy/internal/audit"
	"go-command-compression-proxy/internal/contracts"
	"go-command-compression-proxy/internal/engine"
	"go-command-compression-proxy/internal/replay"
)

type ReplayResult struct {
	Output    string
	Decisions string
}

func (r *Runner) Verify(args []string, stdout, stderr io.Reader) (string, error) {
	events, err := replay.ReadEventReaders(stdout, stderr)
	if err != nil {
		return "", err
	}
	result, err := r.ReplayWithExitCode(args, events, 0)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (r *Runner) ReplayWithExitCode(args []string, events []replay.Event, exitCode int) (ReplayResult, error) {
	command, err := ParseCommandArgs(args)
	if err != nil {
		return ReplayResult{}, err
	}
	auditCommand := r.auditCommand(command.RawInput)
	if err := audit.Append("verify_start", map[string]any{
		"command": auditCommand,
		"tool":    command.Tool,
	}); err != nil {
		return ReplayResult{}, err
	}

	registry, err := r.loadRegistry()
	if err != nil {
		return ReplayResult{}, errors.Join(err, audit.Append("verify_registry_error", map[string]any{
			"command": auditCommand,
			"tool":    command.Tool,
			"error":   err.Error(),
		}))
	}
	resolved := registry.Resolve(command)
	command, err = resolved.PrepareCommand(command)
	if err != nil {
		return ReplayResult{}, err
	}
	command.Dispatch = resolved.Dispatch(command)
	state := engine.NewEngine(registry).Start(command)
	collector := &replayCollector{}
	for _, event := range events {
		var (
			action  contracts.Action
			entries []engine.BufferEntry
		)
		switch event.Stream {
		case contracts.StreamStderr:
			action, entries = state.StderrAction(event.Line)
		default:
			action, entries = state.StdoutAction(event.Line)
		}
		collector.recordInput(event, action, entries)
	}
	exitAction, exitEntries := state.ExitAction(exitCode)
	collector.recordExit(exitAction, exitEntries)
	if err := audit.Append("verify_finish", map[string]any{
		"command":      auditCommand,
		"tool":         command.Tool,
		"dispatch":     command.Dispatch,
		"output_bytes": len(collector.output.String()),
	}); err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{
		Output:    collector.output.String(),
		Decisions: collector.decisions.String(),
	}, nil
}

type replayCollector struct {
	output    bytes.Buffer
	decisions bytes.Buffer
}

func (c *replayCollector) writeEntries(entries []engine.BufferEntry) int {
	written := 0
	for _, entry := range entries {
		written += len(entry.Line)
		_, _ = c.output.WriteString(entry.Line)
	}
	return written
}

func (c *replayCollector) recordInput(event replay.Event, action contracts.Action, emitted []engine.BufferEntry) {
	c.writeDecision(labelForInputAction(action), event.Line)
	c.writeEntries(emitted)
	if action.Kind == contracts.ActionReplace {
		c.writeSynthetic(emitted)
	}
}

func (c *replayCollector) recordExit(_ contracts.Action, emitted []engine.BufferEntry) {
	c.writeEntries(emitted)
}

func (c *replayCollector) writeSynthetic(entries []engine.BufferEntry) {
	for _, entry := range entries {
		c.writeDecision("<emit>", entry.Line)
	}
}

func (c *replayCollector) writeDecision(label, line string) {
	for _, part := range splitDecisionLines(line) {
		text := strings.TrimSuffix(part, "\n")
		_, _ = fmt.Fprintf(&c.decisions, "%-10s| %s\n", label, text)
	}
}

func labelForInputAction(action contracts.Action) string {
	switch action.Kind {
	case contracts.ActionIgnore:
		return "<skip>"
	case contracts.ActionReplace:
		return "<replace>"
	case contracts.ActionEmit, contracts.ActionKeep, "":
		return "<keep>"
	default:
		return "<keep>"
	}
}

func splitDecisionLines(line string) []string {
	if line == "" {
		return []string{""}
	}
	parts := strings.SplitAfter(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
	if parts[len(parts)-1] == "" {
		return parts[:len(parts)-1]
	}
	return parts
}
