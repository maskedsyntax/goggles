package cli

import (
	"encoding/json"
	"os"

	"github.com/maskedsyntax/goggles/internal/jobs"
	"github.com/spf13/cobra"
)

func newHistoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "history", Short: "Published items"}
	var dest, outPath string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List successful publications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			items, err := jobs.ListPublications(cmd.Context(), app.DB, dest, limit)
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Out.Success(map[string]any{"publications": items})
			}
			rows := make([][]string, 0, len(items))
			for _, p := range items {
				rows = append(rows, []string{p.ID, p.Destination, p.Platform, p.ExternalMediaID, p.PublishedAt})
			}
			return app.Out.Table([]string{"ID", "DESTINATION", "PLATFORM", "MEDIA", "PUBLISHED"}, rows)
		},
	}
	list.Flags().StringVar(&dest, "destination", "", "filter by destination")
	list.Flags().IntVar(&limit, "limit", 50, "max rows")

	export := &cobra.Command{
		Use:   "export",
		Short: "Export publication history as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			items, err := jobs.ListPublications(cmd.Context(), app.DB, dest, 0)
			if err != nil {
				return err
			}
			if outPath == "" {
				return app.Out.Success(map[string]any{"publications": items})
			}
			data, err := json.MarshalIndent(items, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"path": outPath, "count": len(items)})
		},
	}
	export.Flags().StringVar(&dest, "destination", "", "filter by destination")
	export.Flags().StringVar(&outPath, "out", "", "write to a file instead of stdout")

	cmd.AddCommand(
		list, export,
		&cobra.Command{
			Use:   "show <id>",
			Short: "Show one publication (id, job id, or external media id)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				p, err := jobs.GetPublication(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"publication": p})
			},
		},
	)
	return cmd
}
