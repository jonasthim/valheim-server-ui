-- +goose Up
-- Personal API tokens (F-2.5): scripts and monitors authenticate with
-- `Authorization: Bearer vsui_...` instead of the session cookie. Only the
-- sha256 hash of the secret is stored; prefix is the first 12 characters of
-- the secret (shown in the token list so a user can match a token to a
-- script without ever seeing the secret again).
CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    hash         TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    last_used_at TEXT,
    expires_at   TEXT
);
CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);

-- +goose Down
DROP TABLE api_tokens;
