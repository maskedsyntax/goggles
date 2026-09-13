package publish

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/jobs"
	"github.com/maskedsyntax/goggles/internal/keychain"
	"github.com/maskedsyntax/goggles/internal/media"
	"github.com/maskedsyntax/goggles/internal/meta"
	"github.com/maskedsyntax/goggles/internal/platform"
	"github.com/maskedsyntax/goggles/internal/platform/instagram"
	"github.com/maskedsyntax/goggles/internal/platform/youtube"
	"github.com/maskedsyntax/goggles/internal/profile"
	"github.com/maskedsyntax/goggles/internal/storage"
)

type Request struct {
	Path            string
	Paths           []string
	CarouselDir     string
	CarouselPaths   []string
	Audio           string
	SlideSeconds    float64
	SlideSecondsSet bool
	ReplaceAudio    bool
	ReplaceAudioSet bool
	Profile         string
	Destination     string
	Platforms       []platform.Platform
	AllowDuplicate  bool
	DryRun          bool
	Caption         string
	ShareToFeed     bool
	Title           string
	Description     string
	Tags            []string
	Privacy         string
	CategoryID      string
	MadeForKids     bool
	MadeForKidsSet  bool
	ShareToFeedSet  bool
	PublishAt       string
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
	Kind       string      `json:"kind,omitempty"`
	Items      []string    `json:"items,omitempty"`
	Audio      string      `json:"audio,omitempty"`
	Jobs       []JobResult `json:"jobs"`
}

type Runner struct {
	DB          *sql.DB
	Host        storage.VideoHost
	Keychain    keychain.Store
	GraphBase   string
	YouTubeBase string
	NewIGClient func(token string) instagram.API
	NewYTClient func(token string) youtube.API
}

func (r *Runner) Run(ctx context.Context, req Request) (*Result, error) {
	dests, err := r.resolve(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(dests) == 0 {
		return nil, apperr.New(apperr.DestinationNotFound, "no destinations to publish to")
	}
	if req.Path == "" {
		if req.CarouselDir != "" {
			req.Path = req.CarouselDir
		} else if len(req.Paths) > 0 {
			req.Path = req.Paths[0]
		}
	}
	sidecar, err := meta.LoadFor(req.Path)
	if err != nil {
		return nil, apperr.Wrap(apperr.InvalidInput, "cannot read sidecar yaml", err)
	}
	req = overlayMedia(req, sidecar)
	items, err := collectItems(ctx, req)
	if err != nil {
		return nil, err
	}
	carousel := media.IsCarousel(items)
	if carousel {
		if n := len(items); n < media.MinCarouselItems || n > media.MaxCarouselItems {
			return nil, apperr.Invalid(fmt.Sprintf("Instagram carousels need 2–10 items, got %d", n))
		}
	} else if req.Audio != "" || req.ReplaceAudio || req.SlideSecondsSet {
		return nil, apperr.Invalid("--audio, --slide-seconds, and --replace-audio apply to Instagram carousels")
	}
	slide := req.SlideSeconds
	if slide <= 0 {
		slide = media.DefaultSlideSeconds
	}
	if slide > media.MaxSlideSeconds {
		return nil, apperr.Invalid(fmt.Sprintf("--slide-seconds must be <= %.0f", media.MaxSlideSeconds))
	}
	sourcePaths := itemPaths(items)
	hash := items[0].Info.Hash
	if carousel {
		hash, err = media.HashCarousel(sourcePaths, req.Audio, slide, req.ReplaceAudio)
		if err != nil {
			return nil, err
		}
	}

	out := &Result{
		SourceFile: req.Path,
		Profile:    req.Profile,
		DryRun:     req.DryRun,
		Items:      sourcePaths,
		Audio:      req.Audio,
		Kind:       "reel",
	}
	if carousel {
		out.Kind = "carousel"
	}

	if req.DryRun {
		out.Success = true
		for _, d := range dests {
			out.Jobs = append(out.Jobs, JobResult{Destination: d.Alias, Platform: string(d.Platform), Status: "planned"})
		}
		return out, nil
	}

	if media.NeedsPrepare(items, req.Audio) {
		workDir, err := os.MkdirTemp("", "goggles-carousel-*")
		if err != nil {
			return nil, apperr.Wrap(apperr.VideoInvalid, "cannot create carousel work directory", err)
		}
		defer os.RemoveAll(workDir)
		items, err = media.Prepare(ctx, items, media.PrepareOpts{
			Audio:        req.Audio,
			SlideSeconds: slide,
			ReplaceAudio: req.ReplaceAudio,
			WorkDir:      workDir,
		})
		if err != nil {
			return nil, err
		}
	}
	platMedia := toPlatformMedia(items, hash)

	batch, err := jobs.CreateBatch(ctx, r.DB, req.Path, hash)
	if err != nil {
		return nil, err
	}
	out.BatchID = batch.ID

	ok, fail := 0, 0
	for _, d := range dests {
		jr := r.runOne(ctx, batch.ID, d, platMedia, hash, req, sidecar)
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

func (r *Runner) runOne(ctx context.Context, batchID string, d account.Destination, m platform.Media, hash string, req Request, sidecar meta.File) JobResult {
	jr := JobResult{Destination: d.Alias, Platform: string(d.Platform)}
	job, err := jobs.CreateJob(ctx, r.DB, batchID, d.ID, d.Platform)
	if err != nil {
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.DatabaseError)
		jr.ErrorMessage = err.Error()
		return jr
	}
	jr.JobID = job.ID
	req = overlaySidecar(req, sidecar, d.Alias, d.Platform)
	if !d.Enabled {
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.DestinationDisabled)
		jr.ErrorMessage = d.Alias + " is disabled"
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, apperr.New(apperr.DestinationDisabled, jr.ErrorMessage), "", "")
		return jr
	}
	if err := r.checkDailyLimit(ctx, d); err != nil {
		jr.Status = jobs.Failed
		if e, ok := apperr.As(err); ok {
			jr.ErrorCode = string(e.Code)
			jr.ErrorMessage = e.Message
		} else {
			jr.ErrorCode = string(apperr.DailyLimit)
			jr.ErrorMessage = err.Error()
		}
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
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

	if len(m.Items) >= 2 && d.Platform == platform.YouTube {
		err := apperr.Invalid("Instagram carousels cannot be published to YouTube")
		jr.Status = jobs.Failed
		jr.ErrorCode = string(apperr.InvalidInput)
		jr.ErrorMessage = err.Error()
		_ = jobs.SetStatus(ctx, r.DB, job.ID, jobs.Failed, err, "", "")
		return jr
	}

	pub, err := r.publisherFor(d, job.ID)
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
	meta := platform.PlatformMetadata{
		"caption":       req.Caption,
		"share_to_feed": req.ShareToFeed,
		"title":         req.Title,
		"description":   req.Description,
		"tags":          req.Tags,
		"privacy":       req.Privacy,
		"category_id":   req.CategoryID,
		"made_for_kids": req.MadeForKids,
		"publish_at":    req.PublishAt,
	}
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

func (r *Runner) publisherFor(d account.Destination, jobID string) (platform.Publisher, error) {
	token, err := r.token(d)
	if err != nil {
		return nil, err
	}
	switch d.Platform {
	case platform.Instagram:
		var api instagram.API
		if r.NewIGClient != nil {
			api = r.NewIGClient(token)
		} else {
			api = instagram.NewClient(r.GraphBase, token, nil)
		}
		p := instagram.New(api, r.Host)
		p.DB = r.DB
		p.JobID = jobID
		return p, nil
	case platform.YouTube:
		var api youtube.API
		if r.NewYTClient != nil {
			api = r.NewYTClient(token)
		} else {
			base := r.YouTubeBase
			if base == "" {
				base = "https://www.googleapis.com"
			}
			api = youtube.NewClient(base, token, nil)
		}
		return youtube.New(api), nil
	default:
		return nil, apperr.NotImpl(string(d.Platform) + " publishing")
	}
}

func (r *Runner) token(d account.Destination) (string, error) {
	if r.Keychain == nil {
		return "", apperr.New(apperr.AuthRequired, "no keychain")
	}
	tok, err := r.Keychain.Get(string(d.Platform) + "/" + d.AccountID)
	if err != nil {
		return "", apperr.New(apperr.AuthRequired, string(d.Platform)+" token missing for "+d.Alias)
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

func overlayMedia(req Request, file meta.File) Request {
	ig := file.Instagram
	req.Audio = firstNonEmpty(req.Audio, ig.Audio)
	if req.Audio != "" {
		req.Audio = absFrom(file.Dir, req.Audio)
	}
	if !req.SlideSecondsSet && ig.SlideSeconds != nil {
		req.SlideSeconds = *ig.SlideSeconds
		req.SlideSecondsSet = true
	}
	if !req.ReplaceAudioSet && ig.ReplaceAudio != nil {
		req.ReplaceAudio = *ig.ReplaceAudio
		req.ReplaceAudioSet = true
	}
	if len(req.CarouselPaths) == 0 && len(ig.Carousel) > 0 {
		req.CarouselPaths = resolveAll(file.Dir, ig.Carousel)
	}
	return req
}

func collectItems(ctx context.Context, req Request) ([]media.Item, error) {
	if strings.TrimSpace(req.CarouselDir) != "" && len(req.Paths) > 0 {
		return nil, apperr.Invalid("use --carousel or file arguments, not both")
	}
	if req.CarouselDir != "" {
		if len(req.CarouselPaths) > 0 {
			return media.Collect(ctx, req.CarouselPaths)
		}
		return media.CollectDir(ctx, req.CarouselDir)
	}
	paths := append([]string{}, req.Paths...)
	if len(paths) == 0 && req.Path != "" {
		paths = []string{req.Path}
	}
	if len(paths) == 1 {
		st, err := os.Stat(paths[0])
		if err != nil {
			return nil, mapPathError(paths[0], err)
		}
		if st.IsDir() {
			if len(req.CarouselPaths) > 0 {
				return media.Collect(ctx, req.CarouselPaths)
			}
			return media.CollectDir(ctx, paths[0])
		}
		if len(req.CarouselPaths) > 0 {
			return media.Collect(ctx, req.CarouselPaths)
		}
	}
	return media.Collect(ctx, paths)
}

func toPlatformMedia(items []media.Item, hash string) platform.Media {
	first := media.ToPlatformMedia(items[0].Info)
	first.Hash = hash
	if media.IsCarousel(items) {
		first.Items = make([]platform.Media, 0, len(items))
		for _, it := range items {
			first.Items = append(first.Items, media.ToPlatformMedia(it.Info))
		}
	}
	return first
}

func itemPaths(items []media.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Path)
	}
	return out
}

func resolveAll(dir string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, absFrom(dir, p))
	}
	return out
}

func absFrom(dir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	if dir == "" {
		return path
	}
	joined := filepath.Join(dir, path)
	if _, err := os.Stat(joined); err == nil {
		return joined
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return joined
}

func mapPathError(path string, err error) error {
	if os.IsNotExist(err) {
		return apperr.New(apperr.FileNotFound, path)
	}
	return apperr.Wrap(apperr.FileUnreadable, "cannot read "+path, err)
}

func overlaySidecar(req Request, file meta.File, destAlias string, p platform.Platform) Request {
	dest, plat := file.For(destAlias, p)
	req.Caption = firstNonEmpty(req.Caption, dest.Caption, plat.Caption)
	req.Title = firstNonEmpty(req.Title, dest.Title, plat.Title)
	req.Description = firstNonEmpty(req.Description, dest.Description, plat.Description)
	req.Privacy = firstNonEmpty(req.Privacy, dest.Privacy, plat.Privacy)
	req.CategoryID = firstNonEmpty(req.CategoryID, dest.CategoryID, plat.CategoryID)
	if len(req.Tags) == 0 {
		req.Tags = dest.Tags
		if len(req.Tags) == 0 {
			req.Tags = plat.Tags
		}
	}
	if !req.ShareToFeedSet {
		if dest.ShareToFeed != nil {
			req.ShareToFeed = *dest.ShareToFeed
		} else if plat.ShareToFeed != nil {
			req.ShareToFeed = *plat.ShareToFeed
		}
	}
	if !req.MadeForKidsSet {
		if dest.MadeForKids != nil {
			req.MadeForKids = *dest.MadeForKids
		} else if plat.MadeForKids != nil {
			req.MadeForKids = *plat.MadeForKids
		}
	}
	return req
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (r *Runner) checkDailyLimit(ctx context.Context, d account.Destination) error {
	limit := account.DailyLimit(d)
	if limit <= 0 {
		return nil
	}
	loc := time.UTC
	if d.Timezone != "" {
		if l, err := time.LoadLocation(d.Timezone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	n, err := jobs.CountPublishedSince(ctx, r.DB, d.ID, start.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if n >= limit {
		return apperr.New(apperr.DailyLimit, fmt.Sprintf("%s has reached its daily limit (%d)", d.Alias, limit))
	}
	return nil
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
