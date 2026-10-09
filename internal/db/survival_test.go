package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestSurvivalStoreSeparatesWorldsAndCharactersAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	store := NewSurvivalRepo(sqlDB)
	at := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	items := []domain.SurvivalMoment{
		{InstanceID: "main", WorldUID: 10, RunID: "run-a", SourceSeq: 1, Kind: "death", CharacterID: "char-a", PlayerName: "Quin", Day: 3, At: at},
		{InstanceID: "main", WorldUID: 10, RunID: "run-a", SourceSeq: 2, Kind: "death", CharacterID: "char-b", PlayerName: "Bjorn", Day: 4, At: at},
		{InstanceID: "main", WorldUID: 11, RunID: "run-b", SourceSeq: 1, Kind: "death", CharacterID: "char-a", PlayerName: "Quin", Day: 1, At: at},
		{InstanceID: "other", WorldUID: 10, RunID: "run-a", SourceSeq: 1, Kind: "death", CharacterID: "char-a", PlayerName: "Quin", Day: 3, At: at},
	}
	for _, item := range items {
		if err := store.Insert(ctx, item); err != nil {
			t.Fatal(err)
		}
		if err := store.Insert(ctx, item); err != nil {
			t.Fatalf("duplicate insert: %v", err)
		}
	}
	got, err := store.List(ctx, "main", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("world 10: got %d moments, want 2", len(got))
	}
	got, err = store.List(ctx, "main", 10, "char-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PlayerName != "Quin" {
		t.Fatalf("character filter: %+v", got)
	}
	worlds, err := store.Worlds(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 2 || worlds[0] != 11 || worlds[1] != 10 {
		t.Fatalf("worlds: %v", worlds)
	}
}
