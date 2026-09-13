package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type Platform string

const (
	Instagram Platform = "instagram"
	YouTube   Platform = "youtube"
)

func Parse(s string) (Platform, error) {
	switch Platform(strings.ToLower(strings.TrimSpace(s))) {
	case Instagram:
		return Instagram, nil
	case YouTube:
		return YouTube, nil
	default:
		return "", apperr.Invalid("unsupported platform " + s + " (instagram|youtube)")
	}
}

func (p Platform) String() string { return string(p) }

type Destination struct {
	ID               string
	AccountID        string
	Alias            string
	Platform         Platform
	ExternalID       string
	ExternalUsername string
	ExternalTitle    string
	Timezone         string
	Enabled          bool
	Metadata         map[string]any
}

type Media struct {
	Path       string
	Hash       string
	Container  string
	VideoCodec string
	AudioCodec string
	Width      int
	Height     int
	Duration   time.Duration
	FrameRate  float64
	Bitrate    int64
	SizeBytes  int64
	HasAudio   bool
}

type PlatformMetadata map[string]any

type PublishResult struct {
	ExternalMediaID string
	ExternalURL     string
	ProviderState   map[string]any
}

type PublishStatus struct {
	State   string
	Message string
}

type Publisher interface {
	Platform() Platform
	Validate(ctx context.Context, dest Destination, media Media, meta PlatformMetadata) error
	Publish(ctx context.Context, dest Destination, media Media, meta PlatformMetadata) (*PublishResult, error)
	Status(ctx context.Context, dest Destination, externalID string) (*PublishStatus, error)
}

type Router struct {
	publishers map[Platform]Publisher
}

func NewRouter(publishers ...Publisher) *Router {
	r := &Router{publishers: map[Platform]Publisher{}}
	for _, p := range publishers {
		r.publishers[p.Platform()] = p
	}
	return r
}

func (r *Router) For(p Platform) (Publisher, error) {
	pub, ok := r.publishers[p]
	if !ok {
		return nil, apperr.NotImpl(fmt.Sprintf("%s publisher", p))
	}
	return pub, nil
}
