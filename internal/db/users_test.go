package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func testDB(t *testing.T) *Users {
	t.Helper()
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return NewUsers(sqldb)
}

func TestUsersCreateGet(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	hash := "hash1"
	got, err := u.Create(ctx, NewUser{Username: "admin", DisplayName: "Admin", Email: "a@b.com", PasswordHash: &hash, Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == 0 || got.Username != "admin" || !got.HasPassword || got.Role != domain.RoleAdmin {
		t.Fatalf("unexpected user: %+v", got)
	}
	if got.PasswordHash != hash {
		t.Fatalf("password hash not preserved: %q", got.PasswordHash)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("created_at not set")
	}

	fetched, err := u.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Username != "admin" {
		t.Fatalf("Get mismatch: %+v", fetched)
	}

	byName, err := u.GetByUsername(ctx, "ADMIN")
	if err != nil {
		t.Fatalf("GetByUsername should be case-insensitive lookup by stored lowercase: %v", err)
	}
	// NOTE: caller is responsible for lowercasing before calling; verify the
	// exact stored form still round-trips.
	if byName == nil {
		t.Fatalf("expected user")
	}
}

func TestUsersDuplicateUsername(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	if _, err := u.Create(ctx, NewUser{Username: "dup", Role: domain.RoleViewer}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := u.Create(ctx, NewUser{Username: "dup", Role: domain.RoleViewer})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeConflict {
		t.Fatalf("expected conflict code, got %v", de.Code)
	}
}

func TestUsersUpdateAndLastAdmin(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	admin, err := u.Create(ctx, NewUser{Username: "admin", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := u.CountEnabledAdmins(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expected 1 enabled admin, got %d err=%v", n, err)
	}

	name := "New Name"
	updated, err := u.Update(ctx, admin.ID, UserUpdate{DisplayName: &name})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != name {
		t.Fatalf("display name not applied: %+v", updated)
	}

	disabled := true
	if _, err := u.Update(ctx, admin.ID, UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	n, err = u.CountEnabledAdmins(ctx)
	if err != nil || n != 0 {
		t.Fatalf("expected 0 enabled admins after disable, got %d err=%v", n, err)
	}
}

func TestUsersIdentities(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	usr, err := u.Create(ctx, NewUser{Username: "oidc-user", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := u.AddIdentity(ctx, usr.ID, "https://issuer.example", "sub-123"); err != nil {
		t.Fatalf("add identity: %v", err)
	}
	found, err := u.FindByIdentity(ctx, "https://issuer.example", "sub-123")
	if err != nil {
		t.Fatalf("find by identity: %v", err)
	}
	if found.ID != usr.ID {
		t.Fatalf("wrong user resolved: %+v", found)
	}
	if len(found.Identities) != 1 || found.Identities[0].Subject != "sub-123" {
		t.Fatalf("identities not populated: %+v", found.Identities)
	}

	_, err = u.FindByIdentity(ctx, "https://issuer.example", "missing")
	if domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestUsersDeleteAndCount(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	n, err := u.Count(ctx)
	if err != nil || n != 0 {
		t.Fatalf("expected 0 users, got %d err=%v", n, err)
	}
	usr, err := u.Create(ctx, NewUser{Username: "temp", Role: domain.RoleOperator})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := u.Delete(ctx, usr.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := u.Get(ctx, usr.ID); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if err := u.Delete(ctx, usr.ID); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not found deleting twice, got %v", err)
	}
}

func TestUsersSetPasswordHash(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	usr, err := u.Create(ctx, NewUser{Username: "pw", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if usr.HasPassword {
		t.Fatalf("new oidc-only user should have no password")
	}
	hash := "newhash"
	if err := u.SetPasswordHash(ctx, usr.ID, &hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	got, err := u.Get(ctx, usr.ID)
	if err != nil || !got.HasPassword || got.PasswordHash != hash {
		t.Fatalf("password not set: %+v err=%v", got, err)
	}
	if err := u.SetPasswordHash(ctx, usr.ID, nil); err != nil {
		t.Fatalf("clear password: %v", err)
	}
	got, err = u.Get(ctx, usr.ID)
	if err != nil || got.HasPassword {
		t.Fatalf("password not cleared: %+v err=%v", got, err)
	}
}

func TestUsersUpdateLastLogin(t *testing.T) {
	u := testDB(t)
	ctx := context.Background()
	usr, err := u.Create(ctx, NewUser{Username: "login", Role: domain.RoleViewer})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now()
	if err := u.UpdateLastLogin(ctx, usr.ID, now); err != nil {
		t.Fatalf("update last login: %v", err)
	}
	got, err := u.Get(ctx, usr.ID)
	if err != nil || got.LastLoginAt == nil {
		t.Fatalf("last login not set: %+v err=%v", got, err)
	}
}
