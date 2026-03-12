package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LineupRecommendationRepository struct {
	baseRepository[entity.LineupRecommendation]
}

func NewLineupRecommendationRepository(db *gorm.DB) *LineupRecommendationRepository {
	return &LineupRecommendationRepository{
		baseRepository: baseRepository[entity.LineupRecommendation]{db: db},
	}
}

// BatchSave 批量保存推荐阵容（按 game_date + recommendation_type + rank 覆盖）
func (r *LineupRecommendationRepository) BatchSave(ctx context.Context, recs []entity.LineupRecommendation) error {
	if len(recs) == 0 {
		return nil
	}
	gameDate := recs[0].GameDate
	ranksByType := make(map[uint8][]uint, 3)
	for _, rec := range recs {
		ranksByType[rec.RecommendationType] = append(ranksByType[rec.RecommendationType], rec.Rank)
	}
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		for recommendationType, ranks := range ranksByType {
			if err := tx.
				Where("game_date = ? AND recommendation_type = ? AND `rank` NOT IN ?", gameDate, recommendationType, ranks).
				Delete(&entity.LineupRecommendation{}).Error; err != nil {
				return err
			}
		}

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "game_date"},
				{Name: "recommendation_type"},
				{Name: "rank"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_predicted_power",
				"total_actual_power",
				"total_salary",
				"player1_id",
				"player2_id",
				"player3_id",
				"player4_id",
				"player5_id",
				"detail_json",
				"updated_at",
			}),
		}).Create(&recs).Error
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
		Where("LEFT(game_date, 10) = ? AND recommendation_type = ?", gameDate[:min(10, len(gameDate))], recommendationType).
		Order("`rank` ASC").
		Find(&recs).Error
	return recs, err
}

// GetByDatesAndType 批量获取多个日期、指定类型的推荐阵容
func (r *LineupRecommendationRepository) GetByDatesAndType(
	ctx context.Context,
	gameDates []string,
	recommendationType uint8,
) ([]entity.LineupRecommendation, error) {
	if len(gameDates) == 0 {
		return nil, nil
	}
	normalizedDates := normalizeGameDates(gameDates)
	var recs []entity.LineupRecommendation
	err := r.ctx(ctx).
		Where("LEFT(game_date, 10) IN ? AND recommendation_type = ?", normalizedDates, recommendationType).
		Order("game_date DESC, `rank` ASC").
		Find(&recs).Error
	return recs, err
}

// GetLatestGameDate 获取大于等于给定日期的最近一个有推荐数据的日期
// todayStr: 传入当前日期的字符串 (YYYY-MM-DD)，用于查找 >= today 的最新推荐
func (r *LineupRecommendationRepository) GetLatestGameDate(ctx context.Context, todayStr string) (string, error) {
	var gameDate string
	queryToday := todayStr
	if len(queryToday) == 10 {
		queryToday = queryToday + "T"
	}
	err := r.ctx(ctx).Model(&entity.LineupRecommendation{}).
		Where("game_date >= ?", queryToday).
		Select("MAX(game_date)").
		Scan(&gameDate).Error
	return gameDate, err
}

// GetRecentGameDates 获取小于给定日期的最近 limit 个有推荐数据的日期
func (r *LineupRecommendationRepository) GetRecentGameDates(ctx context.Context, beforeDate string, limit int) ([]string, error) {
	var dates []string

	// 为了兼容只传 2026-03-10 和完整的 2026-03-10T... 字符串，
	// 如果传入的是短日期，确保查询能匹配。实际库里的 game_date 包含 T... 时，如果 beforeDate 不带，可以直接前缀比对。
	queryBefore := beforeDate
	if len(queryBefore) == 10 {
		queryBefore = queryBefore + "T"
	}

	err := r.ctx(ctx).Model(&entity.LineupRecommendation{}).
		Where("game_date < ?", queryBefore).
		Select("DISTINCT game_date").
		Order("game_date DESC").
		Limit(limit).
		Pluck("game_date", &dates).Error
	return dates, err
}

func normalizeGameDates(gameDates []string) []string {
	normalized := make([]string, 0, len(gameDates))
	seen := make(map[string]struct{}, len(gameDates))
	for _, gameDate := range gameDates {
		key := gameDate
		if len(key) > 10 {
			key = key[:10]
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
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
	now := time.Now()

	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		for _, rank := range ranks {
			if err := tx.Model(&entity.LineupRecommendation{}).
				Where(
					"game_date = ? AND recommendation_type = ? AND `rank` = ?",
					gameDate,
					entity.LineupRecommendationTypeAIRecommended,
					rank,
				).
				Updates(map[string]any{
					"total_actual_power": rankPowerMap[rank],
					"updated_at":         now,
				}).Error; err != nil {
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
	now := time.Now()

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
				Updates(map[string]any{
					"total_actual_power": actualPower,
					"updated_at":         now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
