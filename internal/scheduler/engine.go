package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/robfig/cron/v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// Schedule.last_result values (docs/openapi.yaml → Schedule.last_result enum).
const (
	resultOK      = "ok"
	resultSkipped = "skipped"
	resultFailed  = "failed"
)

// Run loads every enabled schedule into one robfig/cron instance (host
// location) and blocks until ctx is done, at which point the cron engine and
// every job-wait goroutine started by a firing are stopped before Run
// returns. Run must be called at most once per Service.
func (s *Service) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scheduler: already running")
	}
	s.started = true
	s.mu.Unlock()

	s.reload(ctx)

	<-ctx.Done()

	s.baseCancel()

	s.mu.Lock()
	eng := s.cronEng
	s.cronEng = nil
	s.mu.Unlock()
	if eng != nil {
		<-eng.Stop().Done()
	}
	s.wg.Wait()
	return nil
}

// reload rebuilds the cron engine from the schedules currently enabled in the
// database. Called after every Create/Update/Delete and once at Run startup.
// Errors are logged, never returned: a bad reload must not take down the
// service, and callers (handlers) already succeeded at persisting the
// change by the time reload runs.
func (s *Service) reload(ctx context.Context) {
	s.mu.Lock()
	running := s.started
	s.mu.Unlock()
	if !running {
		return // Run hasn't started yet; it will load everything itself.
	}

	rows, err := s.listEnabledScheduleRows(ctx)
	if err != nil {
		s.log.Error("scheduler: reload: list schedules", "err", err)
		return
	}

	next := cron.New(cron.WithLocation(s.loc))
	for _, r := range rows {
		sched, err := s.parser.Parse(r.CronExpr)
		if err != nil {
			s.log.Warn("scheduler: reload: skip schedule with invalid cron", "schedule", r.ID, "cron", r.CronExpr, "err", err)
			continue
		}
		id := r.ID
		next.Schedule(sched, cron.FuncJob(func() { s.fireSchedule(id) }))
	}
	next.Start()

	s.mu.Lock()
	old := s.cronEng
	s.cronEng = next
	s.mu.Unlock()
	if old != nil {
		old.Stop()
	}
}

// fireSchedule runs the schedule scheduled to fire now. It never lets a
// panic or error propagate out: one broken schedule must not stop the cron
// engine or affect any other schedule.
func (s *Service) fireSchedule(id int64) {
	row, err := s.getScheduleRow(s.baseCtx, id)
	if err != nil {
		s.log.Error("scheduler: fire: load schedule", "schedule", id, "err", err)
		return
	}
	if _, _, err := s.runAndRecord(s.baseCtx, row, "scheduler"); err != nil {
		s.log.Error("scheduler: fire: execute schedule", "schedule", row.ID, "instance", row.InstanceID, "kind", row.Kind, "err", err)
	}
}

// runAndRecord executes sched's action, persists last_run_at/last_result/
// last_job_id, and — when a job was enqueued — starts a goroutine that waits
// for it and downgrades last_result to "failed" if the job does not
// succeed. It returns the job (if any), the skip reason (if any), and any
// error the action itself raised (as opposed to a job later failing, which
// only updates last_result asynchronously).
func (s *Service) runAndRecord(ctx context.Context, sched scheduleRow, requestedBy string) (*domain.Job, string, error) {
	job, skipReason, err := s.safeExecute(ctx, sched, requestedBy)

	result := resultOK
	jobID := ""
	switch {
	case err != nil:
		result = resultFailed
	case skipReason != "":
		result = resultSkipped
	case job != nil:
		jobID = job.ID
	}

	at := s.now().UTC()
	if uerr := s.updateLastRun(context.Background(), sched.ID, at, result, jobID); uerr != nil {
		s.log.Error("scheduler: persist last run", "schedule", sched.ID, "err", uerr)
	}

	if job != nil && err == nil {
		s.waitAndRecordFailure(sched.ID, job.ID)
	}

	return job, skipReason, err
}

// safeExecute wraps execute with a recover so a panicking hook or job
// function cannot take down the cron engine or another schedule's firing.
func (s *Service) safeExecute(ctx context.Context, sched scheduleRow, requestedBy string) (job *domain.Job, skipReason string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("schedule %d panicked: %v", sched.ID, p)
		}
	}()
	return s.execute(ctx, sched, requestedBy)
}

// execute runs one schedule's action per ARCHITECTURE.md §11:
//   - restart: skipped if only_when_empty and players are known to be online;
//     otherwise enqueues a scheduled_restart job that itself no-ops (without
//     failing) when the instance is not running.
//   - backup: delegates to hooks.Backup.
//   - update: skipped if no update is available, or if only_when_empty and
//     players are known to be online; otherwise delegates to hooks.Update.
func (s *Service) execute(ctx context.Context, sched scheduleRow, requestedBy string) (*domain.Job, string, error) {
	switch domain.ScheduleKind(sched.Kind) {
	case domain.ScheduleRestart:
		return s.executeRestart(ctx, sched, requestedBy)
	case domain.ScheduleBackup:
		job, err := s.hooks.Backup(ctx, sched.InstanceID, requestedBy)
		return job, "", err
	case domain.ScheduleUpdate:
		return s.executeUpdate(ctx, sched, requestedBy)
	default:
		return nil, "", domain.Ef(domain.CodeInternal, "unknown schedule kind %q", sched.Kind)
	}
}

func (s *Service) executeRestart(ctx context.Context, sched scheduleRow, requestedBy string) (*domain.Job, string, error) {
	if reason, skip := s.skipForPlayers(ctx, sched); skip {
		return nil, reason, nil
	}

	instanceID := sched.InstanceID
	job, err := s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobScheduledRestart,
		InstanceID:  instanceID,
		Title:       "Scheduled restart",
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		st, err := s.inst.Status(ctx, instanceID)
		if err != nil {
			return err
		}
		if st.State != domain.StateRunning {
			log.Printf("instance not running, nothing to restart")
			return nil
		}
		_, err = s.inst.Restart(ctx, instanceID)
		return err
	})
	return job, "", err
}

func (s *Service) executeUpdate(ctx context.Context, sched scheduleRow, requestedBy string) (*domain.Job, string, error) {
	avail, err := s.hooks.UpdateAvailable(ctx, sched.InstanceID)
	if err != nil {
		return nil, "", err
	}
	if !avail {
		return nil, "no update available", nil
	}
	if reason, skip := s.skipForPlayers(ctx, sched); skip {
		return nil, reason, nil
	}
	job, err := s.hooks.Update(ctx, sched.InstanceID, requestedBy, true)
	return job, "", err
}

// skipForPlayers reports whether sched should be skipped because
// only_when_empty is set and players are known to currently be online.
func (s *Service) skipForPlayers(ctx context.Context, sched scheduleRow) (reason string, skip bool) {
	if !sched.OnlyWhenEmpty {
		return "", false
	}
	n, known := s.players.PlayersOnline(ctx, sched.InstanceID)
	if !known || n <= 0 {
		return "", false
	}
	if n == 1 {
		return "1 player online", true
	}
	return fmt.Sprintf("%d players online", n), true
}

// waitAndRecordFailure waits for jobID to reach a terminal state and, if it
// did not succeed, downgrades the schedule's last_result to "failed". It
// runs on s.baseCtx so it is cancelled (not leaked) when Run's ctx is done,
// and is tracked by s.wg so Run can wait for it to exit before returning.
func (s *Service) waitAndRecordFailure(scheduleID int64, jobID string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		job, err := s.runner.WaitFor(s.baseCtx, jobID)
		if err != nil {
			return // context cancelled (shutdown) or job unknown; nothing to record
		}
		if job.Status == domain.JobSucceeded {
			return
		}
		if uerr := s.updateLastResult(context.Background(), scheduleID, resultFailed); uerr != nil {
			s.log.Error("scheduler: persist last result", "schedule", scheduleID, "job", jobID, "err", uerr)
		}
	}()
}
