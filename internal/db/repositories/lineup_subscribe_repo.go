package repositories

import (
	"context"
	"time"

	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LineupSubscribeRepository struct {
	baseRepository[entity.LineupSubscribe]
}

func NewLineupSubscribeRepository(db *gorm.DB) *LineupSubscribeRepository {
	return &LineupSubscribeRepository{
		baseRepository: baseRepository[entity.LineupSubscribe]{db: db},
	}
}

// Upsert 创建或更新订阅记录（在原数据上修改，不删除再插入）
func (r *LineupSubscribeRepository) Upsert(ctx context.Context, userID uint, status uint8) error {
	return r.ctx(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"status": status, "updated_at": time.Now()}),
	}).Create(&entity.LineupSubscribe{
		UserID: userID,
		Status: status,
	}).Error
}

// GetByUserID 查询指定用户的订阅状态
func (r *LineupSubscribeRepository) GetByUserID(ctx context.Context, userID uint) (*entity.LineupSubscribe, error) {
	var sub entity.LineupSubscribe
	err := r.ctx(ctx).Where("user_id = ?", userID).First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetActiveSubscribers 获取所有已订阅用户 ID 列表
func (r *LineupSubscribeRepository) GetActiveSubscribers(ctx context.Context) ([]entity.LineupSubscribe, error) {
	var subs []entity.LineupSubscribe
	err := r.ctx(ctx).Where("status = ?", 1).Find(&subs).Error
	return subs, err
}

// UpdatePushStats 更新推送成功后的统计信息
func (r *LineupSubscribeRepository) UpdatePushStats(ctx context.Context, userID uint) error {
	return r.ctx(ctx).Model(&entity.LineupSubscribe{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"push_count":     gorm.Expr("push_count + ?", 1),
			"last_push_time": time.Now(),
		}).Error
}
