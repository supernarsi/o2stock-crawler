package repositories

import (
	"context"
	"strings"
	"time"

	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NBAGamePlayerRepository struct {
	baseRepository[entity.NBAGamePlayer]
}

func NewNBAGamePlayerRepository(db *gorm.DB) *NBAGamePlayerRepository {
	return &NBAGamePlayerRepository{
		baseRepository: baseRepository[entity.NBAGamePlayer]{db: db},
	}
}

// BatchUpsert 批量插入/更新候选球员（按 game_date + nba_player_id 去重）
func (r *NBAGamePlayerRepository) BatchUpsert(ctx context.Context, players []entity.NBAGamePlayer) error {
	if len(players) == 0 {
		return nil
	}
	return r.ctx(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "game_date"},
				{Name: "nba_player_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"match_id", "nba_team_id", "player_name", "player_en_name",
				"team_name", "is_home", "salary", "combat_power", "position",
			}),
		}).
		CreateInBatches(players, 100).Error
}

// ReplaceByGameDate 全量替换某日候选球员数据（先删后插）
func (r *NBAGamePlayerRepository) ReplaceByGameDate(ctx context.Context, gameDate string, players []entity.NBAGamePlayer) error {
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("game_date = ?", gameDate).Delete(&entity.NBAGamePlayer{}).Error; err != nil {
			return err
		}
		if len(players) == 0 {
			return nil
		}
		return tx.CreateInBatches(players, 100).Error
	})
}

// GetByGameDate 获取指定日期所有候选球员
func (r *NBAGamePlayerRepository) GetByGameDate(ctx context.Context, gameDate string) ([]entity.NBAGamePlayer, error) {
	var players []entity.NBAGamePlayer
	err := r.ctx(ctx).
		Where("game_date = ?", gameDate).
		Find(&players).Error
	return players, err
}

// UpdatePredictedPower 更新球员预测战力值
func (r *NBAGamePlayerRepository) UpdatePredictedPower(ctx context.Context, id uint, power float64) error {
	return r.model(ctx).
		Where("id = ?", id).
		Update("predicted_power", power).Error
}

// GetByNBAPlayerIDs 根据 NBA 球员 ID 批量获取候选球员
func (r *NBAGamePlayerRepository) GetByNBAPlayerIDs(ctx context.Context, gameDate string, nbaPlayerIDs []uint) ([]entity.NBAGamePlayer, error) {
	var players []entity.NBAGamePlayer
	err := r.ctx(ctx).
		Where("game_date = ? AND nba_player_id IN ?", gameDate, nbaPlayerIDs).
		Find(&players).Error
	return players, err
}

// ListLatestSalaryProfiles 从历史候选池中构建每个球员的最新固定画像。
// team/name/position 使用最近一条记录；combat_power 优先取最近一条非 0 记录，避免伤停日将基础战力覆盖为 0。
func (r *NBAGamePlayerRepository) ListLatestSalaryProfiles(ctx context.Context) ([]entity.NBAPlayerSalary, error) {
	const query = `
WITH latest_profile AS (
	SELECT
		game_date,
		nba_player_id,
		nba_team_id,
		player_name,
		player_en_name,
		team_name,
		salary,
		combat_power,
		position,
		ROW_NUMBER() OVER (
			PARTITION BY nba_player_id
			ORDER BY game_date DESC, id DESC
		) AS rn
	FROM nba_game_player
),
latest_non_zero_power AS (
	SELECT
		nba_player_id,
		combat_power,
		ROW_NUMBER() OVER (
			PARTITION BY nba_player_id
			ORDER BY game_date DESC, id DESC
		) AS rn
	FROM nba_game_player
	WHERE combat_power > 0
)
SELECT
	lp.nba_player_id,
	lp.nba_team_id,
	lp.player_name,
	lp.player_en_name,
	lp.team_name,
	lp.salary,
	COALESCE(nzp.combat_power, lp.combat_power) AS combat_power,
	lp.position,
	DATE_FORMAT(lp.game_date, '%Y-%m-%d') AS source_game_date
FROM latest_profile lp
LEFT JOIN latest_non_zero_power nzp
	ON nzp.nba_player_id = lp.nba_player_id
	AND nzp.rn = 1
WHERE lp.rn = 1
ORDER BY lp.nba_player_id ASC
`

	var rows []entity.NBAPlayerSalary
	err := r.ctx(ctx).Raw(query).Scan(&rows).Error
	for i := range rows {
		rows[i].SourceGameDate = normalizeSalarySourceGameDate(rows[i].SourceGameDate)
	}
	return rows, err
}

func normalizeSalarySourceGameDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if len(value) >= 10 {
		prefix := value[:10]
		if _, err := time.Parse("2006-01-02", prefix); err == nil {
			return prefix
		}
	}

	if dt, err := time.Parse(time.RFC3339, value); err == nil {
		return dt.Format("2006-01-02")
	}
	if dt, err := time.Parse("2006-01-02", value); err == nil {
		return dt.Format("2006-01-02")
	}

	if len(value) > 10 {
		return value[:10]
	}
	return value
}
