package agent

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type transientSurvivalStore struct {
	*db.SurvivalRepo
	fail bool
}

func seedSurvivalInstance(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	if _, err := sqlDB.Exec(`INSERT INTO instances(id,name,config_json,created_at,updated_at) VALUES ('main','main','{}','2026-10-09T10:00:00Z','2026-10-09T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
}

func (s *transientSurvivalStore) Insert(ctx context.Context, m domain.SurvivalMoment) error {
	if m.Kind == "progression" && s.fail {
		s.fail = false
		return errors.New("temporary SQLite error")
	}
	return s.SurvivalRepo.Insert(ctx, m)
}

func TestSurvivalPollPersistsDeathOnceAndWorldMilestone(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	seedSurvivalInstance(t, sqlDB)
	store := db.NewSurvivalRepo(sqlDB)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		_, _ = w.Write([]byte(`{"next":1,"events":[{"seq":1,"kind":"player.death","at":"2026-10-09T10:00:00Z","data":{"world_uid":55,"world_name":"Midgard","character_id":"991","player_name":"Quin","day":12,"biome":"Meadows","x":4,"z":7,"visible":false}}]}`))
	}))
	defer server.Close()
	svc := NewService(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	svc.SetSurvivalStore(store)
	client := NewClientForURL(server.URL, "token", server.Client())
	status := &domain.AgentStatus{Ready: true, World: domain.AgentWorld{WorldUID: 55, Name: "Midgard", Day: 12}, GlobalKeys: []string{"defeated_eikthyr"}}
	svc.pollSurvival(ctx, "main", client, status)
	svc.pollSurvival(ctx, "main", client, status)
	got, err := store.List(ctx, "main", 55, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "death" || got[0].PlayerName != "Quin" || got[0].Visible {
		t.Fatalf("death ingestion: %+v", got)
	}
	status.GlobalKeys = append(status.GlobalKeys, "defeated_gdking")
	svc.pollSurvival(ctx, "main", client, status)
	got, err = store.List(ctx, "main", 55, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Kind != "progression" {
		t.Fatalf("milestone ingestion: %+v", got)
	}
	status.Modifiers = map[string]string{"combat": "hard"}
	svc.pollSurvival(ctx, "main", client, status)
	status.Modifiers = map[string]string{}
	svc.pollSurvival(ctx, "main", client, status)
	got, err = store.List(ctx, "main", 55, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[2].Kind != "setting" || got[3].Kind != "setting" {
		t.Fatalf("setting additions and removals: %+v", got)
	}
}

func TestSurvivalViewerCannotSeeHiddenDeathPosition(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	seedSurvivalInstance(t, sqlDB)
	store := db.NewSurvivalRepo(sqlDB)
	x, z := 4.0, 7.0
	if err := store.Insert(ctx, domain.SurvivalMoment{InstanceID: "main", WorldUID: 55, RunID: "r", SourceSeq: 1, Kind: "death", CharacterID: "991", At: time.Now(), X: &x, Z: &z, Visible: false}); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(ctx, domain.SurvivalMoment{InstanceID: "main", WorldUID: 55, RunID: "r", SourceSeq: 2, Kind: "death", CharacterID: "992", At: time.Now(), X: &x, Z: &z, Visible: true}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(&fakeInstances{inst: domain.Instance{ID: "main"}}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	svc.SetSurvivalStore(store)
	viewer, err := svc.Survival(ctx, "main", 55, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, moment := range viewer.Moments {
		if moment.X != nil || moment.Z != nil {
			t.Fatalf("viewer saw historical position after sharing may have changed: %+v", moment)
		}
	}
	operator, err := svc.Survival(ctx, "main", 55, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if operator.Moments[0].X == nil || *operator.Moments[0].X != 4 {
		t.Fatalf("operator lost position: %+v", operator.Moments[0])
	}
}

func TestSurvivalPollRetriesFailedMilestone(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	seedSurvivalInstance(t, sqlDB)
	store := &transientSurvivalStore{SurvivalRepo: db.NewSurvivalRepo(sqlDB), fail: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"next":0,"events":[]}`)) }))
	defer server.Close()
	svc := NewService(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	svc.SetSurvivalStore(store)
	client := NewClientForURL(server.URL, "token", server.Client())
	status := &domain.AgentStatus{Ready: true, World: domain.AgentWorld{WorldUID: 99, Day: 10}}
	svc.pollSurvival(ctx, "main", client, status)
	status.GlobalKeys = []string{"defeated_eikthyr"}
	svc.pollSurvival(ctx, "main", client, status)
	svc.pollSurvival(ctx, "main", client, status)
	got, err := store.List(ctx, "main", 99, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "progression" {
		t.Fatalf("milestone lost after transient insert failure: %+v", got)
	}
}
