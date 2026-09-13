package storage

import "github.com/maskedsyntax/goggles/internal/apperr"

func errNoPublicURL() error {
	return apperr.New(apperr.R2URLUnavailable, "r2.public_base_url is missing or invalid")
}

func errNotConfigured() error {
	return apperr.New(apperr.ConfigMissing, "R2 is not configured (account_id, bucket, public_base_url)")
}

func errNoCredentials() error {
	return apperr.New(apperr.AuthRequired, "R2 credentials are missing; run goggles storage credentials")
}
