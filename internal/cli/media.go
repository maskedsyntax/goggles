package cli

import (
	"fmt"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/profile"
	"github.com/spf13/cobra"
)

func newMediaCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "media", Short: "Inspect and validate video files"}
	var platformFlag, profileFlag string
	check := &cobra.Command{
		Use:   "check <file>",
		Short: "Validate a video with ffprobe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platforms, err := resolveCheckPlatforms(cmd, app, platformFlag, profileFlag)
			if err != nil {
				return err
			}
			rep, err := media.CheckFile(cmd.Context(), args[0], platforms)
			payload := map[string]any{"report": rep}
			if err != nil {
				if app.JSON {
					_ = app.Out.Result(map[string]any{"success": false, "report": rep, "error": jsonError(err)})
					return apperr.Printed(err)
				}
				if rep.Info.Path != "" {
					_ = app.Out.Table([]string{"CHECK", "OK", "MESSAGE"}, checkRows(rep))
				}
				return err
			}
			if app.JSON {
				payload["success"] = true
				payload["info"] = rep.Info
				payload["checks"] = rep.Checks
				return app.Out.Result(payload)
			}
			return app.Out.Table([]string{"CHECK", "OK", "MESSAGE"}, checkRows(rep))
		},
	}
	check.Flags().StringVar(&platformFlag, "platform", "", "instagram, youtube, or comma-separated list")
	check.Flags().StringVar(&profileFlag, "profile", "", "validate against all destinations in a profile")
	cmd.AddCommand(check)
	return cmd
}

func resolveCheckPlatforms(cmd *cobra.Command, app *App, platformFlag, profileFlag string) ([]platform.Platform, error) {
	if platformFlag != "" && profileFlag != "" {
		return nil, apperr.Invalid("use --platform or --profile, not both")
	}
	if platformFlag != "" {
		parts, err := parsePlatformList(platformFlag)
		if err != nil {
			return nil, apperr.Invalid(err.Error())
		}
		out := make([]platform.Platform, 0, len(parts))
		for _, p := range parts {
			parsed, err := platform.Parse(p)
			if err != nil {
				return nil, err
			}
			out = append(out, parsed)
		}
		return out, nil
	}
	if profileFlag == "" {
		return nil, nil
	}
	if err := app.openDB(); err != nil {
		return nil, err
	}
	view, err := profile.Show(cmd.Context(), app.DB, profileFlag)
	if err != nil {
		return nil, err
	}
	seen := map[platform.Platform]struct{}{}
	var out []platform.Platform
	for _, d := range view.Destinations {
		if _, ok := seen[d.Platform]; ok {
			continue
		}
		seen[d.Platform] = struct{}{}
		out = append(out, d.Platform)
	}
	return out, nil
}

func checkRows(rep media.Report) [][]string {
	rows := make([][]string, 0, len(rep.Checks)+1)
	rows = append(rows, []string{"hash", "true", rep.Info.Hash})
	for _, c := range rep.Checks {
		rows = append(rows, []string{c.Name, fmt.Sprintf("%v", c.OK), c.Message})
	}
	return rows
}

func jsonError(err error) map[string]any {
	if e, ok := apperr.As(err); ok {
		return map[string]any{"code": e.Code, "message": e.Message, "retryable": e.Retryable}
	}
	return map[string]any{"code": "INTERNAL", "message": err.Error(), "retryable": false}
}
