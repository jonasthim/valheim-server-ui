package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// InstanceEventRepo is the repository for the instance_events table: the
// per-instance lifecycle timeline used for crash detection (F-1.2).
type InstanceEventRepo struct{ DB *sql.DB }

// NewInstanceEventRepo constructs an InstanceEventRepo.
func NewInstanceEventRepo(db *sql.DB) *InstanceEventRepo { return &InstanceEventRepo{DB: db} }

// Insert writes one instance_events row. ev.At (if zero) is set to now.
func (r *InstanceEventRepo) Insert(ctx context.Context, ev domain.InstanceEvent) error {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO instance_events (instance_id, at, kind, detail)
		VALUES (?, ?, ?, ?)`,
		ev.InstanceID, nowString(ev.At), ev.Kind, ev.Detail)
	if err != nil {
		return fmt.Errorf("insert instance event: %w", err)
	}
	return nil
}

// List returns instanceID's events newest-first, optionally paged with a
// "before" timestamp cursor (nil = no cursor, i.e. start from the newest).
func (r *InstanceEventRepo) List(ctx context.Context, instanceID string, limit int, before *time.Time) ([]domain.InstanceEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, instance_id, at, kind, detail FROM instance_events WHERE instance_id = ?`
	args := []any{instanceID}
	if before != nil {
		q += ` AND at < ?`
		args = append(args, nowString(*before))
	}
	q += ` ORDER BY at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list instance events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.InstanceEvent
	for rows.Next() {
		ev, err := scanInstanceEvent(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan instance event: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// CountSince counts instanceID's events of the given kind at or after since.
func (r *InstanceEventRepo) CountSince(ctx context.Context, instanceID, kind string, since time.Time) (int, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM instance_events WHERE instance_id = ? AND kind = ? AND at >= ?`,
		instanceID, kind, nowString(since)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count instance events: %w", err)
	}
	return n, nil
}

// Latest returns instanceID's newest event of the given kind, or nil if none.
func (r *InstanceEventRepo) Latest(ctx context.Context, instanceID, kind string) (*domain.InstanceEvent, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, instance_id, at, kind, detail FROM instance_events
		WHERE instance_id = ? AND kind = ? ORDER BY at DESC, id DESC LIMIT 1`,
		instanceID, kind)
	ev, err := scanInstanceEvent(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest instance event: %w", err)
	}
	return &ev, nil
}

func scanInstanceEvent(scan func(...any) error) (domain.InstanceEvent, error) {
	var (
		ev domain.InstanceEvent
		at string
	)
	if err := scan(&ev.ID, &ev.InstanceID, &at, &ev.Kind, &ev.Detail); err != nil {
		return domain.InstanceEvent{}, err
	}
	ev.At = parseTime(at)
	return ev, nil
}
