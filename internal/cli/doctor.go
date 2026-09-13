package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/daemon"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/storage"
	"github.com/maskedsyntax/goggles/internal/storage/r2"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local prerequisites",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := []doctorCheck{
				checkPath("config", true, app.Paths.ConfigFile, "config file"),
				app.checkSQLite(),
				app.checkWritable("data_dir", true, app.Paths.DataDir),
				app.checkWritable("log_dir", true, app.Paths.LogDir),
				app.checkKeychain(),
				checkBinary("ffprobe", true),
				checkBinary("ffmpeg", false),
				app.checkR2(cmd.Context()),
				{Name: "meta", OK: app.Config.Meta.AppID != "", Required: false, Message: emptyOr("Meta app id is not configured", app.Config.Meta.AppID != "", "Meta app id is set")},
				{Name: "google", OK: app.Config.Google.ClientID != "", Required: false, Message: emptyOr("Google OAuth client id is not configured", app.Config.Google.ClientID != "", "Google client id is set")},
				app.checkDestinations(),
				app.checkAuth(),
				app.checkStaleUploads(),
				app.checkDaemon(),
			}

			ok := true
			rows := make([][]string, 0, len(checks))
			for _, c := range checks {
				if c.Required && !c.OK {
					ok = false
				}
				status := "ok"
				if !c.OK && c.Required {
					status = "FAIL"
				} else if !c.OK {
					status = "warn"
				}
				rows = append(rows, []string{c.Name, status, c.Message})
			}

			if app.JSON {
				payload := map[string]any{"success": ok, "checks": checks}
				if err := app.Out.Result(payload); err != nil {
					return err
				}
				if !ok {
					return apperr.Printed(apperr.New(apperr.ConfigMissing, "doctor found required failures"))
				}
				return nil
			}
			if err := app.Out.Table([]string{"CHECK", "STATUS", "MESSAGE"}, rows); err != nil {
				return err
			}
			if !ok {
				return apperr.New(apperr.ConfigMissing, "doctor found required failures")
			}
			return nil
		},
	}
}

func checkPath(name string, required bool, path, label string) doctorCheck {
	_, err := os.Stat(path)
	if err != nil {
		return doctorCheck{Name: name, OK: false, Required: required, Message: label + " missing: " + path}
	}
	return doctorCheck{Name: name, OK: true, Required: required, Message: path}
}

func (a *App) checkWritable(name string, required bool, dir string) doctorCheck {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return doctorCheck{Name: name, OK: false, Required: required, Message: err.Error()}
	}
	f := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(f, []byte("ok"), 0o600); err != nil {
		return doctorCheck{Name: name, OK: false, Required: required, Message: err.Error()}
	}
	_ = os.Remove(f)
	return doctorCheck{Name: name, OK: true, Required: required, Message: dir}
}

func (a *App) checkSQLite() doctorCheck {
	sqlDB, err := db.Open(a.Paths.DBFile)
	if err != nil {
		return doctorCheck{Name: "sqlite", OK: false, Required: true, Message: err.Error()}
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		return doctorCheck{Name: "sqlite", OK: false, Required: true, Message: err.Error()}
	}
	return doctorCheck{Name: "sqlite", OK: true, Required: true, Message: a.Paths.DBFile}
}

func (a *App) checkKeychain() doctorCheck {
	const probe = "goggles-doctor-probe"
	if err := a.Keychain.Set(probe, "ok"); err != nil {
		return doctorCheck{Name: "keychain", OK: false, Required: false, Message: err.Error()}
	}
	if _, err := a.Keychain.Get(probe); err != nil {
		return doctorCheck{Name: "keychain", OK: false, Required: false, Message: err.Error()}
	}
	_ = a.Keychain.Delete(probe)
	return doctorCheck{Name: "keychain", OK: true, Required: false, Message: a.Keychain.Name() + " backend"}
}

func (a *App) checkDestinations() doctorCheck {
	sqlDB, err := db.Open(a.Paths.DBFile)
	if err != nil {
		return doctorCheck{Name: "destinations", OK: false, Required: false, Message: err.Error()}
	}
	defer sqlDB.Close()
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM destinations`).Scan(&n); err != nil {
		return doctorCheck{Name: "destinations", OK: false, Required: false, Message: err.Error()}
	}
	if n == 0 {
		return doctorCheck{Name: "destinations", OK: false, Required: false, Message: "no destinations configured"}
	}
	return doctorCheck{Name: "destinations", OK: true, Required: false, Message: "configured"}
}

func checkBinary(name string, required bool) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: name, OK: false, Required: required, Message: name + " not found on PATH"}
	}
	return doctorCheck{Name: name, OK: true, Required: required, Message: path}
}

func (a *App) checkR2(ctx context.Context) doctorCheck {
	if os.Getenv("GOGGLES_STORAGE") == "memory" {
		return doctorCheck{Name: "r2", OK: true, Required: false, Message: "memory backend"}
	}
	if !r2.Configured(a.Config) {
		return doctorCheck{Name: "r2", OK: false, Required: false, Message: "R2 is not configured"}
	}
	if !r2.CredentialsPresent(a.Config.R2, a.Keychain) {
		return doctorCheck{Name: "r2", OK: false, Required: false, Message: "R2 credentials missing"}
	}
	host, err := a.openHost(ctx)
	if err != nil {
		return doctorCheck{Name: "r2", OK: false, Required: false, Message: err.Error()}
	}
	if p, ok := host.(storage.Pinger); ok {
		if err := p.Ping(ctx); err != nil {
			return doctorCheck{Name: "r2", OK: false, Required: false, Message: err.Error()}
		}
	}
	return doctorCheck{Name: "r2", OK: true, Required: false, Message: "R2 reachable"}
}

func emptyOr(empty string, ok bool, good string) string {
	if ok {
		return good
	}
	return empty
}

func (a *App) checkDaemon() doctorCheck {
	st, err := daemon.LaunchAgentStatus()
	if err != nil {
		return doctorCheck{Name: "daemon", OK: false, Required: false, Message: err.Error()}
	}
	installed, _ := st["installed"].(bool)
	loaded, _ := st["loaded"].(bool)
	if loaded {
		return doctorCheck{Name: "daemon", OK: true, Required: false, Message: "LaunchAgent loaded"}
	}
	if installed {
		return doctorCheck{Name: "daemon", OK: false, Required: false, Message: "plist installed but not loaded"}
	}
	return doctorCheck{Name: "daemon", OK: false, Required: false, Message: "LaunchAgent not installed"}
}

func (a *App) checkStaleUploads() doctorCheck {
	sqlDB, err := db.Open(a.Paths.DBFile)
	if err != nil {
		return doctorCheck{Name: "r2_cleanup", OK: false, Required: false, Message: err.Error()}
	}
	defer sqlDB.Close()
	n, err := storage.CountPending(context.Background(), sqlDB)
	if err != nil {
		return doctorCheck{Name: "r2_cleanup", OK: false, Required: false, Message: err.Error()}
	}
	if n > 0 {
		return doctorCheck{Name: "r2_cleanup", OK: false, Required: false, Message: "pending temporary objects"}
	}
	return doctorCheck{Name: "r2_cleanup", OK: true, Required: false, Message: "no pending R2 objects"}
}

func (a *App) checkAuth() doctorCheck {
	sqlDB, err := db.Open(a.Paths.DBFile)
	if err != nil {
		return doctorCheck{Name: "auth", OK: false, Required: false, Message: err.Error()}
	}
	defer sqlDB.Close()
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM accounts WHERE credential_ref IS NOT NULL AND credential_ref != ''`).Scan(&n); err != nil {
		return doctorCheck{Name: "auth", OK: false, Required: false, Message: err.Error()}
	}
	if n == 0 {
		return doctorCheck{Name: "auth", OK: false, Required: false, Message: "no stored platform tokens"}
	}
	return doctorCheck{Name: "auth", OK: true, Required: false, Message: "tokens stored"}
}
