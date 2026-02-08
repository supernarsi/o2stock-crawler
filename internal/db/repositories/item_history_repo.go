package repositories

import (
	"context"
	"math"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type ItemHistoryRepository struct {
	baseRepository[entity.ItemPriceHistory]
}

func NewItemHistoryRepository(db *gorm.DB) *ItemHistoryRepository {
	return &ItemHistoryRepository{
		baseRepository: baseRepository[entity.ItemPriceHistory]{db: db},
	}
}

// GetByItemID 按 item_id 查询价格历史，用于 GET /item-history（兼容旧逻辑，默认 24h + limit）
func (r *ItemHistoryRepository) GetByItemID(ctx context.Context, itemID uint, startTime time.Time, limit int) ([]entity.ItemPriceHistory, error) {
	var history []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Where("item_id = ? AND at_date_hour >= ?", itemID, startTime.Format("200601021504")).
		Order("at_date_hour ASC").
		Limit(limit).
		Find(&history).Error
	return history, err
}

// GetRealtime 近 24 小时分时数据（与球员 GetRealtime 一致）
func (r *ItemHistoryRepository) GetRealtime(ctx context.Context, itemID uint) ([]entity.ItemPriceHistory, error) {
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	var history []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Where("item_id = ? AND at_date_hour >= ?", itemID, startTime.Format("200601021504")).
		Order("at_date_hour ASC").
		Find(&history).Error
	return history, err
}

// Get5Days 近 5 日数据（当日 0 点起 -4 天至今日结束）
func (r *ItemHistoryRepository) Get5Days(ctx context.Context, itemID uint) ([]entity.ItemPriceHistory, error) {
	now := time.Now()
	fiveDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -4)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	var history []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Where("item_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			itemID, fiveDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	return history, err
}

// GetDays 近 N 日数据（当日 0 点起 -(days-1) 天至今日结束），每日取一条（12:00 或最近）
func (r *ItemHistoryRepository) GetDays(ctx context.Context, itemID uint, days int) ([]entity.ItemPriceHistory, error) {
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	var history []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Where("item_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			itemID, startDate.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}
	return r.selectDailyRecords(history, startDate, now), nil
}

// GetDailyK 近 30 日日 K（开高低收聚合）
func (r *ItemHistoryRepository) GetDailyK(ctx context.Context, itemID uint) ([]entity.ItemPriceHistory, error) {
	now := time.Now()
	thirtyDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	var history []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Where("item_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			itemID, thirtyDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}
	return r.aggregateDailyKLine(history, thirtyDaysAgo, now), nil
}

// aggregateDailyKLine 按日聚合为日 K（开、高、低、收），与球员 history_repo 逻辑一致
func (r *ItemHistoryRepository) aggregateDailyKLine(rows []entity.ItemPriceHistory, startDate, endDate time.Time) []entity.ItemPriceHistory {
	if len(rows) == 0 {
		return []entity.ItemPriceHistory{}
	}
	dateGroups := make(map[string][]entity.ItemPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}
	result := make([]entity.ItemPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())
	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]
		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}
		openRow := &dayRows[0]
		closeRow := &dayRows[len(dayRows)-1]
		highPrice := -1
		lowPrice := math.MaxInt
		var highRow, lowRow *entity.ItemPriceHistory
		for i := range dayRows {
			row := &dayRows[i]
			price := int(row.PriceCurrentSale)
			if price <= 0 {
				price = int(row.PriceStandard)
			}
			if price > highPrice {
				highPrice = price
				highRow = row
			}
			if price < lowPrice {
				lowPrice = price
				lowRow = row
			}
		}
		uniqueMap := make(map[string]*entity.ItemPriceHistory)
		uniqueMap[openRow.AtDateHour] = openRow
		uniqueMap[closeRow.AtDateHour] = closeRow
		if highRow != nil {
			uniqueMap[highRow.AtDateHour] = highRow
		}
		if lowRow != nil {
			uniqueMap[lowRow.AtDateHour] = lowRow
		}
		dayK := make([]entity.ItemPriceHistory, 0, len(uniqueMap))
		for _, v := range uniqueMap {
			dayK = append(dayK, *v)
		}
		for i := 0; i < len(dayK)-1; i++ {
			for j := i + 1; j < len(dayK); j++ {
				if dayK[i].AtDateHour > dayK[j].AtDateHour {
					dayK[i], dayK[j] = dayK[j], dayK[i]
				}
			}
		}
		result = append(result, dayK...)
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return result
}

// selectDailyRecords 每日取一条：优先 12:00，否则取当日最后一条
func (r *ItemHistoryRepository) selectDailyRecords(rows []entity.ItemPriceHistory, startDate, endDate time.Time) []entity.ItemPriceHistory {
	if len(rows) == 0 {
		return []entity.ItemPriceHistory{}
	}
	dateGroups := make(map[string][]entity.ItemPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}
	result := make([]entity.ItemPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())
	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]
		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}
		targetStr := currentDate.Format("20060102") + "1200"
		var selected *entity.ItemPriceHistory
		for i := range dayRows {
			if dayRows[i].AtDateHour >= targetStr {
				selected = &dayRows[i]
				break
			}
		}
		if selected == nil {
			selected = &dayRows[len(dayRows)-1]
		}
		result = append(result, *selected)
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return result
}

// GetPriceHistoryMap 取 at_date_hour >= startTime 时每个 item_id 最小 at_date_hour 的一条，用于计算 1d/7d 基准价
func (r *ItemHistoryRepository) GetPriceHistoryMap(ctx context.Context, startTime time.Time) (map[uint]entity.ItemPriceHistory, error) {
	startStr := startTime.Format("200601021504")
	subQuery := r.model(ctx).
		Select("item_id, MIN(at_date_hour) as min_hour").
		Where("at_date_hour >= ?", startStr).
		Group("item_id")

	var results []entity.ItemPriceHistory
	err := r.ctx(ctx).
		Table("p_i_history p1").
		Joins("INNER JOIN (?) p2 ON p1.item_id = p2.item_id AND p1.at_date_hour = p2.min_hour", subQuery).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	m := make(map[uint]entity.ItemPriceHistory, len(results))
	for _, h := range results {
		m[h.ItemID] = h
	}
	return m, nil
}
