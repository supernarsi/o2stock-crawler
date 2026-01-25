package repositories

import (
	"context"
	"o2stock-crawler/internal/db/models"

	"gorm.io/gorm"
)

type StatsRepository struct {
	db *gorm.DB
}

func NewStatsRepository(db *gorm.DB) *StatsRepository {
	return &StatsRepository{db: db}
}

func (r *StatsRepository) GetSeasonStats(ctx context.Context, nbaPlayerID uint) (*models.PlayerSeasonStats, error) {
	var stats models.PlayerSeasonStats
	err := r.db.WithContext(ctx).
		Where("player_id = ?", nbaPlayerID).
		Order("season DESC, season_type DESC").
		First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *StatsRepository) GetRecentGameStats(ctx context.Context, nbaPlayerID uint, limit int) ([]models.PlayerGameStats, error) {
	var stats []models.PlayerGameStats
	err := r.db.WithContext(ctx).
		Where("player_id = ?", nbaPlayerID).
		Order("game_date DESC").
		Limit(limit).
		Find(&stats).Error
	return stats, err
}
