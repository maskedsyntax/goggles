package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/queue"
	"github.com/maskedsyntax/goggles/internal/schedule"
)

func TestTickPublishesDueSlotOnce(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if _, err := account.Add(ctx, sqlDB, account.AddInput{Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Timezone: "UTC", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := schedule.Set(ctx, sqlDB, "yt", []string{"12:00"}, "UTC"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, []byte("mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Add(ctx, sqlDB, queue.AddInput{Path: path, Destination: "yt"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 12, 5, 0, 0, time.UTC)
	var published []string
	eng := &Engine{
		DB:     sqlDB,
		Now:    func() time.Time { return now },
		Window: 15 * time.Minute,
		Publish: func(_ context.Context, dest string, item queue.Item) error {
			published = append(published, dest+"|"+item.FilePath)
			return nil
		},
	}
	res, err := eng.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fired != 1 || len(published) != 1 {
		t.Fatalf("tick %+v published=%v", res, published)
	}
	res, err = eng.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fired != 0 || len(published) != 1 {
		t.Fatalf("second tick %+v published=%v", res, published)
	}
}
