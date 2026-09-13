package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(path, []byte("goggles"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Fatalf("hash length %d", len(h))
	}
}

func TestMissingFile(t *testing.T) {
	_, err := Probe(context.Background(), filepath.Join(t.TempDir(), "nope.mp4"))
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.FileNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestProbeGeneratedReel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	path := filepath.Join(t.TempDir(), "reel.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1080x1920:d=3:r=30",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-t", "3", "-y", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	rep, err := CheckFile(context.Background(), path, []platform.Platform{platform.Instagram, platform.YouTube})
	if err != nil {
		t.Fatalf("check: %v checks=%v", err, rep.Checks)
	}
	if !rep.OK {
		t.Fatalf("expected ok, checks=%v", rep.Checks)
	}
	if rep.Info.Width != 1080 || rep.Info.Height != 1920 {
		t.Fatalf("dims %dx%d", rep.Info.Width, rep.Info.Height)
	}
}

func TestListCarouselFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"10.jpg", "02.jpg", "notes.yaml", "song.mp3", ".hidden.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListCarouselFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "02.jpg" || filepath.Base(got[1]) != "10.jpg" {
		t.Fatalf("got %v", got)
	}
}

func TestHashCarouselIncludesAudio(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	song := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(a, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(song, []byte("song"), 0o600); err != nil {
		t.Fatal(err)
	}
	h1, err := HashCarousel([]string{a, b}, "", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashCarousel([]string{a, b}, song, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("audio should change hash")
	}
}

func TestProbeImage(t *testing.T) {
	requireFFmpegProbe(t)
	path := filepath.Join(t.TempDir(), "slide.jpg")
	writeJPEG(t, path)
	info, err := Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != KindImage {
		t.Fatalf("kind %s", info.Kind)
	}
	if info.Width < 1 || info.Height < 1 {
		t.Fatalf("dims %dx%d", info.Width, info.Height)
	}
}

func TestPrepareBakesAudioOntoSlides(t *testing.T) {
	requireFFmpegProbe(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "01.jpg")
	b := filepath.Join(dir, "02.jpg")
	song := filepath.Join(dir, "song.m4a")
	writeJPEG(t, a)
	writeJPEG(t, b)
	writeAudio(t, song, 8)
	items, err := Collect(context.Background(), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := Prepare(context.Background(), items, PrepareOpts{
		Audio:        song,
		SlideSeconds: 3,
		WorkDir:      work,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len %d", len(out))
	}
	for i, it := range out {
		if it.Kind != KindVideo || !it.Info.HasAudio {
			t.Fatalf("item %d kind=%s audio=%v", i, it.Kind, it.Info.HasAudio)
		}
		if it.Info.DurationS < 2.5 || it.Info.DurationS > 3.5 {
			t.Fatalf("item %d duration %f", i, it.Info.DurationS)
		}
	}
}

func requireFFmpegProbe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
}

func writeJPEG(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=red:s=1080x1350", "-frames:v", "1", "-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jpeg: %v\n%s", err, out)
	}
}

func writeAudio(t *testing.T, path string, seconds int) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%d", seconds),
		"-c:a", "aac", "-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("audio: %v\n%s", err, out)
	}
}
