# goggles

CLI-first short-form publisher for Instagram Reels and YouTube Shorts.

Humans and coding agents queue finished MP4s; `goggles` owns credentials, validation, uploads, retries, and history. No desktop or web UI.

See `spec.md` for the locked product specification.

## Status

Phase 1 skeleton: config, SQLite, local accounts/profiles, media checks, and `doctor`. Instagram, YouTube, R2, queue, scheduler, and daemon come in later phases.

## Requirements

- Go 1.27+
- `ffprobe` (and `ffmpeg`) on `PATH` for media validation

## Build

```sh
go build -o goggles ./cmd/goggles
```

## Quick start

```sh
goggles doctor
goggles config show --json

goggles account add instagram --alias patterns-instagram --username patterns_app
goggles account add youtube --alias patterns-youtube-main --channel-id UCabc123

goggles profile create patterns
goggles profile add-destination patterns patterns-instagram
goggles profile add-destination patterns patterns-youtube-main
goggles profile show patterns --json

goggles media check ./video.mp4 --profile patterns --json
```

Global flags: `--json`, `--dry-run`, `--verbose`, `--quiet`, `--non-interactive`.

Config defaults to `~/.config/goggles/config.toml`. Data defaults to `~/.local/share/goggles/`. Set `GOGGLES_HOME` to override both (useful in tests).
