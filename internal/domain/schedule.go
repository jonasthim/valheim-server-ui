package domain

import "time"

type ScheduleKind string

const (
	ScheduleRestart  ScheduleKind = "restart"
	ScheduleBackup   ScheduleKind = "backup"
	ScheduleUpdate   ScheduleKind = "update"
	ScheduleAnnounce ScheduleKind = "announce"
	ScheduleCommand  ScheduleKind = "command"
	ScheduleSave     ScheduleKind = "save"
)

type ScheduleInput struct {
	Kind          ScheduleKind `json:"kind"`
	Cron          string       `json:"cron"`
	Enabled       bool         `json:"enabled"`
	OnlyWhenEmpty bool         `json:"only_when_empty"`
	Note          string       `json:"note,omitempty"`
	// Message is the text an "announce" schedule broadcasts to players.
	Message string `json:"message,omitempty"`
	// Command is the agent command a "command" schedule sends.
	Command *AgentCommandRequest `json:"command,omitempty"`
	// LeadSeconds is a "restart" schedule's own warning lead, in seconds; 0
	// means the scheduler's default (scheduler.scheduledRestartLeadSeconds).
	LeadSeconds int `json:"lead_seconds"`
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
