package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

func NewState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func OpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

type Session struct {
	Redirect string
	Port     int
	ch       chan result
	srv      *http.Server
	ln       net.Listener
}

type result struct {
	code string
	err  string
}

func Listen(port int, wantState string) (*Session, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if port < 0 {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, apperr.Wrap(apperr.AuthRequired, "cannot bind OAuth callback port", err)
	}
	bound := ln.Addr().(*net.TCPAddr).Port
	s := &Session{
		Redirect: fmt.Sprintf("http://127.0.0.1:%d/callback", bound),
		Port:     bound,
		ch:       make(chan result, 1),
		ln:       ln,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if wantState != "" && q.Get("state") != wantState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			s.send(result{err: "state mismatch"})
			return
		}
		if e := q.Get("error"); e != "" {
			msg := e
			if d := q.Get("error_description"); d != "" {
				msg = e + ": " + d
			}
			fmt.Fprint(w, page("Authorization failed", msg))
			s.send(result{err: msg})
			return
		}
		code := strings.TrimSuffix(q.Get("code"), "#_")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			s.send(result{err: "missing code"})
			return
		}
		fmt.Fprint(w, page("goggles", "Login complete. You can close this window and return to the terminal."))
		s.send(result{code: code})
	})
	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *Session) Wait(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-ctx.Done():
		return "", apperr.New(apperr.AuthRequired, "timed out waiting for browser login")
	case res := <-s.ch:
		if res.err != "" {
			return "", apperr.New(apperr.AuthRequired, res.err)
		}
		return res.code, nil
	}
}

func (s *Session) Close() {
	if s == nil || s.srv == nil {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(c)
}

func (s *Session) send(res result) {
	select {
	case s.ch <- res:
	default:
	}
}

func AuthURL(base string, params url.Values) string {
	return strings.TrimRight(base, "?") + "?" + params.Encode()
}

func page(title, body string) string {
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head><body style="font-family:sans-serif;padding:2rem"><h1>%s</h1><p>%s</p></body></html>`, title, title, body)
}
