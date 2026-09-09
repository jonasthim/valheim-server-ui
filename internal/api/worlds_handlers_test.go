package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestWorldsHandlers_List(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.request(t, http.MethodGet, "/api/v1/instances/main/worlds", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Worlds []domain.World `json:"worlds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Worlds) != 1 || body.Worlds[0].Name != "Dedicated" || !body.Worlds[0].Active {
		t.Fatalf("unexpected worlds list: %+v", body.Worlds)
	}
}

// buildWorldPairMultipart builds a multipart body with two "files" parts:
// <world>.db and <world>.fwl.
func buildWorldPairMultipart(t *testing.T, world string, overwrite bool) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ name, content string }{
		{world + ".db", "new-db"},
		{world + ".fwl", "new-fwl"},
	} {
		w, err := mw.CreateFormFile("files", part.name)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := w.Write([]byte(part.content)); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if err := mw.WriteField("overwrite", boolString(overwrite)); err != nil {
		t.Fatalf("write overwrite field: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, mw.FormDataContentType()
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestWorldsHandlers_ImportPair(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	body, contentType := buildWorldPairMultipart(t, "Imported", false)
	rec := api.request(t, http.MethodPost, "/api/v1/instances/main/worlds", body, contentType)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	job := api.waitForJob(t, rec)
	if job.Status != domain.JobSucceeded {
		t.Fatalf("expected import job to succeed, got %v (err=%s)", job.Status, job.Error)
	}

	call, ok := api.audit.last()
	if !ok || call.action != "world.import" {
		t.Fatalf("expected a world.import audit call, got %+v (ok=%v)", call, ok)
	}

	dir := api.inst.Paths("main").WorldsDir()
	got, err := os.ReadFile(filepath.Join(dir, "Imported.db"))
	if err != nil || string(got) != "new-db" {
		t.Errorf("unexpected Imported.db content: %q err=%v", got, err)
	}
}

func TestWorldsHandlers_ImportZipMissingFWLRejected422(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("Incomplete.db")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("db-only")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("files", "world.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(zipBuf.Bytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	rec := api.request(t, http.MethodPost, "/api/v1/instances/main/worlds", &body, mw.FormDataContentType())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWorldsHandlers_DeleteActiveWorldConflict(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.request(t, http.MethodDelete, "/api/v1/instances/main/worlds/Dedicated", nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWorldsHandlers_DeleteInactiveWorld(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)
	dir := api.inst.Paths("main").WorldsDir()
	if err := os.WriteFile(filepath.Join(dir, "Old.db"), []byte("x"), 0o640); err != nil {
		t.Fatalf("write Old.db: %v", err)
	}

	rec := api.request(t, http.MethodDelete, "/api/v1/instances/main/worlds/Old", nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	call, ok := api.audit.last()
	if !ok || call.action != "world.delete" {
		t.Fatalf("expected a world.delete audit call, got %+v (ok=%v)", call, ok)
	}
}

func TestWorldsHandlers_Export(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.request(t, http.MethodGet, "/api/v1/instances/main/worlds/Dedicated/download", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected application/zip, got %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("read exported zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["worlds_local/Dedicated.db"] || !names["worlds_local/Dedicated.fwl"] {
		t.Errorf("expected both save files in export, got %v", names)
	}
	call, ok := api.audit.last()
	if !ok || call.action != "world.export" {
		t.Fatalf("expected a world.export audit call, got %+v (ok=%v)", call, ok)
	}
}

func TestWorldsHandlers_ExportNotFound(t *testing.T) {
	api := newBackupTestAPI(t)
	api.createInstance(t, "main", "Dedicated", 2456)

	rec := api.request(t, http.MethodGet, "/api/v1/instances/main/worlds/NoSuchWorld/download", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
