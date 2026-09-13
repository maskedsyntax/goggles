package account

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/id"
	"github.com/maskedsyntax/goggles/internal/platform"
)

var aliasRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

type Account struct {
	ID            string            `json:"id"`
	Platform      platform.Platform `json:"platform"`
	Alias         string            `json:"alias"`
	CredentialRef string            `json:"credential_ref,omitempty"`
	DisplayName   string            `json:"display_name,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
}

type Destination struct {
	ID               string            `json:"id"`
	AccountID        string            `json:"account_id"`
	Platform         platform.Platform `json:"platform"`
	Alias            string            `json:"alias"`
	ExternalID       string            `json:"external_id,omitempty"`
	ExternalUsername string            `json:"external_username,omitempty"`
	ExternalTitle    string            `json:"external_title,omitempty"`
	Timezone         string            `json:"timezone,omitempty"`
	Enabled          bool              `json:"enabled"`
	MetadataJSON     string            `json:"-"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

type AddInput struct {
	Platform     platform.Platform
	Alias        string
	DisplayName  string
	Username     string
	ChannelID    string
	ChannelTitle string
	UserID       string
	Timezone     string
	Enabled      bool
}

type Pair struct {
	Account     Account     `json:"account"`
	Destination Destination `json:"destination"`
}

func ValidateAlias(alias string) error {
	if !aliasRE.MatchString(alias) {
		return apperr.Invalid("alias must start with a letter or digit and contain only letters, digits, _ or -")
	}
	return nil
}

func Add(ctx context.Context, sqlDB *sql.DB, in AddInput) (Pair, error) {
	if err := ValidateAlias(in.Alias); err != nil {
		return Pair{}, err
	}
	if in.Platform == platform.YouTube && strings.TrimSpace(in.ChannelID) == "" {
		return Pair{}, apperr.New(apperr.YouTubeChannelRequired, "pass --channel-id; goggles never selects a YouTube channel silently")
	}

	now := db.Now()
	acc := Account{
		ID:          id.New(id.Account),
		Platform:    in.Platform,
		Alias:       in.Alias,
		DisplayName: firstNonEmpty(in.DisplayName, in.ChannelTitle, in.Username, in.Alias),
		Enabled:     in.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	dest := Destination{
		ID:               id.New(id.Destination),
		AccountID:        acc.ID,
		Platform:         in.Platform,
		Alias:            in.Alias,
		ExternalID:       strings.TrimSpace(in.ChannelID),
		ExternalUsername: strings.TrimPrefix(strings.TrimSpace(in.Username), "@"),
		ExternalTitle:    in.ChannelTitle,
		Timezone:         in.Timezone,
		Enabled:          in.Enabled,
		MetadataJSON:     "{}",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if dest.Platform == platform.Instagram {
		if strings.TrimSpace(in.UserID) != "" {
			dest.ExternalID = strings.TrimSpace(in.UserID)
		} else if dest.ExternalID == "" {
			dest.ExternalID = dest.ExternalUsername
		}
	}

	err := db.WithTx(ctx, sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO accounts (id, platform, alias, credential_ref, display_name, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			acc.ID, acc.Platform, acc.Alias, acc.CredentialRef, acc.DisplayName, boolToInt(acc.Enabled), acc.CreatedAt, acc.UpdatedAt,
		); err != nil {
			return mapUnique(err, "account alias already exists")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO destinations (
				id, account_id, platform, alias, external_id, external_username, external_title,
				timezone, enabled, metadata_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			dest.ID, dest.AccountID, dest.Platform, dest.Alias, nullIfEmpty(dest.ExternalID),
			nullIfEmpty(dest.ExternalUsername), nullIfEmpty(dest.ExternalTitle), nullIfEmpty(dest.Timezone),
			boolToInt(dest.Enabled), dest.MetadataJSON, dest.CreatedAt, dest.UpdatedAt,
		); err != nil {
			return mapUnique(err, "destination alias or YouTube channel already exists")
		}
		return nil
	})
	if err != nil {
		return Pair{}, err
	}
	return Pair{Account: acc, Destination: dest}, nil
}

func List(ctx context.Context, sqlDB *sql.DB) ([]Pair, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT
			a.id, a.platform, a.alias, a.credential_ref, a.display_name, a.enabled, a.created_at, a.updated_at,
			d.id, d.account_id, d.platform, d.alias, d.external_id, d.external_username, d.external_title,
			d.timezone, d.enabled, d.metadata_json, d.created_at, d.updated_at
		FROM accounts a
		JOIN destinations d ON d.account_id = a.id
		ORDER BY a.alias`)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list accounts", err)
	}
	defer rows.Close()
	var out []Pair
	for rows.Next() {
		p, err := scanPair(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func GetByAlias(ctx context.Context, sqlDB *sql.DB, alias string) (Pair, error) {
	row := sqlDB.QueryRowContext(ctx, `
		SELECT
			a.id, a.platform, a.alias, a.credential_ref, a.display_name, a.enabled, a.created_at, a.updated_at,
			d.id, d.account_id, d.platform, d.alias, d.external_id, d.external_username, d.external_title,
			d.timezone, d.enabled, d.metadata_json, d.created_at, d.updated_at
		FROM accounts a
		JOIN destinations d ON d.account_id = a.id
		WHERE a.alias = ? OR d.alias = ?`, alias, alias)
	p, err := scanPair(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Pair{}, apperr.New(apperr.AccountNotFound, alias)
	}
	if err != nil {
		return Pair{}, apperr.Wrap(apperr.DatabaseError, "cannot load account", err)
	}
	return p, nil
}

func GetDestinationByAlias(ctx context.Context, sqlDB *sql.DB, alias string) (Destination, error) {
	p, err := GetByAlias(ctx, sqlDB, alias)
	if err != nil {
		if e, ok := apperr.As(err); ok && e.Code == apperr.AccountNotFound {
			return Destination{}, apperr.New(apperr.DestinationNotFound, alias)
		}
		return Destination{}, err
	}
	return p.Destination, nil
}

func SetCredentialRef(ctx context.Context, sqlDB *sql.DB, accountID, ref string) error {
	_, err := sqlDB.ExecContext(ctx, `UPDATE accounts SET credential_ref = ?, updated_at = ? WHERE id = ?`, ref, db.Now(), accountID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot store credential ref", err)
	}
	return nil
}

func Remove(ctx context.Context, sqlDB *sql.DB, alias string) error {
	p, err := GetByAlias(ctx, sqlDB, alias)
	if err != nil {
		return err
	}
	if _, err := sqlDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, p.Account.ID); err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot remove account", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPair(row rowScanner) (Pair, error) {
	var (
		p                                        Pair
		cred, extID, extUser, extTitle, tz, meta sql.NullString
		accEnabled, destEnabled                  int
	)
	err := row.Scan(
		&p.Account.ID, &p.Account.Platform, &p.Account.Alias, &cred, &p.Account.DisplayName, &accEnabled, &p.Account.CreatedAt, &p.Account.UpdatedAt,
		&p.Destination.ID, &p.Destination.AccountID, &p.Destination.Platform, &p.Destination.Alias, &extID, &extUser, &extTitle,
		&tz, &destEnabled, &meta, &p.Destination.CreatedAt, &p.Destination.UpdatedAt,
	)
	if err != nil {
		return Pair{}, err
	}
	p.Account.CredentialRef = cred.String
	p.Account.Enabled = accEnabled == 1
	p.Destination.ExternalID = extID.String
	p.Destination.ExternalUsername = extUser.String
	p.Destination.ExternalTitle = extTitle.String
	p.Destination.Timezone = tz.String
	p.Destination.Enabled = destEnabled == 1
	p.Destination.MetadataJSON = meta.String
	return p, nil
}

func mapUnique(err error, message string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return apperr.New(apperr.AlreadyExists, message)
	}
	return apperr.Wrap(apperr.DatabaseError, "cannot write account", err)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
