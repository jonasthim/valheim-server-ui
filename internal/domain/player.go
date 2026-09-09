package domain

import "time"

type ListKind string

const (
	ListAdmin     ListKind = "admin"
	ListBanned    ListKind = "banned"
	ListPermitted ListKind = "permitted"
)

// FileName returns the file Valheim reads for this list, relative to savedir.
func (k ListKind) FileName() string {
	switch k {
	case ListAdmin:
		return "adminlist.txt"
	case ListBanned:
		return "bannedlist.txt"
	case ListPermitted:
		return "permittedlist.txt"
	}
	return ""
}

func (k ListKind) Valid() bool { return k.FileName() != "" }

type PlayerListEntry struct {
	ID      string `json:"id"`
	Comment string `json:"comment,omitempty"`
}

type PlayerList struct {
	Kind    ListKind          `json:"kind"`
	Entries []PlayerListEntry `json:"entries"`
}

type OnlinePlayer struct {
	PlatformID  string     `json:"platform_id,omitempty"`
	Name        string     `json:"name"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
}

type KnownPlayer struct {
	PlatformID   string    `json:"platform_id"`
	Name         string    `json:"name,omitempty"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	SessionCount int       `json:"session_count"`
}

type PlayersResponse struct {
	Online      []OnlinePlayer `json:"online"`
	OnlineCount int            `json:"online_count"`
	CountSource string         `json:"count_source"` // a2s|log|none
	Known       []KnownPlayer  `json:"known"`
}
