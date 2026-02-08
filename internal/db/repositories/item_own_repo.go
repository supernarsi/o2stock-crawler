package repositories

import (
	"context"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type ItemOwnRepository struct {
	baseRepository[entity.UserItemOwn]
}

func NewItemOwnRepository(db *gorm.DB) *ItemOwnRepository {
	return &ItemOwnRepository{
		baseRepository: baseRepository[entity.UserItemOwn]{db: db},
	}
}

// GetByUserID returns all own records for a user (own_sta in Purchased,Sold), order by dt_in desc.
func (r *ItemOwnRepository) GetByUserID(ctx context.Context, userID uint) ([]entity.UserItemOwn, error) {
	var results []entity.UserItemOwn
	err := r.ctx(ctx).
		Where("uid = ? AND own_sta IN (?, ?)", userID, consts.OwnStaPurchased, consts.OwnStaSold).
		Order("dt_in DESC, id DESC").
		Find(&results).Error
	return results, err
}

// GetByItemIDs returns own records for a user within itemIDs (own_sta in Purchased,Sold).
func (r *ItemOwnRepository) GetByItemIDs(ctx context.Context, userID uint, itemIDs []uint) ([]entity.UserItemOwn, error) {
	if len(itemIDs) == 0 {
		return []entity.UserItemOwn{}, nil
	}
	var results []entity.UserItemOwn
	err := r.ctx(ctx).
		Where("uid = ? AND item_id IN ? AND own_sta IN (?, ?)", userID, itemIDs, consts.OwnStaPurchased, consts.OwnStaSold).
		Order("dt_in DESC, id DESC").
		Find(&results).Error
	return results, err
}

func (r *ItemOwnRepository) GetByRecordID(ctx context.Context, userID, ownID uint) (*entity.UserItemOwn, error) {
	var result entity.UserItemOwn
	err := r.ctx(ctx).
		Where("uid = ? AND id = ?", userID, ownID).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ItemOwnRepository) Create(ctx context.Context, userID, itemID, num, cost uint, dt time.Time, notifyType uint8) error {
	own := entity.UserItemOwn{
		UserID:     userID,
		ItemID:     itemID,
		BuyCount:   num,
		BuyPrice:   cost,
		BuyTime:    dt,
		Sta:        consts.OwnStaPurchased,
		NotifyType: notifyType,
	}
	return r.ctx(ctx).Create(&own).Error
}

// MarkAsSoldByID updates a specific own record to sold. It is safe for concurrent calls:
// update is guarded by own_sta=OwnStaPurchased and dt_out IS NULL.
func (r *ItemOwnRepository) MarkAsSoldByID(ctx context.Context, userID, ownID, cost uint, dt time.Time) (int64, error) {
	res := r.model(ctx).
		Where("uid = ? AND id = ? AND own_sta = ? AND dt_out IS NULL", userID, ownID, consts.OwnStaPurchased).
		Updates(map[string]any{
			"own_sta":   consts.OwnStaSold,
			"price_out": cost,
			"dt_out":    dt,
		})
	return res.RowsAffected, res.Error
}

// UpdateNotifyByUserAndItem 更新用户持有该道具的订阅类型，并将 notify_time 置空（仅更新 own_sta=1 且 dt_out 为空的记录，同球员逻辑）
func (r *ItemOwnRepository) UpdateNotifyByUserAndItem(ctx context.Context, userID, itemID uint, notifyType uint8) (int64, error) {
	res := r.model(ctx).
		Where("uid = ? AND item_id = ? AND own_sta = ? AND dt_out IS NULL", userID, itemID, consts.OwnStaPurchased).
		Updates(map[string]any{
			"notify_type": notifyType,
			"notify_time": nil,
		})
	return res.RowsAffected, res.Error
}
