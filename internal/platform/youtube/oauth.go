package youtube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/oauth"
)

const (
	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"
	ScopeUpload  = "https://www.googleapis.com/auth/youtube.upload"
	ScopeRead    = "https://www.googleapis.com/auth/youtube.readonly"
)

var tokenEndpoint = TokenURL

type PKCE struct {
	Verifier  string
	Challenge string
}

func NewPKCE() PKCE {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}
}

func AuthorizeURLWith(clientID, redirect, state, challenge string) string {
	q := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {ScopeUpload + " " + ScopeRead},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return oauth.AuthURL(AuthorizeURL, q)
}

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func Exchange(ctx context.Context, httpClient *http.Client, clientID, clientSecret, redirect, code, verifier string) (Token, error) {
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
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := httpClient.Do(req)
	if err != nil {
		return Token{}, apperr.Wrap(apperr.GoogleAuthFailed, "Google token exchange failed", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return Token{}, apperr.New(apperr.GoogleAuthFailed, "Google token exchange HTTP "+res.Status)
	}
	var tok Token
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return Token{}, apperr.New(apperr.GoogleAuthFailed, "cannot parse Google token response")
	}
	return tok, nil
}
