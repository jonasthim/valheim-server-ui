package instance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// row is the persisted shape of one instances table row (internal/db/migrations/00001_init.sql).
type row struct {
	ID               string
	Name             string
	Config           domain.InstanceConfig
	Autostart        bool
	PendingRestart   bool
	InstalledBuildID string
	LatestBuildID    string
	BuildIDCheckedAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const rowColumns = `id, name, config_json, autostart, pending_restart, installed_buildid, latest_buildid, buildid_checked_at, created_at, updated_at`

func scanRow(scan func(...any) error) (row, error) {
	var (
		r                         row
		cfgJSON                   string
		autostart, pendingRestart int
		buildIDCheckedAt          sql.NullString
		createdAt, updatedAt      string
	)
	if err := scan(&r.ID, &r.Name, &cfgJSON, &autostart, &pendingRestart,
		&r.InstalledBuildID, &r.LatestBuildID, &buildIDCheckedAt, &createdAt, &updatedAt); err != nil {
		return row{}, err
	}
	if err := json.Unmarshal([]byte(cfgJSON), &r.Config); err != nil {
		return row{}, fmt.Errorf("decode config_json for instance %s: %w", r.ID, err)
	}
	r.Autostart = autostart != 0
	r.PendingRestart = pendingRestart != 0
	if buildIDCheckedAt.Valid && buildIDCheckedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, buildIDCheckedAt.String); err == nil {
			r.BuildIDCheckedAt = &t
		}
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		r.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		r.UpdatedAt = t
	}
	return r, nil
}

func (s *Service) getRow(ctx context.Context, id string) (row, error) {
	q := `SELECT ` + rowColumns + ` FROM instances WHERE id = ?`
	rs := s.db.QueryRowContext(ctx, q, id)
	r, err := scanRow(rs.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row{}, domain.NotFound("instance")
		}
		return row{}, fmt.Errorf("query instance %s: %w", id, err)
	}
	return r, nil
}

func (s *Service) listRows(ctx context.Context) ([]row, error) {
	q := `SELECT ` + rowColumns + ` FROM instances ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []row
	for rows.Next() {
		r, err := scanRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	return out, nil
}

func (s *Service) insertRow(ctx context.Context, r row) error {
	cfgJSON, err := json.Marshal(r.Config) //nolint:gosec // config_json is meant to persist the game password
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	q := `INSERT INTO instances (id, name, config_json, autostart, pending_restart,
		installed_buildid, latest_buildid, buildid_checked_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q, r.ID, r.Name, string(cfgJSON), boolToInt(r.Autostart), boolToInt(r.PendingRestart),
		r.InstalledBuildID, r.LatestBuildID, nullableTime(r.BuildIDCheckedAt), formatTime(r.CreatedAt), formatTime(r.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert instance %s: %w", r.ID, err)
	}
	return nil
}

func (s *Service) saveRow(ctx context.Context, r row) error {
	cfgJSON, err := json.Marshal(r.Config) //nolint:gosec // config_json is meant to persist the game password
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	q := `UPDATE instances SET name = ?, config_json = ?, autostart = ?, pending_restart = ?,
		installed_buildid = ?, latest_buildid = ?, buildid_checked_at = ?, updated_at = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, r.Name, string(cfgJSON), boolToInt(r.Autostart), boolToInt(r.PendingRestart),
		r.InstalledBuildID, r.LatestBuildID, nullableTime(r.BuildIDCheckedAt), formatTime(r.UpdatedAt), r.ID)
	if err != nil {
		return fmt.Errorf("update instance %s: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("instance")
	}
	return nil
}

func (s *Service) deleteRow(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete instance %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("instance")
	}
	return nil
}

func (s *Service) existsRow(ctx context.Context, id string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM instances WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check instance %s: %w", id, err)
	}
	return true, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}
