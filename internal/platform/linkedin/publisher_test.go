package linkedin

import (
	"context"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/platform"
)

type fakeAPI struct {
	imagePaths, videoPaths      []string
	owner, commentary, mediaURN string
	images                      []string
}

func (f *fakeAPI) Me(context.Context) (string, string, error) { return "42", "Member", nil }
func (f *fakeAPI) UploadImage(_ context.Context, _ string, path string) (string, error) {
	f.imagePaths = append(f.imagePaths, path)
	return "urn:li:image:" + path, nil
}
func (f *fakeAPI) UploadVideo(_ context.Context, _ string, path string) (string, error) {
	f.videoPaths = append(f.videoPaths, path)
	return "urn:li:video:" + path, nil
}
func (f *fakeAPI) CreatePost(_ context.Context, owner, text, title, mediaURN string, images []string) (string, error) {
	f.owner = owner
	f.commentary = text
	f.mediaURN = mediaURN
	f.images = images
	return "urn:li:share:123", nil
}
func (f *fakeAPI) PostStatus(context.Context, string) (string, error) { return "PUBLISHED", nil }
func TestPublisherPostTypes(t *testing.T) {
	dest := platform.Destination{ExternalID: "urn:li:person:42"}
	cases := []struct {
		name           string
		media          platform.Media
		images, videos int
	}{
		{"text", platform.Media{}, 0, 0},
		{"image", platform.Media{Kind: "image", Path: "a.jpg"}, 1, 0},
		{"video", platform.Media{Kind: "video", Path: "a.mp4", SizeBytes: 100 * 1024, Duration: 4 * time.Second}, 0, 1},
		{"multi_image", platform.Media{Items: []platform.Media{{Kind: "image", Path: "a.jpg"}, {Kind: "image", Path: "b.png"}}}, 2, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{}
			p := New(api)
			res, err := p.Publish(context.Background(), dest, tc.media, platform.PlatformMetadata{"caption": "hello"})
			if err != nil {
				t.Fatal(err)
			}
			if res.ExternalMediaID != "urn:li:share:123" || api.owner != dest.ExternalID || api.commentary != "hello" {
				t.Fatalf("bad result: %+v %+v", res, api)
			}
			if len(api.imagePaths) != tc.images || len(api.videoPaths) != tc.videos {
				t.Fatalf("uploads images=%d videos=%d", len(api.imagePaths), len(api.videoPaths))
			}
			if tc.name == "multi_image" && len(api.images) != 2 {
				t.Fatal("missing multiImage content")
			}
		})
	}
}
func TestPublisherRejectsMixedMedia(t *testing.T) {
	p := New(&fakeAPI{})
	d := platform.Destination{ExternalID: "urn:li:person:42"}
	m := platform.Media{Items: []platform.Media{{Kind: "image", Path: "a.jpg"}, {Kind: "video", Path: "b.mp4"}}}
	if _, err := p.Publish(context.Background(), d, m, platform.PlatformMetadata{"caption": "hello"}); err == nil {
		t.Fatal("expected rejection")
	}
}
