package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// APITokens is the repository for the api_tokens table (F-2.5): personal
// access tokens used for `Authorization: Bearer vsui_...` requests. Only the
// sha256 hash of the secret is stored; the plaintext never touches the
// database.
type APITokens struct{ DB *sql.DB }

// NewAPITokens constructs an APITokens repository.
func NewAPITokens(db *sql.DB) *APITokens { return &APITokens{DB: db} }

const apiTokenColumns = `id, user_id, name, prefix, created_at, last_used_at, expires_at` //nolint:gosec // SQL column list, not a credential

// scanAPIToken scans one api_tokens row (see apiTokenColumns for the column
// order); it never scans hash, which never needs to leave the database once
// stored.
func scanAPIToken(row interface{ Scan(dest ...any) error }) (userID int64, tok domain.APIToken, err error) {
	var (
		createdAt  string
		lastUsedAt sql.NullString
		expiresAt  sql.NullString
	)
	if err = row.Scan(&tok.ID, &userID, &tok.Name, &tok.Prefix, &createdAt, &lastUsedAt, &expiresAt); err != nil {
		return 0, domain.APIToken{}, err
	}
	tok.CreatedAt = parseTime(createdAt)
	tok.LastUsedAt = nullTime(lastUsedAt)
	tok.ExpiresAt = nullTime(expiresAt)
	return userID, tok, nil
}

// Create inserts a new API token row and returns the stored (secret-free)
// row. expiresAt of nil means the token never expires.
func (a *APITokens) Create(ctx context.Context, userID int64, name, hash, prefix string, expiresAt *time.Time) (domain.APIToken, error) {
	var expiresAtVal any
	if expiresAt != nil {
		expiresAtVal = nowString(*expiresAt)
	}
	res, err := a.DB.ExecContext(ctx, `
		INSERT INTO api_tokens (user_id, name, hash, prefix, created_at, last_used_at, expires_at)
		VALUES (?, ?, ?, ?, ?, NULL, ?)`,
		userID, name, hash, prefix, nowString(time.Now()), expiresAtVal)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.APIToken{}, domain.E(domain.CodeConflict, "token hash collision; try again")
		}
		return domain.APIToken{}, fmt.Errorf("insert api token: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("insert api token: %w", err)
	}
	row := a.DB.QueryRowContext(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE id = ?`, id)
	_, tok, err := scanAPIToken(row)
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("get inserted api token: %w", err)
	}
	return tok, nil
}

// GetByHash loads a token by the sha256 hash of its secret, along with the id
// of the user it belongs to. Used by Authenticate's bearer-token path.
func (a *APITokens) GetByHash(ctx context.Context, hash string) (int64, domain.APIToken, error) {
	row := a.DB.QueryRowContext(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE hash = ?`, hash)
	userID, tok, err := scanAPIToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, domain.APIToken{}, domain.NotFound("api token")
		}
		return 0, domain.APIToken{}, fmt.Errorf("get api token by hash: %w", err)
	}
	return userID, tok, nil
}

// ListByUser returns userID's API tokens, newest first.
func (a *APITokens) ListByUser(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens for user %d: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.APIToken
	for rows.Next() {
		_, tok, err := scanAPIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		out = append(out, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list api tokens for user %d: %w", userID, err)
	}
	return out, nil
}

// Delete removes one of userID's own API tokens. A token that exists but
// belongs to a different user is reported not found, same as one that does
// not exist at all.
func (a *APITokens) Delete(ctx context.Context, userID, id int64) error {
	res, err := a.DB.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("api token")
	}
	return nil
}

// TouchLastUsed stamps last_used_at, called at most once a minute by
// Authenticate's bearer-token path.
func (a *APITokens) TouchLastUsed(ctx context.Context, id int64, at time.Time) error {
	_, err := a.DB.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, nowString(at), id)
	if err != nil {
		return fmt.Errorf("touch api token: %w", err)
	}
	return nil
}
