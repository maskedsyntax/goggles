# goggles

CLI-first short-form publisher for Instagram Reels and YouTube Shorts.

Humans and coding agents queue finished MP4s; `goggles` owns credentials, validation, uploads, retries, and history. No desktop or web UI.

See `spec.md` for the locked product specification.

## Status

Phase 4: Instagram Reels and YouTube Shorts immediate publish, R2 transport for Instagram, per-destination jobs and duplicate checks. Queue, scheduler, and daemon come next.

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

goggles config set r2.account_id YOUR_ACCOUNT_ID
goggles config set r2.bucket goggles-temp
goggles config set r2.public_base_url https://media.example.com
goggles storage credentials --access-key ... --secret-key ...
goggles storage test
goggles storage cleanup

goggles auth login instagram --alias patterns-instagram --access-token TOKEN --user-id IG_USER_ID --username patterns_app
goggles publish ./video.mp4 --destination patterns-instagram --caption "Hello" --json

goggles account channels youtube --access-token GOOGLE_TOKEN
goggles auth login youtube --alias patterns-youtube-main --access-token GOOGLE_TOKEN --channel-id UCabc123
goggles publish ./video.mp4 --destination patterns-youtube-main --title "Hello" --json
```

Global flags: `--json`, `--dry-run`, `--verbose`, `--quiet`, `--non-interactive`.

Config defaults to `~/.config/goggles/config.toml`. Data defaults to `~/.local/share/goggles/`. Set `GOGGLES_HOME` to override both (useful in tests).
