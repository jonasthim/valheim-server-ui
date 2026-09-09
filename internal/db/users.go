package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Users is the repository for the users and user_identities tables.
type Users struct{ DB *sql.DB }

// NewUsers constructs a Users repository.
func NewUsers(db *sql.DB) *Users { return &Users{DB: db} }

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// NewUser holds the fields needed to insert a user row. PasswordHash is nil
// for OIDC-only accounts.
type NewUser struct {
	Username     string
	DisplayName  string
	Email        string
	PasswordHash *string
	Role         domain.Role
}

// Create inserts a new user (username lowercased by the caller) and returns
// the stored row.
func (u *Users) Create(ctx context.Context, in NewUser) (*domain.User, error) {
	now := nowString(time.Now())
	res, err := u.DB.ExecContext(ctx, `
		INSERT INTO users (username, display_name, email, password_hash, role, disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		in.Username, in.DisplayName, in.Email, in.PasswordHash, string(in.Role), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.Ef(domain.CodeConflict, "username %q already exists", in.Username)
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return u.Get(ctx, id)
}

const userColumns = `id, username, display_name, email, password_hash, role, disabled, created_at, updated_at, last_login_at`

func (u *Users) scanUser(row interface {
	Scan(dest ...any) error
}) (*domain.User, error) {
	var (
		usr          domain.User
		passwordHash sql.NullString
		disabled     int
		createdAt    string
		updatedAt    string
		lastLogin    sql.NullString
	)
	if err := row.Scan(&usr.ID, &usr.Username, &usr.DisplayName, &usr.Email, &passwordHash,
		&usr.Role, &disabled, &createdAt, &updatedAt, &lastLogin); err != nil {
		return nil, err
	}
	usr.Disabled = disabled != 0
	usr.CreatedAt = parseTime(createdAt)
	usr.LastLoginAt = nullTime(lastLogin)
	if passwordHash.Valid && passwordHash.String != "" {
		usr.HasPassword = true
		usr.PasswordHash = passwordHash.String
	}
	return &usr, nil
}

// Get loads a user by id, including linked identities.
func (u *Users) Get(ctx context.Context, id int64) (*domain.User, error) {
	row := u.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	usr, err := u.scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFound("user")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	if usr.Identities, err = u.ListIdentities(ctx, id); err != nil {
		return nil, err
	}
	return usr, nil
}

// GetByUsername loads a user by (lowercase) username.
func (u *Users) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := u.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, strings.ToLower(username))
	usr, err := u.scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFound("user")
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	if usr.Identities, err = u.ListIdentities(ctx, usr.ID); err != nil {
		return nil, err
	}
	return usr, nil
}

// List returns every user ordered by id, including identities.
func (u *Users) List(ctx context.Context) ([]domain.User, error) {
	rows, err := u.DB.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.User
	for rows.Next() {
		usr, err := u.scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, *usr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ids, err := u.ListIdentities(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Identities = ids
	}
	return out, nil
}

// UserUpdate carries optional fields; nil means "leave unchanged".
type UserUpdate struct {
	DisplayName *string
	Email       *string
	Role        *domain.Role
	Disabled    *bool
}

// Update applies a partial update and returns the resulting row.
func (u *Users) Update(ctx context.Context, id int64, in UserUpdate) (*domain.User, error) {
	sets := []string{"updated_at = ?"}
	args := []any{nowString(time.Now())}
	if in.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *in.DisplayName)
	}
	if in.Email != nil {
		sets = append(sets, "email = ?")
		args = append(args, *in.Email)
	}
	if in.Role != nil {
		sets = append(sets, "role = ?")
		args = append(args, string(*in.Role))
	}
	if in.Disabled != nil {
		sets = append(sets, "disabled = ?")
		args = append(args, boolToInt(*in.Disabled))
	}
	args = append(args, id)
	//nolint:gosec // sets is built only from fixed column-name literals above, never from user input; all values are bound params.
	q := fmt.Sprintf(`UPDATE users SET %s WHERE id = ?`, strings.Join(sets, ", "))
	res, err := u.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, domain.NotFound("user")
	}
	return u.Get(ctx, id)
}

// Delete removes a user (cascades to identities and sessions).
func (u *Users) Delete(ctx context.Context, id int64) error {
	res, err := u.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("user")
	}
	return nil
}

// SetPasswordHash sets or clears (hash == nil) the stored password hash.
func (u *Users) SetPasswordHash(ctx context.Context, id int64, hash *string) error {
	res, err := u.DB.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, nowString(time.Now()), id)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("user")
	}
	return nil
}

// UpdateLastLogin stamps last_login_at.
func (u *Users) UpdateLastLogin(ctx context.Context, id int64, when time.Time) error {
	_, err := u.DB.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, nowString(when), id)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

// Count returns the total number of users.
func (u *Users) Count(ctx context.Context) (int, error) {
	var n int
	if err := u.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// CountEnabledAdmins returns the number of non-disabled admins, used for the
// last-admin protection.
func (u *Users) CountEnabledAdmins(ctx context.Context) (int, error) {
	var n int
	err := u.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0`, string(domain.RoleAdmin)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

// ListIdentities returns the OIDC identities linked to a user.
func (u *Users) ListIdentities(ctx context.Context, userID int64) ([]domain.Identity, error) {
	rows, err := u.DB.QueryContext(ctx, `SELECT provider, subject FROM user_identities WHERE user_id = ? ORDER BY provider, subject`, userID)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.Identity{}
	for rows.Next() {
		var id domain.Identity
		if err := rows.Scan(&id.Provider, &id.Subject); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FindByIdentity resolves a user by (issuer, subject).
func (u *Users) FindByIdentity(ctx context.Context, provider, subject string) (*domain.User, error) {
	var userID int64
	err := u.DB.QueryRowContext(ctx, `SELECT user_id FROM user_identities WHERE provider = ? AND subject = ?`,
		provider, subject).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NotFound("identity")
		}
		return nil, fmt.Errorf("find identity: %w", err)
	}
	return u.Get(ctx, userID)
}

// AddIdentity links an (issuer, subject) pair to a user.
func (u *Users) AddIdentity(ctx context.Context, userID int64, provider, subject string) error {
	_, err := u.DB.ExecContext(ctx,
		`INSERT INTO user_identities (provider, subject, user_id, created_at) VALUES (?, ?, ?, ?)`,
		provider, subject, userID, nowString(time.Now()))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.E(domain.CodeConflict, "identity already linked")
		}
		return fmt.Errorf("add identity: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
