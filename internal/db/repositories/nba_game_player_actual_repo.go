package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NBAGamePlayerActualRepository struct {
	baseRepository[entity.NBAGamePlayerActual]
}

func NewNBAGamePlayerActualRepository(db *gorm.DB) *NBAGamePlayerActualRepository {
	return &NBAGamePlayerActualRepository{
		baseRepository: baseRepository[entity.NBAGamePlayerActual]{db: db},
	}
}

// ReplaceByGameDate 全量替换某日反馈（先删后插）
func (r *NBAGamePlayerActualRepository) ReplaceByGameDate(ctx context.Context, gameDate string, rows []entity.NBAGamePlayerActual) error {
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("game_date = ?", gameDate).Delete(&entity.NBAGamePlayerActual{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

// BatchUpsert 批量写入反馈（按 game_date + rank + nba_player_id 去重更新）
func (r *NBAGamePlayerActualRepository) BatchUpsert(ctx context.Context, rows []entity.NBAGamePlayerActual) error {
	if len(rows) == 0 {
		return nil
	}
	return r.ctx(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "game_date"},
				{Name: "rank"},
				{Name: "nba_player_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"salary",
				"actual_power",
				"source",
				"updated_at",
			}),
		}).
		CreateInBatches(rows, 100).Error
}

func (r *NBAGamePlayerActualRepository) GetByGameDate(ctx context.Context, gameDate string) ([]entity.NBAGamePlayerActual, error) {
	var rows []entity.NBAGamePlayerActual
	err := r.ctx(ctx).
		Where("game_date = ?", gameDate).
		Order("`rank` ASC, nba_player_id ASC").
		Find(&rows).Error
	return rows, err
}
