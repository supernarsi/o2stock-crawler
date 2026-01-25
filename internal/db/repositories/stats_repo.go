package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type StatsRepository struct {
	baseRepository[entity.PlayerSeasonStats]
}

func NewStatsRepository(db *gorm.DB) *StatsRepository {
	return &StatsRepository{
		baseRepository: baseRepository[entity.PlayerSeasonStats]{db: db},
	}
}

func (r *StatsRepository) GetSeasonStats(ctx context.Context, nbaPlayerID uint) (*entity.PlayerSeasonStats, error) {
	var stats entity.PlayerSeasonStats
	err := r.ctx(ctx).
		Where("player_id = ?", nbaPlayerID).
		Order("season DESC, season_type DESC").
		First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *StatsRepository) GetRecentGameStats(ctx context.Context, nbaPlayerID uint, limit int) ([]entity.PlayerGameStats, error) {
	var stats []entity.PlayerGameStats
	err := r.ctx(ctx).Model(&entity.PlayerGameStats{}).
		Where("player_id = ?", nbaPlayerID).
		Order("game_date DESC").
		Limit(limit).
		Find(&stats).Error
	return stats, err
}
