package db

import (
	"context"
	"o2stock-crawler/internal/db/repositories"
)

// FavQuery 用户收藏查询
type FavQuery struct {
	repo *repositories.FavRepository
}

func NewFavQuery(database *DB) *FavQuery {
	return &FavQuery{repo: repositories.NewFavRepository(database.DB)}
}

// CountFavPlayer 统计用户收藏的指定球员数量
func (q *FavQuery) CountFavPlayer(ctx context.Context, userID, playerID uint) (int, error) {
	count, err := q.repo.Count(ctx, userID, playerID)
	return int(count), err
}

// CountUserFavs 统计用户收藏的总球员数量
func (q *FavQuery) CountUserFavs(ctx context.Context, userID uint) (int, error) {
	count, err := q.repo.CountUserTotal(ctx, userID)
	return int(count), err
}

// FavCommand 用户收藏操作
type FavCommand struct {
	repo *repositories.FavRepository
}

func NewFavCommand(database *DB) *FavCommand {
	return &FavCommand{repo: repositories.NewFavRepository(database.DB)}
}

// InsertFavPlayer 插入一条收藏记录
func (c *FavCommand) InsertFavPlayer(ctx context.Context, userID, playerID uint) error {
	return c.repo.Add(ctx, userID, playerID)
}

// DeleteFavPlayer 删除一条收藏记录
func (c *FavCommand) DeleteFavPlayer(ctx context.Context, userID, playerID uint) error {
	return c.repo.Remove(ctx, userID, playerID)
}

// GetFavPlayerIDs 获取用户收藏的所有球员ID
func (q *FavQuery) GetFavPlayerIDs(ctx context.Context, userID uint) ([]uint, error) {
	return q.repo.GetPlayerIDs(ctx, userID)
}

// GetFavMapByPlayerIDs 批量获取用户对指定球员的收藏状态
func (q *FavQuery) GetFavMapByPlayerIDs(ctx context.Context, userID uint, playerIDs []uint) (map[uint]bool, error) {
	return q.repo.GetFavMap(ctx, userID, playerIDs)
}
