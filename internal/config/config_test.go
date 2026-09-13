package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg, err := Ensure(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.GraphVersion != "v25.0" {
		t.Fatalf("graph version: %s", cfg.Meta.GraphVersion)
	}
	if err := cfg.Set("general.timezone", "Asia/Kolkata"); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.General.Timezone != "Asia/Kolkata" {
		t.Fatalf("timezone: %s", loaded.General.Timezone)
	}
}

func TestResolvePathsGOGGLES_HOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	p, err := ResolvePaths("")
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigFile != filepath.Join(dir, "config.toml") {
		t.Fatalf("config path: %s", p.ConfigFile)
	}
	if p.DBFile != filepath.Join(dir, "goggles.db") {
		t.Fatalf("db path: %s", p.DBFile)
	}
}

func TestSetUnknownKey(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("nope", "1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	if _, err := Ensure(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
