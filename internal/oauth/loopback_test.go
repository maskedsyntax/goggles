package oauth

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestListenCallback(t *testing.T) {
	state := NewState()
	sess, err := Listen(0, state)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	errCh := make(chan error, 1)
	go func() {
		_, err := sess.Wait(context.Background(), 5*time.Second)
		errCh <- err
	}()
	resp, err := http.Get(sess.Redirect + "?code=abc123&state=" + state)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestListenRejectsBadState(t *testing.T) {
	sess, err := Listen(0, "want")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	go func() { _, _ = sess.Wait(context.Background(), 3*time.Second) }()
	resp, err := http.Get(sess.Redirect + "?code=abc&state=other")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
