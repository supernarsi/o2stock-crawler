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

// GetByDate 获取指定日期推荐阵容（仅 AI 推荐）
func (r *LineupRecommendationRepository) GetByDate(ctx context.Context, gameDate string) ([]entity.LineupRecommendation, error) {
	return r.GetByDateAndType(ctx, gameDate, entity.LineupRecommendationTypeAIRecommended)
}

// GetAllByDate 获取指定日期所有推荐阵容（包括 AI 推荐、3 日均值、5 日均值）
func (r *LineupRecommendationRepository) GetAllByDate(ctx context.Context, gameDate string) ([]entity.LineupRecommendation, error) {
	var recs []entity.LineupRecommendation
	err := r.ctx(ctx).
		Where("game_date = ?", gameDate).
		Order("recommendation_type, `rank` ASC").
		Find(&recs).Error
	return recs, err
}

// GetByDateAndType 获取指定日期、指定类型的推荐阵容
func (r *LineupRecommendationRepository) GetByDateAndType(
	ctx context.Context,
	gameDate string,
	recommendationType uint8,
) ([]entity.LineupRecommendation, error) {
	var recs []entity.LineupRecommendation
	err := r.ctx(ctx).
		Where("game_date = ? AND recommendation_type = ?", gameDate, recommendationType).
		Order("`rank` ASC").
		Find(&recs).Error
	return recs, err
}

// BatchUpdateActualPower 批量更新推荐阵容的实际总战力（按 game_date + rank）
// 仅更新 recommendation_type=1 (AI 推荐) 的数据
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
				Where(
					"game_date = ? AND recommendation_type = ? AND `rank` = ?",
					gameDate,
					entity.LineupRecommendationTypeAIRecommended,
					rank,
				).
				Update("total_actual_power", rankPowerMap[rank]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchUpdateActualPowerForAllTypes 批量更新所有推荐类型的实际总战力（用于回测）
func (r *LineupRecommendationRepository) BatchUpdateActualPowerForAllTypes(
	ctx context.Context,
	gameDate string,
	recs []entity.LineupRecommendation,
	actualPowerMap map[[5]uint]float64,
) error {
	if len(recs) == 0 {
		return nil
	}

	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		for _, rec := range recs {
			playerIDs := [5]uint{rec.Player1ID, rec.Player2ID, rec.Player3ID, rec.Player4ID, rec.Player5ID}
			actualPower, ok := actualPowerMap[playerIDs]
			if !ok {
				continue
			}
			if err := tx.Model(&entity.LineupRecommendation{}).
				Where(
					"game_date = ? AND recommendation_type = ? AND `rank` = ?",
					gameDate,
					rec.RecommendationType,
					rec.Rank,
				).
				Update("total_actual_power", actualPower).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
