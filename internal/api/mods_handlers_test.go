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
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/mods"
)

// --- a minimal fake Thunderstore index server, independent of the
// internal/mods package's own (unexported) test fixtures. Field tags mirror
// the real Thunderstore v1 index schema (see internal/mods/thunderstore.go). ---

type modsTestVersion struct {
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	VersionNumber string    `json:"version_number"`
	Dependencies  []string  `json:"dependencies"`
	DownloadURL   string    `json:"download_url"`
	Downloads     int64     `json:"downloads"`
	DateCreated   time.Time `json:"date_created"`
	WebsiteURL    string    `json:"website_url"`
}

type modsTestPackage struct {
	Name        string            `json:"name"`
	FullName    string            `json:"full_name"`
	Owner       string            `json:"owner"`
	DateCreated time.Time         `json:"date_created"`
	DateUpdated time.Time         `json:"date_updated"`
	RatingScore int               `json:"rating_score"`
	Categories  []string          `json:"categories"`
	Versions    []modsTestVersion `json:"versions"`
}

// apiRewriteTransport redirects every request to base's scheme+host so
// production code hard-coding the real thunderstore.io URLs can be exercised
// against an httptest.Server.
type apiRewriteTransport struct{ base *url.URL }

func (t *apiRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	req.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

func buildTestModZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"manifest.json":       `{"name":"CoreLib","version_number":"1.0.0","dependencies":[]}`,
		"plugins/CoreLib.dll": "corelib-dll",
		"config/corelib.cfg":  "[General]\nEnabled = true\n",
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// modsTestAPI wires a full router with real Instances/Jobs/Mods/Thunderstore
// services (per WORKPLAN.md WP-08's test plan: "handler tests via
// api.NewRouter with deps.Auth nil and a real instance.Service on
// db.OpenMemory"), backed by a fake Thunderstore index server.
type modsTestAPI struct {
	handler http.Handler
	inst    *instance.Service
	runner  *jobs.Runner
}

func newModsTestAPI(t *testing.T) *modsTestAPI {
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
	instSvc := instance.New(sqldb, bus, sup, cfg, log)
	runner := jobs.New(sqldb, bus, cfg.JobsDir(), log)

	now := time.Now().UTC()
	pkg := modsTestPackage{
		Name: "CoreLib", FullName: "Alice-CoreLib", Owner: "Alice",
		DateCreated: now, DateUpdated: now, RatingScore: 10, Categories: []string{"Libraries"},
		Versions: []modsTestVersion{{
			Name: "CoreLib", FullName: "Alice-CoreLib-1.0.0", VersionNumber: "1.0.0",
			DateCreated: now, DownloadURL: "https://thunderstore.io/package/download/Alice/CoreLib/1.0.0/",
		}},
	}
	zipBytes := buildTestModZip(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/c/valheim/api/v1/package/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]modsTestPackage{pkg})
	})
	mux.HandleFunc("/package/download/Alice/CoreLib/1.0.0/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Transport: &apiRewriteTransport{base: srvURL}}

	ts := mods.NewThunderstore(httpClient, cfg.CacheDir(), func() time.Duration { return time.Hour }, "test-agent", log)
	if err := ts.Refresh(ctx); err != nil {
		t.Fatalf("refresh thunderstore index: %v", err)
	}

	modSvc := mods.NewService(sqldb, ts, instSvc, runner, cfg.CacheDir(), log)
	tsSvc := mods.NewThunderstoreService(ts, runner)

	deps := &Deps{
		Cfg: cfg, Log: log, DB: sqldb, Bus: bus, Supervisor: sup,
		Auth:         nil, // devFakeAdmin: every request acts as an admin (WORKPLAN.md WP-08 test plan)
		Instances:    instSvc,
		Jobs:         runner,
		Mods:         modSvc,
		Thunderstore: tsSvc,
	}
	return &modsTestAPI{handler: NewRouter(deps, nil), inst: instSvc, runner: runner}
}

func (a *modsTestAPI) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set(CSRFHeader, CSRFHeaderValue)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func (a *modsTestAPI) waitJob(t *testing.T, jobID string) *domain.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job, err := a.runner.WaitFor(ctx, jobID)
	if err != nil {
		t.Fatalf("WaitFor(%s): %v", jobID, err)
	}
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job %s ended %s: %s", jobID, job.Status, job.Error)
	}
	return job
}

func createModsTestInstance(t *testing.T, a *modsTestAPI, id string) {
	t.Helper()
	cfg := domain.InstanceConfig{Name: "Main", World: "Dedicated", Password: "secret123", Port: 2456, Public: true}
	cfg.ApplyDefaults()
	if _, err := a.inst.Create(context.Background(), id, "Main", cfg, false); err != nil {
		t.Fatalf("create instance: %v", err)
	}
}

func TestModsAPI_OverviewEmpty(t *testing.T) {
	a := newModsTestAPI(t)
	createModsTestInstance(t, a, "main")

	rec := a.do(t, http.MethodGet, "/api/v1/instances/main/mods", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var overview domain.ModsOverview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.BepInEx.Installed {
		t.Error("expected bepinex not installed on a fresh instance")
	}
	if len(overview.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", overview.Mods)
	}
}

func TestModsAPI_InstallUnknownPackage404(t *testing.T) {
	a := newModsTestAPI(t)
	createModsTestInstance(t, a, "main")

	rec := a.do(t, http.MethodPost, "/api/v1/instances/main/mods", map[string]any{"owner": "Nobody", "name": "Nothing"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestModsAPI_InstallListEnableUninstall(t *testing.T) {
	a := newModsTestAPI(t)
	createModsTestInstance(t, a, "main")

	rec := a.do(t, http.MethodPost, "/api/v1/instances/main/mods", map[string]any{"owner": "Alice", "name": "CoreLib"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobResp struct {
		Job domain.Job `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jobResp); err != nil {
		t.Fatal(err)
	}
	a.waitJob(t, jobResp.Job.ID)

	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/mods", nil)
	var overview domain.ModsOverview
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if len(overview.Mods) != 1 {
		t.Fatalf("expected 1 installed mod, got %+v", overview.Mods)
	}
	modID := overview.Mods[0].ID

	// Config editor: list, get, update.
	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/mods/configs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list configs: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var filesResp struct {
		Files []domain.ConfigFileInfo `json:"files"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &filesResp)
	if len(filesResp.Files) != 1 || filesResp.Files[0].Name != "corelib.cfg" {
		t.Fatalf("unexpected config files: %+v", filesResp.Files)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/mods/configs/corelib.cfg", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = a.do(t, http.MethodPut, "/api/v1/instances/main/mods/configs/corelib.cfg", domain.ConfigFileUpdate{
		Values: []domain.ConfigValueUpdate{{Section: "General", Key: "Enabled", Value: "false"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update config: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = a.do(t, http.MethodPut, "/api/v1/instances/main/mods/configs/corelib.cfg", domain.ConfigFileUpdate{
		Values: []domain.ConfigValueUpdate{{Section: "General", Key: "NoSuchKey", Value: "x"}},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update config with unknown key: expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// Disable, then delete with a bad id (404) and the real one (202 job).
	rec = a.do(t, http.MethodPatch, "/api/v1/instances/main/mods/"+strconv.FormatInt(modID, 10), map[string]any{"enabled": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("disable mod: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var modResp struct {
		Mod domain.Mod `json:"mod"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &modResp)
	if modResp.Mod.Enabled {
		t.Error("expected mod reported disabled")
	}

	rec = a.do(t, http.MethodDelete, "/api/v1/instances/main/mods/not-a-number", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete with bad id: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = a.do(t, http.MethodDelete, "/api/v1/instances/main/mods/"+strconv.FormatInt(modID, 10), nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delete: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &jobResp)
	a.waitJob(t, jobResp.Job.ID)

	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/mods", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if len(overview.Mods) != 0 {
		t.Errorf("expected mod removed, got %+v", overview.Mods)
	}
}

func TestModsAPI_Upload(t *testing.T) {
	a := newModsTestAPI(t)
	createModsTestInstance(t, a, "main")

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "MyMod.dll")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("dll-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/main/mods/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobResp struct {
		Job domain.Job `json:"job"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &jobResp)
	a.waitJob(t, jobResp.Job.ID)

	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/mods", nil)
	var overview domain.ModsOverview
	_ = json.Unmarshal(rec.Body.Bytes(), &overview)
	if len(overview.Mods) != 1 || overview.Mods[0].Source != domain.ModSourceManual {
		t.Fatalf("expected 1 manual mod, got %+v", overview.Mods)
	}
}

func TestModsAPI_BepInExRequiresInstalledToEnable(t *testing.T) {
	a := newModsTestAPI(t)
	createModsTestInstance(t, a, "main")

	rec := a.do(t, http.MethodPatch, "/api/v1/instances/main/mods/bepinex", map[string]any{"enabled": true})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 bepinex_missing, got %d: %s", rec.Code, rec.Body.String())
	}
}
