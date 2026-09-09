package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// backupRow is the persisted shape of one backups table row
// (internal/db/migrations/00001_init.sql).
type backupRow struct {
	ID         int64
	InstanceID string
	World      string
	Kind       domain.BackupKind
	Filename   string
	SizeBytes  int64
	Note       string
	CreatedAt  time.Time
}

func (r backupRow) toDomain() domain.Backup {
	return domain.Backup{
		ID:         r.ID,
		InstanceID: r.InstanceID,
		World:      r.World,
		Kind:       r.Kind,
		Filename:   r.Filename,
		SizeBytes:  r.SizeBytes,
		Note:       r.Note,
		CreatedAt:  r.CreatedAt,
	}
}

const backupColumns = `id, instance_id, world, kind, filename, size_bytes, note, created_at`

func scanBackupRow(scan func(...any) error) (backupRow, error) {
	var (
		r         backupRow
		kind      string
		createdAt string
	)
	if err := scan(&r.ID, &r.InstanceID, &r.World, &kind, &r.Filename, &r.SizeBytes, &r.Note, &createdAt); err != nil {
		return backupRow{}, err
	}
	r.Kind = domain.BackupKind(kind)
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		r.CreatedAt = t
	}
	return r, nil
}

func (s *Service) insertBackupRow(ctx context.Context, r backupRow) (int64, error) {
	q := `INSERT INTO backups (instance_id, world, kind, filename, size_bytes, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, q, r.InstanceID, r.World, string(r.Kind), r.Filename, r.SizeBytes, r.Note, r.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert backup row: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert backup row: %w", err)
	}
	return id, nil
}

func (s *Service) getBackupRow(ctx context.Context, instanceID string, id int64) (backupRow, error) {
	q := `SELECT ` + backupColumns + ` FROM backups WHERE instance_id = ? AND id = ?`
	rs := s.db.QueryRowContext(ctx, q, instanceID, id)
	r, err := scanBackupRow(rs.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return backupRow{}, domain.NotFound("backup")
		}
		return backupRow{}, fmt.Errorf("query backup %d: %w", id, err)
	}
	return r, nil
}

// listBackupRows returns instanceID's rows newest first.
func (s *Service) listBackupRows(ctx context.Context, instanceID string) ([]backupRow, error) {
	q := `SELECT ` + backupColumns + ` FROM backups WHERE instance_id = ? ORDER BY created_at DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, q, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []backupRow
	for rows.Next() {
		r, err := scanBackupRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	return out, nil
}

func (s *Service) deleteBackupRow(ctx context.Context, instanceID string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE instance_id = ? AND id = ?`, instanceID, id)
	if err != nil {
		return fmt.Errorf("delete backup %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("backup")
	}
	return nil
}
