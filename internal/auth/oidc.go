package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

const (
	oidcCallbackPath    = "/api/v1/auth/oidc/callback"
	oidcStateCookie     = "vsui_oidc_state"
	oidcStateCookiePath = "/api/v1/auth/oidc"
	oidcStateTTL        = 10 * time.Minute
)

// oidcRuntime is the set of objects built from one snapshot of OIDCSettings.
type oidcRuntime struct {
	settings     domain.OIDCSettings
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth2Config oauth2.Config
}

// oidcState is the payload of the short-lived state cookie set before
// redirecting to the provider.
type oidcState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"x"`
}

// InvalidateOIDC forces the next call to getOIDCRuntime to rebuild the
// provider. Wire it as the onChange callback passed to NewSettings so a
// successful Settings.Put picks up a changed issuer/client/secret on the
// next OIDC login without a restart.
func (s *Service) InvalidateOIDC() {
	s.oidcMu.Lock()
	s.oidcGen++
	s.oidcMu.Unlock()
}

// getOIDCRuntime lazily builds (and caches) the OIDC provider/verifier/oauth2
// config from the current settings. It is rebuilt whenever the settings
// generation counter or the OIDC settings themselves change.
func (s *Service) getOIDCRuntime(ctx context.Context) (*oidcRuntime, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Auth.OIDC.Enabled {
		return nil, domain.E(domain.CodeOIDCDisabled, "oidc login is not enabled")
	}

	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidcRuntime != nil && s.oidcBuiltGen == s.oidcGen && reflect.DeepEqual(s.oidcRuntime.settings, settings.Auth.OIDC) {
		return s.oidcRuntime, nil
	}

	provider, err := oidc.NewProvider(ctx, settings.Auth.OIDC.IssuerURL)
	if err != nil {
		return nil, domain.Wrap(domain.CodeOIDCError, "discover OIDC provider", err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: settings.Auth.OIDC.ClientID})
	rt := &oidcRuntime{
		settings: settings.Auth.OIDC,
		provider: provider,
		verifier: verifier,
		oauth2Config: oauth2.Config{
			ClientID:     settings.Auth.OIDC.ClientID,
			ClientSecret: settings.Auth.OIDC.ClientSecret,
			RedirectURL:  s.cfg.BaseURL + oidcCallbackPath,
			Endpoint:     provider.Endpoint(),
			Scopes:       settings.Auth.OIDC.Scopes,
		},
	}
	s.oidcRuntime = rt
	s.oidcBuiltGen = s.oidcGen
	return rt, nil
}

// clearOIDCStateCookie deletes the short-lived state cookie set by
// OIDCLogin, once OIDCCallback has consumed (or rejected) it.
func clearOIDCStateCookie(w http.ResponseWriter, cfg config.Config) {
	//nolint:gosec // HttpOnly/SameSite are set; Secure is intentionally derived from cfg.InsecureCookies.
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: "", Path: oidcStateCookiePath,
		HttpOnly: true, Secure: !cfg.InsecureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: -1, Expires: time.Unix(0, 0),
	})
}

func randToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// OIDCLogin starts the authorization code + PKCE flow: GET /auth/oidc/login.
func (s *Service) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := s.getOIDCRuntime(ctx)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	next := r.URL.Query().Get("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	state, err := randToken(16)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	nonce, err := randToken(16)
	if err != nil {
		api.WriteError(w, err)
		return
	}
	verifier := oauth2.GenerateVerifier()

	payload, err := json.Marshal(oidcState{State: state, Nonce: nonce, Verifier: verifier, Next: next})
	if err != nil {
		api.WriteError(w, domain.Wrap(domain.CodeInternal, "encode oidc state", err))
		return
	}
	//nolint:gosec // HttpOnly/SameSite are set; Secure is intentionally derived from cfg.InsecureCookies.
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: base64.RawURLEncoding.EncodeToString(payload),
		Path: oidcStateCookiePath, HttpOnly: true, Secure: !s.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: s.clock().Add(oidcStateTTL),
	})

	authURL := rt.oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// OIDCCallback completes the flow: GET /auth/oidc/callback.
func (s *Service) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(code domain.ErrorCode) {
		clearOIDCStateCookie(w, s.cfg)
		//nolint:gosec // code is always one of our fixed domain.ErrorCode constants, never user input
		http.Redirect(w, r, "/login?error="+string(code), http.StatusFound)
	}

	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil || stateCookie.Value == "" {
		fail(domain.CodeOIDCError)
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(stateCookie.Value)
	if err != nil {
		fail(domain.CodeOIDCError)
		return
	}
	var st oidcState
	if err := json.Unmarshal(raw, &st); err != nil {
		fail(domain.CodeOIDCError)
		return
	}
	clearOIDCStateCookie(w, s.cfg)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		fail(domain.CodeOIDCError)
		return
	}
	if r.URL.Query().Get("state") != st.State || st.State == "" {
		fail(domain.CodeOIDCError)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		fail(domain.CodeOIDCError)
		return
	}

	rt, err := s.getOIDCRuntime(ctx)
	if err != nil {
		fail(domain.AsError(err).Code)
		return
	}
	tok, err := rt.oauth2Config.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		fail(domain.CodeOIDCError)
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		fail(domain.CodeOIDCError)
		return
	}
	idToken, err := rt.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		fail(domain.CodeOIDCError)
		return
	}
	if idToken.Nonce != st.Nonce {
		fail(domain.CodeOIDCError)
		return
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		fail(domain.CodeOIDCError)
		return
	}
	// Many providers (Authelia, Google, Authentik and Keycloak by default) put
	// groups, email or name only in the UserInfo response, not in the ID token.
	// Merge UserInfo claims for keys the ID token does not carry; a UserInfo
	// failure is not fatal since the ID token alone was verified.
	s.mergeUserInfo(ctx, rt, tok, claims)

	usr, linked, err := s.resolveOIDCUser(ctx, rt.settings, idToken.Subject, claims)
	if err != nil {
		s.log.Warn("oidc login rejected", "err", err)
		fail(domain.AsError(err).Code)
		return
	}
	if linked && s.auditor != nil {
		s.auditor.Record(r, "auth.oidc.link", "", usr.Username, map[string]any{"issuer": rt.settings.IssuerURL, "method": "email"})
	}
	if err := s.createSession(ctx, w, r, usr); err != nil {
		fail(domain.CodeInternal)
		return
	}
	_ = s.users.UpdateLastLogin(ctx, usr.ID, s.clock())
	if s.auditor != nil {
		s.auditor.Record(r, "auth.login", "", usr.Username, map[string]any{"method": "oidc"})
	}
	http.Redirect(w, r, st.Next, http.StatusFound)
}

// TestOIDCDiscovery performs discovery only against issuerURL, used by
// POST /settings/oidc/test. It needs no service state, so it is a free
// function rather than a method.
func TestOIDCDiscovery(ctx context.Context, issuerURL string) (ok bool, issuer, authEndpoint, errMsg string) {
	if issuerURL == "" {
		return false, "", "", "issuer_url is required"
	}
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return false, "", "", err.Error()
	}
	return true, issuerURL, provider.Endpoint().AuthURL, ""
}

// resolveOIDCUser maps a verified token to a local user: link an existing
// identity, auto-create on first login, apply/sync role mapping.
func (s *Service) resolveOIDCUser(ctx context.Context, settings domain.OIDCSettings, subject string, claims map[string]any) (usr *domain.User, linked bool, err error) {
	role, allowed := mapRole(settings.RoleMapping, extractGroups(claims[groupsClaimOrDefault(settings)]), settings.DefaultRole)
	if !allowed {
		return nil, false, domain.E(domain.CodeForbidden, "no role mapped for this account")
	}

	usr, err = s.users.FindByIdentity(ctx, settings.IssuerURL, subject)
	if err != nil {
		if domain.AsError(err).Code != domain.CodeNotFound {
			return nil, false, err
		}
		usr = nil
	}

	if usr == nil {
		// Merge with an existing local account that has the same (verified)
		// email: the SSO identity is linked to it, the password and role stay.
		if existing := s.matchByVerifiedEmail(ctx, claims); existing != nil {
			if existing.Disabled {
				return nil, false, domain.E(domain.CodeAccountDisabled, "account disabled")
			}
			if err := s.users.AddIdentity(ctx, existing.ID, settings.IssuerURL, subject); err != nil {
				return nil, false, err
			}
			s.log.Info("oidc: linked identity to existing account by email", "user", existing.Username)
			linked = true
			usr = existing
			if usr.Identities == nil {
				usr.Identities = []domain.Identity{}
			}
			usr.Identities = append(usr.Identities, domain.Identity{Provider: settings.IssuerURL, Subject: subject})
		}
	}

	if usr == nil {
		if !settings.AutoCreateUsers {
			return nil, false, domain.E(domain.CodeForbidden, "account does not exist and auto-creation is disabled")
		}
		uname, err := s.uniqueUsername(ctx, candidateUsername(claims))
		if err != nil {
			return nil, false, err
		}
		displayName, _ := claims["name"].(string)
		email, _ := claims["email"].(string)
		created, err := s.users.Create(ctx, db.NewUser{Username: uname, DisplayName: displayName, Email: email, Role: role})
		if err != nil {
			return nil, false, err
		}
		if err := s.users.AddIdentity(ctx, created.ID, settings.IssuerURL, subject); err != nil {
			return nil, false, err
		}
		return created, false, nil
	}

	if usr.Disabled {
		return nil, false, domain.E(domain.CodeAccountDisabled, "account disabled")
	}
	if settings.SyncRoles && role != usr.Role {
		updated, err := s.users.Update(ctx, usr.ID, db.UserUpdate{Role: &role})
		if err != nil {
			return nil, false, err
		}
		usr = updated
	}
	return usr, linked, nil
}

func groupsClaimOrDefault(settings domain.OIDCSettings) string {
	if settings.GroupsClaim == "" {
		return "groups"
	}
	return settings.GroupsClaim
}

// extractGroups normalises the groups claim, which providers encode either
// as a JSON array of strings or (rarely) a single string.
func extractGroups(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, g := range t {
			if s, ok := g.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

// mapRole picks the highest-ranked role among the groups mapped in mapping;
// if none match, defaultRole applies ("deny" rejects the login).
func mapRole(mapping map[string]domain.Role, groups []string, defaultRole string) (domain.Role, bool) {
	var best domain.Role
	for _, g := range groups {
		if r, ok := mapping[g]; ok && r.Level() > best.Level() {
			best = r
		}
	}
	if best != "" {
		return best, true
	}
	if defaultRole == "" {
		defaultRole = string(domain.RoleViewer)
	}
	if defaultRole == "deny" {
		return "", false
	}
	r := domain.Role(defaultRole)
	if !r.Valid() {
		return "", false
	}
	return r, true
}

// candidateUsername picks a starting username from OIDC claims:
// preferred_username, else the local part of email.
func candidateUsername(claims map[string]any) string {
	if v, ok := claims["preferred_username"].(string); ok && v != "" {
		return v
	}
	if v, ok := claims["email"].(string); ok && v != "" {
		if i := strings.Index(v, "@"); i > 0 {
			return v[:i]
		}
		return v
	}
	return "user"
}

func sanitizeUsername(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) < 2 {
		out = "user"
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// uniqueUsername returns base if free, else base + a numeric suffix.
func (s *Service) uniqueUsername(ctx context.Context, base string) (string, error) {
	base = sanitizeUsername(base)
	candidate := base
	for i := 2; i < 1000; i++ {
		_, err := s.users.GetByUsername(ctx, candidate)
		if err != nil {
			if domain.AsError(err).Code == domain.CodeNotFound {
				return candidate, nil
			}
			return "", err
		}
		suffix := strconv.Itoa(i)
		trimmed := base
		if len(trimmed)+len(suffix) > 32 {
			trimmed = trimmed[:32-len(suffix)]
		}
		candidate = trimmed + suffix
	}
	return "", domain.E(domain.CodeConflict, "could not generate a unique username")
}

// mergeUserInfo fetches the provider's UserInfo endpoint (when discovery
// advertises one) and adds claims missing from the ID token. The subject
// must match; otherwise the response is ignored.
func (s *Service) mergeUserInfo(ctx context.Context, rt *oidcRuntime, tok *oauth2.Token, claims map[string]any) {
	if rt == nil || rt.provider == nil || tok == nil {
		return
	}
	var ep struct {
		UserInfoEndpoint string `json:"userinfo_endpoint"`
	}
	if err := rt.provider.Claims(&ep); err != nil || ep.UserInfoEndpoint == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, err := rt.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil {
		s.log.Debug("oidc: userinfo unavailable, using id_token claims only", "err", err)
		return
	}
	if sub, _ := claims["sub"].(string); sub != "" && info.Subject != "" && sub != info.Subject {
		s.log.Warn("oidc: userinfo subject mismatch ignored", "id_token_sub", sub, "userinfo_sub", info.Subject)
		return
	}
	var extra map[string]any
	if err := info.Claims(&extra); err != nil {
		return
	}
	for k, v := range extra {
		if _, exists := claims[k]; !exists {
			claims[k] = v
		}
	}
}

// matchByVerifiedEmail returns a local user whose email equals the token's
// email claim, but only when the provider vouches for the address:
// email_verified must be true or absent. An explicit false never links, so an
// unverified address registered at the IdP cannot take over a local account.
func (s *Service) matchByVerifiedEmail(ctx context.Context, claims map[string]any) *domain.User {
	email, _ := claims["email"].(string)
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil
	}
	if v, present := claims["email_verified"]; present {
		switch t := v.(type) {
		case bool:
			if !t {
				return nil
			}
		case string:
			if !strings.EqualFold(t, "true") {
				return nil
			}
		default:
			return nil
		}
	}
	usr, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil
	}
	return usr
}
