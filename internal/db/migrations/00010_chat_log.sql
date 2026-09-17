-- +goose Up
-- Persistent chat history (F-2.3): every in-game chat line the agent poller
-- has seen, kept across agent reconnects and world restarts, searchable via
-- GET /instances/{id}/chat.
CREATE TABLE chat_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id TEXT NOT NULL,
    at          TEXT NOT NULL,
    type        TEXT NOT NULL,
    sender      TEXT NOT NULL,
    text        TEXT NOT NULL,
    x           REAL NULL,
    z           REAL NULL,
    run_seq     INTEGER NOT NULL
);
CREATE INDEX idx_chat_log_instance_id ON chat_log(instance_id, id);

-- +goose Down
DROP TABLE chat_log;
