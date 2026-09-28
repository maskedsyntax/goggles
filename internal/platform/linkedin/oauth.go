package linkedin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/oauth"
)

const authorizationURL = "https://www.linkedin.com/oauth/v2/authorization"
const tokenURL = "https://www.linkedin.com/oauth/v2/accessToken"

func AuthorizeURL(clientID, redirect, state string) string {
	return oauth.AuthURL(authorizationURL, url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect}, "state": {state}, "scope": {"openid profile w_member_social"}})
}

type Token struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func Exchange(ctx context.Context, client *http.Client, clientID, secret, redirect, code string) (Token, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "client_id": {clientID}, "client_secret": {secret}}
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return Token{}, apperr.Wrap(apperr.AuthRequired, "LinkedIn token exchange failed", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Token{}, apperr.New(apperr.AuthRequired, "LinkedIn token exchange returned "+res.Status)
	}
	var tok Token
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil || tok.AccessToken == "" {
		return Token{}, apperr.New(apperr.AuthRequired, "invalid LinkedIn token response")
	}
	return tok, nil
}
