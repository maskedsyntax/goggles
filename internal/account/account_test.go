package account

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestAddYouTubeRequiresChannel(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	_, err = Add(context.Background(), sqlDB, AddInput{Platform: platform.YouTube, Alias: "yt", Enabled: true})
	if err == nil {
		t.Fatal("expected channel required")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.YouTubeChannelRequired {
		t.Fatalf("got %v", err)
	}
}

func TestDuplicateAlias(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := Add(ctx, sqlDB, AddInput{Platform: platform.Instagram, Alias: "patterns-instagram", Username: "patterns_app", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, err = Add(ctx, sqlDB, AddInput{Platform: platform.Instagram, Alias: "patterns-instagram", Enabled: true})
	if err == nil {
		t.Fatal("expected duplicate alias")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.AlreadyExists {
		t.Fatalf("got %v", err)
	}
}

func TestDailyLimit(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := Add(ctx, sqlDB, AddInput{Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := SetDailyLimit(ctx, sqlDB, "yt", 5); err != nil {
		t.Fatal(err)
	}
	d, err := GetDestinationByAlias(ctx, sqlDB, "yt")
	if err != nil {
		t.Fatal(err)
	}
	if DailyLimit(d) != 5 {
		t.Fatalf("limit %d", DailyLimit(d))
	}
}
