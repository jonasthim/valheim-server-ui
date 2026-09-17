package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func newAPITokensTestDB(t *testing.T) (*Users, *APITokens) {
	t.Helper()
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return NewUsers(sqldb), NewAPITokens(sqldb)
}

func TestAPITokensCreateAndGetByHash(t *testing.T) {
	users, tokens := newAPITokensTestDB(t)
	ctx := context.Background()
	usr, err := users.Create(ctx, NewUser{Username: "u1", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	tok, err := tokens.Create(ctx, usr.ID, "ci", "hash-1", "vsui_abcdefg", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok.ID == 0 || tok.Name != "ci" || tok.Prefix != "vsui_abcdefg" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if tok.LastUsedAt != nil {
		t.Fatalf("expected nil last_used_at, got %v", tok.LastUsedAt)
	}
	if tok.ExpiresAt != nil {
		t.Fatalf("expected nil expires_at, got %v", tok.ExpiresAt)
	}

	gotUserID, got, err := tokens.GetByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if gotUserID != usr.ID || got.ID != tok.ID {
		t.Fatalf("unexpected lookup: userID=%d tok=%+v", gotUserID, got)
	}

	if _, _, err := tokens.GetByHash(ctx, "no-such-hash"); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not_found for unknown hash, got %v", err)
	}
}

func TestAPITokensCreateWithExpiry(t *testing.T) {
	users, tokens := newAPITokensTestDB(t)
	ctx := context.Background()
	usr, err := users.Create(ctx, NewUser{Username: "u1", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	tok, err := tokens.Create(ctx, usr.ID, "expiring", "hash-exp", "vsui_1111222", &expires)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok.ExpiresAt == nil || !tok.ExpiresAt.Equal(expires) {
		t.Fatalf("expected expires_at %v, got %v", expires, tok.ExpiresAt)
	}
}

func TestAPITokensListByUserNewestFirst(t *testing.T) {
	users, tokens := newAPITokensTestDB(t)
	ctx := context.Background()
	u1, err := users.Create(ctx, NewUser{Username: "u1", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create u1: %v", err)
	}
	u2, err := users.Create(ctx, NewUser{Username: "u2", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create u2: %v", err)
	}
	if _, err := tokens.Create(ctx, u1.ID, "first", "hash-a", "vsui_aaaaaaa", nil); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := tokens.Create(ctx, u1.ID, "second", "hash-b", "vsui_bbbbbbb", nil); err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := tokens.Create(ctx, u2.ID, "other-user", "hash-c", "vsui_ccccccc", nil); err != nil {
		t.Fatalf("create other user token: %v", err)
	}

	got, err := tokens.ListByUser(ctx, u1.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens for u1, got %d: %+v", len(got), got)
	}
	if got[0].Name != "second" || got[1].Name != "first" {
		t.Fatalf("expected newest first, got order %s, %s", got[0].Name, got[1].Name)
	}
}

func TestAPITokensDeleteRequiresOwnerAndTouchUpdatesLastUsed(t *testing.T) {
	users, tokens := newAPITokensTestDB(t)
	ctx := context.Background()
	owner, err := users.Create(ctx, NewUser{Username: "owner", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := users.Create(ctx, NewUser{Username: "other", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	tok, err := tokens.Create(ctx, owner.ID, "mine", "hash-owner", "vsui_ownerpr", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Deleting as a different user must behave like the token does not exist,
	// and must not actually delete it.
	if err := tokens.Delete(ctx, other.ID, tok.ID); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not_found deleting another user's token, got %v", err)
	}
	if _, _, err := tokens.GetByHash(ctx, "hash-owner"); err != nil {
		t.Fatalf("expected token to still exist after other-user delete attempt: %v", err)
	}

	touchAt := time.Now().Add(time.Minute).Truncate(time.Second)
	if err := tokens.TouchLastUsed(ctx, tok.ID, touchAt); err != nil {
		t.Fatalf("touch: %v", err)
	}
	_, got, err := tokens.GetByHash(ctx, "hash-owner")
	if err != nil {
		t.Fatalf("get after touch: %v", err)
	}
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(touchAt) {
		t.Fatalf("expected last_used_at %v, got %v", touchAt, got.LastUsedAt)
	}

	if err := tokens.Delete(ctx, owner.ID, tok.ID); err != nil {
		t.Fatalf("delete by owner: %v", err)
	}
	if _, _, err := tokens.GetByHash(ctx, "hash-owner"); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not_found after delete, got %v", err)
	}
}
