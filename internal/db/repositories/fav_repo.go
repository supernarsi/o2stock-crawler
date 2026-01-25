package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type FavRepository struct {
	baseRepository[entity.UserPlayerFav]
}

func NewFavRepository(db *gorm.DB) *FavRepository {
	return &FavRepository{
		baseRepository: baseRepository[entity.UserPlayerFav]{db: db},
	}
}

func (r *FavRepository) Count(ctx context.Context, userID, playerID uint) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ? AND pid = ?", userID, playerID).
		Count(&count).Error
	return count, err
}

func (r *FavRepository) CountUserTotal(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ?", userID).
		Count(&count).Error
	return count, err
}

func (r *FavRepository) Add(ctx context.Context, userID, playerID uint) error {
	fav := entity.UserPlayerFav{
		UserID:   userID,
		PlayerID: playerID,
	}
	return r.ctx(ctx).Create(&fav).Error
}

func (r *FavRepository) Remove(ctx context.Context, userID, playerID uint) error {
	return r.model(ctx).
		Where("uid = ? AND pid = ?", userID, playerID).
		Delete(&entity.UserPlayerFav{}).Error
}

func (r *FavRepository) GetPlayerIDs(ctx context.Context, userID uint) ([]uint, error) {
	var pids []uint
	err := r.model(ctx).
		Where("uid = ?", userID).
		Pluck("pid", &pids).Error
	return pids, err
}

func (r *FavRepository) GetFavMap(ctx context.Context, userID uint, playerIDs []uint) (map[uint]bool, error) {
	var pids []uint
	err := r.model(ctx).
		Where("uid = ? AND pid IN ?", userID, playerIDs).
		Pluck("pid", &pids).Error
	if err != nil {
		return nil, err
	}
	favMap := make(map[uint]bool)
	for _, pid := range pids {
		favMap[pid] = true
	}
	return favMap, nil
}
