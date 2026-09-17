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

// AllBackupKinds lists every known backup kind, e.g. for validating
// InstanceConfig.RemoteBackup.Kinds (F-1.4).
var AllBackupKinds = []BackupKind{BackupManual, BackupScheduled, BackupPreUpdate, BackupPreRestore, BackupUploaded}

// validBackupKind reports whether k is one of AllBackupKinds.
func validBackupKind(k BackupKind) bool {
	for _, known := range AllBackupKinds {
		if k == known {
			return true
		}
	}
	return false
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
	// RemoteStatus/RemoteError track the off-site copy (F-1.4): ""
	// (never configured), "pending", "ok" or "failed", set by
	// backup.Service.SetRemoteStatus.
	RemoteStatus string `json:"remote_status,omitempty"`
	RemoteError  string `json:"remote_error,omitempty"`
}

// BackupTargetType is where an off-site backup copy is sent.
type BackupTargetType string

const (
	BackupTargetLocal  BackupTargetType = "local"
	BackupTargetRclone BackupTargetType = "rclone"
)

// BackupTarget is one configured off-site backup destination
// (Settings.Backups.Targets, F-1.4). Path is only meaningful for
// Type==local, Remote only for Type==rclone.
type BackupTarget struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Type     BackupTargetType `json:"type"`
	Path     string           `json:"path,omitempty"`
	Remote   string           `json:"remote,omitempty"`
	KeepLast int              `json:"keep_last"`
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
