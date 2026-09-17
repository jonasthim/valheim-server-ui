package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

var (
	_ api.Authenticator  = (*Service)(nil)
	_ api.UserService    = (*Service)(nil)
	_ api.SessionService = (*Service)(nil)
	_ api.TokenService   = (*Service)(nil)
)

// Service implements api.Authenticator, api.UserService and api.SettingsService
// on top of the db repositories, plus the OIDC login/callback flow (oidc.go).
type Service struct {
	cfg      config.Config
	log      *slog.Logger
	users    *db.Users
	sessions *db.Sessions
	tokens   *db.APITokens
	settings *db.SettingsRepo
	lockout  *Lockout
	clock    Clock
	auditor  api.Auditor // optional; used only by the OIDC callback (see oidc.go)

	setupMu sync.Mutex

	oidcMu       sync.Mutex
	oidcGen      uint64
	oidcBuiltGen uint64
	oidcRuntime  *oidcRuntime
}

// Option customises Service construction (used by tests).
type Option func(*Service)

// WithClock overrides the clock used for session/lockout timing. Tests use
// this for deterministic expiry checks.
func WithClock(c Clock) Option {
	return func(s *Service) { s.clock = c }
}

// WithAuditor lets the OIDC callback record its own "auth.login" entry,
// since it redirects the browser and never returns the logged-in user to
// the HTTP handler that would otherwise record it.
func WithAuditor(a api.Auditor) Option {
	return func(s *Service) { s.auditor = a }
}

// NewService constructs the auth service on top of an open database.
func NewService(sqldb *sql.DB, cfg config.Config, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		cfg:      cfg,
		log:      log,
		users:    db.NewUsers(sqldb),
		sessions: db.NewSessions(sqldb),
		tokens:   db.NewAPITokens(sqldb),
		settings: db.NewSettingsRepo(sqldb),
		clock:    time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	s.lockout = NewLockout(s.clock)
	return s
}

// ---- api.Authenticator ----------------------------------------------------

// Authenticate resolves a personal API bearer token or the session cookie (in
// that order) and stores the user in the request context. It never rejects a
// request itself; RequireRole does that once a user is required.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), bearerAuthPrefix) {
			// A request whose Authorization header names a personal API
			// token (F-2.5) is decided here, before any cookie is looked at:
			// a valid token authenticates as its owner even if a stale or
			// unrelated session cookie also rides along; an invalid one
			// (unknown, expired, disabled owner) leaves the request
			// unauthenticated rather than falling back to the cookie.
			if usr, ok := s.authenticateBearer(r); ok {
				next.ServeHTTP(w, r.WithContext(api.WithUser(api.WithTokenAuth(r.Context()), usr)))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		sess, err := s.sessions.Get(ctx, HashToken(cookie.Value))
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		now := s.clock()
		if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeenAt) > SessionIdleTTL {
			_ = s.sessions.Delete(ctx, sess.ID)
			next.ServeHTTP(w, r)
			return
		}
		usr, err := s.users.Get(ctx, sess.UserID)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		if now.Sub(sess.LastSeenAt) >= touchInterval {
			_ = s.sessions.Touch(ctx, sess.ID, now)
		}
		next.ServeHTTP(w, r.WithContext(api.WithUser(ctx, usr)))
	})
}

// ---- login / logout / setup ------------------------------------------------

func (s *Service) createSession(ctx context.Context, w http.ResponseWriter, r *http.Request, usr *domain.User) error {
	raw, hash, err := NewSessionToken()
	if err != nil {
		return err
	}
	now := s.clock()
	sess := domain.Session{
		ID: hash, UserID: usr.ID, CreatedAt: now, ExpiresAt: now.Add(SessionAbsoluteTTL),
		LastSeenAt: now, IP: api.ClientIP(r), UserAgent: r.UserAgent(),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return err
	}
	SetSessionCookie(w, s.cfg, raw, now.Add(SessionIdleTTL))
	return nil
}

// Setup creates the first admin account. It only succeeds while there are no
// users at all; afterwards it returns domain.CodeSetupDone.
func (s *Service) Setup(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, displayName, email string) (*domain.User, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()

	count, err := s.users.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, domain.E(domain.CodeSetupDone, "setup already completed")
	}
	uname, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if err := validateDisplayName(displayName); err != nil {
		return nil, err
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	usr, err := s.users.Create(ctx, db.NewUser{
		Username: uname, DisplayName: displayName, Email: email, PasswordHash: &hash, Role: domain.RoleAdmin,
	})
	if err != nil {
		return nil, err
	}
	if err := s.createSession(ctx, w, r, usr); err != nil {
		return nil, err
	}
	_ = s.users.UpdateLastLogin(ctx, usr.ID, s.clock())
	return usr, nil
}

// Login authenticates a local user and, on success, sets the session cookie.
func (s *Service) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password string) (*domain.User, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Auth.LocalLoginEnabled {
		return nil, domain.E(domain.CodeForbidden, "local login is disabled")
	}
	username = strings.ToLower(strings.TrimSpace(username))
	const invalid = "invalid username or password"
	// Usernames are at most 32 characters (validate.go); anything longer can
	// never match and is refused before it reaches the lockout table or the
	// audit log, so an unauthenticated caller cannot grow either with junk.
	if len(username) > maxUsernameLen {
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
	}
	ip := remoteIP(r)
	if locked, until := s.lockout.LockedIP(ip); locked {
		return nil, domain.Ef(domain.CodeAccountLocked, "too many failed attempts; locked until %s", until.UTC().Format(time.RFC3339))
	}
	if locked, until := s.lockout.Locked(username); locked {
		return nil, domain.Ef(domain.CodeAccountLocked, "too many failed attempts; locked until %s", until.UTC().Format(time.RFC3339))
	}
	usr, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		// Spend the same argon2id work as a real check so the response time
		// does not reveal whether the username exists.
		_ = VerifyPassword(timingEqualiserHash, password)
		s.lockout.RecordFailure(username, ip)
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
	}
	if !usr.HasPassword {
		_ = VerifyPassword(timingEqualiserHash, password)
		s.lockout.RecordFailure(username, ip)
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
	}
	if !VerifyPassword(usr.PasswordHash, password) {
		s.lockout.RecordFailure(username, ip)
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
	}
	// Only a caller holding the correct password learns that the account is
	// disabled; a wrong password answers exactly like an unknown user.
	if usr.Disabled {
		return nil, domain.E(domain.CodeAccountDisabled, "account disabled")
	}
	s.lockout.Reset(username)
	if err := s.createSession(ctx, w, r, usr); err != nil {
		return nil, err
	}
	_ = s.users.UpdateLastLogin(ctx, usr.ID, s.clock())
	return usr, nil
}

// Logout deletes the current session (if any) and clears the cookie.
func (s *Service) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil && cookie.Value != "" {
		_ = s.sessions.Delete(ctx, HashToken(cookie.Value))
	}
	ClearSessionCookie(w, s.cfg)
}

// ---- api.UserService --------------------------------------------------------

func (s *Service) List(ctx context.Context) ([]domain.User, error) { return s.users.List(ctx) }

func (s *Service) Get(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.Get(ctx, id)
}

func (s *Service) Count(ctx context.Context) (int, error) { return s.users.Count(ctx) }

func (s *Service) Create(ctx context.Context, username, password, displayName, email string, role domain.Role) (*domain.User, error) {
	if !role.Valid() {
		return nil, domain.Validation([]domain.FieldError{{Field: "role", Message: "invalid role"}})
	}
	uname, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	if err := validateDisplayName(displayName); err != nil {
		return nil, err
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	var hashPtr *string
	if password != "" {
		if err := validatePassword(password); err != nil {
			return nil, err
		}
		h, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		hashPtr = &h
	}
	return s.users.Create(ctx, db.NewUser{Username: uname, DisplayName: displayName, Email: email, PasswordHash: hashPtr, Role: role})
}

// wouldRemoveLastAdmin reports whether applying role/disabled changes to a
// currently-enabled admin would leave zero enabled admins.
func (s *Service) wouldRemoveLastAdmin(ctx context.Context, current *domain.User, role *domain.Role, disabled *bool) (bool, error) {
	if current.Role != domain.RoleAdmin || current.Disabled {
		return false, nil
	}
	losesAdmin := (role != nil && *role != domain.RoleAdmin) || (disabled != nil && *disabled)
	if !losesAdmin {
		return false, nil
	}
	n, err := s.users.CountEnabledAdmins(ctx)
	if err != nil {
		return false, err
	}
	return n <= 1, nil
}

func (s *Service) Update(ctx context.Context, id int64, displayName, email *string, role *domain.Role, disabled *bool) (*domain.User, error) {
	if role != nil && !role.Valid() {
		return nil, domain.Validation([]domain.FieldError{{Field: "role", Message: "invalid role"}})
	}
	if displayName != nil {
		if err := validateDisplayName(*displayName); err != nil {
			return nil, err
		}
	}
	if email != nil {
		if err := validateEmail(*email); err != nil {
			return nil, err
		}
	}
	current, err := s.users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	blocked, err := s.wouldRemoveLastAdmin(ctx, current, role, disabled)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, domain.E(domain.CodeLastAdmin, "cannot remove the last enabled admin")
	}
	updated, err := s.users.Update(ctx, id, db.UserUpdate{DisplayName: displayName, Email: email, Role: role, Disabled: disabled})
	if err != nil {
		return nil, err
	}
	if disabled != nil && *disabled {
		_ = s.sessions.DeleteByUser(ctx, id)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	current, err := s.users.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Role == domain.RoleAdmin && !current.Disabled {
		n, err := s.users.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return domain.E(domain.CodeLastAdmin, "cannot delete the last enabled admin")
		}
	}
	return s.users.Delete(ctx, id)
}

// ErrSSOManaged is returned when a password operation targets an account
// linked to the identity provider: its credential lives at the IdP.
func errSSOManaged() error {
	return domain.E(domain.CodeConflict, "this account signs in through single sign-on; its password is managed by the identity provider")
}

// SetPassword is the administrative reset. Every session of the user is
// revoked: a stolen session must not survive the credential being replaced.
func (s *Service) SetPassword(ctx context.Context, id int64, newPassword string) error {
	usr, err := s.users.Get(ctx, id)
	if err != nil {
		return err
	}
	if len(usr.Identities) > 0 {
		return errSSOManaged()
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	h, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.SetPasswordHash(ctx, id, &h); err != nil {
		return err
	}
	s.lockout.Reset(usr.Username)
	return s.sessions.DeleteByUserExcept(ctx, id, "")
}

// ChangePassword is the self-service change. All other sessions of the user
// are revoked; keepSessionID (the caller's own session id, or "" to revoke
// everything) stays valid so the user is not logged out of the tab they are
// using.
func (s *Service) ChangePassword(ctx context.Context, id int64, current, newPassword string) error {
	return s.ChangePasswordKeepingSession(ctx, id, current, newPassword, "")
}

// ChangePasswordFromRequest is ChangePassword that keeps the session the
// request carries (the tab performing the change) and revokes all others.
func (s *Service) ChangePasswordFromRequest(ctx context.Context, r *http.Request, id int64, current, newPassword string) error {
	return s.ChangePasswordKeepingSession(ctx, id, current, newPassword, SessionIDFromRequest(r))
}

// ChangePasswordKeepingSession is ChangePassword with an explicit session to
// preserve; see ChangePassword.
func (s *Service) ChangePasswordKeepingSession(ctx context.Context, id int64, current, newPassword, keepSessionID string) error {
	usr, err := s.users.Get(ctx, id)
	if err != nil {
		return err
	}
	if len(usr.Identities) > 0 {
		return errSSOManaged()
	}
	if !usr.HasPassword {
		return domain.E(domain.CodeValidationFailed, "account has no local password (OIDC-only)")
	}
	if !VerifyPassword(usr.PasswordHash, current) {
		return domain.E(domain.CodeInvalidCredentials, "current password is incorrect")
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	h, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.SetPasswordHash(ctx, id, &h); err != nil {
		return err
	}
	return s.sessions.DeleteByUserExcept(ctx, id, keepSessionID)
}

// SessionIDFromRequest returns the hashed id of the session the request
// carries, or "" when it has none. Handlers pass it to
// ChangePasswordKeepingSession.
func SessionIDFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	return HashToken(cookie.Value)
}

// ListSessions returns userID's active sessions for the account page, most
// recently active first, marking the one the request carries as current.
func (s *Service) ListSessions(ctx context.Context, r *http.Request, userID int64) ([]domain.SessionInfo, error) {
	rows, err := s.sessions.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	current := SessionIDFromRequest(r)
	out := make([]domain.SessionInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.SessionInfo{
			ID:         row.ID,
			CreatedAt:  row.CreatedAt,
			LastSeenAt: row.LastSeenAt,
			ExpiresAt:  row.ExpiresAt,
			IP:         row.IP,
			UserAgent:  row.UserAgent,
			Current:    row.ID == current,
		})
	}
	return out, nil
}

// RevokeSession deletes one of userID's own sessions. A session that exists
// but belongs to a different user answers identically to one that does not
// exist at all, so a caller cannot use this to probe other users' session ids.
func (s *Service) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.UserID != userID {
		return domain.NotFound("session")
	}
	return s.sessions.Delete(ctx, sessionID)
}

// RevokeOtherSessions signs userID out of every session except the one the
// request carries ("sign out everywhere else").
func (s *Service) RevokeOtherSessions(ctx context.Context, r *http.Request, userID int64) error {
	return s.sessions.DeleteByUserExcept(ctx, userID, SessionIDFromRequest(r))
}

// ---- api.TokenService (F-2.5) -----------------------------------------------

const (
	tokenSecretPrefix  = "vsui_"
	bearerAuthPrefix   = "Bearer " + tokenSecretPrefix
	tokenSecretBytes   = 32
	tokenPrefixLen     = 12
	maxTokenNameLen    = 64
	maxTokenExpiryDays = 3650
)

// CreateToken mints a personal API token for userID. The plaintext secret is
// returned once and is never stored or retrievable again; only its sha256
// hash (HashToken) and a tokenPrefixLen-character prefix (so the owner can
// recognise it in the list) persist. expiresInDays of 0 means the token never
// expires.
func (s *Service) CreateToken(ctx context.Context, userID int64, name string, expiresInDays int) (domain.APIToken, string, error) {
	if l := len(name); l < 1 || l > maxTokenNameLen {
		return domain.APIToken{}, "", domain.Validation([]domain.FieldError{
			{Field: "name", Message: "must be 1-64 characters"},
		})
	}
	if expiresInDays < 0 || expiresInDays > maxTokenExpiryDays {
		return domain.APIToken{}, "", domain.Validation([]domain.FieldError{
			{Field: "expires_in_days", Message: "must be 0 (never) or 1-3650"},
		})
	}
	buf := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return domain.APIToken{}, "", fmt.Errorf("generate api token: %w", err)
	}
	secret := tokenSecretPrefix + base64.RawURLEncoding.EncodeToString(buf)
	hash := HashToken(secret)
	prefix := secret[:tokenPrefixLen]

	var expiresAt *time.Time
	if expiresInDays > 0 {
		t := s.clock().AddDate(0, 0, expiresInDays)
		expiresAt = &t
	}
	tok, err := s.tokens.Create(ctx, userID, name, hash, prefix, expiresAt)
	if err != nil {
		return domain.APIToken{}, "", err
	}
	return tok, secret, nil
}

// ListTokens returns userID's API tokens, newest first.
func (s *Service) ListTokens(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	return s.tokens.ListByUser(ctx, userID)
}

// RevokeToken deletes one of userID's own API tokens.
func (s *Service) RevokeToken(ctx context.Context, userID, id int64) error {
	return s.tokens.Delete(ctx, userID, id)
}

// authenticateBearer resolves the personal API token named by r's
// Authorization header (already confirmed to carry bearerAuthPrefix by the
// caller). ok is false whenever the token is unknown, expired, or belongs to
// a disabled user; Authenticate then treats the request as unauthenticated,
// exactly like a missing or invalid session cookie, rather than rejecting it
// itself.
func (s *Service) authenticateBearer(r *http.Request) (usr *domain.User, ok bool) {
	secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	ctx := r.Context()
	userID, tok, err := s.tokens.GetByHash(ctx, HashToken(secret))
	if err != nil {
		return nil, false
	}
	now := s.clock()
	if tok.ExpiresAt != nil && now.After(*tok.ExpiresAt) {
		return nil, false
	}
	usr, err = s.users.Get(ctx, userID)
	if err != nil || usr.Disabled {
		return nil, false
	}
	// Touch at most once a minute so a busy script does not write on every
	// request; touchInterval is the same constant the session cookie uses.
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= touchInterval {
		_ = s.tokens.TouchLastUsed(ctx, tok.ID, now)
	}
	return usr, true
}

// RunSessionPurge periodically deletes expired sessions until ctx is done.
// Not required for correctness (Get and Authenticate re-validate expiry on
// every use) but keeps the table from growing without bound.
func (s *Service) RunSessionPurge(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sessions.DeleteExpired(ctx, s.clock()); err != nil {
				s.log.Warn("purge expired sessions", "err", err)
			}
		}
	}
}

// remoteIP is the client address after the router's realIP middleware has
// rewritten RemoteAddr (loopback proxies only), without the port.
func remoteIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.Trim(host, "[]")
}

// maxUsernameLen mirrors validate.go's upper bound.
const maxUsernameLen = 32

// timingEqualiserHash is a real argon2id hash of a throwaway secret, verified
// against on the "no such user" path so both branches of Login cost the same.
var timingEqualiserHash = mustHash("timing-equaliser-not-a-real-password")

func mustHash(pw string) string {
	h, err := HashPassword(pw)
	if err != nil {
		// HashPassword only fails when crypto/rand does; nothing sensible can
		// run without it, and the value is used only to burn CPU time.
		return ""
	}
	return h
}
