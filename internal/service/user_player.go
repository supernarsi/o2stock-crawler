package service

import (
	"context"
	"fmt"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/model"
	"time"
)

// UserPlayerService 用户球员服务
type UserPlayerService struct {
	db *db.DB
}

// NewUserPlayerService 创建用户球员服务实例
func NewUserPlayerService(database *db.DB) *UserPlayerService {
	return &UserPlayerService{db: database}
}

// PlayerIn 标记购买球员
func (s *UserPlayerService) PlayerIn(ctx context.Context, userID, playerID, num, cost uint, dt time.Time) error {
	// 检查是否已拥有超过 2 条
	query := db.NewUserPlayerOwnQuery(userID)
	count, err := query.CountOwnedPlayers(ctx, s.db, playerID)
	if err != nil {
		return fmt.Errorf("failed to count owned players: %w", err)
	}
	if count >= 2 {
		return fmt.Errorf("already owned more than 2 players")
	}

	// 插入购买记录
	cmd := db.NewUserPlayerOwnCommand()
	if err := cmd.InsertPlayerOwn(ctx, s.db, userID, playerID, num, cost, dt); err != nil {
		return fmt.Errorf("failed to insert player own: %w", err)
	}

	return nil
}

// PlayerOut 标记出售球员
func (s *UserPlayerService) PlayerOut(ctx context.Context, userID, playerID, cost uint, dt time.Time) error {
	// 更新为已出售状态
	cmd := db.NewUserPlayerOwnCommand()
	if err := cmd.UpdatePlayerOwnToSold(ctx, s.db, userID, playerID, cost, dt); err != nil {
		if err == db.ErrNoRows {
			return fmt.Errorf("not own this player yet")
		}
		return fmt.Errorf("failed to update player own to sold: %w", err)
	}

	return nil
}

// GetUserPlayers 获取用户拥有球员列表
func (s *UserPlayerService) GetUserPlayers(ctx context.Context, userID uint) ([]api.OwnedPlayer, error) {
	// 获取用户拥有的球员记录
	query := db.NewUserPlayerOwnQuery(userID)
	ownedList, err := query.GetUserOwnedPlayers(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get user owned players: %w", err)
	}

	if len(ownedList) == 0 {
		return []api.OwnedPlayer{}, nil
	}

	// 获取球员 ID 列表
	playerIDs := make([]uint, len(ownedList))
	for i, o := range ownedList {
		playerIDs[i] = o.PlayerID
	}

	// 查询球员详细信息
	playersQuery := db.NewPlayersQuery(1, 100, "", true)
	players, err := playersQuery.GetPlayersByIDs(ctx, s.db, playerIDs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	// 构建响应
	playerMap := make(map[uint]*model.Players)
	for _, p := range players {
		playerMap[p.PlayerID] = p
	}

	rosters := make([]api.OwnedPlayer, 0, len(ownedList))
	for _, o := range ownedList {
		pp := playerMap[o.PlayerID]
		if pp == nil {
			continue // 跳过找不到球员信息的记录
		}
		rosters = append(rosters, api.OwnedPlayer{
			PlayerID: o.PlayerID,
			PriceIn:  o.PriceIn,
			PriceOut: o.PriceOut,
			OwnSta:   o.OwnSta,
			OwnNum:   o.NumIn,
			DtIn:     o.DtIn.Format("2006-01-02 15:04:05"),
			DtOut:    formatTimeOrEmpty(o.DtOut),
			PP:       *pp,
		})
	}

	return rosters, nil
}

// GetUserFavPlayers 获取用户收藏球员列表
func (s *UserPlayerService) GetUserFavPlayers(ctx context.Context, userID uint) ([]api.PlayerWithOwned, error) {
	// 1. 获取用户收藏的球员ID列表
	favIDs, err := db.GetFavPlayerIDs(ctx, s.db, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fav player ids: %w", err)
	}

	if len(favIDs) == 0 {
		return []api.PlayerWithOwned{}, nil
	}

	// 2. 获取球员详细信息（包含价格变动）
	// 默认 period = 1 (1天)
	playersQuery := db.NewPlayersQuery(1, 100, "", true)
	players, err := playersQuery.GetPlayersWithPriceChangeByIDs(ctx, s.db, favIDs, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get players with price change: %w", err)
	}

	// 3. 获取拥有信息
	ownedQuery := db.NewUserPlayerOwnQuery(userID)
	ownedMap, err := ownedQuery.GetOwnedInfoByPlayerIDs(ctx, s.db, favIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned info: %w", err)
	}

	// 4. 构建响应
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: *p,
			Owned:                 []*model.OwnInfo{},
			IsFav:                 true, // 既然是收藏列表，肯定都是已收藏
		}
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				result[i].Owned = owned
			}
		}
	}

	return result, nil
}

// FavPlayer 用户收藏球员
func (s *UserPlayerService) FavPlayer(ctx context.Context, userID, playerID uint) error {
	// 检查是否已收藏
	count, err := db.CountFavPlayer(ctx, s.db, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to count fav player: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("already fav this player")
	}

	// 插入收藏记录
	if err := db.InsertFavPlayer(ctx, s.db, userID, playerID); err != nil {
		return fmt.Errorf("failed to insert fav player: %w", err)
	}

	return nil
}

// formatTimeOrEmpty 格式化时间为字符串，如果为 nil 则返回空字符串
func formatTimeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
