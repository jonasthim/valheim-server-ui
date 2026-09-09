package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InstanceIDPattern is the allowed shape of an instance id (slug). It is used
// verbatim in unit names (valheim@<id>.service) and directory names, and is
// re-validated by deploy/unitctl.
var InstanceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

var worldNamePattern = regexp.MustCompile(`^[A-Za-z0-9_\- ]{1,32}$`)

// Presets accepted by -preset ("" means none).
var Presets = []string{"", "normal", "casual", "easy", "hard", "hardcore", "immersive", "hammer"}

// ModifierValues lists the allowed values per -modifier key, in Valheim's order.
var ModifierValues = map[string][]string{
	"combat":       {"veryeasy", "easy", "hard", "veryhard"},
	"deathpenalty": {"casual", "veryeasy", "easy", "hard", "hardcore"},
	"resources":    {"muchless", "less", "more", "muchmore", "most"},
	"raids":        {"none", "muchless", "less", "more", "muchmore"},
	"portals":      {"casual", "hard", "veryhard"},
}

// ModifierOrder is the deterministic order modifiers are emitted in.
var ModifierOrder = []string{"combat", "deathpenalty", "resources", "raids", "portals"}

// SetKeys accepted by -setkey.
var SetKeys = []string{"nobuildcost", "playerevents", "passivemobs", "nomap"}

// Modifiers holds -modifier values; empty string means "not set".
type Modifiers struct {
	Combat       string `json:"combat,omitempty"`
	DeathPenalty string `json:"deathpenalty,omitempty"`
	Resources    string `json:"resources,omitempty"`
	Raids        string `json:"raids,omitempty"`
	Portals      string `json:"portals,omitempty"`
}

func (m Modifiers) get(key string) string {
	switch key {
	case "combat":
		return m.Combat
	case "deathpenalty":
		return m.DeathPenalty
	case "resources":
		return m.Resources
	case "raids":
		return m.Raids
	case "portals":
		return m.Portals
	}
	return ""
}

// InstanceConfig is everything that ends up on the Valheim command line plus the
// manager-side backup policy. See docs/ARCHITECTURE.md §5.1.
type InstanceConfig struct {
	Name               string    `json:"name"`
	World              string    `json:"world"`
	Password           string    `json:"password"`
	Port               int       `json:"port"`
	Public             bool      `json:"public"`
	Crossplay          bool      `json:"crossplay"`
	Preset             string    `json:"preset"`
	Modifiers          Modifiers `json:"modifiers"`
	SetKeys            []string  `json:"setkeys"`
	SaveIntervalSec    int       `json:"save_interval_sec"`
	GameBackups        int       `json:"game_backups"`
	GameBackupShortSec int       `json:"game_backup_short_sec"`
	GameBackupLongSec  int       `json:"game_backup_long_sec"`
	ExtraArgs          []string  `json:"extra_args"`
	BepInExEnabled     bool      `json:"bepinex_enabled"`
	BackupKeepLast     int       `json:"backup_keep_last"`
	BackupKeepDays     int       `json:"backup_keep_days"`
	BackupBeforeUpdate bool      `json:"backup_before_update"`
}

// DefaultInstanceConfig returns the defaults applied to omitted fields.
func DefaultInstanceConfig() InstanceConfig {
	return InstanceConfig{
		World:              "Dedicated",
		Port:               2456,
		Public:             false,
		Crossplay:          false,
		SetKeys:            []string{},
		SaveIntervalSec:    1800,
		GameBackups:        4,
		GameBackupShortSec: 7200,
		GameBackupLongSec:  43200,
		ExtraArgs:          []string{},
		BackupKeepLast:     10,
		BackupKeepDays:     30,
		BackupBeforeUpdate: true,
	}
}

// ApplyDefaults fills zero-valued numeric fields and nil slices with defaults.
// Booleans are left as provided.
func (c *InstanceConfig) ApplyDefaults() {
	d := DefaultInstanceConfig()
	if c.World == "" {
		c.World = d.World
	}
	if c.Port == 0 {
		c.Port = d.Port
	}
	if c.SaveIntervalSec == 0 {
		c.SaveIntervalSec = d.SaveIntervalSec
	}
	if c.GameBackupShortSec == 0 {
		c.GameBackupShortSec = d.GameBackupShortSec
	}
	if c.GameBackupLongSec == 0 {
		c.GameBackupLongSec = d.GameBackupLongSec
	}
	if c.SetKeys == nil {
		c.SetKeys = []string{}
	}
	if c.ExtraArgs == nil {
		c.ExtraArgs = []string{}
	}
}

// PortSpan is how many consecutive UDP ports an instance occupies (game, query, +1).
const PortSpan = 3

// PortsOverlap reports whether two instances' port ranges collide.
func PortsOverlap(a, b int) bool {
	return a < b+PortSpan && b < a+PortSpan
}

// Validate checks the config against Valheim's rules. Returns a *Error with
// CodeValidationFailed and per-field messages, or nil.
func (c InstanceConfig) Validate() error {
	var fs []FieldError
	add := func(f, m string) { fs = append(fs, FieldError{Field: f, Message: m}) }

	if n := strings.TrimSpace(c.Name); n == "" || len(n) > 64 {
		add("name", "must be 1-64 characters")
	}
	if !worldNamePattern.MatchString(c.World) {
		add("world", "letters, digits, space, _ and - only; 1-32 characters")
	}
	if len(c.Password) < 5 || len(c.Password) > 32 {
		add("password", "must be 5-32 characters")
	} else if strings.Contains(strings.ToLower(c.Name), strings.ToLower(c.Password)) {
		add("password", "must not be contained in the server name")
	}
	if c.Port < 1024 || c.Port > 65000 {
		add("port", "must be between 1024 and 65000")
	}
	if !contains(Presets, c.Preset) {
		add("preset", "unknown preset")
	}
	for _, key := range ModifierOrder {
		v := c.Modifiers.get(key)
		if v != "" && !contains(ModifierValues[key], v) {
			add("modifiers."+key, "invalid value "+strconv.Quote(v))
		}
	}
	seen := map[string]bool{}
	for _, k := range c.SetKeys {
		if !contains(SetKeys, k) {
			add("setkeys", "unknown key "+strconv.Quote(k))
		}
		if seen[k] {
			add("setkeys", "duplicate key "+strconv.Quote(k))
		}
		seen[k] = true
	}
	if c.SaveIntervalSec < 60 {
		add("save_interval_sec", "must be at least 60")
	}
	if c.GameBackups < 0 {
		add("game_backups", "must be >= 0")
	}
	if c.GameBackupShortSec < 60 {
		add("game_backup_short_sec", "must be at least 60")
	}
	if c.GameBackupLongSec < 60 {
		add("game_backup_long_sec", "must be at least 60")
	}
	for i, a := range c.ExtraArgs {
		if strings.ContainsAny(a, "\x00\n") {
			add(fmt.Sprintf("extra_args[%d]", i), "invalid characters")
		}
	}
	if c.BackupKeepLast < 0 {
		add("backup_keep_last", "must be >= 0")
	}
	if c.BackupKeepDays < 0 {
		add("backup_keep_days", "must be >= 0")
	}
	return Validation(fs)
}

// BuildArgs renders the Valheim command line arguments in the deterministic
// order documented in ARCHITECTURE.md §5.1 / §7. savedir is absolute.
func (c InstanceConfig) BuildArgs(savedir string) []string {
	args := []string{
		"-name", c.Name,
		"-port", strconv.Itoa(c.Port),
		"-world", c.World,
		"-password", c.Password,
		"-public", boolInt(c.Public),
		"-savedir", savedir,
	}
	if c.Crossplay {
		args = append(args, "-crossplay")
	}
	if c.Preset != "" {
		args = append(args, "-preset", c.Preset)
	}
	for _, key := range ModifierOrder {
		if v := c.Modifiers.get(key); v != "" {
			args = append(args, "-modifier", key, v)
		}
	}
	keys := append([]string(nil), c.SetKeys...)
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-setkey", k)
	}
	args = append(args,
		"-saveinterval", strconv.Itoa(c.SaveIntervalSec),
		"-backups", strconv.Itoa(c.GameBackups),
		"-backupshort", strconv.Itoa(c.GameBackupShortSec),
		"-backuplong", strconv.Itoa(c.GameBackupLongSec),
	)
	args = append(args, c.ExtraArgs...)
	args = append(args, "-nographics", "-batchmode")
	return args
}

// Masked returns a copy with the password replaced, for viewer-role responses.
func (c InstanceConfig) Masked() InstanceConfig {
	c.Password = "********"
	return c
}

func boolInt(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// InstanceState is the coarse lifecycle state shown in the UI.
type InstanceState string

const (
	StateNotInstalled InstanceState = "not_installed"
	StateStopped      InstanceState = "stopped"
	StateStarting     InstanceState = "starting"
	StateRunning      InstanceState = "running"
	StateStopping     InstanceState = "stopping"
	StateFailed       InstanceState = "failed"
)

// A2SInfo is the parsed A2S_INFO response from the query port.
type A2SInfo struct {
	ServerName        string    `json:"server_name"`
	Map               string    `json:"map,omitempty"`
	Players           int       `json:"players"`
	MaxPlayers        int       `json:"max_players"`
	Version           string    `json:"version,omitempty"`
	PasswordProtected bool      `json:"password_protected"`
	QueriedAt         time.Time `json:"queried_at"`
}

// InstanceStatus is the live view of an instance.
type InstanceStatus struct {
	InstanceID       string        `json:"instance_id"`
	State            InstanceState `json:"state"`
	Ready            bool          `json:"ready"`
	PID              int           `json:"pid,omitempty"`
	Since            *time.Time    `json:"since,omitempty"`
	Autostart        bool          `json:"autostart"`
	PendingRestart   bool          `json:"pending_restart"`
	Detail           string        `json:"detail,omitempty"`
	PlayersOnline    int           `json:"players_online"`
	MaxPlayers       int           `json:"max_players,omitempty"`
	JoinCode         string        `json:"join_code,omitempty"`
	A2S              *A2SInfo      `json:"a2s,omitempty"`
	InstalledBuildID string        `json:"installed_buildid,omitempty"`
	UpdateAvailable  bool          `json:"update_available"`
	BepInExInstalled bool          `json:"bepinex_installed"`
	BepInExEnabled   bool          `json:"bepinex_enabled"`
	ActiveJob        *Job          `json:"active_job,omitempty"`
}

// InstancePaths are the absolute directories of one instance.
type InstancePaths struct {
	Root    string `json:"root"`
	Server  string `json:"server"`
	Save    string `json:"save"`
	Backups string `json:"backups"`
	Logs    string `json:"logs"`
}

// Instance is the persisted entity plus its live status.
type Instance struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Config           InstanceConfig `json:"config"`
	Autostart        bool           `json:"-"`
	PendingRestart   bool           `json:"-"`
	InstalledBuildID string         `json:"-"`
	LatestBuildID    string         `json:"-"`
	BuildIDCheckedAt *time.Time     `json:"-"`
	Status           InstanceStatus `json:"status"`
	Paths            InstancePaths  `json:"paths"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// UpdateInfo is the result of a Steam build id check.
type UpdateInfo struct {
	InstanceID       string     `json:"instance_id"`
	InstalledBuildID string     `json:"installed_buildid,omitempty"`
	LatestBuildID    string     `json:"latest_buildid,omitempty"`
	UpdateAvailable  bool       `json:"update_available"`
	CheckedAt        *time.Time `json:"checked_at,omitempty"`
}

// Launch is the contract written to instances/<id>/launch.json and consumed by
// `valheim-ui launch`. See ARCHITECTURE.md §7.
type Launch struct {
	Version    int      `json:"version"`
	InstanceID string   `json:"instance_id"`
	ServerDir  string   `json:"server_dir"`
	LogDir     string   `json:"log_dir"`
	Args       []string `json:"args"`
	BepInEx    bool     `json:"bepinex"`
}

// LaunchVersion is the current launch.json schema version.
const LaunchVersion = 1

// SteamAppID is the Valheim dedicated server app id; SteamGameAppID is the
// game id exported as SteamAppId when launching.
const (
	SteamAppID     = "896660"
	SteamGameAppID = "892970"
)
