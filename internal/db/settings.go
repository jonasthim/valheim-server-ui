package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// SettingsRepo is the repository for the single-row settings table.
type SettingsRepo struct{ DB *sql.DB }

// NewSettingsRepo constructs a SettingsRepo.
func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{DB: db} }

// Get returns the stored settings, or domain.DefaultSettings() when the row
// does not exist yet.
func (r *SettingsRepo) Get(ctx context.Context) (domain.Settings, error) {
	var doc string
	err := r.DB.QueryRowContext(ctx, `SELECT doc FROM settings WHERE id = 1`).Scan(&doc)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DefaultSettings(), nil
		}
		return domain.Settings{}, fmt.Errorf("get settings: %w", err)
	}
	var s domain.Settings
	if err := json.Unmarshal([]byte(doc), &s); err != nil {
		return domain.Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	return s, nil
}

// Put replaces the stored settings document.
func (r *SettingsRepo) Put(ctx context.Context, s domain.Settings) error {
	doc, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	_, err = r.DB.ExecContext(ctx, `
		INSERT INTO settings (id, doc, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET doc = excluded.doc, updated_at = excluded.updated_at`,
		string(doc), nowString(time.Now()))
	if err != nil {
		return fmt.Errorf("put settings: %w", err)
	}
	return nil
}
