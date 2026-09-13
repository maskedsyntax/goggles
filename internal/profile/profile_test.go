package profile

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestProfileFanOut(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.Instagram, Alias: "patterns-instagram", Username: "patterns_app", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.YouTube, Alias: "patterns-youtube-main", ChannelID: "UCabc123", ChannelTitle: "Patterns", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, sqlDB, "patterns"); err != nil {
		t.Fatal(err)
	}
	if err := AddDestination(ctx, sqlDB, "patterns", "patterns-instagram"); err != nil {
		t.Fatal(err)
	}
	if err := AddDestination(ctx, sqlDB, "patterns", "patterns-youtube-main"); err != nil {
		t.Fatal(err)
	}
	view, err := Show(ctx, sqlDB, "patterns")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Destinations) != 2 {
		t.Fatalf("destinations = %d", len(view.Destinations))
	}
	igyt := DestinationsForPlatforms(view, []platform.Platform{platform.Instagram})
	if len(igyt) != 1 {
		t.Fatalf("instagram filter = %d", len(igyt))
	}
}
