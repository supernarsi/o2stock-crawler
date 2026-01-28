package repositories

import (
	"context"
	"math"
	"o2stock-crawler/internal/entity"
	"time"

	"gorm.io/gorm"
)

type HistoryRepository struct {
	baseRepository[entity.PlayerPriceHistory]
}

func NewHistoryRepository(db *gorm.DB) *HistoryRepository {
	return &HistoryRepository{
		baseRepository: baseRepository[entity.PlayerPriceHistory]{db: db},
	}
}

func (r *HistoryRepository) GetByPlayerID(ctx context.Context, playerID uint, startTime time.Time, limit int) ([]entity.PlayerPriceHistory, error) {
	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ?", playerID, startTime.Format("200601021504")).
		Order("at_date_hour DESC").
		Limit(limit).
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetPriceHistoryMap(ctx context.Context, startTime time.Time) (map[uint]entity.PlayerPriceHistory, error) {
	var results []entity.PlayerPriceHistory

	// Complex subquery for MIN(at_date_hour)
	subQuery := r.model(ctx).
		Select("player_id, MIN(at_date_hour) as min_hour").
		Where("at_date_hour >= ?", startTime.Format("200601021504")).
		Group("player_id")

	err := r.ctx(ctx).
		Table("p_p_history p1").
		Joins("INNER JOIN (?) p2 ON p1.player_id = p2.player_id AND p1.at_date_hour = p2.min_hour", subQuery).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	historyMap := make(map[uint]entity.PlayerPriceHistory)
	for _, h := range results {
		historyMap[h.PlayerID] = h
	}
	return historyMap, nil
}

func (r *HistoryRepository) GetRawHistory(ctx context.Context, playerID uint, startTime, endTime time.Time) ([]entity.PlayerPriceHistory, error) {
	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, startTime.Format("200601021504"), endTime.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetRealtime(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error) {
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)

	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ?",
			playerID, startTime.Format("200601021504")).
		Order("at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) Get5Days(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error) {
	now := time.Now()
	fiveDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -4)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, fiveDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetDailyK(ctx context.Context, playerID uint) ([]entity.PlayerPriceHistory, error) {
	now := time.Now()
	thirtyDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, thirtyDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}

	return r.AggregateDailyKLine(history, thirtyDaysAgo, now), nil
}

func (r *HistoryRepository) GetDays(ctx context.Context, playerID uint, days int) ([]entity.PlayerPriceHistory, error) {
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []entity.PlayerPriceHistory
	err := r.ctx(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, startDate.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}

	return r.SelectDailyRecords(history, startDate, now), nil
}

func (r *HistoryRepository) GetMultiPlayersHistory(ctx context.Context, playerIDs []uint, limit int) (map[uint][]entity.PlayerPriceHistory, error) {
	if len(playerIDs) == 0 {
		return make(map[uint][]entity.PlayerPriceHistory), nil
	}

	var results []entity.PlayerPriceHistory
	// ROW_NUMBER() handling with GORM
	subQuery := r.model(ctx).
		Select("p_p_history.*, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY at_date_hour DESC) AS rn").
		Where("player_id IN ?", playerIDs)

	err := r.ctx(ctx).
		Table("(?) as t", subQuery).
		Where("rn <= ?", limit).
		Order("at_date_hour ASC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	resMap := make(map[uint][]entity.PlayerPriceHistory)
	for _, h := range results {
		resMap[h.PlayerID] = append(resMap[h.PlayerID], h)
	}
	return resMap, nil
}

func (r *HistoryRepository) AggregateDailyKLine(rows []entity.PlayerPriceHistory, startDate, endDate time.Time) []entity.PlayerPriceHistory {
	if len(rows) == 0 {
		return []entity.PlayerPriceHistory{}
	}

	dateGroups := make(map[string][]entity.PlayerPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}

	result := make([]entity.PlayerPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]

		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		var openRow, closeRow, highRow, lowRow *entity.PlayerPriceHistory
		var highPrice, lowPrice int

		openRow = &dayRows[0]
		closeRow = &dayRows[len(dayRows)-1]

		highPrice = -1
		lowPrice = math.MaxInt

		for i := range dayRows {
			row := &dayRows[i]
			price := int(row.PriceStandard)
			if row.PriceCurrentSale >= 0 {
				price = row.PriceCurrentSale
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

		uniqueMap := make(map[string]*entity.PlayerPriceHistory)
		uniqueMap[openRow.AtDateHour] = openRow
		uniqueMap[lowRow.AtDateHour] = lowRow
		uniqueMap[highRow.AtDateHour] = highRow
		uniqueMap[closeRow.AtDateHour] = closeRow

		dayK := make([]entity.PlayerPriceHistory, 0, len(uniqueMap))
		for _, v := range uniqueMap {
			dayK = append(dayK, *v)
		}

		// Sort by time
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

func (r *HistoryRepository) SelectDailyRecords(rows []entity.PlayerPriceHistory, startDate, endDate time.Time) []entity.PlayerPriceHistory {
	if len(rows) == 0 {
		return []entity.PlayerPriceHistory{}
	}

	dateGroups := make(map[string][]entity.PlayerPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}

	result := make([]entity.PlayerPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]
		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		var selected *entity.PlayerPriceHistory
		targetStr := currentDate.Format("20060102") + "1200"
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

func (r *HistoryRepository) Create(ctx context.Context, history *entity.PlayerPriceHistory) error {
	return r.ctx(ctx).Create(history).Error
}
