package daemon

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/maskedsyntax/goggles/internal/account"
	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/jobs"
	"github.com/maskedsyntax/goggles/internal/publish"
	"github.com/maskedsyntax/goggles/internal/queue"
	"github.com/maskedsyntax/goggles/internal/schedule"
	"github.com/maskedsyntax/goggles/internal/storage"
)

const DefaultWindow = 15 * time.Minute
const DefaultInterval = 30 * time.Second
const DefaultStale = 10 * time.Minute

type Engine struct {
	DB         *sql.DB
	Log        *slog.Logger
	Runner     *publish.Runner
	Host       storage.VideoHost
	Now        func() time.Time
	Window     time.Duration
	Interval   time.Duration
	StaleAfter time.Duration
	Publish    func(ctx context.Context, destAlias string, item queue.Item) error
	Retry      func(ctx context.Context, job jobs.View) error
}

type TickResult struct {
	Fired     int `json:"fired"`
	Failed    int `json:"failed"`
	Cleaned   int `json:"cleaned"`
	Retried   int `json:"retried,omitempty"`
	Recovered int `json:"recovered,omitempty"`
	Refreshed int `json:"refreshed,omitempty"`
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) window() time.Duration {
	if e.Window > 0 {
		return e.Window
	}
	return DefaultWindow
}

func (e *Engine) Run(ctx context.Context) error {
	interval := e.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	if _, err := e.Tick(ctx); err != nil && e.Log != nil {
		e.Log.Error("daemon tick failed", "err", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := e.Tick(ctx); err != nil && e.Log != nil {
				e.Log.Error("daemon tick failed", "err", err)
			}
		}
	}
}

func (e *Engine) staleAfter() time.Duration {
	if e.StaleAfter > 0 {
		return e.StaleAfter
	}
	return DefaultStale
}

func (e *Engine) Tick(ctx context.Context) (TickResult, error) {
	var res TickResult
	n, f, err := e.processOneOffs(ctx)
	res.Fired += n
	res.Failed += f
	if err != nil {
		return res, err
	}
	n, f, err = e.processDueSlots(ctx)
	res.Fired += n
	res.Failed += f
	if err != nil {
		return res, err
	}
	n, f, err = e.processRetries(ctx)
	res.Retried += n
	res.Failed += f
	if err != nil {
		return res, err
	}
	n, f, err = e.recoverStale(ctx)
	res.Recovered += n
	res.Failed += f
	if err != nil {
		return res, err
	}
	res.Refreshed = e.refreshTokens(ctx)
	if e.Host != nil {
		cr, err := storage.Cleanup(ctx, e.DB, e.Host, e.now().UTC(), false)
		if err != nil && e.Log != nil {
			e.Log.Warn("r2 cleanup failed", "err", err)
		} else {
			res.Cleaned = cr.Deleted
		}
	}
	return res, nil
}

func (e *Engine) processDueSlots(ctx context.Context) (fired, failed int, err error) {
	slots, err := schedule.ListEnabled(ctx, e.DB)
	if err != nil {
		return 0, 0, err
	}
	now := e.now()
	seen := map[string]struct{}{}
	for _, slot := range slots {
		loc, err := schedule.LoadTZ(slot.Timezone)
		if err != nil {
			continue
		}
		var last *time.Time
		if slot.LastFiredAt != nil {
			if t, err := time.Parse(time.RFC3339Nano, *slot.LastFiredAt); err == nil {
				last = &t
			} else if t, err := time.Parse(time.RFC3339, *slot.LastFiredAt); err == nil {
				last = &t
			}
		}
		occ, due, err := schedule.Due(now, loc, slot.TimeOfDay, last, e.window())
		if err != nil || !due {
			continue
		}
		paused, err := queue.Paused(ctx, e.DB, "", slot.Destination)
		if err != nil {
			return fired, failed, err
		}
		if paused {
			continue
		}
		item, err := queue.NextDue(ctx, e.DB, slot.DestinationID, now)
		if err != nil {
			if ae, ok := apperr.As(err); ok && ae.Code == apperr.QueueEmpty {
				continue
			}
			return fired, failed, err
		}
		if item.Profile != "" {
			ppaused, err := queue.Paused(ctx, e.DB, item.Profile, "")
			if err != nil {
				return fired, failed, err
			}
			if ppaused {
				continue
			}
		}
		key := slot.DestinationID + "|" + occ.UTC().Format(time.RFC3339)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := e.publishItem(ctx, slot.Destination, item); err != nil {
			failed++
			_ = queue.SetStatus(ctx, e.DB, item.ID, queue.Failed)
			_ = schedule.MarkFired(ctx, e.DB, slot.ID, occ)
			if e.Log != nil {
				e.Log.Error("scheduled publish failed", "destination", slot.Destination, "item", item.ID, "err", err)
			}
			continue
		}
		_ = queue.CompleteIfDone(ctx, e.DB, item)
		_ = schedule.MarkFired(ctx, e.DB, slot.ID, occ)
		fired++
	}
	return fired, failed, nil
}

func (e *Engine) processOneOffs(ctx context.Context) (fired, failed int, err error) {
	items, err := queue.ListDueOneOffs(ctx, e.DB, e.now())
	if err != nil {
		return 0, 0, err
	}
	for _, item := range items {
		if item.Destination != "" {
			paused, err := queue.Paused(ctx, e.DB, "", item.Destination)
			if err != nil {
				return fired, failed, err
			}
			if paused {
				continue
			}
			if err := e.publishItem(ctx, item.Destination, item); err != nil {
				failed++
				_ = queue.SetStatus(ctx, e.DB, item.ID, queue.Failed)
				continue
			}
			_ = queue.CompleteIfDone(ctx, e.DB, item)
			fired++
			continue
		}
		paused, err := queue.Paused(ctx, e.DB, item.Profile, "")
		if err != nil {
			return fired, failed, err
		}
		if paused {
			continue
		}
		if err := e.publishProfile(ctx, item); err != nil {
			failed++
			_ = queue.SetStatus(ctx, e.DB, item.ID, queue.Failed)
			continue
		}
		_ = queue.SetStatus(ctx, e.DB, item.ID, queue.Published)
		fired++
	}
	return fired, failed, nil
}

func (e *Engine) publishProfile(ctx context.Context, item queue.Item) error {
	if e.Runner == nil {
		return apperr.New(apperr.DaemonUnavailable, "publish runner is not configured")
	}
	res, err := e.Runner.Run(ctx, publish.Request{Path: item.FilePath, Profile: item.Profile})
	if err != nil {
		return err
	}
	if res == nil || !res.Success {
		return apperr.New(apperr.Partial, "profile publish did not fully succeed")
	}
	return nil
}

func (e *Engine) publishItem(ctx context.Context, destAlias string, item queue.Item) error {
	if e.Publish != nil {
		return e.Publish(ctx, destAlias, item)
	}
	if e.Runner == nil {
		return apperr.New(apperr.DaemonUnavailable, "publish runner is not configured")
	}
	res, err := e.Runner.Run(ctx, publish.Request{
		Path:        item.FilePath,
		Destination: destAlias,
	})
	if err != nil {
		return err
	}
	if res == nil || !res.Success {
		msg := "publish failed"
		if res != nil && len(res.Jobs) > 0 && res.Jobs[0].ErrorMessage != "" {
			msg = res.Jobs[0].ErrorMessage
		}
		return apperr.New(apperr.MetaRequestFailed, msg)
	}
	return nil
}

func (e *Engine) processRetries(ctx context.Context) (fired, failed int, err error) {
	due, err := jobs.ListDueRetries(ctx, e.DB, e.now())
	if err != nil {
		return 0, 0, err
	}
	for _, job := range due {
		if err := e.retryJob(ctx, job); err != nil {
			failed++
			if e.Log != nil {
				e.Log.Error("job retry failed", "job", job.ID, "err", err)
			}
			continue
		}
		fired++
	}
	return fired, failed, nil
}

func (e *Engine) recoverStale(ctx context.Context) (fired, failed int, err error) {
	stale, err := jobs.ListStale(ctx, e.DB, e.now().Add(-e.staleAfter()))
	if err != nil {
		return 0, 0, err
	}
	for _, job := range stale {
		if err := e.retryJob(ctx, job); err != nil {
			failed++
			if e.Log != nil {
				e.Log.Error("stale job recovery failed", "job", job.ID, "err", err)
			}
			continue
		}
		fired++
	}
	return fired, failed, nil
}

func (e *Engine) retryJob(ctx context.Context, job jobs.View) error {
	if e.Retry != nil {
		return e.Retry(ctx, job)
	}
	if e.Runner == nil {
		return apperr.New(apperr.DaemonUnavailable, "publish runner is not configured")
	}
	res, err := e.Runner.Retry(ctx, job)
	if err != nil {
		return err
	}
	if res != nil && res.Success {
		return nil
	}
	if res != nil && len(res.Jobs) > 0 && res.Jobs[0].Status == jobs.RetryWait {
		return nil
	}
	msg := "retry failed"
	if res != nil && len(res.Jobs) > 0 && res.Jobs[0].ErrorMessage != "" {
		msg = res.Jobs[0].ErrorMessage
	}
	return apperr.New(apperr.MetaRequestFailed, msg)
}

func (e *Engine) refreshTokens(ctx context.Context) int {
	if e.Runner == nil || e.Runner.Tokens == nil {
		return 0
	}
	pairs, err := account.List(ctx, e.DB)
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range pairs {
		if p.Account.CredentialRef == "" {
			continue
		}
		if _, err := e.Runner.Tokens.Access(ctx, p.Account.Platform, p.Account.ID); err != nil {
			if e.Log != nil {
				e.Log.Warn("token refresh failed", "alias", p.Account.Alias, "err", err)
			}
			continue
		}
		n++
	}
	return n
}
