package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
)

func TestNewSessionTokenUniqueAndHashMatches(t *testing.T) {
	raw1, hash1, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	raw2, hash2, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if raw1 == raw2 || hash1 == hash2 {
		t.Fatalf("expected distinct tokens")
	}
	if HashToken(raw1) != hash1 {
		t.Fatalf("HashToken mismatch")
	}
	if len(hash1) != 64 { // sha256 hex
		t.Fatalf("expected 64 hex chars, got %d", len(hash1))
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	cfg := config.Config{InsecureCookies: false}
	rec := httptest.NewRecorder()
	exp := time.Now().Add(time.Hour)
	SetSessionCookie(rec, cfg, "tok123", exp)
	res := rec.Result()
	var got *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == CookieName {
			got = c
		}
	}
	if got == nil {
		t.Fatalf("cookie not set")
	}
	if !got.HttpOnly || !got.Secure || got.SameSite != http.SameSiteLaxMode || got.Path != "/" {
		t.Fatalf("unexpected cookie attrs: %+v", got)
	}
	if got.Value != "tok123" {
		t.Fatalf("unexpected value: %s", got.Value)
	}

	rec2 := httptest.NewRecorder()
	ClearSessionCookie(rec2, cfg)
	res2 := rec2.Result()
	var cleared *http.Cookie
	for _, c := range res2.Cookies() {
		if c.Name == CookieName {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("expected cleared cookie with negative MaxAge: %+v", cleared)
	}
}

func TestSessionCookieInsecureMode(t *testing.T) {
	cfg := config.Config{InsecureCookies: true}
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, cfg, "tok", time.Now().Add(time.Hour))
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName && c.Secure {
			t.Fatalf("expected Secure=false when InsecureCookies is set")
		}
	}
}
