package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
)

func TestWithTokenAuthIsTokenAuth(t *testing.T) {
	ctx := context.Background()
	if IsTokenAuth(ctx) {
		t.Fatalf("expected IsTokenAuth false for a plain context")
	}
	ctx = WithTokenAuth(ctx)
	if !IsTokenAuth(ctx) {
		t.Fatalf("expected IsTokenAuth true after WithTokenAuth")
	}
}

func TestCSRFGuardSkipsChecksForTokenAuth(t *testing.T) {
	guard := csrfGuard(config.Config{})
	called := false
	h := guard(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(WithTokenAuth(req.Context()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatalf("expected token-authenticated POST without a CSRF header to pass through, got status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCSRFGuardRejectsNonTokenAuthWithoutHeader(t *testing.T) {
	guard := csrfGuard(config.Config{})
	called := false
	h := guard(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatalf("expected non-token POST without a CSRF header to be rejected")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
