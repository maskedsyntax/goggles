CREATE TABLE platforms (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

INSERT INTO platforms (id, name, enabled, created_at) VALUES
    ('instagram', 'instagram', 1, '1970-01-01T00:00:00Z'),
    ('youtube', 'youtube', 1, '1970-01-01T00:00:00Z');

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    platform TEXT NOT NULL,
    alias TEXT NOT NULL UNIQUE,
    credential_ref TEXT,
    display_name TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (platform) REFERENCES platforms(name)
);

CREATE TABLE destinations (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    alias TEXT NOT NULL UNIQUE,
    external_id TEXT,
    external_username TEXT,
    external_title TEXT,
    timezone TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (platform) REFERENCES platforms(name)
);

CREATE UNIQUE INDEX destinations_youtube_channel
    ON destinations(platform, external_id)
    WHERE platform = 'youtube' AND external_id IS NOT NULL AND external_id != '';

CREATE TABLE profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE profile_destinations (
    profile_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    PRIMARY KEY (profile_id, destination_id),
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY (destination_id) REFERENCES destinations(id) ON DELETE CASCADE
);

CREATE TABLE schedules (
    id TEXT PRIMARY KEY,
    destination_id TEXT NOT NULL,
    time_of_day TEXT NOT NULL,
    timezone TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (destination_id, time_of_day),
    FOREIGN KEY (destination_id) REFERENCES destinations(id) ON DELETE CASCADE
);

CREATE TABLE queue_items (
    id TEXT PRIMARY KEY,
    profile_id TEXT,
    destination_id TEXT,
    file_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    generic_metadata_json TEXT NOT NULL DEFAULT '{}',
    platform_metadata_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    position INTEGER NOT NULL,
    scheduled_for TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (profile_id IS NOT NULL AND destination_id IS NULL)
        OR (profile_id IS NULL AND destination_id IS NOT NULL)
    ),
    FOREIGN KEY (profile_id) REFERENCES profiles(id),
    FOREIGN KEY (destination_id) REFERENCES destinations(id)
);

CREATE TABLE batches (
    id TEXT PRIMARY KEY,
    queue_item_id TEXT,
    source_file_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY (queue_item_id) REFERENCES queue_items(id)
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error_code TEXT,
    last_error_message TEXT,
    scheduled_for TEXT,
    external_container_id TEXT,
    external_media_id TEXT,
    provider_state_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY (batch_id) REFERENCES batches(id),
    FOREIGN KEY (destination_id) REFERENCES destinations(id)
);

CREATE INDEX jobs_status ON jobs(status);
CREATE INDEX jobs_destination_status ON jobs(destination_id, status);

CREATE TABLE publications (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    platform TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    source_file_path TEXT NOT NULL,
    external_media_id TEXT,
    external_url TEXT,
    published_at TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    FOREIGN KEY (job_id) REFERENCES jobs(id),
    FOREIGN KEY (destination_id) REFERENCES destinations(id)
);

CREATE UNIQUE INDEX publications_destination_hash
    ON publications(destination_id, content_hash);

CREATE TABLE uploads (
    id TEXT PRIMARY KEY,
    job_id TEXT,
    provider TEXT NOT NULL,
    object_key TEXT NOT NULL,
    public_url TEXT,
    file_size INTEGER,
    uploaded_at TEXT NOT NULL,
    expires_at TEXT,
    deleted_at TEXT,
    cleanup_status TEXT NOT NULL DEFAULT 'pending',
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);

CREATE INDEX uploads_cleanup ON uploads(cleanup_status, expires_at);

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    batch_id TEXT,
    job_id TEXT,
    destination_id TEXT,
    event_type TEXT NOT NULL,
    message TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
