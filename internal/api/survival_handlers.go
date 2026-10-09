package api

import (
	"net/http"
	"strconv"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func getSurvivalHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var world int64
		if raw := r.URL.Query().Get("world_uid"); raw != "" {
			world, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				WriteValidation(w, domain.FieldError{Field: "world_uid", Message: "must be a signed 64-bit world ID"})
				return
			}
		}
		u := UserFrom(r.Context())
		includeHidden := u != nil && u.Role.AtLeast(domain.RoleOperator)
		history, err := d.Survival.Survival(r.Context(), id, world, r.URL.Query().Get("character_id"), includeHidden)
		if err != nil {
			WriteError(w, err)
			return
		}
		worlds := make([]string, 0, len(history.Worlds))
		for _, uid := range history.Worlds {
			worlds = append(worlds, strconv.FormatInt(uid, 10))
		}
		type momentResponse struct {
			domain.SurvivalMoment
			WorldUID string `json:"world_uid"`
		}
		moments := make([]momentResponse, 0, len(history.Moments))
		for _, m := range history.Moments {
			moments = append(moments, momentResponse{SurvivalMoment: m, WorldUID: strconv.FormatInt(m.WorldUID, 10)})
		}
		WriteJSON(w, http.StatusOK, struct {
			Worlds  []string         `json:"worlds"`
			Moments []momentResponse `json:"moments"`
		}{worlds, moments})
	}
}
