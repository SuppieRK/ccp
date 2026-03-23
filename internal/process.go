package core

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

func DefaultExecutionContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, defaultExecutionSignals()...)
}

func runnerContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent != nil {
		return context.WithCancel(parent)
	}
	return DefaultExecutionContext(parent)
}

func CommandWithPipesContext(ctx context.Context, name string, args []string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	configureManagedCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	return cmd, stdout, stderr, nil
}
