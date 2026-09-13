package cli

import (
	"database/sql"
	"io"
	"log/slog"

	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/output"
)

const version = "0.1.0-dev"

type App struct {
	JSON           bool
	DryRun         bool
	Verbose        bool
	Quiet          bool
	NonInteractive bool
	ConfigPath     string

	Paths    config.Paths
	Config   config.File
	DB       *sql.DB
	Keychain keychain.Store
	Out      *output.Printer
	Log      *slog.Logger
	Stdout   io.Writer
	Stderr   io.Writer
}

func (a *App) Close() {
	if a.DB != nil {
		_ = a.DB.Close()
	}
}
