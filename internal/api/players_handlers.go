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

		updated, err := d.Players.PutList(r.Context(), id, body)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "players.list.update", id, string(kind), map[string]any{
			"kind":  string(kind),
			"count": len(updated.Entries),
		})
		WriteJSON(w, http.StatusOK, updated)
	}
}
