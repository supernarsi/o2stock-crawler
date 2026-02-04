package service

import (
	"context"
	"fmt"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

// UserPlayerService 用户球员服务
type UserPlayerService struct {
	db *db.DB
}

// NewUserPlayerService 创建用户球员服务实例
func NewUserPlayerService(database *db.DB) *UserPlayerService {
	return &UserPlayerService{db: database}
}

// PlayerIn 标记购买球员，notifyType: 0 不订阅 1 回本 2 盈利15%
func (s *UserPlayerService) PlayerIn(ctx context.Context, userID, playerID, num, cost uint, dt time.Time, notifyType uint8) error {
	ownRepo := repositories.NewOwnRepository(s.db.DB)

	// 检查是否已拥有超过 2 条
	count, err := ownRepo.CountOwned(ctx, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to count owned players: %w", err)
	}
	if count >= 2 {
		return fmt.Errorf("already owned more than 2 players")
	}

	if notifyType > 2 {
		notifyType = 0
	}
	// 插入购买记录
	if err := ownRepo.Create(ctx, userID, playerID, num, cost, dt, notifyType); err != nil {
		return fmt.Errorf("failed to insert player own: %w", err)
	}

	return nil
}

// PlayerOut 标记出售球员
func (s *UserPlayerService) PlayerOut(ctx context.Context, userID, playerID, cost uint, dt time.Time) error {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	if err := ownRepo.MarkAsSold(ctx, userID, playerID, cost, dt); err != nil {
		return fmt.Errorf("failed to update player own to sold: %w", err)
	}
	return nil
}

// EditPlayerOwn 修改持仓记录
func (s *UserPlayerService) EditPlayerOwn(ctx context.Context, userID, recordId, priceIn, priceOut, num uint, dtIn, dtOut *time.Time) error {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	updates := map[string]interface{}{
		"price_in":  priceIn,
		"price_out": priceOut,
		"num_in":    num,
		"dt_in":     dtIn,
	}
	if dtOut != nil {
		updates["dt_out"] = dtOut
	}
	if err := ownRepo.Update(ctx, userID, recordId, updates); err != nil {
		return fmt.Errorf("failed to update player own: %w", err)
	}
	return nil
}

// DeletePlayerOwn 删除持仓记录
func (s *UserPlayerService) DeletePlayerOwn(ctx context.Context, userID, recordId uint) error {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	if err := ownRepo.Delete(ctx, userID, recordId); err != nil {
		return fmt.Errorf("failed to delete player own: %w", err)
	}
	return nil
}

// GetPlayerOwn 获取持仓记录
func (s *UserPlayerService) GetPlayerOwn(ctx context.Context, userID, recordId uint) (*dto.UserPlayerOwn, error) {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	record, err := ownRepo.GetByRecordID(ctx, recordId, userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get player own: %w", err)
	}
	return s.entityToOwnDTO(record), nil
}

// GetUserPlayers 获取用户拥有球员列表
func (s *UserPlayerService) GetUserPlayers(ctx context.Context, userID uint) ([]api.OwnedPlayer, error) {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	playerRepo := repositories.NewPlayerRepository(s.db.DB)

	ownedList, err := ownRepo.GetByUserID(ctx, userID)
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
	players, err := playerRepo.BatchGetByIDs(ctx, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	// 构建响应
	playerMap := make(map[uint]entity.Player)
	for _, p := range players {
		playerMap[p.PlayerID] = p
	}

	rosters := make([]api.OwnedPlayer, 0, len(ownedList))
	for _, o := range ownedList {
		pp, ok := playerMap[o.PlayerID]
		if !ok {
			continue
		}
		notifyType := o.NotifyType
		if o.Sta == 0 {
			notifyType = 0
		}
		rosters = append(rosters, api.OwnedPlayer{
			Id:         o.ID,
			PlayerID:   o.PlayerID,
			PriceIn:    o.BuyPrice,
			PriceOut:   o.SellPrice,
			OwnSta:     uint8(o.Sta),
			OwnNum:     o.BuyCount,
			DtIn:       o.BuyTime.Format("2006-01-02 15:04:05"),
			DtOut:      formatTimeOrEmpty(o.SellTime),
			NotifyType: notifyType,
			PP:         ToPlayerDTO(pp),
		})
	}

	return rosters, nil
}

// GetUserFavPlayers 获取用户收藏球员列表
func (s *UserPlayerService) GetUserFavPlayers(ctx context.Context, userID uint) ([]api.PlayerWithOwned, error) {
	favRepo := repositories.NewFavRepository(s.db.DB)
	playerRepo := repositories.NewPlayerRepository(s.db.DB)
	ownRepo := repositories.NewOwnRepository(s.db.DB)

	// 1. 获取用户收藏的球员ID列表
	favIDs, err := favRepo.GetPlayerIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fav player ids: %w", err)
	}

	if len(favIDs) == 0 {
		return []api.PlayerWithOwned{}, nil
	}

	// 2. 获取球员详细信息
	players, err := playerRepo.List(ctx, repositories.PlayerFilter{PlayerIDs: favIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}

	// 3. 获取拥有信息
	ownRecords, err := ownRepo.GetByPlayerIDs(ctx, userID, favIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned info: %w", err)
	}
	ownedMap := s.mapOwnRecordsToInfoMap(ownRecords)

	// 4. 构建响应
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: ToPlayerWithPriceChangeDTO(p),
			Owned:                 []*dto.OwnInfo{},
			IsFav:                 true,
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
	favRepo := repositories.NewFavRepository(s.db.DB)

	// 检查是否已收藏
	count, err := favRepo.Count(ctx, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to count fav player: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("already fav this player")
	}

	// 检查收藏数量是否已达上限 (50)
	totalFavs, err := favRepo.CountUserTotal(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to count user favs: %w", err)
	}
	if totalFavs >= 50 {
		return fmt.Errorf("fav limit exceeded (max 50)")
	}

	// 插入收藏记录
	if err := favRepo.Add(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to insert fav player: %w", err)
	}

	return nil
}

// UnFavPlayer 用户取消收藏球员
func (s *UserPlayerService) UnFavPlayer(ctx context.Context, userID, playerID uint) error {
	favRepo := repositories.NewFavRepository(s.db.DB)
	if err := favRepo.Remove(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to delete fav player: %w", err)
	}
	return nil
}

// SetPlayerNotify 修改用户对某球员的订阅类型（仅更新 own_sta=1 且未出售的记录）
func (s *UserPlayerService) SetPlayerNotify(ctx context.Context, userID, playerID uint, notifyType uint8) error {
	if notifyType > 2 {
		return fmt.Errorf("invalid notify_type")
	}
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	n, err := ownRepo.UpdateNotifyByUserAndPlayer(ctx, userID, playerID, notifyType)
	if err != nil {
		return fmt.Errorf("failed to update notify: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("未找到可修改的持仓记录")
	}
	return nil
}

// Helper methods
func (s *UserPlayerService) entityToOwnDTO(o *entity.UserPlayerOwn) *dto.UserPlayerOwn {
	return &dto.UserPlayerOwn{
		ID:         o.ID,
		UserID:     o.UserID,
		PlayerID:   o.PlayerID,
		OwnSta:     uint8(o.Sta),
		PriceIn:    o.BuyPrice,
		PriceOut:   o.SellPrice,
		NumIn:      o.BuyCount,
		DtIn:       o.BuyTime,
		DtOut:      o.SellTime,
		NotifyType: o.NotifyType,
	}
}

func (s *UserPlayerService) mapOwnRecordsToInfoMap(records []entity.UserPlayerOwn) map[uint][]dto.OwnInfo {
	result := make(map[uint][]dto.OwnInfo)
	for _, o := range records {
		dtOut := ""
		if o.SellTime != nil {
			dtOut = o.SellTime.Format("2006-01-02 15:04:05")
		}
		notifyType := o.NotifyType
		if o.Sta == 0 {
			notifyType = 0
		}
		info := dto.OwnInfo{
			PlayerID:   o.PlayerID,
			PriceIn:    o.BuyPrice,
			PriceOut:   o.SellPrice,
			OwnSta:     uint8(o.Sta),
			OwnNum:     o.BuyCount,
			DtIn:       o.BuyTime.Format("2006-01-02 15:04:05"),
			DtOut:      dtOut,
			NotifyType: notifyType,
		}
		result[o.PlayerID] = append(result[o.PlayerID], info)
	}
	return result
}

func formatTimeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
