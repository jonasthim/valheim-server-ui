package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

func TestGameBackupParseStem(t *testing.T) {
	ts := time.Date(2026, 9, 9, 14, 48, 3, 0, time.UTC)
	tests := []struct {
		name   string
		stem   string
		world  string
		kind   string
		ts     time.Time
		wantOK bool
	}{
		{"auto", "Midgard_backup_auto-20260909144803", "Midgard", "auto", ts, true},
		{"cloud", "Midgard_backup_cloud-20260909144803", "Midgard", "cloud", ts, true},
		{"restore", "Midgard_backup_restore-20260909144803", "Midgard", "restore", ts, true},
		{"world name contains _backup_", "My_backup_World_backup_auto-20260909144803", "My_backup_World", "auto", ts, true},
		{"world name contains a marker", "A_backup_auto-1_backup_cloud-20260909144803", "A_backup_auto-1", "cloud", ts, true},
		{"bad timestamp", "Midgard_backup_auto-garbage", "Midgard", "auto", time.Time{}, true},
		{"empty world", "_backup_auto-20260909144803", "", "", time.Time{}, false},
		{"invalid world with separator", "a/b_backup_auto-20260909144803", "", "", time.Time{}, false},
		{"not a copy", "Midgard", "", "", time.Time{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			world, kind, got, ok := parseGameBackupStem(tc.stem)
			if ok != tc.wantOK || world != tc.world || kind != tc.kind || !got.Equal(tc.ts) {
				t.Errorf("parseGameBackupStem(%q) = %q %q %v %v; want %q %q %v %v", tc.stem, world, kind, got, ok, tc.world, tc.kind, tc.ts, tc.wantOK)
			}
		})
	}
}

// writeCopyDir fabricates a directory-layout rolling copy; the .db2 body is
// the marker so tests can tell copy content from live content.
func writeCopyDir(t *testing.T, worldsDir, stem string, committed bool, marker string) string {
	t.Helper()
	dir := filepath.Join(worldsDir, stem)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"_main.1.fwl2":     "fwl",
		"_main.1.db2":      marker,
		"_main.1.chunks":   "chunks",
		"1e_1e__1_1.chunk": "chunk",
	}
	if committed {
		files["_main.1.ok"] = ""
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestGameBackupScan(t *testing.T) {
	dir := t.TempDir()
	writeCopyDir(t, dir, "Midgard_backup_auto-20260909144803", true, "x")
	writeCopyDir(t, dir, "Midgard_backup_auto-20260910090000", true, "x")
	writeCopyDir(t, dir, "Midgard_backup_cloud-20260908000000", false, "x") // no .ok
	writeFile(t, filepath.Join(dir, "Old_backup_auto-20260101000000.db"), "db")
	writeFile(t, filepath.Join(dir, "Old_backup_auto-20260101000000.fwl"), "fwl")
	writeFile(t, filepath.Join(dir, "Old_backup_auto-20260101000000.db.old"), "ignored")
	writeFile(t, filepath.Join(dir, "Half_backup_restore-20260102000000.fwl"), "fwl") // no .db
	writeFile(t, filepath.Join(dir, "Plain.db"), "db")                                // normal world
	writeCopyDir(t, dir, "Plain", true, "x")                                          // normal world dir
	if err := os.Symlink(filepath.Join(dir, "Midgard_backup_auto-20260910090000"), filepath.Join(dir, "Link_backup_auto-20260911000000")); err != nil {
		t.Fatal(err)
	}

	got, err := scanGameBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name, layout, kind string
		restorable         bool
	}
	want := []row{
		{"Midgard_backup_auto-20260910090000", "directory", "auto", true},
		{"Midgard_backup_auto-20260909144803", "directory", "auto", true},
		{"Midgard_backup_cloud-20260908000000", "directory", "cloud", false},
		{"Half_backup_restore-20260102000000", "legacy", "restore", false},
		{"Old_backup_auto-20260101000000", "legacy", "auto", true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d copies, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.Layout != w.layout || g.Kind != w.kind || g.Restorable != w.restorable {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
	}
	if got[4].SizeBytes != int64(len("db")+len("fwl")) {
		t.Errorf("legacy size = %d, want sum of the pair", got[4].SizeBytes)
	}
	if got[0].World != "Midgard" || !got[0].CreatedAt.Equal(time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected world/created_at: %+v", got[0])
	}
}

func TestGameBackupScan_MissingDir(t *testing.T) {
	got, err := scanGameBackups(filepath.Join(t.TempDir(), "nope"))
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("want empty non-nil slice, got %v err=%v", got, err)
	}
}

func TestGameBackupScan_BadTimestampUsesMtime(t *testing.T) {
	dir := t.TempDir()
	p := writeCopyDir(t, dir, "W_backup_auto-garbage", true, "x")
	mt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	entries, _ := os.ReadDir(p)
	for _, e := range entries {
		if err := os.Chtimes(filepath.Join(p, e.Name()), mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanGameBackups(dir)
	if err != nil || len(got) != 1 || !got[0].CreatedAt.Equal(mt) {
		t.Errorf("want mtime fallback %v, got %+v err=%v", mt, got, err)
	}
}

func (e *testEnv) runGameRestoreSync(instanceID, name string, stopIfRunning bool) *domain.Job {
	e.t.Helper()
	ctx := context.Background()
	job, err := e.svc.EnqueueGameBackupRestore(ctx, instanceID, name, stopIfRunning, "tester")
	if err != nil {
		e.t.Fatalf("EnqueueGameBackupRestore: %v", err)
	}
	final, err := e.run.WaitFor(ctx, job.ID)
	if err != nil {
		e.t.Fatalf("wait: %v", err)
	}
	return final
}

func countPreRestore(t *testing.T, env *testEnv, id string) int {
	t.Helper()
	list, err := env.svc.List(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range list {
		if b.Kind == domain.BackupPreRestore {
			n++
		}
	}
	return n
}

func TestGameBackupRestore_DirectoryOverDirectory(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	worlds := env.inst.Paths("main").WorldsDir()
	live := env.writeWorldDir("main", "Dedicated", worldGen{N: 5, Committed: true})
	const stem = "Dedicated_backup_auto-20260909144803"
	copyDir := writeCopyDir(t, worlds, stem, true, "copy-marker")
	wantFiles := dirFileNames(t, copyDir)

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	if got := dirFileNames(t, live); strings.Join(got, ",") != strings.Join(wantFiles, ",") {
		t.Errorf("world dir files\n got %v\nwant %v", got, wantFiles)
	}
	b, err := os.ReadFile(filepath.Join(live, "_main.1.db2")) //nolint:gosec // test path
	if err != nil || string(b) != "copy-marker" {
		t.Errorf("world db2 = %q err=%v", b, err)
	}
	if got := dirFileNames(t, copyDir); strings.Join(got, ",") != strings.Join(wantFiles, ",") {
		t.Errorf("the copy must stay untouched, got %v", got)
	}
	if countPreRestore(t, env, "main") != 1 {
		t.Errorf("expected one pre_restore backup")
	}
	if pre, _ := job.Summary["pre_restore_backup"].(string); pre == "" || job.Summary["restored_world"] != "Dedicated" {
		t.Errorf("summary = %v", job.Summary)
	}
	// No staging leftovers.
	left, _ := filepath.Glob(filepath.Join(env.inst.Paths("main").Save, ".restore-*"))
	if len(left) != 0 {
		t.Errorf("staging leftovers: %v", left)
	}
}

func TestGameBackupRestore_LegacyOverLegacy(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	paths := env.writeWorldFiles("main", "Dedicated", []byte("live-db"), []byte("live-fwl"))
	worlds := paths.WorldsDir()
	const stem = "Dedicated_backup_auto-20260909144803"
	writeFile(t, filepath.Join(worlds, stem+".db"), "copy-db")
	writeFile(t, filepath.Join(worlds, stem+".fwl"), "copy-fwl")

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	for ext, want := range map[string]string{".db": "copy-db", ".fwl": "copy-fwl"} {
		b, err := os.ReadFile(filepath.Join(worlds, "Dedicated"+ext)) //nolint:gosec // test path
		if err != nil || string(b) != want {
			t.Errorf("Dedicated%s = %q err=%v", ext, b, err)
		}
		if _, err := os.Stat(filepath.Join(worlds, stem+ext)); err != nil {
			t.Errorf("copy %s must remain: %v", ext, err)
		}
	}
	if countPreRestore(t, env, "main") != 1 {
		t.Errorf("expected one pre_restore backup")
	}
}

func TestGameBackupRestore_RunningWithoutStopIfRunning(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldDir("main", "Dedicated", worldGen{N: 5, Committed: true})
	const stem = "Dedicated_backup_auto-20260909144803"
	writeCopyDir(t, env.inst.Paths("main").WorldsDir(), stem, true, "m")
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobFailed || !strings.Contains(job.Error, "stop_if_running") {
		t.Fatalf("want failed instance_running job, got %v %q", job.Status, job.Error)
	}
	if countPreRestore(t, env, "main") != 0 {
		t.Errorf("no backup should be taken when the restore is refused")
	}
}

func TestGameBackupRestore_StopsAndRestarts(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.markInstalled("main")
	env.writeWorldDir("main", "Dedicated", worldGen{N: 5, Committed: true})
	const stem = "Dedicated_backup_auto-20260909144803"
	writeCopyDir(t, env.inst.Paths("main").WorldsDir(), stem, true, "m")
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job := env.runGameRestoreSync("main", stem, true)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	if len(env.sup.stopCalls()) == 0 {
		t.Errorf("expected the instance to be stopped")
	}
	st, err := env.inst.Status(context.Background(), "main")
	if err != nil || st.State != domain.StateRunning {
		t.Errorf("expected restart, got %v err=%v", st.State, err)
	}
}

func TestGameBackupRestore_Rejections(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	worlds := env.inst.Paths("main").WorldsDir()
	writeCopyDir(t, worlds, "Dedicated_backup_auto-20260909144803", false, "m") // not committed
	writeFile(t, filepath.Join(worlds, "Half_backup_auto-20260909144803.db"), "db")
	writeFile(t, filepath.Join(worlds, "Half_backup_auto-20260909144803.old"), "x")

	tests := []struct {
		name     string
		in       string
		wantCode domain.ErrorCode
	}{
		{"unknown", "Nope_backup_auto-20260909144803", domain.CodeNotFound},
		{"not a copy name", "Dedicated", domain.CodeNotFound},
		{"traversal", "../x_backup_auto-1", domain.CodeNotFound},
		{"dotdot", "..", domain.CodeNotFound},
		{"empty", "", domain.CodeNotFound},
		{"not restorable", "Dedicated_backup_auto-20260909144803", domain.CodeValidationFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.svc.EnqueueGameBackupRestore(context.Background(), "main", tc.in, false, "t")
			if err == nil {
				t.Fatalf("expected error")
			}
			if de := requireDomainError(t, err); de.Code != tc.wantCode {
				t.Errorf("code = %v, want %v", de.Code, tc.wantCode)
			}
		})
	}

	// Legacy copy without a .fwl: queued (restorable by .db) but the job fails before touching the world.
	paths := env.writeWorldFiles("main", "Half", []byte("live"), []byte("live-fwl"))
	_ = paths
	job := env.runGameRestoreSync("main", "Half_backup_auto-20260909144803", false)
	if job.Status != domain.JobFailed {
		t.Fatalf("expected failure for a pair without .fwl, got %v", job.Status)
	}
	b, err := os.ReadFile(filepath.Join(worlds, "Half.db")) //nolint:gosec // test path
	if err != nil || string(b) != "live" {
		t.Errorf("live world must be untouched, got %q err=%v", b, err)
	}
}

func TestGameBackupRestore_OtherWorldSwitchesActive(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Alpha", 2456)
	env.writeWorldFiles("main", "Alpha", []byte("alpha-db"), []byte("alpha-fwl"))
	worlds := env.inst.Paths("main").WorldsDir()
	const stem = "Beta_backup_cloud-20260909144803"
	writeCopyDir(t, worlds, stem, true, "beta-marker")

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	inst, err := env.inst.Get(context.Background(), "main")
	if err != nil || inst.Config.World != "Beta" {
		t.Errorf("active world = %q err=%v, want Beta", inst.Config.World, err)
	}
	b, err := os.ReadFile(filepath.Join(worlds, "Beta", "_main.1.db2")) //nolint:gosec // test path
	if err != nil || string(b) != "beta-marker" {
		t.Errorf("Beta db2 = %q err=%v", b, err)
	}
	// Beta had no save, so there was nothing to protect; the previously
	// active world is left exactly as it was.
	if countPreRestore(t, env, "main") != 0 {
		t.Errorf("expected no pre_restore backup when the copy's world has no save yet")
	}
	if a, err := os.ReadFile(filepath.Join(worlds, "Alpha.db")); err != nil || string(a) != "alpha-db" { //nolint:gosec // test path
		t.Errorf("Alpha.db = %q err=%v, want it untouched", a, err)
	}
}

func TestGameBackupRestore_OtherWorldWithSaveIsProtected(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Alpha", 2456)
	env.writeWorldFiles("main", "Alpha", []byte("alpha-db"), []byte("alpha-fwl"))
	env.writeWorldFiles("main", "Beta", []byte("beta-old-db"), []byte("beta-old-fwl"))
	worlds := env.inst.Paths("main").WorldsDir()
	const stem = "Beta_backup_auto-20260909144803"
	writeCopyDir(t, worlds, stem, true, "beta-marker")

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	backups, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	var pre []domain.Backup
	for _, b := range backups {
		if b.Kind == domain.BackupPreRestore {
			pre = append(pre, b)
		}
	}
	if len(pre) != 1 || pre[0].World != "Beta" {
		t.Fatalf("pre_restore backups = %+v, want exactly one for world Beta", pre)
	}
}

func TestGameBackupRestore_ActiveWorldWithoutSaveSkipsPreBackup(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Fresh", 2456) // never saved
	worlds := env.inst.Paths("main").WorldsDir()
	const stem = "Fresh_backup_auto-20260909144803"
	writeCopyDir(t, worlds, stem, true, "m")

	job := env.runGameRestoreSync("main", stem, false)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job = %v err=%s", job.Status, job.Error)
	}
	if countPreRestore(t, env, "main") != 0 {
		t.Errorf("no pre_restore backup possible without a save")
	}
}

func TestGameBackupList_UnknownInstance(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.svc.ListGameBackups(context.Background(), "nope"); err == nil {
		t.Errorf("expected not-found error")
	}
}
