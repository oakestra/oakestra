package enact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/oakestra/oakestra/installer/util/prompt"
)

// Plan holds a series of steps to achieve a specific goal.
type Plan struct {
	Info  string
	Goal  string
	Steps []Step
}

// Execute asks for confirmation when needed and runs the steps sequentially.
func (p Plan) Execute(ctx context.Context, logger *slog.Logger, autoConfirm bool) (bool, error) {
	var descriptions strings.Builder
	for _, step := range p.Steps {
		_, _ = fmt.Fprintln(&descriptions, ">>", step.Description())
	}

	if autoConfirm {
		logger.InfoContext(ctx, fmt.Sprintf("%s, running these steps to %s:\n%s",
			p.Info, p.Goal, strings.TrimSpace(descriptions.String())))
	} else {
		confirmed, err := prompt.Confirm(ctx, fmt.Sprintf("%s.\nRun these steps to %s?\n%s",
			p.Info, p.Goal, descriptions.String()))
		if err != nil || !confirmed {
			return false, err
		}
	}

	for _, step := range p.Steps {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		if err := step.Run(ctx); err != nil {
			return true, fmt.Errorf("failed during step %q: %w", step.Description(), err)
		}
	}
	return true, nil
}
