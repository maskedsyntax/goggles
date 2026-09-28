package linkedin

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
)

type Publisher struct{ API API }

func New(api API) *Publisher                     { return &Publisher{API: api} }
func (p *Publisher) Platform() platform.Platform { return platform.LinkedIn }
func (p *Publisher) Validate(_ context.Context, d platform.Destination, m platform.Media, meta platform.PlatformMetadata) error {
	if !strings.HasPrefix(d.ExternalID, "urn:li:person:") {
		return apperr.Invalid("LinkedIn destination needs a member URN")
	}
	text, _ := meta["caption"].(string)
	if strings.TrimSpace(text) == "" && m.Path == "" && len(m.Items) == 0 {
		return apperr.Invalid("LinkedIn posts need --caption or linkedin.caption in a sidecar")
	}
	if m.Path == "" && len(m.Items) == 0 {
		return nil
	}
	items := m.Items
	if len(items) == 0 {
		items = []platform.Media{m}
	}
	if len(items) > 20 {
		return apperr.Invalid("LinkedIn supports at most 20 images")
	}
	for _, it := range items {
		if it.Kind == "image" {
			switch strings.ToLower(filepath.Ext(it.Path)) {
			case ".jpg", ".jpeg", ".png", ".gif":
			default:
				return apperr.Invalid("LinkedIn images must be JPG, PNG, or GIF")
			}
		}
		if it.Kind != "image" && it.Kind != "video" {
			return apperr.Invalid("LinkedIn supports images or videos")
		}
		if it.Kind == "video" {
			if strings.ToLower(filepath.Ext(it.Path)) != ".mp4" {
				return apperr.Invalid("LinkedIn videos must be MP4")
			}
			if it.SizeBytes < 75*1024 || it.SizeBytes > 500*1024*1024 {
				return apperr.Invalid("LinkedIn videos must be between 75 KB and 500 MB")
			}
			if it.Duration < 3*time.Second || it.Duration > 30*time.Minute {
				return apperr.Invalid("LinkedIn videos must be between 3 seconds and 30 minutes")
			}
		}
		if it.Kind == "image" && it.Width > 0 && it.Height > 0 && int64(it.Width)*int64(it.Height) >= 36152320 {
			return apperr.Invalid("LinkedIn image exceeds the pixel limit")
		}
		if len(items) > 1 && it.Kind != "image" {
			return apperr.Invalid("LinkedIn multi-image posts cannot include videos")
		}
	}
	return nil
}
func (p *Publisher) Publish(ctx context.Context, d platform.Destination, m platform.Media, meta platform.PlatformMetadata) (*platform.PublishResult, error) {
	if p.API == nil {
		return nil, apperr.NotImpl("LinkedIn publishing")
	}
	if err := p.Validate(ctx, d, m, meta); err != nil {
		return nil, err
	}
	caption, _ := meta["caption"].(string)
	title, _ := meta["title"].(string)
	mediaURN := ""
	var images []string
	items := m.Items
	if len(items) == 0 && m.Path != "" {
		items = []platform.Media{m}
	}
	for _, it := range items {
		var id string
		var err error
		if it.Kind == "video" {
			id, err = p.API.UploadVideo(ctx, d.ExternalID, it.Path)
		} else {
			id, err = p.API.UploadImage(ctx, d.ExternalID, it.Path)
		}
		if err != nil {
			return nil, err
		}
		if len(items) > 1 {
			images = append(images, id)
		} else {
			mediaURN = id
		}
	}
	id, err := p.API.CreatePost(ctx, d.ExternalID, caption, title, mediaURN, images)
	if err != nil {
		return nil, err
	}
	return &platform.PublishResult{ExternalMediaID: id, ProviderState: map[string]any{"media_urn": mediaURN}}, nil
}
func (p *Publisher) Status(ctx context.Context, _ platform.Destination, id string) (*platform.PublishStatus, error) {
	st, err := p.API.PostStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	return &platform.PublishStatus{State: st}, nil
}
