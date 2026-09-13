package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maskedsyntax/goggles/internal/daemon"
	"github.com/spf13/cobra"
)

func newDaemonCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "daemon", Short: "Background scheduler"}
	var once bool
	var interval time.Duration
	run := &cobra.Command{
		Use:   "run",
		Short: "Run the scheduler loop in the foreground",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.openDB(); err != nil {
				return err
			}
			host, err := app.openHost(cmd.Context())
			if err != nil {
				return err
			}
			runner := app.runner(host)
			runner.AutoRetry = true
			eng := &daemon.Engine{
				DB:       app.DB,
				Log:      app.Log,
				Host:     host,
				Runner:   runner,
				Interval: interval,
			}
			if once {
				res, err := eng.Tick(cmd.Context())
				if err != nil {
					return err
				}
				return app.Out.Success(map[string]any{"tick": res})
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if app.Log != nil {
				app.Log.Info("goggles daemon started")
			}
			err = eng.Run(ctx)
			if err == context.Canceled {
				return nil
			}
			return err
		},
	}
	run.Flags().BoolVar(&once, "once", false, "run a single tick and exit")
	run.Flags().DurationVar(&interval, "interval", daemon.DefaultInterval, "tick interval")

	cmd.AddCommand(run, newDaemonInstallCmd(app), newDaemonUninstallCmd(app), newDaemonStatusCmd(app), newDaemonLogsCmd(app))
	return cmd
}

func newDaemonInstallCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install a user LaunchAgent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			path, err := daemon.InstallLaunchAgent(exe, app.Paths.LogDir)
			if err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"plist": path})
		},
	}
}

func newDaemonUninstallCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the user LaunchAgent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := daemon.UninstallLaunchAgent(); err != nil {
				return err
			}
			return app.Out.Success(map[string]any{"removed": true})
		},
	}
}

func newDaemonStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show LaunchAgent status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := daemon.LaunchAgentStatus()
			if err != nil {
				return err
			}
			return app.Out.Success(st)
		},
	}
}

func newDaemonLogsCmd(app *App) *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print recent daemon logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := daemon.ReadLogs(app.Paths.LogDir, n)
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Out.Success(map[string]any{"log": out})
			}
			_, err = fmt.Fprint(app.Stdout, out)
			return err
		},
	}
	cmd.Flags().IntVar(&n, "lines", 50, "number of lines")
	return cmd
}
