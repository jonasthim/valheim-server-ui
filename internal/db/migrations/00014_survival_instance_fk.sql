-- +goose Up
-- Rebuild databases that already applied migration 12 before its cascade fix.
CREATE TABLE survival_moments_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    world_uid INTEGER NOT NULL,
    world_name TEXT NOT NULL,
    run_id TEXT NOT NULL,
    source_seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    character_id TEXT NOT NULL,
    player_name TEXT NOT NULL,
    day INTEGER NOT NULL,
    at TEXT NOT NULL,
    label TEXT NOT NULL,
    enemy TEXT NOT NULL,
    enemy_level INTEGER NOT NULL,
    situation TEXT NOT NULL,
    biome TEXT NOT NULL,
    x REAL,
    z REAL,
    visible INTEGER NOT NULL,
    UNIQUE(instance_id, run_id, source_seq)
);
INSERT INTO survival_moments_new
SELECT m.* FROM survival_moments m JOIN instances i ON i.id = m.instance_id;
DROP TABLE survival_moments;
ALTER TABLE survival_moments_new RENAME TO survival_moments;
CREATE INDEX idx_survival_instance_world ON survival_moments(instance_id, world_uid, day, id);

-- +goose Down
-- The cascade is intentionally retained because removing it would revive orphaned history.
SELECT 1;
