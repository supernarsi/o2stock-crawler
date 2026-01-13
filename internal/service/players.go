package service

import (
	"context"
	"fmt"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/model"
)

// PlayersService 球员服务
type PlayersService struct {
	db *db.DB
}

// NewPlayersService 创建球员服务实例
func NewPlayersService(database *db.DB) *PlayersService {
	return &PlayersService{db: database}
}

// ListPlayersWithOwned 获取球员列表，支持分页、排序，并可选地包含用户的拥有信息
func (s *PlayersService) ListPlayersWithOwned(ctx context.Context, page, limit int, orderBy string, orderAsc bool, period uint8, userID *uint) ([]api.PlayerWithOwned, error) {
	query := db.NewPlayersQuery(page, limit, orderBy, orderAsc)
	players, ownedMap, err := query.ListPlayersWithOwned(ctx, s.db, period, orderBy, orderAsc, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list players with owned: %w", err)
	}

	// 构建返回结果，总是包含 owned 字段
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: *p,
			Owned:                 []*model.OwnInfo{}, // 默认为空数组
		}
		// 如果有拥有信息，填充到结果中
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				result[i].Owned = owned
			}
		}
	}

	return result, nil
}

// GetPlayerHistory 获取单个球员历史价格
func (s *PlayersService) GetPlayerHistory(ctx context.Context, playerID uint32, limit int) ([]*model.PriceHistoryRow, error) {
	query := db.NewPlayerHistoryQuery(playerID, limit)
	rows, err := query.GetPlayerHistory(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history: %w", err)
	}
	return rows, nil
}

// GetMultiPlayersHistory 批量获取球员历史价格
func (s *PlayersService) GetMultiPlayersHistory(ctx context.Context, playerIDs []uint32, limit int) ([]api.PlayerHistoryItem, error) {
	if len(playerIDs) == 0 {
		return []api.PlayerHistoryItem{}, nil
	}

	// 限制最多查询的球员数量
	if len(playerIDs) > 30 {
		return nil, fmt.Errorf("too many player_ids, maximum 30")
	}

	query := db.NewMultiPlayersHistoryQuery(playerIDs, limit)
	historyMap, err := query.GetMultiPlayersHistory(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get multi players history: %w", err)
	}

	// 将 map 转换为列表形式，保持请求的 player_ids 顺序
	historyList := make([]api.PlayerHistoryItem, 0, len(playerIDs))
	for _, pid := range playerIDs {
		history, ok := historyMap[pid]
		if !ok {
			history = []*model.PriceHistoryRow{} // 如果没有数据，返回空数组
		}
		historyList = append(historyList, api.PlayerHistoryItem{
			PlayerID: pid,
			History:  history,
		})
	}

	return historyList, nil
}
