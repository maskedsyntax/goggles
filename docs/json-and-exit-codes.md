# JSON output and exit codes

Stable contract for agents. Commands that succeed print a JSON object on stdout when `--json` is set. Failures print `{"success":false,"error":{...}}` on stdout.

## Error object

```json
{
  "success": false,
  "error": {
    "code": "FILE_NOT_FOUND",
    "message": "/tmp/missing.mp4",
    "retryable": false
  }
}
```

`retryable` is true for transient platform/network failures (`META_RATE_LIMITED`, `YOUTUBE_RATE_LIMITED`, 5xx, R2 upload blips, Instagram poll timeouts). The daemon reschedules those jobs using 1 / 5 / 15 / 30 / 60 minute backoff, up to `publishing.max_retries` (default 5).

## Exit codes

| Code | When |
|------|------|
| 0 | Success |
| 1 | Generic failure |
| 2 | Invalid usage / flags |
| 3 | Account, destination, profile, file, or job not found |
| 4 | Validation (`VIDEO_INVALID`, `FILE_UNREADABLE`, `DAILY_LIMIT`) |
| 5 | Auth (`AUTH_REQUIRED`, `AUTH_EXPIRED`, `GOOGLE_AUTH_FAILED`) |
| 6 | Partial batch (`PARTIAL`) — some destinations succeeded |
| 7 | Duplicate content |

## Job status values

`queued`, `publishing`, `uploading`, `processing`, `retry_wait`, `completed`, `failed`, `cancelled`.

`retry_wait` means the job failed retryably and will be attempted again at `scheduled_for`.
