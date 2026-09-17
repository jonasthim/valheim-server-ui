package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestSessionsCreateGetTouchDelete(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	users := NewUsers(sqldb)
	sessions := NewSessions(sqldb)
	ctx := context.Background()

	usr, err := users.Create(ctx, NewUser{Username: "sess", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	sess := domain.Session{
		ID: "deadbeef", UserID: usr.ID, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt: now, IP: "127.0.0.1", UserAgent: "test-agent",
	}
	if err := sessions.Create(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := sessions.Get(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.UserID != usr.ID || got.IP != "127.0.0.1" || got.UserAgent != "test-agent" {
		t.Fatalf("unexpected session: %+v", got)
	}
	if !got.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("expiry mismatch: got %v want %v", got.ExpiresAt, sess.ExpiresAt)
	}

	later := now.Add(5 * time.Minute)
	if err := sessions.Touch(ctx, "deadbeef", later); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, err = sessions.Get(ctx, "deadbeef")
	if err != nil || !got.LastSeenAt.Equal(later) {
		t.Fatalf("touch not applied: %+v err=%v", got, err)
	}

	if err := sessions.Delete(ctx, "deadbeef"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := sessions.Get(ctx, "deadbeef"); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestSessionsDeleteExpiredAndByUser(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	users := NewUsers(sqldb)
	sessions := NewSessions(sqldb)
	ctx := context.Background()

	usr, err := users.Create(ctx, NewUser{Username: "u1", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	expired := domain.Session{ID: "old", UserID: usr.ID, CreatedAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(-1 * time.Hour), LastSeenAt: now.Add(-1 * time.Hour)}
	fresh := domain.Session{ID: "new", UserID: usr.ID, CreatedAt: now, ExpiresAt: now.Add(1 * time.Hour), LastSeenAt: now}
	if err := sessions.Create(ctx, expired); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	if err := sessions.Create(ctx, fresh); err != nil {
		t.Fatalf("create fresh: %v", err)
	}
	if err := sessions.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if _, err := sessions.Get(ctx, "old"); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expired session should be gone")
	}
	if _, err := sessions.Get(ctx, "new"); err != nil {
		t.Fatalf("fresh session should remain: %v", err)
	}
	if err := sessions.DeleteByUser(ctx, usr.ID); err != nil {
		t.Fatalf("delete by user: %v", err)
	}
	if _, err := sessions.Get(ctx, "new"); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("session should be gone after DeleteByUser")
	}
}

func TestSessionsListByUser(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	users := NewUsers(sqldb)
	sessions := NewSessions(sqldb)
	ctx := context.Background()

	u1, err := users.Create(ctx, NewUser{Username: "u1", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create u1: %v", err)
	}
	u2, err := users.Create(ctx, NewUser{Username: "u2", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create u2: %v", err)
	}

	now := time.Now()
	older := domain.Session{
		ID: "s-older", UserID: u1.ID, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt: now.Add(-time.Hour),
	}
	newer := domain.Session{
		ID: "s-newer", UserID: u1.ID, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(24 * time.Hour),
		LastSeenAt: now,
	}
	expired := domain.Session{
		ID: "s-expired", UserID: u1.ID, CreatedAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(-time.Hour),
		LastSeenAt: now.Add(-48 * time.Hour),
	}
	otherUsers := domain.Session{
		ID: "s-other", UserID: u2.ID, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), LastSeenAt: now,
	}
	for _, sess := range []domain.Session{older, newer, expired, otherUsers} {
		if err := sessions.Create(ctx, sess); err != nil {
			t.Fatalf("create session %s: %v", sess.ID, err)
		}
	}

	got, err := sessions.ListByUser(ctx, u1.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 active sessions for u1, got %d: %+v", len(got), got)
	}
	if got[0].ID != "s-newer" || got[1].ID != "s-older" {
		t.Fatalf("expected newest-seen first, got order %s, %s", got[0].ID, got[1].ID)
	}
}
