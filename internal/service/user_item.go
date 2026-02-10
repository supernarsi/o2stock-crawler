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
	if own.PlayerID != itemID {
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
		if _, ok := seen[o.PlayerID]; ok {
			continue
		}
		seen[o.PlayerID] = struct{}{}
		itemIDs = append(itemIDs, o.PlayerID)
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
		itemDTO.Owned = []*dto.ItemOwnInfo{} // 避免循环嵌套；roster 已包含本条 own
		itemDTOByItemID[e.ItemID] = itemDTO
	}

	rosters := make([]api.OwnedItem, 0, len(ownedList))
	for _, o := range ownedList {
		it, ok := itemDTOByItemID[o.PlayerID]
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
			ItemID:     o.PlayerID,
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
