package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerPlayerRoutes mounts /instances/{instanceId}/players and
// /instances/{instanceId}/lists/{listKind} (docs/openapi.yaml "players" tag).
func registerPlayerRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleViewer)).Get("/instances/{instanceId}/players", handleGetPlayers(d))
	r.With(RequireRole(domain.RoleViewer)).Get("/instances/{instanceId}/lists/{listKind}", handleGetPlayerList(d))
	r.With(RequireRole(domain.RoleOperator)).Put("/instances/{instanceId}/lists/{listKind}", handlePutPlayerList(d))
}

// listKindParam reads and validates the {listKind} URL parameter, 404-ing
// (not 422) for an unknown kind since it is part of the route shape.
func listKindParam(r *http.Request) (domain.ListKind, error) {
	kind := domain.ListKind(chi.URLParam(r, "listKind"))
	if !kind.Valid() {
		return "", domain.NotFound("list")
	}
	return kind, nil
}

func handleGetPlayers(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Players == nil {
			WriteError(w, domain.E(domain.CodeInternal, "players service not configured"))
			return
		}
		resp, err := d.Players.Players(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

func handleGetPlayerList(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		kind, err := listKindParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Players == nil {
			WriteError(w, domain.E(domain.CodeInternal, "players service not configured"))
			return
		}
		list, err := d.Players.GetList(r.Context(), id, kind)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, list)
	}
}

func handlePutPlayerList(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		kind, err := listKindParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if d.Players == nil {
			WriteError(w, domain.E(domain.CodeInternal, "players service not configured"))
			return
		}
		var body domain.PlayerList
		if err := DecodeJSON(r, &body); err != nil {
			WriteError(w, err)
			return
		}
		body.Kind = kind // the URL is authoritative

		prev, prevErr := d.Players.GetList(r.Context(), id, kind)
		updated, err := d.Players.PutList(r.Context(), id, body)
		if err != nil {
			WriteError(w, err)
			return
		}
		details := map[string]any{
			"kind":  string(kind),
			"count": len(updated.Entries),
		}
		if prevErr == nil && prev != nil {
			details["changes"] = listAuditChanges(prev.Entries, updated.Entries)
		}
		d.audit(r, "players.list.update", id, string(kind), details)
		WriteJSON(w, http.StatusOK, updated)
	}
}

// listAuditChanges reports which ids were added to or removed from a player
// list as audit changes ("added"/"removed" paths), ignoring comment edits.
func listAuditChanges(before, after []domain.PlayerListEntry) []auditChange {
	prev := map[string]bool{}
	for _, e := range before {
		prev[e.ID] = true
	}
	next := map[string]bool{}
	for _, e := range after {
		next[e.ID] = true
	}
	var added, removed []string
	for _, e := range after {
		if !prev[e.ID] {
			added = append(added, e.ID)
		}
	}
	for _, e := range before {
		if !next[e.ID] {
			removed = append(removed, e.ID)
		}
	}
	changes := []auditChange{}
	if len(added) > 0 {
		changes = append(changes, auditChange{Path: "added", To: added})
	}
	if len(removed) > 0 {
		changes = append(changes, auditChange{Path: "removed", From: removed})
	}
	return changes
}
