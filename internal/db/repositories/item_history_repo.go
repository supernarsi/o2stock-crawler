package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type ItemHistoryRepository struct {
	baseRepository[entity.ItemPriceHistory]
}

func NewItemHistoryRepository(db *gorm.DB) *ItemHistoryRepository {
	return &ItemHistoryRepository{
		baseRepository: baseRepository[entity.ItemPriceHistory]{db: db},
	}
}

// GetPriceHistoryMap 取 at_date_hour >= startTime 时每个 item_id 最小 at_date_hour 的一条，用于计算 1d/7d 基准价
func (r *ItemHistoryRepository) GetPriceHistoryMap(ctx context.Context, startTime time.Time) (map[uint]entity.ItemPriceHistory, error) {
	startStr := startTime.Format("200601021504")
	subQuery := r.model(ctx).
		Select("item_id, MIN(at_date_hour) as min_hour").
		Where("at_date_hour >= ?", startStr).
		Group("item_id")

	var results []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Table("p_i_history p1").
		Joins("INNER JOIN (?) p2 ON p1.item_id = p2.item_id AND p1.at_date_hour = p2.min_hour", subQuery).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	m := make(map[uint]entity.ItemPriceHistory, len(results))
	for _, h := range results {
		m[h.ItemID] = h
	}
	return m, nil
}
