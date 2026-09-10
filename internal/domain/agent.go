package domain

import (
	"encoding/json"
	"time"
)

// The Valheim UI Agent is the server-side BepInEx plugin (plugin/) that the
// manager installs together with BepInEx. It exposes the running world over a
// loopback HTTP API guarded by a per-instance token (ARCHITECTURE.md §20).
const (
	// AgentModOwner/AgentModName is the identity the agent package is
	// recorded under in the mods table (manifest author/name, slugified).
	AgentModOwner = "jonasthim"
	AgentModName  = "valheimui_agent"
	// AgentPluginDLL is the plugin file inside BepInEx/plugins/<owner>-<name>/.
	AgentPluginDLL = "ValheimUI.Agent.dll"
	// AgentConfigFile is the BepInEx config file the manager writes the
	// port and token into before every start.
	AgentConfigFile = "se.jonasthim.valheimui.agent.cfg"
	// AgentAssetName is the release asset carrying the plugin package.
	AgentAssetName = "valheim-ui-agent.zip"
)

// EventAgentStatus carries an AgentInfo for one instance whenever the poller
// has news (connection changes, players moving).
const EventAgentStatus = "agent.status"

// JobAgentInstall installs or updates the agent plugin on an instance.
const JobAgentInstall JobType = "agent_install"

// ModSourceBundled marks a mod shipped with the manager itself (the agent).
const ModSourceBundled ModSource = "bundled"

// AgentCommands are the verbs the plugin accepts.
var AgentCommands = []string{"save", "kick", "ban", "unban", "broadcast"}

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// AgentPlayer is one connected player as the server sees it. Position is
// omitted for players who hide their map position unless the caller may see
// everything (operator and above).
type AgentPlayer struct {
	UID         int64  `json:"uid"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	CharacterID string `json:"character_id,omitempty"`
	Visible     bool   `json:"visible"`
	Position    *Vec3  `json:"position,omitempty"`
}

type AgentWorld struct {
	Name        string  `json:"name"`
	SeedName    string  `json:"seed_name,omitempty"`
	Seed        int     `json:"seed"`
	Day         int     `json:"day"`
	DayFraction float64 `json:"day_fraction"`
	IsNight     bool    `json:"is_night"`
	Weather     string  `json:"weather,omitempty"`
	TimeSeconds float64 `json:"time_seconds"`
}

// AgentStatus is GET /v1/status as reported by the plugin.
type AgentStatus struct {
	AgentVersion  string        `json:"agent_version"`
	GameVersion   string        `json:"game_version,omitempty"`
	UptimeSeconds float64       `json:"uptime_seconds"`
	Ready         bool          `json:"ready"`
	CapturedAt    time.Time     `json:"captured_at"`
	World         AgentWorld    `json:"world"`
	GlobalKeys    []string      `json:"global_keys"`
	Players       []AgentPlayer `json:"players"`
}

// AgentInfo is what the API reports for one instance.
type AgentInfo struct {
	Installed        bool         `json:"installed"`
	InstalledVersion string       `json:"installed_version,omitempty"`
	BundledVersion   string       `json:"bundled_version,omitempty"`
	UpdateAvailable  bool         `json:"update_available"`
	Enabled          bool         `json:"enabled"` // BepInEx (and thus the agent) is enabled
	Connected        bool         `json:"connected"`
	LastSeen         *time.Time   `json:"last_seen,omitempty"`
	LastError        string       `json:"last_error,omitempty"`
	Status           *AgentStatus `json:"status,omitempty"`
}

type AgentEvent struct {
	Seq  int64           `json:"seq"`
	Kind string          `json:"kind"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data"`
}

type AgentCommandRequest struct {
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message,omitempty"`
	Style   string `json:"style,omitempty"` // broadcast: center|topleft
}

type AgentCommandResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// MapInfo is the plugin's render state for the world map (GET /v1/map/info).
type MapInfo struct {
	State          string  `json:"state"` // idle|rendering|encoding|ready|failed
	Progress       float64 `json:"progress"`
	Seed           int     `json:"seed"`
	Size           int     `json:"size"`
	WorldRadius    float64 `json:"world_radius"`
	PlayableRadius float64 `json:"playable_radius"`
	SeaLevel       float64 `json:"sea_level"`
	Error          string  `json:"error,omitempty"`
}

// MapObject is a point of interest read from the server's ZDO store.
type MapObject struct {
	Type  string  `json:"type"` // portal|ship|cart|tombstone|bed
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Text  string  `json:"text,omitempty"` // portal tag, owner name
}

// MapLocation is one of the game's own map icons (boss altars, start temple).
type MapLocation struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}

// MapObjects is GET /v1/map/objects.
type MapObjects struct {
	Objects   []MapObject   `json:"objects"`
	Locations []MapLocation `json:"locations"`
	UpdatedAt *time.Time    `json:"updated_at"`
}

// InstanceMap is what GET /instances/{id}/map returns: everything the Map
// tab needs besides the image itself.
type InstanceMap struct {
	Connected bool `json:"connected"`
	// MapSupported is false when the running agent predates the map API
	// (its /v1/map/info answers 404); AgentVersion says which one it is.
	MapSupported bool   `json:"map_supported"`
	AgentVersion string `json:"agent_version,omitempty"`
	// ImageReady is true when GET /instances/{id}/map.png serves an image now
	// (from the instance cache or the running agent).
	ImageReady bool `json:"image_ready"`
	// Stale is true when the served image was cached from an earlier run and
	// the agent is not reachable to confirm it matches the current world.
	Stale     bool          `json:"stale"`
	Info      *MapInfo      `json:"info,omitempty"`
	Objects   []MapObject   `json:"objects"`
	Locations []MapLocation `json:"locations"`
	ObjectsAt *time.Time    `json:"objects_updated_at,omitempty"`
	Players   []AgentPlayer `json:"players"`
	World     *AgentWorld   `json:"world,omitempty"`
}

// MapRenderRequest asks the plugin to (re)render the map.
type MapRenderRequest struct {
	Size  int  `json:"size,omitempty"`
	Force bool `json:"force,omitempty"`
}
