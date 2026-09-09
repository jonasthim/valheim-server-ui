// Package audit writes and reads the audit_log table (WP-01).
package audit

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Recorder implements api.Auditor on top of the audit_log table.
type Recorder struct {
	repo *db.AuditRepo
	log  *slog.Logger
}

// New constructs a Recorder. log may be nil (defaults to slog.Default()).
func New(repo *db.AuditRepo, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{repo: repo, log: log}
}

// Record implements api.Auditor. It reads the acting user and client IP from
// the request/context, so handlers only need to name the action.
func (r *Recorder) Record(req *http.Request, action, instanceID, target string, details map[string]any) {
	entry := domain.AuditEntry{
		Action:     action,
		InstanceID: instanceID,
		Target:     target,
		Details:    details,
		IP:         api.ClientIP(req),
	}
	if u := api.UserFrom(req.Context()); u != nil {
		id := u.ID
		entry.UserID = &id
		entry.Username = u.Username
	}
	if err := r.repo.Insert(req.Context(), entry); err != nil {
		r.log.Warn("audit: failed to record entry", "action", action, "err", err)
	}
}

// List returns audit entries for the /audit endpoint, newest first.
func (r *Recorder) List(ctx context.Context, instanceID, username string, limit int, before int64) ([]domain.AuditEntry, error) {
	return r.repo.List(ctx, instanceID, username, limit, before)
}
