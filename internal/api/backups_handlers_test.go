package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/backup"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// backupTestAPI bundles a full router (deps.Auth left nil, so devFakeAdmin
// injects an admin for every request per WP-06's test instructions) backed
// by a real instance.Service, jobs.Runner and backup.Service on an
// in-memory DB, plus the fake supervisor and auditor so tests can inspect
// what happened underneath the HTTP layer.
type backupTestAPI struct {
	handler http.Handler
	inst    *instance.Service
	run     *jobs.Runner
	svc     *backup.Service
	sup     *fakeAPISupervisor
	audit   *fakeAuditor
}

func newBackupTestAPI(t *testing.T) *backupTestAPI {
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
	bus := events.NewBus()
	sup := newFakeAPISupervisor()
	inst := instance.New(sqldb, bus, sup, cfg, log)
	runner := jobs.New(sqldb, bus, cfg.JobsDir(), log)
	svc := backup.New(sqldb, inst, runner, bus, "test-version", log)
	auditor := &fakeAuditor{}

	deps := &Deps{
		Cfg:        cfg,
		Log:        log,
		DB:         sqldb,
		Bus:        bus,
		Supervisor: sup,
		Instances:  inst,
		Backups:    svc,
		Audit:      auditor,
	}
	return &backupTestAPI{handler: NewRouter(deps, nil), inst: inst, run: runner, svc: svc, sup: sup, audit: auditor}
}

// createInstance creates an instance and writes real save files for its
// active world so backups have something to zip.
func (a *backupTestAPI) createInstance(t *testing.T, id, world string, port int) domain.InstancePaths {
	t.Helper()
	cfg := domain.InstanceConfig{Name: "Test " + id, World: world, Password: "secret123", Port: port, Public: true}
	cfg.ApplyDefaults()
	if _, err := a.inst.Create(context.Background(), id, "Test "+id, cfg, false); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	paths := a.inst.Paths(id)
	dir := paths.WorldsDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir worlds dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, world+".db"), []byte("db-bytes"), 0o640); err != nil {
		t.Fatalf("write db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, world+".fwl"), []byte("fwl-bytes"), 0o640); err != nil {
		t.Fatalf("write fwl: %v", err)
	}
	return paths
}

func (a *backupTestAPI) request(t *testing.T, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set(CSRFHeader, CSRFHeaderValue)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func (a *backupTestAPI) postJSON(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	return a.request(t, http.MethodPost, path, r, "application/json")
}

// waitForJob decodes {"job": {...}} from rec and blocks until that job
// reaches a terminal state.
func (a *backupTestAPI) waitForJob(t *testing.T, rec *httptest.ResponseRecorder) domain.Job {
	t.Helper()
	var body struct {
		Job domain.Job `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode job response: %v (body=%s)", err, rec.Body.String())
	}
	final, err := a.run.WaitFor(context.Background(), body.Job.ID)
	if err != nil {
		t.Fatalf("wait for job %s: %v", body.Job.ID, err)
	}
	return *final
}

func buildMultipartZip(t *testing.T, field, filename string, entries map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(zipBuf.Bytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, mw.FormDataContentType()
}

func TestBackupsHandlers_CreateListDownloadDelete(t *testing.T) {
	api := newBackupTestAPI(t)
	paths := api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.postJSON(t, "/api/v1/instances/main/backups", map[string]any{"note": "before changes"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	job := api.waitForJob(t, rec)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected backup job to succeed, got %v (err=%s)", job.Status, job.Error)
	}

	call, ok := api.audit.last()
	if !ok || call.action != "backup.create" {
		t.Fatalf("expected a backup.create audit call, got %+v (ok=%v)", call, ok)
	}

	rec = api.request(t, http.MethodGet, "/api/v1/instances/main/backups", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listBody struct {
		Backups []domain.Backup `json:"backups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(listBody.Backups))
	}
	b := listBody.Backups[0]
	if b.Note != "before changes" || b.Kind != domain.BackupManual {
		t.Errorf("unexpected backup: %+v", b)
	}

	rec = api.request(t, http.MethodGet, "/api/v1/instances/main/backups/"+itoa(b.ID)+"/download", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected application/zip content type, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Errorf("expected a Content-Disposition header")
	}
	onDisk, err := os.ReadFile(filepath.Join(paths.Backups, b.Filename))
	if err != nil {
		t.Fatalf("read backup file on disk: %v", err)
	}
	if !bytes.Equal(rec.Body.Bytes(), onDisk) {
		t.Errorf("downloaded body does not match the file on disk")
	}

	rec = api.request(t, http.MethodDelete, "/api/v1/instances/main/backups/"+itoa(b.ID), nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	call, ok = api.audit.last()
	if !ok || call.action != "backup.delete" {
		t.Fatalf("expected a backup.delete audit call, got %+v (ok=%v)", call, ok)
	}

	rec = api.request(t, http.MethodGet, "/api/v1/instances/main/backups", nil, "")
	json.Unmarshal(rec.Body.Bytes(), &listBody) //nolint:errcheck
	if len(listBody.Backups) != 0 {
		t.Errorf("expected the backup list to be empty after delete, got %+v", listBody.Backups)
	}
}

func TestBackupsHandlers_RestoreRoundTrip(t *testing.T) {
	api := newBackupTestAPI(t)
	paths := api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.postJSON(t, "/api/v1/instances/main/backups", nil)
	job := api.waitForJob(t, rec)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected backup job to succeed: %+v", job)
	}
	list, err := api.svc.List(context.Background(), "main")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected exactly one backup, got %v err=%v", list, err)
	}
	backupID := list[0].ID

	// Corrupt the live save files, then restore.
	dbPath := filepath.Join(paths.WorldsDir(), "Dedicated.db")
	if err := os.WriteFile(dbPath, []byte("corrupted"), 0o640); err != nil {
		t.Fatalf("corrupt db: %v", err)
	}

	rec = api.postJSON(t, "/api/v1/instances/main/backups/"+itoa(backupID)+"/restore", map[string]any{"stop_if_running": false})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("restore: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	restoreJob := api.waitForJob(t, rec)
	if restoreJob.Status != domain.JobSucceeded {
		t.Fatalf("expected restore job to succeed, got %v (err=%s)", restoreJob.Status, restoreJob.Error)
	}

	call, ok := api.audit.last()
	if !ok || call.action != "backup.restore" {
		t.Fatalf("expected a backup.restore audit call, got %+v (ok=%v)", call, ok)
	}

	got, err := os.ReadFile(dbPath)
	if err != nil || string(got) != "db-bytes" {
		t.Errorf("expected restored db bytes, got %q err=%v", got, err)
	}
}

func TestBackupsHandlers_RestoreUnknownBackup404(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.postJSON(t, "/api/v1/instances/main/backups/999/restore", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupsHandlers_UploadValidZip(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	body, contentType := buildMultipartZip(t, "file", "mine.zip", map[string]string{
		"manifest.json":             `{"instance_id":"main","world":"Dedicated","kind":"manual","app_version":"x"}`,
		"worlds_local/Dedicated.db": "uploaded-db",
	})
	rec := api.request(t, http.MethodPost, "/api/v1/instances/main/backups/upload", body, contentType)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var respBody struct {
		Backup domain.Backup `json:"backup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if respBody.Backup.Kind != domain.BackupUploaded {
		t.Errorf("expected kind=uploaded, got %+v", respBody.Backup)
	}
	call, ok := api.audit.last()
	if !ok || call.action != "backup.upload" {
		t.Fatalf("expected a backup.upload audit call, got %+v (ok=%v)", call, ok)
	}
}

func TestBackupsHandlers_UploadInvalidZipRejected422(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	body, contentType := buildMultipartZip(t, "file", "mine.zip", map[string]string{"readme.txt": "hello"})
	rec := api.request(t, http.MethodPost, "/api/v1/instances/main/backups/upload", body, contentType)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
