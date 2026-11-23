package enact

import (
	"context"
	"strings"

	"github.com/oakestra/oakestra/installer/util/command"
)

// Step is a single change made by an installation plan.
type Step interface {
	Description() string
	Run(context.Context) error
}

// CommandStep runs an external system command.
type CommandStep struct {
	Command string
	Args    []string
}

func (c CommandStep) Description() string {
	return "Run: " + c.Command + " " + strings.Join(c.Args, " ")
}

func (c CommandStep) Run(ctx context.Context) error {
	return command.RunForwarded(ctx, c.Command, c.Args...)
}

// FuncStep runs a Go function.
type FuncStep struct {
	Name string
	Fn   func(context.Context) error
}

func (f FuncStep) Description() string {
	return f.Name
}

func (f FuncStep) Run(ctx context.Context) error {
	return f.Fn(ctx)
}
