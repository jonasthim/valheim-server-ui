package auth

import (
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestPasswordOperationsRefusedForSSOLinkedAccounts(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"email": "sso@example.com", "email_verified": true, "preferred_username": "sso"})
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{}, "viewer", true, true)
	ctx := context.Background()
	local, err := svc.Create(ctx, "sso", "a-strong-local-password", "SSO", "sso@example.com", domain.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if r := doOIDCLogin(t, appServer, "/"); r.cookie == nil {
		t.Fatalf("login failed: %d", r.statusCode)
	}
	err = svc.SetPassword(ctx, local.ID, "another-strong-password")
	if err == nil || domain.AsError(err).Code != domain.CodeConflict {
		t.Fatalf("admin set password must be refused for SSO-linked user, got %v", err)
	}
	err = svc.ChangePassword(ctx, local.ID, "a-strong-local-password", "another-strong-password")
	if err == nil || domain.AsError(err).Code != domain.CodeConflict {
		t.Fatalf("self change password must be refused for SSO-linked user, got %v", err)
	}
	// A plain local user is unaffected.
	plain, err := svc.Create(ctx, "plain", "a-strong-local-password", "", "", domain.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, plain.ID, "another-strong-password"); err != nil {
		t.Fatalf("local user set password: %v", err)
	}
}
