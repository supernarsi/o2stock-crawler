package repositories

import (
	"context"

	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NBAPlayerSalaryRepository struct {
	baseRepository[entity.NBAPlayerSalary]
}

func NewNBAPlayerSalaryRepository(db *gorm.DB) *NBAPlayerSalaryRepository {
	return &NBAPlayerSalaryRepository{
		baseRepository: baseRepository[entity.NBAPlayerSalary]{db: db},
	}
}

func (r *NBAPlayerSalaryRepository) BatchUpsert(ctx context.Context, rows []entity.NBAPlayerSalary) error {
	if len(rows) == 0 {
		return nil
	}

	return r.ctx(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "nba_player_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"nba_team_id",
				"player_name",
				"player_en_name",
				"team_name",
				"salary",
				"combat_power",
				"position",
				"source_game_date",
				"updated_at",
			}),
		}).
		CreateInBatches(rows, 100).Error
}

func (r *NBAPlayerSalaryRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.model(ctx).Count(&count).Error
	return count, err
}

func (r *NBAPlayerSalaryRepository) GetByTeamIDs(
	ctx context.Context,
	teamIDs []string,
) ([]entity.NBAPlayerSalary, error) {
	var rows []entity.NBAPlayerSalary
	if len(teamIDs) == 0 {
		return rows, nil
	}

	err := r.ctx(ctx).
		Where("nba_team_id IN ?", teamIDs).
		Order("salary DESC, nba_player_id ASC").
		Find(&rows).Error
	return rows, err
}
