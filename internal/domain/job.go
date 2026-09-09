package domain

import "time"

type JobType string

const (
	JobInstall             JobType = "install"
	JobUpdate              JobType = "update"
	JobBackup              JobType = "backup"
	JobRestore             JobType = "restore"
	JobWorldImport         JobType = "world_import"
	JobModInstall          JobType = "mod_install"
	JobModUpdate           JobType = "mod_update"
	JobModUninstall        JobType = "mod_uninstall"
	JobBepInExInstall      JobType = "bepinex_install"
	JobScheduledRestart    JobType = "scheduled_restart"
	JobThunderstoreRefresh JobType = "thunderstore_refresh"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// Terminal reports whether the status is final.
func (s JobStatus) Terminal() bool {
	return s == JobSucceeded || s == JobFailed || s == JobCancelled
}

// Job is a long-running operation. Output lives in <data>/jobs/<id>.log.
type Job struct {
	ID          string         `json:"id"`
	Type        JobType        `json:"type"`
	InstanceID  string         `json:"instance_id,omitempty"`
	Status      JobStatus      `json:"status"`
	Title       string         `json:"title,omitempty"`
	RequestedBy string         `json:"requested_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	Error       string         `json:"error,omitempty"`
	Summary     map[string]any `json:"summary,omitempty"`
}
