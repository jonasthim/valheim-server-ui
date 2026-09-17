// Package api_test exercises the real WP-01 stack through api.NewRouter as
// an external test package: internal/auth imports internal/api (for
// WithUser/ClientIP), so an in-package (package api) test importing
// internal/auth would be an import cycle. An external test package has no
// such restriction because it is compiled separately from package api.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/audit"
	"github.com/jonasthim/valheim-server-ui/internal/auth"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// newAuthTestRouter wires the real WP-01 stack (auth.Service + audit.Recorder
// backed by an in-memory database) behind api.NewRouter, exactly as
// cmd/valheim-ui/wire_auth.go does for the real binary.
func newAuthTestRouter(t *testing.T) (http.Handler, *auth.Service) {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	log := slog.New(slog.DiscardHandler)
	auditRepo := db.NewAuditRepo(sqldb)
	recorder := audit.New(auditRepo, log)
	svc := auth.NewService(sqldb, config.Config{InsecureCookies: true}, log, auth.WithAuditor(recorder))
	settingsRepo := db.NewSettingsRepo(sqldb)
	settings := auth.NewSettings(settingsRepo, config.Config{InsecureCookies: true}, nil)

	deps := &api.Deps{
		Log: log, DB: sqldb, Version: "test",
		Auth: svc, Audit: recorder, Users: svc, Settings: settings,
	}
	return api.NewRouter(deps, nil), svc
}

// newAuthTestRouterWithSessions is newAuthTestRouter plus a SessionService
// (F-2.7), which the real WP-01 stack does not provide on its own. It also
// returns the audit recorder so tests can assert on recorded entries.
func newAuthTestRouterWithSessions(t *testing.T, sessions api.SessionService) (http.Handler, *auth.Service, *audit.Recorder) {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	log := slog.New(slog.DiscardHandler)
	auditRepo := db.NewAuditRepo(sqldb)
	recorder := audit.New(auditRepo, log)
	svc := auth.NewService(sqldb, config.Config{InsecureCookies: true}, log, auth.WithAuditor(recorder))
	settingsRepo := db.NewSettingsRepo(sqldb)
	settings := auth.NewSettings(settingsRepo, config.Config{InsecureCookies: true}, nil)

	deps := &api.Deps{
		Log: log, DB: sqldb, Version: "test",
		Auth: svc, Audit: recorder, Users: svc, Settings: settings, Sessions: sessions,
	}
	return api.NewRouter(deps, nil), svc, recorder
}

// fakeSessionService is a test double for api.SessionService that records
// what it was called with so handler tests can assert on it independently of
// the real auth.Service implementation (which is exercised separately in
// internal/auth/service_test.go).
type fakeSessionService struct {
	sessions []domain.SessionInfo

	revokeCalled bool
	revokeUserID int64
	revokeSessID string
	revokeErr    error

	othersCalled bool
	othersUserID int64
	othersErr    error
}

func (f *fakeSessionService) ListSessions(_ context.Context, _ *http.Request, _ int64) ([]domain.SessionInfo, error) {
	return f.sessions, nil
}

func (f *fakeSessionService) RevokeSession(_ context.Context, userID int64, sessionID string) error {
	f.revokeCalled = true
	f.revokeUserID = userID
	f.revokeSessID = sessionID
	return f.revokeErr
}

func (f *fakeSessionService) RevokeOtherSessions(_ context.Context, _ *http.Request, userID int64) error {
	f.othersCalled = true
	f.othersUserID = userID
	return f.othersErr
}

// setupAdminCookie creates the first admin account directly through svc and
// returns the user and the session cookie set for it, for tests that need an
// authenticated request without going through the HTTP /auth/setup route.
func setupAdminCookie(t *testing.T, svc *auth.Service) (*domain.User, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	usr, err := svc.Setup(context.Background(), rec, req, "admin", "correct-password", "", "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return usr, c
		}
	}
	t.Fatalf("expected session cookie from setup")
	return nil, nil
}

func TestAuthSessions_ListReturnsCurrentFlag(t *testing.T) {
	fake := &fakeSessionService{sessions: []domain.SessionInfo{
		{ID: "sess-1", IP: "127.0.0.1", UserAgent: "test-agent", Current: true},
		{ID: "sess-2", IP: "10.0.0.2", UserAgent: "other-agent", Current: false},
	}}
	h, svc, _ := newAuthTestRouterWithSessions(t, fake)
	_, cookie := setupAdminCookie(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET sessions: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sessions []domain.SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(body.Sessions))
	}
	if !body.Sessions[0].Current || body.Sessions[1].Current {
		t.Fatalf("expected only the first session marked current: %+v", body.Sessions)
	}
}

func TestAuthSessions_DeleteCallsServiceAndAudits(t *testing.T) {
	fake := &fakeSessionService{}
	h, svc, recorder := newAuthTestRouterWithSessions(t, fake)
	usr, cookie := setupAdminCookie(t, svc)

	req := mutatingRequest(http.MethodDelete, "/api/v1/auth/sessions/some-session-id", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE session: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !fake.revokeCalled {
		t.Fatalf("expected RevokeSession to be called")
	}
	if fake.revokeUserID != usr.ID {
		t.Fatalf("expected RevokeSession called with user id %d, got %d", usr.ID, fake.revokeUserID)
	}
	if fake.revokeSessID != "some-session-id" {
		t.Fatalf("expected RevokeSession called with path session id, got %q", fake.revokeSessID)
	}

	entries, err := recorder.List(context.Background(), "", "", 10, 0)
	if err != nil {
		t.Fatalf("audit List: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "auth.session.revoke" && e.Target == "some-session-id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an audit entry for auth.session.revoke, got %+v", entries)
	}
}

func TestAuthSessions_RevokeOthersReturns204(t *testing.T) {
	fake := &fakeSessionService{}
	h, svc, _ := newAuthTestRouterWithSessions(t, fake)
	usr, cookie := setupAdminCookie(t, svc)

	req := mutatingRequest(http.MethodPost, "/api/v1/auth/sessions/revoke-others", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke-others: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !fake.othersCalled {
		t.Fatalf("expected RevokeOtherSessions to be called")
	}
	if fake.othersUserID != usr.ID {
		t.Fatalf("expected RevokeOtherSessions called with user id %d, got %d", usr.ID, fake.othersUserID)
	}
}

func TestAuthSessions_NotConfiguredWhenSessionsNil(t *testing.T) {
	h, svc := newAuthTestRouter(t)
	_, cookie := setupAdminCookie(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when Sessions is not configured, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func mutatingRequest(method, path string, body *bytes.Reader) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set(api.CSRFHeader, api.CSRFHeaderValue)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestAuthFlow_SetupLoginMeLogoutMe(t *testing.T) {
	h, _ := newAuthTestRouter(t)

	// GET /auth/status before setup: needs_setup=true.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var status struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.NeedsSetup {
		t.Fatalf("expected needs_setup=true before any user exists")
	}

	// POST /auth/setup.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/setup", jsonBody(t, map[string]any{
		"username": "admin", "password": "correct-password", "display_name": "Admin",
	})))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: code=%d body=%s", rec.Code, rec.Body.String())
	}
	setupCookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range setupCookies {
		if c.Name == auth.CookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected session cookie from setup")
	}

	// A second setup attempt is now a 404 setup_done.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/setup", jsonBody(t, map[string]any{
		"username": "someoneelse", "password": "correct-password",
	})))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second setup: expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}

	// GET /auth/me without a cookie is 401.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without cookie: expected 401, got %d", rec.Code)
	}

	// Fresh login (a separate "browser") to get an independent session.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, map[string]any{
		"username": "admin", "password": "correct-password",
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var loginCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			loginCookie = c
		}
	}
	if loginCookie == nil {
		t.Fatalf("expected session cookie from login")
	}

	// GET /auth/me with the cookie succeeds.
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(loginCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, meReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var meBody struct {
		User struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meBody.User.Username != "admin" || meBody.User.Role != "admin" {
		t.Fatalf("unexpected /me body: %+v", meBody)
	}

	// POST /auth/logout.
	logoutReq := mutatingRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(loginCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, logoutReq)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// GET /auth/me with the now-logged-out cookie is 401 again.
	meReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq2.AddCookie(loginCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, meReq2)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: expected 401, got %d", rec.Code)
	}
}

func TestAuthFlow_LoginInvalidCredentials(t *testing.T) {
	h, svc := newAuthTestRouter(t)
	if _, err := svc.Setup(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil),
		"admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, map[string]any{
		"username": "admin", "password": "wrong",
	})))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, map[string]any{
		"username": "no-such-user", "password": "whatever",
	})))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown user, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthFlow_UsersAndSettingsRequireAdmin(t *testing.T) {
	h, svc := newAuthTestRouter(t)
	if _, err := svc.Setup(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil),
		"admin", "correct-password", "", ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Create a viewer directly through the service.
	if _, err := svc.Create(context.Background(), "viewer1", "correct-password", "", "", "viewer"); err != nil {
		t.Fatalf("create viewer: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mutatingRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, map[string]any{
		"username": "viewer1", "password": "correct-password",
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer login: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var viewerCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			viewerCookie = c
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.AddCookie(viewerCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer forbidden from /users, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.AddCookie(viewerCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer forbidden from /settings, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	req.AddCookie(viewerCookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer forbidden from /audit, got %d body=%s", rec.Code, rec.Body.String())
	}
}
