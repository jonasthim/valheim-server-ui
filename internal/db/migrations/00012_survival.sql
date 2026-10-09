-- +goose Up
CREATE TABLE survival_moments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL,
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
CREATE INDEX idx_survival_instance_world ON survival_moments(instance_id, world_uid, day, id);

-- +goose Down
DROP TABLE survival_moments;
