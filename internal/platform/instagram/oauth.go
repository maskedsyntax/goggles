package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/oauth"
)

const (
	AuthorizeURL = "https://www.instagram.com/oauth/authorize"
	TokenURL     = "https://api.instagram.com/oauth/access_token"
	LongLivedURL = "https://graph.instagram.com/access_token"
	Scopes       = "instagram_business_basic,instagram_business_content_publish"
)

var (
	tokenEndpoint     = TokenURL
	longLivedEndpoint = LongLivedURL
)

func AuthorizeURLWith(clientID, redirect, state string) string {
	q := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {Scopes},
		"state":         {state},
	}
	return oauth.AuthURL(AuthorizeURL, q)
}

type Token struct {
	AccessToken string
	UserID      string
	ExpiresIn   int
}

func Exchange(ctx context.Context, httpClient *http.Client, clientID, clientSecret, redirect, code string) (Token, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirect},
		"code":          {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := httpClient.Do(req)
	if err != nil {
		return Token{}, apperr.Wrap(apperr.AuthRequired, "Instagram token exchange failed", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return Token{}, apperr.New(apperr.AuthRequired, "Instagram token exchange HTTP "+res.Status)
	}
	tok, err := parseShortToken(body)
	if err != nil {
		return Token{}, err
	}
	return tok, nil
}

func ExchangeLongLived(ctx context.Context, httpClient *http.Client, clientSecret, shortToken string) (Token, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	q := url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {clientSecret},
		"access_token":  {shortToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, longLivedEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return Token{}, err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return Token{}, apperr.Wrap(apperr.AuthRequired, "Instagram long-lived token exchange failed", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return Token{}, apperr.New(apperr.AuthRequired, "Instagram long-lived token HTTP "+res.Status)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return Token{}, apperr.New(apperr.AuthRequired, "cannot parse Instagram long-lived token")
	}
	return Token{AccessToken: out.AccessToken, ExpiresIn: out.ExpiresIn}, nil
}

func parseShortToken(body []byte) (Token, error) {
	var flat struct {
		AccessToken string `json:"access_token"`
		UserID      any    `json:"user_id"`
	}
	if err := json.Unmarshal(body, &flat); err == nil && flat.AccessToken != "" {
		return Token{AccessToken: flat.AccessToken, UserID: stringify(flat.UserID)}, nil
	}
	var wrapped struct {
		Data []struct {
			AccessToken string `json:"access_token"`
			UserID      any    `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Data) > 0 && wrapped.Data[0].AccessToken != "" {
		return Token{AccessToken: wrapped.Data[0].AccessToken, UserID: stringify(wrapped.Data[0].UserID)}, nil
	}
	return Token{}, apperr.New(apperr.AuthRequired, "cannot parse Instagram token response")
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}
