package agent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type deathPayload struct {
	RunID       string   `json:"run_id"`
	WorldUID    int64    `json:"world_uid"`
	WorldName   string   `json:"world_name"`
	CharacterID string   `json:"character_id"`
	PlayerName  string   `json:"player_name"`
	Day         int      `json:"day"`
	Enemy       string   `json:"enemy"`
	EnemyLevel  int      `json:"enemy_level"`
	Situation   string   `json:"situation"`
	Biome       string   `json:"biome"`
	X           *float64 `json:"x"`
	Z           *float64 `json:"z"`
	Visible     bool     `json:"visible"`
}

var bossKeys = map[string]string{
	"defeated_eikthyr":    "Eikthyr progression unlocked",
	"defeated_gdking":     "The Elder progression unlocked",
	"defeated_bonemass":   "Bonemass progression unlocked",
	"defeated_dragon":     "Moder progression unlocked",
	"defeated_goblinking": "Yagluth progression unlocked",
	"defeated_queen":      "The Queen progression unlocked",
	"defeated_fader":      "Fader progression unlocked",
}

// pollSurvival keeps a cursor over the plugin's events, including its durable
// death journal. Snapshot changes become milestones after a world baseline.
func (s *Service) pollSurvival(ctx context.Context, id string, c *Client, st *domain.AgentStatus) {
	s.mu.Lock()
	store := s.survivalStore
	if store == nil || !st.Ready || st.World.WorldUID == 0 {
		s.mu.Unlock()
		return
	}
	x := s.states[id]
	if x == nil {
		x = &state{}
		s.states[id] = x
	}
	if st.UptimeSeconds < x.survivalUptime {
		x.survivalSeq = 0
	}
	x.survivalUptime = st.UptimeSeconds
	since := x.survivalSeq
	s.mu.Unlock()

	events, next, err := c.Events(ctx, since)
	if err != nil {
		s.log.Debug("agent: survival events fetch", "instance", id, "err", err)
		return
	}
	for _, ev := range events {
		if ev.Seq <= since || ev.Kind != "player.death" {
			continue
		}
		var d deathPayload
		if json.Unmarshal(ev.Data, &d) != nil || d.WorldUID == 0 || d.CharacterID == "" {
			continue
		}
		run := d.RunID
		if run == "" {
			run = "legacy"
		}
		m := domain.SurvivalMoment{InstanceID: id, WorldUID: d.WorldUID, WorldName: d.WorldName, RunID: run, SourceSeq: ev.Seq, Kind: "death", CharacterID: d.CharacterID, PlayerName: d.PlayerName, Day: d.Day, At: ev.At, Label: "Death", Enemy: d.Enemy, EnemyLevel: d.EnemyLevel, Situation: d.Situation, Biome: d.Biome, X: d.X, Z: d.Z, Visible: d.Visible}
		if err := store.Insert(ctx, m); err != nil {
			s.log.Warn("agent: survival insert", "instance", id, "err", err)
			return
		}
	}
	s.mu.Lock()
	x = s.states[id]
	if x == nil {
		x = &state{}
		s.states[id] = x
	}
	if next > x.survivalSeq {
		x.survivalSeq = next
	}
	oldWorld := x.survivalWorld
	oldKeys := x.survivalKeys
	oldMods := x.survivalModifiers
	transition := x.survivalTransition
	keys := make(map[string]bool, len(st.GlobalKeys))
	for _, k := range st.GlobalKeys {
		keys[k] = true
	}
	mods := make(map[string]string, len(st.Modifiers))
	for k, v := range st.Modifiers {
		mods[k] = v
	}
	s.mu.Unlock()
	if oldWorld != st.World.WorldUID || oldKeys == nil {
		s.mu.Lock()
		x.survivalWorld = st.World.WorldUID
		x.survivalKeys = keys
		x.survivalModifiers = mods
		s.mu.Unlock()
		return
	}
	var pending []domain.SurvivalMoment
	appendMoment := func(kind, label string) {
		pending = append(pending, domain.SurvivalMoment{InstanceID: id, WorldUID: st.World.WorldUID, WorldName: st.World.Name, RunID: s.survivalRunID, SourceSeq: transition*1000 + int64(len(pending)+1), Kind: kind, Day: st.World.Day, At: time.Now().UTC(), Label: label, Visible: true})
	}
	var bossNames []string
	for key := range bossKeys {
		bossNames = append(bossNames, key)
	}
	sort.Strings(bossNames)
	for _, key := range bossNames {
		if keys[key] && !oldKeys[key] {
			appendMoment("progression", bossKeys[key])
		}
	}
	var modNames []string
	for key := range mods {
		modNames = append(modNames, key)
	}
	sort.Strings(modNames)
	for _, key := range modNames {
		value := mods[key]
		if old, ok := oldMods[key]; !ok || old != value {
			appendMoment("setting", strings.ReplaceAll(key, "_", " ")+" changed to "+value)
		}
	}
	var removed []string
	for key := range oldMods {
		if _, ok := mods[key]; !ok {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	for _, key := range removed {
		if _, ok := mods[key]; !ok {
			appendMoment("setting", strings.ReplaceAll(key, "_", " ")+" returned to default")
		}
	}
	for _, m := range pending {
		if err := store.Insert(ctx, m); err != nil {
			s.log.Warn("agent: survival milestone insert", "instance", id, "err", err)
			return
		}
	}
	s.mu.Lock()
	x.survivalWorld = st.World.WorldUID
	x.survivalKeys = keys
	x.survivalModifiers = mods
	if len(pending) > 0 {
		x.survivalTransition++
	}
	s.mu.Unlock()
}

func (s *Service) Survival(ctx context.Context, instanceID string, worldUID int64, characterID string, includeHidden bool) (*domain.SurvivalHistory, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	store := s.survivalStore
	s.mu.Unlock()
	worlds, err := store.Worlds(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if worldUID == 0 {
		s.mu.Lock()
		if x := s.states[instanceID]; x != nil {
			worldUID = x.survivalWorld
		}
		s.mu.Unlock()
		if worldUID == 0 && len(worlds) > 0 {
			worldUID = worlds[0]
		}
	}
	moments, err := store.List(ctx, instanceID, worldUID, characterID)
	if err != nil {
		return nil, err
	}
	if !includeHidden {
		for i := range moments {
			// A death-time opt-in cannot prove consent today. Keep exact historic
			// positions operator-only even if sharing was enabled at the death.
			moments[i].X = nil
			moments[i].Z = nil
		}
	}
	if worlds == nil {
		worlds = []int64{}
	}
	if moments == nil {
		moments = []domain.SurvivalMoment{}
	}
	return &domain.SurvivalHistory{Worlds: worlds, Moments: moments}, nil
}
