package media

import (
	"context"
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
