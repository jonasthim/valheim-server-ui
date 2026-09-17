package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func f64(v float64) *float64 { return &v }

func TestChatLogInsertAndList(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewChatLogRepo(sqldb)
	ctx := context.Background()

	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	entries := []domain.ChatLogEntry{
		{InstanceID: "main", At: base, Type: "normal", Sender: "Bjorn", Text: "hello there", RunSeq: 0},
		{InstanceID: "main", At: base.Add(time.Minute), Type: "shout", Sender: "Freya", Text: "incoming!", X: f64(10.5), Z: f64(-3.25), RunSeq: 0},
		{InstanceID: "main", At: base.Add(2 * time.Minute), Type: "normal", Sender: "Bjorn", Text: "gg", RunSeq: 0},
		// A different instance's chat must never leak into "main"'s list.
		{InstanceID: "other", At: base.Add(3 * time.Minute), Type: "normal", Sender: "Odin", Text: "hi", RunSeq: 0},
	}
	var ids []int64
	for _, e := range entries {
		id, err := repo.Insert(ctx, e)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if id == 0 {
			t.Fatalf("expected a non-zero id")
		}
		ids = append(ids, id)
	}

	// List: newest first, scoped to the instance.
	list, err := repo.List(ctx, "main", 100, 0, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 entries for main, got %d: %+v", len(list), list)
	}
	if list[0].Text != "gg" || list[0].ID != ids[2] {
		t.Errorf("expected the newest (gg) first, got %+v", list[0])
	}
	if list[2].Text != "hello there" {
		t.Errorf("expected the oldest last, got %+v", list[2])
	}
	// Position round-trips.
	if list[1].X == nil || *list[1].X != 10.5 || list[1].Z == nil || *list[1].Z != -3.25 {
		t.Errorf("expected the shout's position to round-trip, got %+v", list[1])
	}
	if list[0].X != nil || list[0].Z != nil {
		t.Errorf("expected no position on the plain message, got %+v", list[0])
	}
	if list[1].Type != "shout" {
		t.Errorf("expected type=shout, got %+v", list[1])
	}

	// List: before cursor pages (strictly less than the given id).
	page, err := repo.List(ctx, "main", 100, ids[2], "")
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 entries strictly before the cutoff, got %d: %+v", len(page), page)
	}
	for _, e := range page {
		if e.ID >= ids[2] {
			t.Errorf("expected every entry strictly before id %d, got %+v", ids[2], e)
		}
	}

	// List: limit.
	limited, err := repo.List(ctx, "main", 1, 0, "")
	if err != nil || len(limited) != 1 {
		t.Fatalf("expected limit=1, got %d err=%v", len(limited), err)
	}
	if limited[0].Text != "gg" {
		t.Errorf("expected the newest entry, got %+v", limited[0])
	}

	// List: q filters by text, case-insensitive substring.
	byText, err := repo.List(ctx, "main", 100, 0, "HELLO")
	if err != nil {
		t.Fatalf("list q text: %v", err)
	}
	if len(byText) != 1 || byText[0].Text != "hello there" {
		t.Fatalf("expected one text match, got %+v", byText)
	}

	// List: q filters by sender too.
	bySender, err := repo.List(ctx, "main", 100, 0, "bjorn")
	if err != nil {
		t.Fatalf("list q sender: %v", err)
	}
	if len(bySender) != 2 {
		t.Fatalf("expected 2 entries from Bjorn, got %+v", bySender)
	}

	// List: q with no match.
	none, err := repo.List(ctx, "main", 100, 0, "nonexistent")
	if err != nil {
		t.Fatalf("list q none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no matches, got %+v", none)
	}

	// List: a % or _ in q is a literal, not a wildcard.
	literal, err := repo.List(ctx, "main", 100, 0, "%")
	if err != nil {
		t.Fatalf("list q literal percent: %v", err)
	}
	if len(literal) != 0 {
		t.Fatalf("expected a literal %% to match nothing, got %+v", literal)
	}
}

func TestChatLogList_EmptyWhenNoRows(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewChatLogRepo(sqldb)

	list, err := repo.List(context.Background(), "main", 100, 0, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no entries, got %+v", list)
	}
}
