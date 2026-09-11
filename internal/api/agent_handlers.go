package api

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
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
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/agent/catalog", getAgentCatalogHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/agent/chat", getAgentChatHandler(d))
	r.With(RequireRole(domain.RoleOperator), modsGuard).
		Post("/instances/{instanceId}/agent/install", installAgentHandler(d))

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map", getMapHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map.png", getMapImageHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map/explored.png", getExploredImageHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map/tiles/{z}/{x}/{y}.png", getTileHandler(d))
	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/map/water.png", getWaterMaskHandler(d))
	r.With(RequireRole(domain.RoleViewer)).
		Get("/instances/{instanceId}/map/clouds.png", getCloudsHandler())
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

// fogRequested reads ?fog=0 (bare map, operators only). It writes the 403
// itself and returns false in the second value when the caller must stop.
func fogRequested(w http.ResponseWriter, r *http.Request) (fog bool, ok bool) {
	switch r.URL.Query().Get("fog") {
	case "0", "false", "off":
		u := UserFrom(r.Context())
		if u == nil || !u.Role.AtLeast(domain.RoleOperator) {
			WriteError(w, domain.E(domain.CodeForbidden, "the bare map without fog is for operators"))
			return false, false
		}
		return false, true
	}
	return true, true
}

// getTileHandler serves one tile of the deep-zoom pyramid, fogged unless an
// operator asks for the bare map. 202 with MapInfo while the plugin samples
// or its first fog mask encodes; 404 outside the pyramid; 409 for agents
// without layers.
func getTileHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		z, errZ := strconv.Atoi(chi.URLParam(r, "z"))
		x, errX := strconv.Atoi(chi.URLParam(r, "x"))
		y, errY := strconv.Atoi(chi.URLParam(r, "y"))
		if errZ != nil || errX != nil || errY != nil {
			WriteError(w, domain.E(domain.CodeNotFound, "no such tile"))
			return
		}
		fog, ok := fogRequested(w, r)
		if !ok {
			return
		}
		data, etag, info, err := d.Agent.TilePNG(r.Context(), id, z, x, y, fog)
		if err != nil {
			if info != nil && domain.AsError(err).Code == domain.CodeInternal {
				// Still sampling or encoding: progress instead of an error.
				WriteJSON(w, http.StatusAccepted, info)
				return
			}
			WriteError(w, err)
			return
		}
		if etag != "" && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=10")
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data) //nolint:gosec // PNG bytes the service rendered, served as image/png
	}
}

// getWaterMaskHandler serves the fogged water mask (grey PNG, 255 where the
// map shows explored water) the UI confines its water shimmer to.
func getWaterMaskHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		data, etag, err := d.Agent.WaterMaskPNG(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		if etag != "" && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=10")
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data) //nolint:gosec // PNG bytes the service rendered
	}
}

// getCloudsHandler serves the seamless cloud texture the UI drifts over the
// unexplored parchment. Same for every instance; cacheable for a day.
func getCloudsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mapstyle.CloudsPNG()) //nolint:gosec // generated PNG bytes
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
		fog, ok := fogRequested(w, r)
		if !ok {
			return
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
			fields = append(fields, domain.FieldError{Field: "command", Message: "must be one of save, kick, ban, unban, broadcast, time, say, setkey, removekey, event, eventstop"})
		}
		fields = append(fields, validateAgentCommand(req)...)
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
		if req.Key != "" {
			details["key"] = req.Key
		}
		if req.Event != "" {
			details["event"] = req.Event
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

// agentKeyPattern mirrors the plugin's global-key rule (Commands.cs).
var agentKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// validateAgentCommand checks the 1.11 verbs' arguments before they reach the
// agent, so a bad request is a 422 with field detail rather than an OK=false
// round-trip. The command membership is checked by the caller.
func validateAgentCommand(req domain.AgentCommandRequest) []domain.FieldError {
	var fields []domain.FieldError
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
	case "say":
		if req.Message == "" || len(req.Message) > maxAgentMessageLen {
			fields = append(fields, domain.FieldError{Field: "message", Message: "1-200 characters"})
		}
		if len(req.Name) > 32 {
			fields = append(fields, domain.FieldError{Field: "name", Message: "at most 32 characters"})
		}
	case "time":
		set := 0
		if req.Skip != "" {
			set++
			if req.Skip != "morning" {
				fields = append(fields, domain.FieldError{Field: "skip", Message: "the only supported value is morning"})
			}
		}
		if req.Fraction != nil {
			set++
			if *req.Fraction < 0 || *req.Fraction > 1 {
				fields = append(fields, domain.FieldError{Field: "fraction", Message: "between 0 and 1"})
			}
		}
		if req.Seconds != nil {
			set++
			if *req.Seconds < 1 || *req.Seconds > 86400 {
				fields = append(fields, domain.FieldError{Field: "seconds", Message: "between 1 and 86400"})
			}
		}
		if set != 1 {
			fields = append(fields, domain.FieldError{Field: "time", Message: "set exactly one of fraction, skip or seconds"})
		}
	case "setkey", "removekey":
		if !agentKeyPattern.MatchString(req.Key) {
			fields = append(fields, domain.FieldError{Field: "key", Message: "must match [A-Za-z0-9_]{1,64}"})
		}
	case "event":
		if req.Event == "" || len(req.Event) > 64 {
			fields = append(fields, domain.FieldError{Field: "event", Message: "event name, 1-64 characters"})
		}
		if (req.X == nil) != (req.Z == nil) {
			fields = append(fields, domain.FieldError{Field: "x", Message: "set both x and z, or neither"})
		}
	}
	return fields
}

func getAgentCatalogHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		cat, err := d.Agent.Catalog(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, cat)
	}
}

func getAgentChatHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var since int64
		if v := r.URL.Query().Get("since"); v != "" {
			since, _ = strconv.ParseInt(v, 10, 64)
		}
		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		ch, err := d.Agent.Chat(r.Context(), id, since, limit)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, ch)
	}
}
