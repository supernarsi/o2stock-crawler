package db

import (
	"context"
	"o2stock-crawler/internal/model"
)

// GetPlayerHistory 返回某个球员的历史价格，按时间升序。
func GetPlayerHistory(ctx context.Context, database *DB, playerID uint32, limit int) ([]*model.PriceHistoryRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	const q = `
SELECT player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, price_standard, price_lower, price_upper
FROM p_p_history
WHERE player_id = ?
ORDER BY at_date, at_hour
LIMIT ?`

	rows, err := database.QueryContext(ctx, q, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.PriceHistoryRow
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := rows.Scan(&r.PlayerId, &r.AtDate, &r.AtDateHourStr, &r.AtYear, &r.AtMonth, &r.AtDay, &r.AtHour, &r.PriceStandard, &r.PriceLower, &r.PriceUpper); err != nil {
			return nil, err
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
