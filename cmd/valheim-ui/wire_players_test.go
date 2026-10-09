package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/players"
)

func TestWirePlayersRepairsLegacyInlineAdminCommentAtStartup(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO instances(id,name,config_json,created_at,updated_at) VALUES ('main','Main','{}',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, domain.ListAdmin.FileName())
	id := "V_76561198002701519"
	if err := os.WriteFile(path, []byte(id+" // Bjorn\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	paths := func(string) domain.InstancePaths { return domain.InstancePaths{Save: root} }
	list := func(context.Context) ([]players.InstanceRef, error) {
		return []players.InstanceRef{{ID: "main", Paths: paths("main")}}, nil
	}
	exists := func(context.Context, string) (bool, error) { return true, nil }
	deps := &api.Deps{DB: sqlDB}
	if _, err := wirePlayers(ctx, deps, list, paths, exists, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != id+"\n" {
		t.Fatalf("startup left invalid admin ID line: %q", raw)
	}
	got, err := deps.Players.GetList(ctx, "main", domain.ListAdmin)
	if err != nil || got.Entries[0].Comment != "Bjorn" {
		t.Fatalf("startup lost migrated comment: %+v, %v", got, err)
	}
}
