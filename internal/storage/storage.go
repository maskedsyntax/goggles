package storage

import (
	"context"
	"time"
)

type HostedVideo struct {
	ObjectKey string
	PublicURL string
	SizeBytes int64
	ExpiresAt time.Time
}

type VideoHost interface {
	Upload(ctx context.Context, path string) (*HostedVideo, error)
	Delete(ctx context.Context, objectKey string) error
}
