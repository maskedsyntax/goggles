package schedule

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/profile"
)

func TestSetListClear(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.Instagram, Alias: "ig", Username: "u", Enabled: true, Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	slots, err := Set(ctx, sqlDB, "ig", []string{"9:00", "12:30", "09:00"}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 || slots[0].TimeOfDay != "09:00" || slots[1].TimeOfDay != "12:30" {
		t.Fatalf("%+v", slots)
	}
	if err := SetEnabled(ctx, sqlDB, "ig", false); err != nil {
		t.Fatal(err)
	}
	if err := Clear(ctx, sqlDB, "ig"); err != nil {
		t.Fatal(err)
	}
}

func TestSetProfile(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.Instagram, Alias: "ig", Username: "u", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := profile.Create(ctx, sqlDB, "patterns"); err != nil {
		t.Fatal(err)
	}
	if err := profile.AddDestination(ctx, sqlDB, "patterns", "ig"); err != nil {
		t.Fatal(err)
	}
	if err := profile.AddDestination(ctx, sqlDB, "patterns", "yt"); err != nil {
		t.Fatal(err)
	}
	out, err := SetProfile(ctx, sqlDB, "patterns", []string{"10:00"}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(out["ig"]) != 1 || len(out["yt"]) != 1 {
		t.Fatalf("%+v", out)
	}
}
