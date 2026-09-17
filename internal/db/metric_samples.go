package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// MetricSamplesRepo is the repository for the metric_samples table: resource
// and player history sampled roughly every 60s (F-1.3), one row per running
// instance plus one host row (instance_id NULL) per tick.
type MetricSamplesRepo struct{ DB *sql.DB }

// NewMetricSamplesRepo constructs a MetricSamplesRepo.
func NewMetricSamplesRepo(db *sql.DB) *MetricSamplesRepo { return &MetricSamplesRepo{DB: db} }

// Insert writes one metric_samples row. s.At (if zero) is set to now.
func (r *MetricSamplesRepo) Insert(ctx context.Context, s domain.MetricSample) error {
	if s.At.IsZero() {
		s.At = time.Now()
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO metric_samples (instance_id, at, cpu, mem, players, disk_free, hourly)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.InstanceID, nowString(s.At), s.CPU, s.Mem, s.Players, s.DiskFree, boolToInt(s.Hourly))
	if err != nil {
		return fmt.Errorf("insert metric sample: %w", err)
	}
	return nil
}

// Range returns instanceID's samples (nil = the host) at or after since,
// ascending by time, both raw and already-downsampled rows.
func (r *MetricSamplesRepo) Range(ctx context.Context, instanceID *string, since time.Time) ([]domain.MetricSample, error) {
	q := `SELECT instance_id, at, cpu, mem, players, disk_free, hourly FROM metric_samples WHERE at >= ?`
	args := []any{nowString(since)}
	if instanceID == nil {
		q += ` AND instance_id IS NULL`
	} else {
		q += ` AND instance_id = ?`
		args = append(args, *instanceID)
	}
	q += ` ORDER BY at ASC, id ASC`

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("range metric samples: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.MetricSample
	for rows.Next() {
		s, err := scanMetricSample(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan metric sample: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Downsample collapses raw (hourly=0) rows older than olderThan into one
// hourly=1 row per (instance_id, hour) bucket — averaging cpu/mem, taking the
// max players and min disk_free seen in that hour — then deletes the raw rows
// it summarised. Idempotent: rows already marked hourly are never
// re-aggregated, so a second call over the same window is a no-op.
func (r *MetricSamplesRepo) Downsample(ctx context.Context, olderThan time.Time) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("downsample metric samples: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cutoff := nowString(olderThan)
	const bucket = `strftime('%Y-%m-%dT%H:00:00Z', at)`
	_, err = tx.ExecContext(ctx, `
		INSERT INTO metric_samples (instance_id, at, cpu, mem, players, disk_free, hourly)
		SELECT instance_id, `+bucket+`, AVG(cpu), CAST(ROUND(AVG(mem)) AS INTEGER), MAX(players), MIN(disk_free), 1
		FROM metric_samples
		WHERE hourly = 0 AND at < ?
		GROUP BY instance_id, `+bucket, cutoff)
	if err != nil {
		return fmt.Errorf("downsample metric samples: aggregate: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM metric_samples WHERE hourly = 0 AND at < ?`, cutoff); err != nil {
		return fmt.Errorf("downsample metric samples: delete raw rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("downsample metric samples: commit: %w", err)
	}
	return nil
}

// Prune permanently deletes every row (raw or hourly) older than olderThan.
func (r *MetricSamplesRepo) Prune(ctx context.Context, olderThan time.Time) error {
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM metric_samples WHERE at < ?`, nowString(olderThan)); err != nil {
		return fmt.Errorf("prune metric samples: %w", err)
	}
	return nil
}

func scanMetricSample(scan func(...any) error) (domain.MetricSample, error) {
	var (
		s        domain.MetricSample
		instance sql.NullString
		at       string
		hourly   int
	)
	if err := scan(&instance, &at, &s.CPU, &s.Mem, &s.Players, &s.DiskFree, &hourly); err != nil {
		return domain.MetricSample{}, err
	}
	if instance.Valid {
		v := instance.String
		s.InstanceID = &v
	}
	s.At = parseTime(at)
	s.Hourly = hourly != 0
	return s, nil
}
