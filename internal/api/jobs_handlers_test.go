package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// jobsFakeAuth injects a fixed user, bypassing real authentication, so tests can
// exercise role gating without WP-01.
type jobsFakeAuth struct{ user *domain.User }

func (f jobsFakeAuth) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), f.user)))
	})
}

// The remaining Authenticator methods are unused by these tests; they exist
// only so jobsFakeAuth satisfies the full interface.
func (f jobsFakeAuth) Setup(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, displayName, email string) (*domain.User, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f jobsFakeAuth) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password string) (*domain.User, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f jobsFakeAuth) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {}

func (f jobsFakeAuth) OIDCLogin(w http.ResponseWriter, r *http.Request) {}

func (f jobsFakeAuth) OIDCCallback(w http.ResponseWriter, r *http.Request) {}

// jobsFakeAuditor records calls for assertions.
type jobsFakeAuditor struct {
	calls []jobsAuditCall
}

type jobsAuditCall struct {
	action, instanceID, target string
}

func (f *jobsFakeAuditor) Record(r *http.Request, action, instanceID, target string, details map[string]any) {
	f.calls = append(f.calls, jobsAuditCall{action, instanceID, target})
}

// List is unused by these tests; it exists only so jobsFakeAuditor satisfies
// the full Auditor interface.
func (f *jobsFakeAuditor) List(ctx context.Context, instanceID, username string, limit int, before int64) ([]domain.AuditEntry, error) {
	return nil, nil
}

// jobsFakeJobService implements JobService in-memory for handler tests.
type jobsFakeJobService struct {
	jobs      map[string]domain.Job
	cancelErr error
}

func newJobsFakeJobService() *jobsFakeJobService {
	return &jobsFakeJobService{jobs: map[string]domain.Job{}}
}

func (f *jobsFakeJobService) List(ctx context.Context, instanceID string, status domain.JobStatus, limit int) ([]domain.Job, error) {
	var out []domain.Job
	for _, j := range f.jobs {
		if instanceID != "" && j.InstanceID != instanceID {
			continue
		}
		if status != "" && j.Status != status {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func (f *jobsFakeJobService) Get(ctx context.Context, id string) (*domain.Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return nil, domain.NotFound("job")
	}
	return &j, nil
}

func (f *jobsFakeJobService) Log(ctx context.Context, id string) ([]string, error) {
	if _, ok := f.jobs[id]; !ok {
		return nil, domain.NotFound("job")
	}
	return []string{"line one", "line two"}, nil
}

func (f *jobsFakeJobService) Cancel(ctx context.Context, id string) (*domain.Job, error) {
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	j, ok := f.jobs[id]
	if !ok {
		return nil, domain.NotFound("job")
	}
	j.Status = domain.JobCancelled
	f.jobs[id] = j
	return &j, nil
}

func newJobsTestDeps(t *testing.T) (*Deps, *jobsFakeJobService, *jobsFakeAuditor) {
	t.Helper()
	jobs := newJobsFakeJobService()
	audit := &jobsFakeAuditor{}
	d := &Deps{
		Cfg:   config.Config{},
		Log:   slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Jobs:  jobs,
		Audit: audit,
		Auth:  jobsFakeAuth{user: &domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}},
	}
	return d, jobs, audit
}

func jobsDoJSON(t *testing.T, router http.Handler, method, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, body
}

func TestJobsHandlers_ListGetLog(t *testing.T) {
	d, jobs, _ := newJobsTestDeps(t)
	now := time.Now().UTC()
	jobs.jobs["j1"] = domain.Job{ID: "j1", Type: domain.JobInstall, InstanceID: "inst-a", Status: domain.JobRunning, CreatedAt: now}
	jobs.jobs["j2"] = domain.Job{ID: "j2", Type: domain.JobBackup, InstanceID: "inst-b", Status: domain.JobSucceeded, CreatedAt: now}
	router := NewRouter(d, nil)

	rec, body := jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	list, ok := body["jobs"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("expected 2 jobs, got %v", body)
	}

	rec, body = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs?instance=inst-a")
	if rec.Code != http.StatusOK {
		t.Fatalf("list filtered: expected 200, got %d", rec.Code)
	}
	list = body["jobs"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 job for inst-a, got %d", len(list))
	}

	rec, body = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs?limit=0")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for limit=0, got %d: %v", rec.Code, body)
	}

	rec, body = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs/j1")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
	job := body["job"].(map[string]any)
	if job["id"] != "j1" {
		t.Fatalf("expected job j1, got %v", job)
	}

	rec, _ = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown job, got %d", rec.Code)
	}

	rec, body = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs/j1/log")
	if rec.Code != http.StatusOK {
		t.Fatalf("log: expected 200, got %d", rec.Code)
	}
	lines := body["lines"].([]any)
	if len(lines) != 2 || lines[0] != "line one" {
		t.Fatalf("unexpected log lines: %v", lines)
	}
}

func TestJobsHandlers_Cancel(t *testing.T) {
	d, jobs, audit := newJobsTestDeps(t)
	jobs.jobs["j1"] = domain.Job{ID: "j1", Type: domain.JobInstall, InstanceID: "inst-a", Status: domain.JobRunning}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/j1/cancel", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	job := body["job"].(map[string]any)
	if job["status"] != "cancelled" {
		t.Fatalf("expected cancelled status, got %v", job["status"])
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "job.cancel" || audit.calls[0].instanceID != "inst-a" {
		t.Fatalf("expected one job.cancel audit record for inst-a, got %+v", audit.calls)
	}
}

func TestJobsHandlers_CancelRequiresOperator(t *testing.T) {
	d, jobs, _ := newJobsTestDeps(t)
	jobs.jobs["j1"] = domain.Job{ID: "j1", Type: domain.JobInstall, InstanceID: "inst-a", Status: domain.JobRunning}
	d.Auth = jobsFakeAuth{user: &domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer}}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/j1/cancel", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a viewer cancelling a job, got %d: %s", rec.Code, rec.Body.String())
	}

	// A viewer can still list/get/log (x-role: viewer).
	rec, _ = jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected viewer to list jobs, got %d", rec.Code)
	}
}

func TestJobsHandlers_RequiresAuthentication(t *testing.T) {
	d, jobs, _ := newJobsTestDeps(t)
	jobs.jobs["j1"] = domain.Job{ID: "j1"}
	d.Auth = jobsFakeAuth{user: nil}
	router := NewRouter(d, nil)

	rec, _ := jobsDoJSON(t, router, http.MethodGet, "/api/v1/jobs")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no authenticated user, got %d", rec.Code)
	}
}
