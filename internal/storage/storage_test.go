package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/db"
)

func TestJoinPublicURL(t *testing.T) {
	got, err := JoinPublicURL("https://media.example.com/reels/", "goggles/tmp/a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://media.example.com/reels/goggles/tmp/a.mp4" {
		t.Fatalf("got %s", got)
	}
	if _, err := JoinPublicURL("", "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestObjectKey(t *testing.T) {
	k := ObjectKey("clip.mp4")
	if !strings.HasPrefix(k, "goggles/tmp/") || !strings.HasSuffix(k, ".mp4") {
		t.Fatalf("key %s", k)
	}
}

func TestMemoryUploadDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewMemory("https://r2.test", time.Hour)
	hosted, err := m.Upload(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Has(hosted.ObjectKey) {
		t.Fatal("missing object")
	}
	if !strings.HasPrefix(hosted.PublicURL, "https://r2.test/") {
		t.Fatalf("url %s", hosted.PublicURL)
	}
	if err := m.Delete(context.Background(), hosted.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if m.Has(hosted.ObjectKey) {
		t.Fatal("expected delete")
	}
}

func TestCleanupExpired(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	host := NewMemory("https://r2.test", time.Hour)
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	hosted, err := host.Upload(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	hosted.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	if _, err := RecordHosted(ctx, sqlDB, hosted, ""); err != nil {
		t.Fatal(err)
	}
	res, err := Cleanup(ctx, sqlDB, host, time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.Failed != 0 {
		t.Fatalf("cleanup %+v", res)
	}
	if host.Has(hosted.ObjectKey) {
		t.Fatal("object still present")
	}
	n, err := CountPending(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pending %d", n)
	}
}
