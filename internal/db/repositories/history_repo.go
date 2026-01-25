package repositories

import (
	"context"
	"o2stock-crawler/internal/db/models"
	"time"

	"gorm.io/gorm"
)

type HistoryRepository struct {
	db *gorm.DB
}

func NewHistoryRepository(db *gorm.DB) *HistoryRepository {
	return &HistoryRepository{db: db}
}

func (r *HistoryRepository) GetByPlayerID(ctx context.Context, playerID uint, startTime time.Time, limit int) ([]models.PlayerPriceHistory, error) {
	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ?", playerID, startTime.Format("200601021504")).
		Order("at_date_hour DESC").
		Limit(limit).
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetPriceHistoryMap(ctx context.Context, startTime time.Time) (map[uint]models.PlayerPriceHistory, error) {
	var results []models.PlayerPriceHistory

	// Complex subquery for MIN(at_date_hour)
	subQuery := r.db.Model(&models.PlayerPriceHistory{}).
		Select("player_id, MIN(at_date_hour) as min_hour").
		Where("at_date_hour >= ?", startTime.Format("200601021504")).
		Group("player_id")

	err := r.db.WithContext(ctx).
		Table("p_p_history p1").
		Joins("INNER JOIN (?) p2 ON p1.player_id = p2.player_id AND p1.at_date_hour = p2.min_hour", subQuery).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	historyMap := make(map[uint]models.PlayerPriceHistory)
	for _, h := range results {
		historyMap[h.PlayerID] = h
	}
	return historyMap, nil
}

func (r *HistoryRepository) GetRawHistory(ctx context.Context, playerID uint, startTime, endTime time.Time) ([]models.PlayerPriceHistory, error) {
	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, startTime.Format("200601021504"), endTime.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetRealtime(ctx context.Context, playerID uint) ([]models.PlayerPriceHistory, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.AddDate(0, 0, 1)

	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, todayStart.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) Get5Days(ctx context.Context, playerID uint) ([]models.PlayerPriceHistory, error) {
	now := time.Now()
	fiveDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -4)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, fiveDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	return history, err
}

func (r *HistoryRepository) GetDailyK(ctx context.Context, playerID uint) ([]models.PlayerPriceHistory, error) {
	now := time.Now()
	thirtyDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -29)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, thirtyDaysAgo.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}

	return r.AggregateDailyKLine(history, thirtyDaysAgo, now), nil
}

func (r *HistoryRepository) GetDays(ctx context.Context, playerID uint, days int) ([]models.PlayerPriceHistory, error) {
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	var history []models.PlayerPriceHistory
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND at_date_hour >= ? AND at_date_hour < ?",
			playerID, startDate.Format("200601021504"), todayEnd.Format("200601021504")).
		Order("at_date ASC, at_date_hour ASC").
		Find(&history).Error
	if err != nil {
		return nil, err
	}

	return r.SelectDailyRecords(history, startDate, now), nil
}

func (r *HistoryRepository) GetMultiPlayersHistory(ctx context.Context, playerIDs []uint, limit int) (map[uint][]models.PlayerPriceHistory, error) {
	if len(playerIDs) == 0 {
		return make(map[uint][]models.PlayerPriceHistory), nil
	}

	var results []models.PlayerPriceHistory
	// ROW_NUMBER() handling with GORM
	subQuery := r.db.Model(&models.PlayerPriceHistory{}).
		Select("p_p_history.*, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY at_date_hour DESC) AS rn").
		Where("player_id IN ?", playerIDs)

	err := r.db.WithContext(ctx).
		Table("(?) as t", subQuery).
		Where("rn <= ?", limit).
		Order("at_date_hour ASC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	resMap := make(map[uint][]models.PlayerPriceHistory)
	for _, h := range results {
		resMap[h.PlayerID] = append(resMap[h.PlayerID], h)
	}
	return resMap, nil
}

func (r *HistoryRepository) AggregateDailyKLine(rows []models.PlayerPriceHistory, startDate, endDate time.Time) []models.PlayerPriceHistory {
	if len(rows) == 0 {
		return []models.PlayerPriceHistory{}
	}

	dateGroups := make(map[string][]models.PlayerPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}

	result := make([]models.PlayerPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]

		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		var openRow, closeRow, highRow, lowRow *models.PlayerPriceHistory
		var highPrice, lowPrice int

		openRow = &dayRows[0]
		closeRow = &dayRows[len(dayRows)-1]

		highPrice = -1
		lowPrice = 2147483647

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

		uniqueMap := make(map[string]*models.PlayerPriceHistory)
		uniqueMap[openRow.AtDateHour] = openRow
		uniqueMap[lowRow.AtDateHour] = lowRow
		uniqueMap[highRow.AtDateHour] = highRow
		uniqueMap[closeRow.AtDateHour] = closeRow

		dayK := make([]models.PlayerPriceHistory, 0, len(uniqueMap))
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

func (r *HistoryRepository) SelectDailyRecords(rows []models.PlayerPriceHistory, startDate, endDate time.Time) []models.PlayerPriceHistory {
	if len(rows) == 0 {
		return []models.PlayerPriceHistory{}
	}

	dateGroups := make(map[string][]models.PlayerPriceHistory)
	for _, row := range rows {
		dateKey := row.AtDate.Format("2006-01-02")
		dateGroups[dateKey] = append(dateGroups[dateKey], row)
	}

	result := make([]models.PlayerPriceHistory, 0)
	currentDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	endDateDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	for currentDate.Before(endDateDay) || currentDate.Equal(endDateDay) {
		dateKey := currentDate.Format("2006-01-02")
		dayRows, hasData := dateGroups[dateKey]
		if !hasData || len(dayRows) == 0 {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		var selected *models.PlayerPriceHistory
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

func (r *HistoryRepository) Create(ctx context.Context, history *models.PlayerPriceHistory) error {
	return r.db.WithContext(ctx).Create(history).Error
}
