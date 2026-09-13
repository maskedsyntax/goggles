package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

type PrepareOpts struct {
	Audio        string
	SlideSeconds float64
	ReplaceAudio bool
	WorkDir      string
}

func NeedsPrepare(items []Item, audio string) bool {
	if strings.TrimSpace(audio) != "" {
		return true
	}
	for _, it := range items {
		if it.Kind == KindImage && strings.EqualFold(filepath.Ext(it.Path), ".png") {
			return true
		}
	}
	return false
}

func Prepare(ctx context.Context, items []Item, opts PrepareOpts) ([]Item, error) {
	if !NeedsPrepare(items, opts.Audio) {
		return items, nil
	}
	if err := requireFFmpeg(); err != nil {
		return nil, err
	}
	if opts.WorkDir == "" {
		return nil, apperr.New(apperr.VideoInvalid, "carousel work directory missing")
	}
	if opts.SlideSeconds <= 0 {
		opts.SlideSeconds = DefaultSlideSeconds
	}
	if opts.SlideSeconds > MaxSlideSeconds {
		return nil, apperr.Invalid(fmt.Sprintf("--slide-seconds must be <= %.0f", MaxSlideSeconds))
	}

	audioPath := ""
	if strings.TrimSpace(opts.Audio) != "" {
		ai, err := ProbeAudio(ctx, opts.Audio)
		if err != nil {
			return nil, err
		}
		total := timelineDuration(items, opts.SlideSeconds)
		audioPath = opts.Audio
		if ai.DurationS > 0 && total > 0 && ai.DurationS+0.05 < total {
			looped := filepath.Join(opts.WorkDir, "soundtrack.m4a")
			if err := runFFmpeg(ctx,
				"-stream_loop", "-1",
				"-i", opts.Audio,
				"-t", fmt.Sprintf("%.3f", total),
				"-c:a", "aac", "-b:a", "192k",
				"-y", looped,
			); err != nil {
				return nil, err
			}
			audioPath = looped
		}
	}

	out := make([]Item, 0, len(items))
	offset := 0.0
	for i, it := range items {
		prepared, next, err := prepareOne(ctx, it, i, offset, audioPath, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, prepared)
		offset = next
	}
	return out, nil
}

func timelineDuration(items []Item, slideSeconds float64) float64 {
	total := 0.0
	for _, it := range items {
		if it.Kind == KindImage {
			total += slideSeconds
		} else if it.Info.DurationS > 0 {
			total += it.Info.DurationS
		} else {
			total += slideSeconds
		}
	}
	return total
}

func prepareOne(ctx context.Context, it Item, index int, offset float64, audioPath string, opts PrepareOpts) (Item, float64, error) {
	dur := opts.SlideSeconds
	if it.Kind != KindImage && it.Info.DurationS > 0 {
		dur = it.Info.DurationS
	}
	next := offset + dur

	if it.Kind == KindImage && audioPath == "" {
		if !strings.EqualFold(filepath.Ext(it.Path), ".png") {
			return it, next, nil
		}
		dst := filepath.Join(opts.WorkDir, fmt.Sprintf("%02d.jpg", index+1))
		if err := runFFmpeg(ctx, "-i", it.Path, "-frames:v", "1", "-q:v", "2", "-y", dst); err != nil {
			return Item{}, 0, err
		}
		info, err := Probe(ctx, dst)
		if err != nil {
			return Item{}, 0, err
		}
		return Item{Path: dst, Kind: KindImage, Info: info}, next, nil
	}

	if it.Kind == KindImage {
		dst := filepath.Join(opts.WorkDir, fmt.Sprintf("%02d.mp4", index+1))
		ss := fmt.Sprintf("%.3f", offset)
		tdur := fmt.Sprintf("%.3f", dur)
		if err := runFFmpeg(ctx,
			"-loop", "1", "-framerate", "30", "-i", it.Path,
			"-ss", ss, "-t", tdur, "-i", audioPath,
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
			"-c:v", "libx264", "-tune", "stillimage", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192k",
			"-t", tdur, "-shortest",
			"-movflags", "+faststart",
			"-y", dst,
		); err != nil {
			return Item{}, 0, err
		}
		info, err := Probe(ctx, dst)
		if err != nil {
			return Item{}, 0, err
		}
		return Item{Path: dst, Kind: KindVideo, Info: info}, next, nil
	}

	if audioPath == "" || (it.Info.HasAudio && !opts.ReplaceAudio) {
		return it, next, nil
	}

	dst := filepath.Join(opts.WorkDir, fmt.Sprintf("%02d.mp4", index+1))
	ss := fmt.Sprintf("%.3f", offset)
	tdur := fmt.Sprintf("%.3f", dur)
	copyArgs := []string{
		"-i", it.Path,
		"-ss", ss, "-t", tdur, "-i", audioPath,
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
		"-shortest",
		"-movflags", "+faststart",
		"-y", dst,
	}
	if err := runFFmpeg(ctx, copyArgs...); err != nil {
		if err := runFFmpeg(ctx,
			"-i", it.Path,
			"-ss", ss, "-t", tdur, "-i", audioPath,
			"-map", "0:v:0", "-map", "1:a:0",
			"-c:v", "libx264", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "192k",
			"-shortest",
			"-movflags", "+faststart",
			"-y", dst,
		); err != nil {
			return Item{}, 0, err
		}
	}
	info, err := Probe(ctx, dst)
	if err != nil {
		return Item{}, 0, err
	}
	return Item{Path: dst, Kind: KindVideo, Info: info}, next, nil
}

func requireFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return apperr.New(apperr.VideoInvalid, "ffmpeg is not installed")
	}
	return nil
}

func runFFmpeg(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "ffmpeg failed"
		}
		return apperr.Wrap(apperr.VideoInvalid, msg, err)
	}
	return nil
}
