package instagram

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type fakeAPI struct {
	container string
	status    string
	media     string
	created   string
	published string
}

func (f *fakeAPI) CreateReel(_ context.Context, _, videoURL, _ string, _ bool) (string, error) {
	f.created = videoURL
	return f.container, nil
}
func (f *fakeAPI) ContainerStatus(context.Context, string) (string, error) { return f.status, nil }
func (f *fakeAPI) PublishContainer(context.Context, string, string) (string, error) {
	f.published = f.container
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
