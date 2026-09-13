package instagram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type fakeAPI struct {
	container    string
	status       string
	media        string
	created      string
	published    string
	items        []string
	carouselKids []string
}

func (f *fakeAPI) CreateReel(_ context.Context, _, videoURL, _ string, _ bool) (string, error) {
	f.created = videoURL
	return f.container, nil
}
func (f *fakeAPI) CreateCarouselItem(_ context.Context, _, imageURL, videoURL string) (string, error) {
	u := imageURL
	if videoURL != "" {
		u = videoURL
	}
	f.items = append(f.items, u)
	return fmt.Sprintf("child%d", len(f.items)), nil
}
func (f *fakeAPI) CreateCarousel(_ context.Context, _ string, children []string, _ string, _ bool) (string, error) {
	f.carouselKids = children
	return "carousel1", nil
}
func (f *fakeAPI) ContainerStatus(context.Context, string) (string, error) { return f.status, nil }
func (f *fakeAPI) PublishContainer(context.Context, string, string) (string, error) {
	if f.published == "" {
		if len(f.carouselKids) > 0 {
			f.published = "carousel1"
		} else {
			f.published = f.container
		}
	}
	return f.media, nil
}
func (f *fakeAPI) Me(context.Context) (string, string, error) { return "1", "u", nil }

func TestPublisherUploadPublishCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, []byte("mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := storage.NewMemory("https://r2.test", time.Hour)
	api := &fakeAPI{container: "c1", status: "FINISHED", media: "m1"}
	p := New(api, host)
	p.SkipValidate = true
	p.PollEvery = time.Millisecond
	res, err := p.Publish(context.Background(), platform.Destination{ExternalID: "1784", Alias: "ig"}, platform.Media{Path: path}, platform.PlatformMetadata{"caption": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExternalMediaID != "m1" {
		t.Fatalf("media %s", res.ExternalMediaID)
	}
	if !strings.HasPrefix(api.created, "https://r2.test/") {
		t.Fatalf("video url %s", api.created)
	}
	key := strings.TrimPrefix(api.created, "https://r2.test/")
	if host.Has(key) {
		t.Fatal("expected r2 cleanup")
	}
}

func TestPublisherCarouselUploadPublishCleanup(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(a, []byte("jpeg-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("jpeg-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := storage.NewMemory("https://r2.test", time.Hour)
	api := &fakeAPI{status: "FINISHED", media: "m-carousel"}
	p := New(api, host)
	p.SkipValidate = true
	p.PollEvery = time.Millisecond
	m := platform.Media{
		Path: a,
		Kind: "image",
		Items: []platform.Media{
			{Path: a, Kind: "image"},
			{Path: b, Kind: "image"},
		},
	}
	res, err := p.Publish(context.Background(), platform.Destination{ExternalID: "1784", Alias: "ig"}, m, platform.PlatformMetadata{"caption": "swipe"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExternalMediaID != "m-carousel" {
		t.Fatalf("media %s", res.ExternalMediaID)
	}
	if len(api.items) != 2 || len(api.carouselKids) != 2 {
		t.Fatalf("items=%v children=%v", api.items, api.carouselKids)
	}
	for _, url := range api.items {
		key := strings.TrimPrefix(url, "https://r2.test/")
		if host.Has(key) {
			t.Fatalf("expected cleanup of %s", key)
		}
	}
}
