package db

import (
	"path/filepath"
	"testing"
)

func TestOpenMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goggles.db")
	sqlDB, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM platforms`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("platforms = %d", n)
	}

	sqlDB2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB2.Close()
	if err := sqlDB2.QueryRow(`SELECT COUNT(1) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("migrations = %d", n)
	}
}
