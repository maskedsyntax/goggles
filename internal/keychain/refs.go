package keychain

import (
	"errors"
	"os"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

func RefKey(ref, fallback string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fallback
	}
	return strings.TrimPrefix(ref, "keychain:")
}

func GetRef(store Store, ref, fallback, env string) (string, error) {
	if env != "" {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	if store == nil {
		return "", apperr.New(apperr.AuthRequired, "secret store is unavailable")
	}
	v, err := store.Get(RefKey(ref, fallback))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", apperr.New(apperr.AuthRequired, "missing secret "+RefKey(ref, fallback)+"; run goggles auth setup")
		}
		return "", err
	}
	return v, nil
}

func SetRef(store Store, ref, fallback, value string) error {
	if strings.TrimSpace(value) == "" {
		return apperr.Invalid("secret value is empty")
	}
	return store.Set(RefKey(ref, fallback), value)
}
