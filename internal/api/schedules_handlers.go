package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerScheduleRoutes mounts /instances/{instanceId}/schedules* (WP-07,
// docs/openapi.yaml → "schedules" tag).
func registerScheduleRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.Schedules != nil }, "scheduler service not configured")

	r.With(RequireRole(domain.RoleViewer), guard).
		Get("/instances/{instanceId}/schedules", listSchedulesHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/schedules", createScheduleHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Patch("/instances/{instanceId}/schedules/{scheduleId}", updateScheduleHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Delete("/instances/{instanceId}/schedules/{scheduleId}", deleteScheduleHandler(d))
	r.With(RequireRole(domain.RoleOperator), guard).
		Post("/instances/{instanceId}/schedules/{scheduleId}/run", runScheduleHandler(d))
}

// scheduleIDParam reads and parses the {scheduleId} URL parameter. An
// unparseable id is part of the route shape, so it 404s like an unknown id
// would, rather than 422-ing.
func scheduleIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "scheduleId"), 10, 64)
	if err != nil {
		return 0, domain.NotFound("schedule")
	}
	return id, nil
}

func listSchedulesHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		list, err := d.Schedules.List(r.Context(), id)
		if err != nil {
			WriteError(w, err)
			return
		}
		if list == nil {
			list = []domain.Schedule{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"schedules": list})
	}
}

func createScheduleHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var in domain.ScheduleInput
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		sc, err := d.Schedules.Create(r.Context(), id, in)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "schedule.create", id, strconv.FormatInt(sc.ID, 10), map[string]any{
			"kind": sc.Kind, "cron": sc.Cron, "enabled": sc.Enabled,
		})
		WriteJSON(w, http.StatusCreated, map[string]any{"schedule": sc})
	}
}

func updateScheduleHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		scheduleID, err := scheduleIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		var in domain.ScheduleInput
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		sc, err := d.Schedules.Update(r.Context(), id, scheduleID, in)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "schedule.update", id, strconv.FormatInt(scheduleID, 10), map[string]any{
			"kind": sc.Kind, "cron": sc.Cron, "enabled": sc.Enabled,
		})
		WriteJSON(w, http.StatusOK, map[string]any{"schedule": sc})
	}
}

func deleteScheduleHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		scheduleID, err := scheduleIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		if err := d.Schedules.Delete(r.Context(), id, scheduleID); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "schedule.delete", id, strconv.FormatInt(scheduleID, 10), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func runScheduleHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := InstanceID(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		scheduleID, err := scheduleIDParam(r)
		if err != nil {
			WriteError(w, err)
			return
		}
		job, err := d.Schedules.RunNow(r.Context(), id, scheduleID, RequestedBy(r))
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "schedule.run", id, strconv.FormatInt(scheduleID, 10), map[string]any{"job_id": job.ID})
		WriteJSON(w, http.StatusAccepted, map[string]any{"job": job})
	}
}
