package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// testEnv bundles everything a backup service test needs: a real
// instance.Service and jobs.Runner backed by an in-memory DB, and a fake
// supervisor so start/stop/status never touch a real process.
type testEnv struct {
	t    *testing.T
	svc  *Service
	inst *instance.Service
	sup  *fakeSupervisor
	run  *jobs.Runner
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Supervisor = "direct"

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup := newFakeSupervisor()
	inst := instance.New(sqldb, nil, sup, cfg, log)
	runner := jobs.New(sqldb, nil, cfg.JobsDir(), log)
	svc := New(sqldb, inst, runner, nil, "test-version", log)

	return &testEnv{t: t, svc: svc, inst: inst, sup: sup, run: runner}
}

// createInstance creates an instance whose active world is `world`, with no
// save files yet.
func (e *testEnv) createInstance(id, world string, port int) *domain.Instance {
	e.t.Helper()
	cfg := domain.InstanceConfig{Name: "Test " + id, World: world, Password: "secret123", Port: port, Public: true}
	cfg.ApplyDefaults()
	inst, err := e.inst.Create(context.Background(), id, "Test Instance "+id, cfg, false)
	if err != nil {
		e.t.Fatalf("create instance %s: %v", id, err)
	}
	return inst
}

// writeWorldFiles writes world.db/.fwl (and, if given, list files) under
// id's worlds_local directory with the given content.
func (e *testEnv) writeWorldFiles(id, world string, db, fwl []byte) domain.InstancePaths {
	e.t.Helper()
	paths := e.inst.Paths(id)
	dir := paths.WorldsDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		e.t.Fatalf("mkdir worlds dir: %v", err)
	}
	if db != nil {
		if err := os.WriteFile(filepath.Join(dir, world+".db"), db, 0o640); err != nil {
			e.t.Fatalf("write %s.db: %v", world, err)
		}
	}
	if fwl != nil {
		if err := os.WriteFile(filepath.Join(dir, world+".fwl"), fwl, 0o640); err != nil {
			e.t.Fatalf("write %s.fwl: %v", world, err)
		}
	}
	return paths
}

// markInstalled writes a fake server binary so instance.Service.Start does
// not refuse with instance_not_installed.
func (e *testEnv) markInstalled(id string) {
	e.t.Helper()
	bin := e.inst.Paths(id).ServerBinary()
	if err := os.MkdirAll(filepath.Dir(bin), 0o750); err != nil {
		e.t.Fatalf("mkdir server dir: %v", err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture executable
		e.t.Fatalf("write fake server binary: %v", err)
	}
}

func requireDomainError(t *testing.T, err error) *domain.Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	return domain.AsError(err)
}

// readZipManifest opens path and decodes its manifest.json entry.
func readZipManifest(t *testing.T, path string) domain.BackupManifest {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open manifest.json: %v", err)
		}
		defer func() { _ = rc.Close() }()
		var m domain.BackupManifest
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			t.Fatalf("decode manifest.json: %v", err)
		}
		return m
	}
	t.Fatalf("zip %s has no manifest.json", path)
	return domain.BackupManifest{}
}

func zipEntryNames(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer func() { _ = zr.Close() }()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// runJobSync enqueues fn against env's real jobs.Runner (the only way to get
// a live *jobs.Logger, since jobs.Logger has no exported constructor) and
// blocks until it finishes.
func (e *testEnv) runJobSync(instanceID string, jobType domain.JobType, fn jobs.Func) *domain.Job {
	e.t.Helper()
	ctx := context.Background()
	job, err := e.run.Enqueue(ctx, jobs.Spec{Type: jobType, InstanceID: instanceID, Title: "test"}, fn)
	if err != nil {
		e.t.Fatalf("enqueue job: %v", err)
	}
	final, err := e.run.WaitFor(ctx, job.ID)
	if err != nil {
		e.t.Fatalf("wait for job: %v", err)
	}
	return final
}

// ---------------------------------------------------------------- Create

func TestCreate_Success(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	paths := env.writeWorldFiles("main", "Dedicated", []byte("db-content"), []byte("fwl-content"))
	if err := os.WriteFile(filepath.Join(paths.Save, domain.ListAdmin.FileName()), []byte("76561198000000000\n"), 0o640); err != nil {
		t.Fatalf("write adminlist: %v", err)
	}

	b, err := env.svc.Create(context.Background(), "main", domain.BackupScheduled, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.InstanceID != "main" || b.World != "Dedicated" || b.Kind != domain.BackupScheduled {
		t.Errorf("unexpected backup: %+v", b)
	}
	if b.ID == 0 {
		t.Errorf("expected a non-zero id")
	}
	full := filepath.Join(paths.Backups, b.Filename)
	fi, err := os.Stat(full)
	if err != nil {
		t.Fatalf("stat backup file: %v", err)
	}
	if fi.Size() != b.SizeBytes {
		t.Errorf("SizeBytes mismatch: row=%d disk=%d", b.SizeBytes, fi.Size())
	}

	names := zipEntryNames(t, full)
	for _, want := range []string{"worlds_local/Dedicated.fwl", "worlds_local/Dedicated.db", "adminlist.txt", "manifest.json"} {
		if !containsString(names, want) {
			t.Errorf("expected zip to contain %q, got %v", want, names)
		}
	}

	m := readZipManifest(t, full)
	if m.InstanceID != "main" || m.World != "Dedicated" || m.Kind != domain.BackupScheduled || m.AppVersion != "test-version" {
		t.Errorf("unexpected manifest: %+v", m)
	}
}

func TestCreate_MissingDB(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	// No .db written at all.
	_, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestCreate_UnknownInstance(t *testing.T) {
	env := newTestEnv(t)
	_, err := env.svc.Create(context.Background(), "nope", domain.BackupManual, "")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
}

func TestPreUpdateBackup(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))

	job := env.runJobSync("main", domain.JobUpdate, func(ctx context.Context, log *jobs.Logger) error {
		return env.svc.PreUpdateBackup(ctx, "main", log)
	})
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected job to succeed, got %v (err=%s)", job.Status, job.Error)
	}
	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Kind != domain.BackupPreUpdate {
		t.Fatalf("expected one pre_update backup, got %+v", list)
	}
}
