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
var AgentCommands = []string{"save", "kick", "ban", "unban", "broadcast", "time", "say", "setkey", "removekey", "event", "eventstop"}

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
	WorldUID    int64   `json:"world_uid,omitempty"`
	// Event is the active random event (raid), or nil when none is running.
	Event *AgentWorldEvent `json:"event,omitempty"`
}

// AgentWorldEvent is the running random event (world.event / an event command's data).
type AgentWorldEvent struct {
	Name             string  `json:"name"`
	RemainingSeconds float64 `json:"remaining_seconds"`
	Position         Vec3    `json:"position"`
}

// AgentStatus is GET /v1/status as reported by the plugin.
type AgentStatus struct {
	AgentVersion  string     `json:"agent_version"`
	GameVersion   string     `json:"game_version,omitempty"`
	UptimeSeconds float64    `json:"uptime_seconds"`
	Ready         bool       `json:"ready"`
	CapturedAt    time.Time  `json:"captured_at"`
	World         AgentWorld `json:"world"`
	GlobalKeys    []string   `json:"global_keys"`
	// Modifiers are the world's difficulty modifiers (e.g. preset, playerdamage),
	// kept separate from GlobalKeys which now carries plain progression keys only.
	Modifiers map[string]string `json:"modifiers,omitempty"`
	Players   []AgentPlayer     `json:"players"`
	// Pings are the map pings of the last few seconds (agents 1.10+).
	Pings []AgentPing `json:"pings"`
}

// AgentPing is a map ping a player sent in game.
type AgentPing struct {
	Name     string    `json:"name"`
	Position Vec3      `json:"position"`
	At       time.Time `json:"at"`
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
	// Explored is the fog-of-war state, refreshed with every poll so the
	// map learns about new mask versions through the event stream.
	Explored *ExploredInfo `json:"explored,omitempty"`
}

type AgentEvent struct {
	Seq  int64           `json:"seq"`
	Kind string          `json:"kind"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data"`
}

type AgentCommandRequest struct {
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`  // kick/ban/unban: player name or platform id
	Message string `json:"message,omitempty"` // broadcast/say: the text
	Style   string `json:"style,omitempty"`   // broadcast: center|topleft
	// time (exactly one of):
	Skip     string   `json:"skip,omitempty"`     // "morning" (the game's own skip)
	Fraction *float64 `json:"fraction,omitempty"` // time of day 0..1
	Seconds  *float64 `json:"seconds,omitempty"`  // advance the clock by this many seconds (1..86400)
	// say: the sender name shown in chat (default the server name)
	Name string `json:"name,omitempty"`
	// setkey/removekey: the global key
	Key string `json:"key,omitempty"`
	// event: the event name (from the catalog) and an optional anchor
	Event string   `json:"event,omitempty"`
	X     *float64 `json:"x,omitempty"`
	Z     *float64 `json:"z,omitempty"`
}

type AgentCommandResult struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// AgentCatalog is GET /v1/catalog: the pickers for the key and event commands.
type AgentCatalog struct {
	GlobalKeys []string        `json:"global_keys"`
	Events     []AgentEventDef `json:"events"`
	ServerName string          `json:"server_name,omitempty"`
}

// AgentEventDef is one random event the world can run.
type AgentEventDef struct {
	Name            string  `json:"name"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// AgentChat is GET /v1/chat: recent shouts and normal chat (never whispers).
type AgentChat struct {
	Messages []AgentChatMessage `json:"messages"`
	Next     int64              `json:"next"`
}

// AgentChatMessage is one line in the chat feed.
type AgentChatMessage struct {
	Seq      int64     `json:"seq"`
	At       time.Time `json:"at"`
	Type     string    `json:"type"` // shout|normal
	Sender   string    `json:"sender"`
	Text     string    `json:"text"`
	Position *Vec3     `json:"position,omitempty"`
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
	// Layers is true when the agent exports raw map layers (1.10+) for the
	// manager to draw; older agents serve a flat styled image instead.
	Layers        bool   `json:"layers,omitempty"`
	LayersVersion int    `json:"layers_version,omitempty"`
	Error         string `json:"error,omitempty"`
}

// MapObject is a point of interest read from the server's ZDO store.
type MapObject struct {
	Type  string  `json:"type"` // portal|ship|cart|tombstone|bed
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Text  string  `json:"text,omitempty"` // portal tag, owner name
	// Explored is false when the object lies under the fog; nil from agents
	// that predate fog (treat as explored).
	Explored *bool `json:"explored,omitempty"`
}

// MapLocation is one of the game's own map icons (boss altars, start temple).
type MapLocation struct {
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Explored *bool   `json:"explored,omitempty"`
	// Discovered is true when a boss pin shared on a cartography table (the
	// mark a Vegvisir adds) sits at this location, so it shows through the fog.
	Discovered *bool `json:"discovered,omitempty"`
}

// MapPin is a pin players shared on a cartography table: their own marks and
// the boss locations Vegvisir runestones add to their maps.
type MapPin struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
	// Type is a stable name for the game's PinType: fire, house, mine, cave,
	// death, bed, portal, boss, hildir or other.
	Type    string `json:"type"`
	TypeID  int    `json:"type_id"`
	Checked bool   `json:"checked"`
	// Author is the placing player's name when known; "" for an offline
	// player whose id the game recorded.
	Author string `json:"author,omitempty"`
	// Source is "table" for a pin shared on a cartography table or
	// "vegvisir" for a location the game revealed through a runestone.
	Source string `json:"source,omitempty"`
}

// ExploredInfo is the plugin's fog-of-war state (GET /v1/map/explored/info).
type ExploredInfo struct {
	Version       int        `json:"version"`
	Size          int        `json:"size"`
	ExploredCells int        `json:"explored_cells"`
	TotalCells    int        `json:"total_cells"`
	Percent       float64    `json:"percent"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	// MaskVersion is the exploration version the served mask image reflects.
	MaskVersion int `json:"mask_version"`
}

// MapObjects is GET /v1/map/objects.
type MapObjects struct {
	Objects   []MapObject   `json:"objects"`
	Pins      []MapPin      `json:"pins"`
	Locations []MapLocation `json:"locations"`
	UpdatedAt *time.Time    `json:"updated_at"`
}

// MapTiles describes the tile pyramid GET /instances/{id}/map/tiles/{z}/{x}/{y}.png
// serves: TileSize px tiles, 2^z per side at zoom z up to MaxZoom. Version
// changes whenever tile bytes would (style, texture pack or fog mask), so
// clients append it to tile URLs.
type MapTiles struct {
	TileSize int    `json:"tile_size"`
	MaxZoom  int    `json:"max_zoom"`
	Version  string `json:"version"`
}

// InstanceMap is what GET /instances/{id}/map returns: everything the Map
// tab needs besides the image itself.
type InstanceMap struct {
	Connected bool `json:"connected"`
	// MapSupported is false when the running agent predates the map API
	// (its /v1/map/info answers 404); AgentVersion says which one it is.
	MapSupported bool   `json:"map_supported"`
	AgentVersion string `json:"agent_version,omitempty"`
	// LayersSupported is true when the running agent exports raw layers so
	// the manager draws the map in the in-game style; false for agents
	// before 1.10.0, whose own flat render is served instead.
	LayersSupported bool `json:"layers_supported"`
	// StyleVersion is the manager's map style; it changes when the look does.
	StyleVersion int `json:"style_version,omitempty"`
	// FogSupported is false when the agent has no exploration tracking
	// (agents before 1.7.0); Explored carries the fog state otherwise.
	FogSupported bool          `json:"fog_supported"`
	Explored     *ExploredInfo `json:"explored,omitempty"`
	// ImageReady is true when GET /instances/{id}/map.png serves an image now
	// (from the instance cache or the running agent).
	ImageReady bool `json:"image_ready"`
	// ImageVersion changes whenever map.png would serve different bytes (a
	// new render or a fog rebuild); clients append it to the image URL.
	ImageVersion string `json:"image_version,omitempty"`
	// Tiles is the deep-zoom pyramid; nil when the agent exports no layers.
	Tiles *MapTiles `json:"tiles,omitempty"`
	// Stale is true when the served image was cached from an earlier run and
	// the agent is not reachable to confirm it matches the current world.
	Stale     bool          `json:"stale"`
	Info      *MapInfo      `json:"info,omitempty"`
	Objects   []MapObject   `json:"objects"`
	Pins      []MapPin      `json:"pins"`
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
