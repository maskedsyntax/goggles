package cli

import (
	"fmt"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/queue"
	"github.com/spf13/cobra"
)

func newQueueCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "queue", Short: "Manage destination and profile queues"}

	var profileName, dest, at, status string

	add := &cobra.Command{
		Use:   "add <file>",
		Short: "Enqueue a video",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			scheduled, err := parseOptionalRFC3339(at)
			if err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "path": args[0], "profile": profileName, "destination": dest})
			}
			item, err := queue.Add(cmd.Context(), app.DB, queue.AddInput{
				Path: args[0], Profile: profileName, Destination: dest, ScheduledFor: scheduled,
			})
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"item": item})
		},
	}
	add.Flags().StringVar(&profileName, "profile", "", "enqueue for a profile")
	add.Flags().StringVar(&dest, "destination", "", "enqueue for one destination")
	add.Flags().StringVar(&at, "at", "", "RFC3339 one-off time")

	fill := &cobra.Command{
		Use:   "fill <dir>",
		Short: "Enqueue every mp4/mov in a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "dir": args[0]})
			}
			items, err := queue.Fill(cmd.Context(), app.DB, args[0], queue.AddInput{Profile: profileName, Destination: dest})
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"count": len(items), "items": items})
		},
	}
	fill.Flags().StringVar(&profileName, "profile", "", "enqueue for a profile")
	fill.Flags().StringVar(&dest, "destination", "", "enqueue for one destination")

	list := &cobra.Command{
		Use:   "list",
		Short: "List queue items",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			items, err := queue.List(cmd.Context(), app.DB, queue.ListFilter{Profile: profileName, Destination: dest, Status: status})
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Out.Success(map[string]any{"items": items})
			}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				target := it.Profile
				if target == "" {
					target = it.Destination
				}
				rows = append(rows, []string{it.ID, fmt.Sprintf("%d", it.Position), it.Status, target, it.FilePath})
			}
			return app.Out.Table([]string{"ID", "POS", "STATUS", "TARGET", "FILE"}, rows)
		},
	}
	list.Flags().StringVar(&profileName, "profile", "", "filter by profile")
	list.Flags().StringVar(&dest, "destination", "", "filter by destination")
	list.Flags().StringVar(&status, "status", "", "filter by status")

	clear := &cobra.Command{
		Use:   "clear",
		Short: "Remove ready/failed/cancelled items",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true})
			}
			n, err := queue.Clear(cmd.Context(), app.DB, queue.ListFilter{Profile: profileName, Destination: dest})
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"removed": n})
		},
	}
	clear.Flags().StringVar(&profileName, "profile", "", "limit to a profile")
	clear.Flags().StringVar(&dest, "destination", "", "limit to a destination")

	pause := &cobra.Command{
		Use:   "pause",
		Short: "Pause automatic publishing for a queue",
		Args:  cobra.NoArgs,
		RunE:  queuePauseRun(app, true),
	}
	pause.Flags().StringVar(&profileName, "profile", "", "pause a profile queue")
	pause.Flags().StringVar(&dest, "destination", "", "pause a destination queue")

	resume := &cobra.Command{
		Use:   "resume",
		Short: "Resume a paused queue",
		Args:  cobra.NoArgs,
		RunE:  queuePauseRun(app, false),
	}
	resume.Flags().StringVar(&profileName, "profile", "", "resume a profile queue")
	resume.Flags().StringVar(&dest, "destination", "", "resume a destination queue")

	cmd.AddCommand(
		add, fill, list, clear, pause, resume,
		&cobra.Command{
			Use:   "remove <id>",
			Short: "Remove one queue item",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "id": args[0]})
				}
				if err := queue.Remove(cmd.Context(), app.DB, args[0]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"removed": args[0]})
			},
		},
	)
	return cmd
}

func queuePauseRun(app *App, paused bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := app.openDB(); err != nil {
			return err
		}
		profileName, _ := cmd.Flags().GetString("profile")
		dest, _ := cmd.Flags().GetString("destination")
		if app.DryRun {
			return app.Out.Success(map[string]any{"dry_run": true, "paused": paused})
		}
		if err := queue.SetPaused(cmd.Context(), app.DB, profileName, dest, paused); err != nil {
			return err
		}
		return app.Out.Success(map[string]any{"paused": paused, "profile": profileName, "destination": dest})
	}
}

func parseOptionalRFC3339(at string) (string, error) {
	if at == "" {
		return "", nil
	}
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return "", apperr.Invalid("--at must be RFC3339")
	}
	return ts.UTC().Format(time.RFC3339), nil
}
