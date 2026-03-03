package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type LineupRecommendationRepository struct {
	baseRepository[entity.LineupRecommendation]
}

func NewLineupRecommendationRepository(db *gorm.DB) *LineupRecommendationRepository {
	return &LineupRecommendationRepository{
		baseRepository: baseRepository[entity.LineupRecommendation]{db: db},
	}
}

// BatchSave 批量保存推荐阵容（先删除同日期旧记录，再插入）
func (r *LineupRecommendationRepository) BatchSave(ctx context.Context, recs []entity.LineupRecommendation) error {
	if len(recs) == 0 {
		return nil
	}
	gameDate := recs[0].GameDate
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		// 删除旧记录
		if err := tx.Where("game_date = ?", gameDate).Delete(&entity.LineupRecommendation{}).Error; err != nil {
			return err
		}
		// 插入新记录
		return tx.Create(&recs).Error
	})
}

// GetByDate 获取指定日期推荐阵容
func (r *LineupRecommendationRepository) GetByDate(ctx context.Context, gameDate string) ([]entity.LineupRecommendation, error) {
	var recs []entity.LineupRecommendation
	err := r.ctx(ctx).
		Where("game_date = ?", gameDate).
		Order("`rank` ASC").
		Find(&recs).Error
	return recs, err
}
