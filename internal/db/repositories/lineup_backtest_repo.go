package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LineupBacktestResultRepository struct {
	baseRepository[entity.LineupBacktestResult]
}

func NewLineupBacktestResultRepository(db *gorm.DB) *LineupBacktestResultRepository {
	return &LineupBacktestResultRepository{
		baseRepository: baseRepository[entity.LineupBacktestResult]{db: db},
	}
}

// ReplaceByGameDateAndType 按 game_date + result_type + rank 覆盖某日某类回测结果
func (r *LineupBacktestResultRepository) ReplaceByGameDateAndType(
	ctx context.Context,
	gameDate string,
	resultType uint8,
	rows []entity.LineupBacktestResult,
) error {
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if len(rows) == 0 {
			return tx.Where("game_date = ? AND result_type = ?", gameDate, resultType).
				Delete(&entity.LineupBacktestResult{}).Error
		}

		ranks := make([]uint, 0, len(rows))
		for _, row := range rows {
			ranks = append(ranks, row.Rank)
		}
		if err := tx.
			Where("game_date = ? AND result_type = ? AND `rank` NOT IN ?", gameDate, resultType, ranks).
			Delete(&entity.LineupBacktestResult{}).Error; err != nil {
			return err
		}

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "game_date"},
				{Name: "result_type"},
				{Name: "rank"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
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
		}).Create(&rows).Error
	})
}

func (r *LineupBacktestResultRepository) GetByGameDateAndType(
	ctx context.Context,
	gameDate string,
	resultType uint8,
) ([]entity.LineupBacktestResult, error) {
	var rows []entity.LineupBacktestResult
	err := r.ctx(ctx).
		Where("LEFT(game_date, 10) = ? AND result_type = ?", gameDate[:min(10, len(gameDate))], resultType).
		Order("`rank` ASC").
		Find(&rows).Error
	return rows, err
}

func (r *LineupBacktestResultRepository) GetByGameDatesAndType(
	ctx context.Context,
	gameDates []string,
	resultType uint8,
) ([]entity.LineupBacktestResult, error) {
	if len(gameDates) == 0 {
		return nil, nil
	}
	normalizedDates := normalizeGameDates(gameDates)
	var rows []entity.LineupBacktestResult
	err := r.ctx(ctx).
		Where("LEFT(game_date, 10) IN ? AND result_type = ?", normalizedDates, resultType).
		Order("game_date DESC, `rank` ASC").
		Find(&rows).Error
	return rows, err
}
