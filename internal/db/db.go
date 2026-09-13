package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/migrations"
	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot create database directory", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", filepath.ToSlash(path))
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot open sqlite", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = sqlDB.Close()
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot apply sqlite pragmas", err)
	}
	if err := Migrate(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return sqlDB, nil
}

func Migrate(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot create schema_migrations", err)
	}

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot read migrations", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version, err := parseVersion(name)
		if err != nil {
			return err
		}
		var existing int
		err = sqlDB.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&existing)
		if err != nil {
			return apperr.Wrap(apperr.DatabaseError, "cannot query schema_migrations", err)
		}
		if existing > 0 {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return apperr.Wrap(apperr.DatabaseError, "cannot read migration "+name, err)
		}
		tx, err := sqlDB.Begin()
		if err != nil {
			return apperr.Wrap(apperr.DatabaseError, "cannot begin migration transaction", err)
		}
		for _, stmt := range splitSQL(string(body)) {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return apperr.Wrap(apperr.DatabaseError, "cannot apply migration "+name, err)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			version, name, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return apperr.Wrap(apperr.DatabaseError, "cannot record migration "+name, err)
		}
		if err := tx.Commit(); err != nil {
			return apperr.Wrap(apperr.DatabaseError, "cannot commit migration "+name, err)
		}
	}
	return nil
}

func splitSQL(body string) []string {
	raw := strings.Split(body, ";")
	var out []string
	for _, stmt := range raw {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(filepath.Base(name), ".sql")
	parts := strings.SplitN(base, "_", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, apperr.New(apperr.DatabaseError, "invalid migration filename "+name)
	}
	return n, nil
}

func WithTx(ctx context.Context, sqlDB *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot begin transaction", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot commit transaction", err)
	}
	return nil
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
