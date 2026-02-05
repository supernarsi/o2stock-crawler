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

// GetLatestActiveByUserAndPlayer 获取用户某球员最新的一条持仓记录（仅 own_sta=1 且未出售）
func (r *OwnRepository) GetLatestActiveByUserAndPlayer(ctx context.Context, userID, playerID uint) (*entity.UserPlayerOwn, error) {
	var result entity.UserPlayerOwn
	err := r.ctx(ctx).
		Where("uid = ? AND pid = ? AND own_sta = 1 AND dt_out IS NULL", userID, playerID).
		Order("dt_in DESC").
		Limit(1).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OwnRepository) Create(ctx context.Context, userID, playerID, num, cost uint, dt time.Time, notifyType uint8) error {
	own := entity.UserPlayerOwn{
		UserID:     userID,
		PlayerID:   playerID,
		BuyCount:   num,
		BuyPrice:   cost,
		BuyTime:    dt,
		Sta:        1,
		NotifyType: notifyType,
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

// UpdateNotifyByUserAndPlayer 更新用户持有该球员的订阅类型，并将 notify_time 置空（仅更新 own_sta=1 且 dt_out 为空的记录）
func (r *OwnRepository) UpdateNotifyByUserAndPlayer(ctx context.Context, userID, playerID uint, notifyType uint8) (int64, error) {
	res := r.model(ctx).
		Where("uid = ? AND pid = ? AND own_sta = 1 AND dt_out IS NULL", userID, playerID).
		Updates(map[string]interface{}{
			"notify_type": notifyType,
			"notify_time": nil,
		})
	return res.RowsAffected, res.Error
}

// GetActiveNotifyOwnsByPlayerIDs 获取在给定球员 ID 下的、已购买未出售且订阅了通知的持仓（own_sta=1, dt_out IS NULL, notify_type IN (1,2)）
func (r *OwnRepository) GetActiveNotifyOwnsByPlayerIDs(ctx context.Context, playerIDs []uint) ([]entity.UserPlayerOwn, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	var results []entity.UserPlayerOwn
	err := r.ctx(ctx).
		Where("pid IN ? AND own_sta = 1 AND dt_out IS NULL AND notify_type IN (1, 2)", playerIDs).
		Find(&results).Error
	return results, err
}

// SetNotifyTime 将指定记录的 notify_time 更新为给定时间
func (r *OwnRepository) SetNotifyTime(ctx context.Context, ownID uint, t time.Time) error {
	return r.model(ctx).
		Where("id = ?", ownID).
		Update("notify_time", t).Error
}

// GetOwnRecordsForInvestmentStats 获取用于投资盈亏统计的持仓记录（含持有与已售），按球员 ID 聚合用
// playerIDs 为空时查询全部球员
func (r *OwnRepository) GetOwnRecordsForInvestmentStats(ctx context.Context, playerIDs []uint) ([]entity.UserPlayerOwn, error) {
	query := r.ctx(ctx).Where("own_sta IN (1, 2)")
	if len(playerIDs) > 0 {
		query = query.Where("pid IN ?", playerIDs)
	}
	var results []entity.UserPlayerOwn
	err := query.Order("pid, dt_in").Find(&results).Error
	return results, err
}
