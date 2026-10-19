package linkedin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}
}
func TestVideoMultipartUpload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, []byte("abcdefgh"), 0600); err != nil {
		t.Fatal(err)
	}
	var parts []string
	finalized := false
	client := NewClient("https://api.linkedin.com", "token", "202609", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/rest/videos" && r.URL.Query().Get("action") == "initializeUpload":
			if r.Header.Get("Linkedin-Version") != "202609" || r.Header.Get("Authorization") != "Bearer token" {
				t.Fatal("missing API headers")
			}
			return response(200, `{"value":{"video":"urn:li:video:1","uploadToken":"session","uploadInstructions":[{"uploadUrl":"https://www.linkedin.com/upload/1","firstByte":0,"lastByte":3},{"uploadUrl":"https://www.linkedin.com/upload/2","firstByte":4,"lastByte":7}]}}`, nil), nil
		case strings.HasPrefix(r.URL.Path, "/upload/"):
			if r.Header.Get("Authorization") != "" {
				t.Fatal("video upload must not send bearer token")
			}
			b, _ := io.ReadAll(r.Body)
			parts = append(parts, string(b))
			h := make(http.Header)
			h.Set("ETag", `"etag-`+r.URL.Path[len("/upload/"):]+`"`)
			return response(200, "", h), nil
		case r.URL.Path == "/rest/videos" && r.URL.Query().Get("action") == "finalizeUpload":
			var v struct {
				Request struct {
					Parts []string `json:"uploadedPartIds"`
				} `json:"finalizeUploadRequest"`
			}
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				t.Fatal(err)
			}
			if strings.Join(v.Request.Parts, ",") != "etag-1,etag-2" {
				t.Fatalf("parts = %v", v.Request.Parts)
			}
			finalized = true
			return response(200, "{}", nil), nil
		case r.URL.EscapedPath() == "/rest/videos/urn%3Ali%3Avideo%3A1":
			return response(200, `{"status":"AVAILABLE"}`, nil), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	})})
	id, err := client.UploadVideo(context.Background(), "urn:li:person:42", path)
	if err != nil {
		t.Fatal(err)
	}
	if id != "urn:li:video:1" || strings.Join(parts, ",") != "abcd,efgh" || !finalized {
		t.Fatalf("id=%s parts=%v finalized=%t", id, parts, finalized)
	}
}
func TestTextPostRequest(t *testing.T) {
	c := NewClient("https://api.linkedin.com", "token", "202609", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/rest/posts" {
			t.Fatal(r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["author"] != "urn:li:person:42" || body["commentary"] != "hello" || body["content"] != nil {
			t.Fatalf("body=%v", body)
		}
		h := make(http.Header)
		h.Set("x-restli-id", "urn:li:share:123")
		return response(201, "", h), nil
	})})
	id, err := c.CreatePost(context.Background(), "urn:li:person:42", "hello", "", "", nil)
	if err != nil || id != "urn:li:share:123" {
		t.Fatalf("id=%s err=%v", id, err)
	}
}

func TestPostStatusEncodesURN(t *testing.T) {
	c := NewClient("https://api.linkedin.com", "token", "202609", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.EscapedPath(); got != "/rest/posts/urn%3Ali%3Ashare%3A123" {
			t.Fatalf("path = %s", got)
		}
		return response(200, `{"lifecycleState":"PUBLISHED"}`, nil), nil
	})})
	status, err := c.PostStatus(context.Background(), "urn:li:share:123")
	if err != nil || status != "PUBLISHED" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestImageUpload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("image-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	uploaded := false
	c := NewClient("https://api.linkedin.com", "token", "202609", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/rest/images" {
			return response(200, `{"value":{"uploadUrl":"https://www.linkedin.com/upload/image","image":"urn:li:image:1"}}`, nil), nil
		}
		if r.URL.Path == "/upload/image" {
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Fatal("image upload must be authorized")
			}
			b, _ := io.ReadAll(r.Body)
			if string(b) != "image-bytes" {
				t.Fatalf("body=%q", b)
			}
			uploaded = true
			return response(201, "", nil), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	})})
	id, err := c.UploadImage(context.Background(), "urn:li:person:42", path)
	if err != nil || id != "urn:li:image:1" || !uploaded {
		t.Fatalf("id=%s uploaded=%t err=%v", id, uploaded, err)
	}
}
