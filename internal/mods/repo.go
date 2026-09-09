package mods

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// modRow is the persisted shape of one mods table row
// (internal/db/migrations/00001_init.sql). Files/Deps are stored as JSON
// arrays; files are paths relative to the instance's server dir.
type modRow struct {
	ID          int64
	InstanceID  string
	Source      domain.ModSource
	Owner       string
	Name        string
	Version     string
	Enabled     bool
	Files       []string
	Deps        []string
	WebsiteURL  string
	IconURL     string
	InstalledAt time.Time
	UpdatedAt   *time.Time
}

func (r modRow) fullNameLower() string { return lowerFullName(r.Owner, r.Name) }

const modColumns = `id, instance_id, source, owner, name, version, enabled, files_json, deps_json, website_url, icon_url, installed_at, updated_at`

func scanModRow(scan func(...any) error) (modRow, error) {
	var (
		r                   modRow
		source              string
		enabled             int
		filesJSON, depsJSON string
		installedAt         string
		updatedAt           sql.NullString
	)
	if err := scan(&r.ID, &r.InstanceID, &source, &r.Owner, &r.Name, &r.Version, &enabled,
		&filesJSON, &depsJSON, &r.WebsiteURL, &r.IconURL, &installedAt, &updatedAt); err != nil {
		return modRow{}, err
	}
	r.Source = domain.ModSource(source)
	r.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(filesJSON), &r.Files); err != nil {
		return modRow{}, fmt.Errorf("decode files_json for mod %d: %w", r.ID, err)
	}
	if err := json.Unmarshal([]byte(depsJSON), &r.Deps); err != nil {
		return modRow{}, fmt.Errorf("decode deps_json for mod %d: %w", r.ID, err)
	}
	if t, err := time.Parse(time.RFC3339, installedAt); err == nil {
		r.InstalledAt = t
	}
	if updatedAt.Valid && updatedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
			r.UpdatedAt = &t
		}
	}
	return r, nil
}

func (s *Service) listModRows(ctx context.Context, instanceID string) ([]modRow, error) {
	q := `SELECT ` + modColumns + ` FROM mods WHERE instance_id = ? ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list mods: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []modRow
	for rows.Next() {
		r, err := scanModRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan mod: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list mods: %w", err)
	}
	return out, nil
}

func (s *Service) getModRow(ctx context.Context, instanceID string, id int64) (modRow, error) {
	q := `SELECT ` + modColumns + ` FROM mods WHERE instance_id = ? AND id = ?`
	r, err := scanModRow(s.db.QueryRowContext(ctx, q, instanceID, id).Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return modRow{}, domain.NotFound("mod")
		}
		return modRow{}, fmt.Errorf("query mod %d: %w", id, err)
	}
	return r, nil
}

func (s *Service) getModByFullName(ctx context.Context, instanceID, owner, name string) (*modRow, error) {
	q := `SELECT ` + modColumns + ` FROM mods WHERE instance_id = ? AND lower(owner) = lower(?) AND lower(name) = lower(?)`
	r, err := scanModRow(s.db.QueryRowContext(ctx, q, instanceID, owner, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query mod %s-%s: %w", owner, name, err)
	}
	return &r, nil
}

func (s *Service) insertModRow(ctx context.Context, r modRow) (int64, error) {
	filesJSON, depsJSON, err := marshalFilesDeps(r)
	if err != nil {
		return 0, err
	}
	q := `INSERT INTO mods (instance_id, source, owner, name, version, enabled, files_json, deps_json, website_url, icon_url, installed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, q, r.InstanceID, string(r.Source), r.Owner, r.Name, r.Version, boolToInt(r.Enabled),
		filesJSON, depsJSON, r.WebsiteURL, r.IconURL, formatTime(r.InstalledAt), nullableTime(r.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("insert mod %s-%s: %w", r.Owner, r.Name, err)
	}
	return res.LastInsertId()
}

func (s *Service) saveModRow(ctx context.Context, r modRow) error {
	filesJSON, depsJSON, err := marshalFilesDeps(r)
	if err != nil {
		return err
	}
	q := `UPDATE mods SET source = ?, version = ?, enabled = ?, files_json = ?, deps_json = ?, website_url = ?, icon_url = ?, updated_at = ?
		WHERE instance_id = ? AND id = ?`
	res, err := s.db.ExecContext(ctx, q, string(r.Source), r.Version, boolToInt(r.Enabled), filesJSON, depsJSON, r.WebsiteURL, r.IconURL,
		nullableTime(r.UpdatedAt), r.InstanceID, r.ID)
	if err != nil {
		return fmt.Errorf("update mod %d: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("mod")
	}
	return nil
}

func (s *Service) deleteModRow(ctx context.Context, instanceID string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mods WHERE instance_id = ? AND id = ?`, instanceID, id)
	if err != nil {
		return fmt.Errorf("delete mod %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NotFound("mod")
	}
	return nil
}

func marshalFilesDeps(r modRow) (filesJSON, depsJSON string, err error) {
	files := r.Files
	if files == nil {
		files = []string{}
	}
	deps := r.Deps
	if deps == nil {
		deps = []string{}
	}
	fb, err := json.Marshal(files)
	if err != nil {
		return "", "", fmt.Errorf("encode files: %w", err)
	}
	db, err := json.Marshal(deps)
	if err != nil {
		return "", "", fmt.Errorf("encode deps: %w", err)
	}
	return string(fb), string(db), nil
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

// toDomain converts a modRow (plus Thunderstore-derived latest-version info)
// to the API-facing domain.Mod.
func (r modRow) toDomain(latestVersion string) domain.Mod {
	m := domain.Mod{
		ID:            r.ID,
		InstanceID:    r.InstanceID,
		Source:        r.Source,
		Owner:         r.Owner,
		Name:          r.Name,
		Version:       r.Version,
		Enabled:       r.Enabled,
		LatestVersion: latestVersion,
		Dependencies:  r.Deps,
		IconURL:       r.IconURL,
		WebsiteURL:    r.WebsiteURL,
		Files:         r.Files,
		InstalledAt:   r.InstalledAt,
		UpdatedAt:     r.UpdatedAt,
	}
	if latestVersion != "" && compareVersions(latestVersion, r.Version) > 0 {
		m.UpdateAvailable = true
	}
	return m
}
