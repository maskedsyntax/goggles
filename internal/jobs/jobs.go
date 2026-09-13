package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/id"
	"github.com/maskedsyntax/goggles/internal/platform"
)

const (
	BatchPending   = "pending"
	BatchPartial   = "partial"
	BatchCompleted = "completed"
	BatchFailed    = "failed"

	Queued         = "queued"
	Validating     = "validating"
	Preparing      = "preparing"
	Uploading      = "uploading"
	Processing     = "processing"
	Publishing     = "publishing"
	Published      = "published"
	CleanupPending = "cleanup_pending"
	Completed      = "completed"
	RetryWait      = "retry_wait"
	Failed         = "failed"
	Cancelled      = "cancelled"
)

type Batch struct {
	ID             string `json:"id"`
	QueueItemID    string `json:"queue_item_id,omitempty"`
	SourceFilePath string `json:"source_file_path"`
	ContentHash    string `json:"content_hash"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

type Job struct {
	ID                  string `json:"id"`
	BatchID             string `json:"batch_id"`
	DestinationID       string `json:"destination_id"`
	Platform            string `json:"platform"`
	Status              string `json:"status"`
	AttemptCount        int    `json:"attempt_count"`
	LastErrorCode       string `json:"last_error_code,omitempty"`
	LastErrorMessage    string `json:"last_error_message,omitempty"`
	ExternalContainerID string `json:"external_container_id,omitempty"`
	ExternalMediaID     string `json:"external_media_id,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	CompletedAt         string `json:"completed_at,omitempty"`
}

type Publication struct {
	ID              string `json:"id"`
	JobID           string `json:"job_id"`
	DestinationID   string `json:"destination_id"`
	Platform        string `json:"platform"`
	ContentHash     string `json:"content_hash"`
	SourceFilePath  string `json:"source_file_path"`
	ExternalMediaID string `json:"external_media_id,omitempty"`
	ExternalURL     string `json:"external_url,omitempty"`
	PublishedAt     string `json:"published_at"`
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func CreateBatch(ctx context.Context, sqlDB *sql.DB, path, hash string) (Batch, error) {
	b := Batch{
		ID:             id.New(id.Batch),
		SourceFilePath: path,
		ContentHash:    hash,
		Status:         BatchPending,
		CreatedAt:      now(),
	}
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO batches (id, source_file_path, content_hash, status, created_at)
		VALUES (?, ?, ?, ?, ?)`, b.ID, b.SourceFilePath, b.ContentHash, b.Status, b.CreatedAt)
	if err != nil {
		return Batch{}, apperr.Wrap(apperr.DatabaseError, "cannot create batch", err)
	}
	return b, nil
}

func CreateJob(ctx context.Context, sqlDB *sql.DB, batchID, destID string, p platform.Platform) (Job, error) {
	j := Job{
		ID:            id.New(id.Job),
		BatchID:       batchID,
		DestinationID: destID,
		Platform:      string(p),
		Status:        Queued,
		CreatedAt:     now(),
		UpdatedAt:     now(),
	}
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO jobs (id, batch_id, destination_id, platform, status, attempt_count, provider_state_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, '{}', ?, ?)`,
		j.ID, j.BatchID, j.DestinationID, j.Platform, j.Status, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return Job{}, apperr.Wrap(apperr.DatabaseError, "cannot create job", err)
	}
	return j, nil
}

func SetStatus(ctx context.Context, sqlDB *sql.DB, jobID, status string, err error, mediaID, containerID string) error {
	var code, msg string
	if err != nil {
		if e, ok := apperr.As(err); ok {
			code = string(e.Code)
			msg = e.Message
		} else {
			code = string(apperr.MetaRequestFailed)
			msg = err.Error()
		}
	}
	completed := ""
	if status == Completed || status == Failed || status == Cancelled {
		completed = now()
	}
	_, dbErr := sqlDB.ExecContext(ctx, `
		UPDATE jobs SET
			status = ?,
			last_error_code = ?,
			last_error_message = ?,
			external_media_id = COALESCE(NULLIF(?, ''), external_media_id),
			external_container_id = COALESCE(NULLIF(?, ''), external_container_id),
			attempt_count = attempt_count + 1,
			updated_at = ?,
			completed_at = CASE WHEN ? != '' THEN ? ELSE completed_at END
		WHERE id = ?`,
		status, nullIfEmpty(code), nullIfEmpty(msg), mediaID, containerID, now(), completed, completed, jobID)
	if dbErr != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot update job", dbErr)
	}
	return nil
}

func FinishBatch(ctx context.Context, sqlDB *sql.DB, batchID, status string) error {
	_, err := sqlDB.ExecContext(ctx, `UPDATE batches SET status = ?, completed_at = ? WHERE id = ?`, status, now(), batchID)
	if err != nil {
		return apperr.Wrap(apperr.DatabaseError, "cannot update batch", err)
	}
	return nil
}

func HasPublication(ctx context.Context, sqlDB *sql.DB, destID, hash string) (bool, error) {
	var n int
	err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM publications WHERE destination_id = ? AND content_hash = ?`, destID, hash).Scan(&n)
	if err != nil {
		return false, apperr.Wrap(apperr.DatabaseError, "cannot check publications", err)
	}
	return n > 0, nil
}

func InsertPublication(ctx context.Context, sqlDB *sql.DB, p Publication) (Publication, error) {
	if p.ID == "" {
		p.ID = id.New(id.Publication)
	}
	if p.PublishedAt == "" {
		p.PublishedAt = now()
	}
	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO publications (
			id, job_id, destination_id, platform, content_hash, source_file_path,
			external_media_id, external_url, published_at, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?)`,
		p.ID, p.JobID, p.DestinationID, p.Platform, p.ContentHash, p.SourceFilePath,
		nullIfEmpty(p.ExternalMediaID), nullIfEmpty(p.ExternalURL), p.PublishedAt, p.PublishedAt)
	if err != nil {
		return Publication{}, apperr.Wrap(apperr.DatabaseError, "cannot record publication", err)
	}
	return p, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func IsJobSuccess(status string) bool {
	return status == Completed || status == Published
}

var ErrNoRows = sql.ErrNoRows

func Discard(err error) bool { return errors.Is(err, sql.ErrNoRows) }

type Filter struct {
	Status      string
	Destination string
	Limit       int
}

type View struct {
	Job
	Destination    string `json:"destination"`
	SourceFilePath string `json:"source_file_path"`
	ContentHash    string `json:"content_hash"`
}

func List(ctx context.Context, sqlDB *sql.DB, f Filter) ([]View, error) {
	q := `
		SELECT j.id, j.batch_id, j.destination_id, j.platform, j.status, j.attempt_count,
			j.last_error_code, j.last_error_message, j.external_container_id, j.external_media_id,
			j.created_at, j.updated_at, j.completed_at, d.alias, b.source_file_path, b.content_hash
		FROM jobs j
		JOIN destinations d ON d.id = j.destination_id
		JOIN batches b ON b.id = j.batch_id
		WHERE 1=1`
	var args []any
	if f.Status != "" {
		q += ` AND j.status = ?`
		args = append(args, f.Status)
	}
	if f.Destination != "" {
		q += ` AND d.alias = ?`
		args = append(args, f.Destination)
	}
	q += ` ORDER BY j.created_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list jobs", err)
	}
	defer rows.Close()
	var out []View
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func Get(ctx context.Context, sqlDB *sql.DB, jobID string) (View, error) {
	row := sqlDB.QueryRowContext(ctx, `
		SELECT j.id, j.batch_id, j.destination_id, j.platform, j.status, j.attempt_count,
			j.last_error_code, j.last_error_message, j.external_container_id, j.external_media_id,
			j.created_at, j.updated_at, j.completed_at, d.alias, b.source_file_path, b.content_hash
		FROM jobs j
		JOIN destinations d ON d.id = j.destination_id
		JOIN batches b ON b.id = j.batch_id
		WHERE j.id = ?`, jobID)
	v, err := scanView(row)
	if errors.Is(err, sql.ErrNoRows) {
		return View{}, apperr.New(apperr.JobNotFound, jobID)
	}
	if err != nil {
		return View{}, apperr.Wrap(apperr.DatabaseError, "cannot load job", err)
	}
	return v, nil
}

func Cancel(ctx context.Context, sqlDB *sql.DB, jobID string) error {
	v, err := Get(ctx, sqlDB, jobID)
	if err != nil {
		return err
	}
	if v.Status == Completed || v.Status == Published || v.Status == Cancelled {
		return apperr.Invalid("job cannot be cancelled in status " + v.Status)
	}
	return SetStatus(ctx, sqlDB, jobID, Cancelled, nil, v.ExternalMediaID, v.ExternalContainerID)
}

func CountPublishedSince(ctx context.Context, sqlDB *sql.DB, destID, sinceRFC3339 string) (int, error) {
	var n int
	err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM publications WHERE destination_id = ? AND published_at >= ?`, destID, sinceRFC3339).Scan(&n)
	if err != nil {
		return 0, apperr.Wrap(apperr.DatabaseError, "cannot count publications", err)
	}
	return n, nil
}

type PubView struct {
	Publication
	Destination string `json:"destination"`
}

func ListPublications(ctx context.Context, sqlDB *sql.DB, destAlias string, limit int) ([]PubView, error) {
	q := `
		SELECT p.id, p.job_id, p.destination_id, p.platform, p.content_hash, p.source_file_path,
			p.external_media_id, p.external_url, p.published_at, d.alias
		FROM publications p
		JOIN destinations d ON d.id = p.destination_id
		WHERE 1=1`
	var args []any
	if destAlias != "" {
		q += ` AND d.alias = ?`
		args = append(args, destAlias)
	}
	q += ` ORDER BY p.published_at DESC`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := sqlDB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperr.Wrap(apperr.DatabaseError, "cannot list history", err)
	}
	defer rows.Close()
	var out []PubView
	for rows.Next() {
		var v PubView
		var extID, extURL sql.NullString
		if err := rows.Scan(&v.ID, &v.JobID, &v.DestinationID, &v.Platform, &v.ContentHash, &v.SourceFilePath, &extID, &extURL, &v.PublishedAt, &v.Destination); err != nil {
			return nil, apperr.Wrap(apperr.DatabaseError, "cannot scan publication", err)
		}
		v.ExternalMediaID = extID.String
		v.ExternalURL = extURL.String
		out = append(out, v)
	}
	return out, rows.Err()
}

func GetPublication(ctx context.Context, sqlDB *sql.DB, id string) (PubView, error) {
	row := sqlDB.QueryRowContext(ctx, `
		SELECT p.id, p.job_id, p.destination_id, p.platform, p.content_hash, p.source_file_path,
			p.external_media_id, p.external_url, p.published_at, d.alias
		FROM publications p
		JOIN destinations d ON d.id = p.destination_id
		WHERE p.id = ? OR p.job_id = ? OR p.external_media_id = ?`, id, id, id)
	var v PubView
	var extID, extURL sql.NullString
	err := row.Scan(&v.ID, &v.JobID, &v.DestinationID, &v.Platform, &v.ContentHash, &v.SourceFilePath, &extID, &extURL, &v.PublishedAt, &v.Destination)
	if errors.Is(err, sql.ErrNoRows) {
		return PubView{}, apperr.New(apperr.JobNotFound, "publication not found")
	}
	if err != nil {
		return PubView{}, apperr.Wrap(apperr.DatabaseError, "cannot load publication", err)
	}
	v.ExternalMediaID = extID.String
	v.ExternalURL = extURL.String
	return v, nil
}

func scanView(row interface{ Scan(dest ...any) error }) (View, error) {
	var v View
	var errCode, errMsg, container, media, completed sql.NullString
	if err := row.Scan(&v.ID, &v.BatchID, &v.DestinationID, &v.Platform, &v.Status, &v.AttemptCount,
		&errCode, &errMsg, &container, &media, &v.CreatedAt, &v.UpdatedAt, &completed, &v.Destination, &v.SourceFilePath, &v.ContentHash); err != nil {
		return View{}, err
	}
	v.LastErrorCode = errCode.String
	v.LastErrorMessage = errMsg.String
	v.ExternalContainerID = container.String
	v.ExternalMediaID = media.String
	v.CompletedAt = completed.String
	return v, nil
}
