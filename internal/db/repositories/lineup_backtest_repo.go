package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type LineupBacktestResultRepository struct {
	baseRepository[entity.LineupBacktestResult]
}

func NewLineupBacktestResultRepository(db *gorm.DB) *LineupBacktestResultRepository {
	return &LineupBacktestResultRepository{
		baseRepository: baseRepository[entity.LineupBacktestResult]{db: db},
	}
}

// ReplaceByGameDateAndType 全量替换某日某类回测结果（先删后插）
func (r *LineupBacktestResultRepository) ReplaceByGameDateAndType(
	ctx context.Context,
	gameDate string,
	resultType uint8,
	rows []entity.LineupBacktestResult,
) error {
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("game_date = ? AND result_type = ?", gameDate, resultType).
			Delete(&entity.LineupBacktestResult{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

func (r *LineupBacktestResultRepository) GetByGameDateAndType(
	ctx context.Context,
	gameDate string,
	resultType uint8,
) ([]entity.LineupBacktestResult, error) {
	var rows []entity.LineupBacktestResult
	err := r.ctx(ctx).
		Where("game_date = ? AND result_type = ?", gameDate, resultType).
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
	var rows []entity.LineupBacktestResult
	err := r.ctx(ctx).
		Where("game_date IN ? AND result_type = ?", gameDates, resultType).
		Order("game_date DESC, `rank` ASC").
		Find(&rows).Error
	return rows, err
}
