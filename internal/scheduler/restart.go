package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// restartMarks are the standard "seconds remaining" points at which players
// are warned before a graceful restart. The actual delay is prepended when it
// is not already one of these, so the first warning always states the full
// wait. Descending; 0 (the restart itself) is not a warning.
var restartMarks = []int{600, 300, 120, 60, 30, 10}

// restartWarnMarks returns the countdown points (seconds remaining, descending)
// for a graceful restart delay: the delay itself, then every standard mark
// below it. Returns nil for a non-positive delay, where the caller restarts
// immediately with no warning.
func restartWarnMarks(delay int) []int {
	if delay <= 0 {
		return nil
	}
	marks := []int{delay}
	for _, m := range restartMarks {
		if m < delay {
			marks = append(marks, m)
		}
	}
	return marks
}

// restartCountdownMessage is the broadcast shown at secondsLeft before a
// restart, e.g. "Server restarting in 5 minutes" or "... in 30 seconds".
func restartCountdownMessage(secondsLeft int) string {
	switch {
	case secondsLeft%60 == 0 && secondsLeft >= 60:
		m := secondsLeft / 60
		if m == 1 {
			return "Server restarting in 1 minute"
		}
		return fmt.Sprintf("Server restarting in %d minutes", m)
	default:
		return fmt.Sprintf("Server restarting in %d seconds", secondsLeft)
	}
}

// EnqueueRestart enqueues a graceful restart of instanceID. With players
// online and delaySeconds > 0 it broadcasts a shrinking countdown, waits, then
// restarts; with nobody online (or no way to warn) it restarts immediately.
// The returned job is cancellable, which aborts the wait before the restart.
func (s *Service) EnqueueRestart(ctx context.Context, instanceID string, delaySeconds int, requestedBy string) (*domain.Job, error) {
	return s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobRestart,
		InstanceID:  instanceID,
		Title:       "Restart",
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		return s.gracefulRestart(ctx, instanceID, delaySeconds, log)
	})
}

// gracefulRestart is the shared warn-then-restart body used by both the manual
// restart endpoint and scheduled restarts. It never fails on a broadcast
// error (the restart still matters); a cancelled context aborts before the
// restart happens.
func (s *Service) gracefulRestart(ctx context.Context, instanceID string, delaySeconds int, log *jobs.Logger) error {
	st, err := s.inst.Status(ctx, instanceID)
	if err != nil {
		return err
	}
	if st.State != domain.StateRunning {
		log.Printf("instance not running, nothing to restart")
		return nil
	}

	// Only warn and wait when players are known to be online; an empty server
	// (or one we cannot count) restarts right away.
	n, known := s.players.PlayersOnline(ctx, instanceID)
	marks := restartWarnMarks(delaySeconds)
	if !known || n <= 0 || len(marks) == 0 {
		log.Printf("restarting now (players online: %d, known: %v)", n, known)
		_, err = s.inst.Restart(ctx, instanceID)
		return err
	}

	log.Printf("warning %d player(s) and restarting in %ds", n, delaySeconds)
	prev := delaySeconds
	for _, mark := range marks {
		// Wait from the previous mark down to this one, then announce it.
		if wait := prev - mark; wait > 0 {
			if err := s.sleep(ctx, time.Duration(wait)*time.Second); err != nil {
				return err
			}
		}
		s.broadcast(ctx, instanceID, restartCountdownMessage(mark), log)
		prev = mark
	}
	// Sleep the final stretch down to zero, then restart.
	if prev > 0 {
		if err := s.sleep(ctx, time.Duration(prev)*time.Second); err != nil {
			return err
		}
	}
	s.broadcast(ctx, instanceID, "Server restarting now", log)
	_, err = s.inst.Restart(ctx, instanceID)
	return err
}

// broadcast sends a centre-screen message through the agent, logging (never
// failing) when no agent is connected to carry it.
func (s *Service) broadcast(ctx context.Context, instanceID, message string, log *jobs.Logger) {
	if s.hooks.Broadcast == nil {
		return
	}
	if err := s.hooks.Broadcast(ctx, instanceID, message); err != nil {
		log.Printf("could not broadcast %q: %v", message, err)
	}
}
