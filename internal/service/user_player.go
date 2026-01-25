package service

import (
	"context"
	"fmt"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
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
	query := db.NewUserPlayerOwnQuery(s.db, userID)
	count, err := query.CountOwnedPlayers(ctx, s.db, playerID)
	if err != nil {
		return fmt.Errorf("failed to count owned players: %w", err)
	}
	if count >= 2 {
		return fmt.Errorf("already owned more than 2 players")
	}

	// 插入购买记录
	cmd := db.NewUserPlayerOwnCommand(s.db)
	if err := cmd.InsertPlayerOwn(ctx, s.db, userID, playerID, num, cost, dt); err != nil {
		return fmt.Errorf("failed to insert player own: %w", err)
	}

	return nil
}

// PlayerOut 标记出售球员
func (s *UserPlayerService) PlayerOut(ctx context.Context, userID, playerID, cost uint, dt time.Time) error {
	// 更新为已出售状态
	cmd := db.NewUserPlayerOwnCommand(s.db)
	if err := cmd.UpdatePlayerOwnToSold(ctx, s.db, userID, playerID, cost, dt); err != nil {
		return fmt.Errorf("failed to update player own to sold: %w", err)
	}

	return nil
}

// EditPlayerOwn 修改持仓记录
func (s *UserPlayerService) EditPlayerOwn(ctx context.Context, userID, recordId, priceIn, priceOut, num uint, dtIn, dtOut *time.Time) error {
	// 更新持仓记录
	cmd := db.NewUserPlayerOwnCommand(s.db)
	if err := cmd.UpdatePlayerOwn(ctx, s.db, userID, recordId, priceIn, priceOut, num, dtIn, dtOut); err != nil {
		return fmt.Errorf("failed to update player own: %w", err)
	}
	return nil
}

// DeletePlayerOwn 删除持仓记录
func (s *UserPlayerService) DeletePlayerOwn(ctx context.Context, userID, recordId uint) error {
	// 删除持仓记录
	cmd := db.NewUserPlayerOwnCommand(s.db)
	if err := cmd.DeletePlayerOwn(ctx, s.db, userID, recordId); err != nil {
		return fmt.Errorf("failed to delete player own: %w", err)
	}
	return nil
}

// GetPlayerOwn 获取持仓记录
func (s *UserPlayerService) GetPlayerOwn(ctx context.Context, userID, recordId uint) (*model.UserPlayerOwn, error) {
	// 获取持仓记录
	query := db.NewUserPlayerOwnQuery(s.db, userID)
	record, err := query.GetPlayerOwnByRecordID(ctx, s.db, recordId, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player own: %w", err)
	}
	return record, nil
}

// GetUserPlayers 获取用户拥有球员列表
func (s *UserPlayerService) GetUserPlayers(ctx context.Context, userID uint) ([]api.OwnedPlayer, error) {
	// 获取用户拥有的球员记录
	query := db.NewUserPlayerOwnQuery(s.db, userID)
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
	playersQuery := db.NewPlayersQuery(s.db, repositories.PlayerFilter{})
	players, err := playersQuery.GetPlayersByIDs(ctx, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	// 构建响应
	playerMap := make(map[uint]models.Player)
	for _, p := range players {
		playerMap[p.PlayerID] = p
	}

	rosters := make([]api.OwnedPlayer, 0, len(ownedList))
	for _, o := range ownedList {
		pp, ok := playerMap[o.PlayerID]
		if !ok {
			continue // 跳过找不到球员信息的记录
		}
		rosters = append(rosters, api.OwnedPlayer{
			Id:       o.ID,
			PlayerID: o.PlayerID,
			PriceIn:  o.PriceIn,
			PriceOut: o.PriceOut,
			OwnSta:   o.OwnSta,
			OwnNum:   o.NumIn,
			DtIn:     o.DtIn.Format("2006-01-02 15:04:05"),
			DtOut:    formatTimeOrEmpty(o.DtOut),
			PP: model.Players{
				PlayerID:          pp.PlayerID,
				NBAPlayerID:       pp.NBAPlayerID,
				ShowName:          pp.ShowName,
				EnName:            pp.EnName,
				TeamAbbr:          pp.TeamAbbr,
				Version:           pp.Version,
				CardType:          pp.CardType,
				PlayerImg:         pp.PlayerImg,
				PriceStandard:     pp.PriceStandard,
				PriceCurrentLower: pp.PriceCurrentLowest,
				PriceSaleLower:    pp.PriceSaleLower,
				PriceSaleUpper:    pp.PriceSaleUpper,
				OverAll:           pp.OverAll,
				PowerPer5:         pp.PowerPer5,
				PowerPer10:        pp.PowerPer10,
				PriceChange1d:     pp.PriceChange1d,
				PriceChange7d:     pp.PriceChange7d,
			},
		})
	}

	return rosters, nil
}

// GetUserFavPlayers 获取用户收藏球员列表
func (s *UserPlayerService) GetUserFavPlayers(ctx context.Context, userID uint) ([]api.PlayerWithOwned, error) {
	// 1. 获取用户收藏的球员ID列表
	favQuery := db.NewFavQuery(s.db)
	favIDs, err := favQuery.GetFavPlayerIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fav player ids: %w", err)
	}

	if len(favIDs) == 0 {
		return []api.PlayerWithOwned{}, nil
	}

	// 2. 获取球员详细信息
	playersQuery := db.NewPlayersQuery(s.db, repositories.PlayerFilter{PlayerIDs: favIDs})
	players, err := playersQuery.ListPlayers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}

	// 3. 获取拥有信息
	ownedQuery := db.NewUserPlayerOwnQuery(s.db, userID)
	ownedMap, err := ownedQuery.GetOwnedInfoByPlayerIDs(ctx, s.db, favIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned info: %w", err)
	}

	// 4. 构建响应
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: p,
			Owned:                 []*model.OwnInfo{},
			IsFav:                 true, // 既然是收藏列表，肯定都是已收藏
		}
		if ownedMap != nil {
			if owned, ok := ownedMap[p.PlayerID]; ok {
				for j := range owned {
					result[i].Owned = append(result[i].Owned, &owned[j])
				}
			}
		}
	}

	return result, nil
}

// FavPlayer 用户收藏球员
func (s *UserPlayerService) FavPlayer(ctx context.Context, userID, playerID uint) error {
	// 检查是否已收藏
	favQuery := db.NewFavQuery(s.db)
	count, err := favQuery.CountFavPlayer(ctx, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to count fav player: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("already fav this player")
	}

	// 检查收藏数量是否已达上限 (50)
	totalFavs, err := favQuery.CountUserFavs(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to count user favs: %w", err)
	}
	if totalFavs >= 50 {
		return fmt.Errorf("fav limit exceeded (max 50)")
	}

	// 插入收藏记录
	favCmd := db.NewFavCommand(s.db)
	if err := favCmd.InsertFavPlayer(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to insert fav player: %w", err)
	}

	return nil
}

// UnFavPlayer 用户取消收藏球员
func (s *UserPlayerService) UnFavPlayer(ctx context.Context, userID, playerID uint) error {
	// 删除收藏记录
	favCmd := db.NewFavCommand(s.db)
	if err := favCmd.DeleteFavPlayer(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to delete fav player: %w", err)
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
