package players

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestServiceListCommentsStayInDatabase(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	root := t.TempDir()
	paths := func(string) domain.InstancePaths { return domain.InstancePaths{Save: root} }
	svc := NewService(nil, store, paths, nil)
	id := "V_76561198002701519"
	for _, kind := range []domain.ListKind{domain.ListAdmin, domain.ListBanned, domain.ListPermitted} {
		got, err := svc.PutList(ctx, "main", domain.PlayerList{Kind: kind, Entries: []domain.PlayerListEntry{{ID: id, Comment: "Bjorn"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Entries) != 1 || got.Entries[0].Comment != "Bjorn" {
			t.Fatalf("%s response lost comment: %+v", kind, got)
		}
		raw, err := os.ReadFile(filepath.Join(root, kind.FileName()))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != id+"\n" {
			t.Fatalf("%s file has non-ID content: %q", kind, raw)
		}
	}
	// A new service process must recover comments from the manager database.
	svc = NewService(nil, store, paths, nil)
	got, err := svc.GetList(ctx, "main", domain.ListAdmin)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Comment != "Bjorn" {
		t.Fatalf("database comment after restart: %+v, %v", got, err)
	}
	if _, err := svc.PutList(ctx, "main", domain.PlayerList{Kind: domain.ListAdmin, Entries: []domain.PlayerListEntry{{ID: id}}}); err != nil {
		t.Fatal(err)
	}
	admin, err := svc.GetList(ctx, "main", domain.ListAdmin)
	if err != nil || admin.Entries[0].Comment != "" {
		t.Fatalf("cleared comment returned: %+v, %v", admin, err)
	}
	banned, err := svc.GetList(ctx, "main", domain.ListBanned)
	if err != nil || banned.Entries[0].Comment != "Bjorn" {
		t.Fatalf("admin comment edit affected banned list: %+v, %v", banned, err)
	}
}

func TestServiceRepairsLegacyInlineComment(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	root := t.TempDir()
	paths := func(string) domain.InstancePaths { return domain.InstancePaths{Save: root} }
	id := "V_76561198002701519"
	path := filepath.Join(root, domain.ListAdmin.FileName())
	if err := os.WriteFile(path, []byte("// List admin players ID  ONE per line\n"+id+" // Bjorn\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, store, paths, nil)
	got, err := svc.GetList(ctx, "main", domain.ListAdmin)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Comment != "Bjorn" {
		t.Fatalf("legacy comment was not migrated: %+v, %v", got, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "// List admin players ID  ONE per line\n\n"+id+"\n" {
		t.Fatalf("legacy file still has inline comment: %q", raw)
	}
	got, err = NewService(nil, store, paths, nil).GetList(ctx, "main", domain.ListAdmin)
	if err != nil || got.Entries[0].Comment != "Bjorn" {
		t.Fatalf("migrated comment lost after restart: %+v, %v", got, err)
	}
}

func TestServiceRepairsEmptyLegacyInlineComment(t *testing.T) {
	for _, suffix := range []string{" //", " // \t"} {
		t.Run(suffix, func(t *testing.T) {
			ctx := context.Background()
			store, _ := newSeededStore(t, "main")
			root := t.TempDir()
			id := "V_76561198002701519"
			path := filepath.Join(root, domain.ListAdmin.FileName())
			if err := os.WriteFile(path, []byte(id+suffix+"\n"), 0o640); err != nil {
				t.Fatal(err)
			}

			svc := NewService(nil, store, func(string) domain.InstancePaths {
				return domain.InstancePaths{Save: root}
			}, nil)
			got, err := svc.GetList(ctx, "main", domain.ListAdmin)
			if err != nil || len(got.Entries) != 1 || got.Entries[0].Comment != "" {
				t.Fatalf("empty legacy comment was not handled: %+v, %v", got, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != id+"\n" {
				t.Fatalf("legacy delimiter remained in game list: %q", raw)
			}
			comments, err := store.ListComments(ctx, "main", domain.ListAdmin)
			if err != nil {
				t.Fatal(err)
			}
			if len(comments) != 0 {
				t.Fatalf("empty legacy comment was persisted: %+v", comments)
			}
		})
	}
}

func TestServiceDoesNotChangeGameListWhenCommentStoreFails(t *testing.T) {
	ctx := context.Background()
	store, sqlDB := newSeededStore(t, "main")
	root := t.TempDir()
	svc := NewService(nil, store, func(string) domain.InstancePaths { return domain.InstancePaths{Save: root} }, nil)
	if _, err := sqlDB.Exec(`DROP TABLE player_list_comments`); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PutList(ctx, "main", domain.PlayerList{Kind: domain.ListAdmin, Entries: []domain.PlayerListEntry{{ID: "V_76561198002701519", Comment: "Bjorn"}}})
	if err == nil {
		t.Fatal("expected database failure")
	}
	if _, err := os.Stat(filepath.Join(root, domain.ListAdmin.FileName())); !os.IsNotExist(err) {
		t.Fatalf("game list changed despite failed comment save: %v", err)
	}
}

func TestServiceRestoresCommentsWhenGameFileWriteFails(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, store, func(string) domain.InstancePaths {
		return domain.InstancePaths{Save: filepath.Join(blocked, "save")}
	}, nil)
	_, err := svc.PutList(ctx, "main", domain.PlayerList{Kind: domain.ListAdmin, Entries: []domain.PlayerListEntry{{ID: "V_76561198002701519", Comment: "Bjorn"}}})
	if err == nil {
		t.Fatal("expected game file write failure")
	}
	comments, err := store.ListComments(ctx, "main", domain.ListAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("failed write left misleading UI comments in database: %+v", comments)
	}
}
