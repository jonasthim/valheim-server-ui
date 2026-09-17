-- +goose Up
-- F-2.1: schedules can also broadcast an announcement, run an agent command,
-- or save the world, and a restart schedule can carry its own warning lead
-- instead of the fixed default. SQLite cannot alter a CHECK constraint or
-- widen it to an open set of future kinds, so rebuild the table: drop the
-- kind CHECK (kinds are validated in Go from here on) and add payload_json
-- (the announce message or agent command, JSON) and lead_seconds (restart
-- warning lead in seconds; 0 means "use the default"). Copy every existing
-- row; legacy rows get the column defaults.
CREATE TABLE schedules_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id     TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    cron_expr       TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    only_when_empty INTEGER NOT NULL DEFAULT 1,
    note            TEXT NOT NULL DEFAULT '',
    payload_json    TEXT NOT NULL DEFAULT '{}',
    lead_seconds    INTEGER NOT NULL DEFAULT 120,
    last_run_at     TEXT,
    last_result     TEXT NOT NULL DEFAULT '',
    last_job_id     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
INSERT INTO schedules_new (id, instance_id, kind, cron_expr, enabled, only_when_empty, note,
    last_run_at, last_result, last_job_id, created_at, updated_at)
SELECT id, instance_id, kind, cron_expr, enabled, only_when_empty, note,
    last_run_at, last_result, last_job_id, created_at, updated_at
FROM schedules;
DROP TABLE schedules;
ALTER TABLE schedules_new RENAME TO schedules;
CREATE INDEX idx_schedules_instance ON schedules(instance_id);

-- +goose Down
-- Reverting drops any announce/command/save schedules (the old constraint
-- forbids them) and every schedule's payload_json/lead_seconds.
CREATE TABLE schedules_old (
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
INSERT INTO schedules_old (id, instance_id, kind, cron_expr, enabled, only_when_empty, note,
    last_run_at, last_result, last_job_id, created_at, updated_at)
SELECT id, instance_id, kind, cron_expr, enabled, only_when_empty, note,
    last_run_at, last_result, last_job_id, created_at, updated_at
FROM schedules
WHERE kind IN ('restart','backup','update');
DROP TABLE schedules;
ALTER TABLE schedules_old RENAME TO schedules;
CREATE INDEX idx_schedules_instance ON schedules(instance_id);
