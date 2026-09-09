package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerJobRoutes mounts GET /jobs, GET /jobs/{jobId}, GET
// /jobs/{jobId}/log and POST /jobs/{jobId}/cancel (docs/openapi.yaml → /jobs*).
func registerJobRoutes(r chi.Router, d *Deps) {
	r.Route("/jobs", func(r chi.Router) {
		r.Use(RequireRole(domain.RoleViewer))
		r.Get("/", listJobsHandler(d))
		r.Route("/{jobId}", func(r chi.Router) {
			r.Get("/", getJobHandler(d))
			r.Get("/log", getJobLogHandler(d))
			r.With(RequireRole(domain.RoleOperator)).Post("/cancel", cancelJobHandler(d))
		})
	})
}

const (
	defaultJobsLimit = 50
	maxJobsLimit     = 200
)

func listJobsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Jobs == nil {
			WriteError(w, domain.E(domain.CodeInternal, "jobs service not configured"))
			return
		}
		q := r.URL.Query()
		instance := q.Get("instance")
		status := domain.JobStatus(q.Get("status"))

		limit := defaultJobsLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				WriteValidation(w, domain.FieldError{Field: "limit", Message: "must be a positive integer"})
				return
			}
			limit = n
		}
		if limit > maxJobsLimit {
			limit = maxJobsLimit
		}

		jobs, err := d.Jobs.List(r.Context(), instance, status, limit)
		if err != nil {
			WriteError(w, err)
			return
		}
		if jobs == nil {
			jobs = []domain.Job{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	}
}

func getJobHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Jobs == nil {
			WriteError(w, domain.E(domain.CodeInternal, "jobs service not configured"))
			return
		}
		job, err := d.Jobs.Get(r.Context(), chi.URLParam(r, "jobId"))
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"job": job})
	}
}

func getJobLogHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Jobs == nil {
			WriteError(w, domain.E(domain.CodeInternal, "jobs service not configured"))
			return
		}
		lines, err := d.Jobs.Log(r.Context(), chi.URLParam(r, "jobId"))
		if err != nil {
			WriteError(w, err)
			return
		}
		if lines == nil {
			lines = []string{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"lines": lines})
	}
}

func cancelJobHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Jobs == nil {
			WriteError(w, domain.E(domain.CodeInternal, "jobs service not configured"))
			return
		}
		id := chi.URLParam(r, "jobId")
		job, err := d.Jobs.Cancel(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "job.cancel", job.InstanceID, id, nil)
		WriteJSON(w, http.StatusOK, map[string]any{"job": job})
	}
}
