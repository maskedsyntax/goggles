package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type Channel struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type VideoMeta struct {
	Title       string
	Description string
	Tags        []string
	CategoryID  string
	Privacy     string
	PublishAt   string
	MadeForKids bool
}

type API interface {
	ListChannels(ctx context.Context) ([]Channel, error)
	Upload(ctx context.Context, path string, meta VideoMeta) (string, error)
	VideoStatus(ctx context.Context, videoID string) (string, error)
}

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://www.googleapis.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: httpClient}
}

func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	q := url.Values{"part": {"id,snippet"}, "mine": {"true"}, "maxResults": {"50"}}
	if err := c.get(ctx, "/youtube/v3/channels", q, &out); err != nil {
		return nil, err
	}
	chs := make([]Channel, 0, len(out.Items))
	for _, it := range out.Items {
		chs = append(chs, Channel{ID: it.ID, Title: it.Snippet.Title})
	}
	return chs, nil
}

func (c *Client) Upload(ctx context.Context, path string, meta VideoMeta) (string, error) {
	if strings.TrimSpace(meta.Title) == "" {
		return "", apperr.Invalid("YouTube title is required")
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperr.New(apperr.FileNotFound, path)
		}
		return "", apperr.Wrap(apperr.FileUnreadable, "cannot stat file", err)
	}
	body, err := json.Marshal(videoResource(meta))
	if err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "cannot encode video metadata", err)
	}
	session, err := c.startSession(ctx, body, st.Size())
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", apperr.Wrap(apperr.FileUnreadable, "cannot open file", err)
	}
	defer f.Close()
	return c.putFile(ctx, session, f, st.Size())
}

func (c *Client) VideoStatus(ctx context.Context, videoID string) (string, error) {
	var out struct {
		Items []struct {
			Status struct {
				UploadStatus string `json:"uploadStatus"`
			} `json:"status"`
		} `json:"items"`
	}
	q := url.Values{"part": {"status"}, "id": {videoID}}
	if err := c.get(ctx, "/youtube/v3/videos", q, &out); err != nil {
		return "", err
	}
	if len(out.Items) == 0 {
		return "", apperr.New(apperr.YouTubeProcessingFailed, "video not found")
	}
	return out.Items[0].Status.UploadStatus, nil
}

func (c *Client) startSession(ctx context.Context, metadata []byte, size int64) (string, error) {
	u := c.BaseURL + "/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(metadata))
	if err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "cannot build upload session", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("X-Upload-Content-Type", "video/*")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "YouTube session request failed", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return "", ytError(b, res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		return "", apperr.New(apperr.YouTubeUploadFailed, "missing resumable upload Location")
	}
	ref, err := res.Request.URL.Parse(loc)
	if err != nil {
		return loc, nil
	}
	return ref.String(), nil
}

func (c *Client) putFile(ctx context.Context, session string, r io.ReadSeeker, size int64) (string, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "cannot rewind file", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, session, r)
	if err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "cannot build upload PUT", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "video/*")
	req.ContentLength = size
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", apperr.Wrap(apperr.YouTubeUploadFailed, "YouTube upload failed", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 400 {
		return "", ytError(b, res.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.ID == "" {
		return "", apperr.New(apperr.YouTubeUploadFailed, "YouTube upload response missing video id")
	}
	return out.ID, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, dest any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return apperr.Wrap(apperr.GoogleAuthFailed, "cannot build request", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return apperr.Wrap(apperr.GoogleAuthFailed, "YouTube request failed", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return ytError(b, res.StatusCode)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return apperr.Wrap(apperr.YouTubeUploadFailed, "cannot decode YouTube response", err)
	}
	return nil
}

func videoResource(meta VideoMeta) map[string]any {
	cat := meta.CategoryID
	if cat == "" {
		cat = "22"
	}
	privacy := strings.ToLower(strings.TrimSpace(meta.Privacy))
	if privacy == "" {
		privacy = "public"
	}
	status := map[string]any{
		"privacyStatus":           privacy,
		"selfDeclaredMadeForKids": meta.MadeForKids,
	}
	if strings.TrimSpace(meta.PublishAt) != "" {
		status["privacyStatus"] = "private"
		status["publishAt"] = meta.PublishAt
	}
	snippet := map[string]any{
		"title":       meta.Title,
		"description": meta.Description,
		"categoryId":  cat,
	}
	if len(meta.Tags) > 0 {
		snippet["tags"] = meta.Tags
	}
	return map[string]any{"snippet": snippet, "status": status}
}

type ytErrBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

func ytError(body []byte, status int) error {
	var ge ytErrBody
	_ = json.Unmarshal(body, &ge)
	msg := strings.TrimSpace(ge.Error.Message)
	if msg == "" {
		msg = fmt.Sprintf("YouTube HTTP %d", status)
	}
	code := apperr.YouTubeUploadFailed
	retryable := status == 429 || status >= 500
	reason := ""
	if len(ge.Error.Errors) > 0 {
		reason = ge.Error.Errors[0].Reason
	}
	switch {
	case status == 401 || reason == "authError":
		code = apperr.GoogleAuthFailed
	case reason == "quotaExceeded" || reason == "dailyLimitExceeded":
		code = apperr.YouTubeQuotaExceeded
	case reason == "rateLimitExceeded" || reason == "userRateLimitExceeded" || status == 429:
		code = apperr.YouTubeRateLimited
		retryable = true
	}
	err := apperr.New(code, msg)
	err.Retryable = retryable
	err.Details = map[string]any{"http_status": status, "reason": reason}
	return err
}

func FindChannel(channels []Channel, id string) (Channel, bool) {
	for _, ch := range channels {
		if ch.ID == id {
			return ch, true
		}
	}
	return Channel{}, false
}
