package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestBackoff(t *testing.T) {
	if Backoff(1) != time.Minute || Backoff(5) != 60*time.Minute {
		t.Fatalf("backoff %v %v", Backoff(1), Backoff(5))
	}
}

func TestScheduleRetryAndListDue(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := CreateBatch(ctx, sqlDB, "/tmp/a.mp4", "abc")
	if err != nil {
		t.Fatal(err)
	}
	job, err := CreateJob(ctx, sqlDB, batch.ID, pair.Destination.ID, platform.YouTube)
	if err != nil {
		t.Fatal(err)
	}
	fail := apperr.New(apperr.YouTubeRateLimited, "slow down")
	fail.Retryable = true
	dueAt := time.Now().UTC().Add(-time.Minute)
	if err := ScheduleRetry(ctx, sqlDB, job.ID, fail, dueAt, "", ""); err != nil {
		t.Fatal(err)
	}
	due, err := ListDueRetries(ctx, sqlDB, time.Now().UTC())
	if err != nil || len(due) != 1 || due[0].ID != job.ID || due[0].Status != RetryWait {
		t.Fatalf("due %+v %v", due, err)
	}
	if due[0].AttemptCount != 1 {
		t.Fatalf("attempts %d", due[0].AttemptCount)
	}
}

func TestProviderState(t *testing.T) {
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.YouTube, Alias: "yt", ChannelID: "UCabc", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := CreateBatch(ctx, sqlDB, "/tmp/a.mp4", "abc")
	if err != nil {
		t.Fatal(err)
	}
	job, err := CreateJob(ctx, sqlDB, batch.ID, pair.Destination.ID, platform.YouTube)
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeProviderState(ctx, sqlDB, job.ID, map[string]any{"youtube_upload_session": "https://up"}); err != nil {
		t.Fatal(err)
	}
	st, err := ProviderState(ctx, sqlDB, job.ID)
	if err != nil || st["youtube_upload_session"] != "https://up" {
		t.Fatalf("%v %v", st, err)
	}
}
