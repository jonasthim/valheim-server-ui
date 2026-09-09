-- +goose Up
-- Authoritative data model. Timestamps are RFC3339 UTC strings.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,            -- lowercase
    display_name  TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    password_hash TEXT,                            -- NULL for OIDC-only accounts
    role          TEXT NOT NULL CHECK (role IN ('viewer','operator','admin')),
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    last_login_at TEXT
);

CREATE TABLE user_identities (
    provider   TEXT NOT NULL,                      -- issuer URL
    subject    TEXT NOT NULL,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (provider, subject)
);
CREATE INDEX idx_user_identities_user ON user_identities(user_id);

CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,                 -- sha256 hex of the cookie token
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ip           TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    doc        TEXT NOT NULL,                      -- JSON domain.Settings
    updated_at TEXT NOT NULL
);

CREATE TABLE instances (
    id                 TEXT PRIMARY KEY,           -- slug
    name               TEXT NOT NULL,
    config_json        TEXT NOT NULL,              -- JSON domain.InstanceConfig
    autostart          INTEGER NOT NULL DEFAULT 0,
    pending_restart    INTEGER NOT NULL DEFAULT 0,
    installed_buildid  TEXT NOT NULL DEFAULT '',
    latest_buildid     TEXT NOT NULL DEFAULT '',
    buildid_checked_at TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE TABLE jobs (
    id           TEXT PRIMARY KEY,                 -- uuid
    type         TEXT NOT NULL,
    instance_id  TEXT,                             -- NULL for global jobs; not FK so history survives deletes
    status       TEXT NOT NULL CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
    title        TEXT NOT NULL DEFAULT '',
    requested_by TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    started_at   TEXT,
    finished_at  TEXT,
    error        TEXT NOT NULL DEFAULT '',
    summary_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_jobs_instance_created ON jobs(instance_id, created_at DESC);
CREATE INDEX idx_jobs_status ON jobs(status);

CREATE TABLE backups (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    world       TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('manual','scheduled','pre_update','pre_restore','uploaded')),
    filename    TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    UNIQUE (instance_id, filename)
);
CREATE INDEX idx_backups_instance_created ON backups(instance_id, created_at DESC);

CREATE TABLE schedules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id     TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('restart','backup','update')),
    cron_expr       TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    only_when_empty INTEGER NOT NULL DEFAULT 1,
    note            TEXT NOT NULL DEFAULT '',
    last_run_at     TEXT,
    last_result     TEXT NOT NULL DEFAULT '',
    last_job_id     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_schedules_instance ON schedules(instance_id);

CREATE TABLE mods (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id  TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('thunderstore','manual')),
    owner        TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1,
    files_json   TEXT NOT NULL DEFAULT '[]',       -- paths relative to server dir
    deps_json    TEXT NOT NULL DEFAULT '[]',
    website_url  TEXT NOT NULL DEFAULT '',
    icon_url     TEXT NOT NULL DEFAULT '',
    installed_at TEXT NOT NULL,
    updated_at   TEXT,
    UNIQUE (instance_id, owner, name)
);

CREATE TABLE players (
    instance_id   TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    platform_id   TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    first_seen_at TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    session_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (instance_id, platform_id)
);

CREATE TABLE audit_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ts           TEXT NOT NULL,
    user_id      INTEGER,
    username     TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    instance_id  TEXT NOT NULL DEFAULT '',
    target       TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}',
    ip           TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_ts ON audit_log(ts DESC);
CREATE INDEX idx_audit_instance ON audit_log(instance_id, ts DESC);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE players;
DROP TABLE mods;
DROP TABLE schedules;
DROP TABLE backups;
DROP TABLE jobs;
DROP TABLE instances;
DROP TABLE settings;
DROP TABLE sessions;
DROP TABLE user_identities;
DROP TABLE users;
