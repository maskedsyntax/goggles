package instagram

import (
	"context"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/platform"
)

type Publisher struct{}

func New() Publisher { return Publisher{} }

func (Publisher) Platform() platform.Platform { return platform.Instagram }

func (Publisher) Validate(ctx context.Context, _ platform.Destination, m platform.Media, _ platform.PlatformMetadata) error {
	_, err := media.CheckFile(ctx, m.Path, []platform.Platform{platform.Instagram})
	return err
}

func (Publisher) Publish(context.Context, platform.Destination, platform.Media, platform.PlatformMetadata) (*platform.PublishResult, error) {
	return nil, apperr.NotImpl("instagram publishing")
}

func (Publisher) Status(context.Context, platform.Destination, string) (*platform.PublishStatus, error) {
	return nil, apperr.NotImpl("instagram status")
}
