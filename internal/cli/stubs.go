package cli

import (
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/spf13/cobra"
)

func notImplemented(feature string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		return apperr.NotImpl(feature)
	}
}

func stubTree(use, short string, children ...string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short}
	if len(children) == 0 {
		cmd.RunE = notImplemented(use)
		return cmd
	}
	for _, name := range children {
		child := &cobra.Command{Use: name, Short: name, RunE: notImplemented(use + " " + name)}
		cmd.AddCommand(child)
	}
	return cmd
}

func newJobCmd(_ *App) *cobra.Command {
	return stubTree("job", "Inspect publication jobs", "list", "show", "retry", "cancel")
}

func newHistoryCmd(_ *App) *cobra.Command {
	return stubTree("history", "Published items", "list", "show", "export")
}
