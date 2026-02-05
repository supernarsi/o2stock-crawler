package service

import (
	"context"
	"log"
	"math"
	"o2stock-crawler/internal/crawler"
	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ItemSnapshotService 处理道具快照与涨跌幅
type ItemSnapshotService struct {
	db *db.DB
}

// NewItemSnapshotService 创建 ItemSnapshotService
func NewItemSnapshotService(database *db.DB) *ItemSnapshotService {
	return &ItemSnapshotService{db: database}
}

// SaveItemSnapshot 将道具快照写入 items 和 p_i_history，事务内执行
func (s *ItemSnapshotService) SaveItemSnapshot(ctx context.Context, itemList []crawler.ItemModel, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		unique := make(map[uint]*crawler.ItemModel)
		itemIDs := make([]uint, 0, len(itemList))
		for i := range itemList {
			item := &itemList[i]
			if _, ok := unique[item.ItemID]; !ok {
				unique[item.ItemID] = item
				itemIDs = append(itemIDs, item.ItemID)
			}
		}

		var existing []entity.Item
		if err := tx.WithContext(ctx).Where("item_id IN ?", itemIDs).Find(&existing).Error; err != nil {
			return err
		}
		existingMap := make(map[uint]entity.Item)
		for _, e := range existing {
			existingMap[e.ItemID] = e
		}

		for itemID, item := range unique {
			existing, exists := existingMap[itemID]
			if err := s.upsertItem(ctx, tx, item, now, exists, &existing); err != nil {
				return err
			}
			if err := s.insertItemHistory(ctx, tx, item, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ItemSnapshotService) upsertItem(ctx context.Context, tx *gorm.DB, item *crawler.ItemModel, now time.Time, exists bool, existing *entity.Item) error {
	updates := map[string]any{
		"name":                 item.Name,
		"desc":                 item.Desc,
		"icon":                 item.Icon,
		"price_standard":       uint(item.PriceStandard),
		"price_current_lowest": item.PriceCurrentLowest,
		"update_at":            now,
	}
	if exists {
		return tx.WithContext(ctx).Model(existing).Updates(updates).Error
	}
	return tx.WithContext(ctx).Create(&entity.Item{
		ItemID:             item.ItemID,
		Name:               item.Name,
		Desc:               item.Desc,
		Icon:               item.Icon,
		PriceStandard:      uint(item.PriceStandard),
		PriceCurrentLowest: item.PriceCurrentLowest,
		UpdatedAt:          now,
	}).Error
}

func (s *ItemSnapshotService) insertItemHistory(ctx context.Context, tx *gorm.DB, item *crawler.ItemModel, now time.Time) error {
	history := entity.ItemPriceHistory{
		ItemID:           item.ItemID,
		AtDate:           now,
		AtDateHour:       now.Format("200601021504"),
		AtYear:           now.Format("2006"),
		AtMonth:          now.Format("01"),
		AtDay:            now.Format("02"),
		AtHour:           now.Format("15"),
		AtMinute:         now.Format("04"),
		PriceStandard:    uint(item.PriceStandard),
		PriceCurrentSale: item.PriceCurrentLowest,
		CTime:            now,
	}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "at_date_hour"}, {Name: "item_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"price_standard", "price_current_sale"}),
	}).Create(&history).Error
}

// SyncAllItemsPriceChanges 计算并回写所有道具的 1d/7d 涨跌幅到 items 表（p_i_history 无涨跌幅字段）
func (s *ItemSnapshotService) SyncAllItemsPriceChanges(ctx context.Context, _ string) error {
	log.Printf(">>> 开始同步道具价格涨跌幅 <<<")
	startTime := time.Now()

	itemRepo := repositories.NewItemRepository(s.db.DB)
	historyRepo := repositories.NewItemHistoryRepository(s.db.DB)

	items, err := itemRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	oneDayAgo := now.AddDate(0, 0, -1)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	map1d, _ := historyRepo.GetPriceHistoryMap(ctx, oneDayAgo)
	map7d, _ := historyRepo.GetPriceHistoryMap(ctx, sevenDaysAgo)

	total := len(items)
	successCount := 0
	log.Printf("找到道具数量: %d", total)

	for _, item := range items {
		pc1d := 0.0
		pc7d := 0.0
		if h1d, ok := map1d[item.ItemID]; ok && h1d.PriceStandard > 0 {
			pc1d = float64(int(item.PriceStandard)-int(h1d.PriceStandard)) / float64(h1d.PriceStandard)
		}
		if h7d, ok := map7d[item.ItemID]; ok && h7d.PriceStandard > 0 {
			pc7d = float64(int(item.PriceStandard)-int(h7d.PriceStandard)) / float64(h7d.PriceStandard)
		}
		pc1d = roundFloat(pc1d*100, 2)
		pc7d = roundFloat(pc7d*100, 2)

		if err := itemRepo.UpdatePriceChanges(ctx, item.ItemID, pc1d, pc7d); err != nil {
			log.Printf("[ItemID: %d] 更新涨跌幅失败: %v", item.ItemID, err)
		} else {
			successCount++
		}
	}

	log.Printf(">>> 同步道具价格涨跌幅完成，耗时: %v, 总数: %d, 成功: %d <<<", time.Since(startTime), total, successCount)
	return nil
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
