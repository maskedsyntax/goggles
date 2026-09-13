package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
	"github.com/maskedsyntax/goggles/internal/platform"
)

type Info struct {
	Path       string        `json:"path"`
	Hash       string        `json:"hash"`
	SizeBytes  int64         `json:"size_bytes"`
	Container  string        `json:"container"`
	Duration   time.Duration `json:"duration_ns"`
	DurationS  float64       `json:"duration_seconds"`
	Bitrate    int64         `json:"bitrate"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	Aspect     float64       `json:"aspect_ratio"`
	FrameRate  float64       `json:"frame_rate"`
	VideoCodec string        `json:"video_codec"`
	AudioCodec string        `json:"audio_codec"`
	HasAudio   bool          `json:"has_audio"`
	HasVideo   bool          `json:"has_video"`
}

type Check struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"` // error or warning
	Message  string `json:"message"`
}

type Report struct {
	OK       bool    `json:"ok"`
	Info     Info    `json:"info"`
	Checks   []Check `json:"checks"`
	Platform string  `json:"platform,omitempty"`
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", mapOpenError(path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", apperr.Wrap(apperr.FileUnreadable, "cannot hash file", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Probe(ctx context.Context, path string) (Info, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Info{}, mapOpenError(path, err)
	}
	if st.IsDir() {
		return Info{}, apperr.New(apperr.VideoInvalid, "path is a directory")
	}

	hash, err := HashFile(path)
	if err != nil {
		return Info{}, err
	}

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-hide_banner",
		"-loglevel", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		msg := "ffprobe failed"
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		} else if err != nil && strings.Contains(err.Error(), "executable file not found") {
			return Info{}, apperr.New(apperr.VideoInvalid, "ffprobe is not installed")
		}
		return Info{}, apperr.Wrap(apperr.VideoInvalid, msg, err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return Info{}, apperr.Wrap(apperr.VideoInvalid, "cannot parse ffprobe output", err)
	}

	info := Info{
		Path:      path,
		Hash:      hash,
		SizeBytes: st.Size(),
		Container: probe.Format.FormatName,
	}
	if probe.Format.Duration != "" {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			info.DurationS = d
			info.Duration = time.Duration(d * float64(time.Second))
		}
	}
	if probe.Format.BitRate != "" {
		if n, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
			info.Bitrate = n
		}
	}
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "video":
			if info.HasVideo {
				continue
			}
			info.HasVideo = true
			info.VideoCodec = s.CodecName
			info.Width = s.Width
			info.Height = s.Height
			if info.Height > 0 {
				info.Aspect = float64(info.Width) / float64(info.Height)
			}
			info.FrameRate = parseFrameRate(s.AvgFrameRate)
			if info.FrameRate == 0 {
				info.FrameRate = parseFrameRate(s.RFrameRate)
			}
		case "audio":
			if info.HasAudio {
				continue
			}
			info.HasAudio = true
			info.AudioCodec = s.CodecName
		}
	}
	if !info.HasVideo {
		return Info{}, apperr.New(apperr.VideoInvalid, "no video stream found")
	}
	return info, nil
}

func CheckFile(ctx context.Context, path string, platforms []platform.Platform) (Report, error) {
	info, err := Probe(ctx, path)
	if err != nil {
		return Report{}, err
	}
	rep := Report{OK: true, Info: info}
	rep.Checks = append(rep.Checks, genericChecks(info)...)
	for _, p := range platforms {
		rep.Checks = append(rep.Checks, platformChecks(info, p)...)
	}
	if len(platforms) == 1 {
		rep.Platform = string(platforms[0])
	}
	for _, c := range rep.Checks {
		if !c.OK && c.Severity == "error" {
			rep.OK = false
		}
	}
	if !rep.OK {
		return rep, apperr.New(apperr.VideoInvalid, "video failed validation")
	}
	return rep, nil
}

func ToPlatformMedia(info Info) platform.Media {
	return platform.Media{
		Path:       info.Path,
		Hash:       info.Hash,
		Container:  info.Container,
		VideoCodec: info.VideoCodec,
		AudioCodec: info.AudioCodec,
		Width:      info.Width,
		Height:     info.Height,
		Duration:   info.Duration,
		FrameRate:  info.FrameRate,
		Bitrate:    info.Bitrate,
		SizeBytes:  info.SizeBytes,
		HasAudio:   info.HasAudio,
	}
}

func genericChecks(info Info) []Check {
	var out []Check
	out = append(out, okCheck("exists", "file exists and is readable"))
	out = append(out, okCheck("video_stream", fmt.Sprintf("video codec %s %dx%d", info.VideoCodec, info.Width, info.Height)))
	if info.HasAudio {
		out = append(out, okCheck("audio_stream", "audio codec "+info.AudioCodec))
	} else {
		out = append(out, Check{Name: "audio_stream", OK: false, Severity: "warning", Message: "no audio stream"})
	}
	if info.SizeBytes == 0 {
		out = append(out, Check{Name: "file_size", OK: false, Severity: "error", Message: "file is empty"})
	} else {
		out = append(out, okCheck("file_size", fmt.Sprintf("%d bytes", info.SizeBytes)))
	}
	if info.DurationS <= 0 {
		out = append(out, Check{Name: "duration", OK: false, Severity: "error", Message: "duration is missing"})
	} else {
		out = append(out, okCheck("duration", fmt.Sprintf("%.3fs", info.DurationS)))
	}
	return out
}

func platformChecks(info Info, p platform.Platform) []Check {
	switch p {
	case platform.Instagram:
		return instagramChecks(info)
	case platform.YouTube:
		return youtubeChecks(info)
	default:
		return nil
	}
}

func instagramChecks(info Info) []Check {
	var out []Check
	if !containerAllowed(info.Container, "mp4", "mov") {
		out = append(out, fail("instagram.container", "expected mp4 or mov, got "+info.Container))
	} else {
		out = append(out, okCheck("instagram.container", info.Container))
	}
	if !codecAllowed(info.VideoCodec, "h264", "hevc", "h265") {
		out = append(out, fail("instagram.video_codec", "expected h264 or hevc, got "+info.VideoCodec))
	} else {
		out = append(out, okCheck("instagram.video_codec", info.VideoCodec))
	}
	if info.HasAudio && !codecAllowed(info.AudioCodec, "aac") {
		out = append(out, fail("instagram.audio_codec", "expected aac, got "+info.AudioCodec))
	} else if info.HasAudio {
		out = append(out, okCheck("instagram.audio_codec", info.AudioCodec))
	}
	if info.Width > 1920 {
		out = append(out, fail("instagram.width", "max width is 1920"))
	}
	if info.DurationS < 3 || info.DurationS > 15*60 {
		out = append(out, fail("instagram.duration", "must be between 3s and 15m"))
	} else {
		out = append(out, okCheck("instagram.duration", fmt.Sprintf("%.3fs", info.DurationS)))
	}
	if info.SizeBytes > 300*1024*1024 {
		out = append(out, fail("instagram.file_size", "max size is 300MB"))
	}
	if info.FrameRate > 0 && (info.FrameRate < 23 || info.FrameRate > 60) {
		out = append(out, fail("instagram.frame_rate", "must be 23-60 fps"))
	}
	if info.Aspect < 0.01 || info.Aspect > 10 {
		out = append(out, fail("instagram.aspect", "aspect ratio out of range"))
	} else if !near(info.Aspect, 9.0/16.0, 0.03) {
		out = append(out, Check{Name: "instagram.aspect", OK: true, Severity: "warning", Message: "9:16 is recommended for Reels"})
	} else {
		out = append(out, okCheck("instagram.aspect", fmt.Sprintf("%.4f", info.Aspect)))
	}
	return out
}

func youtubeChecks(info Info) []Check {
	var out []Check
	if info.DurationS > 180 {
		out = append(out, fail("youtube.duration", "Shorts must be 180 seconds or less"))
	} else {
		out = append(out, okCheck("youtube.duration", fmt.Sprintf("%.3fs", info.DurationS)))
	}
	if info.Width > info.Height {
		out = append(out, fail("youtube.aspect", "Shorts must be vertical or square"))
	} else if !near(info.Aspect, 9.0/16.0, 0.03) && info.Width != info.Height {
		out = append(out, Check{Name: "youtube.aspect", OK: true, Severity: "warning", Message: "9:16 is recommended for Shorts"})
	} else {
		out = append(out, okCheck("youtube.aspect", fmt.Sprintf("%.4f", info.Aspect)))
	}
	return out
}

func okCheck(name, msg string) Check {
	return Check{Name: name, OK: true, Severity: "info", Message: msg}
}

func fail(name, msg string) Check {
	return Check{Name: name, OK: false, Severity: "error", Message: msg}
}

func containerAllowed(got string, want ...string) bool {
	g := strings.ToLower(got)
	for _, w := range want {
		if strings.Contains(g, w) {
			return true
		}
	}
	return false
}

func codecAllowed(got string, want ...string) bool {
	g := strings.ToLower(got)
	for _, w := range want {
		if g == w {
			return true
		}
	}
	return false
}

func near(got, want, tol float64) bool {
	if got > want {
		return got-want <= tol
	}
	return want-got <= tol
}

func parseFrameRate(s string) float64 {
	if s == "" || s == "0/0" {
		return 0
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

func mapOpenError(path string, err error) error {
	if os.IsNotExist(err) {
		return apperr.New(apperr.FileNotFound, path)
	}
	return apperr.Wrap(apperr.FileUnreadable, "cannot read "+path, err)
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	RFrameRate   string `json:"r_frame_rate"`
	AvgFrameRate string `json:"avg_frame_rate"`
}

type ffprobeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}
