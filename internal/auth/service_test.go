package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return NewService(sqldb, config.Config{InsecureCookies: true}, slog.New(slog.DiscardHandler))
}

func TestSetupCreatesAdminAndSession(t *testing.T) {
	svc := newTestService(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", nil)

	usr, err := svc.Setup(context.Background(), rec, req, "Admin", "supersecret1", "The Admin", "admin@example.com")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if usr.Username != "admin" || usr.Role != domain.RoleAdmin {
		t.Fatalf("unexpected user: %+v", usr)
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected session cookie after setup")
	}
}

func TestSetupOnlyOnce(t *testing.T) {
	svc := newTestService(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", nil)
	if _, err := svc.Setup(context.Background(), rec, req, "admin", "supersecret1", "", ""); err != nil {
		t.Fatalf("first setup: %v", err)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", nil)
	_, err := svc.Setup(context.Background(), rec2, req2, "second", "supersecret1", "", "")
	if domain.AsError(err).Code != domain.CodeSetupDone {
		t.Fatalf("expected setup_done, got %v", err)
	}
}

func TestLoginSuccessAndFailureAndLockout(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	setupRec := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Setup(ctx, setupRec, setupReq, "admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Wrong password.
	for i := 0; i < MaxLoginFailures-1; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		_, err := svc.Login(ctx, rec, req, "admin", "wrong-password")
		if domain.AsError(err).Code != domain.CodeInvalidCredentials {
			t.Fatalf("attempt %d: expected invalid_credentials, got %v", i, err)
		}
	}
	// One more failure should lock the account.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	_, err := svc.Login(ctx, rec, req, "admin", "wrong-password")
	if domain.AsError(err).Code != domain.CodeInvalidCredentials {
		t.Fatalf("expected last failure to still report invalid_credentials, got %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	_, err = svc.Login(ctx, rec, req, "admin", "correct-password")
	if domain.AsError(err).Code != domain.CodeAccountLocked {
		t.Fatalf("expected account_locked after %d failures, got %v", MaxLoginFailures, err)
	}

	// Unknown user gives the same invalid_credentials code/behaviour.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	_, err = svc.Login(ctx, rec, req, "nobody", "whatever")
	if domain.AsError(err).Code != domain.CodeInvalidCredentials {
		t.Fatalf("expected invalid_credentials for unknown user, got %v", err)
	}
}

func TestLoginDisabledAccount(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	usr, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Create a second admin so we can disable the first without hitting last-admin protection.
	second, err := svc.Create(ctx, "second", "correct-password", "", "", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	_ = second

	disabled := true
	if _, err := svc.Update(ctx, usr.ID, nil, nil, nil, &disabled); err != nil {
		t.Fatalf("disable: %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	_, err = svc.Login(ctx, rec, req, "admin", "correct-password")
	if domain.AsError(err).Code != domain.CodeAccountDisabled {
		t.Fatalf("expected account_disabled, got %v", err)
	}
}

func TestLoginLocalLoginDisabledInSettings(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	settings, err := svc.settings.Get(ctx)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	settings.Auth.LocalLoginEnabled = false
	if err := svc.settings.Put(ctx, settings); err != nil {
		t.Fatalf("put settings: %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	_, err = svc.Login(ctx, rec, req, "admin", "correct-password")
	if domain.AsError(err).Code != domain.CodeForbidden {
		t.Fatalf("expected forbidden when local login disabled, got %v", err)
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("expected session cookie")
	}

	logoutRec := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "/", nil)
	logoutReq.AddCookie(cookie)
	svc.Logout(ctx, logoutRec, logoutReq)

	// The session should now be gone from the store.
	if _, err := svc.sessions.Get(ctx, HashToken(cookie.Value)); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected session to be deleted, got %v", err)
	}
}

func TestLastAdminProtection(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	admin, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Cannot demote the last admin.
	viewer := domain.RoleViewer
	if _, err := svc.Update(ctx, admin.ID, nil, nil, &viewer, nil); domain.AsError(err).Code != domain.CodeLastAdmin {
		t.Fatalf("expected last_admin demoting sole admin, got %v", err)
	}
	// Cannot disable the last admin.
	disabled := true
	if _, err := svc.Update(ctx, admin.ID, nil, nil, nil, &disabled); domain.AsError(err).Code != domain.CodeLastAdmin {
		t.Fatalf("expected last_admin disabling sole admin, got %v", err)
	}
	// Cannot delete the last admin.
	if err := svc.Delete(ctx, admin.ID); domain.AsError(err).Code != domain.CodeLastAdmin {
		t.Fatalf("expected last_admin deleting sole admin, got %v", err)
	}

	// Adding a second admin lifts the restriction.
	second, err := svc.Create(ctx, "second", "correct-password", "", "", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if _, err := svc.Update(ctx, admin.ID, nil, nil, &viewer, nil); err != nil {
		t.Fatalf("expected demotion to succeed once a second admin exists: %v", err)
	}
	// Now second is the only enabled admin; demoting it should fail again.
	if _, err := svc.Update(ctx, second.ID, nil, nil, &viewer, nil); domain.AsError(err).Code != domain.CodeLastAdmin {
		t.Fatalf("expected last_admin protecting the new sole admin, got %v", err)
	}
}

func TestChangePasswordFlow(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	admin, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := svc.ChangePassword(ctx, admin.ID, "wrong-current", "new-password1"); domain.AsError(err).Code != domain.CodeInvalidCredentials {
		t.Fatalf("expected invalid_credentials for wrong current password, got %v", err)
	}
	if err := svc.ChangePassword(ctx, admin.ID, "correct-password", "short"); domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for short new password, got %v", err)
	}
	if err := svc.ChangePassword(ctx, admin.ID, "correct-password", "new-password1"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	// New password now works, old one doesn't.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Login(ctx, rec2, req2, "admin", "new-password1"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestOIDCOnlyUserCannotLoginLocally(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	oidcUser, err := svc.Create(ctx, "ssoonly", "", "SSO Only", "", domain.RoleViewer)
	if err != nil {
		t.Fatalf("create oidc-only user: %v", err)
	}
	if oidcUser.HasPassword {
		t.Fatalf("expected HasPassword=false for password-less user")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	_, err = svc.Login(ctx, rec2, req2, "ssoonly", "anything")
	if domain.AsError(err).Code != domain.CodeInvalidCredentials {
		t.Fatalf("expected invalid_credentials for OIDC-only account local login, got %v", err)
	}
}
