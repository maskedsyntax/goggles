package storage

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type Memory struct {
	mu         sync.Mutex
	Objects    map[string][]byte
	PublicBase string
	TTL        time.Duration
}

func NewMemory(publicBase string, ttl time.Duration) *Memory {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if publicBase == "" {
		publicBase = "https://r2.test"
	}
	return &Memory{
		Objects:    map[string][]byte{},
		PublicBase: publicBase,
		TTL:        ttl,
	}
}

func (m *Memory) Upload(_ context.Context, path string) (*HostedVideo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperr.New(apperr.FileNotFound, path)
		}
		return nil, apperr.Wrap(apperr.R2UploadFailed, "cannot read file for upload", err)
	}
	key := ObjectKey(path)
	pub, err := JoinPublicURL(m.PublicBase, key)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.Objects[key] = data
	m.mu.Unlock()
	return &HostedVideo{
		ObjectKey: key,
		PublicURL: pub,
		SizeBytes: int64(len(data)),
		ExpiresAt: time.Now().UTC().Add(m.TTL),
	}, nil
}

func (m *Memory) Delete(_ context.Context, objectKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Objects, objectKey)
	return nil
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Objects[key]
	return ok
}
