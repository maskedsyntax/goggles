package keychain

import (
	"errors"
	"os"
	"sync"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/zalando/go-keyring"
)

const (
	Service = "goggles"
	envMode = "GOGGLES_KEYCHAIN"
)

var ErrNotFound = errors.New("keychain: not found")

type Store interface {
	Name() string
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}

func Open() Store {
	if os.Getenv(envMode) == "memory" {
		return NewMemory()
	}
	return System{}
}

type System struct{}

func (System) Name() string { return "system" }

func (System) Set(key, value string) error {
	if err := keyring.Set(Service, key, value); err != nil {
		return apperr.Wrap(apperr.KeychainError, "cannot store secret", err)
	}
	return nil
}

func (System) Get(key string) (string, error) {
	v, err := keyring.Get(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", apperr.Wrap(apperr.KeychainError, "cannot read secret", err)
	}
	return v, nil
}

func (System) Delete(key string) error {
	err := keyring.Delete(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return apperr.Wrap(apperr.KeychainError, "cannot delete secret", err)
	}
	return nil
}

type Memory struct {
	mu   sync.Mutex
	data map[string]string
}

func NewMemory() *Memory {
	return &Memory{data: map[string]string{}}
}

func (m *Memory) Name() string { return "memory" }

func (m *Memory) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *Memory) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *Memory) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; !ok {
		return ErrNotFound
	}
	delete(m.data, key)
	return nil
}
