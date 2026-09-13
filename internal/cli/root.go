package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/logging"
	"github.com/maskedsyntax/goggles/internal/output"
	"github.com/spf13/cobra"
)

func Execute(stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	app := &App{Stdout: stdout, Stderr: stderr}
	root := newRoot(app)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		var printed *apperr.PrintedError
		if errors.As(err, &printed) {
			return apperr.ExitCode(printed.Err)
		}
		if app.Out == nil {
			app.Out = output.New(app.JSON, stdout, stderr)
		}
		app.Out.PrintError(err)
		if isUsageError(err) {
			return apperr.ExitUsage
		}
		return apperr.ExitCode(err)
	}
	return apperr.ExitOK
}

func newRoot(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "goggles",
		Short:         "Publish short-form video to Instagram Reels and YouTube Shorts",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return app.setup(cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			app.Close()
		},
	}
	cmd.PersistentFlags().BoolVar(&app.JSON, "json", false, "machine-readable JSON on stdout")
	cmd.PersistentFlags().BoolVar(&app.DryRun, "dry-run", false, "show what would happen without making changes")
	cmd.PersistentFlags().BoolVarP(&app.Verbose, "verbose", "v", false, "debug logs on stderr")
	cmd.PersistentFlags().BoolVarP(&app.Quiet, "quiet", "q", false, "only errors on stderr")
	cmd.PersistentFlags().BoolVar(&app.NonInteractive, "non-interactive", false, "never prompt")
	cmd.PersistentFlags().StringVar(&app.ConfigPath, "config", "", "path to config.toml")

	cmd.AddCommand(
		newAuthCmd(app),
		newAccountCmd(app),
		newProfileCmd(app),
		newMediaCmd(app),
		newPublishCmd(app),
		newQueueCmd(app),
		newScheduleCmd(app),
		newJobCmd(app),
		newHistoryCmd(app),
		newDestinationCmd(app),
		newStorageCmd(app),
		newDaemonCmd(app),
		newConfigCmd(app),
		newDoctorCmd(app),
	)
	return cmd
}

func (a *App) setup(cmd *cobra.Command) error {
	if cmd.Name() == "help" || cmd.Name() == "completion" {
		return nil
	}
	a.Out = output.New(a.JSON, a.Stdout, a.Stderr)
	a.Log = logging.New(a.Verbose, a.Quiet, a.Stderr)
	a.Keychain = keychain.Open()

	paths, err := config.ResolvePaths(a.ConfigPath)
	if err != nil {
		return err
	}
	a.Paths = paths
	cfg, err := config.Ensure(paths.ConfigFile)
	if err != nil {
		return err
	}
	a.Config = cfg
	if err := config.EnsureDataDirs(paths); err != nil {
		return err
	}
	return nil
}

func (a *App) openDB() error {
	if a.DB != nil {
		return nil
	}
	sqlDB, err := db.Open(a.Paths.DBFile)
	if err != nil {
		return err
	}
	a.DB = sqlDB
	return nil
}

func isUsageError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "required flag") ||
		strings.Contains(msg, "accepts") ||
		strings.Contains(msg, "arg") && strings.Contains(msg, "received")
}

func parsePlatformList(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty platform list")
	}
	return out, nil
}
