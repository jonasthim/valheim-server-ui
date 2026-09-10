package domain

import "time"

// BepInExPack identifies the Thunderstore package that provides the loader.
const (
	BepInExOwner = "denikson"
	BepInExName  = "BepInExPack_Valheim"
)

type ModSource string

const (
	ModSourceThunderstore ModSource = "thunderstore"
	ModSourceHexium       ModSource = "hexium"
	ModSourceManual       ModSource = "manual"
)

// Mod registries are Thunderstore-compatible package indexes. Both expose the
// same v1 package API, so the same client serves either; only the index URL
// and the host a download may come from differ.
const (
	RegistryThunderstoreID       = "thunderstore"
	RegistryThunderstoreName     = "Thunderstore"
	RegistryThunderstoreIndexURL = "https://thunderstore.io/c/valheim/api/v1/package/"

	RegistryHexiumID       = "hexium"
	RegistryHexiumName     = "Hexium"
	RegistryHexiumIndexURL = "https://valheim.hexium.gg/api/v1/package/"
)

// Registry is the API projection of one configured package registry.
type Registry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Mod struct {
	ID              int64      `json:"id"`
	InstanceID      string     `json:"instance_id"`
	Source          ModSource  `json:"source"`
	Owner           string     `json:"owner"`
	Name            string     `json:"name"`
	Version         string     `json:"version"`
	Enabled         bool       `json:"enabled"`
	LatestVersion   string     `json:"latest_version,omitempty"`
	UpdateAvailable bool       `json:"update_available"`
	Dependencies    []string   `json:"dependencies,omitempty"`
	IconURL         string     `json:"icon_url,omitempty"`
	WebsiteURL      string     `json:"website_url,omitempty"`
	Files           []string   `json:"-"` // relative to server dir
	InstalledAt     time.Time  `json:"installed_at"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// FullName is the Thunderstore identifier owner-name.
func (m Mod) FullName() string { return m.Owner + "-" + m.Name }

type BepInExStatus struct {
	Installed       bool   `json:"installed"`
	Enabled         bool   `json:"enabled"`
	Version         string `json:"version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

type ModsOverview struct {
	BepInEx        BepInExStatus `json:"bepinex"`
	Mods           []Mod         `json:"mods"`
	PendingRestart bool          `json:"pending_restart"`
}

type ConfigFileInfo struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

type ConfigRange struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

type ConfigEntry struct {
	Section          string       `json:"section"`
	Key              string       `json:"key"`
	Value            string       `json:"value"`
	Description      string       `json:"description,omitempty"`
	Type             string       `json:"type,omitempty"`
	DefaultValue     string       `json:"default_value,omitempty"`
	AcceptableValues []string     `json:"acceptable_values,omitempty"`
	Range            *ConfigRange `json:"range,omitempty"`
}

type ConfigFile struct {
	Name       string        `json:"name"`
	Raw        string        `json:"raw"`
	Entries    []ConfigEntry `json:"entries"`
	ModifiedAt time.Time     `json:"modified_at"`
}

type ConfigValueUpdate struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

type ConfigFileUpdate struct {
	Raw    *string             `json:"raw,omitempty"`
	Values []ConfigValueUpdate `json:"values,omitempty"`
}

// Thunderstore types (API projections of the v1 package index).

type PackageVersion struct {
	Version      string    `json:"version"`
	Description  string    `json:"description,omitempty"`
	DownloadURL  string    `json:"download_url"`
	Dependencies []string  `json:"dependencies"`
	Downloads    int64     `json:"downloads,omitempty"`
	FileSize     int64     `json:"file_size,omitempty"`
	DateCreated  time.Time `json:"date_created"`
}

type PackageSummary struct {
	Owner          string    `json:"owner"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name"`
	Description    string    `json:"description,omitempty"`
	IconURL        string    `json:"icon_url,omitempty"`
	PackageURL     string    `json:"package_url,omitempty"`
	LatestVersion  string    `json:"latest_version"`
	RatingScore    int       `json:"rating_score"`
	TotalDownloads int64     `json:"total_downloads"`
	IsDeprecated   bool      `json:"is_deprecated"`
	IsPinned       bool      `json:"is_pinned"`
	Categories     []string  `json:"categories"`
	DateUpdated    time.Time `json:"date_updated"`
}

type Package struct {
	PackageSummary
	WebsiteURL string           `json:"website_url,omitempty"`
	Versions   []PackageVersion `json:"versions"`
}

type PackageSearch struct {
	Query             string
	Category          string
	Sort              string // rating|downloads|updated|name
	IncludeDeprecated bool
	Page              int
	PageSize          int
}

type PackageSearchResult struct {
	Packages       []PackageSummary `json:"packages"`
	Total          int              `json:"total"`
	Page           int              `json:"page"`
	PageSize       int              `json:"page_size"`
	IndexUpdatedAt time.Time        `json:"index_updated_at"`
}
