package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

func TestListWorlds(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db-bytes"), []byte("fwl-bytes"))
	env.writeWorldFiles("main", "Other", []byte("x"), nil) // .db only, no .fwl

	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 2 {
		t.Fatalf("expected 2 worlds, got %d: %+v", len(worlds), worlds)
	}
	byName := map[string]domain.World{}
	for _, w := range worlds {
		byName[w.Name] = w
	}
	ded, ok := byName["Dedicated"]
	if !ok {
		t.Fatalf("expected Dedicated in the list")
	}
	if !ded.Active || !ded.HasDB || !ded.HasFWL || ded.SizeBytes != int64(len("db-bytes")+len("fwl-bytes")) {
		t.Errorf("unexpected Dedicated world entry: %+v", ded)
	}
	other, ok := byName["Other"]
	if !ok {
		t.Fatalf("expected Other in the list")
	}
	if other.Active || !other.HasDB || other.HasFWL {
		t.Errorf("unexpected Other world entry: %+v", other)
	}
}

func TestListWorlds_NoDirectoryYet(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	// worlds_local exists (created by instance.Create) but is empty.
	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 0 {
		t.Errorf("expected no worlds, got %+v", worlds)
	}
}

func buildWorldZip(t *testing.T, world string, db, fwl []byte) *bytes.Buffer {
	t.Helper()
	files := map[string][]byte{}
	if db != nil {
		files[world+".db"] = db
	}
	if fwl != nil {
		files[world+".fwl"] = fwl
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return &buf
}

func TestWorldImport_Pair(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	files := map[string]io.Reader{
		"Imported.db":  bytes.NewReader([]byte("imported-db")),
		"Imported.fwl": bytes.NewReader([]byte("imported-fwl")),
	}
	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, false, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("wait for job: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected job to succeed, got %v (err=%s)", final.Status, final.Error)
	}

	dir := env.inst.Paths("main").WorldsDir()
	gotDB, err := os.ReadFile(filepath.Join(dir, "Imported.db"))
	if err != nil || string(gotDB) != "imported-db" {
		t.Errorf("unexpected Imported.db content: %q err=%v", gotDB, err)
	}
	gotFWL, err := os.ReadFile(filepath.Join(dir, "Imported.fwl"))
	if err != nil || string(gotFWL) != "imported-fwl" {
		t.Errorf("unexpected Imported.fwl content: %q err=%v", gotFWL, err)
	}
}

func TestWorldImport_Zip(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	z := buildWorldZip(t, "ZippedWorld", []byte("zdb"), []byte("zfwl"))
	files := map[string]io.Reader{"world.zip": z}
	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, false, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("wait for job: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected job to succeed, got %v (err=%s)", final.Status, final.Error)
	}

	dir := env.inst.Paths("main").WorldsDir()
	gotDB, err := os.ReadFile(filepath.Join(dir, "ZippedWorld.db"))
	if err != nil || string(gotDB) != "zdb" {
		t.Errorf("unexpected ZippedWorld.db content: %q err=%v", gotDB, err)
	}
}

// TestWorldImport_ZipMissingFWL_Rejected422 covers WP-06's "Done when: upload
// of a zip missing .fwl is rejected 422" -- the validation happens
// synchronously in EnqueueWorldImport, before any job is created.
func TestWorldImport_ZipMissingFWL_Rejected422(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	z := buildWorldZip(t, "Incomplete", []byte("db-only"), nil)
	files := map[string]io.Reader{"world.zip": z}
	_, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, false, "tester")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed (422), got %v", de.Code)
	}
	if de.HTTPStatus() != 422 {
		t.Errorf("expected HTTP 422, got %d", de.HTTPStatus())
	}
}

func TestWorldImport_RefusesOverwriteWithoutFlag(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Existing", []byte("orig-db"), []byte("orig-fwl"))

	files := map[string]io.Reader{
		"Existing.db":  bytes.NewReader([]byte("new-db")),
		"Existing.fwl": bytes.NewReader([]byte("new-fwl")),
	}
	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, false, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("wait for job: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected job to fail without overwrite=true, got %v", final.Status)
	}

	// The original file must be untouched.
	dir := env.inst.Paths("main").WorldsDir()
	got, err := os.ReadFile(filepath.Join(dir, "Existing.db"))
	if err != nil || string(got) != "orig-db" {
		t.Errorf("expected the existing world file to be left alone, got %q err=%v", got, err)
	}
}

func TestWorldImport_RefusesActiveWorldWhileRunning(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("orig-db"), []byte("orig-fwl"))
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	files := map[string]io.Reader{
		"Dedicated.db":  bytes.NewReader([]byte("new-db")),
		"Dedicated.fwl": bytes.NewReader([]byte("new-fwl")),
	}
	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, true, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("wait for job: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected job to fail while overwriting the active world running, got %v", final.Status)
	}
}

func TestWorldImport_MarksPendingRestartForActiveWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("orig-db"), []byte("orig-fwl"))

	files := map[string]io.Reader{
		"Dedicated.db":  bytes.NewReader([]byte("new-db")),
		"Dedicated.fwl": bytes.NewReader([]byte("new-fwl")),
	}
	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", files, true, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("wait for job: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected job to succeed, got %v (err=%s)", final.Status, final.Error)
	}

	inst, err := env.inst.Get(context.Background(), "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !inst.PendingRestart {
		t.Errorf("expected pending_restart after importing over the active world")
	}
}

func TestDeleteWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Inactive", []byte("db"), []byte("fwl"))

	if err := env.svc.DeleteWorld(context.Background(), "main", "Inactive"); err != nil {
		t.Fatalf("DeleteWorld: %v", err)
	}
	dir := env.inst.Paths("main").WorldsDir()
	if _, err := os.Stat(filepath.Join(dir, "Inactive.db")); !os.IsNotExist(err) {
		t.Errorf("expected Inactive.db to be removed")
	}
}

func TestDeleteWorld_RefusesActiveWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))

	err := env.svc.DeleteWorld(context.Background(), "main", "Dedicated")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeConflict {
		t.Errorf("expected conflict, got %v", de.Code)
	}
}

func TestDeleteWorld_RejectsPathTraversal(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	err := env.svc.DeleteWorld(context.Background(), "main", "../../etc")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestExportWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db-bytes"), []byte("fwl-bytes"))

	var buf bytes.Buffer
	if err := env.svc.ExportWorld(context.Background(), "main", "Dedicated", &buf); err != nil {
		t.Fatalf("ExportWorld: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read exported zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["worlds_local/Dedicated.db"] || !names["worlds_local/Dedicated.fwl"] {
		t.Errorf("expected both save files in the export, got %v", names)
	}
}

func TestExportWorld_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	var buf bytes.Buffer
	err := env.svc.ExportWorld(context.Background(), "main", "NoSuchWorld", &buf)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
	if buf.Len() != 0 {
		t.Errorf("expected nothing written to w on a not_found error, got %d bytes", buf.Len())
	}
}

// ---------------------------------------------------------------- Valheim 1.0 directory layout

func TestListWorlds_DirectoryLayout(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	dir := env.writeWorldDir("main", "Dedicated", worldGen{N: 3, Committed: true})
	// Valheim's own rolling copy is a sibling directory; never a world.
	env.writeWorldDir("main", "Dedicated_backup_auto-20260909-185939", worldGen{N: 1, Committed: true})

	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 1 {
		t.Fatalf("expected exactly the Dedicated world, got %+v", worlds)
	}
	w := worlds[0]
	if w.Name != "Dedicated" || !w.Active || !w.HasDB || !w.HasFWL {
		t.Errorf("unexpected world entry: %+v", w)
	}
	var want int64
	for _, n := range dirFileNames(t, dir) {
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		want += fi.Size()
	}
	if w.SizeBytes != want {
		t.Errorf("size should sum every file in the world directory: got %d want %d", w.SizeBytes, want)
	}
}

func TestListWorlds_DirectoryLayout_UncommittedGenerationIsNotASave(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	// _main.2.* exists but the game never wrote _main.2.ok: a torn save.
	env.writeWorldDir("main", "Dedicated", worldGen{N: 2, Committed: false})

	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 1 {
		t.Fatalf("expected the world to be listed, got %+v", worlds)
	}
	if worlds[0].HasDB || worlds[0].HasFWL {
		t.Errorf("an uncommitted generation must not count as a save: %+v", worlds[0])
	}
}

func TestListWorlds_MixedLayoutIsOneWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	// A pre-1.0 world after migration: the legacy pair stays next to the new directory.
	env.writeWorldFiles("main", "Dedicated", []byte("legacy-db"), []byte("legacy-fwl"))
	env.writeWorldDir("main", "Dedicated", worldGen{N: 1, Committed: true})

	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 1 {
		t.Fatalf("legacy files and the directory are the same world, got %+v", worlds)
	}
	if !worlds[0].HasDB || !worlds[0].HasFWL {
		t.Errorf("unexpected entry: %+v", worlds[0])
	}
}

func TestListWorlds_EmptyDirectoryIgnored(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldDir("main", "Nothing") // directory with no save files at all

	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil {
		t.Fatalf("ListWorlds: %v", err)
	}
	if len(worlds) != 0 {
		t.Errorf("an empty directory is not a world, got %+v", worlds)
	}
}

func TestDeleteWorld_DirectoryLayout_RemovesDirAndRollingCopies(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	dir := env.writeWorldDir("main", "Inactive", worldGen{N: 2, Committed: true})
	rolling := env.writeWorldDir("main", "Inactive_backup_auto-20260909-185939", worldGen{N: 1, Committed: true})
	keep := env.writeWorldDir("main", "Inactive2", worldGen{N: 1, Committed: true}) // prefix-similar, unrelated

	if err := env.svc.DeleteWorld(context.Background(), "main", "Inactive"); err != nil {
		t.Fatalf("DeleteWorld: %v", err)
	}
	for _, p := range []string{dir, rolling} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", p)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("unrelated world Inactive2 must survive: %v", err)
	}
}

func TestExportWorld_DirectoryLayout(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	dir := env.writeWorldDir("main", "Dedicated", worldGen{N: 3, Committed: true})

	var buf bytes.Buffer
	if err := env.svc.ExportWorld(context.Background(), "main", "Dedicated", &buf); err != nil {
		t.Fatalf("ExportWorld: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read exported zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, n := range dirFileNames(t, dir) {
		if !names["worlds_local/Dedicated/"+n] {
			t.Errorf("export is missing %s; got %v", n, names)
		}
	}
}

// buildWorldDirZip builds a zip holding a Valheim 1.0 world directory under
// prefix (e.g. "worlds_local/Imported/" or "Imported/"), one committed
// generation, with content "<name>" per file.
func buildWorldDirZip(t *testing.T, prefix string, gen int) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{
		fmt.Sprintf("_main.%d.fwl2", gen), fmt.Sprintf("_main.%d.db2", gen),
		fmt.Sprintf("_main.%d.chunks", gen), fmt.Sprintf("_main.%d.ok", gen),
		fmt.Sprintf("1e_1e__1_%d.chunk", gen),
	} {
		w, err := zw.Create(prefix + n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestWorldImport_ZipWithDirectoryWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	job, err := env.svc.EnqueueWorldImport(context.Background(), "main",
		map[string]io.Reader{"Imported.zip": buildWorldDirZip(t, "worlds_local/Imported/", 4)}, false, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil || final.Status != domain.JobSucceeded {
		t.Fatalf("import job: status=%v err=%v jobErr=%s", final.Status, err, final.Error)
	}
	dir := filepath.Join(env.inst.Paths("main").WorldsDir(), "Imported")
	got := dirFileNames(t, dir)
	if len(got) != 5 || got[0] != "1e_1e__1_4.chunk" || got[4] != "_main.4.ok" {
		t.Errorf("unexpected imported directory: %v", got)
	}
	b, err := os.ReadFile(filepath.Join(dir, "_main.4.db2")) //nolint:gosec // test-controlled path
	if err != nil || string(b) != "_main.4.db2" {
		t.Errorf("imported db2 content %q err=%v", b, err)
	}
	worlds, err := env.svc.ListWorlds(context.Background(), "main")
	if err != nil || len(worlds) != 1 || !worlds[0].HasDB || !worlds[0].HasFWL {
		t.Errorf("imported world should be listed with a save: %+v err=%v", worlds, err)
	}
}

func TestWorldImport_PairOverwriteReplacesDirectoryWorld(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	dir := env.writeWorldDir("main", "Old", worldGen{N: 9, Committed: true})

	job, err := env.svc.EnqueueWorldImport(context.Background(), "main", map[string]io.Reader{
		"Old.db":  bytes.NewReader([]byte("new-db")),
		"Old.fwl": bytes.NewReader([]byte("new-fwl")),
	}, true, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil || final.Status != domain.JobSucceeded {
		t.Fatalf("import job: status=%v err=%v jobErr=%s", final.Status, err, final.Error)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("overwrite must remove the directory world, or the game would keep loading generation 9")
	}
}

func TestWorldImport_DirectoryWorld_WithoutOverwrite_Conflicts(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldDir("main", "Imported", worldGen{N: 1, Committed: true})

	job, err := env.svc.EnqueueWorldImport(context.Background(), "main",
		map[string]io.Reader{"Imported.zip": buildWorldDirZip(t, "worlds_local/Imported/", 2)}, false, "tester")
	if err != nil {
		t.Fatalf("EnqueueWorldImport: %v", err)
	}
	final, err := env.run.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.JobFailed || !strings.Contains(final.Error, "already exists") {
		t.Errorf("expected a conflict on an existing directory world, got status=%v err=%q", final.Status, final.Error)
	}
}
