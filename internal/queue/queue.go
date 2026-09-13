package queue

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/db"
	"github.com/maskedsyntax/goggles/internal/id"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/profile"
)

const (
	Ready     = "ready"
	Reserved  = "reserved"
	Published = "published"
	Failed    = "failed"
	Skipped   = "skipped"
	Cancelled = "cancelled"
)

type Item struct {
	ID            string `json:"id"`
	ProfileID     string `json:"profile_id,omitempty"`
	DestinationID string `json:"destination_id,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Destination   string `json:"destination,omitempty"`
	FilePath      string `json:"file_path"`
	ContentHash   string `json:"content_hash"`
	Status        string `json:"status"`
	Position      int    `json:"position"`
	ScheduledFor  string `json:"scheduled_for,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type AddInput struct {
	Path         string
	Profile      string
	Destination  string
	ScheduledFor string
}

func Add(ctx context.Context, sqlDB *sql.DB, in AddInput) (Item, error) {
	if in.Profile != "" && in.Destination != "" {
		return Item{}, apperr.Invalid("use --profile or --destination, not both")
	}
	if in.Profile == "" && in.Destination == "" {
		return Item{}, apperr.Invalid("pass --profile or --destination")
	}
	st, err := os.Stat(in.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return Item{}, apperr.New(apperr.FileNotFound, in.Path)
		}
		return Item{}, apperr.Wrap(apperr.FileUnreadable, "cannot read file", err)
	}
	if st.IsDir() {
		return Item{}, apperr.Invalid("path is a directory; use queue fill")
	}
	abs, err := filepath.Abs(in.Path)
	if err != nil {
		abs = in.Path
	}
	hash, err := media.HashFile(abs)
	if err != nil {
		return Item{}, err
	}

	var profileID, destID any
	profileName, destAlias := "", ""
	if in.Profile != "" {
		p, err := profile.GetByName(ctx, sqlDB, in.Profile)
		if err != nil {
			return Item{}, err
		}
		profileID = p.ID
		profileName = p.Name
	} else {
		d, err := account.GetDestinationByAlias(ctx, sqlDB, in.Destination)
		if err != nil {
			return Item{}, err
		}
		destID = d.ID
		destAlias = d.Alias
	}

	pos, err := nextPosition(ctx, sqlDB, profileID, destID)
	if err != nil {
		return Item{}, err
	}
	now := db.Now()
	item := Item{
		ID:           id.New(id.Queue),
		Profile:      profileName,
		Destination:  destAlias,
		FilePath:     abs,
		ContentHash:  hash,
		Status:       Ready,
		Position:     pos,
		ScheduledFor: in.ScheduledFor,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if profileID != nil {
		item.ProfileID = profileID.(string)
	}
	if destID != nil {
		item.DestinationID = destID.(string)
	}
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO queue_items (
			id, profile_id, destination_id, file_path, content_hash,
			generic_metadata_json, platform_metadata_json, status, position, scheduled_for, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, '{}', '{}', ?, ?, ?, ?, ?)`,
		item.ID, profileID, destID, item.FilePath, item.ContentHash, item.Status, item.Position,
		nullIfEmpty(item.ScheduledFor), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return Item{}, apperr.Wrap(apperr.DatabaseError, "cannot enqueue", err)
	}
	return item, nil
}

func Fill(ctx context.Context, sqlDB *sql.DB, dir string, in AddInput) ([]Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperr.New(apperr.FileNotFound, dir)
		}
		return nil, apperr.Wrap(apperr.FileUnreadable, "cannot read directory", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".mp4" || ext == ".mov" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, apperr.New(apperr.QueueEmpty, "no mp4/mov files in "+dir)
	}
	var out []Item
	for _, f := range files {
		in.Path = f
		item, err := Add(ctx, sqlDB, in)
		if err != nil {
			return out, err
		}
		out = append(out, item)
	}
	return out, nil
}

type ListFilter struct {
	Profile     string
	Destination string
	Status      string
	DueBefore   string
}

func List(ctx context.Context, sqlDB *sql.DB, f ListFilter) ([]Item, error) {
	q := `
		SELECT q.id, q.profile_id, q.destination_id, p.name, d.alias, q.file_path, q.content_hash,
			q.status, q.position, q.scheduled_for, q.created_at, q.updated_at
		FROM queue_items q
		LEFT JOIN profiles p ON p.id = q.profile_id
		LEFT JOIN destinations d ON d.id = q.destination_id
		WHERE 1=1`
	var args []any
	if f.Profile != "" {
		q += ` AND p.name = ?`
		args = append(args, f.Profile)
	}
	if f.Destination != "" {
		q += ` AND d.alias = ?`
		args = append(args, f.Destination)
	}
	if f.Status != "" {
		q += ` AND q.status = ?`
		args = append(args, f.Status)
	}
	if f.DueBefore != "" {
		q += ` AND q.scheduled_for IS NOT NULL AND q.scheduled_for <= ?`
		args = append(args, f.DueBefore)
	}
	q += ` ORDER BY q.position, q.created_at`
	rows, err := sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list queue", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func Remove(ctx context.Context, sqlDB *sql.DB, itemID string) error {
	res, err := sqlDB.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ? AND status IN (?, ?, ?)`, itemID, Ready, Failed, Cancelled)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot remove queue item", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperr.New(apperr.QueueEmpty, "queue item not found or not removable: "+itemID)
	}
	return nil
}

func Clear(ctx context.Context, sqlDB *sql.DB, f ListFilter) (int, error) {
	items, err := List(ctx, sqlDB, f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		if it.Status != Ready && it.Status != Failed && it.Status != Cancelled {
			continue
		}
		if err := Remove(ctx, sqlDB, it.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func SetPaused(ctx context.Context, sqlDB *sql.DB, profileName, destAlias string, paused bool) error {
	scope, err := scopeKey(ctx, sqlDB, profileName, destAlias)
	if err != nil {
		return err
	}
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO queue_controls (scope, paused, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(scope) DO UPDATE SET paused = excluded.paused, updated_at = excluded.updated_at`,
		scope, boolToInt(paused), db.Now())
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot update queue pause", err)
	}
	return nil
}

func Paused(ctx context.Context, sqlDB *sql.DB, profileName, destAlias string) (bool, error) {
	scope, err := scopeKey(ctx, sqlDB, profileName, destAlias)
	if err != nil {
		return false, err
	}
	var paused int
	err = sqlDB.QueryRowContext(ctx, `SELECT paused FROM queue_controls WHERE scope = ?`, scope).Scan(&paused)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, apperr.Wrap(apperr.DatabaseError, "cannot read queue pause", err)
	}
	return paused == 1, nil
}

func scopeKey(ctx context.Context, sqlDB *sql.DB, profileName, destAlias string) (string, error) {
	if profileName != "" && destAlias != "" {
		return "", apperr.Invalid("use --profile or --destination, not both")
	}
	if profileName != "" {
		if _, err := profile.GetByName(ctx, sqlDB, profileName); err != nil {
			return "", err
		}
		return "profile:" + profileName, nil
	}
	if destAlias != "" {
		if _, err := account.GetDestinationByAlias(ctx, sqlDB, destAlias); err != nil {
			return "", err
		}
		return "destination:" + destAlias, nil
	}
	return "global", nil
}

func nextPosition(ctx context.Context, sqlDB *sql.DB, profileID, destID any) (int, error) {
	var n int
	err := sqlDB.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position), 0) FROM queue_items
		WHERE (profile_id IS ? AND destination_id IS ?)`, profileID, destID).Scan(&n)
	if err != nil {
		return 0, apperr.Wrap(apperr.DatabaseError, "cannot read queue position", err)
	}
	return n + 1, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (Item, error) {
	var (
		it                     Item
		profileID, destID      sql.NullString
		profileName, destAlias sql.NullString
		scheduled              sql.NullString
	)
	if err := row.Scan(&it.ID, &profileID, &destID, &profileName, &destAlias, &it.FilePath, &it.ContentHash, &it.Status, &it.Position, &scheduled, &it.CreatedAt, &it.UpdatedAt); err != nil {
		return Item{}, apperr.Wrap(apperr.DatabaseError, "cannot scan queue item", err)
	}
	it.ProfileID = profileID.String
	it.DestinationID = destID.String
	it.Profile = profileName.String
	it.Destination = destAlias.String
	it.ScheduledFor = scheduled.String
	return it, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func ListDueOneOffs(ctx context.Context, sqlDB *sql.DB, now time.Time) ([]Item, error) {
	return List(ctx, sqlDB, ListFilter{Status: Ready, DueBefore: now.UTC().Format(time.RFC3339)})
}

func SetStatus(ctx context.Context, sqlDB *sql.DB, itemID, status string) error {
	_, err := sqlDB.ExecContext(ctx, `UPDATE queue_items SET status = ?, updated_at = ? WHERE id = ?`, status, db.Now(), itemID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot update queue item", err)
	}
	return nil
}

func NextDue(ctx context.Context, sqlDB *sql.DB, destID string, now time.Time) (Item, error) {
	nowStr := now.UTC().Format(time.RFC3339)
	row := sqlDB.QueryRowContext(ctx, `
		SELECT q.id, q.profile_id, q.destination_id, p.name, d.alias, q.file_path, q.content_hash,
			q.status, q.position, q.scheduled_for, q.created_at, q.updated_at
		FROM queue_items q
		LEFT JOIN profiles p ON p.id = q.profile_id
		LEFT JOIN destinations d ON d.id = q.destination_id
		WHERE q.status = ?
		  AND (q.scheduled_for IS NULL OR q.scheduled_for <= ?)
		  AND (
			q.destination_id = ?
			OR (
				q.profile_id IS NOT NULL
				AND EXISTS (
					SELECT 1 FROM profile_destinations pd
					WHERE pd.profile_id = q.profile_id AND pd.destination_id = ? AND pd.enabled = 1
				)
				AND NOT EXISTS (
					SELECT 1 FROM publications pub
					WHERE pub.destination_id = ? AND pub.content_hash = q.content_hash
				)
			)
		  )
		ORDER BY CASE WHEN q.destination_id = ? THEN 0 ELSE 1 END, q.position, q.created_at
		LIMIT 1`, Ready, nowStr, destID, destID, destID, destID)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, apperr.New(apperr.QueueEmpty, "no ready queue item")
	}
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func CompleteIfDone(ctx context.Context, sqlDB *sql.DB, item Item) error {
	if item.DestinationID != "" {
		return SetStatus(ctx, sqlDB, item.ID, Published)
	}
	var remaining int
	err := sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM profile_destinations pd
		WHERE pd.profile_id = ? AND pd.enabled = 1
		  AND NOT EXISTS (
			SELECT 1 FROM publications pub
			WHERE pub.destination_id = pd.destination_id AND pub.content_hash = ?
		  )`, item.ProfileID, item.ContentHash).Scan(&remaining)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot check queue completion", err)
	}
	if remaining == 0 {
		return SetStatus(ctx, sqlDB, item.ID, Published)
	}
	return nil
}
