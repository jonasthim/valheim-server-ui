package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerAgentRoutes mounts /instances/{instanceId}/agent* (docs/openapi.yaml
// "agent" tag): the server plugin's status, its admin commands and its
// install/update job.
func registerAgentRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.Agent != nil }, "agent service not configured")
	modsGuard := requireService(func() bool { return d.Mods != nil }, "mods service not configured")

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/agent", getAgentHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/agent/commands", agentCommandHandler(d))
	r.With(RequireRole(domain.RoleOperator), modsGuard).
		Post("/instances/{instanceId}/agent/install", installAgentHandler(d))

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map", getMapHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map.png", getMapImageHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map/explored.png", getExploredImageHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/map/render", renderMapHandler(d))
}

// getExploredImageHandler serves the fog mask (grey+alpha PNG, opaque where
// unexplored). 202 with ExploredInfo while the plugin encodes its first mask.
func getExploredImageHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		path, info, err := d.Agent.ExploredPNG(r.Context(), id)
		if err != nil {
			if info != nil {
				WriteJSON(w, http.StatusAccepted, info)
				return
			}
			WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=10")
		http.ServeFile(w, r, path) //nolint:gosec // path is a file the agent service wrote into the instance's own cache dir
	}
}

func getMapHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		u := UserFrom(r.Context())
		operator := u != nil && u.Role.AtLeast(domain.RoleOperator)
		m, err := d.Agent.Map(r.Context(), id, operator)
		if err != nil {
			WriteError(w, err)
			return
		}
		if !operator {
			stripUnexplored(m)
		}
		WriteJSON(w, http.StatusOK, m)
	}
}

// stripUnexplored removes what lies under the fog from a viewer's answer,
// so the JSON does not reveal what the fogged image hides.
func stripUnexplored(m *domain.InstanceMap) {
	objs := m.Objects[:0]
	for _, o := range m.Objects {
		if o.Explored == nil || *o.Explored {
			objs = append(objs, o)
		}
	}
	m.Objects = objs
	locs := m.Locations[:0]
	for _, l := range m.Locations {
		if l.Explored == nil || *l.Explored || (l.Discovered != nil && *l.Discovered) {
			locs = append(locs, l)
		}
	}
	m.Locations = locs
}

// getMapImageHandler serves the rendered map with the fog of war composited
// in on the server. `?fog=0` asks for the bare render and is operators only.
// 200 image/png when available, 202 with MapInfo while the plugin renders
// or the first fog mask encodes, 409 when nothing exists yet.
func getMapImageHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		fog := true
		switch r.URL.Query().Get("fog") {
		case "0", "false", "off":
			u := UserFrom(r.Context())
			if u == nil || !u.Role.AtLeast(domain.RoleOperator) {
				WriteError(w, domain.E(domain.CodeForbidden, "the bare map without fog is for operators"))
				return
			}
			fog = false
		}
		path, info, err := d.Agent.MapPNG(r.Context(), id, fog)
		if err != nil {
			if info != nil {
				WriteJSON(w, http.StatusAccepted, info)
				return
			}
			WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		// Clients key the URL on InstanceMap.image_version, which changes on
		// every render and fog rebuild; keep the browser cache short anyway.
		w.Header().Set("Cache-Control", "private, max-age=10")
		http.ServeFile(w, r, path) //nolint:gosec // path is a file the agent service wrote into the instance's own cache dir
	}
}

func renderMapHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req domain.MapRenderRequest
		if r.ContentLength != 0 {
			if err := DecodeJSON(r, &req); err != nil {
				WriteError(w, err)
				return
			}
		}
		info, err := d.Agent.RenderMap(r.Context(), id, req)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "agent.map_render", id, "", map[string]any{"size": info.Size, "force": req.Force})
		WriteJSON(w, http.StatusAccepted, info)
	}
}

func getAgentHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		// Players who hide their map position stay hidden from viewers; an
		// operator sees everyone (the same people they can kick).
		u := UserFrom(r.Context())
		includeHidden := u != nil && u.Role.AtLeast(domain.RoleOperator)
		info, err := d.Agent.Info(r.Context(), id, includeHidden)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, info)
	}
}

const maxAgentMessageLen = 200

func agentCommandHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req domain.AgentCommandRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, err)
			return
		}
		var fields []domain.FieldError
		known := false
		for _, c := range domain.AgentCommands {
			if c == req.Command {
				known = true
			}
		}
		if !known {
			fields = append(fields, domain.FieldError{Field: "command", Message: "must be one of save, kick, ban, unban, broadcast"})
		}
		switch req.Command {
		case "kick", "ban", "unban":
			if req.Target == "" || len(req.Target) > 64 {
				fields = append(fields, domain.FieldError{Field: "target", Message: "player name or platform id, 1-64 characters"})
			}
		case "broadcast":
			if req.Message == "" || len(req.Message) > maxAgentMessageLen {
				fields = append(fields, domain.FieldError{Field: "message", Message: "1-200 characters"})
			}
			if req.Style != "" && req.Style != "center" && req.Style != "topleft" {
				fields = append(fields, domain.FieldError{Field: "style", Message: "center or topleft"})
			}
		}
		if len(fields) > 0 {
			WriteValidation(w, fields...)
			return
		}
		res, err := d.Agent.Command(r.Context(), id, req)
		if err != nil {
			WriteError(w, err)
			return
		}
		details := map[string]any{"command": req.Command, "ok": res.OK}
		if req.Target != "" {
			details["target"] = req.Target
		}
		if req.Message != "" {
			details["message"] = req.Message
		}
		d.audit(r, "agent.command", id, req.Target, details)
		WriteJSON(w, http.StatusOK, res)
	}
}

func installAgentHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var req struct {
			StopIfRunning bool `json:"stop_if_running"`
		}
		if r.ContentLength != 0 {
			if err := DecodeJSON(r, &req); err != nil {
				WriteError(w, err)
				return
			}
		}
		job, err := d.Mods.EnqueueAgentInstall(r.Context(), id, req.StopIfRunning, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "agent.install", id, job.ID, map[string]any{"stop_if_running": req.StopIfRunning})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}
