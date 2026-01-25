package db

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"

	"gorm.io/gorm"
)

// PlayerStatsQuery 球员统计数据查询
type PlayerStatsQuery struct {
	repo        *repositories.StatsRepository
	nbaPlayerID uint
}

// NewPlayerStatsQuery 创建一个 PlayerStatsQuery
func NewPlayerStatsQuery(database *DB, nbaPlayerID uint) *PlayerStatsQuery {
	return &PlayerStatsQuery{
		repo:        repositories.NewStatsRepository(database.DB),
		nbaPlayerID: nbaPlayerID,
	}
}

// GetSeasonStats 查询球员最近赛季平均数据
func (q *PlayerStatsQuery) GetSeasonStats(ctx context.Context) (*models.PlayerSeasonStats, error) {
	stats, err := q.repo.GetSeasonStats(ctx, q.nbaPlayerID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return stats, nil
}

// GetRecentGameStats 查询球员最近几场比赛数据
func (q *PlayerStatsQuery) GetRecentGameStats(ctx context.Context, limit int) ([]models.PlayerGameStats, error) {
	return q.repo.GetRecentGameStats(ctx, q.nbaPlayerID, limit)
}
