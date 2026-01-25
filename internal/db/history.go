package db

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"o2stock-crawler/internal/db/repositories"
	"time"
)

// HistoryQuery 球员价格历史查询
type HistoryQuery struct {
	repo     *repositories.HistoryRepository
	playerID uint
}

// NewHistoryQuery 创建一个 HistoryQuery
func NewHistoryQuery(database *DB, playerID uint) *HistoryQuery {
	return &HistoryQuery{
		repo:     repositories.NewHistoryRepository(database.DB),
		playerID: playerID,
	}
}

// GetPlayerHistory 返回某个球员的历史价格
func (q *HistoryQuery) GetPlayerHistory(ctx context.Context, period uint8) ([]models.PlayerPriceHistory, error) {
	startTime := q.calculateStartTime(period)
	return q.repo.GetByPlayerID(ctx, q.playerID, startTime, 1000)
}

func (q *HistoryQuery) calculateStartTime(period uint8) time.Time {
	now := time.Now()
	switch period {
	case Period1Day:
		return now.AddDate(0, 0, -1)
	case Period3Days:
		return now.AddDate(0, 0, -3)
	case Period1Week:
		return now.AddDate(0, 0, -7)
	default:
		return now.AddDate(0, 0, -1)
	}
}

// HistoryCommand 历史记录操作
type HistoryCommand struct {
	repo *repositories.HistoryRepository
}

func NewHistoryCommand(database *DB) *HistoryCommand {
	return &HistoryCommand{repo: repositories.NewHistoryRepository(database.DB)}
}

// SaveHistory 保存价格历史数据
func (c *HistoryCommand) SaveHistory(ctx context.Context, playerID uint, priceStandard uint, currentLowest int, priceLower, priceUpper uint, now time.Time) error {
	h := &models.PlayerPriceHistory{
		PlayerID:         playerID,
		AtDate:           now,
		AtDateHour:       now.Format("200601021504"),
		AtYear:           now.Format("2006"),
		AtMonth:          now.Format("01"),
		AtDay:            now.Format("02"),
		AtHour:           now.Format("15"),
		AtMinute:         now.Format("04"),
		PriceStandard:    priceStandard,
		PriceCurrentSale: currentLowest,
		PriceLower:       priceLower,
		PriceUpper:       priceUpper,
		CTime:            now,
	}
	return c.repo.Create(ctx, h)
}
