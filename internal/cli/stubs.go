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

func newAuthCmd(app *App) *cobra.Command {
	cmd := stubTree("auth", "Authenticate with Meta and Google", "login", "status", "logout")
	for _, c := range cmd.Commands() {
		if c.Use == "status" {
			c.RunE = func(cmd *cobra.Command, args []string) error {
				return app.Out.Success(map[string]any{
					"instagram": "unauthenticated",
					"youtube":   "unauthenticated",
				})
			}
		}
	}
	return cmd
}

func newPublishCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <file>",
		Short: "Publish a video now or at a timestamp",
		Args:  cobra.ExactArgs(1),
		RunE:  notImplemented("publish"),
	}
	cmd.Flags().String("profile", "", "publish to every destination in a profile")
	cmd.Flags().String("destination", "", "publish to one destination")
	cmd.Flags().String("platform", "", "limit a profile publish to platforms")
	cmd.Flags().String("at", "", "RFC3339 timestamp")
	cmd.Flags().Bool("allow-duplicate", false, "allow posting the same hash to a destination again")
	return cmd
}

func newQueueCmd(_ *App) *cobra.Command {
	return stubTree("queue", "Manage destination and profile queues", "add", "list", "fill", "remove", "clear", "pause", "resume")
}

func newScheduleCmd(_ *App) *cobra.Command {
	return stubTree("schedule", "Recurring posting slots per destination", "set", "show", "clear", "enable", "disable")
}

func newJobCmd(_ *App) *cobra.Command {
	return stubTree("job", "Inspect publication jobs", "list", "show", "retry", "cancel")
}

func newHistoryCmd(_ *App) *cobra.Command {
	return stubTree("history", "Published items", "list", "show", "export")
}

func newDaemonCmd(_ *App) *cobra.Command {
	return stubTree("daemon", "Background scheduler", "run", "install", "uninstall", "status", "logs")
}
