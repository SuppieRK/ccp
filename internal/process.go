package core

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

type executionSignal struct{ signal os.Signal }

func (e executionSignal) Error() string { return e.signal.String() }

func DefaultExecutionContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancelCause := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, defaultExecutionSignals()...)
	go func() {
		select {
		case received := <-signals:
			cancelCause(executionSignal{signal: received})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		cancelCause(context.Canceled)
	}
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
	configureManagedCommand(cmd, ctx)
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

func CommandAttachedContext(ctx context.Context, name string, args []string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	configureManagedCommand(cmd, ctx)
	return cmd
}
