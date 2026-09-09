package players

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// PlayerStore persists the known-players registry (the `players` table,
// ARCHITECTURE.md §5). Implementations must be safe for concurrent use.
type PlayerStore interface {
	// Upsert records that platformID joined instanceID: creates the row on
	// first sight (first_seen_at = now), otherwise bumps last_seen_at and
	// session_count. name is stored when non-empty; an empty name never
	// clobbers a previously known one.
	Upsert(ctx context.Context, instanceID, platformID, name string, now time.Time) error
	// UpdateName sets name and touches last_seen_at without incrementing
	// session_count (used when the log heuristic later learns a name for an
	// id that was already recorded by Upsert).
	UpdateName(ctx context.Context, instanceID, platformID, name string, now time.Time) error
	// List returns known players for instanceID ordered by last_seen_at
	// descending, at most limit rows.
	List(ctx context.Context, instanceID string, limit int) ([]domain.KnownPlayer, error)
}

// SQLStore is the PlayerStore backed by the shared manager database.
type SQLStore struct{ db *sql.DB }

// NewSQLStore wraps db. db may be nil, in which case every method is a no-op
// returning empty results — useful for tests that don't need persistence.
func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

func (s *SQLStore) Upsert(ctx context.Context, instanceID, platformID, name string, now time.Time) error {
	if s.db == nil || platformID == "" {
		return nil
	}
	ts := now.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO players (instance_id, platform_id, name, first_seen_at, last_seen_at, session_count)
		VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(instance_id, platform_id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			session_count = players.session_count + 1,
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE players.name END
	`, instanceID, platformID, name, ts, ts)
	if err != nil {
		return fmt.Errorf("players: upsert %s/%s: %w", instanceID, platformID, err)
	}
	return nil
}

func (s *SQLStore) UpdateName(ctx context.Context, instanceID, platformID, name string, now time.Time) error {
	if s.db == nil || platformID == "" {
		return nil
	}
	ts := now.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO players (instance_id, platform_id, name, first_seen_at, last_seen_at, session_count)
		VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(instance_id, platform_id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			name = excluded.name
	`, instanceID, platformID, name, ts, ts)
	if err != nil {
		return fmt.Errorf("players: update name %s/%s: %w", instanceID, platformID, err)
	}
	return nil
}

func (s *SQLStore) List(ctx context.Context, instanceID string, limit int) ([]domain.KnownPlayer, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT platform_id, name, first_seen_at, last_seen_at, session_count
		FROM players WHERE instance_id = ?
		ORDER BY last_seen_at DESC
		LIMIT ?
	`, instanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("players: list %s: %w", instanceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.KnownPlayer
	for rows.Next() {
		var kp domain.KnownPlayer
		var first, last string
		if err := rows.Scan(&kp.PlatformID, &kp.Name, &first, &last, &kp.SessionCount); err != nil {
			return nil, fmt.Errorf("players: scan %s: %w", instanceID, err)
		}
		kp.FirstSeenAt, _ = time.Parse(time.RFC3339, first)
		kp.LastSeenAt, _ = time.Parse(time.RFC3339, last)
		out = append(out, kp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("players: list %s: %w", instanceID, err)
	}
	return out, nil
}
