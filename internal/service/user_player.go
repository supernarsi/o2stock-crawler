package service

import (
	"context"
	"fmt"
	"log"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

// UserPlayerService 用户球员服务
type UserPlayerService struct {
	db         *db.DB
	ownRepo    *repositories.OwnRepository
	playerRepo *repositories.PlayerRepository
	itemRepo   *repositories.ItemRepository
	favRepo    *repositories.FavRepository
}

// NewUserPlayerService 创建用户球员服务实例
func NewUserPlayerService(database *db.DB, ownRepo *repositories.OwnRepository, playerRepo *repositories.PlayerRepository, itemRepo *repositories.ItemRepository, favRepo *repositories.FavRepository) *UserPlayerService {
	return &UserPlayerService{
		db:         database,
		ownRepo:    ownRepo,
		playerRepo: playerRepo,
		itemRepo:   itemRepo,
		favRepo:    favRepo,
	}
}

// PlayerIn 标记购买球员，notifyType: 0 不订阅 1 回本 2 盈利15%
func (s *UserPlayerService) PlayerIn(ctx context.Context, userID, playerID, num, cost uint, dt time.Time, notifyType uint8) error {

	// 检查是否已拥有超过 2 条
	count, err := s.ownRepo.CountOwned(ctx, userID, playerID, consts.OwnGoodsPlayer)
	if err != nil {
		return fmt.Errorf("failed to count owned players: %w", err)
	}
	if count >= 2 {
		return fmt.Errorf("already owned more than 2 players")
	}

	if notifyType > 2 {
		notifyType = consts.NotifyTypeNone
	}
	// 插入购买记录
	if err := s.ownRepo.Create(ctx, userID, playerID, num, cost, dt, notifyType, consts.OwnGoodsPlayer); err != nil {
		return fmt.Errorf("failed to insert player own: %w", err)
	}

	return nil
}

// PlayerOut 标记出售球员
func (s *UserPlayerService) PlayerOut(ctx context.Context, userID, playerID, cost uint, dt time.Time) error {
	if err := s.ownRepo.MarkAsSold(ctx, userID, playerID, cost, dt, consts.OwnGoodsPlayer); err != nil {
		return fmt.Errorf("failed to update player own to sold: %w", err)
	}
	return nil
}

// EditPlayerOwn 修改持仓记录
func (s *UserPlayerService) EditPlayerOwn(ctx context.Context, userID, recordId, priceIn, priceOut, num uint, dtIn, dtOut *time.Time) error {
	updates := map[string]interface{}{
		"price_in":  priceIn,
		"price_out": priceOut,
		"num_in":    num,
		"dt_in":     dtIn,
	}
	if dtOut != nil {
		updates["dt_out"] = dtOut
	}
	if err := s.ownRepo.Update(ctx, userID, recordId, updates); err != nil {
		return fmt.Errorf("failed to update player own: %w", err)
	}
	return nil
}

// DeletePlayerOwn 删除持仓记录
func (s *UserPlayerService) DeletePlayerOwn(ctx context.Context, userID, recordId uint) error {
	if err := s.ownRepo.Delete(ctx, userID, recordId); err != nil {
		return fmt.Errorf("failed to delete player own: %w", err)
	}
	return nil
}

// GetPlayerOwn 获取持仓记录
func (s *UserPlayerService) GetPlayerOwn(ctx context.Context, userID, recordId uint) (*dto.UserPlayerOwn, error) {
	record, err := s.ownRepo.GetByRecordID(ctx, recordId, userID)
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
	ownedList, err := s.ownRepo.GetByUserID(ctx, userID, consts.OwnGoodsPlayer)
	if err != nil {
		return nil, fmt.Errorf("failed to get user owned players: %w", err)
	}

	if len(ownedList) == 0 {
		return []api.OwnedPlayer{}, nil
	}

	// 获取球员 ID 列表
	playerIDs := make([]uint, len(ownedList))
	for i, o := range ownedList {
		playerIDs[i] = o.PID
	}

	// 查询球员详细信息
	players, err := s.playerRepo.BatchGetByIDs(ctx, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get players by IDs: %w", err)
	}

	// 获取球员扩展信息
	var badgeMap map[uint]*dto.Badges
	extras, err := s.playerRepo.GetExtraByPlayerIDs(ctx, playerIDs)
	if err != nil {
		log.Printf("failed to get player extra info: %v", err)
	} else {
		badgeMap = make(map[uint]*dto.Badges)
		for _, e := range extras {
			badgeMap[e.PlayerID] = ToBadgesDTO(&e)
		}
	}

	// 构建响应
	playerMap := make(map[uint]entity.Player)
	for _, p := range players {
		playerMap[p.PlayerID] = p
	}

	rosters := make([]api.OwnedPlayer, 0, len(ownedList))
	for _, o := range ownedList {
		pp, ok := playerMap[o.PID]
		if !ok {
			continue
		}
		var badges *dto.Badges
		if badgeMap != nil {
			badges = badgeMap[o.PID]
		}
		notifyType := o.NotifyType
		if o.Sta == int(consts.OwnStaNone) {
			notifyType = consts.NotifyTypeNone
		}
		rosters = append(rosters, api.OwnedPlayer{
			Id:         o.ID,
			PlayerID:   o.PID,
			PriceIn:    o.BuyPrice,
			PriceOut:   o.SellPrice,
			OwnSta:     uint8(o.Sta),
			OwnNum:     o.BuyCount,
			DtIn:       o.BuyTime.Format("2006-01-02 15:04:05"),
			DtOut:      formatTimeOrEmpty(o.SellTime),
			NotifyType: notifyType,
			PP:         ToPlayerDTO(pp, badges),
		})
	}

	return rosters, nil
}

// GetUserFavPlayers 获取用户收藏球员列表
func (s *UserPlayerService) GetUserFavPlayers(ctx context.Context, userID uint) ([]api.PlayerWithOwned, error) {
	// 1. 获取用户收藏的球员ID列表
	favIDs, err := s.favRepo.GetPlayerIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fav player ids: %w", err)
	}

	if len(favIDs) == 0 {
		return []api.PlayerWithOwned{}, nil
	}

	// 2. 获取球员详细信息
	players, err := s.playerRepo.List(ctx, repositories.PlayerFilter{PlayerIDs: favIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}

	// 3. 获取拥有信息
	ownRecords, err := s.ownRepo.GetByGoodsIDs(ctx, userID, favIDs, consts.OwnGoodsPlayer)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned info: %w", err)
	}
	ownedMap := ToOwnInfoDTOMap(ownRecords)

	// 4. 获取球员扩展信息
	var badgeMap map[uint]*dto.Badges
	extras, err := s.playerRepo.GetExtraByPlayerIDs(ctx, favIDs)
	if err != nil {
		log.Printf("failed to get player extra info: %v", err)
	} else {
		badgeMap = make(map[uint]*dto.Badges)
		for _, e := range extras {
			badgeMap[e.PlayerID] = ToBadgesDTO(&e)
		}
	}

	// 5. 构建响应
	result := make([]api.PlayerWithOwned, len(players))
	for i, p := range players {
		var badges *dto.Badges
		if badgeMap != nil {
			badges = badgeMap[p.PlayerID]
		}
		result[i] = api.PlayerWithOwned{
			PlayerWithPriceChange: ToPlayerWithPriceChangeDTO(p, badges),
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
	// 检查是否已收藏
	count, err := s.favRepo.Count(ctx, userID, playerID)
	if err != nil {
		return fmt.Errorf("failed to count fav player: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("already fav this player")
	}

	// 检查收藏数量是否已达上限 (50)
	totalFavs, err := s.favRepo.CountUserTotal(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to count user favs: %w", err)
	}
	if totalFavs >= 50 {
		return fmt.Errorf("fav limit exceeded (max 50)")
	}

	// 插入收藏记录
	if err := s.favRepo.Add(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to insert fav player: %w", err)
	}

	return nil
}

// UnFavPlayer 用户取消收藏球员
func (s *UserPlayerService) UnFavPlayer(ctx context.Context, userID, playerID uint) error {
	if err := s.favRepo.Remove(ctx, userID, playerID); err != nil {
		return fmt.Errorf("failed to delete fav player: %w", err)
	}
	return nil
}

// SetPlayerNotify 修改用户对某球员的订阅类型（仅更新 own_sta=1 且未出售的记录）
func (s *UserPlayerService) SetPlayerNotify(ctx context.Context, userID, playerID uint, notifyType uint8) error {
	if notifyType > 2 {
		return fmt.Errorf("invalid notify_type")
	}
	n, err := s.ownRepo.UpdateNotifyByUserAndGoods(ctx, userID, playerID, notifyType, consts.OwnGoodsPlayer)
	if err != nil {
		return fmt.Errorf("failed to update notify: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("未找到可修改的持仓记录")
	}
	return nil
}

// Helper methods
func (s *UserPlayerService) entityToOwnDTO(o *entity.UserPOwn) *dto.UserPlayerOwn {
	return &dto.UserPlayerOwn{
		ID:         o.ID,
		UserID:     o.UserID,
		PlayerID:   o.PID,
		OwnSta:     uint8(o.Sta),
		PriceIn:    o.BuyPrice,
		PriceOut:   o.SellPrice,
		NumIn:      o.BuyCount,
		DtIn:       o.BuyTime,
		DtOut:      o.SellTime,
		NotifyType: o.NotifyType,
	}
}

func formatTimeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// GetUnifiedOwnGoods 获取统一持仓列表
func (s *UserPlayerService) GetUnifiedOwnGoods(ctx context.Context, userID uint) ([]dto.UnifiedOwnGoods, error) {
	// 1. 获取用户所有持仓记录
	allOwns, err := s.ownRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unified owns: %w", err)
	}

	if len(allOwns) == 0 {
		return []dto.UnifiedOwnGoods{}, nil
	}

	// 2. 分类 ID
	playerIDs := []uint{}
	itemIDs := []uint{}
	for _, o := range allOwns {
		switch o.OwnGoods {
		case consts.OwnGoodsPlayer:
			playerIDs = append(playerIDs, o.PID)
		case consts.OwnGoodsItem:
			itemIDs = append(itemIDs, o.PID)
		}
	}

	// 3. 批量获取详情
	playerMap := make(map[uint]entity.Player)
	if len(playerIDs) > 0 {
		players, err := s.playerRepo.BatchGetByIDs(ctx, playerIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch get players: %w", err)
		}
		for _, p := range players {
			playerMap[p.PlayerID] = p
		}
	}

	itemMap := make(map[uint]entity.Item)
	if len(itemIDs) > 0 {
		items, err := s.itemRepo.BatchGetByItemIDs(ctx, itemIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to batch get items: %w", err)
		}
		for _, it := range items {
			itemMap[it.ItemID] = it
		}
	}

	// 4. 组装
	result := make([]dto.UnifiedOwnGoods, 0, len(allOwns))
	for _, o := range allOwns {
		res := dto.UnifiedOwnGoods{
			OwnID:    o.ID,
			OwnGoods: o.OwnGoods,
			GoodsID:  o.PID,
			PriceIn:  o.BuyPrice,
			PriceOut: o.SellPrice,
			OwnSta:   uint8(o.Sta),
			OwnNum:   o.BuyCount,
			DtIn:     o.BuyTime.Format("2006-01-02 15:04:05"),
		}
		if o.SellTime != nil {
			res.DtOut = o.SellTime.Format("2006-01-02 15:04:05")
		}

		if o.OwnGoods == consts.OwnGoodsPlayer {
			if p, ok := playerMap[o.PID]; ok {
				res.GoodsName = p.ShowName
				res.GoodsNameEn = p.EnName
				res.GoodsImg = p.PlayerImg
				res.PriceStandard = p.PriceStandard
				res.PriceCurrentLowest = p.PriceCurrentLowest
			} else {
				continue // 找不到球员详情，跳过
			}
		} else if o.OwnGoods == consts.OwnGoodsItem {
			if it, ok := itemMap[o.PID]; ok {
				res.GoodsName = it.Name
				res.GoodsNameEn = "" // 道具没有英文名
				res.GoodsImg = it.Icon
				res.PriceStandard = it.PriceStandard
				res.PriceCurrentLowest = it.PriceCurrentLowest
			} else {
				continue // 找不到道具详情，跳过
			}
		}
		result = append(result, res)
	}

	return result, nil
}
