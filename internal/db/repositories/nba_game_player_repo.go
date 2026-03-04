package repositories

import (
	"context"
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
