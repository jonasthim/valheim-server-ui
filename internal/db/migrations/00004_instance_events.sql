-- +goose Up
-- Per-instance lifecycle timeline (F-1.2): start/ready/stop/crash/update
-- markers, used for the Overview "Recent events" list and the 24h crash count.
CREATE TABLE instance_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL,
    at          TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('start','ready','stop','crash','update')),
    detail      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_instance_events_instance_at ON instance_events(instance_id, at);

-- +goose Down
DROP TABLE instance_events;
