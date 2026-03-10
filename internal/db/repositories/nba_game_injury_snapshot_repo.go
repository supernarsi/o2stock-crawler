package repositories

import (
	"context"
	"strconv"

	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type NBAGameInjurySnapshotRepository struct {
	baseRepository[entity.NBAGameInjurySnapshot]
}

func NewNBAGameInjurySnapshotRepository(db *gorm.DB) *NBAGameInjurySnapshotRepository {
	return &NBAGameInjurySnapshotRepository{
		baseRepository: baseRepository[entity.NBAGameInjurySnapshot]{db: db},
	}
}

// ReplaceByGameDate 全量替换某日伤病快照。
func (r *NBAGameInjurySnapshotRepository) ReplaceByGameDate(
	ctx context.Context,
	gameDate string,
	rows []entity.NBAGameInjurySnapshot,
) error {
	rows = dedupeInjurySnapshots(rows)
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("game_date = ?", gameDate).Delete(&entity.NBAGameInjurySnapshot{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

func (r *NBAGameInjurySnapshotRepository) GetByGameDate(
	ctx context.Context,
	gameDate string,
) ([]entity.NBAGameInjurySnapshot, error) {
	var rows []entity.NBAGameInjurySnapshot
	err := r.ctx(ctx).
		Where("game_date = ?", gameDate).
		Order("nba_player_id ASC").
		Find(&rows).Error
	return rows, err
}

func dedupeInjurySnapshots(rows []entity.NBAGameInjurySnapshot) []entity.NBAGameInjurySnapshot {
	if len(rows) <= 1 {
		return rows
	}

	indexByKey := make(map[string]int, len(rows))
	result := make([]entity.NBAGameInjurySnapshot, 0, len(rows))
	for _, row := range rows {
		key := row.GameDate + "#" + strconv.FormatUint(uint64(row.NBAPlayerID), 10)
		if idx, ok := indexByKey[key]; ok {
			result[idx] = row
			continue
		}
		indexByKey[key] = len(result)
		result = append(result, row)
	}
	return result
}
