package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerInstanceJobRoutes mounts POST /instances/{instanceId}/install,
// /update and /update-check (docs/openapi.yaml → "instances" tag, WP-05).
func registerInstanceJobRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/install", d.installInstance)
	r.With(RequireRole(domain.RoleOperator)).Post("/instances/{instanceId}/update", d.updateInstanceGame)
	r.With(RequireRole(domain.RoleViewer)).Post("/instances/{instanceId}/update-check", d.checkInstanceUpdate)
}

func (d *Deps) installInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	if d.Steam == nil {
		WriteError(w, domain.E(domain.CodeInternal, "steam service not configured"))
		return
	}
	job, err := d.Steam.EnqueueInstall(r.Context(), id, RequestedBy(r))
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.install", id, job.ID, nil)
	WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

type updateGameRequest struct {
	StopIfRunning bool `json:"stop_if_running"`
}

func (d *Deps) updateInstanceGame(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	if d.Steam == nil {
		WriteError(w, domain.E(domain.CodeInternal, "steam service not configured"))
		return
	}
	var req updateGameRequest
	if err := DecodeOptionalJSON(r, &req); err != nil {
		WriteError(w, err)
		return
	}
	job, err := d.Steam.EnqueueUpdate(r.Context(), id, RequestedBy(r), req.StopIfRunning)
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.update", id, job.ID, map[string]any{"stop_if_running": req.StopIfRunning})
	WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (d *Deps) checkInstanceUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	if d.Steam == nil {
		WriteError(w, domain.E(domain.CodeInternal, "steam service not configured"))
		return
	}
	info, err := d.Steam.CheckUpdate(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.update_check", id, "", map[string]any{
		"installed_buildid": info.InstalledBuildID,
		"latest_buildid":    info.LatestBuildID,
		"update_available":  info.UpdateAvailable,
	})
	WriteJSON(w, http.StatusOK, info)
}
