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

func (r *StatsRepository) GetSeasonStats(ctx context.Context, txPlayerID uint) (*entity.PlayerSeasonStats, error) {
	var stats entity.PlayerSeasonStats
	err := r.ctx(ctx).
		Where("tx_player_id = ?", txPlayerID).
		Order("season DESC, season_type DESC").
		First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *StatsRepository) GetRecentGameStats(ctx context.Context, txPlayerID uint, limit int) ([]entity.PlayerGameStats, error) {
	var stats []entity.PlayerGameStats
	err := r.ctx(ctx).Model(&entity.PlayerGameStats{}).
		Where("tx_player_id = ?", txPlayerID).
		Order("game_date DESC").
		Limit(limit).
		Find(&stats).Error
	return stats, err
}

// GetSeasonStatsByTxPlayerIDs 批量查询指定赛季、赛季类型的球员赛季数据
// season 如 "25-26"，seasonType 1=常规赛；若 season 为空则默认 "25-26"，seasonType<=0 则默认 1
func (r *StatsRepository) GetSeasonStatsByTxPlayerIDs(ctx context.Context, txPlayerIDs []uint, season string, seasonType int) (map[uint]*entity.PlayerSeasonStats, error) {
	out := make(map[uint]*entity.PlayerSeasonStats)
	if len(txPlayerIDs) == 0 {
		return out, nil
	}
	if season == "" {
		season = "2025-26"
	}
	if seasonType <= 0 {
		seasonType = 1
	}
	var list []entity.PlayerSeasonStats
	err := r.model(ctx).
		Where("tx_player_id IN ? AND season = ? AND season_type = ?", txPlayerIDs, season, seasonType).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	for i := range list {
		out[list[i].TxPlayerID] = &list[i]
	}
	return out, nil
}
