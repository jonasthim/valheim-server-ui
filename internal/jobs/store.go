package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// timeLayout is used for every timestamp column: RFC3339 with nanoseconds,
// always UTC, so plain text ordering ("ORDER BY created_at") matches time
// ordering even for jobs created within the same second.
const timeLayout = time.RFC3339Nano

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

func nullInstance(id string) sql.NullString {
	if id == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: id, Valid: true}
}

func (r *Runner) insertJob(ctx context.Context, j domain.Job) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO jobs (id, type, instance_id, status, title, requested_by, created_at, started_at, finished_at, error, summary_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, '', '{}')`,
		j.ID, string(j.Type), nullInstance(j.InstanceID), string(j.Status), j.Title, j.RequestedBy, formatTime(j.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (r *Runner) updateJobRunning(ctx context.Context, id string, startedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = ?, started_at = ? WHERE id = ?`,
		string(domain.JobRunning), formatTime(startedAt), id)
	if err != nil {
		return fmt.Errorf("update job running: %w", err)
	}
	return nil
}

func (r *Runner) updateJobFinished(ctx context.Context, id string, status domain.JobStatus, finishedAt time.Time, errMsg string, summary map[string]any) error {
	summaryJSON := "{}"
	if len(summary) > 0 {
		b, err := json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("marshal job summary: %w", err)
		}
		summaryJSON = string(b)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET status = ?, finished_at = ?, error = ?, summary_json = ? WHERE id = ?`,
		string(status), formatTime(finishedAt), errMsg, summaryJSON, id)
	if err != nil {
		return fmt.Errorf("update job finished: %w", err)
	}
	return nil
}

const jobSelectColumns = `id, type, instance_id, status, title, requested_by, created_at, started_at, finished_at, error, summary_json`

func scanJob(scan func(dest ...any) error) (domain.Job, error) {
	var (
		j                                 domain.Job
		typ, status                       string
		instanceID, startedAt, finishedAt sql.NullString
		createdAt                         string
		summaryJSON                       string
	)
	if err := scan(&j.ID, &typ, &instanceID, &status, &j.Title, &j.RequestedBy, &createdAt, &startedAt, &finishedAt, &j.Error, &summaryJSON); err != nil {
		return domain.Job{}, err
	}
	j.Type = domain.JobType(typ)
	j.Status = domain.JobStatus(status)
	j.InstanceID = instanceID.String
	if t, err := parseTime(createdAt); err == nil {
		j.CreatedAt = t
	}
	if startedAt.Valid {
		if t, err := parseTime(startedAt.String); err == nil {
			j.StartedAt = &t
		}
	}
	if finishedAt.Valid {
		if t, err := parseTime(finishedAt.String); err == nil {
			j.FinishedAt = &t
		}
	}
	if summaryJSON != "" && summaryJSON != "{}" {
		var m map[string]any
		if err := json.Unmarshal([]byte(summaryJSON), &m); err == nil && len(m) > 0 {
			j.Summary = m
		}
	}
	return j, nil
}

// getJobFromDB reads one job row. Returns a *domain.Error(NotFound) when absent.
func (r *Runner) getJobFromDB(ctx context.Context, id string) (*domain.Job, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+jobSelectColumns+` FROM jobs WHERE id = ?`, id)
	j, err := scanJob(row.Scan)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NotFound("job")
		}
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &j, nil
}

// listJobsFromDB lists jobs newest-first. Empty instanceID/status mean "no
// filter"; limit <= 0 means "no limit".
func (r *Runner) listJobsFromDB(ctx context.Context, instanceID string, status domain.JobStatus, limit int) ([]domain.Job, error) {
	query := `SELECT ` + jobSelectColumns + ` FROM jobs
		WHERE (? = '' OR instance_id = ?) AND (? = '' OR status = ?)
		ORDER BY created_at DESC, rowid DESC`
	args := []any{instanceID, instanceID, string(status), string(status)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Job{}
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return out, nil
}

// recoveredJob is the minimal shape needed to mark a leftover row failed and
// announce it on the bus.
type recoveredJob struct {
	id, instanceID, title, requestedBy string
	typ                                domain.JobType
}

// recoverStaleJobs marks every row left in queued/running (from a process
// that died without a clean shutdown) as failed, and returns them so the
// caller can publish job.updated events. Jobs this process already owns
// (enqueued before Start, which is allowed) are not stale and are skipped.
func (r *Runner) recoverStaleJobs(ctx context.Context) ([]recoveredJob, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, instance_id, type, title, requested_by FROM jobs WHERE status IN ('queued','running')`)
	if err != nil {
		return nil, fmt.Errorf("query stale jobs: %w", err)
	}
	var stale []recoveredJob
	for rows.Next() {
		var rj recoveredJob
		var instanceID sql.NullString
		var typ string
		if err := rows.Scan(&rj.id, &instanceID, &typ, &rj.title, &rj.requestedBy); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan stale job: %w", err)
		}
		rj.instanceID = instanceID.String
		rj.typ = domain.JobType(typ)
		stale = append(stale, rj)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("query stale jobs: %w", err)
	}
	_ = rows.Close()

	r.mu.Lock()
	owned := stale[:0]
	for _, rj := range stale {
		if _, ok := r.jobs[rj.id]; !ok {
			owned = append(owned, rj)
		}
	}
	r.mu.Unlock()
	stale = owned

	now := time.Now().UTC()
	for _, rj := range stale {
		if err := r.updateJobFinished(ctx, rj.id, domain.JobFailed, now, "manager restarted", nil); err != nil {
			return nil, err
		}
	}
	return stale, nil
}
