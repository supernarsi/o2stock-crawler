package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"o2stock-crawler/internal/model"
)

// UserQuery 用户查询
type UserQuery struct {
	QueryBase
}

func NewUserQuery() *UserQuery {
	return &UserQuery{QueryBase: QueryBase{}}
}

// GetUserByOpenID 根据 OpenID 获取用户
func (q *UserQuery) GetUserByOpenID(ctx context.Context, database *DB, openID string) (*model.User, error) {
	const query = `
SELECT id, nick, avatar, wx_openid, wx_unionid, sta, c_time
FROM user
WHERE wx_openid = ?`

	var u model.User
	err := database.QueryRowContext(ctx, query, openID).Scan(
		&u.ID, &u.Nick, &u.Avatar, &u.WxOpenID, &u.WxUnionID, &u.Sta, &u.CTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // 用户不存在
		}
		return nil, fmt.Errorf("failed to get user by openid: %w", err)
	}
	return &u, nil
}

// GetUserByID 根据 ID 获取用户
func (q *UserQuery) GetUserByID(ctx context.Context, database *DB, id uint) (*model.User, error) {
	const query = `
SELECT id, nick, avatar, wx_openid, wx_unionid, sta, c_time
FROM user
WHERE id = ?`

	var u model.User
	err := database.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Nick, &u.Avatar, &u.WxOpenID, &u.WxUnionID, &u.Sta, &u.CTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return &u, nil
}

// UserCommand 用户操作
type UserCommand struct {
	DbBase
}

func NewUserCommand() *UserCommand {
	return &UserCommand{DbBase: DbBase{}}
}

// CreateUser 创建新用户
func (c *UserCommand) CreateUser(ctx context.Context, database *DB, u *model.User) error {
	const query = `
INSERT INTO user (nick, avatar, wx_openid, wx_unionid, wx_session_key, sta, c_time)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	res, err := database.ExecContext(ctx, query, u.Nick, u.Avatar, u.WxOpenID, u.WxUnionID, u.WxSessionKey, 1, now)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	u.ID = uint(id)
	u.CTime = now
	u.Sta = 1
	return nil
}

// UpdateUser 更新用户信息
func (c *UserCommand) UpdateUser(ctx context.Context, database *DB, u *model.User) error {
	const query = `
UPDATE user
SET nick = ?, avatar = ?, wx_session_key = ?
WHERE id = ?`

	_, err := database.ExecContext(ctx, query, u.Nick, u.Avatar, u.WxSessionKey, u.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}
