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

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// metricsFakeInstanceService implements InstanceService for handler tests;
// only Get (the existence check the metrics handler performs) matters here.
type metricsFakeInstanceService struct {
	instances map[string]*domain.Instance
}

func (f *metricsFakeInstanceService) List(context.Context) ([]domain.Instance, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Get(_ context.Context, id string) (*domain.Instance, error) {
	inst, ok := f.instances[id]
	if !ok {
		return nil, domain.NotFound("instance")
	}
	return inst, nil
}
func (f *metricsFakeInstanceService) Create(context.Context, string, string, domain.InstanceConfig, bool) (*domain.Instance, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Update(context.Context, string, *string, *domain.InstanceConfig, *bool) (*domain.Instance, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Delete(context.Context, string, bool) error {
	return domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Start(context.Context, string) (*domain.InstanceStatus, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Stop(context.Context, string) (*domain.InstanceStatus, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Restart(context.Context, string) (*domain.InstanceStatus, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) Status(context.Context, string) (*domain.InstanceStatus, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) TailLog(context.Context, string, int) ([]string, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) OpenLog(context.Context, string) (io.ReadCloser, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *metricsFakeInstanceService) InstanceEvents(context.Context, string, int, *time.Time) ([]domain.InstanceEvent, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

// metricsFakeHistoryService implements MetricsHistoryService for handler tests.
type metricsFakeHistoryService struct {
	series         domain.MetricSeries
	err            error
	lastInstanceID *string
	calledWithNil  bool
	lastSince      time.Time
}

func (f *metricsFakeHistoryService) Series(_ context.Context, instanceID *string, since time.Time) (domain.MetricSeries, error) {
	f.lastInstanceID = instanceID
	f.calledWithNil = instanceID == nil
	f.lastSince = since
	if f.err != nil {
		return domain.MetricSeries{}, f.err
	}
	return f.series, nil
}

// newMetricsTestDeps wires a Deps with fake Instances/MetricsHistory services
// and reuses systemFakeAuth (system_handlers_test.go, same package) to inject
// a fixed-role user, mirroring newNotifyTestDeps in notify_handlers_test.go.
func newMetricsTestDeps(t *testing.T, role domain.Role) (*Deps, *metricsFakeInstanceService, *metricsFakeHistoryService) {
	t.Helper()
	instances := &metricsFakeInstanceService{instances: map[string]*domain.Instance{
		"main": {ID: "main", Name: "Main"},
	}}
	history := &metricsFakeHistoryService{}
	d := &Deps{
		Log:            slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Instances:      instances,
		MetricsHistory: history,
		Auth:           systemFakeAuth{user: &domain.User{ID: 1, Username: "tester", Role: role}},
	}
	return d, instances, history
}

func fullMetricSeries() domain.MetricSeries {
	return domain.MetricSeries{
		TS:       []string{"2026-09-17T09:00:00Z", "2026-09-17T10:00:00Z"},
		CPU:      []float64{10, 20},
		Mem:      []int64{100, 200},
		Players:  []int{1, 2},
		DiskFree: []int64{5000, 4900},
	}
}

func TestInstanceMetricsHandler_ReturnsAlignedArrays(t *testing.T) {
	d, _, history := newMetricsTestDeps(t, domain.RoleViewer)
	history.series = fullMetricSeries()
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/metrics?range=24h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got domain.MetricSeries
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	n := len(got.TS)
	if n == 0 || len(got.CPU) != n || len(got.Mem) != n || len(got.Players) != n || len(got.DiskFree) != n {
		t.Fatalf("expected equal-length arrays, got %+v", got)
	}
	if history.lastInstanceID == nil || *history.lastInstanceID != "main" {
		t.Fatalf("expected the service to be called with instance id main, got %v", history.lastInstanceID)
	}
}

func TestInstanceMetricsHandler_InvalidRange(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/metrics?range=1y", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for an unsupported range, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceMetricsHandler_MissingRange(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when range is missing, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceMetricsHandler_UnknownInstanceIs404(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/nope/metrics?range=24h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown instance, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemMetricsHandler_ReturnsAlignedArrays(t *testing.T) {
	d, _, history := newMetricsTestDeps(t, domain.RoleViewer)
	history.series = fullMetricSeries()
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics?range=1h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got domain.MetricSeries
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	n := len(got.TS)
	if n == 0 || len(got.CPU) != n || len(got.Mem) != n || len(got.Players) != n || len(got.DiskFree) != n {
		t.Fatalf("expected equal-length arrays, got %+v", got)
	}
	if !history.calledWithNil {
		t.Fatalf("expected the host series (nil instance id), got %v", history.lastInstanceID)
	}
}

func TestSystemMetricsHandler_InvalidRange(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics?range=1y", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for an unsupported range, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMetricsRoutes_ServiceNotConfigured(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	d.MetricsHistory = nil
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics?range=1h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the metrics history service is not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceMetricsRoutes_ServiceNotConfigured(t *testing.T) {
	d, _, _ := newMetricsTestDeps(t, domain.RoleViewer)
	d.MetricsHistory = nil
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/metrics?range=1h", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the metrics history service is not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}
