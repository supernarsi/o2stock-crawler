package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"sort"

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

// BatchUpdateActualPower 批量更新推荐阵容的实际总战力（按 game_date + rank）
func (r *LineupRecommendationRepository) BatchUpdateActualPower(
	ctx context.Context,
	gameDate string,
	rankPowerMap map[uint]float64,
) error {
	if len(rankPowerMap) == 0 {
		return nil
	}

	ranks := make([]uint, 0, len(rankPowerMap))
	for rank := range rankPowerMap {
		ranks = append(ranks, rank)
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] < ranks[j] })

	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		for _, rank := range ranks {
			if err := tx.Model(&entity.LineupRecommendation{}).
				Where("game_date = ? AND `rank` = ?", gameDate, rank).
				Update("total_actual_power", rankPowerMap[rank]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
