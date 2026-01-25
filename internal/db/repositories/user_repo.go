package repositories

import (
	"context"
	"o2stock-crawler/internal/entity"

	"gorm.io/gorm"
)

type UserRepository struct {
	baseRepository[entity.User]
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		baseRepository: baseRepository[entity.User]{db: db},
	}
}

func (r *UserRepository) GetByOpenID(ctx context.Context, openID string) (*entity.User, error) {
	var user entity.User
	err := r.ctx(ctx).Where("wx_openid = ?", openID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uint) (*entity.User, error) {
	var user entity.User
	err := r.ctx(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	return r.ctx(ctx).Create(user).Error
}

func (r *UserRepository) Update(ctx context.Context, user *entity.User) error {
	return r.ctx(ctx).Save(user).Error
}
