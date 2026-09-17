-- +goose Up
-- F-2.2: per-player playtime tracking and operator notes.
ALTER TABLE players ADD COLUMN total_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN last_session_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN note TEXT NOT NULL DEFAULT '';

CREATE TABLE player_sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL,
    platform_id TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    ended_at    TEXT NULL
);
CREATE INDEX idx_player_sessions_open ON player_sessions(instance_id, platform_id, ended_at);

-- +goose Down
-- SQLite cannot drop columns portably; total_seconds, last_session_seconds
-- and note on players are left in place (harmless extra columns).
DROP TABLE player_sessions;
