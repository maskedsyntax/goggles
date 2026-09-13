package instagram

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type Publisher struct {
	API          API
	Host         storage.VideoHost
	DB           *sql.DB
	JobID        string
	SkipValidate bool
	PollEvery    time.Duration
	PollFor      time.Duration
}

func New(api API, host storage.VideoHost) *Publisher {
	return &Publisher{
		API:       api,
		Host:      host,
		PollEvery: 5 * time.Second,
		PollFor:   5 * time.Minute,
	}
}

func (p *Publisher) Platform() platform.Platform { return platform.Instagram }

func (p *Publisher) Validate(ctx context.Context, _ platform.Destination, m platform.Media, _ platform.PlatformMetadata) error {
	if p.SkipValidate {
		return nil
	}
	if len(m.Items) >= 2 {
		if len(m.Items) > media.MaxCarouselItems {
			return apperr.Invalid("Instagram carousels are limited to 10 items")
		}
		for _, it := range m.Items {
			if _, err := media.CheckCarouselItem(ctx, it.Path); err != nil {
				return err
			}
		}
		return nil
	}
	if m.Kind == media.KindImage {
		return apperr.Invalid("Instagram carousels need 2–10 items")
	}
	_, err := media.CheckFile(ctx, m.Path, []platform.Platform{platform.Instagram})
	return err
}

func (p *Publisher) Publish(ctx context.Context, dest platform.Destination, m platform.Media, meta platform.PlatformMetadata) (*platform.PublishResult, error) {
	if dest.ExternalID == "" {
		return nil, apperr.New(apperr.DestinationNotFound, "Instagram destination is missing ig user id")
	}
	if p.API == nil || p.Host == nil {
		return nil, apperr.NotImpl("instagram publishing")
	}
	if err := p.Validate(ctx, dest, m, meta); err != nil {
		return nil, err
	}
	if len(m.Items) >= 2 {
		return p.publishCarousel(ctx, dest, m, meta)
	}
	hosted, err := p.Host.Upload(ctx, m.Path)
	if err != nil {
		return nil, err
	}
	if p.DB != nil {
		if _, err := storage.RecordHosted(ctx, p.DB, hosted, p.JobID); err != nil {
			_ = p.Host.Delete(ctx, hosted.ObjectKey)
			return nil, err
		}
	}

	containerID, err := p.API.CreateReel(ctx, dest.ExternalID, hosted.PublicURL, caption(meta), shareToFeed(meta))
	if err != nil {
		_ = p.Host.Delete(ctx, hosted.ObjectKey)
		return nil, err
	}
	if err := p.waitFinished(ctx, containerID); err != nil {
		_ = p.Host.Delete(ctx, hosted.ObjectKey)
		return nil, err
	}
	mediaID, err := p.API.PublishContainer(ctx, dest.ExternalID, containerID)
	if err != nil {
		_ = p.Host.Delete(ctx, hosted.ObjectKey)
		return nil, err
	}
	if delErr := p.Host.Delete(ctx, hosted.ObjectKey); delErr != nil {
		return &platform.PublishResult{
			ExternalMediaID: mediaID,
			ProviderState:   map[string]any{"container_id": containerID, "r2_key": hosted.ObjectKey, "cleanup_error": delErr.Error()},
		}, nil
	}
	return &platform.PublishResult{
		ExternalMediaID: mediaID,
		ProviderState:   map[string]any{"container_id": containerID},
	}, nil
}

func (p *Publisher) publishCarousel(ctx context.Context, dest platform.Destination, m platform.Media, meta platform.PlatformMetadata) (*platform.PublishResult, error) {
	var hosted []*storage.HostedVideo
	cleanup := func() {
		for _, h := range hosted {
			_ = p.Host.Delete(ctx, h.ObjectKey)
		}
	}
	var childIDs []string
	for _, it := range m.Items {
		h, err := p.Host.Upload(ctx, it.Path)
		if err != nil {
			cleanup()
			return nil, err
		}
		hosted = append(hosted, h)
		if p.DB != nil {
			if _, err := storage.RecordHosted(ctx, p.DB, h, p.JobID); err != nil {
				cleanup()
				return nil, err
			}
		}
		imageURL, videoURL := "", ""
		if itemIsImage(it) {
			imageURL = h.PublicURL
		} else {
			videoURL = h.PublicURL
		}
		cid, err := p.API.CreateCarouselItem(ctx, dest.ExternalID, imageURL, videoURL)
		if err != nil {
			cleanup()
			return nil, err
		}
		if err := p.waitFinished(ctx, cid); err != nil {
			cleanup()
			return nil, err
		}
		childIDs = append(childIDs, cid)
	}
	parentID, err := p.API.CreateCarousel(ctx, dest.ExternalID, childIDs, caption(meta), shareToFeed(meta))
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := p.waitFinished(ctx, parentID); err != nil {
		cleanup()
		return nil, err
	}
	mediaID, err := p.API.PublishContainer(ctx, dest.ExternalID, parentID)
	if err != nil {
		cleanup()
		return nil, err
	}
	state := map[string]any{"container_id": parentID, "children": childIDs, "kind": "carousel"}
	var cleanupErr error
	var leftover []string
	for _, h := range hosted {
		if err := p.Host.Delete(ctx, h.ObjectKey); err != nil {
			if cleanupErr == nil {
				cleanupErr = err
			}
			leftover = append(leftover, h.ObjectKey)
		}
	}
	if cleanupErr != nil {
		state["cleanup_error"] = cleanupErr.Error()
		state["r2_keys"] = leftover
	}
	return &platform.PublishResult{ExternalMediaID: mediaID, ProviderState: state}, nil
}

func itemIsImage(m platform.Media) bool {
	if m.Kind == media.KindImage {
		return true
	}
	switch strings.ToLower(filepath.Ext(m.Path)) {
	case ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

func (p *Publisher) Status(ctx context.Context, _ platform.Destination, externalID string) (*platform.PublishStatus, error) {
	st, err := p.API.ContainerStatus(ctx, externalID)
	if err != nil {
		return nil, err
	}
	return &platform.PublishStatus{State: st}, nil
}

func (p *Publisher) waitFinished(ctx context.Context, containerID string) error {
	every := p.PollEvery
	if every <= 0 {
		every = time.Second
	}
	deadline := time.Now().Add(p.PollFor)
	if p.PollFor <= 0 {
		deadline = time.Now().Add(5 * time.Minute)
	}
	for {
		status, err := p.API.ContainerStatus(ctx, containerID)
		if err != nil {
			return err
		}
		switch status {
		case "FINISHED", "PUBLISHED":
			return nil
		case "ERROR":
			return apperr.New(apperr.InstagramProcessingFailed, "Instagram container processing failed")
		case "EXPIRED":
			return apperr.New(apperr.InstagramProcessingFailed, "Instagram container expired")
		}
		if time.Now().After(deadline) {
			return apperr.New(apperr.JobTimeout, "timed out waiting for Instagram processing")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

func caption(meta platform.PlatformMetadata) string {
	if meta == nil {
		return ""
	}
	if s, ok := meta["caption"].(string); ok {
		return s
	}
	return ""
}

func shareToFeed(meta platform.PlatformMetadata) bool {
	if meta == nil {
		return false
	}
	switch v := meta["share_to_feed"].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}
