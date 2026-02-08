package repositories

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type ItemRepository struct {
	baseRepository[entity.Item]
}

// ItemFilter 道具列表筛选
type ItemFilter struct {
	Page     int
	Limit    int
	OrderBy  string
	OrderAsc bool
	ItemName string
	SoldOut  bool // true 时仅筛选 price_current_lowest = 0
}

func NewItemRepository(db *gorm.DB) *ItemRepository {
	return &ItemRepository{
		baseRepository: baseRepository[entity.Item]{db: db},
	}
}

// List 分页查询道具列表，供 GET /items 使用
func (r *ItemRepository) List(ctx context.Context, filter ItemFilter) ([]entity.Item, error) {
	query := r.model(ctx)
	if filter.ItemName != "" {
		query = query.Where("name LIKE ?", "%"+filter.ItemName+"%")
	}
	if filter.SoldOut {
		query = query.Where("price_current_lowest = 0")
	}
	if filter.OrderBy != "" {
		direction := "DESC"
		if filter.OrderAsc {
			direction = "ASC"
		}
		col := orderByColumn(filter.OrderBy)
		if col != "" {
			query = query.Order(fmt.Sprintf("%s %s", col, direction))
		}
	}
	if filter.Limit > 0 {
		offset := (filter.Page - 1) * filter.Limit
		if offset < 0 {
			offset = 0
		}
		query = query.Offset(offset).Limit(filter.Limit)
	}
	var items []entity.Item
	err := query.Find(&items).Error
	return items, err
}

// GetByItemID 按 item_id 查询单条道具
func (r *ItemRepository) GetByItemID(ctx context.Context, itemID uint) (*entity.Item, error) {
	var item entity.Item
	err := r.ctx(ctx).Where("item_id = ?", itemID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
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
			"price_change_7d": pc7d,
		}).Error
}

// orderByColumn 只允许白名单列名，防止 SQL 注入
func orderByColumn(s string) string {
	switch s {
	case "item_id", "name", "price_standard", "price_current_lowest", "price_change_1d", "price_change_7d", "update_at":
		return s
	default:
		return ""
	}
}
