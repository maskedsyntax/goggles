package token

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/platform/instagram"
	"github.com/maskedsyntax/goggles/internal/platform/youtube"
)

const (
	googleLeeway = 5 * time.Minute
	igLeeway     = 7 * 24 * time.Hour
)

type Source struct {
	Store  keychain.Store
	Config config.File
	HTTP   *http.Client
	Now    func() time.Time
}

func (s *Source) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Source) Access(ctx context.Context, p platform.Platform, accountID string) (string, error) {
	if s == nil || s.Store == nil {
		return "", apperr.New(apperr.AuthRequired, "no keychain")
	}
	ref := string(p) + "/" + accountID
	access, err := s.Store.Get(ref)
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return "", apperr.New(apperr.AuthRequired, string(p)+" token missing")
		}
		return "", err
	}
	exp := s.expires(ref)
	switch p {
	case platform.YouTube:
		refresh, rerr := s.Store.Get(ref + "/refresh")
		if rerr != nil {
			refresh = ""
		}
		need := refresh != "" && (exp.IsZero() || s.now().Add(googleLeeway).After(exp))
		if need {
			return s.refreshGoogle(ctx, ref, refresh)
		}
	case platform.Instagram:
		need := !exp.IsZero() && s.now().Add(igLeeway).After(exp)
		if need {
			tok, err := instagram.Refresh(ctx, s.HTTP, access)
			if err != nil {
				return "", err
			}
			if err := s.save(ref, tok.AccessToken, "", tok.ExpiresIn); err != nil {
				return "", err
			}
			return tok.AccessToken, nil
		}
	}
	return access, nil
}

func (s *Source) save(ref, access, refresh string, expiresIn int) error {
	if err := s.Store.Set(ref, access); err != nil {
		return err
	}
	if refresh != "" {
		if err := s.Store.Set(ref+"/refresh", refresh); err != nil {
			return err
		}
	}
	if expiresIn > 0 {
		exp := s.now().Add(time.Duration(expiresIn) * time.Second)
		if err := s.Store.Set(ref+"/expires", exp.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) expires(ref string) time.Time {
	raw, err := s.Store.Get(ref + "/expires")
	if err != nil || raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (s *Source) refreshGoogle(ctx context.Context, ref, refresh string) (string, error) {
	secret, err := keychain.GetRef(s.Store, s.Config.Google.ClientSecretRef, "goggles-google-client-secret", "GOGGLES_GOOGLE_CLIENT_SECRET")
	if err != nil {
		return "", err
	}
	if s.Config.Google.ClientID == "" {
		return "", apperr.New(apperr.ConfigMissing, "set Google client id via goggles auth setup youtube")
	}
	tok, err := youtube.Refresh(ctx, s.HTTP, s.Config.Google.ClientID, secret, refresh)
	if err != nil {
		return "", err
	}
	if err := s.save(ref, tok.AccessToken, tok.RefreshToken, tok.ExpiresIn); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func StoreExpiry(store keychain.Store, ref string, expiresIn int, now time.Time) error {
	if store == nil || expiresIn <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	exp := now.Add(time.Duration(expiresIn) * time.Second)
	return store.Set(ref+"/expires", exp.UTC().Format(time.RFC3339))
}
