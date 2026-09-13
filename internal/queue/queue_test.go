package queue

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/profile"
)

func TestQueueAddListPause(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := profile.Create(ctx, sqlDB, "patterns"); err != nil {
		t.Fatal(err)
	}
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.Instagram, Alias: "ig", Username: "u", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.AddDestination(ctx, sqlDB, "patterns", "ig"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, []byte("mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, err := Add(ctx, sqlDB, AddInput{Path: path, Profile: "patterns"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != Ready || item.Position != 1 {
		t.Fatalf("%+v", item)
	}
	items, err := List(ctx, sqlDB, ListFilter{Profile: "patterns"})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %v", items, err)
	}
	if err := SetPaused(ctx, sqlDB, "patterns", "", true); err != nil {
		t.Fatal(err)
	}
	paused, err := Paused(ctx, sqlDB, "patterns", "")
	if err != nil || !paused {
		t.Fatalf("paused %v %v", paused, err)
	}
	if err := Remove(ctx, sqlDB, item.ID); err != nil {
		t.Fatal(err)
	}
}

func TestQueueFill(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"b.mp4", "a.mp4", "skip.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := Fill(ctx, sqlDB, dir, AddInput{Destination: "yt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Position != 1 || items[1].Position != 2 {
		t.Fatalf("%+v", items)
	}
	n, err := Clear(ctx, sqlDB, ListFilter{Destination: "yt"})
	if err != nil || n != 2 {
		t.Fatalf("clear %d %v", n, err)
	}
}
