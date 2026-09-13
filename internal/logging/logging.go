package logging

import (
	"io"
	"log/slog"
	"os"
)

func New(verbose, quiet bool, stderr io.Writer) *slog.Logger {
	if stderr == nil {
		stderr = os.Stderr
	}
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	if quiet {
		level = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
}
