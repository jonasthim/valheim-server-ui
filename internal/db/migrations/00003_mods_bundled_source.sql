-- +goose Up
-- Allow "bundled" (the manager's own agent plugin) as a mod source.
-- SQLite cannot alter a CHECK constraint in place, so rebuild the table.
-- Nothing references mods, so the drop/rename is foreign-key safe.
CREATE TABLE mods_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id  TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('thunderstore','hexium','manual','bundled')),
    owner        TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1,
    files_json   TEXT NOT NULL DEFAULT '[]',
    deps_json    TEXT NOT NULL DEFAULT '[]',
    website_url  TEXT NOT NULL DEFAULT '',
    icon_url     TEXT NOT NULL DEFAULT '',
    installed_at TEXT NOT NULL,
    updated_at   TEXT,
    UNIQUE (instance_id, owner, name)
);
INSERT INTO mods_new SELECT * FROM mods;
DROP TABLE mods;
ALTER TABLE mods_new RENAME TO mods;

-- +goose Down
-- Reverting drops any bundled-sourced rows (the agent plugin), which the old constraint forbids.
CREATE TABLE mods_old (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id  TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('thunderstore','hexium','manual')),
    owner        TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 1,
    files_json   TEXT NOT NULL DEFAULT '[]',
    deps_json    TEXT NOT NULL DEFAULT '[]',
    website_url  TEXT NOT NULL DEFAULT '',
    icon_url     TEXT NOT NULL DEFAULT '',
    installed_at TEXT NOT NULL,
    updated_at   TEXT,
    UNIQUE (instance_id, owner, name)
);
INSERT INTO mods_old SELECT * FROM mods WHERE source IN ('thunderstore','hexium','manual');
DROP TABLE mods;
ALTER TABLE mods_old RENAME TO mods;
