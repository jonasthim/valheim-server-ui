// Package supervisor abstracts how the game process is started and stopped.
// Production uses systemd through the sudo `unitctl` wrapper; development and
// tests use the direct child-process implementation. See ARCHITECTURE.md §6.
package supervisor

import (
	"context"
	"time"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateFailed   State = "failed"
)

type Status struct {
	State     State
	PID       int
	Since     time.Time // zero if unknown
	Autostart bool
	Detail    string // e.g. systemd Result= when failed
}

// Supervisor controls one unit per instance id. Implementations must be safe
// for concurrent use. Start on a running instance and Stop on a stopped one are
// no-ops that return nil.
type Supervisor interface {
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error
	Status(ctx context.Context, id string) (Status, error)
	SetAutostart(ctx context.Context, id string, on bool) error
	// Kind is "systemd" or "direct".
	Kind() string
}

// UnitName returns the systemd unit for an instance id.
func UnitName(id string) string { return "valheim@" + id + ".service" }
