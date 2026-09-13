package youtube

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
)

type stubAPI struct {
	channels []Channel
	uploaded string
	title    string
}

func (s *stubAPI) ListChannels(context.Context) ([]Channel, error) { return s.channels, nil }
func (s *stubAPI) Upload(_ context.Context, _ string, meta VideoMeta) (string, error) {
	s.title = meta.Title
	return s.uploaded, nil
}
func (s *stubAPI) VideoStatus(context.Context, string) (string, error) { return "processed", nil }

func TestPublisherRequiresMatchingChannel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &stubAPI{channels: []Channel{{ID: "UCabc", Title: "Patterns"}}, uploaded: "vid"}
	p := New(api)
	p.SkipValidate = true
	_, err := p.Publish(context.Background(), platform.Destination{ExternalID: "UCzzz"}, platform.Media{Path: path}, nil)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.YouTubeChannelNotFound {
		t.Fatalf("got %v", err)
	}
	res, err := p.Publish(context.Background(), platform.Destination{ExternalID: "UCabc"}, platform.Media{Path: path}, platform.PlatformMetadata{"title": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExternalMediaID != "vid" || api.title != "Hello" {
		t.Fatalf("%+v title=%s", res, api.title)
	}
}

func TestPublisherRejectsCarousel(t *testing.T) {
	p := New(&stubAPI{channels: []Channel{{ID: "UCabc"}}, uploaded: "vid"})
	p.SkipValidate = true
	_, err := p.Publish(context.Background(), platform.Destination{ExternalID: "UCabc"}, platform.Media{
		Path:  "a.jpg",
		Kind:  "image",
		Items: []platform.Media{{Path: "a.jpg"}, {Path: "b.jpg"}},
	}, nil)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.InvalidInput {
		t.Fatalf("got %v", err)
	}
}
