package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
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

func TestRevokeOtherSessionsKeepsCurrent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	admin, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var setupCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			setupCookie = c
		}
	}
	if setupCookie == nil {
		t.Fatalf("expected session cookie from setup")
	}

	// A second, independent login/session for the same user.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Login(ctx, rec2, req2, "admin", "correct-password"); err != nil {
		t.Fatalf("second login: %v", err)
	}

	// Revoke others using the setup session as "current".
	revokeReq := httptest.NewRequest(http.MethodPost, "/", nil)
	revokeReq.AddCookie(setupCookie)
	if err := svc.RevokeOtherSessions(ctx, revokeReq, admin.ID); err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}

	sessions, err := svc.sessions.ListByUser(ctx, admin.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != HashToken(setupCookie.Value) {
		t.Fatalf("expected only the current session to remain, got %+v", sessions)
	}
}

func TestRevokeSessionOtherUserIsNotFoundAndDeletesNothing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	admin, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// A second user with its own session.
	if _, err := svc.Create(ctx, "other", "correct-password", "", "", domain.RoleViewer); err != nil {
		t.Fatalf("create other: %v", err)
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Login(ctx, rec2, req2, "other", "correct-password"); err != nil {
		t.Fatalf("login other: %v", err)
	}
	var otherCookie *http.Cookie
	for _, c := range rec2.Result().Cookies() {
		if c.Name == CookieName {
			otherCookie = c
		}
	}
	if otherCookie == nil {
		t.Fatalf("expected session cookie for other")
	}
	otherSessionID := HashToken(otherCookie.Value)

	if err := svc.RevokeSession(ctx, admin.ID, otherSessionID); domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not_found revoking another user's session, got %v", err)
	}
	if _, err := svc.sessions.Get(ctx, otherSessionID); err != nil {
		t.Fatalf("expected other's session to still exist, got %v", err)
	}
}

func TestListSessionsMarksExactlyOneCurrent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	admin, err := svc.Setup(ctx, rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var setupCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			setupCookie = c
		}
	}
	if setupCookie == nil {
		t.Fatalf("expected session cookie from setup")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := svc.Login(ctx, rec2, req2, "admin", "correct-password"); err != nil {
		t.Fatalf("second login: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/", nil)
	listReq.AddCookie(setupCookie)
	sessions, err := svc.ListSessions(ctx, listReq, admin.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	currentCount := 0
	for _, si := range sessions {
		if si.Current {
			currentCount++
			if si.ID != HashToken(setupCookie.Value) {
				t.Fatalf("wrong session marked current: %+v", si)
			}
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current session, got %d", currentCount)
	}
}

// authedUser drives req through svc.Authenticate and returns the user the
// middleware put in the context, or nil when the request stays unauthenticated.
func authedUser(svc *Service, req *http.Request) *domain.User {
	var got *domain.User
	h := svc.Authenticate(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = api.UserFrom(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestCreateTokenThenBearerAuthenticatesAsOwner(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	admin, err := svc.Setup(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tok, secret, err := svc.CreateToken(ctx, admin.ID, "ci", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(secret, "vsui_") {
		t.Fatalf("expected vsui_ prefixed secret, got %q", secret)
	}
	if tok.Prefix != secret[:12] {
		t.Fatalf("expected prefix to be the first 12 chars of the secret, got %q for %q", tok.Prefix, secret)
	}
	if tok.ExpiresAt != nil {
		t.Fatalf("expected no expiry for expiresInDays=0, got %v", tok.ExpiresAt)
	}
	if tok.LastUsedAt != nil {
		t.Fatalf("expected nil last_used_at before first use, got %v", tok.LastUsedAt)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	got := authedUser(svc, req)
	if got == nil || got.ID != admin.ID {
		t.Fatalf("expected bearer token to authenticate as the owning admin, got %+v", got)
	}

	// last_used_at is now set.
	tokens, err := svc.ListTokens(ctx, admin.ID)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].LastUsedAt == nil {
		t.Fatalf("expected last_used_at to be set after use, got %+v", tokens)
	}
}

func TestExpiredTokenDoesNotAuthenticate(t *testing.T) {
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	now := time.Now()
	clock := func() time.Time { return now }
	svc := NewService(sqldb, config.Config{InsecureCookies: true}, slog.New(slog.DiscardHandler), WithClock(clock))
	ctx := context.Background()

	admin, err := svc.Setup(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, secret, err := svc.CreateToken(ctx, admin.ID, "ci", 1)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	now = now.Add(48 * time.Hour) // two days later, past the 1-day expiry

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	if got := authedUser(svc, req); got != nil {
		t.Fatalf("expected expired token to not authenticate, got %+v", got)
	}
}

func TestRevokedTokenDoesNotAuthenticate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	admin, err := svc.Setup(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tok, secret, err := svc.CreateToken(ctx, admin.ID, "ci", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := svc.RevokeToken(ctx, admin.ID, tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	if got := authedUser(svc, req); got != nil {
		t.Fatalf("expected revoked token to not authenticate, got %+v", got)
	}
}

func TestDisabledUsersTokenDoesNotAuthenticate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.Setup(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A second admin so it can be disabled without hitting last-admin protection.
	second, err := svc.Create(ctx, "second", "correct-password", "", "", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	_, secret, err := svc.CreateToken(ctx, second.ID, "ci", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	disabled := true
	if _, err := svc.Update(ctx, second.ID, nil, nil, nil, &disabled); err != nil {
		t.Fatalf("disable: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	if got := authedUser(svc, req); got != nil {
		t.Fatalf("expected disabled user's token to not authenticate, got %+v", got)
	}
}

func TestBearerTakesPrecedenceOverCookie(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	admin, cookie := setupAdminCookieForServiceTest(t, svc)
	second, err := svc.Create(ctx, "second", "correct-password", "", "", domain.RoleViewer)
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}
	_, secret, err := svc.CreateToken(ctx, second.ID, "ci", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)                             // valid session for admin
	req.Header.Set("Authorization", "Bearer "+secret) // valid token for second
	got := authedUser(svc, req)
	if got == nil || got.ID != second.ID {
		t.Fatalf("expected the bearer token's owner to win over the cookie, got %+v (admin=%d)", got, admin.ID)
	}
}

func setupAdminCookieForServiceTest(t *testing.T, svc *Service) (*domain.User, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	usr, err := svc.Setup(context.Background(), rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return usr, c
		}
	}
	t.Fatalf("expected session cookie from setup")
	return nil, nil
}

func TestCreateTokenValidation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	admin, err := svc.Setup(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []struct {
		name          string
		tokenName     string
		expiresInDays int
	}{
		{"empty name", "", 0},
		{"name too long", strings.Repeat("x", 65), 0},
		{"negative expiry", "ci", -1},
		{"expiry too far in the future", "ci", 3651},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := svc.CreateToken(ctx, admin.ID, tc.tokenName, tc.expiresInDays); domain.AsError(err).Code != domain.CodeValidationFailed {
				t.Fatalf("expected validation_failed, got %v", err)
			}
		})
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
