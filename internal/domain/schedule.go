package domain

import "time"

type ScheduleKind string

const (
	ScheduleRestart ScheduleKind = "restart"
	ScheduleBackup  ScheduleKind = "backup"
	ScheduleUpdate  ScheduleKind = "update"
)

type ScheduleInput struct {
	Kind          ScheduleKind `json:"kind"`
	Cron          string       `json:"cron"`
	Enabled       bool         `json:"enabled"`
	OnlyWhenEmpty bool         `json:"only_when_empty"`
	Note          string       `json:"note,omitempty"`
}

type Schedule struct {
	ScheduleInput
	ID         int64      `json:"id"`
	InstanceID string     `json:"instance_id"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastResult string     `json:"last_result,omitempty"` // ok|skipped|failed
	LastJobID  string     `json:"last_job_id,omitempty"`
}
