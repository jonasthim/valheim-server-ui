-- +goose Up
-- Off-box backup targets (F-1.4): per-backup off-site copy status, shown next
-- to each row in the Backups tab with retry.
ALTER TABLE backups ADD COLUMN remote_status TEXT NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN remote_error TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE backups DROP COLUMN remote_error;
ALTER TABLE backups DROP COLUMN remote_status;
