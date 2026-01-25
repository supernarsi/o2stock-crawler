package db

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/model"
)

const (
	// OrderByPriceStandard 按标准价格排序
	OrderByPriceStandard = "price_standard"
	// OrderByPriceChange 按涨跌幅排序
	OrderByPriceChange = "price_change_1d"
	// OrderByPlayerID 按球员ID排序（默认）
	OrderByPlayerID = "player_id"
	// OrderByPowerPer5 按近5场战力值排序
	OrderByPowerPer5 = "power_per5"
	// OrderByPowerPer10 按近10场战力值排序
	OrderByPowerPer10 = "power_per10"
	// OrderByOverAll 按球员能力值排序
	OrderByOverAll = "over_all"
)

const (
	// Period1Day 24小时数据涨跌幅
	Period1Day uint8 = 1
	// Period3Days 3天数据涨跌幅
	Period3Days uint8 = 2
	// Period1Week 1周数据涨跌幅
	Period1Week uint8 = 3
)

// PlayersQuery 获取球员列表
type PlayersQuery struct {
	repo   *repositories.PlayerRepository
	filter repositories.PlayerFilter
}

// NewPlayersQuery 创建一个 PlayersQuery
func NewPlayersQuery(database *DB, filter repositories.PlayerFilter) *PlayersQuery {
	// 限制排序字段
	validOrderBy := map[string]string{
		"price_change":       "price_change_1d",
		OrderByPriceStandard: OrderByPriceStandard,
		OrderByPowerPer5:     OrderByPowerPer5,
		OrderByPowerPer10:    OrderByPowerPer10,
		OrderByOverAll:       OrderByOverAll,
	}

	if filter.OrderBy == "price_change" && filter.Limit == 0 { // Just an example of how to handle period
		// This needs more thought on how to pass Period to repository
	}

	if mapped, ok := validOrderBy[filter.OrderBy]; ok {
		filter.OrderBy = mapped
	} else {
		filter.OrderBy = OrderByPlayerID
	}

	return &PlayersQuery{
		repo:   repositories.NewPlayerRepository(database.DB),
		filter: filter,
	}
}

// GetAllPlayers 获取所有球员基础信息（无价格过滤）
func (s *PlayersQuery) GetAllPlayers(ctx context.Context) ([]models.Player, error) {
	return s.repo.List(ctx, repositories.PlayerFilter{Limit: -1})
}

// ListPlayers 返回简单的球员列表
func (s *PlayersQuery) ListPlayers(ctx context.Context) ([]model.PlayerWithPriceChange, error) {
	players, err := s.repo.List(ctx, s.filter)
	if err != nil {
		return nil, err
	}

	result := make([]model.PlayerWithPriceChange, len(players))
	for i, p := range players {
		changeRatio := p.PriceChange1d
		// Note: Filter should probably handle this mapped field
		result[i] = model.PlayerWithPriceChange{
			Players: model.Players{
				PlayerID:          p.PlayerID,
				NBAPlayerID:       p.NBAPlayerID,
				ShowName:          p.ShowName,
				EnName:            p.EnName,
				TeamAbbr:          p.TeamAbbr,
				Version:           p.Version,
				CardType:          p.CardType,
				PlayerImg:         p.PlayerImg,
				PriceStandard:     p.PriceStandard,
				PriceCurrentLower: p.PriceCurrentLowest,
				PriceSaleLower:    p.PriceSaleLower,
				PriceSaleUpper:    p.PriceSaleUpper,
				OverAll:           p.OverAll,
				PowerPer5:         p.PowerPer5,
				PowerPer10:        p.PowerPer10,
				PriceChange1d:     p.PriceChange1d,
				PriceChange7d:     p.PriceChange7d,
			},
			PriceChange: changeRatio,
		}
	}
	return result, nil
}

// GetPlayerInfo 获取单个球员信息
func (s *PlayersQuery) GetPlayerInfo(ctx context.Context, playerID uint) (*models.Player, error) {
	return s.repo.GetByID(ctx, playerID)
}

// GetPlayersByIDs 根据球员 ID 列表获取球员信息
func (s *PlayersQuery) GetPlayersByIDs(ctx context.Context, playerIDs []uint) ([]models.Player, error) {
	return s.repo.BatchGetByIDs(ctx, playerIDs)
}

// UpdatePlayerPriceChanges 更新球员涨跌幅
func (s *PlayersQuery) UpdatePlayerPriceChanges(ctx context.Context, playerID uint, pc1d, pc7d float64) error {
	return s.repo.UpdatePriceChanges(ctx, playerID, pc1d, pc7d)
}

// UpdatePlayerPower 更新球员战力值
func (s *PlayersQuery) UpdatePlayerPower(ctx context.Context, playerID uint, power5, power10 float64) error {
	return s.repo.UpdatePower(ctx, playerID, power5, power10)
}

// GetAllTargetPlayers 获取需要更新战力值的球员
func (s *PlayersQuery) GetAllTargetPlayers(ctx context.Context) ([]models.Player, error) {
	return s.repo.GetAllTargetPlayers(ctx)
}
