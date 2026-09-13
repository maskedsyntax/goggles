package profile

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/id"
	"github.com/maskedsyntax/goggles/internal/platform"
)

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type View struct {
	Profile      Profile               `json:"profile"`
	Destinations []account.Destination `json:"destinations"`
}

func Create(ctx context.Context, sqlDB *sql.DB, name string) (Profile, error) {
	if !nameRE.MatchString(name) {
		return Profile{}, apperr.Invalid("profile name must start with a letter or digit and contain only letters, digits, _ or -")
	}
	now := db.Now()
	p := Profile{
		ID:        id.New(id.Profile),
		Name:      name,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO profiles (id, name, enabled, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?)`, p.ID, p.Name, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Profile{}, apperr.New(apperr.AlreadyExists, "profile already exists")
		}
		return Profile{}, apperr.Wrap(apperr.DatabaseError, "cannot create profile", err)
	}
	return p, nil
}

func List(ctx context.Context, sqlDB *sql.DB) ([]Profile, error) {
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, name, enabled, created_at, updated_at FROM profiles ORDER BY name`)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list profiles", err)
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, apperr.Wrap(apperr.DatabaseError, "cannot scan profile", err)
		}
		p.Enabled = enabled == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func GetByName(ctx context.Context, sqlDB *sql.DB, name string) (Profile, error) {
	var p Profile
	var enabled int
	err := sqlDB.QueryRowContext(ctx, `SELECT id, name, enabled, created_at, updated_at FROM profiles WHERE name = ?`, name).
		Scan(&p.ID, &p.Name, &enabled, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, apperr.New(apperr.ProfileNotFound, name)
	}
	if err != nil {
		return Profile{}, apperr.Wrap(apperr.DatabaseError, "cannot load profile", err)
	}
	p.Enabled = enabled == 1
	return p, nil
}

func Show(ctx context.Context, sqlDB *sql.DB, name string) (View, error) {
	p, err := GetByName(ctx, sqlDB, name)
	if err != nil {
		return View{}, err
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT
			d.id, d.account_id, d.platform, d.alias, d.external_id, d.external_username, d.external_title,
			d.timezone, d.enabled, d.metadata_json, d.created_at, d.updated_at
		FROM profile_destinations pd
		JOIN destinations d ON d.id = pd.destination_id
		WHERE pd.profile_id = ?
		ORDER BY d.alias`, p.ID)
	if err != nil {
		return View{}, apperr.Wrap(apperr.DatabaseError, "cannot list profile destinations", err)
	}
	defer rows.Close()
	view := View{Profile: p}
	for rows.Next() {
		var d account.Destination
		var extID, extUser, extTitle, tz, meta sql.NullString
		var enabled int
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Platform, &d.Alias, &extID, &extUser, &extTitle, &tz, &enabled, &meta, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return View{}, apperr.Wrap(apperr.DatabaseError, "cannot scan destination", err)
		}
		d.ExternalID = extID.String
		d.ExternalUsername = extUser.String
		d.ExternalTitle = extTitle.String
		d.Timezone = tz.String
		d.Enabled = enabled == 1
		d.MetadataJSON = meta.String
		view.Destinations = append(view.Destinations, d)
	}
	return view, rows.Err()
}

func AddDestination(ctx context.Context, sqlDB *sql.DB, profileName, destAlias string) error {
	p, err := GetByName(ctx, sqlDB, profileName)
	if err != nil {
		return err
	}
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return err
	}
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO profile_destinations (profile_id, destination_id, enabled, created_at)
		VALUES (?, ?, 1, ?)`, p.ID, dest.ID, db.Now())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return apperr.New(apperr.AlreadyExists, "destination already attached to profile")
		}
		return apperr.Wrap(apperr.DatabaseError, "cannot attach destination", err)
	}
	return nil
}

func RemoveDestination(ctx context.Context, sqlDB *sql.DB, profileName, destAlias string) error {
	p, err := GetByName(ctx, sqlDB, profileName)
	if err != nil {
		return err
	}
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return err
	}
	res, err := sqlDB.ExecContext(ctx, `DELETE FROM profile_destinations WHERE profile_id = ? AND destination_id = ?`, p.ID, dest.ID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot detach destination", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperr.New(apperr.DestinationNotFound, destAlias+" is not attached to "+profileName)
	}
	return nil
}

func Remove(ctx context.Context, sqlDB *sql.DB, name string) error {
	p, err := GetByName(ctx, sqlDB, name)
	if err != nil {
		return err
	}
	if _, err := sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, p.ID); err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot remove profile", err)
	}
	return nil
}

func DestinationsForPlatforms(view View, platforms []platform.Platform) []account.Destination {
	if len(platforms) == 0 {
		return view.Destinations
	}
	allow := map[platform.Platform]struct{}{}
	for _, p := range platforms {
		allow[p] = struct{}{}
	}
	var out []account.Destination
	for _, d := range view.Destinations {
		if _, ok := allow[d.Platform]; ok {
			out = append(out, d)
		}
	}
	return out
}
