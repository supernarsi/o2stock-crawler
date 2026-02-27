package repositories

import (
	"context"
	"time"

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

func (r *UserRepository) UpdateLoginTime(ctx context.Context, id uint, loginTime time.Time) error {
	return r.ctx(ctx).Model(&entity.User{}).Where("id = ?", id).UpdateColumn("login_time", loginTime).Error
}

// GetByIDs 批量根据用户 ID 获取用户，返回 map[uint]*entity.User
func (r *UserRepository) GetByIDs(ctx context.Context, ids []uint) (map[uint]*entity.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []entity.User
	if err := r.ctx(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]*entity.User, len(users))
	for i := range users {
		out[users[i].ID] = &users[i]
	}
	return out, nil
}
