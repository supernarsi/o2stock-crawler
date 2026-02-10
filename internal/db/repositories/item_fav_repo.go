package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type ItemFavRepository struct {
	baseRepository[entity.UserItemFav]
}

func NewItemFavRepository(db *gorm.DB) *ItemFavRepository {
	return &ItemFavRepository{
		baseRepository: baseRepository[entity.UserItemFav]{db: db},
	}
}

func (r *ItemFavRepository) Count(ctx context.Context, userID, itemID uint) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ? AND item_id = ?", userID, itemID).
		Count(&count).Error
	return count, err
}

func (r *ItemFavRepository) CountUserTotal(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.model(ctx).
		Where("uid = ?", userID).
		Count(&count).Error
	return count, err
}

func (r *ItemFavRepository) Add(ctx context.Context, userID, itemID uint) error {
	fav := entity.UserItemFav{
		UserID: userID,
		ItemID: itemID,
	}
	return r.ctx(ctx).Create(&fav).Error
}

func (r *ItemFavRepository) Remove(ctx context.Context, userID, itemID uint) error {
	return r.model(ctx).
		Where("uid = ? AND item_id = ?", userID, itemID).
		Delete(&entity.UserItemFav{}).Error
}

func (r *ItemFavRepository) GetItemIDs(ctx context.Context, userID uint) ([]uint, error) {
	var itemIDs []uint
	err := r.model(ctx).
		Where("uid = ?", userID).
		Pluck("item_id", &itemIDs).Error
	return itemIDs, err
}

func (r *ItemFavRepository) GetFavMap(ctx context.Context, userID uint, itemIDs []uint) (map[uint]bool, error) {
	if len(itemIDs) == 0 {
		return make(map[uint]bool), nil
	}
	var fids []uint
	err := r.model(ctx).
		Where("uid = ? AND item_id IN ?", userID, itemIDs).
		Pluck("item_id", &fids).Error
	if err != nil {
		return nil, err
	}
	favMap := make(map[uint]bool)
	for _, id := range fids {
		favMap[id] = true
	}
	return favMap, nil
}
