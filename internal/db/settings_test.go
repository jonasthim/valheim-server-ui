package db

import (
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestSettingsDefaultThenPut(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewSettingsRepo(sqldb)
	ctx := context.Background()

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := domain.DefaultSettings()
	if got.Auth.LocalLoginEnabled != want.Auth.LocalLoginEnabled || got.Updates.CheckIntervalMinutes != want.Updates.CheckIntervalMinutes {
		t.Fatalf("expected defaults, got %+v", got)
	}

	got.Auth.LocalLoginEnabled = false
	got.Auth.OIDC.Enabled = true
	got.Auth.OIDC.IssuerURL = "https://issuer.example"
	if err := repo.Put(ctx, got); err != nil {
		t.Fatalf("Put: %v", err)
	}

	reloaded, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get after put: %v", err)
	}
	if reloaded.Auth.LocalLoginEnabled != false || !reloaded.Auth.OIDC.Enabled || reloaded.Auth.OIDC.IssuerURL != "https://issuer.example" {
		t.Fatalf("settings not persisted: %+v", reloaded)
	}

	// Put again (upsert path) must not error and must overwrite.
	reloaded.Updates.CheckIntervalMinutes = 30
	if err := repo.Put(ctx, reloaded); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	final, err := repo.Get(ctx)
	if err != nil || final.Updates.CheckIntervalMinutes != 30 {
		t.Fatalf("upsert did not apply: %+v err=%v", final, err)
	}
}
