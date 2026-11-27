package command

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

func RunForwarded(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return commandError(ctx, cmd.Run())
}

func RunSilent(ctx context.Context, name string, args ...string) error {
	return commandError(ctx, exec.CommandContext(ctx, name, args...).Run())
}

func RunCaptured(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	return output, commandError(ctx, err)
}

func RunCapturedWithInput(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = input
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	return output, commandError(ctx, err)
}

// IsContextError reports whether err includes cancellation or a context deadline.
func IsContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func commandError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
		return errors.Join(ctxErr, err)
	}
	return err
}
