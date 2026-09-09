package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
)

// CookieName is the session cookie (ARCHITECTURE.md §13).
const CookieName = "vsui_session"

const (
	// SessionIdleTTL is how long a session stays valid without activity.
	SessionIdleTTL = 7 * 24 * time.Hour
	// SessionAbsoluteTTL is the hard cap on a session's lifetime regardless
	// of activity.
	SessionAbsoluteTTL = 30 * 24 * time.Hour
	// touchInterval is the minimum gap between last_seen_at updates, to
	// avoid a write on every request.
	touchInterval     = time.Minute
	sessionTokenBytes = 32
)

// NewSessionToken generates a fresh session token: the raw value that goes in
// the cookie, and the sha256 hex digest that is stored in the database.
func NewSessionToken() (raw string, hashed string, err error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	hashed = HashToken(raw)
	return raw, hashed, nil
}

// HashToken returns the sha256 hex digest of a raw cookie token, the form
// stored in the sessions table.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SetSessionCookie writes the session cookie for token, expiring at expiresAt.
func SetSessionCookie(w http.ResponseWriter, cfg config.Config, token string, expiresAt time.Time) {
	//nolint:gosec // HttpOnly/SameSite set; Secure derived from cfg.InsecureCookies.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

// ClearSessionCookie deletes the session cookie (logout).
func ClearSessionCookie(w http.ResponseWriter, cfg config.Config) {
	//nolint:gosec // HttpOnly/SameSite set; Secure derived from cfg.InsecureCookies.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
