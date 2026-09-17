package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// ChatLogRepo is the repository for the chat_log table: every in-game chat
// line the agent poller has seen, kept across agent reconnects and world
// restarts (F-2.3).
type ChatLogRepo struct{ DB *sql.DB }

// NewChatLogRepo constructs a ChatLogRepo.
func NewChatLogRepo(db *sql.DB) *ChatLogRepo { return &ChatLogRepo{DB: db} }

// Insert writes one chat_log row and returns its id.
func (r *ChatLogRepo) Insert(ctx context.Context, e domain.ChatLogEntry) (int64, error) {
	res, err := r.DB.ExecContext(ctx, `
		INSERT INTO chat_log (instance_id, at, type, sender, text, x, z, run_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.InstanceID, nowString(e.At), e.Type, e.Sender, e.Text, nullFloat(e.X), nullFloat(e.Z), e.RunSeq)
	if err != nil {
		return 0, fmt.Errorf("insert chat log entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("chat log entry id: %w", err)
	}
	return id, nil
}

// List returns instanceID's chat lines newest-first (id DESC), optionally
// paged with a before-id cursor (0 = no cursor, start from the newest) and
// filtered to lines whose text or sender contains q (case-insensitive
// substring; % and _ in q are matched literally, not as wildcards).
func (r *ChatLogRepo) List(ctx context.Context, instanceID string, limit int, before int64, q string) ([]domain.ChatLogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, instance_id, at, type, sender, text, x, z, run_seq FROM chat_log WHERE instance_id = ?`
	args := []any{instanceID}
	if before > 0 {
		query += ` AND id < ?`
		args = append(args, before)
	}
	if q != "" {
		query += ` AND (text LIKE ? ESCAPE '\' OR sender LIKE ? ESCAPE '\')`
		pattern := "%" + escapeLike(q) + "%"
		args = append(args, pattern, pattern)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list chat log entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ChatLogEntry
	for rows.Next() {
		var (
			e    domain.ChatLogEntry
			at   string
			x, z sql.NullFloat64
		)
		if err := rows.Scan(&e.ID, &e.InstanceID, &at, &e.Type, &e.Sender, &e.Text, &x, &z, &e.RunSeq); err != nil {
			return nil, fmt.Errorf("scan chat log entry: %w", err)
		}
		e.At = parseTime(at)
		if x.Valid {
			v := x.Float64
			e.X = &v
		}
		if z.Valid {
			v := z.Float64
			e.Z = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// nullFloat converts an optional float column value for a bind argument: nil
// stays SQL NULL, otherwise the dereferenced value.
func nullFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// escapeLike escapes the LIKE wildcard characters (and the escape character
// itself) so q is matched as a literal substring via LIKE ... ESCAPE '\'.
func escapeLike(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q)
}
