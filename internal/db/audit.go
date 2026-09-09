package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// AuditRepo is the repository for the audit_log table.
type AuditRepo struct{ DB *sql.DB }

// NewAuditRepo constructs an AuditRepo.
func NewAuditRepo(db *sql.DB) *AuditRepo { return &AuditRepo{DB: db} }

// Insert writes one audit row. entry.ID and entry.TS (if zero) are assigned.
func (r *AuditRepo) Insert(ctx context.Context, entry domain.AuditEntry) error {
	if entry.TS.IsZero() {
		entry.TS = time.Now()
	}
	details := entry.Details
	if details == nil {
		details = map[string]any{}
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	var userID sql.NullInt64
	if entry.UserID != nil {
		userID = sql.NullInt64{Int64: *entry.UserID, Valid: true}
	}
	_, err = r.DB.ExecContext(ctx, `
		INSERT INTO audit_log (ts, user_id, username, action, instance_id, target, details_json, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		nowString(entry.TS), userID, entry.Username, entry.Action, entry.InstanceID, entry.Target,
		string(detailsJSON), entry.IP)
	if err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// List returns audit entries newest-first, optionally filtered by instance
// and/or username, paged with a "before id" cursor (0 = no cursor).
func (r *AuditRepo) List(ctx context.Context, instanceID, username string, limit int, before int64) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var (
		where []string
		args  []any
	)
	if instanceID != "" {
		where = append(where, "instance_id = ?")
		args = append(args, instanceID)
	}
	if username != "" {
		where = append(where, "username = ?")
		args = append(args, username)
	}
	if before > 0 {
		where = append(where, "id < ?")
		args = append(args, before)
	}
	q := `SELECT id, ts, user_id, username, action, instance_id, target, details_json, ip FROM audit_log`
	if len(where) > 0 {
		//nolint:gosec // where holds only fixed column-name literals from above, never user input; all values are bound params.
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.AuditEntry
	for rows.Next() {
		var (
			e           domain.AuditEntry
			ts          string
			userID      sql.NullInt64
			detailsJSON string
		)
		if err := rows.Scan(&e.ID, &ts, &userID, &e.Username, &e.Action, &e.InstanceID, &e.Target, &detailsJSON, &e.IP); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		e.TS = parseTime(ts)
		if userID.Valid {
			id := userID.Int64
			e.UserID = &id
		}
		if detailsJSON != "" && detailsJSON != "{}" {
			var d map[string]any
			if err := json.Unmarshal([]byte(detailsJSON), &d); err == nil {
				e.Details = d
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
