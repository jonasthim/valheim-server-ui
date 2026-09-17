-- +goose Up
-- Notification delivery log (F-1.1): one row per attempted alert delivery,
-- successful or not, shown on the Settings notifications panel.
CREATE TABLE notification_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    at          TEXT NOT NULL,
    channel_id  TEXT NOT NULL,
    kind        TEXT NOT NULL,
    instance_id TEXT NOT NULL DEFAULT '',
    ok          INTEGER NOT NULL,
    error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_notification_log_at ON notification_log(at);

-- +goose Down
DROP TABLE notification_log;
