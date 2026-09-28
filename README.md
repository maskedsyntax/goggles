# goggles

CLI-first publisher for Instagram Reels, YouTube Shorts, and LinkedIn posts.

Humans and coding agents publish finished media and LinkedIn text posts; `goggles` owns credentials, uploads, retries, and history. No desktop or web UI.

See `spec.md` for the locked product specification.

## Status

Schedules and a foreground/launchd daemon can consume queued videos at destination slots. Browser OAuth (loopback) and `--access-token` login both work. Instagram carousels (2–10 images/videos) can be published with an optional ffmpeg-baked soundtrack. The daemon retries transient failures (1/5/15/30/60 min), refreshes Google and Instagram tokens, and resumes in-flight YouTube uploads / Instagram containers after a crash.

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

goggles auth setup instagram --app-id APP_ID --app-secret APP_SECRET
goggles auth login instagram --alias patterns-instagram
# or: --access-token TOKEN --user-id IG_USER_ID --username patterns_app

Register `http://127.0.0.1:8787/callback` as the OAuth redirect URI in the Meta and Google consoles.
goggles publish ./video.mp4 --destination patterns-instagram --caption "Hello" --json
goggles publish ./slides --destination patterns-instagram --caption "Swipe" --audio ./song.mp3 --json
goggles publish 01.jpg 02.jpg 03.jpg --destination patterns-instagram --caption "Swipe"

goggles auth setup youtube --client-id CLIENT_ID --client-secret CLIENT_SECRET
goggles auth login youtube --alias patterns-youtube-main --channel-id UCabc123
goggles account channels youtube --alias patterns-youtube-main
goggles publish ./video.mp4 --destination patterns-youtube-main --title "Hello" --json

goggles queue add ./video.mp4 --profile patterns
goggles queue fill ./reels --profile patterns
goggles queue list --profile patterns --json

goggles schedule set --destination patterns-instagram 09:00 12:30 16:00 19:30 22:30
goggles schedule set --destination patterns-youtube-main 09:10 12:40 16:10 19:40 22:40
goggles daemon run --once --json
goggles daemon install

goggles destination set patterns-youtube-main --daily-limit 5
goggles account test patterns-instagram --json
goggles job list --json
goggles job retry job_01K... --json
goggles history list --json
```

## LinkedIn member posts

Create a LinkedIn developer app with **Share on LinkedIn** and **Sign In with LinkedIn using OpenID Connect** enabled. Add `http://127.0.0.1:8787/callback` as an authorized redirect URL. Goggles requests `w_member_social`, `openid`, and `profile` and publishes to the signed-in member's feed. LinkedIn's OAuth access tokens expire; run `auth login` again when needed. You can use `auth login linkedin --alias ... --access-token ...` with a token that has those scopes.

```sh
goggles auth setup linkedin --client-id CLIENT_ID --client-secret CLIENT_SECRET
goggles auth login linkedin --alias my-linkedin
goggles account test my-linkedin

goggles publish --destination my-linkedin --caption "A text post"
goggles publish ./photo.jpg --destination my-linkedin --caption "An image post"
goggles publish ./video.mp4 --destination my-linkedin --caption "A video post" --title "Video title"
goggles publish ./photos --destination my-linkedin --caption "A multi-image post"
goggles publish ./one.jpg ./two.png --destination my-linkedin --caption "Two images"

# Schedule one LinkedIn post (RFC3339 time with timezone).
goggles queue add --destination my-linkedin --caption "A text post" --at "2026-10-01T09:00:00+05:30"
goggles queue add ./photo.jpg --destination my-linkedin --caption "An image post" --at "2026-10-01T09:00:00+05:30"
goggles queue add ./video.mp4 --destination my-linkedin --caption "A video post" --title "Video title" --at "2026-10-01T09:00:00+05:30"

# Or set recurring slots and fill the destination queue.
goggles schedule set --destination my-linkedin --timezone Asia/Kolkata 09:00 18:00
goggles queue add ./photo.jpg --destination my-linkedin --caption "Next scheduled image"
goggles daemon install
goggles queue list --destination my-linkedin --json
```

LinkedIn supports one video, one image, or 2–20 images in a post. Mixed image/video collections are unsupported. The `linkedin.caption` and `linkedin.title` sidecar fields work alongside CLI flags. LinkedIn uploads media directly, so it does not need R2. Text-only posts require `--caption` and can be published immediately; the file-backed queue remains for media posts. LinkedIn company pages are not yet supported.

Scheduled posts are published by the Goggles daemon, so keep it running and leave queued media files at their original paths until they publish. LinkedIn tokens expire after about two months; reconnect the account before a future scheduled post if its token has expired. Queue items store `--caption` and `--title` at enqueue time; sidecar metadata remains available when those flags are omitted.

See LinkedIn's [Posts API](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api), [Images API](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/images-api), and [Videos API](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/videos-api) for API and permission details.

Metadata: optional `video.yaml` sidecar next to `video.mp4`, or `carousel.yaml` / `goggles.yaml` inside a carousel directory. Precedence is CLI flags, then destination sidecar, then platform sidecar.

Instagram carousels are 2–10 JPEG images and/or videos (`--carousel DIR`, a directory argument, or multiple files). PNG stills are converted to JPEG. `--audio` bakes a soundtrack into stills (default 3s per slide) and into silent videos; videos that already have audio keep it unless `--replace-audio`. Instagram carousels cannot be published to YouTube; LinkedIn accepts only image-only collections. Queue/daemon still consume single files.

JSON shape and process exit codes: `docs/json-and-exit-codes.md`.

Global flags: `--json`, `--dry-run`, `--verbose`, `--quiet`, `--non-interactive`.

Config defaults to `~/.config/goggles/config.toml`. Data defaults to `~/.local/share/goggles/`. Set `GOGGLES_HOME` to override both (useful in tests).
