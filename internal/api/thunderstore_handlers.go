package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

const (
	defaultPackagePageSize = 50
	maxPackagePageSize     = 100
)

// registerThunderstoreRoutes mounts /thunderstore/* (docs/openapi.yaml
// "thunderstore" tag, WP-08).
func registerThunderstoreRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.Thunderstore != nil }, "thunderstore service not configured")

	r.With(RequireRole(domain.RoleViewer), guard).Get("/thunderstore/packages", searchPackagesHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).Get("/thunderstore/packages/{owner}/{name}", getPackageHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).Get("/thunderstore/categories", listCategoriesHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).Post("/thunderstore/refresh", refreshThunderstoreHandler(d))
}

func searchPackagesHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		page := 1
		if v := q.Get("page"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				WriteValidation(w, domain.FieldError{Field: "page", Message: "must be a positive integer"})
				return
			}
			page = n
		}
		pageSize := defaultPackagePageSize
		if v := q.Get("page_size"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				WriteValidation(w, domain.FieldError{Field: "page_size", Message: "must be a positive integer"})
				return
			}
			pageSize = n
		}
		if pageSize > maxPackagePageSize {
			pageSize = maxPackagePageSize
		}
		includeDeprecated, _ := strconv.ParseBool(q.Get("include_deprecated"))

		search := domain.PackageSearch{
			Query:             q.Get("q"),
			Category:          q.Get("category"),
			Sort:              q.Get("sort"),
			IncludeDeprecated: includeDeprecated,
			Page:              page,
			PageSize:          pageSize,
		}
		result, err := d.Thunderstore.Search(r.Context(), search)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, result)
	}
}

func getPackageHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner := chi.URLParam(r, "owner")
		name := chi.URLParam(r, "name")
		pkg, err := d.Thunderstore.Package(r.Context(), owner, name)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, pkg)
	}
}

func listCategoriesHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := d.Thunderstore.Categories(r.Context())
		if err != nil {
			WriteError(w, err)
			return
		}
		if categories == nil {
			categories = []string{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"categories": categories})
	}
}

func refreshThunderstoreHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := d.Thunderstore.EnqueueRefresh(r.Context(), RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "thunderstore.refresh", "", job.ID, nil)
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}
