package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type StatsRepository struct {
	baseRepository[entity.PlayerSeasonStats]
}

// TeamGameAggregate 球队单场聚合统计（按 tx_game_id + player_team_name 聚合）。
type TeamGameAggregate struct {
	TxGameID       string    `gorm:"column:tx_game_id"`
	PlayerTeamName string    `gorm:"column:player_team_name"`
	VsTeamName     string    `gorm:"column:vs_team_name"`
	GameDate       time.Time `gorm:"column:game_date"`
	TeamPoints     float64   `gorm:"column:team_points"`
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

// BatchGetRecentGameStats 批量获取多球员近 N 场比赛数据，用于 IPI 批量计算
func (r *StatsRepository) BatchGetRecentGameStats(ctx context.Context, txPlayerIDs []uint, limit int) (map[uint][]entity.PlayerGameStats, error) {
	out := make(map[uint][]entity.PlayerGameStats)
	if len(txPlayerIDs) == 0 || limit <= 0 {
		return out, nil
	}

	// 使用 ROW_NUMBER 取每个球员最近 N 场
	subQuery := r.ctx(ctx).Model(&entity.PlayerGameStats{}).
		Select("*, ROW_NUMBER() OVER (PARTITION BY tx_player_id ORDER BY game_date DESC) AS rn").
		Where("tx_player_id IN ?", txPlayerIDs)

	var results []entity.PlayerGameStats
	err := r.ctx(ctx).
		Table("(?) as t", subQuery).
		Where("rn <= ?", limit).
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	for _, g := range results {
		out[g.TxPlayerID] = append(out[g.TxPlayerID], g)
	}
	return out, nil
}

// BatchGetGameStatsByDate 批量获取指定日期的单场数据（tx_player_id -> game_stats）
func (r *StatsRepository) BatchGetGameStatsByDate(ctx context.Context, txPlayerIDs []uint, gameDate string) (map[uint]entity.PlayerGameStats, error) {
	out := make(map[uint]entity.PlayerGameStats)
	if len(txPlayerIDs) == 0 || gameDate == "" {
		return out, nil
	}

	var rows []entity.PlayerGameStats
	err := r.ctx(ctx).
		Model(&entity.PlayerGameStats{}).
		Where("tx_player_id IN ? AND DATE(game_date) = ?", txPlayerIDs, gameDate).
		Order("game_date DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	// 同一球员若出现多条记录，保留按 game_date DESC 排序后的第一条。
	for _, row := range rows {
		if _, exists := out[row.TxPlayerID]; exists {
			continue
		}
		out[row.TxPlayerID] = row
	}
	return out, nil
}

// GetRecentTeamGameAggregates 获取最近 N 天球队单场聚合得分数据，用于估算 DefRating/Pace。
func (r *StatsRepository) GetRecentTeamGameAggregates(ctx context.Context, lookbackDays int) ([]TeamGameAggregate, error) {
	var rows []TeamGameAggregate
	if lookbackDays <= 0 {
		lookbackDays = 120
	}
	startDate := time.Now().AddDate(0, 0, -lookbackDays)

	err := r.ctx(ctx).
		Model(&entity.PlayerGameStats{}).
		Select("tx_game_id, player_team_name, vs_team_name, MAX(game_date) AS game_date, SUM(points) AS team_points").
		Where("game_date >= ?", startDate).
		Group("tx_game_id, player_team_name, vs_team_name").
		Order("game_date DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
