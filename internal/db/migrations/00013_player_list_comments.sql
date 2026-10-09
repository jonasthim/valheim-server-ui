-- +goose Up
CREATE TABLE player_list_comments (
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    platform_id TEXT NOT NULL,
    comment TEXT NOT NULL,
    PRIMARY KEY (instance_id, kind, platform_id)
);

-- +goose Down
DROP TABLE player_list_comments;
