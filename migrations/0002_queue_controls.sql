CREATE TABLE queue_controls (
    scope TEXT PRIMARY KEY,
    paused INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);
