package db

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"o2stock-crawler/internal/model"
	"time"

	"gorm.io/gorm"
)

// UserPlayerOwnQuery 用户球员拥有查询
type UserPlayerOwnQuery struct {
	repo   *repositories.OwnRepository
	userID uint
}

// NewUserPlayerOwnQuery 创建一个 UserPlayerOwnQuery
func NewUserPlayerOwnQuery(database *DB, userID uint) *UserPlayerOwnQuery {
	return &UserPlayerOwnQuery{
		repo:   repositories.NewOwnRepository(database.DB),
		userID: userID,
	}
}

// CountOwnedPlayers 统计用户拥有的指定球员数量
func (q *UserPlayerOwnQuery) CountOwnedPlayers(ctx context.Context, database *DB, playerID uint) (int, error) {
	count, err := q.repo.CountOwned(ctx, q.userID, playerID)
	return int(count), err
}

// GetUserOwnedPlayers 获取用户拥有的所有球员记录
func (q *UserPlayerOwnQuery) GetUserOwnedPlayers(ctx context.Context, database *DB) ([]model.UserPlayerOwn, error) {
	owns, err := q.repo.GetByUserID(ctx, q.userID)
	if err != nil {
		return nil, err
	}

	result := make([]model.UserPlayerOwn, len(owns))
	for i, o := range owns {
		result[i] = q.mapToModel(&o)
	}
	return result, nil
}

// GetOwnedInfoByPlayerIDs 根据球员 ID 列表获取用户的拥有信息
func (q *UserPlayerOwnQuery) GetOwnedInfoByPlayerIDs(ctx context.Context, database *DB, playerIDs []uint) (map[uint][]model.OwnInfo, error) {
	owns, err := q.repo.GetByPlayerIDs(ctx, q.userID, playerIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[uint][]model.OwnInfo)
	for _, o := range owns {
		m := q.mapToModel(&o)
		info := m.ToOwnInfo()
		result[o.PlayerID] = append(result[o.PlayerID], info)
	}
	return result, nil
}

// GetPlayerOwnByRecordID 根据记录 id 查询持仓数据
func (q *UserPlayerOwnQuery) GetPlayerOwnByRecordID(ctx context.Context, database *DB, recordId, uId uint) (*model.UserPlayerOwn, error) {
	own, err := q.repo.GetByRecordID(ctx, recordId, uId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	m := q.mapToModel(own)
	return &m, nil
}

func (q *UserPlayerOwnQuery) mapToModel(o *models.UserPlayerOwn) model.UserPlayerOwn {
	return model.UserPlayerOwn{
		ID:       o.ID,
		UserID:   o.UserID,
		PlayerID: o.PlayerID,
		OwnSta:   uint8(o.Sta),
		PriceIn:  o.BuyPrice,
		PriceOut: o.SellPrice,
		NumIn:    o.BuyCount,
		DtIn:     o.BuyTime,
		DtOut:    o.SellTime,
	}
}

// UserPlayerOwnCommand 用户球员拥有操作
type UserPlayerOwnCommand struct {
	repo *repositories.OwnRepository
}

// NewUserPlayerOwnCommand 创建一个 UserPlayerOwnCommand
func NewUserPlayerOwnCommand(database *DB) *UserPlayerOwnCommand {
	return &UserPlayerOwnCommand{
		repo: repositories.NewOwnRepository(database.DB),
	}
}

// InsertPlayerOwn 插入一条购买记录
func (c *UserPlayerOwnCommand) InsertPlayerOwn(ctx context.Context, database *DB, userID, playerID, num, cost uint, dt time.Time) error {
	return c.repo.Create(ctx, userID, playerID, num, cost, dt)
}

// UpdatePlayerOwnToSold 将已购买的球员标记为已出售
func (c *UserPlayerOwnCommand) UpdatePlayerOwnToSold(ctx context.Context, database *DB, userID, playerID, cost uint, dt time.Time) error {
	err := c.repo.MarkAsSold(ctx, userID, playerID, cost, dt)
	if err != nil {
		return err
	}
	return nil
}

// UpdatePlayerOwn 更新持仓记录
func (c *UserPlayerOwnCommand) UpdatePlayerOwn(ctx context.Context, database *DB, userID, recordId, priceIn, priceOut, num uint, dtIn, dtOut *time.Time) error {
	updates := map[string]interface{}{
		"price_in":  priceIn,
		"price_out": priceOut,
		"num_in":    num,
		"dt_in":     dtIn,
	}
	if dtOut != nil {
		updates["dt_out"] = dtOut
	}
	return c.repo.Update(ctx, userID, recordId, updates)
}

// DeletePlayerOwn 删除持仓记录
func (c *UserPlayerOwnCommand) DeletePlayerOwn(ctx context.Context, database *DB, userID, recordId uint) error {
	return c.repo.Delete(ctx, userID, recordId)
}
