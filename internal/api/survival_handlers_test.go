package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type fakeSurvival struct {
	history   *domain.SurvivalHistory
	world     int64
	character string
	hidden    bool
}

func (f *fakeSurvival) Survival(_ context.Context, _ string, world int64, character string, hidden bool) (*domain.SurvivalHistory, error) {
	f.world, f.character, f.hidden = world, character, hidden
	return f.history, nil
}

func TestSurvivalHandlerPassesFiltersAndViewerPrivacyFlag(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	f := &fakeSurvival{history: &domain.SurvivalHistory{Worlds: []int64{55}, Moments: []domain.SurvivalMoment{{ID: 1, Kind: "death", CharacterID: "991", At: time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)}}}}
	d.Survival = f
	rec := httptest.NewRecorder()
	NewRouter(d, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/survival?world_uid=55&character_id=991", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if f.world != 55 || f.character != "991" || f.hidden {
		t.Fatalf("service args: %+v", f)
	}
	var body struct {
		Moments []struct {
			Kind string `json:"kind"`
		} `json:"moments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Moments) != 1 {
		t.Fatalf("body %s, err %v", rec.Body.String(), err)
	}
}

func TestSurvivalHandlerKeepsLargeWorldUIDExact(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	const uid int64 = 9007199254740993
	f := &fakeSurvival{history: &domain.SurvivalHistory{Worlds: []int64{uid}, Moments: []domain.SurvivalMoment{{ID: 1, WorldUID: uid, InstanceID: "main", Kind: "death", At: time.Now()}}}}
	d.Survival = f
	rec := httptest.NewRecorder()
	NewRouter(d, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/survival?world_uid=9007199254740993", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if f.world != uid {
		t.Fatalf("request UID rounded: %d", f.world)
	}
	var body struct {
		Worlds  []string `json:"worlds"`
		Moments []struct {
			WorldUID string `json:"world_uid"`
		} `json:"moments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Worlds) != 1 || body.Worlds[0] != "9007199254740993" || body.Moments[0].WorldUID != "9007199254740993" {
		t.Fatalf("response UID rounded: %s", rec.Body.String())
	}
}

func TestSurvivalHandlerAcceptsSignedWorldUID(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	f := &fakeSurvival{history: &domain.SurvivalHistory{Worlds: []int64{-42}}}
	d.Survival = f
	rec := httptest.NewRecorder()
	NewRouter(d, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/survival?world_uid=-42", nil))
	if rec.Code != http.StatusOK || f.world != -42 {
		t.Fatalf("signed world UID: status %d, world %d", rec.Code, f.world)
	}
}
