# goggles — Product & Technical Specification

## 1. Overview

`goggles` is a CLI-first, multi-platform short-form video publishing tool designed for both humans and coding agents.

The initial supported platforms are:

- Instagram Reels
- YouTube Shorts

The primary goal is to automate high-volume short-form publishing across multiple Instagram accounts and multiple YouTube channels without requiring a desktop or web UI.

The CLI will:

- publish short-form videos immediately
- schedule videos for future posting
- maintain per-destination recurring posting slots
- maintain per-destination content queues
- publish the same source video to one or many destinations
- validate media before upload
- temporarily host Instagram media on Cloudflare R2
- upload YouTube media directly through the official YouTube Data API
- delete temporary R2 objects after Instagram processing
- store local publishing history
- prevent accidental duplicate publishing
- expose reliable structured JSON output for coding agents
- support unattended background execution through a daemon
- support multiple Instagram accounts
- support multiple YouTube channels

The system is intentionally designed as a Unix-style publishing tool rather than a social-media dashboard.

---

# 2. Locked Product Decisions

## 2.1 Project name

The project name and CLI binary are:

```text
goggles
```

Example:

```bash
goggles publish ./video.mp4 --profile patterns
```

---

## 2.2 Interface

The initial product is CLI-first.

There will be no desktop, web, or mobile UI in the initial version.

The CLI should remain convenient for humans while being optimized for coding agents.

---

## 2.3 Agent-first design

All important commands must support:

```bash
--json
```

Commands that may perform destructive or external actions should support:

```bash
--dry-run
```

Non-interactive operation should be supported:

```bash
--non-interactive
```

The CLI must never require a coding agent to directly handle raw Meta, Google, or Cloudflare credentials.

---

## 2.4 Platforms

Initial platform adapters:

```text
instagram
youtube
```

The architecture must be extensible so additional platforms can be added later without redesigning queueing, scheduling, history, persistence, or agent interfaces.

Potential future adapters may include TikTok or other short-form platforms, but they are explicitly out of scope for MVP.

---

## 2.5 Instagram publishing

Instagram publishing will use the official Meta / Instagram API.

No browser automation, Selenium, Playwright, scraping, password-based posting, or other unofficial publishing mechanisms.

---

## 2.6 YouTube publishing

YouTube publishing will use the official YouTube Data API.

YouTube media should be uploaded directly from the local file through the supported upload flow.

Cloudflare R2 is not required for YouTube uploads.

The system must support multiple YouTube channels.

---

## 2.7 Reel / Short media

`goggles` accepts completed video files.

Music will already be baked into the final MP4 before the file reaches `goggles`.

`goggles` will not:

- add Instagram music
- generate music
- modify music
- perform creative video editing

Music generation and final video composition belong to separate tools or workflows.

---

## 2.8 Temporary hosting

Cloudflare R2 is the selected temporary video host for Instagram.

Instagram flow:

```text
local MP4
   ↓
Cloudflare R2
   ↓
temporary public HTTPS URL
   ↓
Instagram fetches media
   ↓
Instagram finishes processing
   ↓
Instagram publishes Reel
   ↓
R2 object deleted
```

R2 is transport infrastructure, not a permanent media library.

YouTube does not use this path.

---

## 2.9 Language

Implementation language:

```text
Go
```

Reasons:

- excellent HTTP support
- strong concurrency primitives
- easy background services
- simple filesystem handling
- strong CLI ecosystem
- SQLite support
- simple single-binary distribution
- suitable for agent-oriented tooling

---

## 2.10 Local persistence

SQLite will be used for local persistent state.

SQLite stores:

- platform accounts
- profiles
- profile destinations
- publishing queues
- schedules
- posting history
- temporary upload state
- jobs
- retries
- content hashes
- errors
- platform metadata

Secrets must not be stored in plaintext SQLite.

---

## 2.11 Secrets

On macOS, secrets should be stored using macOS Keychain where practical.

Examples:

- Meta access tokens
- Google OAuth credentials / refresh tokens
- Cloudflare R2 access key
- Cloudflare R2 secret key

SQLite and config files may store only non-secret identifiers and Keychain references.

---

## 2.12 Background execution

The CLI will contain a long-running daemon mode.

Example:

```bash
goggles daemon run
```

On macOS, the daemon should support installation through `launchd`.

The scheduler must not depend on an open terminal session.

---

# 3. Core Domain Model

The core architecture must distinguish between:

```text
Platform
Account
Profile
Destination
Queue Item
Schedule
Publication Job
Publication
```

This is important because one logical brand/app may have multiple social destinations.

Example:

```text
Profile: patterns

Destinations:
├── Instagram: @patterns_app
├── YouTube: Patterns
└── YouTube: Patterns Clips
```

A profile is a logical publishing identity.

A destination is a specific platform account/channel where content can be published.

---

# 4. Platform Accounts and Destinations

## 4.1 Instagram

Each connected Instagram account is its own destination.

Example:

```text
platform: instagram
alias: patterns-instagram
username: @patterns_app
```

---

## 4.2 YouTube

Each YouTube channel is its own destination.

This must be modeled explicitly from the beginning.

A Google login must not be assumed to map to only one YouTube channel.

Examples:

```text
platform: youtube
alias: patterns-youtube-main
channel_id: UC...
channel_title: Patterns
```

and:

```text
platform: youtube
alias: patterns-youtube-clips
channel_id: UC...
channel_title: Patterns Clips
```

A single Google identity may expose multiple channels.

The OAuth/authentication layer must preserve the selected `channel_id` as part of the destination identity.

Publishing commands must always resolve to one explicit YouTube channel.

---

# 5. Profiles

Profiles provide a higher-level grouping across platforms.

Example:

```text
profile: patterns
```

Destinations:

```text
patterns-instagram
patterns-youtube-main
```

Another profile:

```text
profile: tumble
```

Destinations:

```text
tumble-instagram
tumble-youtube
```

A profile may contain:

- one destination
- one Instagram + one YouTube destination
- multiple YouTube channels
- multiple destinations on the same platform

This must not be artificially restricted.

---

# 6. Core User Story

Connect destinations:

```bash
goggles account add instagram --alias patterns-instagram
goggles account add youtube --alias patterns-youtube-main
goggles account add youtube --alias patterns-youtube-clips
```

Create profile:

```bash
goggles profile create patterns
```

Attach destinations:

```bash
goggles profile add-destination patterns patterns-instagram
goggles profile add-destination patterns patterns-youtube-main
```

Set schedules:

```bash
goggles schedule set \
  --destination patterns-instagram \
  09:00 12:30 16:00 19:30 22:30
```

```bash
goggles schedule set \
  --destination patterns-youtube-main \
  09:10 12:40 16:10 19:40 22:40
```

Queue content:

```bash
goggles queue add ./content/patterns/001.mp4 \
  --profile patterns
```

Run automation:

```bash
goggles daemon run
```

The daemon handles destination-specific publication.

---

# 7. Cross-platform Publishing

The same source video may be published to several destinations.

Example:

```bash
goggles publish ./video.mp4 \
  --profile patterns
```

This should create separate publication jobs for all enabled destinations attached to `patterns`.

Example expansion:

```text
video.mp4
   │
   ├── Instagram @patterns_app
   └── YouTube Patterns
```

Each publication job is independent.

A YouTube failure must not automatically mark the Instagram publication as failed.

A profile publish is therefore a batch operation containing multiple destination jobs.

---

# 8. Destination-specific Publishing

Publish only to Instagram:

```bash
goggles publish ./video.mp4 \
  --destination patterns-instagram
```

Publish only to one YouTube channel:

```bash
goggles publish ./video.mp4 \
  --destination patterns-youtube-main
```

Publish to all destinations in a profile:

```bash
goggles publish ./video.mp4 \
  --profile patterns
```

Publish to selected platforms:

```bash
goggles publish ./video.mp4 \
  --profile patterns \
  --platform instagram,youtube
```

---

# 9. System Architecture

```text
                    Coding Agent / Human
                            │
                            ▼
                     ┌─────────────┐
                     │   goggles   │
                     └──────┬──────┘
                            │
              ┌─────────────┼─────────────┐
              ▼             ▼             ▼
           SQLite      macOS Keychain   ffprobe
              │
              ▼
       Queue / Scheduler / Jobs
              │
              ▼
        Platform Router
          │         │
          │         │
          ▼         ▼
     Instagram    YouTube
          │         │
          ▼         │
   Cloudflare R2    │
          │         │
          ▼         ▼
      Meta API   YouTube API
          │         │
          └────┬────┘
               ▼
            History
```

---

# 10. Platform Adapter Interface

The core scheduler and queue must not know platform-specific implementation details.

Suggested Go interface:

```go
type Publisher interface {
    Platform() Platform
    Validate(ctx context.Context, req PublishRequest) error
    Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)
    Status(ctx context.Context, externalID string) (*PublishStatus, error)
}
```

Optional richer interface:

```go
type Publisher interface {
    Platform() Platform

    Validate(
        ctx context.Context,
        destination Destination,
        media Media,
        metadata PlatformMetadata,
    ) error

    Publish(
        ctx context.Context,
        destination Destination,
        media Media,
        metadata PlatformMetadata,
    ) (*PublishResult, error)

    Status(
        ctx context.Context,
        destination Destination,
        externalID string,
    ) (*PublishStatus, error)
}
```

---

# 11. Instagram Adapter

Internal flow:

```text
local MP4
   ↓
validate
   ↓
upload temporary object to R2
   ↓
create Instagram media container
   ↓
poll container processing
   ↓
publish
   ↓
delete R2 object
   ↓
record publication
```

Instagram-specific metadata may include:

```text
caption
share_to_feed
```

---

# 12. YouTube Adapter

Internal flow:

```text
local MP4
   ↓
validate
   ↓
direct / resumable upload
   ↓
YouTube video resource
   ↓
publish / scheduled publish state
   ↓
record publication
```

YouTube-specific metadata may include:

```text
title
description
tags
privacy_status
publish_at
category_id
made_for_kids
```

The exact supported fields should be verified against the current YouTube Data API before implementation.

---

# 13. CLI Command Tree

```text
goggles

├── auth
│   ├── login
│   ├── status
│   └── logout
│
├── account
│   ├── add
│   ├── remove
│   ├── list
│   ├── info
│   ├── test
│   └── channels
│
├── profile
│   ├── create
│   ├── remove
│   ├── list
│   ├── show
│   ├── add-destination
│   └── remove-destination
│
├── media
│   └── check
│
├── publish
│
├── queue
│   ├── add
│   ├── list
│   ├── fill
│   ├── remove
│   ├── clear
│   ├── pause
│   └── resume
│
├── schedule
│   ├── set
│   ├── show
│   ├── clear
│   ├── enable
│   └── disable
│
├── job
│   ├── list
│   ├── show
│   ├── retry
│   └── cancel
│
├── history
│   ├── list
│   ├── show
│   └── export
│
├── storage
│   ├── status
│   ├── test
│   └── cleanup
│
├── daemon
│   ├── run
│   ├── install
│   ├── uninstall
│   ├── status
│   └── logs
│
├── config
│   ├── show
│   ├── set
│   └── path
│
└── doctor
```

---

# 14. Global Flags

```text
--json
--dry-run
--verbose
--quiet
--non-interactive
```

When `--json` is active, stdout must contain machine-readable JSON only.

Diagnostics should go to stderr.

---

# 15. Account Management

Generic command:

```bash
goggles account add <platform>
```

Examples:

```bash
goggles account add instagram \
  --alias patterns-instagram
```

```bash
goggles account add youtube \
  --alias patterns-youtube-main
```

The account wizard may be interactive for human setup.

Agent operation should support explicit flags and `--non-interactive`.

---

# 16. YouTube Multiple-channel Discovery

After Google OAuth:

```bash
goggles account channels youtube
```

Possible output:

```text
CHANNEL ID               TITLE
UCabc123...               Patterns
UCdef456...               Patterns Clips
UCghi789...               Tumble
```

The user explicitly chooses the desired channel:

```bash
goggles account add youtube \
  --alias patterns-youtube-main \
  --channel-id UCabc123...
```

Never silently select the first returned channel.

If multiple channels exist and no channel is specified in non-interactive mode, return a structured error:

```text
YOUTUBE_CHANNEL_REQUIRED
```

---

# 17. Authentication

## 17.1 Instagram

Use current official Meta authentication requirements.

Store:

```text
access token reference
token expiry
Instagram account ID
permission state
```

---

## 17.2 YouTube

Use Google OAuth appropriate for YouTube upload access.

Store:

```text
OAuth credential reference
refresh token reference
Google identity metadata
YouTube channel ID
channel title
scope state
```

The channel ID is part of the publishing identity and must never be inferred from display name alone.

---

# 18. Media Validation

Command:

```bash
goggles media check ./video.mp4
```

Validation should use `ffprobe`.

Generic checks:

- file exists
- readable
- supported container
- supported video codec
- supported audio codec
- dimensions
- aspect ratio
- duration
- frame rate
- bitrate
- audio presence
- corruption
- file size

Platform validation:

```bash
goggles media check ./video.mp4 \
  --platform instagram
```

```bash
goggles media check ./video.mp4 \
  --platform youtube
```

Profile validation:

```bash
goggles media check ./video.mp4 \
  --profile patterns
```

Profile validation should run against all intended destinations.

---

# 19. Content Fingerprinting

Each source video receives a deterministic SHA-256 fingerprint.

The same source file may be published to multiple destinations.

Duplicate checks must therefore use:

```text
content_hash + destination_id
```

not merely:

```text
content_hash
```

This allows the same Reel to be published once to Instagram and once to YouTube while still preventing accidental duplicate posts to the same destination.

Override:

```bash
--allow-duplicate
```

Agents should not use this unless explicitly requested.

---

# 20. Publishing Jobs

A high-level publish request creates one job per destination.

Example:

```bash
goggles publish ./001.mp4 --profile patterns
```

May create:

```text
batch_01K...

jobs:
├── job_01K... → patterns-instagram
└── job_01M... → patterns-youtube-main
```

Batch state may be:

```text
pending
partial
completed
failed
```

Individual jobs retain their own platform-specific state.

---

# 21. Generic Job State Machine

```text
queued
validating
preparing
uploading
processing
publishing
published
cleanup_pending
completed
retry_wait
failed
cancelled
```

Not every platform must use every state.

For example, YouTube may skip R2-specific cleanup.

---

# 22. Platform-specific Job State

Platform adapters may persist additional state.

Instagram examples:

```text
r2_object_key
instagram_container_id
instagram_media_id
```

YouTube examples:

```text
youtube_upload_session
youtube_video_id
youtube_processing_status
```

---

# 23. Retry Strategy

Failures are classified as:

```text
retryable
permanent
```

Default:

```text
max_attempts: 5
```

Suggested backoff:

```text
1 minute
5 minutes
15 minutes
30 minutes
60 minutes
```

Each platform adapter is responsible for classifying its own API errors.

---

# 24. Queue Model

Queues should be destination-aware.

A profile queue may fan out into multiple destination jobs.

Example:

```bash
goggles queue add ./video.mp4 \
  --profile patterns
```

This creates one logical queue item associated with a profile.

When a posting slot occurs, the scheduler can create jobs for the profile destinations according to configured scheduling rules.

Alternatively, destination-specific queues may be supported:

```bash
goggles queue add ./video.mp4 \
  --destination patterns-youtube-main
```

---

# 25. Queue Behavior

Each queue item stores:

```text
source path
content hash
profile or destination target
generic metadata
platform metadata
status
created_at
```

Suggested queue states:

```text
ready
reserved
published
failed
skipped
cancelled
```

---

# 26. Recurring Schedules

Schedules belong to destinations, not platforms globally.

Example:

```bash
goggles schedule set \
  --destination patterns-instagram \
  09:00 12:30 16:00 19:30 22:30
```

```bash
goggles schedule set \
  --destination patterns-youtube-main \
  09:10 12:40 16:10 19:40 22:40
```

This matters because one profile may contain multiple YouTube channels with different posting times.

---

# 27. Profile Scheduling

Optional convenience command:

```bash
goggles schedule set \
  --profile patterns \
  09:00 12:30 16:00 19:30 22:30
```

This may apply the same slots to all destinations unless platform-specific overrides exist.

Destination-specific schedule always wins.

---

# 28. One-off Scheduling

```bash
goggles publish ./video.mp4 \
  --destination patterns-youtube-main \
  --at "2026-09-14T19:30:00+05:30"
```

or:

```bash
goggles queue add ./video.mp4 \
  --profile patterns \
  --at "2026-09-14T19:30:00+05:30"
```

Prefer explicit timestamps in v1.

---

# 29. Metadata Model

The project needs both generic and platform-specific metadata.

Suggested sidecar:

```text
001.mp4
001.yaml
```

Example:

```yaml
profile: patterns

instagram:
  caption: |
    You don't need certainty to move forward.

    #ocd #ocdrecovery
  share_to_feed: true

youtube:
  title: "You Don't Need Certainty"
  description: |
    A short reminder for anyone dealing with OCD.
  tags:
    - ocd
    - ocd recovery
    - mental health
  privacy: public
```

If a profile contains multiple YouTube channels, destination-specific override should be possible:

```yaml
youtube:
  title: "You Don't Need Certainty"
  description: "..."

destinations:
  patterns-youtube-clips:
    youtube:
      title: "OCD Reminder: You Don't Need Certainty"
```

---

# 30. Metadata Precedence

Suggested precedence:

```text
explicit CLI flags
    ↓
destination-specific sidecar metadata
    ↓
platform sidecar metadata
    ↓
profile defaults
    ↓
destination defaults
    ↓
platform defaults
```

Document this clearly so agent behavior remains deterministic.

---

# 31. Cloudflare R2

R2 is used only by adapters that require public-fetchable media.

Initial user:

```text
Instagram
```

Storage interface:

```go
type VideoHost interface {
    Upload(ctx context.Context, path string) (*HostedVideo, error)
    Delete(ctx context.Context, objectKey string) error
}
```

Default cleanup TTL:

```text
2 hours
```

---

# 32. YouTube Direct Upload

YouTube uploads should bypass R2.

Conceptual flow:

```text
local file
   ↓
YouTube upload session
   ↓
resumable media upload
   ↓
video resource
   ↓
processing / publication
```

Resume support should be used where practical so large uploads do not restart from byte zero after transient network failures.

---

# 33. Daemon

```bash
goggles daemon run
```

Responsibilities:

- schedule evaluation
- queue reservation
- job creation
- platform routing
- retry handling
- R2 cleanup
- stale job recovery
- token health checks
- publication history
- per-destination concurrency

---

# 34. launchd

```bash
goggles daemon install
goggles daemon status
goggles daemon logs
goggles daemon uninstall
```

User-level LaunchAgent.

---

# 35. SQLite Schema

## platforms

```text
id
name
enabled
created_at
```

---

## accounts

Represents authenticated platform identities.

```text
id
platform
alias
credential_ref
display_name
enabled
created_at
updated_at
```

---

## destinations

Represents the actual publish target.

```text
id
account_id
platform
alias
external_id
external_username
external_title
timezone
enabled
metadata_json
created_at
updated_at
```

For YouTube:

```text
external_id = channel_id
external_title = channel title
```

For Instagram:

```text
external_id = instagram_user_id
external_username = username
```

---

## profiles

```text
id
name
enabled
created_at
updated_at
```

---

## profile_destinations

```text
profile_id
destination_id
enabled
created_at
```

Many-to-many relation.

---

## schedules

```text
id
destination_id
time_of_day
timezone
enabled
created_at
updated_at
```

---

## queue_items

```text
id
profile_id nullable
destination_id nullable
file_path
content_hash
generic_metadata_json
platform_metadata_json
status
position
scheduled_for nullable
created_at
updated_at
```

Exactly one of `profile_id` or `destination_id` should normally be populated.

---

## batches

```text
id
queue_item_id nullable
source_file_path
content_hash
status
created_at
completed_at
```

---

## jobs

```text
id
batch_id
destination_id
platform
status
attempt_count
last_error_code
last_error_message
scheduled_for
external_container_id nullable
external_media_id nullable
provider_state_json
created_at
updated_at
completed_at
```

---

## publications

```text
id
job_id
destination_id
platform
content_hash
source_file_path
external_media_id
external_url nullable
published_at
metadata_json
created_at
```

---

## uploads

Primarily for temporary hosted transport such as R2.

```text
id
job_id
provider
object_key
public_url
file_size
uploaded_at
expires_at
deleted_at
cleanup_status
```

---

## events

```text
id
batch_id nullable
job_id nullable
destination_id nullable
event_type
message
metadata_json
created_at
```

All schema migrations must be versioned.

---

# 36. Configuration

Suggested:

```text
~/.config/goggles/config.toml
~/.local/share/goggles/goggles.db
~/.local/share/goggles/logs/
```

Example:

```toml
[general]
timezone = "Asia/Kolkata"

[publishing]
max_retries = 5
prevent_duplicates = true
global_concurrency = 3

[storage]
provider = "r2"
cleanup_ttl_minutes = 120

[r2]
account_id = "..."
bucket = "goggles-temp"
public_base_url = "https://..."
access_key_ref = "keychain:goggles-r2-access"
secret_key_ref = "keychain:goggles-r2-secret"
```

---

# 37. Agent-friendly JSON Contract

## Batch publication success

```json
{
  "success": true,
  "batch_id": "batch_01K...",
  "profile": "patterns",
  "source_file": "/Users/user/reels/001.mp4",
  "jobs": [
    {
      "job_id": "job_01K...",
      "destination": "patterns-instagram",
      "platform": "instagram",
      "status": "completed",
      "external_media_id": "123456789"
    },
    {
      "job_id": "job_01M...",
      "destination": "patterns-youtube-main",
      "platform": "youtube",
      "status": "completed",
      "external_media_id": "abcDEF123"
    }
  ]
}
```

---

## Partial batch result

```json
{
  "success": false,
  "partial": true,
  "batch_id": "batch_01K...",
  "jobs": [
    {
      "destination": "patterns-instagram",
      "status": "completed"
    },
    {
      "destination": "patterns-youtube-main",
      "status": "failed",
      "error": {
        "code": "YOUTUBE_UPLOAD_FAILED",
        "retryable": true
      }
    }
  ]
}
```

---

# 38. Error Codes

Generic:

```text
CONFIG_MISSING
ACCOUNT_NOT_FOUND
DESTINATION_NOT_FOUND
PROFILE_NOT_FOUND
ACCOUNT_DISABLED
DESTINATION_DISABLED
AUTH_REQUIRED
AUTH_EXPIRED
PERMISSION_MISSING
FILE_NOT_FOUND
FILE_UNREADABLE
VIDEO_INVALID
DUPLICATE_CONTENT
QUEUE_EMPTY
QUEUE_PAUSED
JOB_TIMEOUT
DATABASE_ERROR
KEYCHAIN_ERROR
DAEMON_UNAVAILABLE
```

Instagram:

```text
R2_UPLOAD_FAILED
R2_DELETE_FAILED
R2_URL_UNAVAILABLE
META_REQUEST_FAILED
META_RATE_LIMITED
INSTAGRAM_CONTAINER_FAILED
INSTAGRAM_PROCESSING_FAILED
INSTAGRAM_PUBLISH_FAILED
```

YouTube:

```text
GOOGLE_AUTH_FAILED
YOUTUBE_CHANNEL_REQUIRED
YOUTUBE_CHANNEL_NOT_FOUND
YOUTUBE_UPLOAD_FAILED
YOUTUBE_PROCESSING_FAILED
YOUTUBE_RATE_LIMITED
YOUTUBE_QUOTA_EXCEEDED
YOUTUBE_PUBLISH_FAILED
```

---

# 39. Safety Guards

- duplicate prevention enabled by default per destination
- explicit destination/profile resolution
- unknown destinations rejected
- disabled destinations cannot publish
- validation before upload
- secrets never printed
- permanent failures not endlessly retried
- queue items not marked complete until required jobs succeed
- dry-run support
- publication history
- per-destination daily publish limits
- per-platform concurrency limits
- no silent YouTube channel selection when multiple channels exist

Example:

```bash
goggles destination set patterns-youtube-main \
  --daily-limit 5
```

---

# 40. Concurrency

Initial suggested limits:

```text
max 1 active job per destination
global concurrency = 3
```

Platform-specific concurrency may be introduced later.

---

# 41. Rate Limits and Quotas

Each platform adapter must implement its own rate-limit handling.

Support:

- Retry-After where available
- bounded exponential backoff
- quota-aware errors
- account/destination throttling
- no tight loops

Quota errors must be visible to the agent in structured form.

---

# 42. Time Handling

Persist timestamps in UTC.

Persist destination timezone separately.

Display times in destination/user timezone.

Schedules belong to destinations because different channels/accounts may use different timing strategies.

---

# 43. File Path Handling

Queue entries may initially reference local file paths.

The source file must still exist when the publication job begins.

Future managed import:

```bash
goggles media import ./video.mp4
```

Not required for MVP.

---

# 44. Internal Go Package Structure

```text
goggles/
├── cmd/
│   ├── root.go
│   ├── auth.go
│   ├── account.go
│   ├── profile.go
│   ├── media.go
│   ├── publish.go
│   ├── queue.go
│   ├── schedule.go
│   ├── job.go
│   ├── history.go
│   ├── daemon.go
│   ├── storage.go
│   ├── config.go
│   └── doctor.go
│
├── internal/
│   ├── account/
│   ├── auth/
│   ├── config/
│   ├── db/
│   ├── daemon/
│   ├── destination/
│   ├── jobs/
│   ├── keychain/
│   ├── media/
│   ├── profile/
│   ├── queue/
│   ├── scheduler/
│   ├── platform/
│   │   ├── platform.go
│   │   ├── instagram/
│   │   │   ├── auth.go
│   │   │   ├── client.go
│   │   │   └── publisher.go
│   │   └── youtube/
│   │       ├── auth.go
│   │       ├── client.go
│   │       └── publisher.go
│   ├── storage/
│   │   ├── storage.go
│   │   └── r2/
│   ├── output/
│   └── logging/
│
├── migrations/
├── docs/
├── go.mod
├── go.sum
├── README.md
└── spec.md
```

---

# 45. Testing Strategy

Unit tests:

- profile resolution
- multi-destination fan-out
- destination-specific duplicate detection
- YouTube channel selection
- queue ordering
- schedule calculation
- state transitions
- retries
- JSON output
- config parsing
- error mapping

Integration tests:

- fake Meta API
- fake Google/YouTube API
- fake R2
- Instagram upload flow
- YouTube direct upload flow
- partial profile success
- retries
- cleanup

Recovery tests:

- daemon restart during Instagram R2 upload
- daemon restart during Instagram processing
- daemon restart during YouTube resumable upload
- daemon restart before scheduled publication
- cleanup after partial failure

---

# 46. Doctor Command

```bash
goggles doctor
```

Checks:

- config
- SQLite
- Keychain
- ffmpeg
- ffprobe
- R2
- Meta authentication
- Google authentication
- configured destinations
- YouTube channel mappings
- daemon
- writable directories
- stale R2 objects

---

# 47. Logging

Never log:

- Meta access tokens
- Google OAuth tokens
- refresh tokens
- Cloudflare secret keys
- Authorization headers
- signed secrets

May log:

- batch ID
- job ID
- profile
- destination
- platform
- source path
- external media ID
- status transition
- sanitized error

---

# 48. Music Tool Boundary

`goggles` remains independent from music generation.

External pipeline:

```text
music CLI
    ↓
soundtrack.wav

raw video
    ↓
FFmpeg
    ↓
final-video.mp4

goggles
    ↓
Instagram + YouTube
```

`goggles` only receives the finished short-form video.

---

# 49. Non-goals for MVP

Out of scope:

- Instagram music library integration
- music generation
- video editing
- browser automation
- TikTok
- Stories
- photo carousels
- DMs
- comments
- analytics dashboard
- follower analytics
- social inbox
- hosted multi-user SaaS
- automatic caption generation
- automatic video creation

---

# 50. MVP Definition

The MVP is complete when the following works reliably:

1. configure Cloudflare R2
2. authenticate Meta
3. authenticate Google/YouTube
4. connect multiple Instagram accounts
5. connect multiple YouTube channels
6. create profiles grouping destinations
7. validate a video
8. publish immediately to one Instagram destination
9. publish immediately to one YouTube destination
10. publish one source video to an Instagram + YouTube profile
11. temporarily host Instagram video via R2
12. directly upload YouTube video
13. clean R2 objects
14. queue content
15. configure destination-specific posting schedules
16. daemon publishes queued videos automatically
17. persist state in SQLite
18. expose stable JSON output
19. prevent duplicate publishing per destination
20. retry transient failures
21. survive daemon restart
22. correctly preserve multiple YouTube channel identities

---

# 51. Implementation Phases

## Phase 0 — API verification

Before implementation, verify current official documentation for:

### Meta / Instagram
- account eligibility
- authentication flow
- required permissions
- API version
- media container endpoint
- processing endpoint
- publish endpoint
- supported codecs
- supported duration
- quotas
- rate limits
- token refresh

### Google / YouTube
- OAuth flow
- required scopes
- multiple-channel discovery
- channel ID selection behavior
- video upload endpoint
- resumable upload
- scheduled publication support
- visibility restrictions
- current upload quota
- current API project compliance/audit requirements
- upload processing state

### Cloudflare R2
- public object delivery
- bucket configuration
- S3-compatible auth
- lifecycle and cleanup behavior

Do not hard-code assumptions from older API versions.

---

## Phase 1 — Core

Build:

- CLI
- config
- SQLite
- Keychain abstraction
- generic output abstraction
- error model
- media validation
- hashing
- destinations
- profiles
- platform interfaces

---

## Phase 2 — R2

Build:

- R2 auth
- upload
- public URL
- cleanup
- stale object removal

---

## Phase 3 — Instagram adapter

Build:

- Meta authentication
- Instagram destination discovery
- publish
- processing polling
- cleanup
- history

---

## Phase 4 — YouTube adapter

Build:

- Google OAuth
- channel discovery
- explicit channel selection
- multiple channel persistence
- resumable upload
- metadata
- publish status
- history

---

## Phase 5 — Cross-platform profile publishing

Build:

- profile fan-out
- batches
- independent destination jobs
- partial success handling

---

## Phase 6 — Queue

Build:

- profile queues
- destination queues
- ordering
- enqueue
- pause/resume
- failure handling

---

## Phase 7 — Scheduling

Build:

- destination schedules
- profile schedule convenience
- timezone handling
- one-off scheduling

---

## Phase 8 — Daemon

Build:

- scheduler loop
- workers
- retries
- cleanup
- job recovery
- token health checks

---

## Phase 9 — launchd

Build:

- install
- status
- logs
- uninstall

---

## Phase 10 — Agent hardening

Add:

- stable JSON schemas
- non-interactive operation
- dry-run
- daily publish limits
- channel ambiguity protection
- documented exit codes

---

# 52. Example Complete Workflow

Connect Instagram:

```bash
goggles account add instagram \
  --alias patterns-instagram
```

Connect YouTube:

```bash
goggles account channels youtube
```

Then:

```bash
goggles account add youtube \
  --alias patterns-youtube-main \
  --channel-id UCabc123...
```

Create profile:

```bash
goggles profile create patterns
```

Attach:

```bash
goggles profile add-destination \
  patterns patterns-instagram

goggles profile add-destination \
  patterns patterns-youtube-main
```

Schedule:

```bash
goggles schedule set \
  --destination patterns-instagram \
  09:00 12:30 16:00 19:30 22:30

goggles schedule set \
  --destination patterns-youtube-main \
  09:10 12:40 16:10 19:40 22:40
```

Queue:

```bash
goggles queue add ./reels/001.mp4 \
  --profile patterns
```

Run:

```bash
goggles daemon install
```

Execution:

```text
scheduled profile content
        ↓
batch created
        ↓
   ┌────┴────┐
   ▼         ▼
Instagram   YouTube
   │         │
   ▼         ▼
  R2      direct upload
   │         │
   ▼         ▼
 Meta     YouTube API
   │         │
   └────┬────┘
        ▼
      history
```

---

# 53. Agent Usage Example

User instruction:

```text
Queue the next five Patterns videos for Instagram and the main Patterns YouTube channel.
```

Agent can inspect:

```bash
goggles profile show patterns --json
goggles queue list --profile patterns --json
goggles schedule show --profile patterns --json
```

Then enqueue files:

```bash
goggles queue add ./reels/101.mp4 --profile patterns --json
goggles queue add ./reels/102.mp4 --profile patterns --json
goggles queue add ./reels/103.mp4 --profile patterns --json
goggles queue add ./reels/104.mp4 --profile patterns --json
goggles queue add ./reels/105.mp4 --profile patterns --json
```

The agent never handles raw platform credentials.

---

# 54. Design Principle

The core architectural principle is:

```text
Agents decide.
goggles executes.
```

The agent may decide:

- which video to post
- which profile/destination to use
- platform-specific metadata
- whether to queue or publish immediately

`goggles` owns:

- validation
- credentials
- channel/account identity
- platform APIs
- Cloudflare R2
- upload mechanics
- retries
- scheduling
- duplicate prevention
- persistence
- audit history

---

# 55. Success Criteria

The project succeeds when managing multiple Instagram accounts and multiple YouTube channels at high daily posting volume no longer requires manual timing or manual upload work.

The user should be able to prepare short-form content in batches, attach metadata, queue it, and trust `goggles` to publish reliably.

The final experience should feel closer to:

```text
git
ffmpeg
docker
```

than to a traditional social-media dashboard.

It should be:

- scriptable
- composable
- deterministic
- inspectable
- agent-friendly
- multi-account
- multi-channel
- multi-platform
