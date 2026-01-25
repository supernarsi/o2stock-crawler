package repositories

import (
	"context"
	"fmt"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type PlayerRepository struct {
	baseRepository[entity.Player]
}

func NewPlayerRepository(db *gorm.DB) *PlayerRepository {
	return &PlayerRepository{
		baseRepository: baseRepository[entity.Player]{db: db},
	}
}

type PlayerFilter struct {
	Page       int
	Limit      int
	OrderBy    string
	OrderAsc   bool
	Period     uint8
	SoldOut    bool
	PlayerName string
	PlayerIDs  []uint
	MinPrice   uint
	MaxPrice   uint
	ExFree     bool
}

func (r *PlayerRepository) List(ctx context.Context, filter PlayerFilter) ([]entity.Player, error) {
	var players []entity.Player
	query := r.model(ctx)

	// Apply filters
	if filter.SoldOut {
		query = query.Where("price_current_lowest = 0")
	}
	if filter.PlayerName != "" {
		query = query.Where("p_name_show LIKE ?", "%"+filter.PlayerName+"%")
	}
	if filter.MinPrice > 0 {
		query = query.Where("price_sale_lower >= ?", filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		query = query.Where("price_sale_upper <= ?", filter.MaxPrice)
	}
	if filter.ExFree {
		query = query.Where("team_abbr != ?", "自由球员")
	}
	if len(filter.PlayerIDs) > 0 {
		query = query.Where("player_id IN ?", filter.PlayerIDs)
	}

	// Default filter for list
	query = query.Where("price_standard >= ?", 5000)

	// Sort
	if filter.OrderBy != "" {
		direction := "DESC"
		if filter.OrderAsc {
			direction = "ASC"
		}
		query = query.Order(fmt.Sprintf("%s %s", filter.OrderBy, direction))
	}

	// Pagination
	if filter.Limit > 0 {
		offset := (filter.Page - 1) * filter.Limit
		if offset < 0 {
			offset = 0
		}
		query = query.Offset(offset).Limit(filter.Limit)
	}

	err := query.Find(&players).Error
	return players, err
}

func (r *PlayerRepository) GetByID(ctx context.Context, playerID uint) (*entity.Player, error) {
	var player entity.Player
	err := r.ctx(ctx).Where("player_id = ?", playerID).First(&player).Error
	if err != nil {
		return nil, err
	}
	return &player, nil
}

func (r *PlayerRepository) BatchGetByIDs(ctx context.Context, playerIDs []uint) ([]entity.Player, error) {
	var players []entity.Player
	err := r.ctx(ctx).Where("player_id IN ?", playerIDs).Find(&players).Error
	return players, err
}

func (r *PlayerRepository) UpdatePower(ctx context.Context, playerID uint, power5, power10 float64) error {
	return r.model(ctx).
		Where("player_id = ?", playerID).
		Updates(map[string]interface{}{
			"power_per5":  power5,
			"power_per10": power10,
		}).Error
}

func (r *PlayerRepository) UpdatePriceChanges(ctx context.Context, playerID uint, pc1d, pc7d float64) error {
	return r.model(ctx).
		Where("player_id = ?", playerID).
		Updates(map[string]interface{}{
			"price_change_1d": pc1d,
			"price_change_7d": pc7d,
		}).Error
}

func (r *PlayerRepository) GetAllTargetPlayers(ctx context.Context) ([]entity.Player, error) {
	var players []entity.Player
	err := r.ctx(ctx).Where("nba_player_id > 0 AND team_abbr != ?", "自由球员").Find(&players).Error
	return players, err
}

func (r *PlayerRepository) Transaction(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(fn)
}
