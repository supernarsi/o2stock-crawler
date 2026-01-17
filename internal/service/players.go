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

// PlayerListOptions 封装球员列表查询参数
type PlayerListOptions struct {
	Page       int
	Limit      int
	OrderBy    string
	OrderAsc   bool
	Period     uint8
	UserID     *uint
	SoldOut    bool
	PlayerName string
}

// ListPlayersWithOwned 获取球员列表，支持分页、排序，并可选地包含用户的拥有信息
func (s *PlayersService) ListPlayersWithOwned(ctx context.Context, opts PlayerListOptions) ([]api.PlayerWithOwned, error) {
	// 参数校验与默认值
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	filter := db.PlayerFilter{
		Page:       opts.Page,
		Limit:      opts.Limit,
		OrderBy:    opts.OrderBy,
		OrderAsc:   opts.OrderAsc,
		Period:     opts.Period,
		SoldOut:    opts.SoldOut,
		PlayerName: opts.PlayerName,
	}

	query := db.NewPlayersQuery(filter)
	players, ownedMap, err := query.ListPlayersWithOwned(ctx, s.db, opts.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list players with owned: %w", err)
	}

	var favMap map[uint]bool
	if opts.UserID != nil && len(players) > 0 {
		pids := make([]uint, len(players))
		for i, p := range players {
			pids[i] = p.PlayerID
		}
		favMap, err = db.GetFavMapByPlayerIDs(ctx, s.db, *opts.UserID, pids)
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
	}

	// 构建返回结果，总是包含 owned 字段
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: *p,
			Owned:                 []*model.OwnInfo{}, // 默认为空数组
			IsFav:                 false,
		}
		// 如果有拥有信息，填充到结果中
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				result[i].Owned = owned
			}
		}
		// 如果有收藏信息，填充到结果中
		if favMap != nil {
			if isFav, ok := favMap[p.PlayerID]; ok {
				result[i].IsFav = isFav
			}
		}
	}

	return result, nil
}

// GetPlayerHistory 获取单个球员历史价格
func (s *PlayersService) GetPlayerHistory(ctx context.Context, playerID uint32, period uint8, limit int) ([]*model.PriceHistoryRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := db.NewPlayerHistoryQuery(playerID, limit)
	rows, err := query.GetPlayerHistory(ctx, s.db, period)
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

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
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

// GetPlayerInfo 获取单个球员信息
func (s *PlayersService) GetPlayerInfo(ctx context.Context, playerID uint, userID *uint) (*api.PlayerWithOwned, error) {
	query := db.NewPlayersQuery(db.PlayerFilter{Page: 1, Limit: 1, OrderAsc: true})
	pp, err := query.GetPlayerInfo(ctx, s.db, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player info: %w", err)
	}

	isFav := false
	owned := []*model.OwnInfo{}
	if userID != nil {
		// 查询已拥有的球员
		ownedMap, err := db.NewUserPlayerOwnQuery(*userID).GetOwnedInfoByPlayerIDs(ctx, s.db, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get owned info: %w", err)
		}
		if ownedR, ok := ownedMap[playerID]; ok {
			owned = ownedR
		}

		// 查询已收藏的球员
		favMap, err := db.GetFavMapByPlayerIDs(ctx, s.db, *userID, []uint{playerID})
		if err != nil {
			return nil, fmt.Errorf("failed to get fav info: %w", err)
		}
		if isFavR, ok := favMap[playerID]; ok {
			isFav = isFavR
		}
	}

	return &api.PlayerWithOwned{
		PlayerWithPriceChange: model.PlayerWithPriceChange{Players: *pp},
		Owned:                 owned,
		IsFav:                 isFav,
	}, nil
}

// GetPlayerHistoryRealtime 获取分时数据（当天所有成交记录）
func (s *PlayersService) GetPlayerHistoryRealtime(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.GetPlayerHistoryRealtime(ctx, s.db, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history realtime: %w", err)
	}
	return rows, nil
}

// GetPlayerHistory5Days 获取五日数据（最近5个自然日的所有成交记录）
func (s *PlayersService) GetPlayerHistory5Days(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.GetPlayerHistory5Days(ctx, s.db, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history 5days: %w", err)
	}
	return rows, nil
}

// GetPlayerHistoryDailyK 获取日K线数据（最近30个自然日的K线数据）
func (s *PlayersService) GetPlayerHistoryDailyK(ctx context.Context, playerID uint32) ([]*model.PriceHistoryRow, error) {
	rows, err := db.GetPlayerHistoryDailyK(ctx, s.db, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player history dailyk: %w", err)
	}
	return rows, nil
}
