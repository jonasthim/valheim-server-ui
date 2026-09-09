package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerWorldRoutes mounts /instances/{instanceId}/worlds*
// (docs/openapi.yaml "worlds" tag, WP-06).
func registerWorldRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleViewer)).Get("/instances/{instanceId}/worlds", handleListWorlds(d))
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/worlds", handleImportWorlds(d))
	r.With(RequireRole(domain.RoleOperator)).Delete("/instances/{instanceId}/worlds/{worldName}", handleDeleteWorld(d))
	r.With(RequireRole(domain.RoleOperator)).Get("/instances/{instanceId}/worlds/{worldName}/download", handleExportWorld(d))
}

func handleListWorlds(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		worlds, err := d.Backups.ListWorlds(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"worlds": worlds})
	}
}

func handleImportWorlds(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil { //nolint:gosec // G120: 64MiB is the documented in-memory cap (spec WP-06); larger parts spill to a temp file rather than memory, and this route requires the operator role
			WriteError(w, domain.Wrap(domain.CodeValidationFailed, "invalid multipart form", err))
			return
		}

		var headers []*multipart.FileHeader
		if r.MultipartForm != nil {
			headers = r.MultipartForm.File["files"]
		}
		if len(headers) == 0 {
			WriteValidation(w, domain.FieldError{Field: "files", Message: "at least one file is required"})
			return
		}

		files := make(map[string]io.Reader, len(headers))
		opened := make([]multipart.File, 0, len(headers))
		defer func() {
			for _, f := range opened {
				_ = f.Close()
			}
		}()
		for _, h := range headers {
			f, err := h.Open()
			if err != nil {
				WriteError(w, domain.Wrap(domain.CodeValidationFailed, "read uploaded file", err))
				return
			}
			opened = append(opened, f)
			files[filepath.Base(h.Filename)] = f
		}
		overwrite := r.FormValue("overwrite") == "true" || r.FormValue("overwrite") == "1"

		job, err := d.Backups.EnqueueWorldImport(r.Context(), id, files, overwrite, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "world.import", id, job.ID, map[string]any{"overwrite": overwrite, "file_count": len(headers)})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

// worldNameParam reads and validates the {worldName} URL parameter.
func worldNameParam(r *http.Request) (string, error) {
	name := chi.URLParam(r, "worldName")
	if name == "" || filepath.Base(name) != name {
		return "", domain.E(domain.CodeValidationFailed, "invalid world name")
	}
	return name, nil
}

func handleDeleteWorld(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		world, err := worldNameParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		if err := d.Backups.DeleteWorld(r.Context(), id, world); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "world.delete", id, world, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleExportWorld(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		world, err := worldNameParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}

		// ExportWorld validates existence before writing anything to w, so an
		// error here can still be turned into a proper status code/body.
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", world+".zip"))
		if err := d.Backups.ExportWorld(r.Context(), id, world, w); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "world.export", id, world, nil)
	}
}
