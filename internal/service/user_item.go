package service

import (
	"context"
	"fmt"
	"o2stock-crawler/api"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/dto"
	"time"

	"gorm.io/gorm"
)

// UserItemService 用户道具服务
type UserItemService struct {
	db *db.DB
}

func NewUserItemService(database *db.DB) *UserItemService {
	return &UserItemService{db: database}
}

// ItemIn 标记购买道具，notifyType: 0 不订阅 1 回本 2 盈利15%
func (s *UserItemService) ItemIn(ctx context.Context, userID, itemID, num, cost uint, dt time.Time, notifyType uint8) error {
	if notifyType > 2 {
		notifyType = consts.NotifyTypeNone
	}
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	if err := ownRepo.Create(ctx, userID, itemID, num, cost, dt, notifyType, consts.OwnGoodsItem); err != nil {
		return fmt.Errorf("failed to insert item own: %w", err)
	}
	return nil
}

// ItemOut 标记出售道具（指定持仓记录 ownID）
func (s *UserItemService) ItemOut(ctx context.Context, userID, ownID, itemID, cost uint, dt time.Time) error {
	ownRepo := repositories.NewOwnRepository(s.db.DB)

	own, err := ownRepo.GetByRecordID(ctx, ownID, userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("own record not found")
		}
		return fmt.Errorf("failed to get item own: %w", err)
	}
	if own.PID != itemID {
		return fmt.Errorf("own record item_id mismatch")
	}
	if own.Sta != consts.OwnStaPurchased || own.SellTime != nil {
		return fmt.Errorf("own record not sellable")
	}

	affected, err := ownRepo.MarkAsSoldByID(ctx, userID, ownID, cost, dt)
	if err != nil {
		return fmt.Errorf("failed to update item own to sold: %w", err)
	}
	if affected == 0 {
		// 并发下可能已经卖出
		return fmt.Errorf("own record not sellable")
	}
	return nil
}

// SetItemNotify 修改用户对某道具的订阅类型（仅更新 own_sta=1 且未出售的记录，同球员逻辑）
func (s *UserItemService) SetItemNotify(ctx context.Context, userID, itemID uint, notifyType uint8) error {
	if notifyType > 2 {
		return fmt.Errorf("invalid notify_type")
	}
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	n, err := ownRepo.UpdateNotifyByUserAndGoods(ctx, userID, itemID, notifyType, consts.OwnGoodsItem)
	if err != nil {
		return fmt.Errorf("failed to update notify: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("未找到可修改的持仓记录")
	}
	return nil
}

// GetUserItems 获取用户拥有道具列表
func (s *UserItemService) GetUserItems(ctx context.Context, userID uint) ([]api.OwnedItem, error) {
	ownRepo := repositories.NewOwnRepository(s.db.DB)
	itemRepo := repositories.NewItemRepository(s.db.DB)

	ownedList, err := ownRepo.GetByUserID(ctx, userID, consts.OwnGoodsItem)
	if err != nil {
		return nil, fmt.Errorf("failed to get user owned items: %w", err)
	}
	if len(ownedList) == 0 {
		return []api.OwnedItem{}, nil
	}

	itemIDs := make([]uint, 0, len(ownedList))
	seen := make(map[uint]struct{})
	for _, o := range ownedList {
		if _, ok := seen[o.PID]; ok {
			continue
		}
		seen[o.PID] = struct{}{}
		itemIDs = append(itemIDs, o.PID)
	}

	items, err := itemRepo.BatchGetByItemIDs(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get items by item_ids: %w", err)
	}

	itemsSvc := NewItemsService(s.db)
	itemDTOByItemID := make(map[uint]dto.Item, len(items))
	for i := range items {
		e := &items[i]
		itemDTO := itemsSvc.itemToDTO(e)
		itemDTO.Owned = []*dto.OwnInfo{} // 避免循环嵌套；roster 已包含本条 own
		itemDTOByItemID[e.ItemID] = itemDTO
	}

	rosters := make([]api.OwnedItem, 0, len(ownedList))
	for _, o := range ownedList {
		it, ok := itemDTOByItemID[o.PID]
		if !ok {
			// item 可能已下架/不存在，跳过（与球员实现一致：找不到则 continue）
			continue
		}
		notifyType := o.NotifyType
		if o.Sta == int(consts.OwnStaNone) {
			notifyType = consts.NotifyTypeNone
		}
		rosters = append(rosters, api.OwnedItem{
			Id:         o.ID,
			ItemID:     o.PID,
			PriceIn:    o.BuyPrice,
			PriceOut:   o.SellPrice,
			OwnSta:     uint8(o.Sta),
			OwnNum:     o.BuyCount,
			DtIn:       o.BuyTime.Format("2006-01-02 15:04:05"),
			DtOut:      formatTimeOrEmpty(o.SellTime),
			NotifyType: notifyType,
			Item:       it,
		})
	}

	return rosters, nil
}

// FavItem 用户收藏道具
func (s *UserItemService) FavItem(ctx context.Context, userID, itemID uint) error {
	favRepo := repositories.NewItemFavRepository(s.db.DB)

	// 检查是否已收藏
	count, err := favRepo.Count(ctx, userID, itemID)
	if err != nil {
		return fmt.Errorf("failed to count fav item: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("already fav this item")
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
	if err := favRepo.Add(ctx, userID, itemID); err != nil {
		return fmt.Errorf("failed to insert fav item: %w", err)
	}

	return nil
}

// UnFavItem 用户取消收藏道具
func (s *UserItemService) UnFavItem(ctx context.Context, userID, itemID uint) error {
	favRepo := repositories.NewItemFavRepository(s.db.DB)
	if err := favRepo.Remove(ctx, userID, itemID); err != nil {
		return fmt.Errorf("failed to delete fav item: %w", err)
	}
	return nil
}

// GetUserFavItems 获取用户收藏道具列表（含持仓信息）
func (s *UserItemService) GetUserFavItems(ctx context.Context, userID uint) ([]dto.Item, error) {
	favRepo := repositories.NewItemFavRepository(s.db.DB)
	itemRepo := repositories.NewItemRepository(s.db.DB)
	ownRepo := repositories.NewOwnRepository(s.db.DB)

	// 1. 获取用户收藏的道具ID列表
	itemIDs, err := favRepo.GetItemIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fav item ids: %w", err)
	}

	if len(itemIDs) == 0 {
		return []dto.Item{}, nil
	}

	// 2. 获取道具详细信息
	items, err := itemRepo.BatchGetByItemIDs(ctx, itemIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get items: %w", err)
	}

	// 3. 获取拥有信息
	ownRecords, err := ownRepo.GetByGoodsIDs(ctx, userID, itemIDs, consts.OwnGoodsItem)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned info: %w", err)
	}

	// 4. 构建映射
	ownedMap := ToOwnInfoDTOMap(ownRecords)

	itemsSvc := NewItemsService(s.db)

	// 5. 构建响应
	result := make([]dto.Item, 0, len(items))
	for i := range items {
		it := &items[i]
		itemDTO := itemsSvc.itemToDTO(it)
		itemDTO.IsFav = true
		if owned, ok := ownedMap[it.ItemID]; ok {
			for j := range owned {
				owned[j].OwnGoods = consts.OwnGoodsItem
				itemDTO.Owned = append(itemDTO.Owned, &owned[j])
			}
		}
		result = append(result, itemDTO)
	}

	return result, nil
}
