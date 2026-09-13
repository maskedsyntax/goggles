package publish

import (
	"context"
	"database/sql"
	"strings"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/jobs"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/platform/instagram"
	"github.com/maskedsyntax/goggles/internal/profile"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type Request struct {
	Path           string
	Profile        string
	Destination    string
	Platforms      []platform.Platform
	AllowDuplicate bool
	DryRun         bool
	Caption        string
	ShareToFeed    bool
}

type JobResult struct {
	JobID           string `json:"job_id,omitempty"`
	Destination     string `json:"destination"`
	Platform        string `json:"platform"`
	Status          string `json:"status"`
	ExternalMediaID string `json:"external_media_id,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

type Result struct {
	Success    bool        `json:"success"`
	Partial    bool        `json:"partial,omitempty"`
	DryRun     bool        `json:"dry_run,omitempty"`
	BatchID    string      `json:"batch_id,omitempty"`
	Profile    string      `json:"profile,omitempty"`
	SourceFile string      `json:"source_file"`
	Jobs       []JobResult `json:"jobs"`
}

type Runner struct {
	DB          *sql.DB
	Host        storage.VideoHost
	Keychain    keychain.Store
	GraphBase   string
	NewIGClient func(token string) instagram.API
}

func (r *Runner) Run(ctx context.Context, req Request) (*Result, error) {
	dests, err := r.resolve(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(dests) == 0 {
		return nil, apperr.New(apperr.DestinationNotFound, "no destinations to publish to")
	}
	info, err := media.Probe(ctx, req.Path)
	if err != nil {
		return nil, err
	}
	platMedia := media.ToPlatformMedia(info)

	out := &Result{SourceFile: req.Path, Profile: req.Profile, DryRun: req.DryRun}
	if req.DryRun {
		out.Success = true
		for _, d := range dests {
			out.Jobs = append(out.Jobs, JobResult{Destination: d.Alias, Platform: string(d.Platform), Status: "planned"})
		}
		return out, nil
	}

	batch, err := jobs.CreateBatch(ctx, r.DB, req.Path, info.Hash)
	if err != nil {
		return nil, err
	}
	out.BatchID = batch.ID

	ok, fail := 0, 0
	for _, d := range dests {
		jr := r.runOne(ctx, batch.ID, d, platMedia, info.Hash, req)
		out.Jobs = append(out.Jobs, jr)
		if jr.Status == jobs.Completed {
			ok++
		} else {
			fail++
		}
	}
	switch {
	case fail == 0:
		out.Success = true
		_ = jobs.FinishBatch(ctx, r.DB, batch.ID, jobs.BatchCompleted)
	case ok == 0:
		out.Success = false
		_ = jobs.FinishBatch(ctx, r.DB, batch.ID, jobs.BatchFailed)
	default:
		out.Success = false
		out.Partial = true
		_ = jobs.FinishBatch(ctx, r.DB, batch.ID, jobs.BatchPartial)
	}
	return out, nil
}

func (r *Runner) runOne(ctx context.Context, batchID string, d account.Destination, m platform.Media, hash string, req Request) JobResult {
	jr := JobResult{Destination: d.Alias, Platform: string(d.Platform)}
	job, err := jobs.CreateJob(ctx, r.DB, batchID, d.ID, d.Platform)
	if err != nil {
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.DatabaseError)
		jr.ErrorMessage = err.Error()
		return jr
	}
	jr.JobID = job.ID
	if !d.Enabled {
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.DestinationDisabled)
		jr.ErrorMessage = d.Alias + " is disabled"
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, apperr.New(apperr.DestinationDisabled, jr.ErrorMessage), "", "")
		return jr
	}
	if !req.AllowDuplicate {
		dup, err := jobs.HasPublication(ctx, r.DB, d.ID, hash)
		if err != nil {
			jr.Status = jobs.Failed
			jr.ErrorCode = string(apperr.DatabaseError)
			jr.ErrorMessage = err.Error()
			_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
			return jr
		}
		if dup {
			err := apperr.New(apperr.DuplicateContent, "already published to "+d.Alias)
			jr.Status = jobs.Failed
			jr.ErrorCode = string(apperr.DuplicateContent)
			jr.ErrorMessage = err.Error()
			_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
			return jr
		}
	}

	pub, err := r.publisherFor(d)
	if err != nil {
		jr.Status = jobs.Failed
		code := apperr.NotImplemented
		if e, ok := apperr.As(err); ok {
			code = e.Code
		}
		jr.ErrorCode = string(code)
		jr.ErrorMessage = err.Error()
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
		return jr
	}
	_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Publishing, nil, "", "")
	meta := platform.PlatformMetadata{"caption": req.Caption, "share_to_feed": req.ShareToFeed}
	res, err := pub.Publish(ctx, toPlatformDest(d), m, meta)
	if err != nil {
		jr.Status = jobs.Failed
		if e, ok := apperr.As(err); ok {
			jr.ErrorCode = string(e.Code)
			jr.ErrorMessage = e.Message
		} else {
			jr.ErrorCode = string(apperr.MetaRequestFailed)
			jr.ErrorMessage = err.Error()
		}
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
		return jr
	}
	mediaID := ""
	if res != nil {
		mediaID = res.ExternalMediaID
		jr.ExternalMediaID = mediaID
	}
	if _, err := jobs.InsertPublication(ctx, r.DB, jobs.Publication{
		JobID:           job.ID,
		DestinationID:   d.ID,
		Platform:        string(d.Platform),
		ContentHash:     hash,
		SourceFilePath:  req.Path,
		ExternalMediaID: mediaID,
	}); err != nil {
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.DatabaseError)
		jr.ErrorMessage = err.Error()
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, mediaID, "")
		return jr
	}
	_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Completed, nil, mediaID, "")
	jr.Status = jobs.Completed
	return jr
}

func (r *Runner) publisherFor(d account.Destination) (platform.Publisher, error) {
	switch d.Platform {
	case platform.Instagram:
		token, err := r.token(d)
		if err != nil {
			return nil, err
		}
		var api instagram.API
		if r.NewIGClient != nil {
			api = r.NewIGClient(token)
		} else {
			api = instagram.NewClient(r.GraphBase, token, nil)
		}
		p := instagram.New(api, r.Host)
		p.DB = r.DB
		return p, nil
	case platform.YouTube:
		return nil, apperr.NotImpl("youtube publishing")
	default:
		return nil, apperr.NotImpl(string(d.Platform) + " publishing")
	}
}

func (r *Runner) token(d account.Destination) (string, error) {
	if r.Keychain == nil {
		return "", apperr.New(apperr.AuthRequired, "no keychain")
	}
	tok, err := r.Keychain.Get("instagram/" + d.AccountID)
	if err != nil {
		return "", apperr.New(apperr.AuthRequired, "Instagram token missing for "+d.Alias)
	}
	return tok, nil
}

func (r *Runner) resolve(ctx context.Context, req Request) ([]account.Destination, error) {
	if strings.TrimSpace(req.Destination) != "" && strings.TrimSpace(req.Profile) != "" {
		return nil, apperr.Invalid("use --destination or --profile, not both")
	}
	if req.Destination != "" {
		d, err := account.GetDestinationByAlias(ctx, r.DB, req.Destination)
		if err != nil {
			return nil, err
		}
		return []account.Destination{d}, nil
	}
	if req.Profile == "" {
		return nil, apperr.Invalid("pass --destination or --profile")
	}
	view, err := profile.Show(ctx, r.DB, req.Profile)
	if err != nil {
		return nil, err
	}
	return profile.DestinationsForPlatforms(view, req.Platforms), nil
}

func toPlatformDest(d account.Destination) platform.Destination {
	return platform.Destination{
		ID:               d.ID,
		AccountID:        d.AccountID,
		Alias:            d.Alias,
		Platform:         d.Platform,
		ExternalID:       d.ExternalID,
		ExternalUsername: d.ExternalUsername,
		ExternalTitle:    d.ExternalTitle,
		Timezone:         d.Timezone,
		Enabled:          d.Enabled,
	}
}
