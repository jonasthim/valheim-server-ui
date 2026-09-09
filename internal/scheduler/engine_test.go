package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// waitForCondition polls cond until it is true or the deadline passes,
// giving the runAndRecord/waitAndRecordFailure background goroutine time to
// persist its result without a fixed sleep.
func waitForCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met before deadline")
	}
}

// testHarness bundles a Service with the real pieces needed to exercise
// execute()/runAndRecord() end to end: a real *instance.Service (for the
// restart path) and a real *jobs.Runner (so hooks can enqueue genuine jobs
// and the scheduler's async WaitFor bookkeeping runs for real).
type testHarness struct {
	svc     *Service
	sup     *fakeSupervisor
	runner  *jobs.Runner
	players *fakePlayers

	backupCalls  []hookCall
	updateCalls  []hookCall
	availChecked []string

	backupErr  error
	updateErr  error
	availErr   error
	available  bool
	failBackup bool // when true, the backup hook's enqueued job Func returns an error
	failUpdate bool
}

type hookCall struct {
	instanceID  string
	requestedBy string
}

func newTestHarness(t *testing.T, opts ...Option) *testHarness {
	t.Helper()
	sqldb := newTestDB(t)
	inst, sup := newTestInstanceService(t, sqldb)
	createTestInstance(t, inst, "main")
	runner := jobs.New(sqldb, nil, t.TempDir(), newTestLogger())
	players := newFakePlayers()

	h := &testHarness{sup: sup, runner: runner, players: players}

	hooks := Hooks{
		Backup: func(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
			h.backupCalls = append(h.backupCalls, hookCall{instanceID, requestedBy})
			if h.backupErr != nil {
				return nil, h.backupErr
			}
			return runner.Enqueue(ctx, jobs.Spec{
				Type: domain.JobBackup, InstanceID: instanceID, Title: "Test backup", RequestedBy: requestedBy,
			}, func(_ context.Context, _ *jobs.Logger) error {
				if h.failBackup {
					return errors.New("simulated backup failure")
				}
				return nil
			})
		},
		Update: func(ctx context.Context, instanceID, requestedBy string, _ bool) (*domain.Job, error) {
			h.updateCalls = append(h.updateCalls, hookCall{instanceID, requestedBy})
			if h.updateErr != nil {
				return nil, h.updateErr
			}
			return runner.Enqueue(ctx, jobs.Spec{
				Type: domain.JobUpdate, InstanceID: instanceID, Title: "Test update", RequestedBy: requestedBy,
			}, func(_ context.Context, _ *jobs.Logger) error {
				if h.failUpdate {
					return errors.New("simulated update failure")
				}
				return nil
			})
		},
		UpdateAvailable: func(_ context.Context, instanceID string) (bool, error) {
			h.availChecked = append(h.availChecked, instanceID)
			return h.available, h.availErr
		},
	}

	h.svc = New(sqldb, inst, runner, players, hooks, newTestLogger(), opts...)
	// Every fire may start a waitAndRecordFailure goroutine that writes to
	// sqldb asynchronously; wait for those to finish before the db closes
	// (t.Cleanup runs LIFO, so registering this after newTestDB's Close
	// means it runs first) so a lingering write can never race a later
	// test's fresh in-memory database.
	t.Cleanup(func() { h.svc.wg.Wait() })
	return h
}

func mustCreateSchedule(t *testing.T, svc *Service, in domain.ScheduleInput) scheduleRow {
	t.Helper()
	sc, err := svc.Create(context.Background(), "main", in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	row, err := svc.getScheduleRow(context.Background(), sc.ID)
	if err != nil {
		t.Fatalf("getScheduleRow: %v", err)
	}
	return row
}

// --- backup ------------------------------------------------------------

func TestExecute_BackupFires_HookInvokedAndOK(t *testing.T) {
	h := newTestHarness(t)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "* * * * *", Enabled: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no skip, got %q", skipReason)
	}
	if job == nil {
		t.Fatal("expected a job")
	}
	if len(h.backupCalls) != 1 || h.backupCalls[0].instanceID != "main" || h.backupCalls[0].requestedBy != "scheduler" {
		t.Fatalf("unexpected backup hook calls: %+v", h.backupCalls)
	}

	if _, err := h.runner.WaitFor(context.Background(), job.ID); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	waitForCondition(t, func() bool {
		r, err := h.svc.getScheduleRow(context.Background(), row.ID)
		return err == nil && r.LastResult == resultOK && r.LastJobID == job.ID
	})
}

func TestExecute_BackupJobFails_LastResultFailed(t *testing.T) {
	h := newTestHarness(t)
	h.failBackup = true
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "* * * * *", Enabled: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no skip, got %q", skipReason)
	}

	finalJob, err := h.runner.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if finalJob.Status != domain.JobFailed {
		t.Fatalf("expected the job to fail, got %v", finalJob.Status)
	}

	// last_result starts "ok" (enqueue succeeded) then flips to "failed"
	// asynchronously once the job's own Func returns an error.
	waitForCondition(t, func() bool {
		r, err := h.svc.getScheduleRow(context.Background(), row.ID)
		return err == nil && r.LastResult == resultFailed
	})
}

func TestExecute_BackupHookError_RecordsFailed(t *testing.T) {
	h := newTestHarness(t)
	h.backupErr = errors.New("boom")
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "* * * * *", Enabled: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err == nil {
		t.Fatal("expected an error")
	}
	if job != nil || skipReason != "" {
		t.Fatalf("expected no job/skip on hook error, got job=%v skip=%q", job, skipReason)
	}

	r, err := h.svc.getScheduleRow(context.Background(), row.ID)
	if err != nil {
		t.Fatalf("getScheduleRow: %v", err)
	}
	if r.LastResult != resultFailed {
		t.Fatalf("expected last_result failed, got %q", r.LastResult)
	}
}

// --- restart -------------------------------------------------------------

func TestExecute_Restart_OnlyWhenEmpty_SkipsWhenPlayersOnline(t *testing.T) {
	h := newTestHarness(t)
	h.players.set("main", 2)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{
		Kind: domain.ScheduleRestart, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: true,
	})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no job when players are online, got %+v", job)
	}
	if skipReason == "" {
		t.Fatal("expected a skip reason")
	}

	r, err := h.svc.getScheduleRow(context.Background(), row.ID)
	if err != nil {
		t.Fatalf("getScheduleRow: %v", err)
	}
	if r.LastResult != resultSkipped {
		t.Fatalf("expected last_result skipped, got %q", r.LastResult)
	}
}

func TestExecute_Restart_OnlyWhenEmpty_RunsWhenPlayersUnknownOrZero(t *testing.T) {
	h := newTestHarness(t)
	// players unknown (never set) -> must not skip.
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{
		Kind: domain.ScheduleRestart, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: true,
	})
	h.sup.setState("main", supervisor.StateRunning)

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no skip when player count is unknown, got %q", skipReason)
	}
	if job == nil {
		t.Fatal("expected a job")
	}
	finalJob, err := h.runner.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if finalJob.Status != domain.JobSucceeded {
		t.Fatalf("expected the restart job to succeed, got %v: %s", finalJob.Status, finalJob.Error)
	}
	if len(h.sup.restartCallsSnapshot()) == 0 {
		t.Fatal("expected the supervisor's Restart to have been called")
	}
}

func TestExecute_Restart_NotRunning_SkipsInsideJobWithoutFailing(t *testing.T) {
	h := newTestHarness(t)
	// Instance defaults to "stopped" in fakeSupervisor.
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{
		Kind: domain.ScheduleRestart, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: false,
	})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no schedule-level skip, got %q", skipReason)
	}
	finalJob, err := h.runner.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if finalJob.Status != domain.JobSucceeded {
		t.Fatalf("expected the job to succeed (no-op) when the instance is not running, got %v: %s", finalJob.Status, finalJob.Error)
	}
	if len(h.sup.restartCallsSnapshot()) != 0 {
		t.Fatal("expected the supervisor's Restart NOT to have been called")
	}
	waitForCondition(t, func() bool {
		r, err := h.svc.getScheduleRow(context.Background(), row.ID)
		return err == nil && r.LastResult == resultOK
	})
}

// --- update ----------------------------------------------------------------

func TestExecute_Update_SkipsWhenNoUpdateAvailable(t *testing.T) {
	h := newTestHarness(t)
	h.available = false
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleUpdate, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no job, got %+v", job)
	}
	if skipReason == "" {
		t.Fatal("expected a skip reason")
	}
	if len(h.updateCalls) != 0 {
		t.Fatalf("hooks.Update must not be called when no update is available: %+v", h.updateCalls)
	}
	if len(h.availChecked) != 1 || h.availChecked[0] != "main" {
		t.Fatalf("expected UpdateAvailable to be checked for main, got %+v", h.availChecked)
	}
}

func TestExecute_Update_OnlyWhenEmpty_SkipsWhenPlayersOnline(t *testing.T) {
	h := newTestHarness(t)
	h.available = true
	h.players.set("main", 1)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleUpdate, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "scheduler")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no job, got %+v", job)
	}
	if skipReason == "" {
		t.Fatal("expected a skip reason")
	}
	if len(h.updateCalls) != 0 {
		t.Fatalf("hooks.Update must not be called when players are online: %+v", h.updateCalls)
	}
}

func TestExecute_Update_RunsAndInvokesHookWithRequestedBy(t *testing.T) {
	h := newTestHarness(t)
	h.available = true
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleUpdate, Cron: "* * * * *", Enabled: true, OnlyWhenEmpty: true})

	job, skipReason, err := h.svc.runAndRecord(context.Background(), row, "someone")
	if err != nil {
		t.Fatalf("runAndRecord: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("expected no skip, got %q", skipReason)
	}
	if job == nil {
		t.Fatal("expected a job")
	}
	if len(h.updateCalls) != 1 || h.updateCalls[0].instanceID != "main" || h.updateCalls[0].requestedBy != "someone" {
		t.Fatalf("unexpected update hook calls: %+v", h.updateCalls)
	}
}

// --- RunNow ------------------------------------------------------------

func TestRunNow_ReturnsJob(t *testing.T) {
	h := newTestHarness(t)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true})

	job, err := h.svc.RunNow(context.Background(), "main", row.ID, "alice")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if job == nil {
		t.Fatal("expected a job")
	}
	if len(h.backupCalls) != 1 || h.backupCalls[0].requestedBy != "alice" {
		t.Fatalf("unexpected backup hook calls: %+v", h.backupCalls)
	}
}

func TestRunNow_SkippedReturnsConflictError(t *testing.T) {
	h := newTestHarness(t)
	h.players.set("main", 3)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{
		Kind: domain.ScheduleRestart, Cron: "@daily", Enabled: true, OnlyWhenEmpty: true,
	})

	job, err := h.svc.RunNow(context.Background(), "main", row.ID, "alice")
	if job != nil {
		t.Fatalf("expected no job, got %+v", job)
	}
	de := requireDomainError(t, err)
	if de.Code != domain.CodeConflict {
		t.Fatalf("expected conflict, got %v", de.Code)
	}
	if len(de.Message) == 0 || de.Message[:len("skipped:")] != "skipped:" {
		t.Fatalf("expected message to start with 'skipped:', got %q", de.Message)
	}
}

func TestRunNow_UnknownInstance(t *testing.T) {
	h := newTestHarness(t)
	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true})
	_, err := h.svc.RunNow(context.Background(), "does-not-exist", row.ID, "alice")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found, got %v", de.Code)
	}
}

func TestRunNow_ScheduleFromOtherInstance(t *testing.T) {
	h := newTestHarness(t)
	inst2, _ := newTestInstanceService(t, h.svc.db)
	createTestInstance(t, inst2, "second")

	row := mustCreateSchedule(t, h.svc, domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true})
	_, err := h.svc.RunNow(context.Background(), "second", row.ID, "alice")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found, got %v", de.Code)
	}
}
