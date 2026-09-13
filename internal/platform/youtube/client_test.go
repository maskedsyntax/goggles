package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

func TestListAndUpload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/youtube/v3/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"code":401,"message":"bad token","errors":[{"reason":"authError"}]}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{"id": "UCabc", "snippet": map[string]any{"title": "Patterns"}},
				map[string]any{"id": "UCdef", "snippet": map[string]any{"title": "Clips"}},
			},
		})
	})
	mux.HandleFunc("/upload/youtube/v3/videos", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"title":"Hello"`) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"missing title"}}`))
			return
		}
		w.Header().Set("Location", "/upload-session")
		w.WriteHeader(200)
	})
	mux.HandleFunc("/upload-session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method", 405)
			return
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "vid123"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "tok", srv.Client())
	chs, err := c.ListChannels(context.Background())
	if err != nil || len(chs) != 2 || chs[0].ID != "UCabc" {
		t.Fatalf("channels %+v %v", chs, err)
	}
	path := filepath.Join(t.TempDir(), "a.mp4")
	if err := os.WriteFile(path, []byte("mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := c.Upload(context.Background(), path, VideoMeta{Title: "Hello"})
	if err != nil || id != "vid123" {
		t.Fatalf("upload %s %v", id, err)
	}
}

func TestQuotaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"quota","errors":[{"reason":"quotaExceeded"}]}}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "tok", srv.Client())
	_, err := c.ListChannels(context.Background())
	e, ok := apperr.As(err)
	if !ok || e.Code != apperr.YouTubeQuotaExceeded {
		t.Fatalf("got %v", err)
	}
}

func TestFindChannel(t *testing.T) {
	chs := []Channel{{ID: "UCabc", Title: "A"}}
	if _, ok := FindChannel(chs, "UCabc"); !ok {
		t.Fatal("expected find")
	}
	if _, ok := FindChannel(chs, "UCzzz"); ok {
		t.Fatal("expected miss")
	}
}
