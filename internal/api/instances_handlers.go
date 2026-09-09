package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerInstanceRoutes mounts every /instances endpoint except
// install/update/update-check (WP-05, registerInstanceJobRoutes) and the
// nested players/worlds/backups/schedules/mods resources (their own WPs).
func registerInstanceRoutes(r chi.Router, d *Deps) {
	r.Route("/instances", func(r chi.Router) {
		r.With(RequireRole(domain.RoleViewer)).Get("/", d.listInstances)
		r.With(RequireRole(domain.RoleAdmin)).Post("/", d.createInstance)

		r.Route("/{instanceId}", func(r chi.Router) {
			r.With(RequireRole(domain.RoleViewer)).Get("/", d.getInstance)
			r.With(RequireRole(domain.RoleOperator)).Patch("/", d.updateInstance)
			r.With(RequireRole(domain.RoleAdmin)).Delete("/", d.deleteInstance)

			r.With(RequireRole(domain.RoleOperator)).Post("/start", d.startInstance)
			r.With(RequireRole(domain.RoleOperator)).Post("/stop", d.stopInstance)
			r.With(RequireRole(domain.RoleOperator)).Post("/restart", d.restartInstance)
			r.With(RequireRole(domain.RoleViewer)).Get("/status", d.instanceStatus)

			r.With(RequireRole(domain.RoleViewer)).Get("/logs", d.instanceLogs)
			r.With(RequireRole(domain.RoleViewer)).Get("/logs/download", d.instanceLogsDownload)
		})
	})
}

// maskInstance returns inst with its config's password masked for viewers
// (ARCHITECTURE.md §13). Callers must not mutate the returned value's Config
// fields in place since it may share memory with inst for non-viewers.
func maskInstance(u *domain.User, inst *domain.Instance) domain.Instance {
	out := *inst
	if u == nil || u.Role == domain.RoleViewer {
		out.Config = out.Config.Masked()
	}
	return out
}

func (d *Deps) listInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := d.Instances.List(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	u := UserFrom(r.Context())
	out := make([]domain.Instance, len(instances))
	for i := range instances {
		out[i] = maskInstance(u, &instances[i])
	}
	WriteJSON(w, http.StatusOK, map[string]any{"instances": out})
}

type createInstanceRequest struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Config    domain.InstanceConfig `json:"config"`
	Autostart *bool                 `json:"autostart"`
	Install   *bool                 `json:"install"`
}

func (d *Deps) createInstance(w http.ResponseWriter, r *http.Request) {
	// Pre-populate defaults so omitted config fields keep them and explicit
	// zeros (e.g. game_backups: 0) survive.
	req := createInstanceRequest{Config: domain.DefaultInstanceConfig()}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, err)
		return
	}
	autostart := req.Autostart != nil && *req.Autostart

	inst, err := d.Instances.Create(r.Context(), req.ID, req.Name, req.Config, autostart)
	if err != nil {
		WriteError(w, err)
		return
	}

	resp := map[string]any{"instance": maskInstance(UserFrom(r.Context()), inst)}
	install := req.Install == nil || *req.Install
	if install && d.Steam != nil {
		job, err := d.Steam.EnqueueInstall(r.Context(), inst.ID, RequestedBy(r))
		if err != nil {
			d.Log.Warn("enqueue install job failed", "instance", inst.ID, "err", err)
		} else {
			resp["job"] = job
		}
	}

	d.audit(r, "instance.create", inst.ID, "", map[string]any{
		"name": inst.Name, "world": inst.Config.World, "port": inst.Config.Port,
	})
	WriteJSON(w, http.StatusCreated, resp)
}

func (d *Deps) getInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	inst, err := d.Instances.Get(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"instance": maskInstance(UserFrom(r.Context()), inst)})
}

type updateInstanceRequest struct {
	Name      *string                `json:"name"`
	Config    *domain.InstanceConfig `json:"config"`
	Autostart *bool                  `json:"autostart"`
}

func (d *Deps) updateInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody))
	if err != nil {
		WriteError(w, domain.Wrap(domain.CodeValidationFailed, "read body", err))
		return
	}
	// Detect whether "config" was sent at all; when it was, decode it over the
	// current config so omitted fields keep their stored values.
	var probe struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		WriteError(w, domain.Wrap(domain.CodeValidationFailed, "invalid JSON body: "+err.Error(), err))
		return
	}
	cur, err := d.Instances.Get(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	var req updateInstanceRequest
	if len(probe.Config) > 0 && string(probe.Config) != "null" {
		cfg := cur.Config
		// "modifiers" is a set, not a list of independent fields: an unset rule
		// is simply absent from the object (the form never sends an empty
		// value), so a sent object replaces the stored one wholesale. Without
		// this, a rule could never be switched back to "not set".
		var cfgProbe struct {
			Modifiers json.RawMessage `json:"modifiers"`
		}
		if err := json.Unmarshal(probe.Config, &cfgProbe); err == nil &&
			len(cfgProbe.Modifiers) > 0 && string(cfgProbe.Modifiers) != "null" {
			cfg.Modifiers = domain.Modifiers{}
		}
		req.Config = &cfg
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		WriteError(w, domain.Wrap(domain.CodeValidationFailed, "invalid JSON body: "+err.Error(), err))
		return
	}
	inst, err := d.Instances.Update(r.Context(), id, req.Name, req.Config, req.Autostart)
	if err != nil {
		WriteError(w, err)
		return
	}

	// Record what actually changed (secrets masked by auditDiff), not the
	// whole config, so the audit log shows e.g. "config.modifiers.portals:
	// hard -> unset" for a rules edit.
	d.audit(r, "instance.update", id, "", map[string]any{
		"changes": auditDiff(instanceAuditView(cur), instanceAuditView(inst)),
	})

	WriteJSON(w, http.StatusOK, map[string]any{"instance": maskInstance(UserFrom(r.Context()), inst)})
}

func (d *Deps) deleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	deleteFiles, _ := strconv.ParseBool(r.URL.Query().Get("delete_files"))
	if err := d.Instances.Delete(r.Context(), id, deleteFiles); err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.delete", id, "", map[string]any{"delete_files": deleteFiles})
	w.WriteHeader(http.StatusNoContent)
}

func (d *Deps) startInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	st, err := d.Instances.Start(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.start", id, "", nil)
	WriteJSON(w, http.StatusOK, map[string]any{"status": st})
}

func (d *Deps) stopInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	st, err := d.Instances.Stop(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.stop", id, "", nil)
	WriteJSON(w, http.StatusOK, map[string]any{"status": st})
}

func (d *Deps) restartInstance(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	st, err := d.Instances.Restart(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	d.audit(r, "instance.restart", id, "", nil)
	WriteJSON(w, http.StatusOK, map[string]any{"status": st})
}

func (d *Deps) instanceStatus(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	st, err := d.Instances.Status(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"status": st})
}

const (
	defaultLogLines = 500
	maxLogLines     = 5000
)

func (d *Deps) instanceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	lines := defaultLogLines
	if v := r.URL.Query().Get("lines"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			WriteValidation(w, domain.FieldError{Field: "lines", Message: "must be a non-negative integer"})
			return
		}
		lines = n
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}
	out, err := d.Instances.TailLog(r.Context(), id, lines)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"lines": out})
}

func (d *Deps) instanceLogsDownload(w http.ResponseWriter, r *http.Request) {
	id, err := InstanceID(r)
	if err != nil {
		WriteError(w, err)
		return
	}
	rc, err := d.Instances.OpenLog(r.Context(), id)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-console.log"`, id))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// instanceAuditView is the subset of an instance that users edit through
// PATCH, in the shape the audit diff reports paths for.
func instanceAuditView(inst *domain.Instance) map[string]any {
	if inst == nil {
		return nil
	}
	return map[string]any{"name": inst.Name, "autostart": inst.Autostart, "config": inst.Config}
}
