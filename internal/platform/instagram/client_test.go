package instagram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

func TestClientReelFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v25.0/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, `{"error":{"message":"bad token","code":190}}`, 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"user_id": "1784140000", "username": "patterns_app"})
	})
	mux.HandleFunc("/v25.0/1784140000/media", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "media_type=REELS") {
			http.Error(w, `{"error":{"message":"bad type","code":1}}`, 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "container1"})
	})
	mux.HandleFunc("/v25.0/container1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status_code": "FINISHED"})
	})
	mux.HandleFunc("/v25.0/1784140000/media_publish", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "media99"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL+"/v25.0", "tok", srv.Client())
	id, user, err := c.Me(context.Background())
	if err != nil || id != "1784140000" || user != "patterns_app" {
		t.Fatalf("me %s %s %v", id, user, err)
	}
	cid, err := c.CreateReel(context.Background(), "1784140000", "https://r2.test/a.mp4", "hi", true)
	if err != nil || cid != "container1" {
		t.Fatalf("create %s %v", cid, err)
	}
	st, err := c.ContainerStatus(context.Background(), cid)
	if err != nil || st != "FINISHED" {
		t.Fatalf("status %s %v", st, err)
	}
	mid, err := c.PublishContainer(context.Background(), "1784140000", cid)
	if err != nil || mid != "media99" {
		t.Fatalf("publish %s %v", mid, err)
	}
}

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"too many calls","code":4}}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "tok", srv.Client())
	_, err := c.CreateReel(context.Background(), "1", "https://x", "", false)
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.MetaRateLimited {
		t.Fatalf("got %v", err)
	}
}
