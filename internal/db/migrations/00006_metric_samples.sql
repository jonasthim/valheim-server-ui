-- +goose Up
-- Resource/player history (F-1.3): one row per instance per sample tick
-- (~60s), plus one host row (instance_id NULL) per tick. hourly=1 rows are
-- produced by Downsample and replace the raw rows they summarise.
CREATE TABLE metric_samples (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NULL,
    at          TEXT NOT NULL,
    cpu         REAL NOT NULL DEFAULT 0,
    mem         INTEGER NOT NULL DEFAULT 0,
    players     INTEGER NOT NULL DEFAULT 0,
    disk_free   INTEGER NOT NULL DEFAULT 0,
    hourly      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_metric_samples_inst_at ON metric_samples(instance_id, at);

-- +goose Down
DROP TABLE metric_samples;
