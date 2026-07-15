-- Schema for st0r4g9. Applied once at startup; idempotent via IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS api_keys (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL DEFAULT '',
    key_hash   TEXT    NOT NULL UNIQUE,   -- sha256 hex of the raw key
    key_prefix TEXT    NOT NULL,          -- leading chars, safe to display
    created_at TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS buckets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    api_key_id INTEGER NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    region     TEXT    NOT NULL DEFAULT 'local-1',
    created_at TEXT    NOT NULL,
    UNIQUE(api_key_id, name)              -- per-key isolated namespace
);

CREATE TABLE IF NOT EXISTS objects (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_id    INTEGER NOT NULL REFERENCES buckets(id) ON DELETE CASCADE,
    key          TEXT    NOT NULL,
    size         INTEGER NOT NULL,
    content_type TEXT    NOT NULL DEFAULT 'application/octet-stream',
    etag         TEXT    NOT NULL,        -- sha256 hex of the content
    storage_path TEXT    NOT NULL,        -- blob path relative to the data dir
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL,
    UNIQUE(bucket_id, key)
);

CREATE INDEX IF NOT EXISTS idx_objects_bucket_key ON objects(bucket_id, key);

CREATE TABLE IF NOT EXISTS object_metadata (
    object_id INTEGER NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    name      TEXT    NOT NULL,
    value     TEXT    NOT NULL,
    PRIMARY KEY(object_id, name)
);

CREATE TABLE IF NOT EXISTS object_annotations (
    object_id INTEGER NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    name      TEXT    NOT NULL,
    value     TEXT    NOT NULL,
    PRIMARY KEY(object_id, name)
);

CREATE INDEX IF NOT EXISTS idx_annotations_name_value
    ON object_annotations(name, value);
