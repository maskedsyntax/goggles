package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, home string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	t.Setenv("GOGGLES_HOME", home)
	t.Setenv("GOGGLES_KEYCHAIN", "memory")
	var out, err bytes.Buffer
	code = Execute(bytes.NewReader(nil), &out, &err, args)
	return out.String(), err.String(), code
}

func TestHelp(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := run(t, home, "--help")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"account", "profile", "media", "doctor", "config"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %s\n%s", want, stdout)
		}
	}
}

func TestConfigAndProfileFlow(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := run(t, home, "config", "path", "--json")
	if code != 0 {
		t.Fatalf("config path: %d %s %s", code, stdout, stderr)
	}
	var pathPayload map[string]any
	if err := json.Unmarshal([]byte(stdout), &pathPayload); err != nil {
		t.Fatal(err)
	}
	if pathPayload["path"] != filepath.Join(home, "config.toml") {
		t.Fatalf("path payload: %v", pathPayload)
	}

	_, stderr, code = run(t, home, "config", "set", "general.timezone", "Asia/Kolkata")
	if code != 0 {
		t.Fatalf("config set: %d %s", code, stderr)
	}

	stdout, stderr, code = run(t, home, "account", "add", "youtube", "--alias", "patterns-youtube-main", "--json")
	if code == 0 {
		t.Fatalf("expected youtube channel required, got %s", stdout)
	}
	if !strings.Contains(stdout, "YOUTUBE_CHANNEL_REQUIRED") {
		t.Fatalf("stdout=%s stderr=%s", stdout, stderr)
	}

	_, stderr, code = run(t, home, "account", "add", "instagram", "--alias", "patterns-instagram", "--username", "patterns_app")
	if code != 0 {
		t.Fatalf("add ig: %d %s", code, stderr)
	}
	_, stderr, code = run(t, home, "account", "add", "youtube", "--alias", "patterns-youtube-main", "--channel-id", "UCabc123", "--channel-title", "Patterns")
	if code != 0 {
		t.Fatalf("add yt: %d %s", code, stderr)
	}
	_, stderr, code = run(t, home, "profile", "create", "patterns")
	if code != 0 {
		t.Fatalf("create profile: %d %s", code, stderr)
	}
	_, stderr, code = run(t, home, "profile", "add-destination", "patterns", "patterns-instagram")
	if code != 0 {
		t.Fatalf("attach ig: %d %s", code, stderr)
	}
	_, stderr, code = run(t, home, "profile", "add-destination", "patterns", "patterns-youtube-main")
	if code != 0 {
		t.Fatalf("attach yt: %d %s", code, stderr)
	}

	stdout, stderr, code = run(t, home, "profile", "show", "patterns", "--json")
	if code != 0 {
		t.Fatalf("show: %d %s", code, stderr)
	}
	var show map[string]any
	if err := json.Unmarshal([]byte(stdout), &show); err != nil {
		t.Fatal(err)
	}
	dests, ok := show["destinations"].([]any)
	if !ok || len(dests) != 2 {
		t.Fatalf("destinations: %v", show)
	}

	stdout, stderr, code = run(t, home, "media", "check", filepath.Join(home, "missing.mp4"), "--json")
	if code == 0 {
		t.Fatal("expected missing file")
	}
	if !strings.Contains(stdout, "FILE_NOT_FOUND") {
		t.Fatalf("stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, code = run(t, home, "doctor", "--json")
	if stdout == "" {
		t.Fatalf("doctor empty stderr=%s code=%d", stderr, code)
	}
	if !strings.Contains(stdout, `"name": "sqlite"`) {
		t.Fatalf("doctor=%s", stdout)
	}
}

func TestAccountTestRequiresAuth(t *testing.T) {
	home := t.TempDir()
	_, stderr, code := run(t, home, "account", "add", "instagram", "--alias", "patterns-instagram", "--username", "patterns_app")
	if code != 0 {
		t.Fatalf("add: %d %s", code, stderr)
	}
	stdout, _, code := run(t, home, "account", "test", "patterns-instagram", "--json")
	if code == 0 {
		t.Fatal("expected auth required")
	}
	if !strings.Contains(stdout, "AUTH_REQUIRED") {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestJSONStdoutOnly(t *testing.T) {
	home := t.TempDir()
	stdout, stderr, code := run(t, home, "profile", "show", "missing", "--json")
	if code == 0 {
		t.Fatal("expected not found")
	}
	if strings.TrimSpace(stderr) != "" && os.Getenv("DEBUG") == "1" {
		t.Log(stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout not json: %s err=%v", stdout, err)
	}
	if payload["success"] != false {
		t.Fatalf("payload=%v", payload)
	}
}

func TestStorageMemoryFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOGGLES_STORAGE", "memory")
	stdout, stderr, code := run(t, home, "storage", "status", "--json")
	if code != 0 {
		t.Fatalf("status: %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"credentials": true`) {
		t.Fatalf("status=%s", stdout)
	}

	path := filepath.Join(home, "probe.txt")
	if err := os.WriteFile(path, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = run(t, home, "storage", "test", path, "--json")
	if code != 0 {
		t.Fatalf("test: %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Fatalf("test=%s", stdout)
	}

	_, stderr, code = run(t, home, "config", "set", "r2.account_id", "acc")
	if code != 0 {
		t.Fatalf("config: %d %s", code, stderr)
	}
	_, stderr, code = run(t, home, "storage", "credentials", "--access-key", "ak", "--secret-key", "sk")
	if code != 0 {
		t.Fatalf("creds: %d %s", code, stderr)
	}
	stdout, stderr, code = run(t, home, "storage", "cleanup", "--json")
	if code != 0 {
		t.Fatalf("cleanup: %d %s %s", code, stdout, stderr)
	}

	stdout, stderr, code = run(t, home, "auth", "login", "youtube", "--alias", "yt", "--access-token", "tok", "--json")
	if code == 0 {
		t.Fatalf("expected channel required, got %s", stdout)
	}
	if !strings.Contains(stdout, "YOUTUBE_CHANNEL_REQUIRED") {
		t.Fatalf("stdout=%s stderr=%s", stdout, stderr)
	}

	_, stderr, code = run(t, home, "auth", "setup", "instagram", "--app-id", "123", "--app-secret", "s3cret")
	if code != 0 {
		t.Fatalf("setup ig: %d %s", code, stderr)
	}
	stdout, stderr, code = run(t, home, "config", "show", "--json")
	if code != 0 {
		t.Fatalf("config show: %d %s", code, stderr)
	}
	if !strings.Contains(stdout, `"AppID": "123"`) && !strings.Contains(stdout, `"app_id"`) {
		t.Fatalf("expected app id in config: %s", stdout)
	}
}
