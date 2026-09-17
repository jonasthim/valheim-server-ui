package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerMetricsRoutes mounts GET /instances/{instanceId}/metrics and GET
// /system/metrics (docs/openapi.yaml → MetricSeries, F-1.3).
func registerMetricsRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.MetricsHistory != nil }, "metrics history service not configured")
	r.With(RequireRole(domain.RoleViewer), guard).Get("/instances/{instanceId}/metrics", instanceMetricsHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).Get("/system/metrics", systemMetricsHandler(d))
}

// metricRangeWindows maps the documented range query values to a lookback
// window ending now.
var metricRangeWindows = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// parseMetricRange resolves ?range= to a lookback window; the bool is false
// when the value is missing or not one of the documented enum values.
func parseMetricRange(r *http.Request) (time.Duration, bool) {
	d, ok := metricRangeWindows[r.URL.Query().Get("range")]
	return d, ok
}

func writeInvalidMetricRange(w http.ResponseWriter) {
	WriteValidation(w, domain.FieldError{Field: "range", Message: "must be one of 1h, 24h, 7d, 30d"})
}

// instanceMetricsHandler is GET /instances/{instanceId}/metrics?range=.
func instanceMetricsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		// Resolve the instance the same way instanceEvents does, so an
		// unknown id is a 404 rather than a (misleadingly valid) empty series.
		if _, err := d.Instances.Get(r.Context(), id); err != nil {
			WriteError(w, err)
			return
		}
		window, ok := parseMetricRange(r)
		if !ok {
			writeInvalidMetricRange(w)
			return
		}
		series, err := d.MetricsHistory.Series(r.Context(), &id, time.Now().Add(-window))
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, series)
	}
}

// systemMetricsHandler is GET /system/metrics?range=.
func systemMetricsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		window, ok := parseMetricRange(r)
		if !ok {
			writeInvalidMetricRange(w)
			return
		}
		series, err := d.MetricsHistory.Series(r.Context(), nil, time.Now().Add(-window))
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, series)
	}
}
