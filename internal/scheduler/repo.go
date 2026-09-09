package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// scheduleRow is the persisted shape of one schedules table row
// (internal/db/migrations/00001_init.sql).
type scheduleRow struct {
	ID            int64
	InstanceID    string
	Kind          string
	CronExpr      string
	Enabled       bool
	OnlyWhenEmpty bool
	Note          string
	LastRunAt     *time.Time
	LastResult    string
	LastJobID     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const scheduleColumns = `id, instance_id, kind, cron_expr, enabled, only_when_empty, note,
	last_run_at, last_result, last_job_id, created_at, updated_at`

func scanScheduleRow(scan func(...any) error) (scheduleRow, error) {
	var (
		r                      scheduleRow
		enabled, onlyWhenEmpty int
		lastRunAt              sql.NullString
		createdAt, updatedAt   string
	)
	if err := scan(&r.ID, &r.InstanceID, &r.Kind, &r.CronExpr, &enabled, &onlyWhenEmpty, &r.Note,
		&lastRunAt, &r.LastResult, &r.LastJobID, &createdAt, &updatedAt); err != nil {
		return scheduleRow{}, err
	}
	r.Enabled = enabled != 0
	r.OnlyWhenEmpty = onlyWhenEmpty != 0
	if lastRunAt.Valid && lastRunAt.String != "" {
		if t, err := time.Parse(time.RFC3339, lastRunAt.String); err == nil {
			r.LastRunAt = &t
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

// getScheduleRow fetches one schedule by id, regardless of instance.
func (s *Service) getScheduleRow(ctx context.Context, id int64) (scheduleRow, error) {
	q := `SELECT ` + scheduleColumns + ` FROM schedules WHERE id = ?`
	rs := s.db.QueryRowContext(ctx, q, id)
	r, err := scanScheduleRow(rs.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scheduleRow{}, domain.NotFound("schedule")
		}
		return scheduleRow{}, fmt.Errorf("query schedule %d: %w", id, err)
	}
	return r, nil
}

// listScheduleRows returns schedules for instanceID (all instances when
// instanceID == ""), ordered by id for stable output.
func (s *Service) listScheduleRows(ctx context.Context, instanceID string) ([]scheduleRow, error) {
	q := `SELECT ` + scheduleColumns + ` FROM schedules`
	args := []any{}
	if instanceID != "" {
		q += ` WHERE instance_id = ?`
		args = append(args, instanceID)
	}
	q += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []scheduleRow
	for rows.Next() {
		r, err := scanScheduleRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return out, nil
}

// listEnabledScheduleRows returns every enabled schedule across all
// instances, for loading the cron engine.
func (s *Service) listEnabledScheduleRows(ctx context.Context) ([]scheduleRow, error) {
	q := `SELECT ` + scheduleColumns + ` FROM schedules WHERE enabled = 1 ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []scheduleRow
	for rows.Next() {
		r, err := scanScheduleRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list enabled schedules: %w", err)
	}
	return out, nil
}

func (s *Service) insertScheduleRow(ctx context.Context, r scheduleRow) (int64, error) {
	q := `INSERT INTO schedules (instance_id, kind, cron_expr, enabled, only_when_empty, note,
		last_run_at, last_result, last_job_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, q, r.InstanceID, r.Kind, r.CronExpr, boolToInt(r.Enabled), boolToInt(r.OnlyWhenEmpty), r.Note,
		nullableTime(r.LastRunAt), r.LastResult, r.LastJobID, formatTime(r.CreatedAt), formatTime(r.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("insert schedule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert schedule: %w", err)
	}
	return id, nil
}

// updateScheduleRow persists the editable fields (kind/cron/enabled/
// only_when_empty/note/updated_at). It never touches last_run_at/
// last_result/last_job_id; use updateLastRun/updateLastResult for those.
func (s *Service) updateScheduleRow(ctx context.Context, r scheduleRow) error {
	q := `UPDATE schedules SET kind = ?, cron_expr = ?, enabled = ?, only_when_empty = ?, note = ?, updated_at = ?
		WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, r.Kind, r.CronExpr, boolToInt(r.Enabled), boolToInt(r.OnlyWhenEmpty), r.Note,
		formatTime(r.UpdatedAt), r.ID)
	if err != nil {
		return fmt.Errorf("update schedule %d: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("schedule")
	}
	return nil
}

func (s *Service) deleteScheduleRow(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("schedule")
	}
	return nil
}

// updateLastRun records the outcome of a fire (scheduled or manual "run now").
func (s *Service) updateLastRun(ctx context.Context, id int64, at time.Time, result, jobID string) error {
	q := `UPDATE schedules SET last_run_at = ?, last_result = ?, last_job_id = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, formatTime(at), result, jobID, id)
	if err != nil {
		return fmt.Errorf("update schedule %d last run: %w", id, err)
	}
	return nil
}

// updateLastResult overwrites last_result only, used once an asynchronously
// awaited job reaches a terminal state after updateLastRun already ran.
func (s *Service) updateLastResult(ctx context.Context, id int64, result string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE schedules SET last_result = ? WHERE id = ?`, result, id)
	if err != nil {
		return fmt.Errorf("update schedule %d last result: %w", id, err)
	}
	return nil
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
