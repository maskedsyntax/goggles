package youtube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.a", "refresh_token": "1//r", "expires_in": 3600,
		})
	}))
	t.Cleanup(srv.Close)
	tokenEndpoint = srv.URL
	t.Cleanup(func() { tokenEndpoint = TokenURL })
	tok, err := Exchange(context.Background(), srv.Client(), "cid", "csec", "http://127.0.0.1:8787/callback", "code", "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "ya29.a" || tok.RefreshToken != "1//r" {
		t.Fatalf("%+v", tok)
	}
}

func TestPKCE(t *testing.T) {
	p := NewPKCE()
	if p.Verifier == "" || p.Challenge == "" || p.Verifier == p.Challenge {
		t.Fatalf("%+v", p)
	}
}
