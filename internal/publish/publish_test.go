package publish

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/platform/instagram"
	"github.com/maskedsyntax/goggles/internal/platform/youtube"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type stubAPI struct {
	media string
}

func (s stubAPI) CreateReel(context.Context, string, string, string, bool) (string, error) {
	return "c1", nil
}
func (s stubAPI) CreateCarouselItem(context.Context, string, string, string) (string, error) {
	return "child1", nil
}
func (s stubAPI) CreateCarousel(context.Context, string, []string, string, bool) (string, error) {
	return "carousel1", nil
}
func (s stubAPI) ContainerStatus(context.Context, string) (string, error) { return "FINISHED", nil }
func (s stubAPI) PublishContainer(context.Context, string, string) (string, error) {
	return s.media, nil
}
func (s stubAPI) Me(context.Context) (string, string, error) { return "1", "u", nil }

func TestPublishDestinationAndDuplicate(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.Instagram, Alias: "patterns-instagram", Username: "patterns_app", UserID: "1784140000", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	kc := keychain.NewMemory()
	if err := kc.Set("instagram/"+pair.Account.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reel.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1080x1920:d=3:r=30",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "3", "-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	runner := &Runner{
		DB:        sqlDB,
		Host:      storage.NewMemory("https://r2.test", time.Hour),
		Keychain:  kc,
		GraphBase: "http://example.invalid",
		NewIGClient: func(string) instagram.API {
			return stubAPI{media: "igmedia"}
		},
	}
	res, err := runner.Run(ctx, Request{Path: path, Destination: "patterns-instagram"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || len(res.Jobs) != 1 || res.Jobs[0].ExternalMediaID != "igmedia" {
		t.Fatalf("%+v", res)
	}
	res2, err := runner.Run(ctx, Request{Path: path, Destination: "patterns-instagram"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Success || res2.Jobs[0].ErrorCode != "DUPLICATE_CONTENT" {
		t.Fatalf("dup %+v", res2)
	}
}

type ytStub struct{}

func (ytStub) ListChannels(context.Context) ([]youtube.Channel, error) {
	return []youtube.Channel{{ID: "UCabc", Title: "Patterns"}}, nil
}
func (ytStub) Upload(context.Context, string, youtube.VideoMeta) (string, error) {
	return "ytvid", nil
}
func (ytStub) VideoStatus(context.Context, string) (string, error) { return "processed", nil }

func TestPublishYouTube(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.YouTube, Alias: "patterns-youtube-main", ChannelID: "UCabc", ChannelTitle: "Patterns", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	kc := keychain.NewMemory()
	if err := kc.Set("youtube/"+pair.Account.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reel.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1080x1920:d=3:r=30",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "3", "-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	runner := &Runner{
		DB: sqlDB, Host: storage.NewMemory("https://r2.test", time.Hour), Keychain: kc,
		NewYTClient: func(string) youtube.API { return ytStub{} },
	}
	res, err := runner.Run(ctx, Request{Path: path, Destination: "patterns-youtube-main", Title: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Jobs[0].ExternalMediaID != "ytvid" {
		t.Fatalf("%+v", res)
	}
}

func TestPublishCarousel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.Instagram, Alias: "patterns-instagram", Username: "patterns_app", UserID: "1784140000", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	kc := keychain.NewMemory()
	if err := kc.Set("instagram/"+pair.Account.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "01.jpg")
	b := filepath.Join(dir, "02.jpg")
	for _, path := range []string{a, b} {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=blue:s=1080x1350", "-frames:v", "1", "-y", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jpeg: %v\n%s", err, out)
		}
	}
	runner := &Runner{
		DB: sqlDB, Host: storage.NewMemory("https://r2.test", time.Hour), Keychain: kc,
		NewIGClient: func(string) instagram.API { return stubAPI{media: "carousel-media"} },
	}
	res, err := runner.Run(ctx, Request{Path: dir, Destination: "patterns-instagram", Caption: "swipe"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Kind != "carousel" || res.Jobs[0].ExternalMediaID != "carousel-media" {
		t.Fatalf("%+v", res)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items %v", res.Items)
	}
}

func TestPublishCarouselRejectedOnYouTube(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.YouTube, Alias: "patterns-youtube-main", ChannelID: "UCabc", ChannelTitle: "Patterns", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	kc := keychain.NewMemory()
	if err := kc.Set("youtube/"+pair.Account.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"01.jpg", "02.jpg"} {
		path := filepath.Join(dir, name)
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=blue:s=1080x1350", "-frames:v", "1", "-y", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jpeg: %v\n%s", err, out)
		}
	}
	runner := &Runner{
		DB: sqlDB, Host: storage.NewMemory("https://r2.test", time.Hour), Keychain: kc,
		NewYTClient: func(string) youtube.API { return ytStub{} },
	}
	res, err := runner.Run(ctx, Request{Path: dir, Destination: "patterns-youtube-main"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || res.Jobs[0].ErrorCode != "INVALID_INPUT" {
		t.Fatalf("%+v", res)
	}
}

type failAPI struct{ stubAPI }

func (failAPI) CreateReel(context.Context, string, string, string, bool) (string, error) {
	e := apperr.New(apperr.MetaRateLimited, "slow down")
	e.Retryable = true
	return "", e
}

func TestAutoRetrySchedulesBackoff(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "goggles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	pair, err := account.Add(ctx, sqlDB, account.AddInput{
		Platform: platform.Instagram, Alias: "patterns-instagram", Username: "patterns_app", UserID: "1784140000", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	kc := keychain.NewMemory()
	if err := kc.Set("instagram/"+pair.Account.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reel.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1080x1920:d=3:r=30",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", "3", "-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	runner := &Runner{
		DB: sqlDB, Host: storage.NewMemory("https://r2.test", time.Hour), Keychain: kc,
		AutoRetry:   true,
		NewIGClient: func(string) instagram.API { return failAPI{} },
	}
	res, err := runner.Run(ctx, Request{Path: path, Destination: "patterns-instagram"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || len(res.Jobs) != 1 || res.Jobs[0].Status != "retry_wait" {
		t.Fatalf("%+v", res)
	}
}
