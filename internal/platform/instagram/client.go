package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type API interface {
	CreateReel(ctx context.Context, igUserID, videoURL, caption string, shareToFeed bool) (string, error)
	CreateCarouselItem(ctx context.Context, igUserID, imageURL, videoURL string) (string, error)
	CreateCarousel(ctx context.Context, igUserID string, children []string, caption string, shareToFeed bool) (string, error)
	ContainerStatus(ctx context.Context, containerID string) (string, error)
	PublishContainer(ctx context.Context, igUserID, containerID string) (string, error)
	Me(ctx context.Context) (userID, username string, err error)
}

type Client struct {
	BaseURL   string
	Token     string
	HTTP      *http.Client
	UserAgent string
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: httpClient}
}

func GraphBase(host, version string) string {
	host = strings.TrimRight(host, "/")
	if host == "" {
		host = "https://graph.facebook.com"
	}
	if version == "" {
		version = "v25.0"
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return host + "/" + version
}

type Me struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

func (c *Client) Me(ctx context.Context) (string, string, error) {
	var me Me
	if err := c.get(ctx, "me", url.Values{"fields": {"user_id,id,username"}}, &me); err != nil {
		return "", "", err
	}
	id := me.UserID
	if id == "" {
		id = me.ID
	}
	if id == "" {
		return "", "", apperr.New(apperr.MetaRequestFailed, "Instagram user id missing from /me")
	}
	return id, me.Username, nil
}

func (c *Client) CreateReel(ctx context.Context, igUserID, videoURL, caption string, shareToFeed bool) (string, error) {
	form := url.Values{
		"media_type":    {"REELS"},
		"video_url":     {videoURL},
		"share_to_feed": {boolStr(shareToFeed)},
	}
	if caption != "" {
		form.Set("caption", caption)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, igUserID+"/media", form, &out); err != nil {
		return "", mapInstagram(err, apperr.InstagramContainerFailed)
	}
	if out.ID == "" {
		return "", apperr.New(apperr.InstagramContainerFailed, "container id missing")
	}
	return out.ID, nil
}

func (c *Client) CreateCarouselItem(ctx context.Context, igUserID, imageURL, videoURL string) (string, error) {
	form := url.Values{"is_carousel_item": {"true"}}
	switch {
	case strings.TrimSpace(videoURL) != "":
		form.Set("media_type", "VIDEO")
		form.Set("video_url", videoURL)
	case strings.TrimSpace(imageURL) != "":
		form.Set("image_url", imageURL)
	default:
		return "", apperr.Invalid("carousel item needs image_url or video_url")
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, igUserID+"/media", form, &out); err != nil {
		return "", mapInstagram(err, apperr.InstagramContainerFailed)
	}
	if out.ID == "" {
		return "", apperr.New(apperr.InstagramContainerFailed, "container id missing")
	}
	return out.ID, nil
}

func (c *Client) CreateCarousel(ctx context.Context, igUserID string, children []string, caption string, shareToFeed bool) (string, error) {
	if len(children) < 2 || len(children) > 10 {
		return "", apperr.Invalid("Instagram carousels need 2–10 items")
	}
	form := url.Values{
		"media_type": {"CAROUSEL"},
		"children":   {strings.Join(children, ",")},
	}
	if caption != "" {
		form.Set("caption", caption)
	}
	if shareToFeed {
		form.Set("share_to_feed", "true")
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, igUserID+"/media", form, &out); err != nil {
		return "", mapInstagram(err, apperr.InstagramContainerFailed)
	}
	if out.ID == "" {
		return "", apperr.New(apperr.InstagramContainerFailed, "container id missing")
	}
	return out.ID, nil
}

func (c *Client) ContainerStatus(ctx context.Context, containerID string) (string, error) {
	var out struct {
		StatusCode string `json:"status_code"`
		Status     string `json:"status"`
	}
	if err := c.get(ctx, containerID, url.Values{"fields": {"status_code"}}, &out); err != nil {
		return "", mapInstagram(err, apperr.InstagramProcessingFailed)
	}
	if out.StatusCode == "" {
		return out.Status, nil
	}
	return out.StatusCode, nil
}

func (c *Client) PublishContainer(ctx context.Context, igUserID, containerID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, igUserID+"/media_publish", url.Values{"creation_id": {containerID}}, &out); err != nil {
		return "", mapInstagram(err, apperr.InstagramPublishFailed)
	}
	if out.ID == "" {
		return "", apperr.New(apperr.InstagramPublishFailed, "media id missing")
	}
	return out.ID, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, dest any) error {
	if q == nil {
		q = url.Values{}
	}
	u := c.BaseURL + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+q.Encode(), nil)
	if err != nil {
		return apperr.Wrap(apperr.MetaRequestFailed, "cannot build request", err)
	}
	return c.do(req, dest)
}

func (c *Client) post(ctx context.Context, path string, form url.Values, dest any) error {
	u := c.BaseURL + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return apperr.Wrap(apperr.MetaRequestFailed, "cannot build request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, dest)
}

func (c *Client) do(req *http.Request, dest any) error {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return apperr.Wrap(apperr.MetaRequestFailed, "Meta request failed", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return apperr.Wrap(apperr.MetaRequestFailed, "cannot read Meta response", err)
	}
	if res.StatusCode >= 400 {
		return graphError(body, res.StatusCode)
	}
	if dest == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return apperr.Wrap(apperr.MetaRequestFailed, "cannot decode Meta response", err)
	}
	return nil
}

type graphErrBody struct {
	Error struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
		IsTransient  bool   `json:"is_transient"`
	} `json:"error"`
}

func graphError(body []byte, status int) error {
	var ge graphErrBody
	_ = json.Unmarshal(body, &ge)
	msg := strings.TrimSpace(ge.Error.Message)
	if msg == "" {
		msg = fmt.Sprintf("Meta HTTP %d", status)
	}
	code := apperr.MetaRequestFailed
	retryable := ge.Error.IsTransient || status == 429 || status >= 500
	switch ge.Error.Code {
	case 4, 17, 32, 613:
		code = apperr.MetaRateLimited
		retryable = true
	}
	err := apperr.New(code, msg)
	err.Retryable = retryable
	err.Details = map[string]any{"meta_code": ge.Error.Code, "meta_subcode": ge.Error.ErrorSubcode, "http_status": status}
	return err
}

func mapInstagram(err error, code apperr.Code) error {
	if err == nil {
		return nil
	}
	if e, ok := apperr.As(err); ok {
		if e.Code == apperr.MetaRateLimited {
			return err
		}
		if e.Code == apperr.MetaRequestFailed {
			wrapped := apperr.New(code, e.Message)
			wrapped.Retryable = e.Retryable
			wrapped.Details = e.Details
			wrapped.Err = err
			return wrapped
		}
	}
	return err
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
