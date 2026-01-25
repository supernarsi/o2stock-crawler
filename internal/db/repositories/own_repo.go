package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type OwnRepository struct {
	baseRepository[entity.UserPlayerOwn]
}

func NewOwnRepository(db *gorm.DB) *OwnRepository {
	return &OwnRepository{
		baseRepository: baseRepository[entity.UserPlayerOwn]{db: db},
	}
}

func (r *OwnRepository) CountOwned(ctx context.Context, userID, playerID uint) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ? AND pid = ? AND own_sta = 1", userID, playerID).
		Count(&count).Error
	return count, err
}

func (r *OwnRepository) GetByUserID(ctx context.Context, userID uint) ([]entity.UserPlayerOwn, error) {
	var results []entity.UserPlayerOwn
	err := r.ctx(ctx).
		Where("uid = ?", userID).
		Order("dt_in DESC").
		Find(&results).Error
	return results, err
}

func (r *OwnRepository) GetByPlayerIDs(ctx context.Context, userID uint, playerIDs []uint) ([]entity.UserPlayerOwn, error) {
	var results []entity.UserPlayerOwn
	err := r.ctx(ctx).
		Where("uid = ? AND pid IN ? AND own_sta IN (1, 2)", userID, playerIDs).
		Order("dt_in DESC").
		Find(&results).Error
	return results, err
}

func (r *OwnRepository) GetByRecordID(ctx context.Context, recordID, userID uint) (*entity.UserPlayerOwn, error) {
	var result entity.UserPlayerOwn
	err := r.ctx(ctx).
		Where("id = ? AND uid = ?", recordID, userID).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OwnRepository) Create(ctx context.Context, userID, playerID, num, cost uint, dt time.Time) error {
	own := entity.UserPlayerOwn{
		UserID:   userID,
		PlayerID: playerID,
		BuyCount: num,
		BuyPrice: cost,
		BuyTime:  dt,
		Sta:      1,
	}
	return r.ctx(ctx).Create(&own).Error
}

func (r *OwnRepository) MarkAsSold(ctx context.Context, userID, playerID, cost uint, dt time.Time) error {
	return r.model(ctx).
		Where("uid = ? AND pid = ? AND own_sta = 1", userID, playerID).
		Limit(1).
		Updates(map[string]interface{}{
			"own_sta":   2,
			"price_out": cost,
			"dt_out":    dt,
		}).Error
}

func (r *OwnRepository) Update(ctx context.Context, userID, recordID uint, updates map[string]interface{}) error {
	return r.model(ctx).
		Where("uid = ? AND id = ?", userID, recordID).
		Updates(updates).Error
}

func (r *OwnRepository) Delete(ctx context.Context, userID, recordID uint) error {
	return r.ctx(ctx).
		Where("uid = ? AND id = ?", userID, recordID).
		Delete(&entity.UserPlayerOwn{}).Error
}
