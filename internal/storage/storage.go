package storage

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/id"
)

const (
	ProviderR2     = "r2"
	CleanupPending = "pending"
	CleanupDeleted = "deleted"
	CleanupFailed  = "failed"
	DefaultTTL     = 2 * time.Hour
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

type Pinger interface {
	Ping(ctx context.Context) error
}

func ObjectKey(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".bin"
	}
	now := time.Now().UTC()
	return "goggles/tmp/" + now.Format("2006/01/02") + "/" + strings.TrimPrefix(id.New("obj"), "obj_") + ext
}

func JoinPublicURL(base, key string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errNoPublicURL()
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errNoPublicURL()
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + strings.TrimLeft(key, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func TTL(minutes int) time.Duration {
	if minutes <= 0 {
		return DefaultTTL
	}
	return time.Duration(minutes) * time.Minute
}

func ContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
