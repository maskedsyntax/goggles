package cli

import (
	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/spf13/cobra"
)

func newDestinationCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "destination", Short: "Destination safety settings"}
	var limit int
	set := &cobra.Command{
		Use:   "set <alias>",
		Short: "Set destination options",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("daily-limit") {
				return apperr.Invalid("pass --daily-limit")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "alias": args[0], "daily_limit": limit})
			}
			if err := account.SetDailyLimit(cmd.Context(), app.DB, args[0], limit); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"alias": args[0], "daily_limit": limit})
		},
	}
	set.Flags().IntVar(&limit, "daily-limit", 0, "max successful publishes per local day (0 = unlimited)")
	cmd.AddCommand(set)
	return cmd
}
