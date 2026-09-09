package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	josejwt "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// fakeOIDCProvider is a minimal httptest-backed OpenID Connect provider:
// discovery + JWKS + an /authorize that immediately redirects back with a
// code, and a /token that returns a signed id_token carrying whatever
// claims the test configured.
type fakeOIDCProvider struct {
	Server    *httptest.Server
	key       *rsa.PrivateKey
	kid       string
	mu        sync.Mutex
	authCodes map[string]authCodeState // code -> state captured at /authorize
	extra     map[string]any           // extra id_token claims (e.g. "groups")
	userinfo  map[string]any           // claims served by /userinfo (nil = no endpoint)
}

type authCodeState struct {
	nonce string
}

func newFakeOIDCProvider(t *testing.T, extraClaims map[string]any) *fakeOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	p := &fakeOIDCProvider{key: key, kid: "test-key-1", authCodes: map[string]authCodeState{}, extra: extraClaims}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/jwks", p.jwks)
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	mux.HandleFunc("/userinfo", p.userInfo)
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	return p
}

func (p *fakeOIDCProvider) discovery(w http.ResponseWriter, r *http.Request) {
	doc := map[string]any{
		"issuer":                                p.Server.URL,
		"authorization_endpoint":                p.Server.URL + "/authorize",
		"token_endpoint":                        p.Server.URL + "/token",
		"jwks_uri":                              p.Server.URL + "/jwks",
		"userinfo_endpoint":                     p.Server.URL + "/userinfo",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func (p *fakeOIDCProvider) jwks(w http.ResponseWriter, r *http.Request) {
	set := josejwt.JSONWebKeySet{Keys: []josejwt.JSONWebKey{{
		Key: &p.key.PublicKey, KeyID: p.kid, Algorithm: "RS256", Use: "sig",
	}}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(set)
}

func (p *fakeOIDCProvider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := fmt.Sprintf("code-%d", time.Now().UnixNano())
	p.mu.Lock()
	p.authCodes[code] = authCodeState{nonce: q.Get("nonce")}
	p.mu.Unlock()

	redirectURI := q.Get("redirect_uri")
	http.Redirect(w, r, redirectURI+"?code="+code+"&state="+q.Get("state"), http.StatusFound)
}

func (p *fakeOIDCProvider) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	code := r.Form.Get("code")
	p.mu.Lock()
	st, ok := p.authCodes[code]
	delete(p.authCodes, code)
	p.mu.Unlock()
	if !ok {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	clientID := r.Form.Get("client_id")
	if clientID == "" {
		if u, _, ok := r.BasicAuth(); ok {
			clientID = u
		}
	}
	claims := map[string]any{
		"iss":   p.Server.URL,
		"sub":   "user-123",
		"aud":   clientID,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": st.nonce,
	}
	for k, v := range p.extra {
		claims[k] = v
	}
	idToken := p.signClaims(claims)

	resp := map[string]any{
		"access_token": "fake-access-token",
		"token_type":   "Bearer",
		"id_token":     idToken,
		"expires_in":   3600,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (p *fakeOIDCProvider) userInfo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer fake-access-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	ui := p.userinfo
	p.mu.Unlock()
	if ui == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body := map[string]any{"sub": "user-123"}
	for k, v := range ui {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (p *fakeOIDCProvider) signClaims(claims map[string]any) string {
	signer, err := josejwt.NewSigner(josejwt.SigningKey{Algorithm: josejwt.RS256, Key: p.key},
		(&josejwt.SignerOptions{}).WithType("JWT").WithHeader("kid", p.kid))
	if err != nil {
		panic(err)
	}
	tok, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		panic(err)
	}
	return tok
}

// newOIDCTestSetup wires a Service with OIDC enabled against provider, and a
// tiny HTTP mux exposing the login/callback endpoints plus a catch-all "/" so
// the final post-login redirect has somewhere to land.
func newOIDCTestSetup(t *testing.T, provider *fakeOIDCProvider, roleMapping map[string]domain.Role, defaultRole string, autoCreate, syncRoles bool) (*Service, *httptest.Server) {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	mux := http.NewServeMux()
	appServer := httptest.NewServer(mux)
	t.Cleanup(appServer.Close)

	cfg := config.Config{BaseURL: appServer.URL, InsecureCookies: true}
	svc := NewService(sqldb, cfg, slog.New(slog.DiscardHandler))

	settingsRepo := db.NewSettingsRepo(sqldb)
	settings := domain.DefaultSettings()
	settings.Auth.OIDC.Enabled = true
	settings.Auth.OIDC.IssuerURL = provider.Server.URL
	settings.Auth.OIDC.ClientID = "test-client"
	settings.Auth.OIDC.ClientSecret = "test-secret"
	settings.Auth.OIDC.Scopes = []string{"openid", "profile", "email", "groups"}
	settings.Auth.OIDC.GroupsClaim = "groups"
	settings.Auth.OIDC.RoleMapping = roleMapping
	settings.Auth.OIDC.DefaultRole = defaultRole
	settings.Auth.OIDC.AutoCreateUsers = autoCreate
	settings.Auth.OIDC.SyncRoles = syncRoles
	if err := settingsRepo.Put(context.Background(), settings); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	mux.HandleFunc("/api/v1/auth/oidc/login", svc.OIDCLogin)
	mux.HandleFunc("/api/v1/auth/oidc/callback", svc.OIDCCallback)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	return svc, appServer
}

// oidcLoginResult is the outcome of driving the full browser-side OIDC
// login flow through a real net/http client, without leaking the final
// *http.Response to callers (its body is closed here).
type oidcLoginResult struct {
	statusCode int
	cookie     *http.Cookie
}

func doOIDCLogin(t *testing.T, appServer *httptest.Server, next string) oidcLoginResult {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	url := appServer.URL + "/api/v1/auth/oidc/login"
	if next != "" {
		url += "?next=" + next
	}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	result := oidcLoginResult{statusCode: resp.StatusCode}
	if u := resp.Request.URL; u != nil {
		for _, c := range jar.Cookies(u) {
			if c.Name == CookieName {
				result.cookie = c
			}
		}
	}
	return result
}

func TestOIDCLoginCreatesUserWithMappedRole(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{
		"groups":             []string{"admins"},
		"preferred_username": "alice",
		"email":              "alice@example.com",
	})
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{"admins": domain.RoleAdmin}, "viewer", true, true)

	result := doOIDCLogin(t, appServer, "/dashboard")
	if result.statusCode != http.StatusOK {
		t.Fatalf("final response status = %d, want 200", result.statusCode)
	}
	if result.cookie == nil {
		t.Fatalf("expected session cookie to be set after successful oidc login")
	}

	usr, err := svc.users.GetByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("expected user 'alice' to be auto-created: %v", err)
	}
	if usr.Role != domain.RoleAdmin {
		t.Fatalf("expected role admin from group mapping, got %v", usr.Role)
	}
	if len(usr.Identities) != 1 || usr.Identities[0].Subject != "user-123" {
		t.Fatalf("expected identity linked, got %+v", usr.Identities)
	}
}

func TestOIDCLoginDeniedWhenDefaultRoleDeny(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"groups": []string{"randoms"}, "email": "bob@example.com"})
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{"admins": domain.RoleAdmin}, "deny", true, true)

	result := doOIDCLogin(t, appServer, "")
	if result.cookie != nil {
		t.Fatalf("expected no session cookie when default_role=deny blocks login")
	}
	if result.statusCode != http.StatusOK { // lands on "/login" served by our catch-all "/"
		t.Fatalf("unexpected status: %d", result.statusCode)
	}
	if n, _ := svc.users.Count(context.Background()); n != 0 {
		t.Fatalf("expected no user created when denied")
	}
}

func TestOIDCLoginAutoCreateDisabledRejectsUnknownUser(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"groups": []string{}, "email": "carol@example.com"})
	svc, appServer := newOIDCTestSetup(t, provider, nil, "viewer", false, true)

	result := doOIDCLogin(t, appServer, "")
	if result.cookie != nil {
		t.Fatalf("expected login to be rejected when auto_create_users is false")
	}
	if n, _ := svc.users.Count(context.Background()); n != 0 {
		t.Fatalf("expected no user created")
	}
}

func TestOIDCLoginSyncRolesUpdatesExistingUser(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"groups": []string{"operators"}, "preferred_username": "dave"})
	svc, appServer := newOIDCTestSetup(t, provider,
		map[string]domain.Role{"admins": domain.RoleAdmin, "operators": domain.RoleOperator}, "viewer", true, true)

	// First login creates the user as operator.
	first := doOIDCLogin(t, appServer, "")
	if first.cookie == nil {
		t.Fatalf("expected first login to succeed")
	}
	usr, err := svc.users.GetByUsername(context.Background(), "dave")
	if err != nil || usr.Role != domain.RoleOperator {
		t.Fatalf("expected operator role after first login: %+v err=%v", usr, err)
	}

	// Change the mapping in the provider's claims to grant admin, and log in
	// again: sync_roles should promote the existing user.
	provider.mu.Lock()
	provider.extra["groups"] = []string{"admins"}
	provider.mu.Unlock()

	second := doOIDCLogin(t, appServer, "")
	if second.cookie == nil {
		t.Fatalf("expected second login to succeed")
	}
	usr, err = svc.users.GetByUsername(context.Background(), "dave")
	if err != nil || usr.Role != domain.RoleAdmin {
		t.Fatalf("expected role synced to admin on second login: %+v err=%v", usr, err)
	}

	n, err := svc.users.Count(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("expected exactly one user (no duplicate created), got %d err=%v", n, err)
	}
}

func TestOIDCLoginDisabledUserRejected(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"groups": []string{}, "preferred_username": "erin"})
	svc, appServer := newOIDCTestSetup(t, provider, nil, "viewer", true, true)

	result := doOIDCLogin(t, appServer, "")
	if result.cookie == nil {
		t.Fatalf("expected first login to succeed")
	}
	usr, err := svc.users.GetByUsername(context.Background(), "erin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	disabled := true
	if _, err := svc.Update(context.Background(), usr.ID, nil, nil, nil, &disabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	second := doOIDCLogin(t, appServer, "")
	if second.cookie != nil {
		t.Fatalf("expected login to be rejected for a disabled account")
	}
}

func TestOIDCLoginDisabledSettingIs404(t *testing.T) {
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	svc := NewService(sqldb, config.Config{}, slog.New(slog.DiscardHandler))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	svc.OIDCLogin(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when oidc disabled, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != string(domain.CodeOIDCDisabled) {
		t.Fatalf("expected code oidc_disabled, got %q", body.Error.Code)
	}
}

func TestOIDCCallbackRejectsBadState(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{})
	svc, appServer := newOIDCTestSetup(t, provider, nil, "viewer", true, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, appServer.URL+"/api/v1/auth/oidc/callback?state=bogus&code=x", nil)
	svc.OIDCCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect on bad state, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/login?error=oidc_error" {
		t.Fatalf("expected redirect to /login?error=oidc_error, got %q", loc)
	}
}

// Groups (and profile claims) delivered only through UserInfo must still map
// to a role: Authelia, Google and default Keycloak/Authentik configurations do
// not put them in the ID token.
func TestOIDCLoginUsesUserInfoClaimsWhenIDTokenLacksThem(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{})
	provider.userinfo = map[string]any{
		"groups":             []string{"valheim-admins"},
		"preferred_username": "bob",
		"email":              "bob@example.com",
		"name":               "Bob",
	}
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{"valheim-admins": domain.RoleAdmin}, "deny", true, true)

	result := doOIDCLogin(t, appServer, "/")
	if result.cookie == nil {
		t.Fatalf("expected a session cookie; final status %d", result.statusCode)
	}
	usr, err := svc.users.GetByUsername(context.Background(), "bob")
	if err != nil {
		t.Fatalf("expected user 'bob' created from userinfo claims: %v", err)
	}
	if usr.Role != domain.RoleAdmin || usr.Email != "bob@example.com" || usr.DisplayName != "Bob" {
		t.Fatalf("userinfo claims not applied: %+v", usr)
	}
}

// ID token claims win over UserInfo when both are present.
func TestOIDCLoginIDTokenClaimsTakePrecedence(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{"groups": []string{"ops"}, "preferred_username": "carol"})
	provider.userinfo = map[string]any{"groups": []string{"admins"}}
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{"admins": domain.RoleAdmin, "ops": domain.RoleOperator}, "deny", true, true)
	if result := doOIDCLogin(t, appServer, "/"); result.cookie == nil {
		t.Fatalf("expected a session cookie; final status %d", result.statusCode)
	}
	usr, err := svc.users.GetByUsername(context.Background(), "carol")
	if err != nil {
		t.Fatal(err)
	}
	if usr.Role != domain.RoleOperator {
		t.Fatalf("expected id_token groups to win, got role %v", usr.Role)
	}
}

// A local account with the same verified email is merged: the SSO identity is
// linked, no second user is created, and the password stays usable.
func TestOIDCLoginMergesWithLocalUserByVerifiedEmail(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{
		"email":              "Jonas@Example.com",
		"email_verified":     true,
		"preferred_username": "jonas.sso",
		"groups":             []string{"admins"},
	})
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{"admins": domain.RoleAdmin}, "viewer", true, false)
	ctx := context.Background()
	local, err := svc.Create(ctx, "jonas", "a-strong-local-password", "Jonas", "jonas@example.com", domain.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}

	if result := doOIDCLogin(t, appServer, "/"); result.cookie == nil {
		t.Fatalf("expected a session; final status %d", result.statusCode)
	}
	users, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected the local user to be reused, got %d users", len(users))
	}
	merged, err := svc.Get(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Identities) != 1 || merged.Identities[0].Subject != "user-123" {
		t.Fatalf("identity not linked: %+v", merged.Identities)
	}
	if !merged.HasPassword || merged.Role != domain.RoleOperator {
		t.Fatalf("merge must keep password and role when sync_roles is off: %+v", merged)
	}
	if _, err := svc.users.GetByUsername(ctx, "jonas.sso"); err == nil {
		t.Fatalf("no separate SSO user must be created")
	}
}

// email_verified=false must never link to a local account.
func TestOIDCLoginDoesNotMergeOnUnverifiedEmail(t *testing.T) {
	provider := newFakeOIDCProvider(t, map[string]any{
		"email":              "victim@example.com",
		"email_verified":     false,
		"preferred_username": "attacker",
	})
	svc, appServer := newOIDCTestSetup(t, provider, map[string]domain.Role{}, "viewer", true, true)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "victim", "a-strong-local-password", "Victim", "victim@example.com", domain.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if result := doOIDCLogin(t, appServer, "/"); result.cookie == nil {
		t.Fatalf("expected a session for the new separate user; final status %d", result.statusCode)
	}
	victim, err := svc.users.GetByUsername(ctx, "victim")
	if err != nil {
		t.Fatal(err)
	}
	if len(victim.Identities) != 0 {
		t.Fatalf("unverified email must not link: %+v", victim.Identities)
	}
	if _, err := svc.users.GetByUsername(ctx, "attacker"); err != nil {
		t.Fatalf("expected a separate auto-created user: %v", err)
	}
}
