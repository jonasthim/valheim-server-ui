package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// maxModUploadBytes bounds a manual mod upload (docs/openapi.yaml
// /instances/{instanceId}/mods/upload; ARCHITECTURE.md/WORKPLAN.md WP-08).
const maxModUploadBytes = 256 << 20

// registerModRoutes mounts /instances/{instanceId}/mods* (docs/openapi.yaml
// "mods" tag, WP-08).
func registerModRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.Mods != nil }, "mods service not configured")

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/mods", getModsOverviewHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/mods", installModHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/mods/upload", uploadModHandler(d))

	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/mods/bepinex", installBepInExHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Patch("/instances/{instanceId}/mods/bepinex", setBepInExEnabledHandler(d))

	r.With(RequireRole(domain.RoleOperator), guard).
		Patch("/instances/{instanceId}/mods/{modId}", setModEnabledHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Delete("/instances/{instanceId}/mods/{modId}", uninstallModHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/mods/{modId}/update", updateModHandler(d))

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/mods/configs", listModConfigsHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/mods/configs/{fileName}", getModConfigHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Put("/instances/{instanceId}/mods/configs/{fileName}", updateModConfigHandler(d))
}

// modIDParam reads and parses the {modId} URL parameter. An unparseable id is
// part of the route shape, so it 404s like an unknown id would.
func modIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "modId"), 10, 64)
	if err != nil {
		return 0, domain.NotFound("mod")
	}
	return id, nil
}

func getModsOverviewHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		overview, err := d.Mods.Overview(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, overview)
	}
}

type installModRequest struct {
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

func installModHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req installModRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		if req.Owner == "" || req.Name == "" {
			WriteValidation(w,
				domain.FieldError{Field: "owner", Message: "required"},
				domain.FieldError{Field: "name", Message: "required"},
			)
			return
		}
		job, err := d.Mods.EnqueueInstall(r.Context(), id, req.Owner, req.Name, req.Version, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.install", id, job.ID, map[string]any{"owner": req.Owner, "name": req.Name, "version": req.Version})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func uploadModHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxModUploadBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil { //nolint:gosec // G120: r.Body is already wrapped in http.MaxBytesReader(maxModUploadBytes) above; 32MiB is just the in-memory threshold before parts spill to a temp file
			WriteError(w, domain.Wrap(domain.CodeValidationFailed, "invalid multipart form, or file exceeds 256 MiB", err))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			WriteValidation(w, domain.FieldError{Field: "file", Message: "file is required"})
			return
		}
		defer func() { _ = file.Close() }()

		job, err := d.Mods.EnqueueUpload(r.Context(), id, header.Filename, file, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.upload", id, job.ID, map[string]any{"filename": header.Filename})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func installBepInExHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req struct {
			StopIfRunning bool `json:"stop_if_running"`
		}
		if err := DecodeOptionalJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Mods.EnqueueBepInExInstall(r.Context(), id, req.StopIfRunning, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "bepinex.install", id, job.ID, map[string]any{"stop_if_running": req.StopIfRunning})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func setBepInExEnabledHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		overview, err := d.Mods.SetBepInExEnabled(r.Context(), id, req.Enabled)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "bepinex.enable", id, "", map[string]any{"enabled": req.Enabled})
		WriteJSON(w, http.StatusOK, overview)
	}
}

func setModEnabledHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		modID, err := modIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		mod, err := d.Mods.SetEnabled(r.Context(), id, modID, req.Enabled)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.enable", id, strconv.FormatInt(modID, 10), map[string]any{"enabled": req.Enabled})
		WriteJSON(w, http.StatusOK, map[string]any{"mod": mod})
	}
}

func uninstallModHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		modID, err := modIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Mods.EnqueueUninstall(r.Context(), id, modID, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.uninstall", id, job.ID, map[string]any{"mod_id": modID})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func updateModHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		modID, err := modIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req struct {
			Version string `json:"version"`
		}
		if err := DecodeOptionalJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Mods.EnqueueUpdate(r.Context(), id, modID, req.Version, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.update", id, job.ID, map[string]any{"mod_id": modID, "version": req.Version})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}

func listModConfigsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		files, err := d.Mods.ListConfigs(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		if files == nil {
			files = []domain.ConfigFileInfo{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"files": files})
	}
}

func getModConfigHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		file := chi.URLParam(r, "fileName")
		cf, err := d.Mods.GetConfig(r.Context(), id, file)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, cf)
	}
}

func updateModConfigHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		file := chi.URLParam(r, "fileName")
		var upd domain.ConfigFileUpdate
		if err := DecodeJSON(r, &upd); err != nil {
			WriteError(w, err)
			return
		}
		cf, err := d.Mods.UpdateConfig(r.Context(), id, file, upd)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "mods.config.update", id, file, map[string]any{"raw": upd.Raw != nil, "values": len(upd.Values)})
		WriteJSON(w, http.StatusOK, cf)
	}
}
