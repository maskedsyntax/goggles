package token

import (
	"context"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/platform"
)

func TestGoogleSkipsRefreshWhenUnexpired(t *testing.T) {
	kc := keychain.NewMemory()
	ref := "youtube/acc1"
	_ = kc.Set(ref, "old")
	_ = kc.Set(ref+"/refresh", "r1")
	_ = kc.Set(ref+"/expires", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	s := &Source{Store: kc, Config: config.File{Google: config.Google{ClientID: "cid"}}}
	tok, err := s.Access(context.Background(), platform.YouTube, "acc1")
	if err != nil || tok != "old" {
		t.Fatalf("unexpired %s %v", tok, err)
	}
}

func TestInstagramUsesStoredTokenWhenNotExpiring(t *testing.T) {
	kc := keychain.NewMemory()
	_ = kc.Set("instagram/acc1", "long-lived")
	_ = kc.Set("instagram/acc1/expires", time.Now().Add(40*24*time.Hour).UTC().Format(time.RFC3339))
	s := &Source{Store: kc}
	tok, err := s.Access(context.Background(), platform.Instagram, "acc1")
	if err != nil || tok != "long-lived" {
		t.Fatalf("%s %v", tok, err)
	}
}
