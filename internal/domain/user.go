package domain

import "time"

// Role is the authorisation level of a user. Ordering matters: Level().
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Level returns a comparable rank; unknown roles rank below viewer.
func (r Role) Level() int {
	switch r {
	case RoleViewer:
		return 1
	case RoleOperator:
		return 2
	case RoleAdmin:
		return 3
	}
	return 0
}

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool { return r.Level() > 0 }

// AtLeast reports whether r grants at least the permissions of min.
func (r Role) AtLeast(min Role) bool { return r.Level() >= min.Level() }

// Identity links an external (OIDC) subject to a user.
type Identity struct {
	Provider string `json:"provider"` // issuer URL
	Subject  string `json:"subject"`
}

// User is an account. PasswordHash is never serialised.
type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name,omitempty"`
	Email        string     `json:"email,omitempty"`
	Role         Role       `json:"role"`
	Disabled     bool       `json:"disabled"`
	HasPassword  bool       `json:"has_password"`
	Identities   []Identity `json:"identities"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	PasswordHash string     `json:"-"`
}

// Session is a browser session. ID is the sha256 hex of the cookie token.
type Session struct {
	ID         string
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	IP         string
	UserAgent  string
}

// OIDCSettings configures the single external identity provider.
type OIDCSettings struct {
	Enabled         bool            `json:"enabled"`
	ProviderName    string          `json:"provider_name"`
	IssuerURL       string          `json:"issuer_url"`
	ClientID        string          `json:"client_id"`
	ClientSecret    string          `json:"client_secret"` // write-only over the API
	Scopes          []string        `json:"scopes"`
	GroupsClaim     string          `json:"groups_claim"`
	RoleMapping     map[string]Role `json:"role_mapping"`
	DefaultRole     string          `json:"default_role"` // viewer|operator|admin|deny
	AutoCreateUsers bool            `json:"auto_create_users"`
	SyncRoles       bool            `json:"sync_roles"`
	RedirectURI     string          `json:"redirect_uri,omitempty"` // computed, read-only
}

// Settings is the single-row settings document.
type Settings struct {
	Auth struct {
		LocalLoginEnabled bool         `json:"local_login_enabled"`
		OIDC              OIDCSettings `json:"oidc"`
	} `json:"auth"`
	Updates struct {
		CheckIntervalMinutes int `json:"check_interval_minutes"`
	} `json:"updates"`
	Thunderstore struct {
		IndexRefreshHours int `json:"index_refresh_hours"`
	} `json:"thunderstore"`
}

// DefaultSettings returns the settings used when the row does not exist yet.
func DefaultSettings() Settings {
	var s Settings
	s.Auth.LocalLoginEnabled = true
	s.Auth.OIDC = OIDCSettings{
		ProviderName:    "SSO",
		Scopes:          []string{"openid", "profile", "email", "groups"},
		GroupsClaim:     "groups",
		RoleMapping:     map[string]Role{},
		DefaultRole:     string(RoleViewer),
		AutoCreateUsers: true,
		SyncRoles:       true,
	}
	s.Updates.CheckIntervalMinutes = 60
	s.Thunderstore.IndexRefreshHours = 6
	return s
}

// AuditEntry is one row of the audit log.
type AuditEntry struct {
	ID         int64          `json:"id"`
	TS         time.Time      `json:"ts"`
	UserID     *int64         `json:"user_id,omitempty"`
	Username   string         `json:"username,omitempty"`
	Action     string         `json:"action"`
	InstanceID string         `json:"instance_id,omitempty"`
	Target     string         `json:"target,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	IP         string         `json:"ip,omitempty"`
}
