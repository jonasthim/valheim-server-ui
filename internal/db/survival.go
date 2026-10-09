package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type SurvivalRepo struct{ DB *sql.DB }

func NewSurvivalRepo(db *sql.DB) *SurvivalRepo { return &SurvivalRepo{DB: db} }

func (r *SurvivalRepo) Insert(ctx context.Context, m domain.SurvivalMoment) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO survival_moments
		(instance_id,world_uid,world_name,run_id,source_seq,kind,character_id,player_name,day,at,label,enemy,enemy_level,situation,biome,x,z,visible)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(instance_id,run_id,source_seq) DO NOTHING`,
		m.InstanceID, m.WorldUID, m.WorldName, m.RunID, m.SourceSeq, m.Kind, m.CharacterID, m.PlayerName, m.Day, nowString(m.At), m.Label, m.Enemy, m.EnemyLevel, m.Situation, m.Biome, nullFloat(m.X), nullFloat(m.Z), m.Visible)
	if err != nil {
		return fmt.Errorf("insert survival moment: %w", err)
	}
	return nil
}

func (r *SurvivalRepo) List(ctx context.Context, instanceID string, worldUID int64, characterID string) ([]domain.SurvivalMoment, error) {
	q := `SELECT id,instance_id,world_uid,world_name,kind,character_id,player_name,day,at,label,enemy,enemy_level,situation,biome,x,z,visible
		FROM survival_moments WHERE instance_id=? AND world_uid=?`
	args := []any{instanceID, worldUID}
	if characterID != "" {
		q += ` AND (character_id=? OR character_id='')`
		args = append(args, characterID)
	}
	q += ` ORDER BY day,id`
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list survival moments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.SurvivalMoment
	for rows.Next() {
		var m domain.SurvivalMoment
		var at string
		var x, z sql.NullFloat64
		if err := rows.Scan(&m.ID, &m.InstanceID, &m.WorldUID, &m.WorldName, &m.Kind, &m.CharacterID, &m.PlayerName, &m.Day, &at, &m.Label, &m.Enemy, &m.EnemyLevel, &m.Situation, &m.Biome, &x, &z, &m.Visible); err != nil {
			return nil, err
		}
		m.At = parseTime(at)
		if x.Valid {
			m.X = &x.Float64
		}
		if z.Valid {
			m.Z = &z.Float64
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SurvivalRepo) Worlds(ctx context.Context, instanceID string) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT world_uid FROM survival_moments WHERE instance_id=? GROUP BY world_uid ORDER BY MAX(id) DESC`, instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
