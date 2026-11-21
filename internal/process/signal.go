// Package process provides cancellation for CLI subprocesses.
package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

type interruption struct{ signal os.Signal }

func (x interruption) Error() string { return x.signal.String() }

// InterruptContext cancels on the first shutdown signal, preserving which
// signal arrived so a running Git command can receive the same one.
func InterruptContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-signals:
			cancel(interruption{signal: sig})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(signals)
		cancel(nil)
	}
}

// Command forwards OS shutdown signals to Git. Ordinary context cancellation
// retains the default kill behavior. WaitDelay bounds the grace period and
// prevents inherited output pipes from holding up shutdown indefinitely.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if name != "git" {
		return cmd
	}
	cmd.WaitDelay = 2 * time.Second
	kill := cmd.Cancel
	cmd.Cancel = func() error {
		if cause, ok := errors.AsType[interruption](context.Cause(ctx)); ok {
			return cmd.Process.Signal(cause.signal)
		}
		return kill()
	}
	return cmd
}
