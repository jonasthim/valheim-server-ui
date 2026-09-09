package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerBackupRoutes mounts /instances/{instanceId}/backups*
// (docs/openapi.yaml "backups" tag, WP-06).
func registerBackupRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleViewer)).Get("/instances/{instanceId}/backups", handleListBackups(d))
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/backups", handleCreateBackup(d))
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/backups/upload", handleUploadBackup(d))
	r.With(RequireRole(domain.RoleOperator)).Delete("/instances/{instanceId}/backups/{backupId}", handleDeleteBackup(d))
	r.With(RequireRole(domain.RoleOperator)).Get("/instances/{instanceId}/backups/{backupId}/download", handleDownloadBackup(d))
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/backups/{backupId}/restore", handleRestoreBackup(d))
}

// backupIDParam reads and validates the {backupId} URL parameter.
func backupIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "backupId"), 10, 64)
	if err != nil {
		return 0, domain.E(domain.CodeValidationFailed, "invalid backup id")
	}
	return id, nil
}

func handleListBackups(d *Deps) http.HandlerFunc {
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
		backups, err := d.Backups.List(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"backups": backups})
	}
}

type createBackupRequest struct {
	Note string `json:"note"`
}

func handleCreateBackup(d *Deps) http.HandlerFunc {
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
		var req createBackupRequest
		if err := DecodeOptionalJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Backups.EnqueueBackup(r.Context(), id, domain.BackupManual, req.Note, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "backup.create", id, job.ID, map[string]any{"note": req.Note})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func handleUploadBackup(d *Deps) http.HandlerFunc {
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
		file, header, err := r.FormFile("file")
		if err != nil {
			WriteError(w, domain.E(domain.CodeValidationFailed, "file field is required"))
			return
		}
		defer func() { _ = file.Close() }()

		b, err := d.Backups.Upload(r.Context(), id, header.Filename, file)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "backup.upload", id, b.Filename, map[string]any{"kind": string(b.Kind), "size_bytes": b.SizeBytes})
		WriteJSON(w, http.StatusCreated, map[string]any{"backup": b})
	}
}

func handleDeleteBackup(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		backupID, err := backupIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		if err := d.Backups.Delete(r.Context(), id, backupID); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "backup.delete", id, strconv.FormatInt(backupID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleDownloadBackup(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		backupID, err := backupIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		b, rc, err := d.Backups.Open(r.Context(), id, backupID)
		if err != nil {
			WriteError(w, err)
			return
		}
		defer func() { _ = rc.Close() }()

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", b.Filename))
		if b.SizeBytes > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(b.SizeBytes, 10))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}
}

type restoreBackupRequest struct {
	StopIfRunning bool `json:"stop_if_running"`
}

func handleRestoreBackup(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		backupID, err := backupIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Backups == nil {
			WriteError(w, domain.E(domain.CodeInternal, "backup service not configured"))
			return
		}
		var req restoreBackupRequest
		if err := DecodeOptionalJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Backups.EnqueueRestore(r.Context(), id, backupID, req.StopIfRunning, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "backup.restore", id, job.ID, map[string]any{"backup_id": backupID, "stop_if_running": req.StopIfRunning})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}
