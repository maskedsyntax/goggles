package instagram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeAndLongLived(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "short", "user_id": 1784140000})
	})
	mux.HandleFunc("/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "long", "expires_in": 5184000})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	tokenEndpoint = srv.URL + "/oauth/access_token"
	longLivedEndpoint = srv.URL + "/access_token"
	t.Cleanup(func() {
		tokenEndpoint = TokenURL
		longLivedEndpoint = LongLivedURL
	})

	tok, err := Exchange(context.Background(), srv.Client(), "id", "secret", "http://127.0.0.1:8787/callback", "code")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "short" || tok.UserID != "1784140000" {
		t.Fatalf("%+v", tok)
	}
	long, err := ExchangeLongLived(context.Background(), srv.Client(), "secret", "short")
	if err != nil || long.AccessToken != "long" {
		t.Fatalf("%+v %v", long, err)
	}
}

func TestParseWrappedToken(t *testing.T) {
	tok, err := parseShortToken([]byte(`{"data":[{"access_token":"t","user_id":"u1"}]}`))
	if err != nil || tok.AccessToken != "t" || tok.UserID != "u1" {
		t.Fatalf("%+v %v", tok, err)
	}
}

func TestRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("grant_type") != "ig_refresh_token" {
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "longer", "expires_in": 5184000})
	}))
	t.Cleanup(srv.Close)
	refreshEndpoint = srv.URL
	t.Cleanup(func() { refreshEndpoint = RefreshURL })
	tok, err := Refresh(context.Background(), srv.Client(), "old")
	if err != nil || tok.AccessToken != "longer" {
		t.Fatalf("%+v %v", tok, err)
	}
}
