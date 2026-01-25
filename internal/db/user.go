package db

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"time"

	"gorm.io/gorm"
)

// UserQuery 用户查询
type UserQuery struct {
	repo *repositories.UserRepository
}

func NewUserQuery(database *DB) *UserQuery {
	return &UserQuery{repo: repositories.NewUserRepository(database.DB)}
}

// GetUserByOpenID 根据 OpenID 获取用户
func (q *UserQuery) GetUserByOpenID(ctx context.Context, database *DB, openID string) (*models.User, error) {
	user, err := q.repo.GetByOpenID(ctx, openID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// GetUserByID 根据 ID 获取用户
func (q *UserQuery) GetUserByID(ctx context.Context, database *DB, id uint) (*models.User, error) {
	user, err := q.repo.GetByID(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// UserCommand 用户操作
type UserCommand struct {
	repo *repositories.UserRepository
}

func NewUserCommand(database *DB) *UserCommand {
	return &UserCommand{repo: repositories.NewUserRepository(database.DB)}
}

// CreateUser 创建新用户
func (c *UserCommand) CreateUser(ctx context.Context, database *DB, u *models.User) error {
	u.Sta = 1
	u.CTime = time.Now()
	return c.repo.Create(ctx, u)
}

// UpdateUser 更新用户信息
func (c *UserCommand) UpdateUser(ctx context.Context, database *DB, u *models.User) error {
	return c.repo.Update(ctx, u)
}
