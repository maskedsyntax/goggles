package cli

import (
	"github.com/maskedsyntax/goggles/internal/config"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Show or change local configuration"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the config file path",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if app.JSON {
					return app.Out.Success(map[string]any{"path": app.Paths.ConfigFile})
				}
				return app.Out.Result(app.Paths.ConfigFile)
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Show the current config",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if app.JSON {
					return app.Out.Success(map[string]any{
						"path":   app.Paths.ConfigFile,
						"config": app.Config,
					})
				}
				data, err := toml.Marshal(app.Config)
				if err != nil {
					return err
				}
				_, err = cmd.OutOrStdout().Write(data)
				return err
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a config key",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				if app.DryRun {
					return app.Out.Success(map[string]any{
						"dry_run": true,
						"key":     args[0],
						"value":   args[1],
					})
				}
				cfg := app.Config
				if err := cfg.Set(args[0], args[1]); err != nil {
					return err
				}
				if err := config.Write(app.Paths.ConfigFile, cfg); err != nil {
					return err
				}
				app.Config = cfg
				return app.Out.Success(map[string]any{"key": args[0], "value": args[1]})
			},
		},
	)
	return cmd
}
