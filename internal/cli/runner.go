package cli

import (
	ig "github.com/maskedsyntax/goggles/internal/platform/instagram"
	"github.com/maskedsyntax/goggles/internal/publish"
	"github.com/maskedsyntax/goggles/internal/storage"
	"github.com/maskedsyntax/goggles/internal/token"
)

func (a *App) tokens() *token.Source {
	return &token.Source{Store: a.Keychain, Config: a.Config}
}

func (a *App) runner(host storage.VideoHost) *publish.Runner {
	return &publish.Runner{
		DB:          a.DB,
		Host:        host,
		Keychain:    a.Keychain,
		Tokens:      a.tokens(),
		GraphBase:   ig.GraphBase(a.Config.Meta.GraphHost, a.Config.Meta.GraphVersion),
		MaxAttempts: a.Config.Publishing.MaxRetries,
	}
}
