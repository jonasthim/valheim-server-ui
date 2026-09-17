package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// NotificationLogRepo is the repository for the notification_log table: one
// row per attempted alert delivery (F-1.1).
type NotificationLogRepo struct{ DB *sql.DB }

// NewNotificationLogRepo constructs a NotificationLogRepo.
func NewNotificationLogRepo(db *sql.DB) *NotificationLogRepo { return &NotificationLogRepo{DB: db} }

// Insert writes one notification_log row. entry.At (if zero) is set to now.
func (r *NotificationLogRepo) Insert(ctx context.Context, entry domain.NotificationLogEntry) error {
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO notification_log (at, channel_id, kind, instance_id, ok, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		nowString(entry.At), entry.ChannelID, entry.Kind, entry.InstanceID, boolToInt(entry.OK), entry.Error)
	if err != nil {
		return fmt.Errorf("insert notification log entry: %w", err)
	}
	return nil
}

// List returns delivery attempts newest-first, optionally paged with a
// "before" timestamp cursor (nil = no cursor, i.e. start from the newest).
func (r *NotificationLogRepo) List(ctx context.Context, limit int, before *time.Time) ([]domain.NotificationLogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, at, channel_id, kind, instance_id, ok, error FROM notification_log`
	var args []any
	if before != nil {
		q += ` WHERE at < ?`
		args = append(args, nowString(*before))
	}
	q += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list notification log entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.NotificationLogEntry
	for rows.Next() {
		var (
			e  domain.NotificationLogEntry
			at string
			ok int
		)
		if err := rows.Scan(&e.ID, &at, &e.ChannelID, &e.Kind, &e.InstanceID, &ok, &e.Error); err != nil {
			return nil, fmt.Errorf("scan notification log entry: %w", err)
		}
		e.At = parseTime(at)
		e.OK = ok != 0
		out = append(out, e)
	}
	return out, rows.Err()
}
