package repositories

import (
	"context"

	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NBAPlayerSalaryRepository struct {
	baseRepository[entity.NBAPlayerSalary]
}

func NewNBAPlayerSalaryRepository(db *gorm.DB) *NBAPlayerSalaryRepository {
	return &NBAPlayerSalaryRepository{
		baseRepository: baseRepository[entity.NBAPlayerSalary]{db: db},
	}
}

func (r *NBAPlayerSalaryRepository) BatchUpsert(ctx context.Context, rows []entity.NBAPlayerSalary) error {
	if len(rows) == 0 {
		return nil
	}

	return r.ctx(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "nba_player_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"nba_team_id",
				"player_name",
				"player_en_name",
				"team_name",
				"salary",
				"combat_power",
				"position",
				"source_game_date",
				"updated_at",
			}),
		}).
		CreateInBatches(rows, 100).Error
}

func (r *NBAPlayerSalaryRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.model(ctx).Count(&count).Error
	return count, err
}

func (r *NBAPlayerSalaryRepository) GetByTeamIDs(
	ctx context.Context,
	teamIDs []string,
) ([]entity.NBAPlayerSalary, error) {
	var rows []entity.NBAPlayerSalary
	if len(teamIDs) == 0 {
		return rows, nil
	}

	err := r.ctx(ctx).
		Where("nba_team_id IN ?", teamIDs).
		Order("salary DESC, nba_player_id ASC").
		Find(&rows).Error
	return rows, err
}

// BatchGetByNBAPlayerIDs 按 nba_player_id 批量获取球员映射（仅返回 tx_player_id > 0 的记录）。
func (r *NBAPlayerSalaryRepository) BatchGetByNBAPlayerIDs(
	ctx context.Context,
	nbaPlayerIDs []uint,
) ([]entity.NBAPlayerSalary, error) {
	var rows []entity.NBAPlayerSalary
	if len(nbaPlayerIDs) == 0 {
		return rows, nil
	}

	err := r.ctx(ctx).
		Select("nba_player_id", "tx_player_id").
		Where("nba_player_id IN ? AND tx_player_id > 0", nbaPlayerIDs).
		Find(&rows).Error
	return rows, err
}

// GetByNBAPlayerIDs 按 nba_player_id 批量获取球员完整信息（API 展示用）。
func (r *NBAPlayerSalaryRepository) GetByNBAPlayerIDs(
	ctx context.Context,
	nbaPlayerIDs []uint,
) ([]entity.NBAPlayerSalary, error) {
	var rows []entity.NBAPlayerSalary
	if len(nbaPlayerIDs) == 0 {
		return rows, nil
	}
	err := r.ctx(ctx).
		Where("nba_player_id IN ?", nbaPlayerIDs).
		Find(&rows).Error
	return rows, err
}

// UpdateCombatPowerFromRecentStats 根据 player_game_stats 更新 combat_power（最近 10 场有效比赛场均战力）
func (r *NBAPlayerSalaryRepository) UpdateCombatPowerFromRecentStats(ctx context.Context) error {
	query := `
		UPDATE nba_player_salary s
		INNER JOIN (
			SELECT
				tx_player_id,
				COUNT(1) AS valid_games,
				SUM(power) AS total_power
			FROM (
				SELECT
					tx_player_id,
					(points + 1.2*rebounds + 1.5*assists + 3*steals + 3*blocks - turnovers) AS power,
					ROW_NUMBER() OVER (PARTITION BY tx_player_id ORDER BY game_date DESC, id DESC) AS rn
				FROM player_game_stats
				WHERE minutes > 0
			) recent
			WHERE rn <= 10
			GROUP BY tx_player_id
		) stats ON s.tx_player_id = stats.tx_player_id
		SET s.combat_power = ROUND(stats.total_power / stats.valid_games, 1)
	`
	return r.ctx(ctx).Exec(query).Error
}

// SyncTxPlayerIDFromPlayers 从 players 表同步 tx_player_id 到 nba_player_salary 表
func (r *NBAPlayerSalaryRepository) SyncTxPlayerIDFromPlayers(ctx context.Context) error {
	query := `
		UPDATE nba_player_salary s
		INNER JOIN players p ON s.nba_player_id = p.nba_player_id
		SET s.tx_player_id = p.tx_player_id
		WHERE s.tx_player_id = 0 AND p.tx_player_id > 0
	`
	return r.ctx(ctx).Exec(query).Error
}
