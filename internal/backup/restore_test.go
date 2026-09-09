package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

func TestRestore_RoundTrip_ByteIdentical_SameWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	paths := env.writeWorldFiles("main", "Dedicated", []byte("original-db-bytes"), []byte("original-fwl-bytes"))

	backup, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "snapshot")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Diverge the live save files after the backup was taken.
	dbPath := filepath.Join(paths.WorldsDir(), "Dedicated.db")
	fwlPath := filepath.Join(paths.WorldsDir(), "Dedicated.fwl")
	if err := os.WriteFile(dbPath, []byte("CORRUPTED"), 0o640); err != nil {
		t.Fatalf("corrupt db: %v", err)
	}
	if err := os.WriteFile(fwlPath, []byte("CORRUPTED"), 0o640); err != nil {
		t.Fatalf("corrupt fwl: %v", err)
	}

	job := env.runRestoreSync("main", backup.ID, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected restore job to succeed, got %v (err=%s)", job.Status, job.Error)
	}

	gotDB, err := os.ReadFile(dbPath) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("read restored db: %v", err)
	}
	if !bytes.Equal(gotDB, []byte("original-db-bytes")) {
		t.Errorf("restored .db bytes do not match the backup: %q", gotDB)
	}
	gotFWL, err := os.ReadFile(fwlPath) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("read restored fwl: %v", err)
	}
	if !bytes.Equal(gotFWL, []byte("original-fwl-bytes")) {
		t.Errorf("restored .fwl bytes do not match the backup: %q", gotFWL)
	}

	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, b := range list {
		if b.Kind == domain.BackupPreRestore {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a pre_restore backup to have been created")
	}

	inst, err := env.inst.Get(context.Background(), "main")
	if err != nil {
		t.Fatalf("Get instance: %v", err)
	}
	if inst.Config.World != "Dedicated" {
		t.Errorf("active world should remain Dedicated, got %q", inst.Config.World)
	}
}

func TestRestore_SwitchesActiveWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "OldWorld", 2456)
	env.writeWorldFiles("main", "OldWorld", []byte("old-db"), []byte("old-fwl"))

	backup, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Switch the active world to something else, as if the operator moved on
	// and has actually played it (it has its own save files).
	newCfg := domain.InstanceConfig{Name: "Test main", World: "NewWorld", Password: "secret123", Port: 2456, Public: true}
	newCfg.ApplyDefaults()
	if _, err := env.inst.Update(context.Background(), "main", nil, &newCfg, nil); err != nil {
		t.Fatalf("Update to NewWorld: %v", err)
	}
	env.writeWorldFiles("main", "NewWorld", []byte("new-db"), []byte("new-fwl"))

	job := env.runRestoreSync("main", backup.ID, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected restore job to succeed, got %v (err=%s)", job.Status, job.Error)
	}

	inst, err := env.inst.Get(context.Background(), "main")
	if err != nil {
		t.Fatalf("Get instance: %v", err)
	}
	if inst.Config.World != "OldWorld" {
		t.Errorf("expected the active world to switch back to OldWorld, got %q", inst.Config.World)
	}
	restoredDB := filepath.Join(env.inst.Paths("main").WorldsDir(), "OldWorld.db")
	if got, err := os.ReadFile(restoredDB); err != nil || !bytes.Equal(got, []byte("old-db")) { //nolint:gosec // test-controlled path
		t.Errorf("expected OldWorld.db to be restored byte-identical, got %q err=%v", got, err)
	}
}

func TestRestore_RunningWithoutStopIfRunning_Fails(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))
	backup, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job := env.runRestoreSync("main", backup.ID, false)
	if job.Status != domain.JobFailed {
		t.Fatalf("expected restore job to fail while running without stop_if_running, got %v", job.Status)
	}
	if job.Error == "" {
		t.Errorf("expected a job error message explaining the instance_running failure")
	}
}

func TestRestore_StopsAndRestartsWhenRunning(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))
	env.markInstalled("main")
	backup, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job := env.runRestoreSync("main", backup.ID, true)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected restore job to succeed with stop_if_running, got %v (err=%s)", job.Status, job.Error)
	}

	st, err := env.inst.Status(context.Background(), "main")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != domain.StateRunning {
		t.Errorf("expected instance to be restarted (running) after restore, got %v", st.State)
	}
	if len(env.sup.stopCalls()) == 0 {
		t.Errorf("expected the instance to have been stopped for the restore")
	}
}

// runRestoreSync runs EnqueueRestore and blocks until the job finishes.
func (e *testEnv) runRestoreSync(instanceID string, backupID int64, stopIfRunning bool) *domain.Job {
	e.t.Helper()
	ctx := context.Background()
	job, err := e.svc.EnqueueRestore(ctx, instanceID, backupID, stopIfRunning, "tester")
	if err != nil {
		e.t.Fatalf("EnqueueRestore: %v", err)
	}
	final, err := e.run.WaitFor(ctx, job.ID)
	if err != nil {
		e.t.Fatalf("wait for restore job: %v", err)
	}
	return final
}
