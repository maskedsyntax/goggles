package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/id"
)

type Upload struct {
	ID            string
	JobID         string
	Provider      string
	ObjectKey     string
	PublicURL     string
	FileSize      int64
	UploadedAt    time.Time
	ExpiresAt     time.Time
	DeletedAt     *time.Time
	CleanupStatus string
}

func InsertUpload(ctx context.Context, sqlDB *sql.DB, u Upload) (Upload, error) {
	if u.ID == "" {
		u.ID = id.New(id.Upload)
	}
	if u.Provider == "" {
		u.Provider = ProviderR2
	}
	if u.CleanupStatus == "" {
		u.CleanupStatus = CleanupPending
	}
	var jobID any
	if u.JobID != "" {
		jobID = u.JobID
	}
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO uploads (
			id, job_id, provider, object_key, public_url, file_size,
			uploaded_at, expires_at, deleted_at, cleanup_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, jobID, u.Provider, u.ObjectKey, u.PublicURL, u.FileSize,
		u.UploadedAt.UTC().Format(time.RFC3339Nano),
		u.ExpiresAt.UTC().Format(time.RFC3339Nano),
		nil, u.CleanupStatus,
	)
	if err != nil {
		return Upload{}, apperr.Wrap(apperr.DatabaseError, "cannot record upload", err)
	}
	return u, nil
}

func RecordHosted(ctx context.Context, sqlDB *sql.DB, hosted *HostedVideo, jobID string) (Upload, error) {
	return InsertUpload(ctx, sqlDB, Upload{
		JobID:         jobID,
		Provider:      ProviderR2,
		ObjectKey:     hosted.ObjectKey,
		PublicURL:     hosted.PublicURL,
		FileSize:      hosted.SizeBytes,
		UploadedAt:    time.Now().UTC(),
		ExpiresAt:     hosted.ExpiresAt,
		CleanupStatus: CleanupPending,
	})
}

func CountPending(ctx context.Context, sqlDB *sql.DB) (int, error) {
	var n int
	err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM uploads WHERE cleanup_status IN (?, ?)`, CleanupPending, CleanupFailed).Scan(&n)
	if err != nil {
		return 0, apperr.Wrap(apperr.DatabaseError, "cannot count uploads", err)
	}
	return n, nil
}

func ListDue(ctx context.Context, sqlDB *sql.DB, now time.Time) ([]Upload, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, job_id, provider, object_key, public_url, file_size, uploaded_at, expires_at, deleted_at, cleanup_status
		FROM uploads
		WHERE cleanup_status IN (?, ?) AND expires_at <= ?
		ORDER BY expires_at`, CleanupPending, CleanupFailed, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list stale uploads", err)
	}
	defer rows.Close()
	var out []Upload
	for rows.Next() {
		u, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func MarkDeleted(ctx context.Context, sqlDB *sql.DB, uploadID string, at time.Time) error {
	_, err := sqlDB.ExecContext(ctx, `
		UPDATE uploads SET cleanup_status = ?, deleted_at = ? WHERE id = ?`,
		CleanupDeleted, at.UTC().Format(time.RFC3339Nano), uploadID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot mark upload deleted", err)
	}
	return nil
}

func MarkFailed(ctx context.Context, sqlDB *sql.DB, uploadID string) error {
	_, err := sqlDB.ExecContext(ctx, `UPDATE uploads SET cleanup_status = ? WHERE id = ?`, CleanupFailed, uploadID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot mark upload failed", err)
	}
	return nil
}

type CleanupResult struct {
	Scanned int      `json:"scanned"`
	Deleted int      `json:"deleted"`
	Failed  int      `json:"failed"`
	Keys    []string `json:"keys,omitempty"`
}

func Cleanup(ctx context.Context, sqlDB *sql.DB, host VideoHost, now time.Time, dryRun bool) (CleanupResult, error) {
	due, err := ListDue(ctx, sqlDB, now)
	if err != nil {
		return CleanupResult{}, err
	}
	res := CleanupResult{Scanned: len(due)}
	for _, u := range due {
		res.Keys = append(res.Keys, u.ObjectKey)
		if dryRun {
			res.Deleted++
			continue
		}
		if err := host.Delete(ctx, u.ObjectKey); err != nil {
			_ = MarkFailed(ctx, sqlDB, u.ID)
			res.Failed++
			continue
		}
		if err := MarkDeleted(ctx, sqlDB, u.ID, now); err != nil {
			res.Failed++
			continue
		}
		res.Deleted++
	}
	return res, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUpload(row rowScanner) (Upload, error) {
	var (
		u          Upload
		jobID      sql.NullString
		uploadedAt string
		expiresAt  string
		deletedAt  sql.NullString
	)
	if err := row.Scan(&u.ID, &jobID, &u.Provider, &u.ObjectKey, &u.PublicURL, &u.FileSize, &uploadedAt, &expiresAt, &deletedAt, &u.CleanupStatus); err != nil {
		return Upload{}, apperr.Wrap(apperr.DatabaseError, "cannot scan upload", err)
	}
	u.JobID = jobID.String
	u.UploadedAt, _ = time.Parse(time.RFC3339Nano, uploadedAt)
	u.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	if deletedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, deletedAt.String)
		u.DeletedAt = &t
	}
	return u, nil
}
