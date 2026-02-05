package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type ItemRepository struct {
	baseRepository[entity.Item]
}

func NewItemRepository(db *gorm.DB) *ItemRepository {
	return &ItemRepository{
		baseRepository: baseRepository[entity.Item]{db: db},
	}
}

// ListAll 返回所有道具，用于 SyncAllItemsPriceChanges 遍历
func (r *ItemRepository) ListAll(ctx context.Context) ([]entity.Item, error) {
	var items []entity.Item
	err := r.ctx(ctx).Find(&items).Error
	return items, err
}

// BatchGetByItemIDs 按 item_id 批量查询，用于 SaveItemSnapshot 时查已存在
func (r *ItemRepository) BatchGetByItemIDs(ctx context.Context, itemIDs []uint) ([]entity.Item, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var items []entity.Item
	err := r.ctx(ctx).Where("item_id IN ?", itemIDs).Find(&items).Error
	return items, err
}

// UpdatePriceChanges 更新道具涨跌幅
func (r *ItemRepository) UpdatePriceChanges(ctx context.Context, itemID uint, pc1d, pc7d float64) error {
	return r.model(ctx).
		Where("item_id = ?", itemID).
		Updates(map[string]any{
			"price_change_1d": pc1d,
			"price_change_7d":  pc7d,
		}).Error
}
