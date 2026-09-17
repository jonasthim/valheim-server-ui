package players

import (
	"context"
	"database/sql"
	"errors"
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
	// OpenSession records that platformID started a new session on
	// instanceID at now. Any session still open for that player is closed
	// first (best effort recovery for a disconnect line the tracker missed),
	// the same way CloseSession would.
	OpenSession(ctx context.Context, instanceID, platformID string, now time.Time) error
	// CloseSession ends platformID's open session (if any) on instanceID as
	// of now, adding its duration to the player's total playtime and
	// recording it as the last session length. Returns 0, nil when there was
	// no open session to close.
	CloseSession(ctx context.Context, instanceID, platformID string, now time.Time) (time.Duration, error)
	// CloseAll closes every session still open for instanceID as of now (the
	// instance stopped, so nobody is connected any more), the same way
	// CloseSession closes one.
	CloseAll(ctx context.Context, instanceID string, now time.Time) error
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
		SELECT platform_id, name, first_seen_at, last_seen_at, session_count,
			total_seconds, last_session_seconds, note
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
		if err := rows.Scan(&kp.PlatformID, &kp.Name, &first, &last, &kp.SessionCount,
			&kp.TotalPlaySeconds, &kp.LastSessionSeconds, &kp.Note); err != nil {
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

// OpenSession implements PlayerStore.
func (s *SQLStore) OpenSession(ctx context.Context, instanceID, platformID string, now time.Time) error {
	if s.db == nil || platformID == "" {
		return nil
	}
	// A still-open session here means a previous disconnect line was missed
	// (e.g. a manager restart): close it out first so at most one session per
	// player is ever open, recovering its playtime instead of losing it.
	if _, err := s.closeOpenSession(ctx, instanceID, platformID, now); err != nil {
		return err
	}
	ts := now.UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO player_sessions (instance_id, platform_id, started_at, ended_at)
		VALUES (?, ?, ?, NULL)
	`, instanceID, platformID, ts); err != nil {
		return fmt.Errorf("players: open session %s/%s: %w", instanceID, platformID, err)
	}
	return nil
}

// CloseSession implements PlayerStore.
func (s *SQLStore) CloseSession(ctx context.Context, instanceID, platformID string, now time.Time) (time.Duration, error) {
	if s.db == nil || platformID == "" {
		return 0, nil
	}
	return s.closeOpenSession(ctx, instanceID, platformID, now)
}

// CloseAll implements PlayerStore.
func (s *SQLStore) CloseAll(ctx context.Context, instanceID string, now time.Time) error {
	if s.db == nil {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT platform_id FROM player_sessions
		WHERE instance_id = ? AND ended_at IS NULL
	`, instanceID)
	if err != nil {
		return fmt.Errorf("players: list open sessions %s: %w", instanceID, err)
	}
	var platformIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("players: scan open session %s: %w", instanceID, err)
		}
		platformIDs = append(platformIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("players: list open sessions %s: %w", instanceID, err)
	}
	_ = rows.Close()

	for _, id := range platformIDs {
		if _, err := s.closeOpenSession(ctx, instanceID, id, now); err != nil {
			return err
		}
	}
	return nil
}

// closeOpenSession closes the most recently opened still-open session (if
// any) for instanceID/platformID as of now, adds its duration to
// players.total_seconds and records it as last_session_seconds. Returns 0,
// nil when there was nothing open. Caller has already checked s.db != nil.
func (s *SQLStore) closeOpenSession(ctx context.Context, instanceID, platformID string, now time.Time) (time.Duration, error) {
	var id int64
	var startedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, started_at FROM player_sessions
		WHERE instance_id = ? AND platform_id = ? AND ended_at IS NULL
		ORDER BY started_at DESC LIMIT 1
	`, instanceID, platformID).Scan(&id, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("players: find open session %s/%s: %w", instanceID, platformID, err)
	}

	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0, fmt.Errorf("players: parse session start %s/%s: %w", instanceID, platformID, err)
	}
	dur := now.Sub(started)
	if dur < 0 {
		dur = 0
	}
	secs := int64(dur / time.Second)

	ts := now.UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `UPDATE player_sessions SET ended_at = ? WHERE id = ?`, ts, id); err != nil {
		return 0, fmt.Errorf("players: close session %d: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE players SET total_seconds = total_seconds + ?, last_session_seconds = ?
		WHERE instance_id = ? AND platform_id = ?
	`, secs, secs, instanceID, platformID); err != nil {
		return 0, fmt.Errorf("players: update playtime %s/%s: %w", instanceID, platformID, err)
	}
	return dur, nil
}

// SetNote sets the operator note for an existing known player. Returns
// domain.NotFound("player") if no row for instanceID/platformID exists.
func (s *SQLStore) SetNote(ctx context.Context, instanceID, platformID, note string) error {
	if s.db == nil {
		return nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE players SET note = ? WHERE instance_id = ? AND platform_id = ?
	`, note, instanceID, platformID)
	if err != nil {
		return fmt.Errorf("players: set note %s/%s: %w", instanceID, platformID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("players: set note %s/%s: %w", instanceID, platformID, err)
	}
	if n == 0 {
		return domain.NotFound("player")
	}
	return nil
}
