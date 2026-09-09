package auth

import (
	"context"
	"database/sql"
	"log/slog"
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
	_ api.Authenticator = (*Service)(nil)
	_ api.UserService   = (*Service)(nil)
)

// Service implements api.Authenticator, api.UserService and api.SettingsService
// on top of the db repositories, plus the OIDC login/callback flow (oidc.go).
type Service struct {
	cfg      config.Config
	log      *slog.Logger
	users    *db.Users
	sessions *db.Sessions
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

// Authenticate resolves the session cookie (if any) and stores the user in
// the request context. It never rejects a request itself; RequireRole does
// that once a user is required.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if locked, until := s.lockout.Locked(username); locked {
		return nil, domain.Ef(domain.CodeAccountLocked, "too many failed attempts; locked until %s", until.UTC().Format(time.RFC3339))
	}
	const invalid = "invalid username or password"
	usr, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		s.lockout.RecordFailure(username)
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
	}
	if usr.Disabled {
		return nil, domain.E(domain.CodeAccountDisabled, "account disabled")
	}
	if !usr.HasPassword || !VerifyPassword(usr.PasswordHash, password) {
		s.lockout.RecordFailure(username)
		return nil, domain.E(domain.CodeInvalidCredentials, invalid)
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

func (s *Service) SetPassword(ctx context.Context, id int64, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	h, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.users.SetPasswordHash(ctx, id, &h)
}

func (s *Service) ChangePassword(ctx context.Context, id int64, current, newPassword string) error {
	usr, err := s.users.Get(ctx, id)
	if err != nil {
		return err
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
	return s.users.SetPasswordHash(ctx, id, &h)
}
