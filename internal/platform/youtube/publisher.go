package youtube

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/platform"
)

type Publisher struct {
	API          API
	SkipValidate bool
}

func New(api API) *Publisher {
	return &Publisher{API: api}
}

func (p *Publisher) Platform() platform.Platform { return platform.YouTube }

func (p *Publisher) Validate(ctx context.Context, _ platform.Destination, m platform.Media, _ platform.PlatformMetadata) error {
	if len(m.Items) >= 2 || m.Kind == media.KindImage {
		return apperr.Invalid("Instagram carousels cannot be published to YouTube")
	}
	if p.SkipValidate {
		return nil
	}
	_, err := media.CheckFile(ctx, m.Path, []platform.Platform{platform.YouTube})
	return err
}

func (p *Publisher) Publish(ctx context.Context, dest platform.Destination, m platform.Media, meta platform.PlatformMetadata) (*platform.PublishResult, error) {
	if p.API == nil {
		return nil, apperr.NotImpl("youtube publishing")
	}
	if strings.TrimSpace(dest.ExternalID) == "" {
		return nil, apperr.New(apperr.YouTubeChannelRequired, "YouTube destination is missing channel id")
	}
	if err := p.Validate(ctx, dest, m, meta); err != nil {
		return nil, err
	}
	chs, err := p.API.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := FindChannel(chs, dest.ExternalID); !ok {
		return nil, apperr.New(apperr.YouTubeChannelNotFound, "authorized Google identity does not include channel "+dest.ExternalID)
	}
	vm := metaToVideo(m.Path, meta)
	id, err := p.API.Upload(ctx, m.Path, vm)
	if err != nil {
		return nil, err
	}
	return &platform.PublishResult{
		ExternalMediaID: id,
		ExternalURL:     "https://www.youtube.com/shorts/" + id,
		ProviderState:   map[string]any{"channel_id": dest.ExternalID},
	}, nil
}

func (p *Publisher) Status(ctx context.Context, _ platform.Destination, externalID string) (*platform.PublishStatus, error) {
	if p.API == nil {
		return nil, apperr.NotImpl("youtube status")
	}
	st, err := p.API.VideoStatus(ctx, externalID)
	if err != nil {
		return nil, err
	}
	return &platform.PublishStatus{State: st}, nil
}

func metaToVideo(path string, meta platform.PlatformMetadata) VideoMeta {
	vm := VideoMeta{
		Privacy:    "public",
		CategoryID: "22",
	}
	if meta != nil {
		vm.Title, _ = meta["title"].(string)
		vm.Description, _ = meta["description"].(string)
		vm.CategoryID, _ = meta["category_id"].(string)
		vm.Privacy, _ = meta["privacy"].(string)
		vm.PublishAt, _ = meta["publish_at"].(string)
		switch v := meta["made_for_kids"].(type) {
		case bool:
			vm.MadeForKids = v
		case string:
			vm.MadeForKids = v == "true" || v == "1"
		}
		switch tags := meta["tags"].(type) {
		case []string:
			vm.Tags = tags
		case []any:
			for _, t := range tags {
				if s, ok := t.(string); ok && s != "" {
					vm.Tags = append(vm.Tags, s)
				}
			}
		case string:
			for _, p := range strings.Split(tags, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					vm.Tags = append(vm.Tags, p)
				}
			}
		}
	}
	if strings.TrimSpace(vm.Title) == "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		vm.Title = base
	}
	if vm.CategoryID == "" {
		vm.CategoryID = "22"
	}
	if vm.Privacy == "" {
		vm.Privacy = "public"
	}
	return vm
}
