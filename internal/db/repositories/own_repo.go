package repositories

import (
	"context"
	"o2stock-crawler/internal/consts"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type OwnRepository struct {
	baseRepository[entity.UserPOwn]
}

func NewOwnRepository(db *gorm.DB) *OwnRepository {
	return &OwnRepository{
		baseRepository: baseRepository[entity.UserPOwn]{db: db},
	}
}

func (r *OwnRepository) CountOwned(ctx context.Context, userID, goodsID uint, ownGoods uint8) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ? AND own_goods = ? AND pid = ? AND own_sta = ?", userID, ownGoods, goodsID, consts.OwnStaPurchased).
		Count(&count).Error
	return count, err
}

func (r *OwnRepository) GetByUserID(ctx context.Context, userID uint, ownGoods ...uint8) ([]entity.UserPOwn, error) {
	var results []entity.UserPOwn
	query := r.ctx(ctx).Where("uid = ?", userID)
	if len(ownGoods) > 0 {
		goods := make([]int, len(ownGoods))
		for i, v := range ownGoods {
			goods[i] = int(v)
		}
		query = query.Where("own_goods IN ?", goods)
	}
	err := query.Order("dt_in DESC, id DESC").Find(&results).Error
	return results, err
}

func (r *OwnRepository) GetByGoodsIDs(ctx context.Context, userID uint, goodsIDs []uint, ownGoods uint8) ([]entity.UserPOwn, error) {
	var results []entity.UserPOwn
	err := r.ctx(ctx).
		Where("uid = ? AND own_goods = ? AND pid IN ? AND own_sta IN ?", userID, ownGoods, goodsIDs, []int{int(consts.OwnStaPurchased), int(consts.OwnStaSold)}).
		Order("dt_in DESC, id DESC").
		Find(&results).Error
	return results, err
}

func (r *OwnRepository) GetByRecordID(ctx context.Context, recordID, userID uint) (*entity.UserPOwn, error) {
	var result entity.UserPOwn
	err := r.ctx(ctx).
		Where("id = ? AND uid = ?", recordID, userID).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetLatestActiveByUserAndGoods 获取用户某球员/道具最新的一条持仓记录（仅 own_sta=1 且未出售）
func (r *OwnRepository) GetLatestActiveByUserAndGoods(ctx context.Context, userID, goodsID uint, ownGoods uint8) (*entity.UserPOwn, error) {
	var result entity.UserPOwn
	err := r.ctx(ctx).
		Where("uid = ? AND own_goods = ? AND pid = ? AND own_sta = ? AND dt_out IS NULL", userID, ownGoods, goodsID, consts.OwnStaPurchased).
		Order("dt_in DESC").
		Limit(1).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OwnRepository) Create(ctx context.Context, userID, goodsID, num, cost uint, dt time.Time, notifyType uint8, ownGoods uint8) error {
	own := entity.UserPOwn{
		UserID:     userID,
		OwnGoods:   ownGoods,
		PID:        goodsID,
		BuyCount:   num,
		BuyPrice:   cost,
		BuyTime:    dt,
		Sta:        int(consts.OwnStaPurchased),
		NotifyType: notifyType,
	}
	return r.ctx(ctx).Create(&own).Error
}

func (r *OwnRepository) MarkAsSold(ctx context.Context, userID, goodsID, cost uint, dt time.Time, ownGoods uint8) error {
	return r.model(ctx).
		Where("uid = ? AND own_goods = ? AND pid = ? AND own_sta = ?", userID, ownGoods, goodsID, consts.OwnStaPurchased).
		Limit(1).
		Updates(map[string]interface{}{
			"own_sta":   consts.OwnStaSold,
			"price_out": cost,
			"dt_out":    dt,
		}).Error
}

// MarkAsSoldByID updates a specific own record to sold.
func (r *OwnRepository) MarkAsSoldByID(ctx context.Context, userID, recordID, cost uint, dt time.Time) (int64, error) {
	res := r.model(ctx).
		Where("uid = ? AND id = ? AND own_sta = ? AND dt_out IS NULL", userID, recordID, consts.OwnStaPurchased).
		Updates(map[string]interface{}{
			"own_sta":   consts.OwnStaSold,
			"price_out": cost,
			"dt_out":    dt,
		})
	return res.RowsAffected, res.Error
}

func (r *OwnRepository) Update(ctx context.Context, userID, recordID uint, updates map[string]interface{}) error {
	return r.model(ctx).
		Where("uid = ? AND id = ?", userID, recordID).
		Updates(updates).Error
}

func (r *OwnRepository) Delete(ctx context.Context, userID, recordID uint) error {
	return r.ctx(ctx).
		Where("uid = ? AND id = ?", userID, recordID).
		Delete(&entity.UserPOwn{}).Error
}

// UpdateNotifyByUserAndGoods 更新用户持有该球员/道具的订阅类型，并将 notify_time 置空（仅更新 own_sta=1 且 dt_out 为空的记录）
func (r *OwnRepository) UpdateNotifyByUserAndGoods(ctx context.Context, userID, goodsID uint, notifyType uint8, ownGoods uint8) (int64, error) {
	res := r.model(ctx).
		Where("uid = ? AND own_goods = ? AND pid = ? AND own_sta = ? AND dt_out IS NULL", userID, ownGoods, goodsID, consts.OwnStaPurchased).
		Updates(map[string]interface{}{
			"notify_type": notifyType,
			"notify_time": nil,
		})
	return res.RowsAffected, res.Error
}

// GetActiveNotifyOwnsByGoodsIDs 获取在给定球员/道具 ID 下的、已购买未出售且订阅了通知的持仓（own_sta=1, dt_out IS NULL, notify_type IN (1,2)）
func (r *OwnRepository) GetActiveNotifyOwnsByGoodsIDs(ctx context.Context, goodsIDs []uint, ownGoods uint8) ([]entity.UserPOwn, error) {
	if len(goodsIDs) == 0 {
		return nil, nil
	}
	var results []entity.UserPOwn
	err := r.ctx(ctx).
		Where("own_goods = ? AND pid IN ? AND own_sta = ? AND dt_out IS NULL AND notify_type IN ?", ownGoods, goodsIDs, consts.OwnStaPurchased, []int{int(consts.NotifyTypeBreakEven), int(consts.NotifyTypeProfit15)}).
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
func (r *OwnRepository) GetOwnRecordsForInvestmentStats(ctx context.Context, goodsIDs []uint, ownGoods uint8) ([]entity.UserPOwn, error) {
	query := r.ctx(ctx).Where("own_goods = ? AND own_sta IN ?", ownGoods, []int{int(consts.OwnStaPurchased), int(consts.OwnStaSold)})
	if len(goodsIDs) > 0 {
		query = query.Where("pid IN ?", goodsIDs)
	}
	var results []entity.UserPOwn
	err := query.Order("pid, dt_in").Find(&results).Error
	return results, err
}
