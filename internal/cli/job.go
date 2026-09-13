package cli

import (
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/jobs"
	"github.com/spf13/cobra"
)

func newJobCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Inspect publication jobs"}
	var status, dest string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			items, err := jobs.List(cmd.Context(), app.DB, jobs.Filter{Status: status, Destination: dest, Limit: limit})
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Out.Success(map[string]any{"jobs": items})
			}
			rows := make([][]string, 0, len(items))
			for _, j := range items {
				rows = append(rows, []string{j.ID, j.Destination, j.Platform, j.Status, j.LastErrorCode})
			}
			return app.Out.Table([]string{"ID", "DESTINATION", "PLATFORM", "STATUS", "ERROR"}, rows)
		},
	}
	list.Flags().StringVar(&status, "status", "", "filter by status")
	list.Flags().StringVar(&dest, "destination", "", "filter by destination alias")
	list.Flags().IntVar(&limit, "limit", 50, "max rows")

	cmd.AddCommand(
		list,
		&cobra.Command{
			Use:   "show <id>",
			Short: "Show one job",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				j, err := jobs.Get(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"job": j})
			},
		},
		&cobra.Command{
			Use:   "cancel <id>",
			Short: "Cancel a job that has not completed",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "id": args[0]})
				}
				if err := jobs.Cancel(cmd.Context(), app.DB, args[0]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"cancelled": args[0]})
			},
		},
		&cobra.Command{
			Use:   "retry <id>",
			Short: "Retry a failed job",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				j, err := jobs.Get(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				if j.Status != jobs.Failed && j.Status != jobs.RetryWait {
					return apperr.Invalid("only failed or retry_wait jobs can be retried")
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "id": j.ID, "destination": j.Destination})
				}
				host, err := app.openHost(cmd.Context())
				if err != nil {
					return err
				}
				res, err := app.runner(host).Retry(cmd.Context(), j)
				if err != nil {
					return err
				}
				if err := app.Out.Result(res); err != nil {
					return err
				}
				if !res.Success {
					return apperr.Printed(apperr.New(apperr.MetaRequestFailed, "retry failed"))
				}
				return nil
			},
		},
	)
	return cmd
}
