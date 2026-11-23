package main

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/oakestra/oakestra/installer/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// Restore default signal handling after the first interrupt.
	context.AfterFunc(ctx, stop)
	err := cli.Execute(ctx)
	interrupted := ctx.Err() != nil || errors.Is(err, context.Canceled)
	stop()

	if interrupted {
		os.Exit(130)
	}
	if err != nil {
		os.Exit(1)
	}
}
