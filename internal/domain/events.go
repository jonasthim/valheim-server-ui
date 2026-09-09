package domain

import (
	"context"
	"time"
)

// Event names sent over the SSE stream. Payloads documented in openapi.yaml → /events.
const (
	EventInstanceStatus  = "instance.status"
	EventInstanceLog     = "instance.log"
	EventInstancePlayers = "instance.players"
	EventJobUpdated      = "job.updated"
	EventJobLog          = "job.log"
	EventUpdateAvailable = "update.available"
	EventHeartbeat       = "heartbeat"
)

// Event is one message on the in-process bus / SSE stream.
type Event struct {
	Name       string    `json:"-"`
	InstanceID string    `json:"-"` // empty for global events
	TS         time.Time `json:"-"`
	Data       any       `json:"-"` // JSON-serialisable payload
}

// Publisher is implemented by events.Bus; packages depend on this interface.
type Publisher interface {
	Publish(ev Event)
}

// StatusEnricher lets feature packages add fields to InstanceStatus without the
// instance package importing them. Enrichers must be fast and never block.
type StatusEnricher interface {
	Enrich(ctx context.Context, st *InstanceStatus)
}

// StatusEnricherFunc adapts a function to StatusEnricher.
type StatusEnricherFunc func(ctx context.Context, st *InstanceStatus)

func (f StatusEnricherFunc) Enrich(ctx context.Context, st *InstanceStatus) { f(ctx, st) }

// PlayerCounter answers "how many players are online right now" for schedules
// and update jobs that must skip when the server is busy.
type PlayerCounter interface {
	PlayersOnline(ctx context.Context, instanceID string) (int, bool)
}
