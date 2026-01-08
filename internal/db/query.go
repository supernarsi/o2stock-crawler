package db

import (
	"context"

	"github.com/narsihuang/o2stock-crawler/internal/model"
)

// ListPlayers 返回简单的球员列表，按 player_id 排序，可分页。
func ListPlayers(ctx context.Context, database *DB, limit, offset int) ([]*model.Players, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	const q = `
SELECT player_id, p_name_show, p_name_en, team_abbr, version, card_type,
       player_img, price_standard, price_current_lowest, price_sale_lower, price_sale_upper
FROM players
ORDER BY player_id ASC
LIMIT ? OFFSET ?`

	rows, err := database.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Players
	for rows.Next() {
		var r model.Players
		if err := rows.Scan(
			&r.PlayerID,
			&r.ShowName,
			&r.EnName,
			&r.TeamAbbr,
			&r.Version,
			&r.CardType,
			&r.PlayerImg,
			&r.PriceStandard,
			&r.PriceCurrentLower,
			&r.PriceSaleLower,
			&r.PriceSaleUpper,
		); err != nil {
			return nil, err
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

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
