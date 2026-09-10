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
