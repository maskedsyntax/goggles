package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type API interface {
	Me(context.Context) (string, string, error)
	UploadImage(context.Context, string, string) (string, error)
	UploadVideo(context.Context, string, string) (string, error)
	CreatePost(context.Context, string, string, string, string, []string) (string, error)
	PostStatus(context.Context, string) (string, error)
}

type Client struct {
	BaseURL, Token, Version string
	HTTP                    *http.Client
}

func NewClient(base, token, version string, httpClient *http.Client) *Client {
	if base == "" {
		base = "https://api.linkedin.com"
	}
	if version == "" {
		version = "202609"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{strings.TrimRight(base, "/"), token, version, httpClient}
}
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Linkedin-Version", c.Version)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, apperr.WrapRetryable(apperr.LinkedInRequestFailed, "LinkedIn request failed", err)
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		e := apperr.New(apperr.LinkedInRequestFailed, fmt.Sprintf("LinkedIn HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b))))
		e.Retryable = res.StatusCode == 429 || res.StatusCode >= 500
		if res.StatusCode == 401 {
			e.Code = apperr.AuthExpired
		}
		if res.StatusCode == 403 {
			e.Code = apperr.PermissionMissing
		}
		return nil, e
	}
	return res, nil
}
func (c *Client) Me(ctx context.Context) (string, string, error) {
	res, err := c.request(ctx, "GET", "/v2/userinfo", nil)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	var v struct {
		Sub  string `json:"sub"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", "", err
	}
	if v.Sub == "" {
		return "", "", apperr.New(apperr.AuthRequired, "LinkedIn userinfo has no member id")
	}
	return v.Sub, v.Name, nil
}
func (c *Client) json(ctx context.Context, path string, body any, out any) error {
	res, err := c.request(ctx, "POST", path, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
func (c *Client) upload(ctx context.Context, uploadURL, path string, start, end int64, authorize bool) (string, error) {
	u, err := url.Parse(uploadURL)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "www.linkedin.com" && u.Hostname() != "www.linkedin-ei.com") {
		return "", apperr.New(apperr.LinkedInUploadFailed, "LinkedIn returned an invalid upload URL")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, io.LimitReader(f, end-start+1))
	if err != nil {
		return "", err
	}
	req.ContentLength = end - start + 1
	req.Header.Set("Content-Type", "application/octet-stream")
	if authorize {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", apperr.WrapRetryable(apperr.LinkedInUploadFailed, "LinkedIn upload failed", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		e := apperr.New(apperr.LinkedInUploadFailed, fmt.Sprintf("LinkedIn upload HTTP %d", res.StatusCode))
		e.Retryable = res.StatusCode == 429 || res.StatusCode >= 500
		return "", e
	}
	return strings.Trim(res.Header.Get("ETag"), "\""), nil
}
func (c *Client) UploadImage(ctx context.Context, owner, path string) (string, error) {
	var v struct {
		Value struct {
			UploadURL string `json:"uploadUrl"`
			Image     string `json:"image"`
		} `json:"value"`
	}
	if err := c.json(ctx, "/rest/images?action=initializeUpload", map[string]any{"initializeUploadRequest": map[string]any{"owner": owner}}, &v); err != nil {
		return "", err
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() == 0 || v.Value.UploadURL == "" || v.Value.Image == "" {
		return "", apperr.Invalid("invalid LinkedIn image upload")
	}
	if _, err := c.upload(ctx, v.Value.UploadURL, path, 0, st.Size()-1, true); err != nil {
		return "", err
	}
	return v.Value.Image, nil
}
func (c *Client) UploadVideo(ctx context.Context, owner, path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() == 0 {
		return "", apperr.Invalid("video is empty")
	}
	var v struct {
		Value struct {
			Video        string `json:"video"`
			UploadToken  string `json:"uploadToken"`
			Instructions []struct {
				UploadURL string `json:"uploadUrl"`
				First     int64  `json:"firstByte"`
				Last      int64  `json:"lastByte"`
			} `json:"uploadInstructions"`
		} `json:"value"`
	}
	body := map[string]any{"initializeUploadRequest": map[string]any{"owner": owner, "fileSizeBytes": st.Size(), "uploadCaptions": false, "uploadThumbnail": false}}
	if err := c.json(ctx, "/rest/videos?action=initializeUpload", body, &v); err != nil {
		return "", err
	}
	if v.Value.Video == "" || len(v.Value.Instructions) == 0 {
		return "", apperr.New(apperr.LinkedInUploadFailed, "LinkedIn returned no video upload instructions")
	}
	ids := make([]string, 0, len(v.Value.Instructions))
	for _, part := range v.Value.Instructions {
		if part.UploadURL == "" || part.First < 0 || part.Last < part.First || part.Last >= st.Size() {
			return "", apperr.New(apperr.LinkedInUploadFailed, "invalid video upload byte range")
		}
		id, err := c.upload(ctx, part.UploadURL, path, part.First, part.Last, false)
		if err != nil {
			return "", err
		}
		if id == "" {
			return "", apperr.New(apperr.LinkedInUploadFailed, "video part response has no ETag")
		}
		ids = append(ids, id)
	}
	final := map[string]any{"finalizeUploadRequest": map[string]any{"video": v.Value.Video, "uploadToken": v.Value.UploadToken, "uploadedPartIds": ids}}
	if err := c.json(ctx, "/rest/videos?action=finalizeUpload", final, nil); err != nil {
		return "", err
	}
	if err := c.waitVideoReady(ctx, v.Value.Video); err != nil {
		return "", err
	}
	return v.Value.Video, nil
}

func (c *Client) waitVideoReady(ctx context.Context, id string) error {
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	for {
		res, err := c.request(ctx, "GET", "/rest/videos/"+url.PathEscape(id), nil)
		if err != nil {
			return err
		}
		var v struct {
			Status string `json:"status"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v)
		res.Body.Close()
		if err != nil {
			return err
		}
		switch v.Status {
		case "AVAILABLE":
			return nil
		case "PROCESSING_FAILED":
			return apperr.New(apperr.LinkedInUploadFailed, "LinkedIn video processing failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			e := apperr.New(apperr.JobTimeout, "timed out waiting for LinkedIn video processing")
			e.Retryable = true
			return e
		case <-time.After(5 * time.Second):
		}
	}
}
func (c *Client) CreatePost(ctx context.Context, owner, commentary, title, mediaURN string, images []string) (string, error) {
	body := map[string]any{"author": owner, "commentary": commentary, "visibility": "PUBLIC", "distribution": map[string]any{"feedDistribution": "MAIN_FEED", "targetEntities": []any{}, "thirdPartyDistributionChannels": []any{}}, "lifecycleState": "PUBLISHED", "isReshareDisabledByAuthor": false}
	if mediaURN != "" {
		m := map[string]any{"id": mediaURN}
		if strings.HasPrefix(mediaURN, "urn:li:video:") && title != "" {
			m["title"] = title
		}
		body["content"] = map[string]any{"media": m}
	}
	if len(images) > 0 {
		list := make([]map[string]string, 0, len(images))
		for _, id := range images {
			list = append(list, map[string]string{"id": id})
		}
		body["content"] = map[string]any{"multiImage": map[string]any{"images": list}}
	}
	res, err := c.request(ctx, "POST", "/rest/posts", body)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	id := res.Header.Get("x-restli-id")
	if id == "" {
		return "", apperr.New(apperr.LinkedInRequestFailed, "LinkedIn created a post without returning its ID")
	}
	return id, nil
}
func (c *Client) PostStatus(ctx context.Context, id string) (string, error) {
	res, err := c.request(ctx, "GET", "/rest/posts/"+id, nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var v struct {
		LifecycleState string `json:"lifecycleState"`
	}
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.LifecycleState, nil
}
