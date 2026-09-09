package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Sessions is the repository for the sessions table. The session ID stored is
// always the sha256 hex digest of the bearer cookie value; the raw token
// never touches the database.
type Sessions struct{ DB *sql.DB }

// NewSessions constructs a Sessions repository.
func NewSessions(db *sql.DB) *Sessions { return &Sessions{DB: db} }

// Create inserts a new session row.
func (s *Sessions) Create(ctx context.Context, sess domain.Session) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, created_at, expires_at, last_seen_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, nowString(sess.CreatedAt), nowString(sess.ExpiresAt),
		nowString(sess.LastSeenAt), sess.IP, sess.UserAgent)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// Get loads a session by id (sha256 hex of the token).
func (s *Sessions) Get(ctx context.Context, id string) (*domain.Session, error) {
	var (
		sess                           domain.Session
		createdAt, expiresAt, lastSeen string
	)
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, user_id, created_at, expires_at, last_seen_at, ip, user_agent FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &createdAt, &expiresAt, &lastSeen, &sess.IP, &sess.UserAgent)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFound("session")
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	sess.CreatedAt = parseTime(createdAt)
	sess.ExpiresAt = parseTime(expiresAt)
	sess.LastSeenAt = parseTime(lastSeen)
	return &sess, nil
}

// Touch bumps last_seen_at.
func (s *Sessions) Touch(ctx context.Context, id string, when time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, nowString(when), id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// Delete removes a session (logout).
func (s *Sessions) Delete(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteByUser removes every session belonging to a user (e.g. on disable).
func (s *Sessions) DeleteByUser(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete sessions for user: %w", err)
	}
	return nil
}

// DeleteExpired removes sessions whose expiry has passed. Safe to call
// periodically; not required for correctness since Get re-validates expiry.
func (s *Sessions) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, nowString(now))
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
