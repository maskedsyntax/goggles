package cli

import (
	"fmt"

	"github.com/maskedsyntax/goggles/internal/profile"
	"github.com/spf13/cobra"
)

func newProfileCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Group destinations into a publishing identity"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "create <name>",
			Short: "Create a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "name": args[0]})
				}
				p, err := profile.Create(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"profile": p})
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List profiles",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				list, err := profile.List(cmd.Context(), app.DB)
				if err != nil {
					return err
				}
				if app.JSON {
					return app.Out.Success(map[string]any{"profiles": list})
				}
				rows := make([][]string, 0, len(list))
				for _, p := range list {
					rows = append(rows, []string{p.Name, fmt.Sprintf("%v", p.Enabled), p.ID})
				}
				return app.Out.Table([]string{"NAME", "ENABLED", "ID"}, rows)
			},
		},
		&cobra.Command{
			Use:   "show <name>",
			Short: "Show a profile and its destinations",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				view, err := profile.Show(cmd.Context(), app.DB, args[0])
				if err != nil {
					return err
				}
				if app.JSON {
					return app.Out.Success(map[string]any{"profile": view.Profile, "destinations": view.Destinations})
				}
				rows := [][]string{{"name", view.Profile.Name}, {"enabled", fmt.Sprintf("%v", view.Profile.Enabled)}}
				for _, d := range view.Destinations {
					rows = append(rows, []string{"destination", d.Alias + " (" + string(d.Platform) + ")"})
				}
				return app.Out.Table([]string{"FIELD", "VALUE"}, rows)
			},
		},
		&cobra.Command{
			Use:   "add-destination <profile> <destination>",
			Short: "Attach a destination to a profile",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "profile": args[0], "destination": args[1]})
				}
				if err := profile.AddDestination(cmd.Context(), app.DB, args[0], args[1]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"profile": args[0], "destination": args[1]})
			},
		},
		&cobra.Command{
			Use:   "remove-destination <profile> <destination>",
			Short: "Detach a destination from a profile",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "profile": args[0], "destination": args[1]})
				}
				if err := profile.RemoveDestination(cmd.Context(), app.DB, args[0], args[1]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"removed": args[1], "profile": args[0]})
			},
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Delete a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.openDB(); err != nil {
					return err
				}
				if app.DryRun {
					return app.Out.Success(map[string]any{"dry_run": true, "name": args[0]})
				}
				if err := profile.Remove(cmd.Context(), app.DB, args[0]); err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"removed": args[0]})
			},
		},
	)
	return cmd
}
