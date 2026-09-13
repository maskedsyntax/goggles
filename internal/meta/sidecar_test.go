package meta

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestOverlayPrecedence(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "001.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	yaml := []byte(`
instagram:
  caption: platform caption
youtube:
  title: platform title
  tags: [ocd]
destinations:
  patterns-youtube-clips:
    youtube:
      title: dest title
`)
	if err := os.WriteFile(filepath.Join(dir, "001.yaml"), yaml, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := LoadFor(video)
	if err != nil {
		t.Fatal(err)
	}
	dest, plat := file.For("patterns-youtube-clips", platform.YouTube)
	if dest.Title != "dest title" {
		t.Fatalf("dest override: %s", dest.Title)
	}
	if plat.Title != "platform title" || len(plat.Tags) != 1 {
		t.Fatalf("platform: %+v", plat)
	}
	_, plat2 := file.For("other", platform.YouTube)
	if plat2.Title != "platform title" {
		t.Fatalf("platform other: %s", plat2.Title)
	}
}

func TestLoadCarouselSidecar(t *testing.T) {
	dir := t.TempDir()
	slides := filepath.Join(dir, "slides")
	if err := os.Mkdir(slides, 0o700); err != nil {
		t.Fatal(err)
	}
	yaml := []byte(`
instagram:
  caption: swipe
  audio: song.mp3
  slide_seconds: 4
  carousel:
    - 01.jpg
    - 02.jpg
`)
	if err := os.WriteFile(filepath.Join(slides, "carousel.yaml"), yaml, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := LoadFor(slides)
	if err != nil {
		t.Fatal(err)
	}
	if file.Dir != slides {
		t.Fatalf("dir %s", file.Dir)
	}
	if file.Instagram.Caption != "swipe" || file.Instagram.Audio != "song.mp3" {
		t.Fatalf("%+v", file.Instagram)
	}
	if file.Instagram.SlideSeconds == nil || *file.Instagram.SlideSeconds != 4 {
		t.Fatalf("slide %+v", file.Instagram.SlideSeconds)
	}
	if len(file.Instagram.Carousel) != 2 {
		t.Fatalf("carousel %v", file.Instagram.Carousel)
	}
}
