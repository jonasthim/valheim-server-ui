package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// heartbeatInterval matches ARCHITECTURE.md §14: "heartbeat (every 25 s)".
const heartbeatInterval = 25 * time.Second

// registerEventRoutes mounts GET /events, the Server-Sent Events stream
// (docs/openapi.yaml → /events).
func registerEventRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleViewer)).Get("/events", sseHandler(d))
}

func sseHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, domain.E(domain.CodeInternal, "streaming unsupported"))
			return
		}

		instance := r.URL.Query().Get("instance")
		sub := d.Bus.Subscribe(instance)
		defer sub.Close()

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		h.Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		var nextID int64
		write := func(name string, data any) bool {
			payload, err := json.Marshal(data)
			if err != nil {
				d.Log.Error("sse: marshal event", "event", name, "err", err)
				return true // skip this one, keep the stream alive
			}
			nextID++
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", nextID, name, payload); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		heartbeat := time.NewTicker(heartbeatInterval)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if !write(ev.Name, ev.Data) {
					return
				}
			case t := <-heartbeat.C:
				if !write(domain.EventHeartbeat, map[string]any{"ts": t.UTC()}) {
					return
				}
			}
		}
	}
}
