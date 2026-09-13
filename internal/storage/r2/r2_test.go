package r2

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maskedsyntax/goggles/internal/config"
	"github.com/maskedsyntax/goggles/internal/keychain"
)

type fakeAPI struct {
	objects map[string][]byte
	putErr  error
	delErr  error
	pingErr error
}

func (f *fakeAPI) PutObject(_ context.Context, key string, body io.Reader, _ string, _ int64) error {
	if f.putErr != nil {
		return f.putErr
	}
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeAPI) DeleteObject(_ context.Context, key string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.objects, key)
	return nil
}

func (f *fakeAPI) HeadBucket(context.Context) error { return f.pingErr }

func TestClientUploadDelete(t *testing.T) {
	api := &fakeAPI{objects: map[string][]byte{}}
	c, err := New(context.Background(), Options{
		AccountID:     "acc",
		Bucket:        "goggles-temp",
		PublicBaseURL: "https://media.example.com",
		TTL:           time.Hour,
		API:           api,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	hosted, err := c.Upload(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(api.objects[hosted.ObjectKey], []byte("mp4")) {
		t.Fatal("object not stored")
	}
	if hosted.PublicURL == "" {
		t.Fatal("missing public url")
	}
	if err := c.Delete(context.Background(), hosted.ObjectKey); err != nil {
		t.Fatal(err)
	}
	if _, ok := api.objects[hosted.ObjectKey]; ok {
		t.Fatal("expected delete")
	}
}

func TestResolveCredentialsEnv(t *testing.T) {
	t.Setenv(envAccess, "ak")
	t.Setenv(envSecret, "sk")
	access, secret, err := ResolveCredentials(config.R2{}, keychain.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	if access != "ak" || secret != "sk" {
		t.Fatalf("%s %s", access, secret)
	}
}

func TestStoreAndResolveKeychain(t *testing.T) {
	kc := keychain.NewMemory()
	cfg := config.Default().R2
	if err := StoreCredentials(kc, cfg, "ak", "sk"); err != nil {
		t.Fatal(err)
	}
	access, secret, err := ResolveCredentials(cfg, kc)
	if err != nil {
		t.Fatal(err)
	}
	if access != "ak" || secret != "sk" {
		t.Fatalf("%s %s", access, secret)
	}
}
