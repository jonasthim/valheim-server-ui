package domain

import "time"

type BackupKind string

const (
	BackupManual     BackupKind = "manual"
	BackupScheduled  BackupKind = "scheduled"
	BackupPreUpdate  BackupKind = "pre_update"
	BackupPreRestore BackupKind = "pre_restore"
	BackupUploaded   BackupKind = "uploaded"
)

// AutoDeletable reports whether retention may remove backups of this kind.
func (k BackupKind) AutoDeletable() bool {
	return k == BackupScheduled || k == BackupPreUpdate || k == BackupPreRestore
}

type Backup struct {
	ID         int64      `json:"id"`
	InstanceID string     `json:"instance_id"`
	World      string     `json:"world"`
	Kind       BackupKind `json:"kind"`
	Filename   string     `json:"filename"`
	SizeBytes  int64      `json:"size_bytes"`
	Note       string     `json:"note,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	Missing    bool       `json:"missing,omitempty"`
}

// BackupManifest is stored as manifest.json inside every backup zip.
type BackupManifest struct {
	InstanceID     string     `json:"instance_id"`
	World          string     `json:"world"`
	Kind           BackupKind `json:"kind"`
	CreatedAt      time.Time  `json:"created_at"`
	ValheimBuildID string     `json:"valheim_buildid,omitempty"`
	AppVersion     string     `json:"app_version"`
	Files          []string   `json:"files"`
}

type World struct {
	Name       string    `json:"name"`
	Active     bool      `json:"active"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
	HasDB      bool      `json:"has_db"`
	HasFWL     bool      `json:"has_fwl"`
}
