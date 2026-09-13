# API notes (Phase 0)

Verified against official docs on 2026-09-13. Pin versions in config; do not hard-code older Graph versions in adapters.

## Meta / Instagram

Sources:

- [Content Publishing](https://developers.facebook.com/documentation/instagram-platform/content-publishing)
- [IG User Media](https://developers.facebook.com/docs/instagram-api/reference/ig-user/media)

### Eligibility

Publishing requires an Instagram professional account (Business or Creator).

Two login styles exist:

| | Instagram Login | Facebook Login |
|---|---|---|
| Host | `graph.instagram.com` | `graph.facebook.com` |
| Token | Instagram user token | Page token |
| Publish scopes | `instagram_business_basic`, `instagram_business_content_publish` | `instagram_basic`, `instagram_content_publish`, `pages_read_engagement` |

Facebook Login still needs a Page connected to the IG professional account. If that Page later requires Page Publishing Authorization, publish fails until PPA is completed.

Prefer **Instagram Login** for a CLI if it covers Reels publish with fewer Page-coupling steps; confirm app review requirements before locking the OAuth flow in Phase 3.

### Graph version

IG User Media examples list **v25.0** as the latest version as of this note. Config key: `meta.graph_version` (default `v25.0`).

### Reels publish flow

1. `POST /{ig-user-id}/media` with `media_type=REELS` and a public `video_url` (or `upload_type=resumable`).
2. Poll `GET /{container-id}?fields=status_code` until `FINISHED` (recommended: once per minute, max ~5 minutes).
3. `POST /{ig-user-id}/media_publish` with `creation_id`.

Container `status_code` values: `EXPIRED`, `ERROR`, `FINISHED`, `IN_PROGRESS`, `PUBLISHED`.

Containers expire after 24 hours. Accounts may create 400 containers / 24h. Publish is limited to **100 API-published posts / 24h** (`GET /{ig-id}/content_publishing_limit`).

Media must be on a **public HTTPS URL** — Instagram cURLs it. That is why Cloudflare R2 exists in this project. Meta also offers resumable upload to `rupload.facebook.com`; that is a future alternative, not the spec path.

Optional Reels fields we should support later: `caption`, `share_to_feed`, `cover_url`, `thumb_offset`.

Use `media_type=REELS` for standalone video. `VIDEO` is for carousel items and will fail as a standalone publish.

### Carousel publish flow

2–10 images, videos, or a mix. Reels are **not** valid carousel children.

1. For each item, `POST /{ig-user-id}/media` with `is_carousel_item=true`. Images use `image_url` (JPEG only). Videos use `media_type=VIDEO` and `video_url`.
2. Poll each child `GET /{container-id}?fields=status_code` until `FINISHED`.
3. `POST /{ig-user-id}/media` with `media_type=CAROUSEL` and `children` as a comma-separated list of child container IDs. Caption belongs on this parent container.
4. Poll the parent container, then `POST /{ig-user-id}/media_publish` with `creation_id` of the parent.

Carousel images are cropped to the first item’s aspect (default 1:1). A carousel counts as one published post. Instagram has no API for attaching its in-app music library; goggles bakes a local soundtrack with ffmpeg before upload.

JPEG is the only image format. PNG is converted locally. Max image size 8 MB.

### Reel file specs (official)

- Container: MOV or MP4, no edit lists, moov atom at the front
- Video: HEVC or H.264, progressive, closed GOP, 4:2:0
- Audio: AAC, ≤48 kHz, mono or stereo
- Frame rate: 23–60 FPS
- Max width: 1920
- Aspect ratio: 0.01:1–10:1 (recommend 9:16)
- Video bitrate: VBR, ≤25 Mbps
- Audio bitrate: 128 kbps (listed; treat as a target, not a hard probe fail)
- Duration: 3 seconds–15 minutes
- File size: **300 MB max**
- Caption: max 2200 characters, 30 hashtags, 20 @tags

## YouTube

Sources:

- [videos.insert](https://developers.google.com/youtube/v3/docs/videos/insert) (updated 2026-09-04)
- [Resumable uploads](https://developers.google.com/youtube/v3/guides/using_resumable_upload_protocol) (updated 2026-09-04)
- [channels.list](https://developers.google.com/youtube/v3/docs/channels/list) (updated 2026-09-04)

### Auth

OAuth 2.0. Minimum upload scope: `https://www.googleapis.com/auth/youtube.upload`.

Channel listing (`channels.list?mine=true`) needs a YouTube scope that can read the authorized channel (`youtube.readonly` or `youtube`). `mine=true` returns the channel(s) bound to the authorized Google identity. A single OAuth grant often maps to **one** channel; additional brand accounts typically require another consent with that channel selected. Still persist `channel_id` explicitly and never infer it from title. If `items` has more than one channel and the caller omitted `--channel-id`, fail with `YOUTUBE_CHANNEL_REQUIRED`.

### Upload

There is **no Shorts endpoint**. Use resumable `videos.insert`:

1. `POST https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status,contentDetails` with the video resource JSON.
2. Store the `Location` session URI in `jobs.provider_state_json`.
3. `PUT` file bytes; on interrupt, `PUT` with `Content-Range: bytes */SIZE` and resume from the `Range` header (`308`).
4. Success is `201` with a `video` resource.

`status.publishAt` is allowed only when `privacyStatus=private`. `status.selfDeclaredMadeForKids` should be set.

Quota: `videos.insert` costs **1600 units**. Default project quota is 10,000 units/day unless Google raises it. Uploads can also hit a separate daily upload cap.

### Shorts classification

YouTube classifies a video as a Short from **duration + aspect ratio**, not from an API flag. Current creator rule (since 2024-10-15): square or vertical, **≤ 3 minutes**. `#Shorts` in title/description is optional for discovery, not a substitute for the file shape.

goggles validates YouTube destinations as Shorts: vertical or square, duration ≤ 180s.

## Cloudflare R2

Sources:

- [Public buckets](https://developers.cloudflare.com/r2/buckets/public-buckets/) (updated 2026-06-16)
- [S3 API compatibility](https://developers.cloudflare.com/r2/api/s3/api/) (updated 2026-07-31)

- S3 endpoint: `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`
- Region: `auto` (`us-east-1` aliases to `auto`)
- Auth: S3-compatible access key + secret (store in Keychain)
- Needed ops: `PutObject`, `DeleteObject`, `HeadBucket`/`HeadObject`
- Public fetch URL is **not** the S3 endpoint. Use a custom domain on the bucket (production) or the rate-limited `r2.dev` URL (dev only).
- Instagram must be able to GET the object over public HTTPS with no auth.
- App deletes objects after IG processing; also set a bucket lifecycle (e.g. 1–2 days) as a safety net. Spec TTL is 2 hours.
