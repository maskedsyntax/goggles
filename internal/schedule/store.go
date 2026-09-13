package schedule

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/id"
	"github.com/maskedsyntax/goggles/internal/profile"
)

type Slot struct {
	ID            string  `json:"id"`
	DestinationID string  `json:"destination_id"`
	Destination   string  `json:"destination,omitempty"`
	TimeOfDay     string  `json:"time_of_day"`
	Timezone      string  `json:"timezone"`
	Enabled       bool    `json:"enabled"`
	LastFiredAt   *string `json:"last_fired_at,omitempty"`
}

func Set(ctx context.Context, sqlDB *sql.DB, destAlias string, times []string, tz string) ([]Slot, error) {
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return nil, err
	}
	if tz == "" {
		tz = dest.Timezone
	}
	if tz == "" {
		tz = "UTC"
	}
	if _, err := LoadTZ(tz); err != nil {
		return nil, err
	}
	norms := make([]string, 0, len(times))
	seen := map[string]struct{}{}
	for _, t := range times {
		_, _, n, err := ParseClock(t)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		norms = append(norms, n)
	}
	now := db.Now()
	err = db.WithTx(ctx, sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM schedules WHERE destination_id = ?`, dest.ID); err != nil {
			return apperr.Wrap(apperr.DatabaseError, "cannot clear schedule", err)
		}
		for _, n := range norms {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO schedules (id, destination_id, time_of_day, timezone, enabled, created_at, updated_at)
				VALUES (?, ?, ?, ?, 1, ?, ?)`, id.New(id.Schedule), dest.ID, n, tz, now, now); err != nil {
				return apperr.Wrap(apperr.DatabaseError, "cannot insert schedule", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return List(ctx, sqlDB, destAlias)
}

func SetProfile(ctx context.Context, sqlDB *sql.DB, profileName string, times []string, tz string) (map[string][]Slot, error) {
	view, err := profile.Show(ctx, sqlDB, profileName)
	if err != nil {
		return nil, err
	}
	out := map[string][]Slot{}
	for _, d := range view.Destinations {
		slots, err := Set(ctx, sqlDB, d.Alias, times, firstTZ(tz, d.Timezone))
		if err != nil {
			return out, err
		}
		out[d.Alias] = slots
	}
	return out, nil
}

func List(ctx context.Context, sqlDB *sql.DB, destAlias string) ([]Slot, error) {
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return nil, err
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT s.id, s.destination_id, d.alias, s.time_of_day, s.timezone, s.enabled, s.last_fired_at
		FROM schedules s JOIN destinations d ON d.id = s.destination_id
		WHERE s.destination_id = ?
		ORDER BY s.time_of_day`, dest.ID)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list schedules", err)
	}
	defer rows.Close()
	var out []Slot
	for rows.Next() {
		slot, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, slot)
	}
	return out, rows.Err()
}

func ListEnabled(ctx context.Context, sqlDB *sql.DB) ([]Slot, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT s.id, s.destination_id, d.alias, s.time_of_day, s.timezone, s.enabled, s.last_fired_at
		FROM schedules s JOIN destinations d ON d.id = s.destination_id
		WHERE s.enabled = 1 AND d.enabled = 1
		ORDER BY d.alias, s.time_of_day`)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list schedules", err)
	}
	defer rows.Close()
	var out []Slot
	for rows.Next() {
		slot, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, slot)
	}
	return out, rows.Err()
}

func Clear(ctx context.Context, sqlDB *sql.DB, destAlias string) error {
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return err
	}
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM schedules WHERE destination_id = ?`, dest.ID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot clear schedule", err)
	}
	return nil
}

func SetEnabled(ctx context.Context, sqlDB *sql.DB, destAlias string, enabled bool) error {
	dest, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias)
	if err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	_, err = sqlDB.ExecContext(ctx, `UPDATE schedules SET enabled = ?, updated_at = ? WHERE destination_id = ?`, v, db.Now(), dest.ID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot update schedule", err)
	}
	return nil
}

func MarkFired(ctx context.Context, sqlDB *sql.DB, slotID string, at time.Time) error {
	_, err := sqlDB.ExecContext(ctx, `UPDATE schedules SET last_fired_at = ?, updated_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), db.Now(), slotID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot mark schedule fired", err)
	}
	return nil
}

func scanSlot(row interface{ Scan(dest ...any) error }) (Slot, error) {
	var s Slot
	var last sql.NullString
	var enabled int
	if err := row.Scan(&s.ID, &s.DestinationID, &s.Destination, &s.TimeOfDay, &s.Timezone, &enabled, &last); err != nil {
		return Slot{}, apperr.Wrap(apperr.DatabaseError, "cannot scan schedule", err)
	}
	s.Enabled = enabled == 1
	if last.Valid {
		v := last.String
		s.LastFiredAt = &v
	}
	return s, nil
}

func firstTZ(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "UTC"
}
