package players

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// ListComments returns UI-only comments for one instance and list kind.
func (s *SQLStore) ListComments(ctx context.Context, instanceID string, kind domain.ListKind) (map[string]string, error) {
	comments := map[string]string{}
	if s.db == nil {
		return comments, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT platform_id, comment FROM player_list_comments WHERE instance_id=? AND kind=?`, instanceID, kind)
	if err != nil {
		return nil, fmt.Errorf("players: list comments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, comment string
		if err := rows.Scan(&id, &comment); err != nil {
			return nil, fmt.Errorf("players: scan list comment: %w", err)
		}
		comments[id] = comment
	}
	return comments, rows.Err()
}

// ReplaceListComments makes the stored comments match the current list.
func (s *SQLStore) ReplaceListComments(ctx context.Context, instanceID string, kind domain.ListKind, entries []domain.PlayerListEntry) error {
	if s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("players: begin list comment update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_list_comments WHERE instance_id=? AND kind=?`, instanceID, kind); err != nil {
		return fmt.Errorf("players: clear list comments: %w", err)
	}
	for _, entry := range entries {
		if entry.Comment == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO player_list_comments(instance_id,kind,platform_id,comment) VALUES (?,?,?,?)`, instanceID, kind, entry.ID, entry.Comment); err != nil {
			return fmt.Errorf("players: save list comment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("players: commit list comments: %w", err)
	}
	return nil
}
