package domain

import "time"

// SurvivalMoment is a confirmed death or a world milestone. Cause fields stay
// empty when the dedicated server did not observe them.
type SurvivalMoment struct {
	ID          int64     `json:"id"`
	InstanceID  string    `json:"instance_id"`
	WorldUID    int64     `json:"world_uid"`
	WorldName   string    `json:"world_name"`
	RunID       string    `json:"-"`
	SourceSeq   int64     `json:"-"`
	Kind        string    `json:"kind"`
	CharacterID string    `json:"character_id,omitempty"`
	PlayerName  string    `json:"player_name,omitempty"`
	Day         int       `json:"day"`
	At          time.Time `json:"at"`
	Label       string    `json:"label"`
	Enemy       string    `json:"enemy,omitempty"`
	EnemyLevel  int       `json:"enemy_level,omitempty"`
	Situation   string    `json:"situation,omitempty"`
	Biome       string    `json:"biome,omitempty"`
	X           *float64  `json:"x,omitempty"`
	Z           *float64  `json:"z,omitempty"`
	Visible     bool      `json:"-"`
}

type SurvivalHistory struct {
	Worlds  []int64          `json:"worlds"`
	Moments []SurvivalMoment `json:"moments"`
}
