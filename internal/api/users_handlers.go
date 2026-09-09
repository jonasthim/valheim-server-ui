package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerUserRoutes mounts /users* (docs/openapi.yaml → users tag, admin only).
func registerUserRoutes(r chi.Router, d *Deps) {
	r.Route("/users", func(r chi.Router) {
		r.Use(RequireRole(domain.RoleAdmin))
		r.Use(requireService(func() bool { return d.Users != nil }, "user service not configured"))
		r.Get("/", listUsersHandler(d))
		r.Post("/", createUserHandler(d))
		r.Route("/{userId}", func(r chi.Router) {
			r.Get("/", getUserHandler(d))
			r.Patch("/", updateUserHandler(d))
			r.Delete("/", deleteUserHandler(d))
			r.Put("/password", setUserPasswordHandler(d))
		})
	})
}

func userIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		return 0, domain.E(domain.CodeValidationFailed, "invalid user id")
	}
	return id, nil
}

func listUsersHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := d.Users.List(r.Context())
		if err != nil {
			WriteError(w, err)
			return
		}
		if users == nil {
			users = []domain.User{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"users": users})
	}
}

type createUserRequest struct {
	Username    string      `json:"username"`
	Password    string      `json:"password"`
	DisplayName string      `json:"display_name"`
	Email       string      `json:"email"`
	Role        domain.Role `json:"role"`
}

func createUserHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in createUserRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Users.Create(r.Context(), in.Username, in.Password, in.DisplayName, in.Email, in.Role)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "user.create", "", usr.Username, map[string]any{"role": usr.Role})
		WriteJSON(w, http.StatusCreated, map[string]any{"user": usr})
	}
}

func getUserHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := userIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Users.Get(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"user": usr})
	}
}

type updateUserRequest struct {
	DisplayName *string      `json:"display_name"`
	Email       *string      `json:"email"`
	Role        *domain.Role `json:"role"`
	Disabled    *bool        `json:"disabled"`
}

func updateUserHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := userIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var in updateUserRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Users.Update(r.Context(), id, in.DisplayName, in.Email, in.Role, in.Disabled)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "user.update", "", usr.Username, map[string]any{
			"display_name": in.DisplayName, "email": in.Email, "role": in.Role, "disabled": in.Disabled,
		})
		WriteJSON(w, http.StatusOK, map[string]any{"user": usr})
	}
}

func deleteUserHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := userIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Users.Get(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		if err := d.Users.Delete(r.Context(), id); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "user.delete", "", usr.Username, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

type setPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func setUserPasswordHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := userIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var in setPasswordRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Users.Get(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		if err := d.Users.SetPassword(r.Context(), id, in.NewPassword); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "user.password.set", "", usr.Username, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}
