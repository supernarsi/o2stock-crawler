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

// GetMultiPlayersHistory 批量获取多个球员的历史价格
func GetMultiPlayersHistory(ctx context.Context, database *DB, playerIDs []uint32, limit int) (map[uint32][]*model.PriceHistoryRow, error) {
	if len(playerIDs) == 0 {
		return make(map[uint32][]*model.PriceHistoryRow), nil
	}

	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	// 构建 IN 查询的占位符
	placeholders := ""
	args := make([]interface{}, 0, len(playerIDs))
	for i, pid := range playerIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, pid)
	}

	q := `
SELECT player_id, at_date, at_date_hour, at_year, at_month, at_day, at_hour, price_standard, price_lower, price_upper
FROM p_p_history
WHERE player_id IN (` + placeholders + `)
ORDER BY player_id, at_date, at_hour
LIMIT ?`

	// todo 待优化：这里 LIMIT 是全局的，不是每个球员的
	// 如果需要每个球员单独限制，需要更复杂的查询
	args = append(args, limit*len(playerIDs)) // 为每个球员预留 limit 条记录

	rows, err := database.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uint32][]*model.PriceHistoryRow)
	for rows.Next() {
		var r model.PriceHistoryRow
		if err := rows.Scan(&r.PlayerId, &r.AtDate, &r.AtDateHourStr, &r.AtYear, &r.AtMonth, &r.AtDay, &r.AtHour, &r.PriceStandard, &r.PriceLower, &r.PriceUpper); err != nil {
			return nil, err
		}
		playerID := uint32(r.PlayerId)
		result[playerID] = append(result[playerID], &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 限制每个球员的记录数
	for pid := range result {
		if len(result[pid]) > limit {
			result[pid] = result[pid][:limit]
		}
	}

	return result, nil
}
