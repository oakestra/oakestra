package prompt

import (
	"context"
	"errors"
	"os"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

func PromptAvailable() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

func RunForm(ctx context.Context, form *huh.Form) error {
	err := form.WithInput(os.Stdin).WithOutput(os.Stderr).RunWithContext(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return context.Canceled
	}
	return err
}

func Confirm(ctx context.Context, title string) (bool, error) {
	confirmed := true
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(title).Value(&confirmed)))
	err := RunForm(ctx, form)
	return confirmed, err
}
