package cli

import (
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/profile"
	"github.com/maskedsyntax/goggles/internal/schedule"
	"github.com/spf13/cobra"
)

func newScheduleCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "schedule", Short: "Recurring posting slots per destination"}
	var dest, profileName, tz string

	set := &cobra.Command{
		Use:   "set [HH:MM...]",
		Short: "Replace recurring slots",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dest != "" && profileName != "" {
				return apperr.Invalid("use --destination or --profile, not both")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true, "times": args})
			}
			if profileName != "" {
				out, err := schedule.SetProfile(cmd.Context(), app.DB, profileName, args, tz)
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"schedules": out})
			}
			if dest == "" {
				return apperr.Invalid("pass --destination or --profile")
			}
			slots, err := schedule.Set(cmd.Context(), app.DB, dest, args, tz)
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"destination": dest, "slots": slots})
		},
	}
	set.Flags().StringVar(&dest, "destination", "", "destination alias")
	set.Flags().StringVar(&profileName, "profile", "", "copy slots to every destination in a profile")
	set.Flags().StringVar(&tz, "timezone", "", "IANA timezone (defaults to destination timezone)")

	show := &cobra.Command{
		Use:   "show",
		Short: "Show schedule slots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			if profileName != "" {
				view, err := profile.Show(cmd.Context(), app.DB, profileName)
				if err != nil {
					return err
				}
				out := map[string]any{}
				for _, d := range view.Destinations {
					slots, err := schedule.List(cmd.Context(), app.DB, d.Alias)
					if err != nil {
						return err
					}
					out[d.Alias] = slotsWithNext(slots)
				}
				return app.Out.Success(map[string]any{"profile": profileName, "schedules": out})
			}
			if dest == "" {
				return apperr.Invalid("pass --destination or --profile")
			}
			slots, err := schedule.List(cmd.Context(), app.DB, dest)
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"destination": dest, "slots": slotsWithNext(slots)})
		},
	}
	show.Flags().StringVar(&dest, "destination", "", "destination alias")
	show.Flags().StringVar(&profileName, "profile", "", "profile name")

	clear := &cobra.Command{
		Use:   "clear",
		Short: "Remove all slots for a destination",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dest == "" {
				return apperr.Invalid("--destination is required")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			if app.DryRun {
				return app.Out.Success(map[string]any{"dry_run": true})
			}
			if err := schedule.Clear(cmd.Context(), app.DB, dest); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"cleared": dest})
		},
	}
	clear.Flags().StringVar(&dest, "destination", "", "destination alias")

	cmd.AddCommand(set, show, clear, scheduleEnableCmd(app, true), scheduleEnableCmd(app, false))
	return cmd
}

func scheduleEnableCmd(app *App, enabled bool) *cobra.Command {
	var dest string
	use := "disable"
	if enabled {
		use = "enable"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: use + " recurring slots for a destination",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dest == "" {
				return apperr.Invalid("--destination is required")
			}
			if err := app.openDB(); err != nil {
				return err
			}
			if err := schedule.SetEnabled(cmd.Context(), app.DB, dest, enabled); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"destination": dest, "enabled": enabled})
		},
	}
	cmd.Flags().StringVar(&dest, "destination", "", "destination alias")
	return cmd
}

func slotsWithNext(slots []schedule.Slot) []map[string]any {
	out := make([]map[string]any, 0, len(slots))
	times := make([]string, 0, len(slots))
	tz := "UTC"
	for _, s := range slots {
		times = append(times, s.TimeOfDay)
		if s.Timezone != "" {
			tz = s.Timezone
		}
	}
	var next string
	if loc, err := schedule.LoadTZ(tz); err == nil {
		if t, err := schedule.Next(time.Now(), loc, times); err == nil {
			next = t.UTC().Format(time.RFC3339)
		}
	}
	for _, s := range slots {
		row := map[string]any{
			"time":     s.TimeOfDay,
			"timezone": s.Timezone,
			"enabled":  s.Enabled,
			"next":     next,
		}
		out = append(out, row)
	}
	return out
}
